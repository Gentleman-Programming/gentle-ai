package system

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFindAllBinaryCopies(t *testing.T) {
	makeBin := func(t *testing.T, dir, name string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatalf("create fake binary: %v", err)
		}
	}

	binaryName := func(name string) string {
		if runtime.GOOS == "windows" {
			return name + ".exe"
		}
		return name
	}

	tests := []struct {
		name      string
		binary    string
		setupDirs int
		binIn     []int // which dirs (0-indexed) get the binary
		wantCount int
	}{
		{
			name:      "single copy found",
			binary:    "engram",
			setupDirs: 3,
			binIn:     []int{1},
			wantCount: 1,
		},
		{
			name:      "duplicate copies in two dirs",
			binary:    "engram",
			setupDirs: 3,
			binIn:     []int{0, 2},
			wantCount: 2,
		},
		{
			name:      "not found anywhere",
			binary:    "engram",
			setupDirs: 2,
			binIn:     []int{},
			wantCount: 0,
		},
		{
			name:      "present in every dir",
			binary:    "gentle-ai",
			setupDirs: 3,
			binIn:     []int{0, 1, 2},
			wantCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var dirs []string
			for i := 0; i < tt.setupDirs; i++ {
				dirs = append(dirs, t.TempDir())
			}

			for _, idx := range tt.binIn {
				makeBin(t, dirs[idx], binaryName(tt.binary))
			}

			t.Setenv("PATH", joinPathList(dirs))

			got := FindAllBinaryCopies(tt.binary)
			if len(got) != tt.wantCount {
				t.Errorf("FindAllBinaryCopies(%q) = %d results, want %d\n  got: %v",
					tt.binary, len(got), tt.wantCount, got)
			}
		})
	}
}

func TestFindAllBinaryCopiesDeduplicatesSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink dedup test requires Unix")
	}

	dir1 := t.TempDir()
	dir2 := t.TempDir()

	realBin := filepath.Join(dir1, "engram")
	if err := os.WriteFile(realBin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("create real binary: %v", err)
	}

	if err := os.Symlink(realBin, filepath.Join(dir2, "engram")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	t.Setenv("PATH", joinPathList([]string{dir1, dir2}))

	got := FindAllBinaryCopies("engram")
	if len(got) != 1 {
		t.Errorf("expected 1 result (symlink deduplicated), got %d: %v", len(got), got)
	}
}

func TestFindAllBinaryCopiesSkipsDirectories(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "engram"), 0o755); err != nil {
		t.Fatalf("create directory: %v", err)
	}

	t.Setenv("PATH", dir)

	got := FindAllBinaryCopies("engram")
	if len(got) != 0 {
		t.Errorf("expected 0 results for directory entry, got %d: %v", len(got), got)
	}
}

func TestFindAllBinaryCopiesSkipsNonExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executable bit not meaningful on Windows")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "engram"), []byte("data"), 0o644); err != nil {
		t.Fatalf("create non-executable file: %v", err)
	}

	t.Setenv("PATH", dir)

	got := FindAllBinaryCopies("engram")
	if len(got) != 0 {
		t.Errorf("expected 0 results for non-executable, got %d: %v", len(got), got)
	}
}

func TestFindAllBinaryCopiesEmptyPath(t *testing.T) {
	t.Setenv("PATH", "")

	got := FindAllBinaryCopies("engram")
	if got != nil {
		t.Errorf("expected nil for empty PATH, got %v", got)
	}
}

// joinPathList builds a PATH string using the OS-appropriate separator.
func joinPathList(dirs []string) string {
	sep := string(os.PathListSeparator)
	result := ""
	for i, d := range dirs {
		if i > 0 {
			result += sep
		}
		result += d
	}
	return result
}
