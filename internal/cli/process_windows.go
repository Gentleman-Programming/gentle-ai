//go:build windows

package cli

import (
	"os/exec"
	"syscall"
)

// hideConsoleWindow sets CREATE_NO_WINDOW on the process so background
// commands (MCP servers, upgrade helpers) don't spawn visible Git Bash
// or cmd.exe popup windows on Windows. Issue #197.
func hideConsoleWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | 0x08000000, // CREATE_NO_WINDOW
	}
}
