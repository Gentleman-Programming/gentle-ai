//go:build windows

package reviewerprovider

import (
	"os/exec"
	"syscall"
)

// configureWindowsBatchLaunch rebuilds command's Windows command line when
// binary is a .bat/.cmd target, so a space in its path survives cmd.exe's own
// command-line reparsing (see windowsBatchCommandLine for why). It is a no-op
// for an ordinary .exe, which Go's default quoting already launches correctly.
func configureWindowsBatchLaunch(command *exec.Cmd, binary string, args []string) {
	if !isWindowsBatchFile(binary) {
		return
	}
	command.SysProcAttr = &syscall.SysProcAttr{CmdLine: windowsBatchCommandLine(binary, args)}
}
