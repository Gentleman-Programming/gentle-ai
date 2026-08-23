package skillregistry

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRefreshSkipFilesystemRoot(t *testing.T) {
	root := "/"
	if runtime.GOOS == "windows" {
		root = filepath.VolumeName(os.Getenv("SystemDrive")) + `\`
	}
	home := t.TempDir()
	if got := RefreshSkip(root, home); got != SkipFilesystemRoot {
		t.Fatalf("RefreshSkip(%q) = %q, want %q", root, got, SkipFilesystemRoot)
	}
}

func TestRefreshSkipHomeDirectoryEvenWithMarkers(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".atl"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := RefreshSkip(home, home); got != SkipHomeDirectory {
		t.Fatalf("RefreshSkip(home, home) = %q, want %q", got, SkipHomeDirectory)
	}
}

func TestRefreshSkipNoProjectMarker(t *testing.T) {
	dir := t.TempDir()
	if got := RefreshSkip(dir, t.TempDir()); got != SkipNoProjectMarker {
		t.Fatalf("RefreshSkip(markerless dir) = %q, want %q", got, SkipNoProjectMarker)
	}
}

func TestRefreshSkipAcceptsProjectMarkers(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, dir string)
	}{
		{"git directory", func(t *testing.T, dir string) {
			if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{"git worktree file", func(t *testing.T, dir string) {
			if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: /elsewhere\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"existing atl registry", func(t *testing.T, dir string) {
			if err := os.MkdirAll(filepath.Join(dir, ".atl"), 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{"project skills workspace", func(t *testing.T, dir string) {
			if err := os.MkdirAll(filepath.Join(dir, ".opencode", "skills"), 0o755); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			tc.setup(t, dir)
			if got := RefreshSkip(dir, t.TempDir()); got != SkipNone {
				t.Fatalf("RefreshSkip(%s) = %q, want SkipNone", tc.name, got)
			}
		})
	}
}
