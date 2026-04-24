//go:build windows

package storage

import (
	"fmt"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

func checkAvailableSpace(path string) (uint64, error) {
	volume := filepath.VolumeName(path)
	if volume == "" {
		volume = "C:\\"
	} else {
		volume += "\\"
	}

	volumePtr, err := windows.UTF16PtrFromString(volume)
	if err != nil {
		return 0, fmt.Errorf("encode volume path %q: %w", volume, err)
	}

	var freeBytesAvailableToCaller, totalNumberOfBytes, totalNumberOfFreeBytes uint64

	modkernel32 := windows.NewLazySystemDLL("kernel32.dll")
	proc := modkernel32.NewProc("GetDiskFreeSpaceExW")

	ret, _, callErr := proc.Call(
		uintptr(unsafe.Pointer(volumePtr)),
		uintptr(unsafe.Pointer(&freeBytesAvailableToCaller)),
		uintptr(unsafe.Pointer(&totalNumberOfBytes)),
		uintptr(unsafe.Pointer(&totalNumberOfFreeBytes)),
	)
	if ret == 0 {
		return 0, fmt.Errorf("GetDiskFreeSpaceExW(%q): %w", volume, callErr)
	}

	return freeBytesAvailableToCaller, nil
}