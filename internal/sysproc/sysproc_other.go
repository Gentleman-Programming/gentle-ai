//go:build !windows

package sysproc

import "os/exec"

// HideConsole is a no-op on non-Windows platforms.
func HideConsole(cmd *exec.Cmd) {
	// No-op
}

// HideBackgroundConsole is a no-op on non-Windows platforms.
func HideBackgroundConsole(cmd *exec.Cmd) {
	// No-op
}

// PreserveConsole is a no-op on non-Windows platforms.
func PreserveConsole(cmd *exec.Cmd) {
	// No-op
}
