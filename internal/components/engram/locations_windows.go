//go:build windows

package engram

import (
	"syscall"
	"unsafe"
)

var (
	kernel32DLL          = syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceExW  = kernel32DLL.NewProc("GetDiskFreeSpaceExW")
)

// platformVolumes returns drive roots that have available disk space on Windows.
// It iterates A–Z and includes any root (e.g. "C:\") where GetDiskFreeSpaceExW
// succeeds — meaning the drive is present and accessible.
func platformVolumes() []string {
	var roots []string
	for c := 'A'; c <= 'Z'; c++ {
		root := string(c) + ":\\"
		ptr, err := syscall.UTF16PtrFromString(root)
		if err != nil {
			continue
		}
		var avail, total, free uint64
		r, _, _ := getDiskFreeSpaceExW.Call(
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
