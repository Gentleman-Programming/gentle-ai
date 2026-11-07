//go:build windows

package shellinstaller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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
		// Windows paths are case-insensitive, so a differently cased destination
		// names the same directory: the refusal must hold on the leaf component
		// and on the drive letter alike, or the cache recreates the destination
		// whenever the operator's casing differs from the resolved cache path.
		{name: "destination differs only by casing", destination: `C:\Users\fixture\AppData\Local\GENTLE-SHELL`, want: true},
		{name: "destination differs only in drive-letter casing", destination: `c:\Users\fixture\AppData\local\gentle-shell`, want: true},
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

// userWindowsCodeGraphSourceFixture mirrors the anonymous element type of
// userWindowsCodeGraphSources so a test can substitute a local source without a
// network or a real pinned archive.
type userWindowsCodeGraphSourceFixture = struct {
	name, url, sri, sha, archive string
	size, unpacked               int64
	entries                      int
	skipNode                     bool
}

// TestUserWindowsCodeGraphAcquireForwardsDestinationToTheCache pins the
// CodeGraph call site of the download cache. The wrapper introduced for issue
// #5847 decides the reuse path and hands everything else to provisioning; if it
// drops req.Destination there, a CodeGraph-enabled fresh install into
// %LOCALAPPDATA%\gentle-shell recreates the download cache, the installer
// creates its own destination and publication aborts with "Windows selection
// changed before publication". The substituted source fails acquisition on
// purpose: the cache is resolved before the download, so the cache path is
// observable without the network.
func TestUserWindowsCodeGraphAcquireForwardsDestinationToTheCache(t *testing.T) {
	parent := t.TempDir()
	if err := userWindowsPrivate(parent); err != nil {
		t.Fatal(err)
	}
	local := t.TempDir()
	t.Setenv("LOCALAPPDATA", local)
	cache := filepath.Join(local, "gentle-shell", "cache", "downloads")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte("not the pinned archive")); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	original := userWindowsCodeGraphSources
	userWindowsCodeGraphSources = []userWindowsCodeGraphSourceFixture{{
		name: "fixture", url: server.URL + "/codegraph.tgz",
		sha: userWindowsSHA([]byte("pinned")), archive: "codegraph.fixture.tgz",
		size: int64(len("pinned")), skipNode: true,
	}}
	t.Cleanup(func() { userWindowsCodeGraphSources = original })

	stage := filepath.Join(parent, "stage")
	if err := os.MkdirAll(stage, 0700); err != nil {
		t.Fatal(err)
	}
	req := UserInstallRequest{Experience: &UserExperience{CodeGraph: "on"}}
	// The destination is the cache path itself: the overlap the wrapper must
	// forward, exactly as a fresh install into %LOCALAPPDATA%\gentle-shell.
	req.Destination = cache
	if err := userWindowsCodeGraphAcquire(context.Background(), stage, req, userInstallScope{}); err == nil {
		t.Fatal("the substituted CodeGraph source did not fail acquisition")
	}
	if _, err := os.Lstat(cache); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("CodeGraph acquire created the download cache under an overlapping destination: %v", err)
	}
}
