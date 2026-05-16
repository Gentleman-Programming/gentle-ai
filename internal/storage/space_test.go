package storage_test

import (
	"os"
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

func TestAvailableBytes_NonExistentChildOfTempDir(t *testing.T) {
	// AvailableBytes walks up to the nearest existing ancestor, so a
	// not-yet-created subdirectory of an existing temp dir must succeed
	// and return the parent volume's available bytes.
	parent := t.TempDir()
	child := parent + "/will-be-created-later/nested"
	n, err := storage.AvailableBytes(child)
	if err != nil {
		t.Fatalf("AvailableBytes(%q): unexpected error for unborn child path: %v", child, err)
	}
	if n <= 0 {
		t.Fatalf("AvailableBytes(%q) = %d, want > 0", child, n)
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
