//go:build windows

package shellinstaller

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func userWindowsZipFixture(t *testing.T, names ...string) ([]byte, userWindowsArtifact) {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, name := range names {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte("fixture-only, never executed")); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	data := buffer.Bytes()
	return data, userWindowsArtifact{SHA: userWindowsSHA(data), Prefix: "fixture", Archive: "fixture.zip", Bound: 32 << 20}
}

// userWindowsCacheFixture points LOCALAPPDATA at a private test directory
// and returns the download cache directory inside it.
func userWindowsCacheFixture(t *testing.T) string {
	t.Helper()
	local := t.TempDir()
	t.Setenv("LOCALAPPDATA", local)
	return filepath.Join(local, "gentle-shell", "cache", "downloads")
}

func userWindowsCacheArtifact(data []byte, url string) userWindowsArtifact {
	return userWindowsArtifact{URL: url, SHA: userWindowsSHA(data), Archive: "fixture.zip", Bound: 32 << 20, Size: int64(len(data))}
}

func userWindowsCacheSeed(t *testing.T, cache string, artifact userWindowsArtifact, data []byte) {
	t.Helper()
	if err := os.MkdirAll(cache, 0700); err != nil {
		t.Fatal(err)
	}
	if err := userWindowsWrite(filepath.Join(cache, artifact.Archive), data); err != nil {
		t.Fatal(err)
	}
}

// userWindowsSymlinkOrSkip creates a verified reparse point. Some
// environments (Wine) report symlink creation success while creating
// nothing, which would silently turn an alias test into a different test;
// such environments skip instead.
func userWindowsSymlinkOrSkip(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skipf("symlink creation requires privileges: %v", err)
	}
	if info, err := os.Lstat(newname); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Skipf("symlink creation is unavailable in this environment: %v %v", info, err)
	}
}

func TestUserWindowsAcquireServesVerifiedCacheWithoutNetwork(t *testing.T) {
	cache := userWindowsCacheFixture(t)
	data, _ := userWindowsZipFixture(t, "fixture/fd.exe")
	artifact := userWindowsCacheArtifact(data, "https://127.0.0.1:1/fixture.zip") // Refused instantly if the cache were bypassed.
	userWindowsCacheSeed(t, cache, artifact, data)
	target := filepath.Join(t.TempDir(), "fixture.zip")
	got, err := userWindowsAcquire(context.Background(), artifact, target)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("verified cache entry was not served: %v", err)
	}
	written, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(written, data) {
		t.Fatalf("cache hit did not write the requested target: %v", err)
	}
}

