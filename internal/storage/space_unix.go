//go:build !windows

package storage

import (
	"fmt"
	"syscall"
)

func checkAvailableSpace(path string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, fmt.Errorf("statfs %q: %w", path, err)
	}
	return stat.Bavail * uint64(stat.Bsize), nil
}
