//go:build windows

package telemetry

import (
	"os"

	"golang.org/x/sys/windows"
)

// lockFileExclusivePlatform blocks (no LOCKFILE_FAIL_IMMEDIATELY) for an
// exclusive byte-range lock over the file, used only for cross-process
// exclusion. lockState installs it as lockFileExclusive; see there.
//
// The locked range is exactly 1 byte at offset 0, not a max-length range:
// the concrete range matches gofrs/flock and bbolt's converged pattern
// after real same-process races (bbolt#121) and this repository's own
// store lock precedent (internal/reviewtransaction/store_lock_windows.go,
// storeLockCoordinationBytes). A 1-byte range always overlaps between
// handles, so cross-process exclusivity does not depend on how the kernel
// interprets a max-length range. Same-process exclusion does not ride this
// lock at all: lockState carries it with a path-keyed mutex, so the
// intermittent same-process LockFileEx overlap (ga-4882) is unreachable by
// construction.
func lockFileExclusivePlatform(f *os.File) error {
	overlapped := new(windows.Overlapped)
	return windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, overlapped)
}

func unlockFile(f *os.File) error {
	overlapped := new(windows.Overlapped)
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, overlapped)
}
