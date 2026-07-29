//go:build unix && !linux && !darwin && !aix && !freebsd && !solaris

package reviewtransaction

import "golang.org/x/sys/unix"

func secureDirectoryOpenFlags() int {
	return unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC
}
