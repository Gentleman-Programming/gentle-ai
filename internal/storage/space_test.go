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

func TestAvailableBytes_NonExistent(t *testing.T) {
	_, err := storage.AvailableBytes("/this/path/does/not/exist/ever")
	if err == nil {
		t.Fatal("expected error for non-existent path, got nil")
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
