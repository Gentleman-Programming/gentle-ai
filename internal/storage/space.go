package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// AvailableBytes returns the number of bytes available to an unprivileged user
// on the volume containing path. path may be an existing file or directory,
// or a path that does not yet exist — in which case the nearest existing
// ancestor is used.
func AvailableBytes(path string) (int64, error) {
	return availableBytes(path)
}

func nearestExistingDir(path string) (string, error) {
	for {
		info, err := os.Stat(path)
		if err == nil {
			if info.IsDir() {
				return path, nil
			}
			path = filepath.Dir(path)
			continue
		}
		if errors.Is(err, os.ErrPermission) {
			return "", fmt.Errorf("permission denied checking space at %q", path)
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("stat %q: %w", path, err)
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", fmt.Errorf("no existing ancestor found for %q", path)
		}
		path = parent
	}
}
