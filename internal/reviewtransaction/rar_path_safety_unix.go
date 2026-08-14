//go:build !windows

package reviewtransaction

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

func rarPathUnsafe(_ string, info fs.FileInfo) bool {
	return info == nil || info.Mode()&os.ModeSymlink != 0
}

func createPrivateRARDirectory(path string) (bool, error) {
	err := os.Mkdir(path, 0o700)
	created := err == nil
	if err != nil && !errors.Is(err, fs.ErrExist) {
		return false, err
	}
	if err := validatePrivateRARDirectory(path); err != nil {
		if created {
			_ = os.Remove(path)
		}
		return false, err
	}
	return created, nil
}

func createPrivateRARFile(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	opened, statErr := file.Stat()
	current, pathErr := os.Lstat(path)
	if statErr != nil || pathErr != nil || !opened.Mode().IsRegular() ||
		!os.SameFile(opened, current) || !privateOpenRARPathSafe(file, opened) {
		_ = file.Close()
		_ = os.Remove(path)
		if statErr != nil {
			return nil, statErr
		}
		if pathErr != nil {
			return nil, pathErr
		}
		return nil, errUnsafeRARAuthorityPath
	}
	return file, nil
}

func privateRARPathSafe(_ string, info fs.FileInfo) bool {
	if info == nil || info.Mode().Perm()&0o077 != 0 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid())
}

func privateOpenRARPathSafe(_ *os.File, info fs.FileInfo) bool {
	return privateRARPathSafe("", info)
}

func rarRepositoryDirectorySafe(_ string, info fs.FileInfo) bool {
	if info == nil || !info.IsDir() {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid())
}

func rarRepositoryOpenDirectorySafe(_ *os.File, info fs.FileInfo) bool {
	return rarRepositoryDirectorySafe("", info)
}

func formatRARAuthorityRefusal(path string) error {
	return fmt.Errorf(
		"RAR authority parent %q is owned by %s, which is neither the current user nor a trusted administrative authority: %w",
		path, rarRepositoryOwnerDescription(path), errUnsafeRARAuthorityPath,
	)
}

func rarRepositoryOwnerDescription(path string) string {
	info, err := os.Lstat(path)
	if err != nil {
		return "an unreadable owner"
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "an unreadable owner"
	}
	return fmt.Sprintf("uid %d (current euid %d)", stat.Uid, os.Geteuid())
}

func openRARPathNoFollow(path string, directory bool) (*os.File, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	parentFD, err := secureOpenLockParent(string(filepath.Separator), filepath.Dir(absolute))
	if err != nil {
		return nil, err
	}
	defer unix.Close(parentFD)
	flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW
	if directory {
		flags |= unix.O_DIRECTORY
	}
	fd, err := unix.Openat(parentFD, filepath.Base(absolute), flags, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}

func validateRARRepositoryParent(path string) error {
	before, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if rarPathUnsafe(path, before) || !before.IsDir() {
		return errUnsafeRARAuthorityPath
	}
	if !rarRepositoryDirectorySafe(path, before) {
		return formatRARAuthorityRefusal(path)
	}
	file, err := openRARPathNoFollow(path, true)
	if err != nil {
		return err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return err
	}
	current, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !os.SameFile(before, opened) || !os.SameFile(opened, current) ||
		!rarRepositoryOpenDirectorySafe(file, opened) {
		return errRARAuthorityPathReplaced
	}
	return nil
}
