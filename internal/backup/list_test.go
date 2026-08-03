package backup

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{-10, "0 B"},
		{0, "0 B"},
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{15728640, "15.0 MB"},
		{1073741824, "1.0 GB"},
	}

	for _, tt := range tests {
		got := FormatBytes(tt.bytes)
		if got != tt.want {
			t.Errorf("FormatBytes(%d) = %q; want %q", tt.bytes, got, tt.want)
		}
	}
}

func TestFormatAge(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		t    time.Time
		want string
	}{
		{time.Time{}, "unknown"},
		{now.Add(time.Minute), "just now"},
		{now.Add(-30 * time.Second), "just now"},
		{now.Add(-5 * time.Minute), "5m ago"},
		{now.Add(-3 * time.Hour), "3h ago"},
		{now.Add(-48 * time.Hour), "2d ago"},
	}

	for _, tt := range tests {
		got := FormatAge(tt.t, now)
		if got != tt.want {
			t.Errorf("FormatAge(%v, %v) = %q; want %q", tt.t, now, got, tt.want)
		}
	}
}

func TestDirSizeBytes(t *testing.T) {
	dir := t.TempDir()

	// Empty dir
	size, err := DirSizeBytes(dir)
	if err != nil {
		t.Fatalf("DirSizeBytes(empty) error = %v", err)
	}
	if size != 0 {
		t.Errorf("DirSizeBytes(empty) = %d; want 0", size)
	}

	// Non-existent dir
	size, err = DirSizeBytes(filepath.Join(dir, "non-existent"))
	if err != nil {
		t.Fatalf("DirSizeBytes(non-existent) error = %v", err)
	}
	if size != 0 {
		t.Errorf("DirSizeBytes(non-existent) = %d; want 0", size)
	}

	// Create subfiles
	file1 := filepath.Join(dir, "file1.txt")
	file2 := filepath.Join(dir, "sub", "file2.txt")

	if err := os.MkdirAll(filepath.Dir(file2), 0o755); err != nil {
		t.Fatalf("mkdir error = %v", err)
	}
	if err := os.WriteFile(file1, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file1 error = %v", err)
	}
	if err := os.WriteFile(file2, []byte("world!"), 0o644); err != nil {
		t.Fatalf("write file2 error = %v", err)
	}

	size, err = DirSizeBytes(dir)
	if err != nil {
		t.Fatalf("DirSizeBytes error = %v", err)
	}
	wantSize := int64(len("hello") + len("world!"))
	if size != wantSize {
		t.Errorf("DirSizeBytes = %d; want %d", size, wantSize)
	}
}

func TestListBackupsAndCleanBackups(t *testing.T) {
	backupDir := t.TempDir()

	defer func(orig func() (string, error)) { BackupRootFn = orig }(BackupRootFn)
	BackupRootFn = func() (string, error) { return backupDir, nil }

	now := time.Now()
	// Create 3 backup manifests
	b1Dir := filepath.Join(backupDir, "upgrade-1")
	b2Dir := filepath.Join(backupDir, "upgrade-2")
	b3Dir := filepath.Join(backupDir, "upgrade-3")

	for _, d := range []string{b1Dir, b2Dir, b3Dir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s error = %v", d, err)
		}
	}

	m1 := Manifest{
		ID:        "b1",
		CreatedAt: now.Add(-2 * time.Hour),
		RootDir:   b1Dir,
		Source:    BackupSourceUpgrade,
		FileCount: 2,
	}
	m2 := Manifest{
		ID:        "b2",
		CreatedAt: now.Add(-1 * time.Hour),
		RootDir:   b2Dir,
		Source:    BackupSourceSync,
		FileCount: 3,
	}
	m3 := Manifest{
		ID:        "b3",
		CreatedAt: now,
		RootDir:   b3Dir,
		Source:    BackupSourceInstall,
		FileCount: 1,
	}

	for _, m := range []Manifest{m1, m2, m3} {
		mPath := filepath.Join(m.RootDir, ManifestFilename)
		if err := WriteManifest(mPath, m); err != nil {
			t.Fatalf("WriteManifest error = %v", err)
		}
		// Write dummy data file
		if err := os.WriteFile(filepath.Join(m.RootDir, "data.txt"), []byte("test-data"), 0o644); err != nil {
			t.Fatalf("Write data file error = %v", err)
		}
	}

	report, err := ListBackups(backupDir)
	if err != nil {
		t.Fatalf("ListBackups error = %v", err)
	}
	if report.TotalCount != 3 {
		t.Errorf("report.TotalCount = %d; want 3", report.TotalCount)
	}
	if len(report.Backups) != 3 {
		t.Fatalf("len(report.Backups) = %d; want 3", len(report.Backups))
	}
	// Verify sorted newest first (b3 -> b2 -> b1)
	if report.Backups[0].ID != "b3" || report.Backups[1].ID != "b2" || report.Backups[2].ID != "b1" {
		t.Errorf("Backups not sorted newest-first: got [%s, %s, %s]", report.Backups[0].ID, report.Backups[1].ID, report.Backups[2].ID)
	}

	// Test CleanBackups keep 2
	deleted, err := CleanBackups(backupDir, 2)
	if err != nil {
		t.Fatalf("CleanBackups error = %v", err)
	}
	if len(deleted) != 1 || deleted[0] != "b1" {
		t.Errorf("CleanBackups deleted = %v; want [b1]", deleted)
	}

	reportAfter, err := ListBackups(backupDir)
	if err != nil {
		t.Fatalf("ListBackups after clean error = %v", err)
	}
	if reportAfter.TotalCount != 2 {
		t.Errorf("reportAfter.TotalCount = %d; want 2", reportAfter.TotalCount)
	}
}
