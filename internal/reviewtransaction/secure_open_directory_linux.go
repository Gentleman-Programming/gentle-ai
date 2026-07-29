//go:build linux

package reviewtransaction

import "golang.org/x/sys/unix"

func secureDirectoryOpenFlags() int {
	return unix.O_PATH | unix.O_DIRECTORY | unix.O_CLOEXEC
}
