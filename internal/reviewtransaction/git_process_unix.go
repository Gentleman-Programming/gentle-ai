//go:build !windows

package reviewtransaction

import (
	"os/exec"
	"syscall"
)

func startGitProcessTree(command *exec.Cmd) (func() error, error) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		return nil, err
	}
	return func() error { return syscall.Kill(-command.Process.Pid, syscall.SIGKILL) }, nil
}
