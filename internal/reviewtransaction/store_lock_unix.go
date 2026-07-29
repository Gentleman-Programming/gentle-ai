//go:build unix

package reviewtransaction

import (
	"errors"
	"os"
	"syscall"
)

func tryLockFile(file *os.File) (bool, error) { return tryLockFileMode(file, true) }

func tryLockFileMode(file *os.File, exclusive bool) (bool, error) {
	mode := syscall.LOCK_SH
	if exclusive {
		mode = syscall.LOCK_EX
	}
	err := syscall.Flock(int(file.Fd()), mode|syscall.LOCK_NB)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
		return false, nil
	}
	return false, err
}

func unlockFile(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}
