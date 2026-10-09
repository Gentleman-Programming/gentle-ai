//go:build !windows

package system

import "os/exec"

// ConfigureCommandProcess configures cmd for safe execution on POSIX systems
// by ensuring a valid working directory via EnsureCommandDir.
func ConfigureCommandProcess(cmd *exec.Cmd, binary string, args []string) {
	if cmd == nil {
		return
	}
	EnsureCommandDir(cmd)
}
