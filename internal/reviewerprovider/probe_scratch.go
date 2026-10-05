package reviewerprovider

import (
	"io/fs"
	"os"
	"path/filepath"
)

// removeProbeScratch removes an adapter scratch directory. A probe may leave
// read-only directories behind (a Go module cache does), which os.RemoveAll
// cannot empty, so on failure every directory is made writable by its owner
// and removal is retried once. Symlinks are never followed.
func removeProbeScratch(dir string) {
	if os.RemoveAll(dir) == nil {
		return
	}
	_ = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err == nil && entry.IsDir() {
			_ = os.Chmod(path, 0o700)
		}
		return nil
	})
	_ = os.RemoveAll(dir)
}
