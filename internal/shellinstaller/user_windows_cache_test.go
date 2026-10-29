//go:build windows

package shellinstaller

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestUserWindowsCacheOverlaps pins the containment predicate that keeps the
// optional download cache away from an installation destination. The cache is
// created before publication, so any overlap would make the installer create
// its own destination and every fresh install would abort with "Windows
// selection changed before publication".
func TestUserWindowsCacheOverlaps(t *testing.T) {
	root := filepath.Join(`C:\Users\fixture\AppData\Local`)
	cache := filepath.Join(root, "gentle-shell", "cache", "downloads")
	cases := []struct {
		name        string
		destination string
		want        bool
	}{
		{name: "destination is the cache", destination: cache, want: true},
		{name: "destination contains the cache", destination: filepath.Join(root, "gentle-shell"), want: true},
		{name: "destination above the cache", destination: root, want: true},
		{name: "destination below the cache", destination: filepath.Join(cache, "nested"), want: true},
		{name: "sibling with a shared prefix", destination: filepath.Join(root, "gentle-shell2"), want: false},
		{name: "unrelated destination", destination: filepath.Join(root, "Programs", "gentle-shell"), want: false},
		{name: "empty destination is not an overlap", destination: "", want: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := userWindowsCacheOverlaps(testCase.destination, cache); got != testCase.want {
				t.Fatalf("userWindowsCacheOverlaps(%q, %q) = %v, want %v", testCase.destination, cache, got, testCase.want)
			}
		})
	}
}

// TestUserWindowsCacheRefusesOverlappingDestination pins the regression this
// fix exists for: with the destination equal to the cache path, resolving the
// cache must decline and leave the destination absent, so the pre-publication
// re-inspection still matches the wizard token.
func TestUserWindowsCacheRefusesOverlappingDestination(t *testing.T) {
	local := t.TempDir()
	t.Setenv("LOCALAPPDATA", local)
	cache := filepath.Join(local, "gentle-shell", "cache", "downloads")

	directory, ok := userWindowsDownloadCacheDirectory(cache)
	if ok || directory != "" {
		t.Fatalf("cache resolved to %q under an overlapping destination, want a bypass", directory)
	}
	if _, err := os.Lstat(cache); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the refused cache created %q: %v", cache, err)
	}
	for _, destination := range []string{
		filepath.Join(local, "gentle-shell"),
		filepath.Join(cache, "nested"),
	} {
		if directory, ok := userWindowsDownloadCacheDirectory(destination); ok || directory != "" {
			t.Fatalf("cache resolved to %q for overlapping destination %q, want a bypass", directory, destination)
		}
		if _, err := os.Lstat(cache); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("the refused cache created %q for destination %q: %v", cache, destination, err)
		}
	}
}

// TestUserWindowsCacheKeepsNonOverlappingDestination pins that the cache stays
// available for an ordinary destination, so the refusal above is a narrow
// bypass and not a blanket disablement.
func TestUserWindowsCacheKeepsNonOverlappingDestination(t *testing.T) {
	local := t.TempDir()
	t.Setenv("LOCALAPPDATA", local)
	cache := filepath.Join(local, "gentle-shell", "cache", "downloads")

	directory, ok := userWindowsDownloadCacheDirectory(filepath.Join(local, "Programs", "gentle-shell"))
	if !ok || directory != cache {
		t.Fatalf("cache resolved to %q (ok=%v), want %q", directory, ok, cache)
	}
	info, err := os.Lstat(cache)
	if err != nil || !info.IsDir() {
		t.Fatalf("the ordinary cache directory was not created: %v", err)
	}
}
