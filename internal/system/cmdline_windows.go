//go:build windows

package system

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// ConfigureCommandProcess configures cmd for safe execution on Windows.
// It ensures a valid working directory via EnsureCommandDir, sets
// CREATE_NO_WINDOW on SysProcAttr.CreationFlags to prevent console flashing,
// and populates SysProcAttr.CmdLine if the target binary is a Windows batch file
// (.bat or .cmd).
//
// NOTE: Windows Job Objects are strictly excluded in this phase.
func ConfigureCommandProcess(cmd *exec.Cmd, binary string, args []string) {
	if cmd == nil {
		return
	}
	EnsureCommandDir(cmd)
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NO_WINDOW

	target := binary
	if !IsWindowsBatchFile(target) && cmd.Path != "" && IsWindowsBatchFile(cmd.Path) {
		target = cmd.Path
	}
	if IsWindowsBatchFile(target) {
		cmd.SysProcAttr.CmdLine = WindowsBatchCommandLine(target, args)
	}
}
