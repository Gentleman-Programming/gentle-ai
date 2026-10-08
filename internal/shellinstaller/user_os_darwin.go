//go:build darwin

package shellinstaller

import (
	"syscall"

	"golang.org/x/sys/unix"
)

// Per-OS syscall seams for the shared POSIX helpers in user_*_unix.go.
// Darwin entry points still refuse in user_install_unsupported.go.

func privateStatTimes(st *syscall.Stat_t) (mtime, ctime syscall.Timespec) {
	return st.Mtimespec, st.Ctimespec
}

func userTermios(fd int) (*unix.Termios, error) {
	return unix.IoctlGetTermios(fd, unix.TIOCGETA)
}

// Publication refuses an existing destination atomically instead of replacing it.
func userRenameNoReplace(from, to string) error {
	return unix.RenamexNp(from, to, unix.RENAME_EXCL)
}
