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
	
	// Concurrency settings
	// Concurrency specifies the desired number of parallel workers.
	// If 0, defaults to runtime.NumCPU().
	Concurrency int
	
	// MaxConcurrency limits the maximum number of parallel workers.
	// Defaults to 4, as benchmarks show diminishing returns beyond this value.
	// The actual number of workers will be min(Concurrency, MaxConcurrency).
	MaxConcurrency int

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
	
	// Set default concurrency to CPU count if not specified
	if c.Concurrency == 0 {
		c.Concurrency = runtime.NumCPU()
	}
	
	// Set default max concurrency
	if c.MaxConcurrency == 0 {
		c.MaxConcurrency = 4
	}
	
	if c.DiskInfo == nil {
		c.DiskInfo = &DefaultDiskInfoProvider{}
	}
	// RemoveEmptyDirs defaults to true, but we can't override explicit false
	// So we don't set it here - let the caller decide
}

// EffectiveWorkerCount returns the actual number of workers that will be used
func (c *CleaningConfig) EffectiveWorkerCount() int {
	workers := c.Concurrency
	if workers > c.MaxConcurrency {
		workers = c.MaxConcurrency
	}
	return workers
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

	if c.Concurrency < 0 {
		return ErrInvalidConfig
	}

	if c.MaxConcurrency < 0 {
		return ErrInvalidConfig
	}

	return nil
}