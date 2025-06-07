package gobackupcleaner

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestCleanBackup tests the main CleanBackup function
func TestCleanBackup(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "backup-cleaner-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test files with different timestamps
	now := time.Now()
	testFiles := []struct {
		name    string
		size    int64
		modTime time.Time
	}{
		{"old1.txt", 1024, now.Add(-72 * time.Hour)},
		{"old2.txt", 2048, now.Add(-48 * time.Hour)},
		{"recent1.txt", 512, now.Add(-1 * time.Hour)},
		{"recent2.txt", 256, now.Add(-30 * time.Minute)},
	}

	// Create test files
	for _, tf := range testFiles {
		path := filepath.Join(tmpDir, tf.name)
		if err := createTestFile(path, tf.size, tf.modTime); err != nil {
			t.Fatal(err)
		}
	}

	// Create subdirectory with files
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := createTestFile(filepath.Join(subDir, "old_sub.txt"), 1024, now.Add(-96*time.Hour)); err != nil {
		t.Fatal(err)
	}

	// Test with MaxSize configuration
	maxSize := int64(2048)
	config := CleaningConfig{
		MaxSize:         &maxSize,
		TimeWindow:      time.Hour,
		RemoveEmptyDirs: true,
		WorkerCount:     2,
		DiskInfo:        &mockDiskInfoProvider{},
	}

	report, err := CleanBackup(tmpDir, config)
	if err != nil {
		t.Fatal(err)
	}

	// Verify results
	if report.DeletedFiles == 0 {
		t.Error("Expected some files to be deleted")
	}
	if report.DeletedSize == 0 {
		t.Error("Expected some bytes to be deleted")
	}

	// Check that old files were deleted
	if _, err := os.Stat(filepath.Join(tmpDir, "old1.txt")); !os.IsNotExist(err) {
		t.Error("Expected old1.txt to be deleted")
	}

	// Check that recent files still exist
	if _, err := os.Stat(filepath.Join(tmpDir, "recent1.txt")); err != nil {
		t.Error("Expected recent1.txt to still exist")
	}
}

// TestCalculateTargetSize tests the target size calculation
func TestCalculateTargetSize(t *testing.T) {
	tests := []struct {
		name           string
		usage          *DiskUsage
		config         *CleaningConfig
		expectedTarget int64
	}{
		{
			name: "MaxSize only",
			usage: &DiskUsage{
				Total:       10 * 1024 * 1024 * 1024, // 10GB
				Used:        8 * 1024 * 1024 * 1024,  // 8GB
				Free:        2 * 1024 * 1024 * 1024,  // 2GB
				UsedPercent: 80.0,
			},
			config: &CleaningConfig{
				MaxSize: int64Ptr(5 * 1024 * 1024 * 1024), // 5GB max
			},
			expectedTarget: 3 * 1024 * 1024 * 1024, // Need to free 3GB
		},
		{
			name: "MaxUsagePercent only",
			usage: &DiskUsage{
				Total:       10 * 1024 * 1024 * 1024, // 10GB
				Used:        8 * 1024 * 1024 * 1024,  // 8GB
				Free:        2 * 1024 * 1024 * 1024,  // 2GB
				UsedPercent: 80.0,
			},
			config: &CleaningConfig{
				MaxUsagePercent: float64Ptr(60.0), // 60% max
			},
			expectedTarget: 2 * 1024 * 1024 * 1024, // Need to free 2GB to reach 60%
		},
		{
			name: "MinFreeSpace only",
			usage: &DiskUsage{
				Total:       10 * 1024 * 1024 * 1024, // 10GB
				Used:        8 * 1024 * 1024 * 1024,  // 8GB
				Free:        2 * 1024 * 1024 * 1024,  // 2GB
				UsedPercent: 80.0,
			},
			config: &CleaningConfig{
				MinFreeSpace: int64Ptr(4 * 1024 * 1024 * 1024), // Need 4GB free
			},
			expectedTarget: 2 * 1024 * 1024 * 1024, // Need to free 2GB
		},
		{
			name: "Multiple constraints - most restrictive wins",
			usage: &DiskUsage{
				Total:       10 * 1024 * 1024 * 1024, // 10GB
				Used:        8 * 1024 * 1024 * 1024,  // 8GB
				Free:        2 * 1024 * 1024 * 1024,  // 2GB
				UsedPercent: 80.0,
			},
			config: &CleaningConfig{
				MaxSize:         int64Ptr(6 * 1024 * 1024 * 1024), // 6GB max (need to free 2GB)
				MaxUsagePercent: float64Ptr(50.0),                 // 50% max (need to free 3GB)
				MinFreeSpace:    int64Ptr(3 * 1024 * 1024 * 1024), // Need 3GB free (need to free 1GB)
			},
			expectedTarget: 3 * 1024 * 1024 * 1024, // MaxUsagePercent is most restrictive
		},
		{
			name: "Already meeting constraints",
			usage: &DiskUsage{
				Total:       10 * 1024 * 1024 * 1024, // 10GB
				Used:        4 * 1024 * 1024 * 1024,  // 4GB
				Free:        6 * 1024 * 1024 * 1024,  // 6GB
				UsedPercent: 40.0,
			},
			config: &CleaningConfig{
				MaxUsagePercent: float64Ptr(60.0), // 60% max (currently at 40%)
			},
			expectedTarget: 0, // No need to delete anything
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := calculateTargetSize(tt.usage, tt.config)
			if target != tt.expectedTarget {
				t.Errorf("Expected target size %d, got %d", tt.expectedTarget, target)
			}
		})
	}
}

