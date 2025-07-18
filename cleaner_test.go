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
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Logf("cleanup failed: %v", err)
		}
	}()

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
		if err := createTestFile(t, path, tf.size, tf.modTime); err != nil {
			t.Fatal(err)
		}
	}

	// Create subdirectory with files
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := createTestFile(t, filepath.Join(subDir, "old_sub.txt"), 1024, now.Add(-96*time.Hour)); err != nil {
		t.Fatal(err)
	}

	// Test with MaxUsagePercent configuration
	// The mock provider shows 80% usage, we want to reduce to 70%
	maxUsage := float64(70)
	config := CleaningConfig{
		MaxUsagePercent: &maxUsage,
		TimeWindow:      time.Hour,
		RemoveEmptyDirs: true,
		Concurrency:     2,
		DiskInfo:        &mockDiskInfoProvider{},
	}

	report, err := CleanBackup(tmpDir, config)
	if err != nil {
		t.Fatal(err)
	}

	// Log the report for debugging
	t.Logf("Report: DeletedFiles=%d, DeletedSize=%d, TimeThreshold=%v",
		report.DeletedFiles, report.DeletedSize, report.TimeThreshold)

	// Verify results
	if report.DeletedFiles == 0 {
		t.Error("Expected some files to be deleted")
	}
	if report.DeletedSize == 0 {
		t.Error("Expected some bytes to be deleted")
	}

	// The deletion should have removed some old files
	// but the exact files depend on the target size calculation
	// Let's just verify that we deleted something and not everything
	remainingFiles := 0
	files := []string{"old1.txt", "old2.txt", "recent1.txt", "recent2.txt"}
	for _, fname := range files {
		if _, err := os.Stat(filepath.Join(tmpDir, fname)); err == nil {
			remainingFiles++
		}
	}

	if remainingFiles == 0 {
		t.Error("All files were deleted, expected some to remain")
	}
	if remainingFiles == len(files) {
		t.Error("No files were deleted, expected some to be deleted")
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
		{
			name: "Negative Concurrency",
			config: CleaningConfig{
				MaxSize:     int64Ptr(1024),
				Concurrency: -1,
			},
			shouldError: true,
		},
		{
			name: "Negative MaxConcurrency",
			config: CleaningConfig{
				MaxSize:        int64Ptr(1024),
				MaxConcurrency: -1,
			},
			shouldError: true,
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
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Logf("cleanup failed: %v", err)
		}
	}()

	// Create multiple test files
	now := time.Now()
	for i := 0; i < 5; i++ {
		testFile := filepath.Join(tmpDir, fmt.Sprintf("test%d.txt", i))
		// Create files with varying ages
		age := time.Duration(i+1) * 24 * time.Hour
		if err := createTestFile(t, testFile, 1024*1024, now.Add(-age)); err != nil {
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

func createTestFile(t *testing.T, path string, size int64, modTime time.Time) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Logf("file close failed: %v", err)
		}
	}()

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

// failingDiskInfoProvider simulates disk usage retrieval failure
type failingDiskInfoProvider struct{}

func (f *failingDiskInfoProvider) GetDiskUsage(path string) (*DiskUsage, error) {
	return nil, fmt.Errorf("disk usage not available")
}

func (f *failingDiskInfoProvider) GetBlockSize(path string) (int64, error) {
	return 4096, nil
}

// TestCleanBackupWithoutDiskUsage tests cleaning when disk usage is not available
func TestCleanBackupWithoutDiskUsage(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "backup-cleaner-nodisk-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Logf("cleanup failed: %v", err)
		}
	}()

	// Create test files
	now := time.Now()
	testFiles := []struct {
		name    string
		size    int64
		modTime time.Time
	}{
		{"old1.txt", 1024 * 1024, now.Add(-72 * time.Hour)},     // 1MB
		{"old2.txt", 2 * 1024 * 1024, now.Add(-48 * time.Hour)}, // 2MB
		{"recent1.txt", 512 * 1024, now.Add(-1 * time.Hour)},    // 512KB
		{"recent2.txt", 256 * 1024, now.Add(-30 * time.Minute)}, // 256KB
	}

	// Create test files
	var totalTestSize int64
	for _, tf := range testFiles {
		path := filepath.Join(tmpDir, tf.name)
		if err := createTestFile(t, path, tf.size, tf.modTime); err != nil {
			t.Fatal(err)
		}
		blockSize := calculateBlockSize(tf.size, 4096)
		totalTestSize += blockSize
		t.Logf("Created %s: %d bytes (block: %d), modTime: %v", tf.name, tf.size, blockSize, tf.modTime)
	}

	// Test with MaxSize when disk usage is not available
	maxSize := int64(2 * 1024 * 1024) // 2MB max
	t.Logf("Total test size (blocks): %d, MaxSize: %d", totalTestSize, maxSize)
	config := CleaningConfig{
		MaxSize:         &maxSize,
		TimeWindow:      time.Hour,
		RemoveEmptyDirs: true,
		DiskInfo:        &failingDiskInfoProvider{},
	}

	report, err := CleanBackup(tmpDir, config)
	if err != nil {
		t.Fatal(err)
	}

	// Should have deleted old files to get under 2MB
	if report.DeletedFiles == 0 {
		t.Error("Expected some files to be deleted")
	}

	t.Logf("TimeThreshold: %v", report.TimeThreshold)

	// Verify that files were deleted
	t.Logf("Deleted %d files, %d bytes", report.DeletedFiles, report.DeletedSize)

	// Check which files remain
	remainingFiles := 0
	var remainingSize int64
	var remainingBlockSize int64
	blockSize := int64(4096)

	files := []string{"old1.txt", "old2.txt", "recent1.txt", "recent2.txt"}
	for _, fname := range files {
		if info, err := os.Stat(filepath.Join(tmpDir, fname)); err == nil {
			remainingFiles++
			remainingSize += info.Size()
			remainingBlockSize += calculateBlockSize(info.Size(), blockSize)
			t.Logf("Remaining: %s (%d bytes, %d block bytes)", fname, info.Size(),
				calculateBlockSize(info.Size(), blockSize))
		}
	}

	// The algorithm should keep total block size under maxSize
	// We need to check block-aligned sizes, not actual file sizes
	if remainingBlockSize > maxSize {
		t.Errorf("Remaining block size %d exceeds max size %d", remainingBlockSize, maxSize)
	}
}

// TestCleanBackupWithoutDiskUsageAndNoMaxSize tests that we fail gracefully when disk usage is not available and no MaxSize
func TestCleanBackupWithoutDiskUsageAndNoMaxSize(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "backup-cleaner-fail-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Logf("cleanup failed: %v", err)
		}
	}()

	// Create a test file
	if err := createTestFile(t, filepath.Join(tmpDir, "test.txt"), 1024, time.Now()); err != nil {
		t.Fatal(err)
	}

	// Test with only MaxUsagePercent when disk usage is not available
	maxUsage := float64(70)
	config := CleaningConfig{
		MaxUsagePercent: &maxUsage,
		DiskInfo:        &failingDiskInfoProvider{},
	}

	_, err = CleanBackup(tmpDir, config)
	if err == nil {
		t.Error("Expected error when disk usage is not available and no MaxSize is specified")
	}
}
