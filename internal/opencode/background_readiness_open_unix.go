//go:build unix

package opencode

import "syscall"

// startupOpenFlags keeps opening a startup file from blocking on a FIFO.
const startupOpenFlags = syscall.O_NONBLOCK
