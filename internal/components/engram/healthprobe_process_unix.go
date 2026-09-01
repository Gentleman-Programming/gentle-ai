//go:build !windows

package engram

import (
	"errors"
	"os/exec"
	"syscall"
)

// startProbeProcessTree isolates the probe in its own process group so a
// child retaining stdout cannot outlive the bounded MCP handshake.
func startProbeProcessTree(command *exec.Cmd) (func() error, error) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		return nil, err
	}
	return func() error {
		if err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			return err
		}
		return nil
	}, nil
}
