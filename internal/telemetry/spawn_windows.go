//go:build windows

package telemetry

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// configureDetachedProcAttr asks Windows not to allocate or inherit a
// console for the detached telemetry sender. Without this, spawning the
// child (itself launched through cmd.exe for a .bat/.cmd fixture in tests,
// and potentially attached to the parent's console in production) can incur
// console-allocation overhead or leave the child tied to a console the
// triggering command no longer owns; CREATE_NEW_PROCESS_GROUP additionally
// keeps Ctrl+C delivered to the parent's console from reaching the child.
func configureDetachedProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS,
	}
}
