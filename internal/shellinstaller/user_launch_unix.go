//go:build linux || darwin

package shellinstaller

import (
	"errors"
	"io"
	"os/exec"
	"syscall"
	"time"
)

// userLaunchGroup runs cmd as a new process group that owns the caller's
// foreground terminal when stdin is it, kills that group on cancellation, and
// reaps it before readback sees the wait result. The terminal is restored only
// after readback; a restore failure turns any result into an uncertain one.
// A descendant that calls setsid or setpgid leaves the group and is not reaped.
func userLaunchGroup(cmd *exec.Cmd, stdin io.Reader, readback func(error) error) (err error) {
	restore, err := userForeground(cmd, stdin)
	if err != nil {
		return err
	}
	defer func() {
		if restoreErr := restore(); restoreErr != nil {
			err = privateError("uncertain", errors.Join(err, restoreErr))
		}
	}()
	cmd.WaitDelay = 2 * time.Second
	cmd.Cancel = func() error { return userKillGroup(cmd.Process.Pid, syscall.SIGKILL) }
	if err := cmd.Start(); err != nil {
		return err
	}
	err = cmd.Wait()
	if reapErr := userReapGroup(cmd.Process.Pid, err); reapErr != nil {
		return reapErr
	}
	return readback(err)
}
