//go:build aix || freebsd || solaris

package reviewtransaction

import "golang.org/x/sys/unix"

func secureDirectoryOpenFlags() int {
	return unix.O_SEARCH | unix.O_DIRECTORY | unix.O_CLOEXEC
}
