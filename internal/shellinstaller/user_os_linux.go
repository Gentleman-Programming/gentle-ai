//go:build linux

package shellinstaller

import (
	"crypto/x509"
	"os"
	"strings"
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

// userTrustedTmp is the root-owned sticky directory userSupervisorSHA accepts
// as a supervisor ancestor.
const userTrustedTmp = "/tmp"

// Linux checks the operator path exactly as given; the strict EvalSymlinks
// checks refuse every symlink.
func userCanonicalPath(path string) (string, error) {
	return path, nil
}

func userPathsOverlap(a, b string) bool {
	return a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}

// Linux mode bits and ownership already describe every checked entry.
func privateExtendedMetadata(string, os.FileInfo, bool) error {
	return nil
}

func userSync(file userSyncedFile) error {
	return file.Sync()
}

// privateRootsEmpty reports a system pool without any trust anchor.
func privateRootsEmpty(roots *x509.CertPool) bool {
	return len(roots.Subjects()) == 0
}
