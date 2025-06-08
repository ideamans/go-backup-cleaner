package gobackupcleaner

// DiskUsage represents disk usage information
type DiskUsage struct {
	Total       uint64
	Free        uint64
	Used        uint64
	UsedPercent float64
}

// DiskInfoProvider is an interface for getting disk information
type DiskInfoProvider interface {
	GetDiskUsage(path string) (*DiskUsage, error)
	GetBlockSize(path string) (int64, error)
}

// DefaultDiskInfoProvider is the default implementation of DiskInfoProvider
type DefaultDiskInfoProvider struct{}

// calculateBlockSize calculates the actual block size used by a file
func calculateBlockSize(fileSize int64, blockSize int64) int64 {
	if blockSize <= 0 {
		return fileSize
	}
	blocks := (fileSize + blockSize - 1) / blockSize
	return blocks * blockSize
}

// GetDiskFreeSpace returns the available disk space for the given directory path.
// This is a convenience function useful for quickly checking if cleanup is needed
// before performing the actual backup cleaning operation.
//
// When using MinFreeSpace configuration (recommended), you can use this function
// to pre-check if cleanup is necessary:
//
//	freeSpace, err := GetDiskFreeSpace("/backup")
//	if err == nil && freeSpace < requiredSpace {
//	    // Perform cleanup
//	}
func GetDiskFreeSpace(dirPath string) (int64, error) {
	provider := &DefaultDiskInfoProvider{}
	usage, err := provider.GetDiskUsage(dirPath)
	if err != nil {
		return 0, err
	}
	return int64(usage.Free), nil
}