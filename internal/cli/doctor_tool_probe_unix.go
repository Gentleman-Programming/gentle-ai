//go:build !windows

package cli

import (
	"context"
	"errors"
	"os/exec"
	"syscall"
)

func doctorToolProbeCommand(ctx context.Context, path, arg string) (*exec.Cmd, error) {
	return exec.CommandContext(ctx, path, arg), nil
}

// startDoctorProbeProcessTree isolates the probe in its own process group so a
// child retaining stdout cannot outlive the bounded MCP handshake.
func startDoctorProbeProcessTree(command *exec.Cmd) (func() error, error) {
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
