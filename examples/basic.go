package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	cleaner "github.com/ideamans/go-backup-cleaner"
)

func main() {
	// Parse command line arguments
	var (
		dir        = flag.String("dir", "", "Directory to clean (required)")
		maxUsage   = flag.Float64("max-usage", 0, "Maximum disk usage percentage")
		maxSize    = flag.Int64("max-size", 0, "Maximum size in GB")
		minFree    = flag.Int64("min-free", 0, "Minimum free space in GB")
		dryRun     = flag.Bool("dry-run", false, "Show what would be deleted without actually deleting")
		verbose    = flag.Bool("verbose", false, "Show detailed progress")
	)
	flag.Parse()

	if *dir == "" {
		log.Fatal("Directory is required. Use -dir flag")
	}

	// Convert GB to bytes
	var maxSizeBytes *int64
	if *maxSize > 0 {
		bytes := *maxSize * 1024 * 1024 * 1024
		maxSizeBytes = &bytes
	}

	var minFreeBytes *int64
	if *minFree > 0 {
		bytes := *minFree * 1024 * 1024 * 1024
		minFreeBytes = &bytes
	}

	var maxUsagePtr *float64
	if *maxUsage > 0 {
		maxUsagePtr = maxUsage
	}

	// Create configuration
	config := cleaner.CleaningConfig{
		MaxSize:         maxSizeBytes,
		MaxUsagePercent: maxUsagePtr,
		MinFreeSpace:    minFreeBytes,
		RemoveEmptyDirs: true,
	}

	// Set up callbacks if verbose
	if *verbose {
		config.Callbacks = cleaner.Callbacks{
			OnStart: func(info cleaner.StartInfo) {
				fmt.Printf("Starting cleanup of %s\n", info.TargetDir)
				fmt.Printf("Current disk usage: %.1f%% (%s used of %s)\n",
					info.CurrentUsage.UsedPercent,
					formatBytes(int64(info.CurrentUsage.Used)),
					formatBytes(int64(info.CurrentUsage.Total)))
				fmt.Printf("Target: free %s\n", formatBytes(info.TargetSize))
			},
			OnScanComplete: func(info cleaner.ScanCompleteInfo) {
				fmt.Printf("\nScan complete: %d files, %s total\n",
					info.ScannedFiles, formatBytes(info.TotalSize))
				fmt.Printf("Will delete files older than: %s\n", 
					info.TimeThreshold.Format("2006-01-02 15:04:05"))
			},
			OnFileDeleted: func(info cleaner.FileDeletedInfo) {
				if !*dryRun {
					fmt.Printf("Deleted: %s (%s)\n", info.Path, formatBytes(info.Size))
				} else {
					fmt.Printf("Would delete: %s (%s)\n", info.Path, formatBytes(info.Size))
				}
			},
			OnDirDeleted: func(info cleaner.DirDeletedInfo) {
				if !*dryRun {
					fmt.Printf("Removed empty dir: %s\n", info.Path)
				} else {
					fmt.Printf("Would remove empty dir: %s\n", info.Path)
				}
			},
			OnError: func(info cleaner.ErrorInfo) {
				log.Printf("Error [%s]: %v", info.Type, info.Error)
			},
		}
	}

	// Validate configuration has at least one constraint
	if maxSizeBytes == nil && maxUsagePtr == nil && minFreeBytes == nil {
		log.Fatal("At least one constraint required: -max-usage, -max-size, or -min-free")
	}

	// Run cleanup
	start := time.Now()
	report, err := cleaner.CleanBackup(*dir, config)
	if err != nil {
		log.Fatal(err)
	}

	// Print summary
	fmt.Printf("\nCleanup complete in %v\n", time.Since(start))
	fmt.Printf("Deleted: %d files, %d directories\n", report.DeletedFiles, report.DeletedDirs)
	fmt.Printf("Freed: %s (actual disk space: %s)\n",
		formatBytes(report.DeletedSize),
		formatBytes(report.DeletedBlockSize))
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}