//go:build windows

package engram

import (
	"syscall"
	"unsafe"
)

var (
	_kernel32            = syscall.NewLazyDLL("kernel32.dll")
	_getDiskFreeSpaceExW = _kernel32.NewProc("GetDiskFreeSpaceExW")
)

func platformVolumes() []string {
	var roots []string
	for c := 'A'; c <= 'Z'; c++ {
		root := string(c) + ":\\"
		ptr, err := syscall.UTF16PtrFromString(root)
		if err != nil {
			continue
		}
		var avail, total, free uint64
		r, _, _ := _getDiskFreeSpaceExW.Call(
			uintptr(unsafe.Pointer(ptr)),
			uintptr(unsafe.Pointer(&avail)),
			uintptr(unsafe.Pointer(&total)),
			uintptr(unsafe.Pointer(&free)),
		)
		if r != 0 {
			roots = append(roots, root)
		}
	}
	return roots
}
