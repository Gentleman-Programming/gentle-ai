//go:build windows

package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	kernel32              = syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceExW   = kernel32.NewProc("GetDiskFreeSpaceExW")
)

func availableBytes(path string) (int64, error) {
	// GetDiskFreeSpaceExW requires an existing directory. Walk up to the nearest
	// existing ancestor so callers can safely pass a not-yet-created destination.
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
			// Reached the filesystem root — path is entirely inaccessible.
			return 0, fmt.Errorf("no existing ancestor found for %q", path)
		}
		path = parent
	}

	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, fmt.Errorf("encode path %q: %w", path, err)
	}

	var avail, total, free uint64
	r, _, err := getDiskFreeSpaceExW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&avail)),
		uintptr(unsafe.Pointer(&total)),
		uintptr(unsafe.Pointer(&free)),
	)
	if r == 0 {
		return 0, fmt.Errorf("GetDiskFreeSpaceExW %q: %w", path, err)
	}
	return int64(avail), nil
}
