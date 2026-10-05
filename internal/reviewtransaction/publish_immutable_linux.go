//go:build linux

package reviewtransaction

import (
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

type immutablePublicationDestination interface {
	io.Writer
	Chmod(os.FileMode) error
	Sync() error
	Close() error
}

// These seams exercise filesystem failures that require a mounted filesystem
// or a full disk in production, including the DrvFS/9p EXDEV case (#2583).
var (
	renameNoReplace                       = unix.Renameat2
	createImmutablePublicationDestination = func(path string, flag int, mode os.FileMode) (immutablePublicationDestination, error) {
		return os.OpenFile(path, flag, mode)
	}
	copyImmutablePublication   = io.Copy
	removeImmutablePublication = os.Remove
)

func publishNoReplace(source, destination string) error {
	err := renameNoReplace(unix.AT_FDCWD, source, unix.AT_FDCWD, destination, unix.RENAME_NOREPLACE)
	if errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EINVAL) || errors.Is(err, unix.EOPNOTSUPP) || errors.Is(err, unix.EXDEV) {
		return publishNoReplaceByCopy(source, destination)
	}
	return err
}

// publishNoReplaceByCopy preserves immutability where Linux cannot rename or
// hard-link across the publication boundary. O_EXCL is the no-replace primitive.
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

	out, err := createImmutablePublicationDestination(destination, os.O_RDWR|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	if err := out.Chmod(info.Mode().Perm()); err != nil {
		return errors.Join(err, out.Close(), removeImmutablePublication(destination))
	}
	if _, err := copyImmutablePublication(out, in); err != nil {
		return errors.Join(err, out.Close(), removeImmutablePublication(destination))
	}
	if err := out.Sync(); err != nil {
		return errors.Join(err, out.Close(), removeImmutablePublication(destination))
	}
	if err := out.Close(); err != nil {
		return errors.Join(err, removeImmutablePublication(destination))
	}
	return removeImmutablePublication(source)
}

func replaceFileAtomic(source, destination string) error {
	return os.Rename(source, destination)
}
