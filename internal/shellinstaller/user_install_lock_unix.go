//go:build linux || darwin

package shellinstaller

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// userLockAttempts bounds the retries when a holder releases (and removes)
// the lock file between this process opening and locking it.
const userLockAttempts = 3

// userLockFile opens (or creates) the private lock file without following a
// link and takes a non-blocking flock. The descriptor is close-on-exec, so no
// installer descendant can keep the lock alive after its owner exits.
//
// The holder removes the file before it releases the lock, so the parent keeps
// no lock residue. A lock taken on a file the name no longer refers to was
// released meanwhile: it is dropped and the name is opened again.
func userLockFile(path, destination string) (func() error, error) {
	for attempt := 0; attempt < userLockAttempts; attempt++ {
		fd, err := unix.Open(path, unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0o600)
		if err != nil {
			return nil, fmt.Errorf("open Gentle Shell operation lock %s: %w", path, err)
		}
		var stat unix.Stat_t
		if err := unix.Fstat(fd, &stat); err != nil {
			_ = unix.Close(fd)
			return nil, fmt.Errorf("inspect Gentle Shell operation lock %s: %w", path, err)
		}
		if stat.Mode&unix.S_IFMT != unix.S_IFREG || int(stat.Uid) != os.Geteuid() || stat.Nlink != 1 || stat.Mode&0o077 != 0 {
			_ = unix.Close(fd)
			return nil, fmt.Errorf("Gentle Shell operation lock %s is not a private regular file owned by this user", path)
		}
		if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
			_ = unix.Close(fd)
			if errors.Is(err, unix.EWOULDBLOCK) {
				return nil, &UserShellOperationInProgressError{Destination: destination}
			}
			return nil, fmt.Errorf("lock Gentle Shell operation %s: %w", path, err)
		}
		var current unix.Stat_t
		if err := unix.Lstat(path, &current); err != nil || current.Dev != stat.Dev || current.Ino != stat.Ino {
			_ = unix.Close(fd)
			if err == nil || errors.Is(err, unix.ENOENT) {
				continue
			}
			return nil, fmt.Errorf("inspect Gentle Shell operation lock %s: %w", path, err)
		}
		return func() error {
			// Removing the name first keeps every later opener off this file;
			// a failed removal only leaves an unlocked file the next run reuses.
			_ = unix.Unlink(path)
			return unix.Close(fd)
		}, nil
	}
	return nil, &UserShellOperationInProgressError{Destination: destination}
}
