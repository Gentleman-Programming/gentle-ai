//go:build unix

package opencode

import (
	"os/exec"
	"syscall"
)

// configureProcessGroup places the child in its own process group and makes
// context cancellation kill the entire group. Without this, a descendant that
// inherited stdout or stderr keeps the pipes open and holds discovery open
// past the context deadline even after the direct child has exited.
//
// It returns the same hook pair as the Windows implementation for a uniform
// call site: both are nil here because the group is fully configured before
// Start and there is no handle to release.
func configureProcessGroup(cmd *exec.Cmd) (afterStart func(), release func()) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}
	return nil, nil
}
