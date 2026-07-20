//go:build windows

package sysproc

import (
	"os/exec"
	"syscall"
)

const CREATE_NO_WINDOW = 0x08000000

// HideConsole configures the command so that it does not spawn a new visible
// console window on Windows.
func HideConsole(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= CREATE_NO_WINDOW
}
