//go:build windows

package storage

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

var (
	kernel32            = syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceExW = kernel32.NewProc("GetDiskFreeSpaceExW")
)

func availableBytes(path string) (int64, error) {
	dir, err := nearestExistingDir(path)
	if err != nil {
		return 0, err
	}

	pathPtr, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		return 0, fmt.Errorf("encode path %q: %w", dir, err)
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
			return 0, fmt.Errorf("permission denied checking space at %q", dir)
		}
		return 0, fmt.Errorf("GetDiskFreeSpaceExW %q: %w", dir, lastErr)
	}
	return int64(avail), nil
}
