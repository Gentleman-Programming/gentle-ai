//go:build windows

package shellinstaller

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
)

// userLockFile opens (or creates) the lock file without following a reparse
// point and takes a non-blocking exclusive LockFileEx on its first byte. The
// handle is not inheritable, so no installer descendant keeps the lock after
// its owner exits; Windows drops the lock when the handle closes.
func userLockFile(path, destination string) (func() error, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		if errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
			return nil, &UserShellOperationInProgressError{Destination: destination}
		}
		return nil, fmt.Errorf("open Gentle Shell operation lock %s: %w", path, err)
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		_ = windows.CloseHandle(handle)
		return nil, fmt.Errorf("inspect Gentle Shell operation lock %s: %w", path, err)
	}
	if info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 || info.NumberOfLinks != 1 {
		_ = windows.CloseHandle(handle)
		return nil, fmt.Errorf("Gentle Shell operation lock %s is not a regular file", path)
	}
	overlapped := new(windows.Overlapped)
	if err := windows.LockFileEx(handle, windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, overlapped); err != nil {
		_ = windows.CloseHandle(handle)
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.ERROR_IO_PENDING) {
			return nil, &UserShellOperationInProgressError{Destination: destination}
		}
		return nil, fmt.Errorf("lock Gentle Shell operation %s: %w", path, err)
	}
	return func() error {
		return errors.Join(windows.UnlockFileEx(handle, 0, 1, 0, overlapped), windows.CloseHandle(handle))
	}, nil
}
