//go:build darwin

package shellinstaller

import (
	"crypto/x509"
	"errors"
	"syscall"

	"golang.org/x/sys/unix"
)

// Per-OS syscall seams for the shared POSIX helpers in user_*_unix.go.

func privateStatTimes(st *syscall.Stat_t) (mtime, ctime syscall.Timespec) {
	return st.Mtimespec, st.Ctimespec
}

// Character devices without terminal ioctls, such as /dev/null, answer ENODEV
// on darwin where Linux answers ENOTTY; both mean "not a terminal".
func userTermios(fd int) (*unix.Termios, error) {
	termios, err := unix.IoctlGetTermios(fd, unix.TIOCGETA)
	if errors.Is(err, unix.ENODEV) {
		return nil, unix.ENOTTY
	}
	return termios, err
}

// userTrustedTmp is the root-owned sticky directory userSupervisorSHA accepts
// as a supervisor ancestor; canonical paths spell /tmp as /private/tmp.
const userTrustedTmp = "/private/tmp"

// Publication refuses an existing destination atomically instead of replacing it.
func userRenameNoReplace(from, to string) error {
	return unix.RenamexNp(from, to, unix.RENAME_EXCL)
}

// privateRootsEmpty is always false on darwin: SystemCertPool delegates to the
// platform verifier and its Subjects list is empty by design. A host without
// trust anchors still fails closed at verification.
func privateRootsEmpty(*x509.CertPool) bool {
	return false
}
