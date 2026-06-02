package storage_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/storage"
)

func TestAvailableBytes_TempDir(t *testing.T) {
	dir := t.TempDir()
	n, err := storage.AvailableBytes(dir)
	if err != nil {
		t.Fatalf("AvailableBytes(%q): unexpected error: %v", dir, err)
	}
	if n <= 0 {
		t.Fatalf("AvailableBytes(%q) = %d, want > 0", dir, n)
	}
}

func TestAvailableBytes_File(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "space-test-*")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	n, err := storage.AvailableBytes(f.Name())
	if err != nil {
		t.Fatalf("AvailableBytes(%q): unexpected error: %v", f.Name(), err)
	}
	if n <= 0 {
		t.Fatalf("AvailableBytes(%q) = %d, want > 0", f.Name(), n)
	}
}

// TestAvailableBytes_NonExistentChild verifies that a path whose parent exists
// succeeds by walking up to the nearest existing ancestor. This covers the
// copy/move preflight check where the destination directory does not exist yet.
func TestAvailableBytes_NonExistentChild(t *testing.T) {
	dir := t.TempDir()
	notYet := filepath.Join(dir, "a", "b", "c", "dest")
	n, err := storage.AvailableBytes(notYet)
	if err != nil {
		t.Fatalf("AvailableBytes(%q): unexpected error: %v", notYet, err)
	}
	if n <= 0 {
		t.Fatalf("AvailableBytes(%q) = %d, want > 0", notYet, n)
	}
}

// TestAvailableBytes_PermissionDenied verifies that a chmod-000 directory
// returns a "permission denied" error, not a misleading space-check failure.
func TestAvailableBytes_PermissionDenied(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod 000 is not meaningful on Windows")
	}
	dir := t.TempDir()
	restricted := filepath.Join(dir, "restricted")
	if err := os.Mkdir(restricted, 0o000); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	t.Cleanup(func() { os.Chmod(restricted, 0o755) }) // allow cleanup

	_, err := storage.AvailableBytes(filepath.Join(restricted, "child"))
	if err == nil {
		t.Fatal("expected permission error, got nil")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("error %q should mention 'permission denied'", err)
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.0 KiB"},
		{1536, "1.5 KiB"},
		{1024 * 1024, "1.0 MiB"},
		{int64(1.5 * 1024 * 1024), "1.5 MiB"},
		{1024 * 1024 * 1024, "1.0 GiB"},
		{int64(2469606195), "2.3 GiB"},
	}
	for _, tt := range tests {
		got := storage.FormatBytes(tt.n)
		if got != tt.want {
			t.Errorf("FormatBytes(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}
