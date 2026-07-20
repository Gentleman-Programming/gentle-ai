//go:build !windows

package sysproc

import "os/exec"

// HideConsole is a no-op on non-Windows platforms.
func HideConsole(cmd *exec.Cmd) {
	// No-op
}
