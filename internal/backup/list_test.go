package backup

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1024 * 1024, "1.0 MB"},
		{13002342, "12.4 MB"},
		{1024 * 1024 * 1024, "1.0 GB"},
	}

	for _, tt := range tests {
		got := FormatSize(tt.bytes)
		if got != tt.want {
			t.Errorf("FormatSize(%d) = %q, want %q", tt.bytes, got, tt.want)
		}
	}
}

func TestFormatAge(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		timestamp time.Time
		want      string
	}{
		{now.Add(-10 * time.Second), "just now"},
		{now.Add(-5 * time.Minute), "5m ago"},
		{now.Add(-2 * time.Hour), "2h ago"},
		{now.Add(-48 * time.Hour), "2d ago"},
	}

	for _, tt := range tests {
		got := FormatAge(tt.timestamp, now)
		if got != tt.want {
			t.Errorf("FormatAge(%v) = %q, want %q", tt.timestamp, got, tt.want)
		}
	}
}

func TestDirSize(t *testing.T) {
	dir := t.TempDir()

	file1 := filepath.Join(dir, "a.txt")
	file2 := filepath.Join(dir, "sub", "b.txt")
	if err := os.MkdirAll(filepath.Dir(file2), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}

	if err := os.WriteFile(file1, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile file1 error = %v", err)
	}
	if err := os.WriteFile(file2, []byte("world!"), 0o644); err != nil {
		t.Fatalf("WriteFile file2 error = %v", err)
	}

	size, err := DirSize(dir)
	if err != nil {
		t.Fatalf("DirSize() error = %v", err)
	}
	// "hello" (5) + "world!" (6) = 11 bytes
	if size != 11 {
		t.Errorf("DirSize() = %d, want 11", size)
	}
}

func TestListBackupsReport_MissingDir(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	report, err := ListBackupsReport(missing)
	if err != nil {
		t.Fatalf("ListBackupsReport(missing) error = %v", err)
	}
	if report.TotalCount != 0 {
		t.Errorf("report.TotalCount = %d, want 0", report.TotalCount)
	}
	if report.TotalBytes != 0 {
		t.Errorf("report.TotalBytes = %d, want 0", report.TotalBytes)
	}
	if len(report.Backups) != 0 {
		t.Errorf("len(report.Backups) = %d, want 0", len(report.Backups))
	}
}

func TestListBackupsReport_WithBackups(t *testing.T) {
	root := t.TempDir()

	b1Dir := filepath.Join(root, "backup-1")
	b2Dir := filepath.Join(root, "backup-2")

	m1 := Manifest{
		ID:          "backup-1",
		CreatedAt:   time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC),
		RootDir:     b1Dir,
		Source:      BackupSourceInstall,
		Description: "initial install",
		FileCount:   2,
	}
	m2 := Manifest{
		ID:          "backup-2",
		CreatedAt:   time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC),
		RootDir:     b2Dir,
		Source:      BackupSourceUpgrade,
		Description: "upgrade-v2",
		FileCount:   1,
		Pinned:      true,
	}

	if err := WriteManifest(filepath.Join(b1Dir, ManifestFilename), m1); err != nil {
		t.Fatalf("WriteManifest 1 error = %v", err)
	}
	if err := WriteManifest(filepath.Join(b2Dir, ManifestFilename), m2); err != nil {
		t.Fatalf("WriteManifest 2 error = %v", err)
	}

	// Add dummy files
	if err := os.WriteFile(filepath.Join(b1Dir, "data.bin"), make([]byte, 1000), 0o644); err != nil {
		t.Fatalf("WriteFile b1 data error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(b2Dir, "data.bin"), make([]byte, 2000), 0o644); err != nil {
		t.Fatalf("WriteFile b2 data error = %v", err)
	}

	report, err := ListBackupsReport(root)
	if err != nil {
		t.Fatalf("ListBackupsReport error = %v", err)
	}

	if report.TotalCount != 2 {
		t.Fatalf("report.TotalCount = %d, want 2", report.TotalCount)
	}

	// Newest first -> backup-2 must be first
	if report.Backups[0].Name != "backup-2" {
		t.Errorf("report.Backups[0].Name = %q, want backup-2", report.Backups[0].Name)
	}
	if !report.Backups[0].Pinned {
		t.Errorf("report.Backups[0].Pinned = false, want true")
	}
	if report.Backups[0].Reason != "upgrade-v2" {
		t.Errorf("report.Backups[0].Reason = %q, want upgrade-v2", report.Backups[0].Reason)
	}
	if report.Backups[1].Name != "backup-1" {
		t.Errorf("report.Backups[1].Name = %q, want backup-1", report.Backups[1].Name)
	}
	if report.TotalBytes <= 0 {
		t.Errorf("report.TotalBytes = %d, want > 0", report.TotalBytes)
	}
}
