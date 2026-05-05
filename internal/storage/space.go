package storage

import (
	"fmt"
	"os"
	"path/filepath"
)

// CheckAvailableSpace returns the number of free bytes on the volume
// containing the given path.
func CheckAvailableSpace(path string) (uint64, error) {
	target, err := nearestExistingPath(path)
	if err != nil {
		return 0, err
	}
	return checkAvailableSpace(target)
}

// RequireFreeSpace validates that the volume containing path has at least
// minBytes of free space. Returns a descriptive error if insufficient.
func RequireFreeSpace(path string, minBytes uint64) error {
	available, err := CheckAvailableSpace(path)
	if err != nil {
		return fmt.Errorf("check disk space for %q: %w", path, err)
	}
	if available < minBytes {
		return fmt.Errorf("insufficient disk space at %q: need %s, have %s",
			path, formatBytes(minBytes), formatBytes(available))
	}
	return nil
}

func nearestExistingPath(path string) (string, error) {
	if path == "" {
		path = "."
	}
	clean, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	for {
		if _, statErr := os.Stat(clean); statErr == nil {
			return clean, nil
		} else if !os.IsNotExist(statErr) {
			return "", statErr
		}
		parent := filepath.Dir(clean)
		if parent == clean {
			return "", os.ErrNotExist
		}
		clean = parent
	}
}