// TestConfigValidation tests configuration validation
func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      CleaningConfig
		shouldError bool
	}{
		{
			name:        "No capacity specified",
			config:      CleaningConfig{},
			shouldError: true,
		},
		{
			name: "Valid MaxSize",
			config: CleaningConfig{
				MaxSize: int64Ptr(1024),
			},
			shouldError: false,
		},
		{
			name: "Negative MaxSize",
			config: CleaningConfig{
				MaxSize: int64Ptr(-1024),
			},
			shouldError: true,
		},
		{
			name: "Invalid MaxUsagePercent (>100)",
			config: CleaningConfig{
				MaxUsagePercent: float64Ptr(150.0),
			},
			shouldError: true,
		},
		{
			name: "Invalid MaxUsagePercent (<0)",
			config: CleaningConfig{
				MaxUsagePercent: float64Ptr(-10.0),
			},
			shouldError: true,
		},
		{
			name: "Valid combination",
			config: CleaningConfig{
				MaxSize:         int64Ptr(1024),
				MaxUsagePercent: float64Ptr(80.0),
				MinFreeSpace:    int64Ptr(512),
			},
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.config.setDefaults()
			err := tt.config.validate()
			if tt.shouldError && err == nil {
				t.Error("Expected error but got nil")
			}
			if !tt.shouldError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestCallbacks tests that callbacks are called correctly
func TestCallbacks(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "callback-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create multiple test files
	now := time.Now()
	for i := 0; i < 5; i++ {
		testFile := filepath.Join(tmpDir, fmt.Sprintf("test%d.txt", i))
		// Create files with varying ages
		age := time.Duration(i+1) * 24 * time.Hour
		if err := createTestFile(testFile, 1024*1024, now.Add(-age)); err != nil {
			t.Fatal(err)
		}
	}

	// Track callback calls
	var (
		mu                 sync.Mutex
		startCalled        bool
		scanCompleteCalled bool
		deleteStartCalled  bool
		fileDeletedCalled  bool
		completeCalled     bool
		deletedCount       int
	)

	// Set max usage to force some deletion
	maxUsage := float64(70) // Current mock shows 80% usage
	config := CleaningConfig{
		MaxUsagePercent: &maxUsage,
		Callbacks: Callbacks{
			OnStart: func(info StartInfo) {
				mu.Lock()
				startCalled = true
				mu.Unlock()
				t.Logf("OnStart: TargetDir=%s, TargetSize=%d, CurrentUsage=%+v",
					info.TargetDir, info.TargetSize, info.CurrentUsage)
			},
			OnScanComplete: func(info ScanCompleteInfo) {
				mu.Lock()
				scanCompleteCalled = true
				mu.Unlock()
				t.Logf("ScanComplete: Files=%d, TotalSize=%d, TimeThreshold=%v",
					info.ScannedFiles, info.TotalSize, info.TimeThreshold)
			},
			OnDeleteStart: func(info DeleteStartInfo) {
				mu.Lock()
				deleteStartCalled = true
				mu.Unlock()
			},
			OnFileDeleted: func(info FileDeletedInfo) {
				mu.Lock()
				fileDeletedCalled = true
				deletedCount++
				mu.Unlock()
				t.Logf("OnFileDeleted: Path=%s, Size=%d", info.Path, info.Size)
			},
			OnComplete: func(info CompleteInfo) {
				mu.Lock()
				completeCalled = true
				mu.Unlock()
			},
		},
		DiskInfo: &mockDiskInfoProvider{},
	}

	report, err := CleanBackup(tmpDir, config)
	if err != nil {
		t.Fatal(err)
	}
	
	t.Logf("Report: DeletedFiles=%d, DeletedSize=%d, TimeThreshold=%v", 
		report.DeletedFiles, report.DeletedSize, report.TimeThreshold)

	// Verify all callbacks were called
	if !startCalled {
		t.Error("OnStart callback was not called")
	}
	if !scanCompleteCalled {
		t.Error("OnScanComplete callback was not called")
	}
	if !deleteStartCalled {
		t.Error("OnDeleteStart callback was not called")
	}
	if !fileDeletedCalled {
		t.Error("OnFileDeleted callback was not called")
	}
	if !completeCalled {
		t.Error("OnComplete callback was not called")
	}
}

// Helper functions

func createTestFile(path string, size int64, modTime time.Time) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	// Write data
	data := make([]byte, size)
	if _, err := file.Write(data); err != nil {
		return err
	}

	// Set modification time
	return os.Chtimes(path, modTime, modTime)
}

func int64Ptr(v int64) *int64 {
	return &v
}

func float64Ptr(v float64) *float64 {
	return &v
}

// mockDiskInfoProvider is a mock implementation for testing
type mockDiskInfoProvider struct{}

func (m *mockDiskInfoProvider) GetDiskUsage(path string) (*DiskUsage, error) {
	return &DiskUsage{
		Total:       10 * 1024 * 1024 * 1024, // 10GB
		Used:        8 * 1024 * 1024 * 1024,  // 8GB - high usage to trigger cleanup
		Free:        2 * 1024 * 1024 * 1024,  // 2GB
		UsedPercent: 80.0,
	}, nil
}

func (m *mockDiskInfoProvider) GetBlockSize(path string) (int64, error) {
	return 4096, nil
}