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

// rarPrivateDirectoryMkdir and rarPrivateDirectoryChmod are the only two
// filesystem primitives that establish an owner-only RAR directory. They are
// variables so tests can reproduce filesystems that silently ignore the mode
// they are handed -- WSL DrvFS, exFAT, and SMB mounts without POSIX extensions
// -- which no mode argument reachable from a test on ext4 or tmpfs can
// produce. Production always uses the no-follow repair below.
var (
	rarPrivateDirectoryMkdir = os.Mkdir
	rarPrivateDirectoryChmod = repairPrivateRARDirectoryNoFollow
)

// repairPrivateRARDirectoryNoFollow reapplies the owner-only mode through a
// descriptor, never the path: os.Chmod resolves symlinks and Linux rejects
// AT_SYMLINK_NOFOLLOW on fchmodat(2). The validator's own no-follow walk opens
// the directory, and fchmod(2), the mode check and the uid check all run
// against that one descriptor -- substituting a symlink for the just-created
// directory is the attack this walk exists to defeat, and on POSIX it is
// essentially the only other reason this repair ever runs.
func repairPrivateRARDirectoryNoFollow(path string, mode fs.FileMode) error {
	file, err := openRARPathNoFollow(path, true)
	if err != nil {
		return err
	}
	defer file.Close()
	// What the entry IS decides, never who created it. O_NOFOLLOW|O_DIRECTORY
	// already failed the open for a symlink, and an entry owned by anyone else
	// is refused before the write: euid 0 could otherwise chmod it.
	before, err := file.Stat()
	if err != nil {
		return err
	}
	if stat, ok := before.Sys().(*syscall.Stat_t); !ok || !before.IsDir() ||
		stat.Uid != uint32(os.Geteuid()) {
		return unsafeRARPathError(path, true)
	}
	// Only an empty directory is repaired, and emptiness is read from this same
	// descriptor. An interrupted creation leaves nothing behind, so recovery
	// works; a populated authority directory whose mode somebody weakened stays
	// refused, because silently re-tightening it would hide from the operator
	// that anything inside it was writable by anyone.
	if names, readErr := file.Readdirnames(-1); readErr != nil || len(names) > 0 {
		return unsafeRARPathError(path, true)
	}
	if err := file.Chmod(mode); err != nil {
		return err
	}
	repaired, err := file.Stat()
	if err != nil {
		return err
	}
	if !repaired.IsDir() || !privateOpenRARPathSafe(file, repaired) {
		return unsafeRARPathError(path, true)
	}
	return nil
}

func createPrivateRARDirectory(path string) (bool, error) {
	err := rarPrivateDirectoryMkdir(path, 0o700)
	created := err == nil
	if err != nil && !errors.Is(err, fs.ErrExist) {
		return false, err
	}
	validateErr := validatePrivateRARDirectory(path)
	if validateErr != nil {
		// A directory at this path can still fail its own owner-only
		// validation, because some filesystems ignore the mode passed to
		// mkdir(2). Ask for the mode explicitly and revalidate before giving
		// up: that is the whole failure on WSL DrvFS, and it is the difference
		// between the kill switch working and the documented self-service
		// delivery exit being unreachable.
		//
		// Do NOT gate this on `created`: a run killed between mkdir(2) and its
		// repair leaves an unsafe directory that every later mkdir answers with
		// EEXIST, so that gate makes recovery once-only. The no-follow
		// descriptor decides instead: tightening an EMPTY directory this euid
		// already owns is not the world-action errUnsafeRARAuthorityPath
		// declines. A symlink, somebody else's entry, and a directory already
		// holding state all still refuse.
		if repairErr := rarPrivateDirectoryChmod(path, 0o700); repairErr == nil {
			validateErr = validatePrivateRARDirectory(path)
		}
	}
	if validateErr != nil {
		// The just-created directory deliberately stays on disk. Removing it
		// made the refusal name a path that no longer existed, so the repair
		// this product prints (`chmod 700 <path>`) failed with "cannot access
		// ...: No such file or directory" and the operator had no runnable way
		// out. Nothing is trusted by leaving it: every reader revalidates, and
		// the next attempt takes the already-exists branch and refuses again.
		return false, validateErr
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
