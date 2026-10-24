package gobackupcleaner

import (
	"os"
	"time"
)

// CleanBackup cleans backup files based on the specified configuration
func CleanBackup(dirPath string, config CleaningConfig) (CleaningReport, error) {
	return CleanBackups([]string{dirPath}, config)
}

// CleanBackups cleans several backup directories as one pool: files from all
// directories are ordered by modification time together, and the oldest ones are
// deleted first, wherever they live. Use it when several backup trees share one
// disk, so that one tree is not emptied while another keeps older files.
//
// Disk usage is taken from the first directory. With MinFreeSpace or
// MaxUsagePercent, all directories must be on the same filesystem
// (ErrDifferentFilesystems otherwise). The directories themselves are never removed.
func CleanBackups(dirPaths []string, config CleaningConfig) (CleaningReport, error) {
	startTime := time.Now()

	// Set defaults and validate configuration
	config.setDefaults()
	if err := config.validate(); err != nil {
		return CleaningReport{}, err
	}
	if len(dirPaths) == 0 {
		return CleaningReport{}, ErrInvalidConfig
	}

	// Check if directories exist
	for _, dirPath := range dirPaths {
		if _, err := os.Stat(dirPath); err != nil {
			if os.IsNotExist(err) {
				return CleaningReport{}, ErrDirectoryNotFound
			}
			return CleaningReport{}, err
		}
	}
	if len(dirPaths) > 1 && (config.MinFreeSpace != nil || config.MaxUsagePercent != nil) {
		if err := checkSameFilesystem(dirPaths, config.DiskInfo); err != nil {
			return CleaningReport{}, err
		}
	}
	dirPath := dirPaths[0]

	// Get current disk usage
	currentUsage, err := config.DiskInfo.GetDiskUsage(dirPath)
	var diskUsageError error
	if err != nil {
		// Save the error for later
		diskUsageError = err
		// Check if we can proceed without disk usage
		if config.MaxSize == nil {
			// Can't proceed without disk usage when only MaxUsagePercent or MinFreeSpace is specified
			return CleaningReport{}, err
		}
	}

	// Calculate target deletion size
	var targetSize int64
	if diskUsageError != nil && config.MaxSize != nil {
		// Special case: can't get disk usage but MaxSize is specified
		// In this case, we'll scan all files and delete until total size is under MaxSize
		// This allows the cleaner to work in environments where disk usage APIs are not available
		// (e.g., restricted permissions, network storage, etc.)
		targetSize = -1 // Special value to indicate "scan and delete until under MaxSize"
	} else {
		targetSize = calculateTargetSize(currentUsage, &config)
		if targetSize <= 0 {
			// No need to delete anything
			return CleaningReport{
				TotalDuration: time.Since(startTime),
				DryRun:        config.DryRun,
			}, nil
		}
	}

	// Get block size
	blockSize, err := config.DiskInfo.GetBlockSize(dirPath)
	if err != nil {
		return CleaningReport{}, err
	}

	// Call OnStart callback
	if currentUsage != nil || targetSize == -1 {
		var usage DiskUsage
		if currentUsage != nil {
			usage = *currentUsage
		}
		callSafe(config.Callbacks.OnStart, StartInfo{
			TargetDir:    dirPath,
			TargetDirs:   dirPaths,
			CurrentUsage: usage,
			TargetSize:   targetSize,
		})
	}

	// Phase 1: Scan files
	scanStartTime := time.Now()
	scanner := newScanner(&config, blockSize)
	for i, root := range dirPaths {
		scanner.root = i
		if err := scanner.scan(root); err != nil {
			return CleaningReport{}, err
		}
	}

	// Get sorted time slots
	timeSlots := scanner.getTimeSlots()
	if len(timeSlots) == 0 {
		// No files found
		return CleaningReport{
			ScanDuration:  time.Since(scanStartTime),
			TotalDuration: time.Since(startTime),
			DryRun:        config.DryRun,
		}, nil
	}

	// Calculate deletion threshold
	var threshold time.Time
	var estimatedFiles int
	var estimatedSize int64

	if targetSize == -1 && config.MaxSize != nil {
		// Special case: delete until total size is under MaxSize
		threshold, estimatedFiles, estimatedSize = calculateThresholdForMaxSize(timeSlots, *config.MaxSize, config.TimeWindow)
	} else {
		threshold, estimatedFiles, estimatedSize = calculateThreshold(timeSlots, targetSize, config.TimeWindow)
	}
	scanDuration := time.Since(scanStartTime)

	// Call OnScanComplete callback
	callSafe(config.Callbacks.OnScanComplete, ScanCompleteInfo{
		ScannedFiles:  scanner.getTotalFiles(),
		TotalSize:     getTotalSize(timeSlots),
		BlockSize:     blockSize,
		TimeThreshold: threshold,
		ScanDuration:  scanDuration,
	})

	roots := make([]RootReport, len(dirPaths))
	for i, root := range dirPaths {
		roots[i].Dir = root
	}
	for _, slot := range timeSlots {
		for _, f := range slot.files {
			roots[f.root].ScannedFiles++
			roots[f.root].ScannedSize += f.size
		}
	}

	// Dry run: report what would be deleted and stop
	if config.DryRun {
		report := CleaningReport{
			ScanDuration:  scanDuration,
			TotalDuration: time.Since(startTime),
			ScannedFiles:  scanner.getTotalFiles(),
			TimeThreshold: threshold,
			BlockSize:     blockSize,
			DryRun:        true,
		}
		for _, slot := range timeSlots {
			for _, f := range slot.files {
				if !f.modTime.Before(threshold) {
					continue
				}
				report.DeletedFiles++
				report.DeletedSize += f.size
				report.DeletedBlockSize += f.blockSize
				roots[f.root].DeletedFiles++
				roots[f.root].DeletedSize += f.size
			}
		}
		report.Roots = roots
		return report, nil
	}

	// Phase 2: Delete files
	deleteStartTime := time.Now()

	// Call OnDeleteStart callback
	callSafe(config.Callbacks.OnDeleteStart, DeleteStartInfo{
		EstimatedFiles: estimatedFiles,
		EstimatedSize:  estimatedSize,
	})

	deleter := newDeleter(&config, blockSize, dirPaths)
	for i, root := range dirPaths {
		deleter.root = i
		if err := deleter.deleteFiles(root, threshold); err != nil {
			return CleaningReport{}, err
		}
	}

	// Phase 3: Delete empty directories
	deletedDirs, _ := deleter.deleteEmptyDirs()
	// Ignore error as it's non-fatal for directory deletion

	deleteDuration := time.Since(deleteStartTime)
	deletedFiles, deletedSize, deletedBlocks := deleter.getStats()
	for i := range roots {
		roots[i].DeletedFiles = deleter.rootFiles[i]
		roots[i].DeletedSize = deleter.rootSize[i]
	}

	// Call OnComplete callback
	callSafe(config.Callbacks.OnComplete, CompleteInfo{
		DeletedFiles:     deletedFiles,
		DeletedSize:      deletedSize,
		DeletedBlockSize: deletedBlocks,
		DeletedDirs:      deletedDirs,
		DeleteDuration:   deleteDuration,
	})

	// Create report
	return CleaningReport{
		DeletedFiles:     deletedFiles,
		DeletedSize:      deletedSize,
		DeletedBlockSize: deletedBlocks,
		DeletedDirs:      deletedDirs,
		ScanDuration:     scanDuration,
		DeleteDuration:   deleteDuration,
		TotalDuration:    time.Since(startTime),
		ScannedFiles:     scanner.getTotalFiles(),
		TimeThreshold:    threshold,
		BlockSize:        blockSize,
		Roots:            roots,
	}, nil
}

