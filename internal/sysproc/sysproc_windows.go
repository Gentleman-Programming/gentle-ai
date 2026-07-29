//go:build windows

package sysproc

import (
	"os/exec"
	"syscall"
)

const CREATE_NO_WINDOW = 0x08000000

// HideConsole configures the command so that background subprocesses spawned on Windows
// do not create a new visible console window when launched from GUI environments.
func HideConsole(cmd *exec.Cmd) {
	HideBackgroundConsole(cmd)
}

// HideBackgroundConsole explicitly scopes CREATE_NO_WINDOW to background subprocesses.
func HideBackgroundConsole(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= CREATE_NO_WINDOW
}

// PreserveConsole explicitly preserves standard console window and handle semantics for commands.
func PreserveConsole(cmd *exec.Cmd) {
	if cmd == nil || cmd.SysProcAttr == nil {
		return
	}
	cmd.SysProcAttr.CreationFlags &^= CREATE_NO_WINDOW
}
