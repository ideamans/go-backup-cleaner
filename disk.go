package gobackupcleaner

import (
	"errors"
	"syscall"
)

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

// GetDiskUsage returns disk usage information for the given path
func (d *DefaultDiskInfoProvider) GetDiskUsage(path string) (*DiskUsage, error) {
	var stat syscall.Statfs_t
	err := syscall.Statfs(path, &stat)
	if err != nil {
		return nil, err
	}

	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bavail * uint64(stat.Bsize)
	used := total - free

	if total == 0 {
		return nil, errors.New("total disk size is 0")
	}

	usedPercent := float64(used) / float64(total) * 100

	return &DiskUsage{
		Total:       total,
		Free:        free,
		Used:        used,
		UsedPercent: usedPercent,
	}, nil
}

// GetBlockSize returns the block size for the given path
func (d *DefaultDiskInfoProvider) GetBlockSize(path string) (int64, error) {
	var stat syscall.Statfs_t
	err := syscall.Statfs(path, &stat)
	if err != nil {
		return 0, err
	}
	return int64(stat.Bsize), nil
}

// calculateBlockSize calculates the actual block size used by a file
func calculateBlockSize(fileSize int64, blockSize int64) int64 {
	if blockSize <= 0 {
		return fileSize
	}
	blocks := (fileSize + blockSize - 1) / blockSize
	return blocks * blockSize
}