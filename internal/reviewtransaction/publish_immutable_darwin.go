//go:build darwin

package reviewtransaction

import (
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// renameNoReplace is a package-level seam so tests can inject the platform
// syscall failures (ENOTSUP/EINVAL/ENOSYS) that only occur for real on an
// ExFAT- or network-hosted Git common directory, without needing one to run
// in CI (1804).
var renameNoReplace = unix.RenameatxNp

func publishNoReplace(source, destination string) error {
	err := renameNoReplace(unix.AT_FDCWD, source, unix.AT_FDCWD, destination, unix.RENAME_EXCL)
	if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EINVAL) || errors.Is(err, unix.ENOSYS) {
		return publishNoReplaceByCopy(source, destination)
	}
	return err
}

// publishNoReplaceByCopy is the fail-safe fallback when the platform rename
// syscall reports it is unsupported. That is the real shape on an
// ExFAT-hosted Git common directory: the exclusive-rename primitive and hard
// links are both unavailable there. Exclusive create is the atomicity
// primitive every filesystem still offers, so this can never silently
// replace an existing immutable artifact; every other errno from the rename
// syscall still propagates unchanged.
func publishNoReplaceByCopy(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(destination, os.O_RDWR|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(destination)
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		_ = os.Remove(destination)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(destination)
		return err
	}
	return os.Remove(source)
}

func replaceFileAtomic(source, destination string) error {
	return os.Rename(source, destination)
}
