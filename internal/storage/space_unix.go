//go:build !windows

package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func availableBytes(path string) (int64, error) {
	// syscall.Statfs requires an existing path. Walk up to the nearest existing
	// ancestor so callers can safely pass a not-yet-created destination.
	for {
		info, err := os.Stat(path)
		if err == nil {
			if !info.IsDir() {
				path = filepath.Dir(path)
				continue
			}
			break // found an existing directory
		}
		parent := filepath.Dir(path)
		if parent == path {
			return 0, fmt.Errorf("no existing ancestor found for %q", path)
		}
		path = parent
	}

	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, fmt.Errorf("statfs %q: %w", path, err)
	}
	// Bavail = blocks available to unprivileged users
	return int64(stat.Bavail) * int64(stat.Bsize), nil
}
