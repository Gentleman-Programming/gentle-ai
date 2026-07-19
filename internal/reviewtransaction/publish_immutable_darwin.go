//go:build darwin

package reviewtransaction

import (
	"os"

	"golang.org/x/sys/unix"
)

func publishNoReplace(source, destination string) error {
	return unix.RenameatxNp(unix.AT_FDCWD, source, unix.AT_FDCWD, destination, unix.RENAME_EXCL)
}

func replaceFileAtomic(source, destination string) error {
	return os.Rename(source, destination)
}
