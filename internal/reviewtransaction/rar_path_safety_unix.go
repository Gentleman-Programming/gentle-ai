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
	if validateErr := repairAndValidatePrivateRARDirectory(path); validateErr != nil {
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

// repairAndValidatePrivateRARDirectory validates path as an owner-only RAR
// directory and, on failure, applies the same no-follow repair
// createPrivateRARDirectory has always used before giving up. #3416: this is
// now the one helper both the just-created path and
// ensureRARDirectoryChain's pre-existing-directory branch call, so repair is
// reachable whether the private directory was just created or already
// existed (some filesystems ignore mkdir(2)'s mode; a directory an older
// release or an interrupted run left behind is exactly as repairable). A
// symlink, somebody else's entry, and a directory already holding state all
// still refuse -- see validatePrivateRARDirectory / the no-follow repair.
func repairAndValidatePrivateRARDirectory(path string) error {
	validateErr := validatePrivateRARDirectory(path)
	if validateErr != nil {
		if repairErr := rarPrivateDirectoryChmod(path, 0o700); repairErr == nil {
			validateErr = validatePrivateRARDirectory(path)
		}
	}
	return validateErr
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

// rarCurrentEUID is os.Geteuid, kept as a variable so a test can pin the
// current-process euid the ownership checks below compare against, instead of
// depending on whichever uid happens to run the test.
var rarCurrentEUID = os.Geteuid

func rarRepositoryDirectorySafe(_ string, info fs.FileInfo) bool {
	if info == nil || !info.IsDir() {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(rarCurrentEUID())
}

func rarRepositoryOpenDirectorySafe(_ *os.File, info fs.FileInfo) bool {
	return rarRepositoryDirectorySafe("", info)
}

// rarPOSIXFUSEProjectedOwnership reports whether path is hosted on a
// filesystem that projects a fixed owner for every path -- a FUSE mount such
// as ntfs-3g given a fixed user_id= -- rather than persisting real per-file
// ownership metadata. Assigned per platform (real statfs-based detection on
// Linux; "never FUSE" elsewhere, since #2838's evidence is Linux-specific);
// kept as a variable so tests can reproduce either outcome, and any probe
// error, without a real FUSE mount.
var rarPOSIXFUSEProjectedOwnership = posixFUSEProjectedOwnership

func formatRARAuthorityRefusal(path string) error {
	// #2838: a directory whose reported owner is a FUSE mount-time
	// projection cannot be fixed with chown -- chown returns EPERM, or is
	// silently ignored, for every path on such a volume -- so the generic
	// remedy is actively unactionable there. Detect that case and name the
	// two routes that can actually change the outcome instead. A probe error
	// falls through to the existing generic message unchanged: an
	// inconclusive probe is not grounds to guess a new one.
	if fuseProjected, probeErr := rarPOSIXFUSEProjectedOwnership(path); probeErr == nil && fuseProjected {
		return errFUSEProjectedRAROwnershipType{path: path, innerErr: errUnsafeRARAuthorityPath}
	}
	return fmt.Errorf(
		"RAR authority parent %q is owned by %s, which is neither the current user nor a trusted administrative authority: %w",
		path, rarRepositoryOwnerDescription(path), errUnsafeRARAuthorityPath,
	)
}

// errFUSEProjectedRAROwnershipType is the distinct #2838 refusal for a
// FUSE-projected owner. It wraps errUnsafeRARAuthorityPath so
// errors.Is(err, errUnsafeRARAuthorityPath) still holds for any caller
// matching on the generic sentinel, but its own message deliberately never
// mentions chown.
type errFUSEProjectedRAROwnershipType struct {
	path     string
	innerErr error
}

func (e errFUSEProjectedRAROwnershipType) Error() string {
	return fmt.Sprintf(
		"RAR authority parent %q is on a filesystem that projects a fixed owner for every path "+
			"(a FUSE mount, such as ntfs-3g given a fixed user_id=), so its reported ownership cannot "+
			"be repaired by changing it: remount without a fixed user_id=/uid= owner projection, or "+
			"move the repository to a filesystem that persists real per-file ownership",
		e.path,
	)
}
func (e errFUSEProjectedRAROwnershipType) Unwrap() error { return e.innerErr }
func (e errFUSEProjectedRAROwnershipType) Is(target error) bool {
	if t, ok := target.(errFUSEProjectedRAROwnershipType); ok {
		return e.innerErr == t.innerErr
	}
	return false
}

var errFUSEProjectedRAROwnership = errFUSEProjectedRAROwnershipType{innerErr: errUnsafeRARAuthorityPath}

func rarRepositoryOwnerDescription(path string) string {
	info, err := os.Lstat(path)
	if err != nil {
		return "an unreadable owner"
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "an unreadable owner"
	}
	return fmt.Sprintf("uid %d (current euid %d)", stat.Uid, rarCurrentEUID())
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
