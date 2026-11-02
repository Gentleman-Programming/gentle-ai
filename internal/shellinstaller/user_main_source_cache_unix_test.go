//go:build linux || darwin

package shellinstaller

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// userMainSourceCacheFixture builds a retained Main layout with one archived
// member so a full compare has real bytes to authenticate.
func userMainSourceCacheFixture(t *testing.T) (string, userSnapshotArtifact) {
	t.Helper()
	root := t.TempDir()
	privateMust(t, os.MkdirAll(filepath.Join(root, "runtime", "main"), 0700))
	privateMust(t, os.MkdirAll(filepath.Join(root, "runtime", "main-source", "pkg"), 0700))
	privateMust(t, os.WriteFile(filepath.Join(root, "runtime", "main-source", "pkg", "index.mjs"), []byte("export const main = 'source';\n"), 0600))
	artifact := userSnapshotArtifact{
		URL:     "https://canelo.invalid/zip/main.zip",
		SHA:     "0000000000000000000000000000000000000000000000000000000000000000",
		Archive: "main.zip",
		Prefix:  "main",
		Bound:   32 << 20,
	}
	return root, artifact
}

func TestUserMainSourceCacheFallsBackAndRecords(t *testing.T) {
	root, artifact := userMainSourceCacheFixture(t)
	// No retained archive exists in the fixture, so the full compare refuses;
	// the cache must never turn a refused compare into an acceptance.
	if err := userCompareMainSourceCached(context.Background(), root, artifact); err == nil {
		t.Fatal("full compare accepted a fixture without a retained archive")
	}
	if _, ok := userReadMainSourceCache(root); ok {
		t.Fatal("cache record written for a refused compare")
	}
}

func TestUserMainSourceCacheIgnoresMalformedRecords(t *testing.T) {
	root, _ := userMainSourceCacheFixture(t)
	privateMust(t, os.Mkdir(filepath.Join(root, "state"), 0700))
	privateMust(t, os.WriteFile(userMainSourceCachePath(root), []byte("]["), 0600))
	if _, ok := userReadMainSourceCache(root); ok {
		t.Fatal("malformed cache record accepted")
	}
	cache := userMainSourceCache{Schema: userMainSourceCacheSchema, Entries: map[string]userStampRecord{}}
	userWriteMainSourceCache(root, cache)
	if _, ok := userReadMainSourceCache(root); !ok {
		t.Fatal("well-formed record not read back")
	}
}
