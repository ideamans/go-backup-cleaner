package gobackupcleaner

import (
	"runtime"
	"time"
)

// CleaningConfig represents the configuration for cleaning operations
type CleaningConfig struct {
	// Capacity specifications (at least one required)
	MaxSize         *int64   // Maximum size in bytes
	MaxUsagePercent *float64 // Maximum disk usage percentage (0-100)
	MinFreeSpace    *int64   // Minimum free space in bytes

	// Optional settings
	TimeWindow      time.Duration // Time interval for file aggregation (default: 1 minute)
	RemoveEmptyDirs bool          // Whether to remove empty directories (default: true)
	WorkerCount     int           // Number of parallel workers (default: runtime.NumCPU())

	// Callbacks
	Callbacks Callbacks

	// Dependency injection
	DiskInfo DiskInfoProvider // If nil, uses default implementation
}

// setDefaults sets default values for the configuration
func (c *CleaningConfig) setDefaults() {
	if c.TimeWindow == 0 {
		c.TimeWindow = time.Minute
	}
	if c.WorkerCount == 0 {
		c.WorkerCount = runtime.NumCPU()
	}
	if c.DiskInfo == nil {
		c.DiskInfo = &DefaultDiskInfoProvider{}
	}
	// RemoveEmptyDirs defaults to true, but we can't override explicit false
	// So we don't set it here - let the caller decide
}

// validate checks if the configuration is valid
func (c *CleaningConfig) validate() error {
	if c.MaxSize == nil && c.MaxUsagePercent == nil && c.MinFreeSpace == nil {
		return ErrNoCapacitySpecified
	}

	if c.MaxSize != nil && *c.MaxSize < 0 {
		return ErrInvalidConfig
	}

	if c.MaxUsagePercent != nil && (*c.MaxUsagePercent < 0 || *c.MaxUsagePercent > 100) {
		return ErrInvalidConfig
	}

	if c.MinFreeSpace != nil && *c.MinFreeSpace < 0 {
		return ErrInvalidConfig
	}

	if c.TimeWindow < 0 {
		return ErrInvalidConfig
	}

	if c.WorkerCount < 1 {
		return ErrInvalidConfig
	}

	return nil
}