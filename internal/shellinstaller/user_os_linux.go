//go:build linux

package shellinstaller

import (
	"syscall"

	"golang.org/x/sys/unix"
)

// Per-OS syscall seams for the shared POSIX helpers in user_*_unix.go.

func privateStatTimes(st *syscall.Stat_t) (mtime, ctime syscall.Timespec) {
	return st.Mtim, st.Ctim
}

func userTermios(fd int) (*unix.Termios, error) {
	return unix.IoctlGetTermios(fd, unix.TCGETS)
}

// Publication refuses an existing destination atomically instead of replacing it.
func userRenameNoReplace(from, to string) error {
	return unix.Renameat2(unix.AT_FDCWD, from, unix.AT_FDCWD, to, unix.RENAME_NOREPLACE)
}
