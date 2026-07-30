//go:build windows

package sysproc

import (
	"os/exec"
	"syscall"
)

const CREATE_NO_WINDOW = 0x08000000

// HideConsole prevents a background subprocess from allocating a console window.
func HideConsole(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= CREATE_NO_WINDOW
}
