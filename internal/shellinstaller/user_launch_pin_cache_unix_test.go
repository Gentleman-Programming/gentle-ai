//go:build linux || darwin

package shellinstaller

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func userPinCacheFixture(t *testing.T) (string, string, string) {
	t.Helper()
	root := userUnixTempRoot(t)
	if err := os.Mkdir(filepath.Join(root, "state"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "bin", "tool")
	data := []byte("pinned tool bytes")
	if err := os.WriteFile(path, data, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	pin := fmt.Sprintf("%x", sha256.Sum256(data))
	return root, path, pin
}

func userMustStat(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info
}

func TestUserPinnedFileCachedVerifiesBytesAndRecordsStamps(t *testing.T) {
	root, _, pin := userPinCacheFixture(t)
	digest, err := userPinnedFileCached(context.Background(), root, "bin/tool", 0700, pin)
	if err != nil {
		t.Fatalf("cold verification refused: %v", err)
	}
	if digest == "" {
		t.Fatal("cold verification returned no digest")
	}
	entry, ok := userReadPinCacheEntry(root, "bin/tool")
	if !ok || entry.Pin != pin || entry.Stamp == "" {
		t.Fatalf("stamp record missing after cold verification: %+v ok=%v", entry, ok)
	}
	// A warm start trusts the recorded stamp only while the pin matches.
	if _, err := userPinnedFileCached(context.Background(), root, "bin/tool", 0700, pin); err != nil {
		t.Fatalf("warm verification refused: %v", err)
	}
	// A different pin (update flow) must miss the cache and re-hash.
	if _, err := userPinnedFileCached(context.Background(), root, "bin/tool", 0700, fmt.Sprintf("%x", sha256.Sum256([]byte("other")))); err == nil {
		t.Fatal("mismatched pin accepted")
	}
}

func TestUserPinnedFileCachedRehashesOnDrift(t *testing.T) {
	root, path, pin := userPinCacheFixture(t)
	if _, err := userPinnedFileCached(context.Background(), root, "bin/tool", 0700, pin); err != nil {
		t.Fatalf("cold verification refused: %v", err)
	}
	// Any content rewrite changes ctime, so the recorded stamp must miss and
	// the full hash must run again — and refuse the tampered bytes.
	if err := os.WriteFile(path, []byte("tampered tool bytes"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := userPinnedFileCached(context.Background(), root, "bin/tool", 0700, pin); err == nil {
		t.Fatal("tampered bytes accepted through the stamp cache")
	}
	entry, ok := userReadPinCacheEntry(root, "bin/tool")
	if !ok {
		t.Fatal("stamp record lost after refused re-hash")
	}
	if entry.Stamp == privateStamp(userMustStat(t, path)) {
		t.Fatal("stamp record kept the tampered file's stamp")
	}
}

func TestUserPinCacheIgnoresMalformedRecords(t *testing.T) {
	root, _, pin := userPinCacheFixture(t)
	if err := os.WriteFile(userPinCachePath(root), []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, ok := userReadPinCacheEntry(root, "bin/tool"); ok {
		t.Fatal("malformed cache record accepted")
	}
	if _, err := userPinnedFileCached(context.Background(), root, "bin/tool", 0700, pin); err != nil {
		t.Fatalf("verification refused with malformed cache: %v", err)
	}
}
