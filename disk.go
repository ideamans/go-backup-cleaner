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