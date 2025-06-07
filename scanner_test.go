package gobackupcleaner

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScanner(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "scanner-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test file structure
	now := time.Now()
	testFiles := []struct {
		path    string
		size    int64
		modTime time.Time
	}{
		{"file1.txt", 1024, now.Add(-2 * time.Hour)},
		{"file2.txt", 2048, now.Add(-1 * time.Hour)},
		{"dir1/file3.txt", 512, now.Add(-30 * time.Minute)},
		{"dir1/dir2/file4.txt", 256, now},
	}

	// Create directories
	os.Mkdir(filepath.Join(tmpDir, "dir1"), 0755)
	os.Mkdir(filepath.Join(tmpDir, "dir1", "dir2"), 0755)

	// Create files
	for _, tf := range testFiles {
		path := filepath.Join(tmpDir, tf.path)
		if err := createTestFile(path, tf.size, tf.modTime); err != nil {
			t.Fatal(err)
		}
	}

	// Test scanner
	config := CleaningConfig{
		TimeWindow:  time.Hour,
		Concurrency: 2,
	}
	config.setDefaults()

	scanner := newScanner(&config, 4096)
	err = scanner.scan(tmpDir)
	if err != nil {
		t.Fatalf("Scanner failed: %v", err)
	}

	// Verify results
	totalFiles := scanner.getTotalFiles()
	if totalFiles != len(testFiles) {
		t.Errorf("Expected %d files, got %d", len(testFiles), totalFiles)
	}

	// Test time slots
	slots := scanner.getTimeSlots()
	if len(slots) == 0 {
		t.Error("Expected at least one time slot")
	}

	// Verify slots are sorted (oldest first)
	for i := 1; i < len(slots); i++ {
		if slots[i-1].time.After(slots[i].time) {
			t.Error("Time slots are not sorted correctly")
		}
	}
}

func TestScannerWithSymlinks(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "scanner-symlink-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a file and a symlink
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := createTestFile(testFile, 1024, time.Now()); err != nil {
		t.Fatal(err)
	}

	symlink := filepath.Join(tmpDir, "link.txt")
	if err := os.Symlink(testFile, symlink); err != nil {
		t.Skip("Cannot create symlinks on this system")
	}

	// Test scanner
	config := CleaningConfig{
		TimeWindow:  time.Hour,
		Concurrency: 1,
	}
	config.setDefaults()

	scanner := newScanner(&config, 4096)
	err = scanner.scan(tmpDir)
	if err != nil {
		t.Fatalf("Scanner failed: %v", err)
	}

	// Should only count regular files, not symlinks
	totalFiles := scanner.getTotalFiles()
	if totalFiles != 1 {
		t.Errorf("Expected 1 file (symlinks should be ignored), got %d", totalFiles)
	}
}

func TestScannerWithPermissionError(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "scanner-perm-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a directory with no read permission
	restrictedDir := filepath.Join(tmpDir, "restricted")
	if err := os.Mkdir(restrictedDir, 0000); err != nil {
		t.Fatal(err)
	}

	// Create a normal file
	if err := createTestFile(filepath.Join(tmpDir, "normal.txt"), 1024, time.Now()); err != nil {
		t.Fatal(err)
	}

	// Test scanner with error callback
	errorCount := 0
	config := CleaningConfig{
		TimeWindow:  time.Hour,
		Concurrency: 1,
		Callbacks: Callbacks{
			OnError: func(info ErrorInfo) {
				errorCount++
			},
		},
	}
	config.setDefaults()

	scanner := newScanner(&config, 4096)
	err = scanner.scan(tmpDir)
	
	// Should continue despite permission error
	totalFiles := scanner.getTotalFiles()
	if totalFiles != 1 {
		t.Errorf("Expected 1 file despite permission error, got %d", totalFiles)
	}

	// Restore permissions for cleanup
	os.Chmod(restrictedDir, 0755)
}

func TestTimeSlotAggregation(t *testing.T) {
	config := CleaningConfig{
		TimeWindow:  time.Hour,
		Concurrency: 1,
	}
	config.setDefaults()

	scanner := newScanner(&config, 4096)

	// Add files with different timestamps
	baseTime := time.Now().Truncate(time.Hour)
	
	// Files in the same time window
	scanner.addFile(fileInfo{
		path:      "file1.txt",
		size:      1000,
		blockSize: 4096,
		modTime:   baseTime.Add(10 * time.Minute),
	})
	scanner.addFile(fileInfo{
		path:      "file2.txt",
		size:      2000,
		blockSize: 4096,
		modTime:   baseTime.Add(30 * time.Minute),
	})

	// File in different time window
	scanner.addFile(fileInfo{
		path:      "file3.txt",
		size:      3000,
		blockSize: 4096,
		modTime:   baseTime.Add(90 * time.Minute),
	})

	slots := scanner.getTimeSlots()
	if len(slots) != 2 {
		t.Errorf("Expected 2 time slots, got %d", len(slots))
	}

	// Check first slot
	if len(slots[0].files) != 2 {
		t.Errorf("Expected 2 files in first slot, got %d", len(slots[0].files))
	}
	if slots[0].totalSize != 3000 {
		t.Errorf("Expected total size 3000 in first slot, got %d", slots[0].totalSize)
	}
	if slots[0].totalBlockSize != 8192 {
		t.Errorf("Expected total block size 8192 in first slot, got %d", slots[0].totalBlockSize)
	}

	// Check second slot
	if len(slots[1].files) != 1 {
		t.Errorf("Expected 1 file in second slot, got %d", len(slots[1].files))
	}
}