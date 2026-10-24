package gobackupcleaner

import "time"

// CleaningReport represents the result of a cleaning operation
type CleaningReport struct {
	// Deletion statistics
	DeletedFiles     int   // Number of deleted files
	DeletedSize      int64 // Actual file size in bytes
	DeletedBlockSize int64 // Block-aligned size in bytes
	DeletedDirs      int   // Number of deleted directories

	// Processing time
	ScanDuration   time.Duration // Time spent scanning files
	DeleteDuration time.Duration // Time spent deleting files
	TotalDuration  time.Duration // Total processing time

	// Other information
	ScannedFiles  int       // Total number of scanned files
	TimeThreshold time.Time // Time threshold for deletion
	BlockSize     int64     // File system block size

	// DryRun is true when nothing was deleted; the deletion figures are estimates
	DryRun bool
	// Roots has the figures per directory given to CleanBackups (in the same order)
	Roots []RootReport
}

// RootReport has the figures for one directory given to CleanBackups
type RootReport struct {
	Dir          string
	ScannedFiles int
	ScannedSize  int64
	DeletedFiles int   // with DryRun, the files that would be deleted
	DeletedSize  int64 // with DryRun, the size that would be freed
}
