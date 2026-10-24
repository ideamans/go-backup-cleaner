package gobackupcleaner

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fixedFreeProvider reports a fixed free space so that a known amount must be deleted
type fixedFreeProvider struct {
	free uint64
	fsID map[string]string
}

func (p *fixedFreeProvider) GetDiskUsage(path string) (*DiskUsage, error) {
	total := uint64(100 * 4096)
	return &DiskUsage{Total: total, Free: p.free, Used: total - p.free, UsedPercent: float64(total-p.free) / float64(total) * 100}, nil
}

func (p *fixedFreeProvider) GetBlockSize(path string) (int64, error) { return 4096, nil }

func (p *fixedFreeProvider) GetFilesystemID(path string) (string, error) {
	if id, ok := p.fsID[filepath.Base(path)]; ok {
		return id, nil
	}
	return "fs0", nil
}

// multiRootFixture makes two roots whose files interleave in time:
//
//	a/old.dat (-10d)  b/older.dat (-20d)  a/new.dat (-1d)  b/sub/mid.dat (-5d)
//
// Each file takes one 4096-byte block.
func multiRootFixture(t *testing.T) (string, string) {
	t.Helper()
	base := t.TempDir()
	a, b := filepath.Join(base, "a"), filepath.Join(base, "b")
	now := time.Now()
	files := []struct {
		path string
		age  time.Duration
	}{
		{filepath.Join(a, "old.dat"), 10 * 24 * time.Hour},
		{filepath.Join(a, "new.dat"), 1 * 24 * time.Hour},
		{filepath.Join(b, "older.dat"), 20 * 24 * time.Hour},
		{filepath.Join(b, "sub", "mid.dat"), 5 * 24 * time.Hour},
	}
	for _, f := range files {
		if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := createTestFile(t, f.path, 1000, now.Add(-f.age)); err != nil {
			t.Fatal(err)
		}
	}
	return a, b
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// The oldest files are deleted first across both roots, not root by root
func TestCleanBackupsDeletesOldestAcrossRoots(t *testing.T) {
	a, b := multiRootFixture(t)
	minFree := int64(12 * 4096) // free is 10 blocks -> 2 blocks must go
	report, err := CleanBackups([]string{a, b}, CleaningConfig{
		MinFreeSpace:    &minFree,
		RemoveEmptyDirs: true,
		DiskInfo:        &fixedFreeProvider{free: 10 * 4096},
	})
	if err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(b, "older.dat")) || exists(filepath.Join(a, "old.dat")) {
		t.Error("the two oldest files (b/older, a/old) should be deleted")
	}
	if !exists(filepath.Join(b, "sub", "mid.dat")) || !exists(filepath.Join(a, "new.dat")) {
		t.Error("newer files should be kept")
	}
	if report.DeletedFiles != 2 || len(report.Roots) != 2 || report.Roots[0].DeletedFiles != 1 || report.Roots[1].DeletedFiles != 1 {
		t.Errorf("unexpected report: %+v", report)
	}
	if !exists(a) || !exists(b) {
		t.Error("roots must never be removed")
	}
}

// Dry run deletes nothing and reports what would be deleted per root
func TestCleanBackupsDryRun(t *testing.T) {
	a, b := multiRootFixture(t)
	minFree := int64(13 * 4096) // 3 blocks: b/older, a/old, b/sub/mid
	report, err := CleanBackups([]string{a, b}, CleaningConfig{
		MinFreeSpace: &minFree,
		DryRun:       true,
		DiskInfo:     &fixedFreeProvider{free: 10 * 4096},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"a/old.dat", "a/new.dat", "b/older.dat", "b/sub/mid.dat"} {
		if !exists(filepath.Join(filepath.Dir(a), p)) {
			t.Errorf("dry run deleted %s", p)
		}
	}
	if !report.DryRun || report.DeletedFiles != 3 || report.Roots[0].DeletedFiles != 1 || report.Roots[1].DeletedFiles != 2 {
		t.Errorf("unexpected report: %+v", report)
	}
}

// Emptied directories inside a root are removed, but never the root itself
func TestCleanBackupsKeepsRoots(t *testing.T) {
	a, b := multiRootFixture(t)
	minFree := int64(100 * 4096) // delete everything
	if _, err := CleanBackups([]string{a, b}, CleaningConfig{
		MinFreeSpace:    &minFree,
		RemoveEmptyDirs: true,
		DiskInfo:        &fixedFreeProvider{free: 10 * 4096},
	}); err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(b, "sub")) {
		t.Error("emptied sub directory should be removed")
	}
	if !exists(a) || !exists(b) || !exists(filepath.Dir(a)) {
		t.Error("roots and their parent must be kept")
	}
}

// A disk-based limit over different filesystems is refused
func TestCleanBackupsRefusesDifferentFilesystems(t *testing.T) {
	a, b := multiRootFixture(t)
	minFree := int64(12 * 4096)
	_, err := CleanBackups([]string{a, b}, CleaningConfig{
		MinFreeSpace: &minFree,
		DiskInfo:     &fixedFreeProvider{free: 10 * 4096, fsID: map[string]string{"b": "fs1"}},
	})
	if !errors.Is(err, ErrDifferentFilesystems) {
		t.Fatalf("got %v", err)
	}
}

// The whole last time slot is deleted (files later than the slot's first second too)
func TestThresholdCoversWholeSlot(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	slots := []*timeSlot{{time: base, totalBlockSize: 4096}, {time: base.Add(5 * time.Minute), totalBlockSize: 4096}}
	th, _, _ := calculateThreshold(slots, 4096, 5*time.Minute)
	if !base.Add(4 * time.Minute).Before(th) || base.Add(5*time.Minute).Before(th) {
		t.Errorf("threshold %v should be the end of the first slot", th)
	}
}
