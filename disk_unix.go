//go:build !windows
// +build !windows

package gobackupcleaner

import (
	"errors"
	"strconv"
	"syscall"
)

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

// GetFilesystemID returns the device number of the filesystem holding path
func (d *DefaultDiskInfoProvider) GetFilesystemID(path string) (string, error) {
	var st syscall.Stat_t
	if err := syscall.Stat(path, &st); err != nil {
		return "", err
	}
	return strconv.FormatUint(uint64(st.Dev), 10), nil
}
