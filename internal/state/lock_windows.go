//go:build windows

package state

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// tryLockFile is the Windows half of the contract documented in lock_unix.go.
// Adapted from internal/reviewtransaction/store_lock_windows.go.
func tryLockFile(file *os.File) (bool, error) {
	overlapped := new(windows.Overlapped)
	flags := uint32(windows.LOCKFILE_FAIL_IMMEDIATELY | windows.LOCKFILE_EXCLUSIVE_LOCK)
	err := windows.LockFileEx(windows.Handle(file.Fd()), flags, 0, 1, 0, overlapped)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return false, nil
	}
	return false, err
}

// unlockFile releases the lock taken by tryLockFile.
func unlockFile(file *os.File) error {
	overlapped := new(windows.Overlapped)
	return windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, overlapped)
}
