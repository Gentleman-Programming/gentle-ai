//go:build windows

package reviewtransaction

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func tryLockFile(file *os.File) (bool, error) { return tryLockFileMode(file, true) }

// The locked range is storeLockCoordinationBytes at offset 0, and Windows
// byte-range locks are mandatory: while this lock is held, every read
// overlapping that range fails from any other handle. Readers that must still
// name the recorded holder skip exactly this range, so the two derive from the
// same constant rather than drifting apart (readStoreLockOwnerRecord).
func tryLockFileMode(file *os.File, exclusive bool) (bool, error) {
	overlapped := new(windows.Overlapped)
	flags := uint32(windows.LOCKFILE_FAIL_IMMEDIATELY)
	if exclusive {
		flags |= windows.LOCKFILE_EXCLUSIVE_LOCK
	}
	err := windows.LockFileEx(windows.Handle(file.Fd()), flags, 0, uint32(storeLockCoordinationBytes), 0, overlapped)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return false, nil
	}
	return false, err
}

func unlockFile(file *os.File) error {
	overlapped := new(windows.Overlapped)
	return windows.UnlockFileEx(windows.Handle(file.Fd()), 0, uint32(storeLockCoordinationBytes), 0, overlapped)
}
