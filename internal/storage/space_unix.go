//go:build !windows

package storage

import (
	"errors"
	"fmt"
	"syscall"
)

func availableBytes(path string) (int64, error) {
	dir, err := nearestExistingDir(path)
	if err != nil {
		return 0, err
	}

	var stat syscall.Statfs_t
	if err := syscall.Statfs(dir, &stat); err != nil {
		if errors.Is(err, syscall.EACCES) {
			return 0, fmt.Errorf("permission denied checking space at %q", dir)
		}
		return 0, fmt.Errorf("statfs %q: %w", dir, err)
	}
	// Bavail = blocks available to unprivileged users.
	return int64(stat.Bavail) * int64(stat.Bsize), nil
}
