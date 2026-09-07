//go:build windows

package telemetry

import (
	"os"

	"golang.org/x/sys/windows"
)

// lockFileExclusive blocks (no LOCKFILE_FAIL_IMMEDIATELY) for an exclusive
// byte-range lock over the whole file.
func lockFileExclusive(f *os.File) error {
	overlapped := new(windows.Overlapped)
	return windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, ^uint32(0), ^uint32(0), overlapped)
}

func unlockFile(f *os.File) error {
	overlapped := new(windows.Overlapped)
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, ^uint32(0), ^uint32(0), overlapped)
}
