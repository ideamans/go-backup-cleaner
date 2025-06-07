package gobackupcleaner

import (
	"os"
	"time"
)

// CleanBackup cleans backup files based on the specified configuration
func CleanBackup(dirPath string, config CleaningConfig) (CleaningReport, error) {
	startTime := time.Now()

	// Set defaults and validate configuration
	config.setDefaults()
	if err := config.validate(); err != nil {
		return CleaningReport{}, err
	}

	// Check if directory exists
	if _, err := os.Stat(dirPath); err != nil {
		if os.IsNotExist(err) {
			return CleaningReport{}, ErrDirectoryNotFound
		}
		return CleaningReport{}, err
	}

	// Get current disk usage
	currentUsage, err := config.DiskInfo.GetDiskUsage(dirPath)
	if err != nil {
		return CleaningReport{}, err
	}

	// Calculate target deletion size
	targetSize := calculateTargetSize(currentUsage, &config)
	if targetSize <= 0 {
		// No need to delete anything
		return CleaningReport{
			TotalDuration: time.Since(startTime),
		}, nil
	}

	// Get block size
	blockSize, err := config.DiskInfo.GetBlockSize(dirPath)
	if err != nil {
		return CleaningReport{}, err
	}

	// Call OnStart callback
	callSafe(config.Callbacks.OnStart, StartInfo{
		TargetDir:    dirPath,
		CurrentUsage: *currentUsage,
		TargetSize:   targetSize,
	})

	// Phase 1: Scan files
	scanStartTime := time.Now()
	scanner := newScanner(&config, blockSize)
	if err := scanner.scan(dirPath); err != nil {
		return CleaningReport{}, err
	}

	// Get sorted time slots
	timeSlots := scanner.getTimeSlots()
	if len(timeSlots) == 0 {
		// No files found
		return CleaningReport{
			ScanDuration:  time.Since(scanStartTime),
			TotalDuration: time.Since(startTime),
		}, nil
	}

	// Calculate deletion threshold
	threshold, estimatedFiles, estimatedSize := calculateThreshold(timeSlots, targetSize)
	scanDuration := time.Since(scanStartTime)

	// Call OnScanComplete callback
	callSafe(config.Callbacks.OnScanComplete, ScanCompleteInfo{
		ScannedFiles:  scanner.getTotalFiles(),
		TotalSize:     getTotalSize(timeSlots),
		BlockSize:     blockSize,
		TimeThreshold: threshold,
		ScanDuration:  scanDuration,
	})

	// Phase 2: Delete files
	deleteStartTime := time.Now()
	
	// Call OnDeleteStart callback
	callSafe(config.Callbacks.OnDeleteStart, DeleteStartInfo{
		EstimatedFiles: estimatedFiles,
		EstimatedSize:  estimatedSize,
	})

	deleter := newDeleter(&config, blockSize)
	if err := deleter.deleteFiles(dirPath, threshold); err != nil {
		return CleaningReport{}, err
	}

	// Phase 3: Delete empty directories
	deletedDirs, err := deleter.deleteEmptyDirs()
	if err != nil {
		// Non-fatal error, continue
	}

	deleteDuration := time.Since(deleteStartTime)
	deletedFiles, deletedSize, deletedBlocks := deleter.getStats()

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
func calculateThreshold(slots []*timeSlot, targetSize int64) (time.Time, int, int64) {
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
			threshold = slot.time.Add(time.Second)
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