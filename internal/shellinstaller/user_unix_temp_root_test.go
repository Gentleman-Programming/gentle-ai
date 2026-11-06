//go:build linux || darwin

package shellinstaller

import (
	"os"
	"path/filepath"
	"testing"
)

// userUnixTempRoot returns an owned 0700 directory spelled through its
// canonical path. On darwin t.TempDir returns the /var alias, which
// privateDirectory refuses; on linux both spellings are identical.
func userUnixTempRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}
