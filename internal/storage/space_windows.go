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
	// GetDiskFreeSpaceExW requires a directory path; resolve parent if path is a file.
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		path = filepath.Dir(path)
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
