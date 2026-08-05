//go:build linux

package reviewtransaction

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func publishNoReplace(source, destination string) error {
	err := unix.Renameat2(unix.AT_FDCWD, source, unix.AT_FDCWD, destination, unix.RENAME_NOREPLACE)
	if errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EINVAL) || errors.Is(err, unix.EOPNOTSUPP) {
		return os.Link(source, destination)
	}
	return err
}

func replaceFileAtomic(source, destination string) error {
	return os.Rename(source, destination)
}