// calculateTargetSize calculates how much space needs to be freed
func calculateTargetSize(usage *DiskUsage, config *CleaningConfig) int64 {
	var targetSize int64

	// Check MaxSize
	if config.MaxSize != nil {
		currentSize := int64(usage.Used)
		if currentSize > *config.MaxSize {
			size := currentSize - *config.MaxSize
			if size > targetSize {
				targetSize = size
			}
		}
	}

	// Check MaxUsagePercent
	if config.MaxUsagePercent != nil {
		if usage.UsedPercent > *config.MaxUsagePercent {
			targetUsage := uint64(float64(usage.Total) * (*config.MaxUsagePercent / 100))
			if usage.Used > targetUsage {
				size := int64(usage.Used - targetUsage)
				if size > targetSize {
					targetSize = size
				}
			}
		}
	}

	// Check MinFreeSpace
	if config.MinFreeSpace != nil {
		currentFree := int64(usage.Free)
		if currentFree < *config.MinFreeSpace {
			size := *config.MinFreeSpace - currentFree
			if size > targetSize {
				targetSize = size
			}
		}
	}

	return targetSize
}

// calculateThreshold calculates the time threshold for deletion
func calculateThreshold(slots []*timeSlot, targetSize int64, window time.Duration) (time.Time, int, int64) {
	var accumulatedSize int64
	var accumulatedFiles int
	var threshold time.Time

	// If no slots, return zero time
	if len(slots) == 0 {
		return time.Time{}, 0, 0
	}

	// Set initial threshold to the latest time + 1 second
	// (so nothing gets deleted by default)
	threshold = slots[len(slots)-1].time.Add(time.Second)

	for _, slot := range slots {
		accumulatedSize += slot.totalBlockSize
		accumulatedFiles += len(slot.files)

		if accumulatedSize >= targetSize {
			// We've reached the target size
			// Include all files up to and including this slot
			// (files in a slot have modTime in [slot.time, slot.time+window))
			threshold = slot.time.Add(window)
			break
		}
	}

	return threshold, accumulatedFiles, accumulatedSize
}

// getTotalSize calculates the total size from time slots
func getTotalSize(slots []*timeSlot) int64 {
	var total int64
	for _, slot := range slots {
		total += slot.totalSize
	}
	return total
}

// calculateThresholdForMaxSize calculates the time threshold when total size must be under maxSize
func calculateThresholdForMaxSize(slots []*timeSlot, maxSize int64, window time.Duration) (time.Time, int, int64) {
	var totalSize int64
	var remainingSize int64
	var deleteFiles int
	var deleteSize int64

	// Calculate total size
	for _, slot := range slots {
		totalSize += slot.totalBlockSize
	}

	// If already under maxSize, no need to delete
	if totalSize <= maxSize {
		return time.Time{}, 0, 0
	}

	// Start from the newest files and work backwards
	// We want to keep as much as possible under maxSize
	remainingSize = totalSize

	// Find the cutoff point - delete old files until we're under maxSize
	for i := 0; i < len(slots); i++ {
		slot := slots[i]

		// Delete this entire slot
		remainingSize -= slot.totalBlockSize
		deleteFiles += len(slot.files)
		deleteSize += slot.totalBlockSize

		// Check if we've deleted enough
		if remainingSize <= maxSize {
			// We've reached our target - set threshold to include this slot
			// (and nothing newer)
			return slot.time.Add(window), deleteFiles, deleteSize
		}
	}

	// If we get here, we need to delete everything (shouldn't happen normally)
	if len(slots) > 0 {
		return time.Now().Add(time.Hour), deleteFiles, deleteSize
	}
	return time.Time{}, 0, 0
}
