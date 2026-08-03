// The `unix` pseudo-tag also matches aix and solaris, neither of which defines
// syscall.Flock, and hurd, which has no port in this toolchain. Listing the
// verified targets keeps the constraint honest and fails closed on new ports.
// android satisfies the linux tag and ios satisfies darwin, so both are covered.
//go:build darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd

package state

import (
	"errors"
	"os"
	"syscall"
)

// tryLockFile attempts to take an exclusive advisory lock without blocking.
// It returns (true, nil) when the lock was taken, (false, nil) when another
// holder has it, and (false, err) on any other failure.
// Adapted from internal/reviewtransaction/store_lock_unix.go.
func tryLockFile(file *os.File) (bool, error) {
	err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
		return false, nil
	}
	return false, err
}

// unlockFile releases the lock taken by tryLockFile.
func unlockFile(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}
