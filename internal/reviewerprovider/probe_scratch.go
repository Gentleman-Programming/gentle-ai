package reviewerprovider

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// probeScratchParent returns the canonical system temp dir a probe scratch is
// created under. os.MkdirTemp honors TMPDIR, so S11's "never inside the
// workspace" is checked here, before anything is created: both paths are
// made absolute and resolved through symlinks, and a temp dir equal to or
// below the reviewed source root is refused. Returning the resolved path lets
// the caller create the scratch exactly where it was checked.
func probeScratchParent(sourceRoot string) (string, error) {
	if sourceRoot == "" {
		return "", errors.New("refuter probe has no source workspace root to keep its scratch outside")
	}
	source, err := canonicalProbePath(sourceRoot)
	if err != nil {
		return "", fmt.Errorf("resolve refuter probe source workspace: %w", err)
	}
	parent, err := canonicalProbePath(os.TempDir())
	if err != nil {
		return "", fmt.Errorf("resolve system temp dir for the refuter probe: %w", err)
	}
	if relative, err := filepath.Rel(source, parent); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("refuter probe scratch would be inside the reviewed workspace: system temp dir %q resolves to %q under %q; point TMPDIR outside the repository", os.TempDir(), parent, source)
	}
	return parent, nil
}

func canonicalProbePath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(absolute)
}

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
