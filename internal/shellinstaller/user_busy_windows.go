//go:build windows

package shellinstaller

import (
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// A file under the installation can be held for a moment by an antivirus scan
// or the search indexer right after it is written, and for as long as it runs
// by a Gentle Shell process (node.exe, supervisor.exe, engram.exe) started from
// the installation. Renames and removals of owned paths therefore retry a
// sharing, lock or access violation with a short bounded backoff (about 2.6 s
// in total) and, when the path stays busy, wrap the Windows error with an
// actionable message that still unwraps to it.
var (
	userWindowsBusyDelays = []time.Duration{100 * time.Millisecond, 250 * time.Millisecond, 500 * time.Millisecond, 750 * time.Millisecond, 1000 * time.Millisecond}
	userWindowsBusySleep  = time.Sleep
)

// userWindowsBusyCause reports whether err is a sharing, lock or access
// violation that a closing handle can clear.
func userWindowsBusyCause(err error) bool {
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.ERROR_ACCESS_DENIED)
}

// userWindowsRetryBusy runs one idempotent-on-failure filesystem operation
// (a rename or a removal) and retries it while it fails as busy. A different
// error returns at once, unchanged. A path still busy after the last attempt
// returns an actionable error that keeps the original cause for errors.Is.
func userWindowsRetryBusy(root string, operation func() error) error {
	err := operation()
	for _, delay := range userWindowsBusyDelays {
		if err == nil || !userWindowsBusyCause(err) {
			return err
		}
		userWindowsBusySleep(delay)
		err = operation()
	}
	if err == nil || !userWindowsBusyCause(err) {
		return err
	}
	return fmt.Errorf("%w; a file under %s is still in use by another program (for example an antivirus scan or an open editor or terminal); close it and retry", err, root)
}

func userWindowsRenameBusy(root, from, to string) error {
	return userWindowsRetryBusy(root, func() error { return os.Rename(from, to) })
}

// userWindowsMoveNoReplaceBusy renames from to to like userWindowsRenameBusy
// but without MOVEFILE_REPLACE_EXISTING: an existing destination is never
// overwritten (ERROR_ALREADY_EXISTS is not a busy cause and returns at once).
func userWindowsMoveNoReplaceBusy(root, from, to string) error {
	source, sourceErr := windows.UTF16PtrFromString(from)
	destination, destinationErr := windows.UTF16PtrFromString(to)
	if sourceErr != nil || destinationErr != nil {
		return errors.Join(sourceErr, destinationErr)
	}
	return userWindowsRetryBusy(root, func() error {
		return windows.MoveFileEx(source, destination, windows.MOVEFILE_WRITE_THROUGH)
	})
}

func userWindowsRemoveBusy(root, path string) error {
	return userWindowsRetryBusy(root, func() error { return os.Remove(path) })
}

func userWindowsRemoveAllBusy(root, path string) error {
	return userWindowsRetryBusy(root, func() error { return os.RemoveAll(path) })
}
