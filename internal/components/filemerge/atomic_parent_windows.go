//go:build windows

package filemerge

import (
	"fmt"
	"os"
	"path/filepath"
)

func resolveAtomicParentJunction(dir string, info os.FileInfo) (string, os.FileInfo, bool, error) {
	if info.Mode()&os.ModeIrregular == 0 {
		return "", nil, false, nil
	}

	// os.Readlink supports Windows symlinks and mount-point junctions. Other
	// reparse-point tags return an error and remain rejected.
	if _, err := os.Readlink(dir); err != nil {
		return "", nil, false, fmt.Errorf("resolving directory junction parent %q: %w", dir, err)
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", nil, false, fmt.Errorf("resolving directory junction parent %q: %w", dir, err)
	}
	targetInfo, err := os.Stat(resolved)
	if err != nil {
		return "", nil, false, fmt.Errorf("stat directory junction target %q: %w", resolved, err)
	}
	return resolved, targetInfo, true, nil
}