func TestUserWindowsAcquireIgnoresUnverifiedCacheEntries(t *testing.T) {
	cache := userWindowsCacheFixture(t)
	data, _ := userWindowsZipFixture(t, "fixture/fd.exe")
	for name, corrupt := range map[string]func([]byte) []byte{
		"size": func(b []byte) []byte { return append(b, 'x') },
		"sha":  func(b []byte) []byte { b[0] ^= 0xff; return b },
	} {
		artifact := userWindowsCacheArtifact(data, "https://127.0.0.1:1/fixture.zip")
		userWindowsCacheSeed(t, cache, artifact, corrupt(data))
		target := filepath.Join(t.TempDir(), "fixture.zip")
		if _, err := userWindowsAcquire(context.Background(), artifact, target); err == nil {
			t.Fatalf("%s-mismatched cache entry was served without verification", name)
		}
		if _, err := os.Lstat(target); !os.IsNotExist(err) {
			t.Fatalf("%s-mismatched cache entry produced a target", name)
		}
		if err := os.Remove(filepath.Join(cache, artifact.Archive)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestUserWindowsAcquireBypassesForeignCache(t *testing.T) {
	cache := userWindowsCacheFixture(t)
	data, _ := userWindowsZipFixture(t, "fixture/fd.exe")
	artifact := userWindowsCacheArtifact(data, "https://127.0.0.1:1/fixture.zip")
	userWindowsCacheSeed(t, cache, artifact, data)
	// Alias the whole cache directory: the entry would be a valid hit if
	// cache custody were not verified. The bypass must reach the network
	// (refused here) instead of serving aliased bytes.
	aliased := filepath.Join(t.TempDir(), "downloads")
	if err := os.Rename(cache, aliased); err != nil {
		t.Fatal(err)
	}
	userWindowsSymlinkOrSkip(t, aliased, cache)
	if _, err := userWindowsAcquire(context.Background(), artifact, filepath.Join(t.TempDir(), "fixture.zip")); err == nil {
		t.Fatal("aliased cache custody was not bypassed")
	}
}

func TestUserWindowsAcquireStoresVerifiedCacheAndHitsIt(t *testing.T) {
	cache := userWindowsCacheFixture(t)
	data, _ := userWindowsZipFixture(t, "fixture/fd.exe")
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if _, err := w.Write(data); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	artifact := userWindowsCacheArtifact(data, server.URL+"/fixture.zip")
	target := filepath.Join(t.TempDir(), "fixture.zip")
	first, err := userWindowsAcquire(context.Background(), artifact, target)
	if err != nil || !bytes.Equal(first, data) || requests != 1 {
		t.Fatalf("download did not reach the supplier once: requests=%d err=%v", requests, err)
	}
	stored, err := os.ReadFile(filepath.Join(cache, artifact.Archive))
	if err != nil || !bytes.Equal(stored, data) {
		t.Fatalf("verified bytes were not stored in the cache: %v", err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	second, err := userWindowsAcquire(context.Background(), artifact, target)
	if err != nil || !bytes.Equal(second, data) || requests != 1 {
		t.Fatalf("second acquire did not come from the cache: requests=%d err=%v", requests, err)
	}
}

func TestUserWindowsAcquireSurvivesHostileCachePath(t *testing.T) {
	// A regular file occupies the cache directory path: cache custody is
	// bad, so caching must be bypassed entirely while the install itself
	// still succeeds over the network.
	cache := userWindowsCacheFixture(t)
	if err := os.MkdirAll(filepath.Dir(cache), 0700); err != nil {
		t.Fatal(err)
	}
	if err := userWindowsWrite(cache, []byte("not a directory")); err != nil {
		t.Fatal(err)
	}
	data, _ := userWindowsZipFixture(t, "fixture/fd.exe")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write(data); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	artifact := userWindowsCacheArtifact(data, server.URL+"/fixture.zip")
	target := filepath.Join(t.TempDir(), "fixture.zip")
	got, err := userWindowsAcquire(context.Background(), artifact, target)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("bad cache custody failed the install: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(cache))
	if err != nil || len(entries) != 1 || entries[0].Name() != filepath.Base(cache) {
		t.Fatalf("bad cache custody was overwritten: %v %v", entries, err)
	}
}

func TestUserWindowsAcquireFailedDownloadLeavesCacheEmpty(t *testing.T) {
	cache := userWindowsCacheFixture(t)
	_, artifact := userWindowsZipFixture(t, "fixture/fd.exe")
	artifact.URL = "https://127.0.0.1:1/fixture.zip" // Refused before any byte is read.
	target := filepath.Join(t.TempDir(), "fixture.zip")
	if _, err := userWindowsAcquire(context.Background(), artifact, target); err == nil {
		t.Fatal("a refused download was not reported")
	}
	entries, err := os.ReadDir(cache)
	if err != nil || len(entries) != 0 {
		t.Fatalf("a failed download wrote the cache: %v %v", entries, err)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("a failed download wrote the target: %v", err)
	}
}

func TestUserWindowsArchivesRefuseUnsafeNamesBeforeEffects(t *testing.T) {
	for _, names := range [][]string{
		{"fixture/good.txt", "fixture/../escape"},
		{"fixture/good.txt", "fixture//double.txt"},
		{"fixture/good.txt", "fixture"},
		{"fixture/good.txt", "foreign/executable.exe"},
		{"fixture/good.txt", "fixture/alias:stream"},
		{"fixture/good.txt", "fixture/NUL.txt"},
		{"fixture/good.txt", "fixture/dot./entry"},
		{"fixture/good.txt", "fixture/SAME.txt", "fixture/same.txt"},
	} {
		root := t.TempDir()
		data, artifact := userWindowsZipFixture(t, names...)
		if _, err := userWindowsZIP(context.Background(), data, artifact, root, ""); err == nil {
			t.Fatalf("unsafe Windows ZIP admitted: %v", names)
		}
		entries, err := os.ReadDir(root)
		if err != nil || len(entries) != 0 {
			t.Fatalf("refusal had extraction effects: %v %v", entries, err)
		}
	}
}

func TestUserWindowsArchiveAuthorityAndOwnedMember(t *testing.T) {
	data, artifact := userWindowsZipFixture(t, "fixture/fd.exe")
	member, err := userWindowsZIP(context.Background(), data, artifact, "", "fixture/fd.exe")
	if err != nil || string(member) != "fixture-only, never executed" {
		t.Fatalf("selected bytes differ: %q %v", member, err)
	}
	artifact.SHA = "wrong"
	if _, err := userWindowsZIP(context.Background(), data, artifact, filepath.Join(t.TempDir(), "unowned"), "fixture/fd.exe"); err == nil {
		t.Fatal("archive not bound to acquisition admitted")
	}
	artifact.SHA = userWindowsSHA(data)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := userWindowsZIP(ctx, data, artifact, "", "fixture/fd.exe"); err == nil {
		t.Fatal("canceled archive admission")
	}
}
