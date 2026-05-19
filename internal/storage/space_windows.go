//go:build windows

package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	kernel32            = syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceExW = kernel32.NewProc("GetDiskFreeSpaceExW")
)

func availableBytes(path string) (int64, error) {
	// GetDiskFreeSpaceExW requires an existing directory. Walk up to the nearest
	// existing ancestor so callers can pass a not-yet-created destination directory
	// (e.g. the copy target during a data-dir management operation).
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
			// Reached the filesystem root and it doesn't exist (e.g. unmounted drive).
			return 0, fmt.Errorf("no existing ancestor found for %q", path)
		}
		path = parent
	}

	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, fmt.Errorf("encode path %q: %w", path, err)
	}

	var avail, total, free uint64
	r, _, lastErr := getDiskFreeSpaceExW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&avail)),
		uintptr(unsafe.Pointer(&total)),
		uintptr(unsafe.Pointer(&free)),
	)
	if r == 0 {
		if errors.Is(lastErr, syscall.ERROR_ACCESS_DENIED) {
			return 0, fmt.Errorf("permission denied checking space at %q", path)
		}
		return 0, fmt.Errorf("GetDiskFreeSpaceExW %q: %w", path, lastErr)
	}
	return int64(avail), nil
}
