//go:build windows
// +build windows

package gobackupcleaner

import (
	"errors"
	"path/filepath"
	"syscall"
	"unsafe"
)

// This implementation uses Windows API directly via syscall
// No external dependencies are required

var (
	kernel32                = syscall.NewLazyDLL("kernel32.dll")
	procGetDiskFreeSpaceEx  = kernel32.NewProc("GetDiskFreeSpaceExW")
	procGetDiskFreeSpace    = kernel32.NewProc("GetDiskFreeSpaceW")
)

// GetDiskUsage returns disk usage information for the given path
func (d *DefaultDiskInfoProvider) GetDiskUsage(path string) (*DiskUsage, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	// For non-existent paths, we should use the path itself to check, not just the volume
	// Try to get disk info using the path first, then fall back to volume
	var freeBytesAvailable, totalBytes, totalFreeBytes uint64

	// Convert path to UTF16 for Windows API
	pathPtr, err := syscall.UTF16PtrFromString(absPath)
	if err != nil {
		return nil, err
	}

	// First try with the actual path
	ret, _, err := procGetDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)

	if ret == 0 {
		// If the path doesn't exist, this should fail
		return nil, err
	}

	used := totalBytes - totalFreeBytes

	if totalBytes == 0 {
		return nil, errors.New("total disk size is 0")
	}

	usedPercent := float64(used) / float64(totalBytes) * 100

	return &DiskUsage{
		Total:       totalBytes,
		Free:        freeBytesAvailable,
		Used:        used,
		UsedPercent: usedPercent,
	}, nil
}

// GetBlockSize returns the block size for the given path
func (d *DefaultDiskInfoProvider) GetBlockSize(path string) (int64, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return 0, err
	}

	// Convert path to UTF16 for Windows API
	pathPtr, err := syscall.UTF16PtrFromString(absPath)
	if err != nil {
		return 0, err
	}

	var sectorsPerCluster, bytesPerSector, numberOfFreeClusters, totalNumberOfClusters uint32

	// First try with the actual path
	ret, _, err := procGetDiskFreeSpace.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&sectorsPerCluster)),
		uintptr(unsafe.Pointer(&bytesPerSector)),
		uintptr(unsafe.Pointer(&numberOfFreeClusters)),
		uintptr(unsafe.Pointer(&totalNumberOfClusters)),
	)

	if ret == 0 {
		// If the path doesn't exist, this should fail
		return 0, err
	}

	// Cluster size is the effective "block size" on Windows
	clusterSize := int64(sectorsPerCluster) * int64(bytesPerSector)
	return clusterSize, nil
}