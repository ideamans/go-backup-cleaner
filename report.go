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
}