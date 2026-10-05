//go:build darwin

package reviewtransaction

import (
	"errors"

	"golang.org/x/sys/unix"
)

type secureOpenLeafOperation func(parentFD int, name string, flags int, mode uint32) (int, error)

func secureOpenLocalStoreLockLeaf(parentFD int, name string, flags int, mode uint32) (int, error) {
	return secureOpenLocalStoreLockLeafWith(parentFD, name, flags, mode, unix.Openat)
}

func secureOpenLocalStoreLockLeafWith(parentFD int, name string, flags int, mode uint32, openat secureOpenLeafOperation) (int, error) {
	var fd int
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		fd, err = openat(parentFD, name, flags, mode)
		if err == nil || !errors.Is(err, unix.ENOENT) || attempt == 2 {
			return fd, err
		}
	}
	return fd, err
}
