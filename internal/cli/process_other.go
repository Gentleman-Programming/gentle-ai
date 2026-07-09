//go:build !windows

package cli

import "os/exec"

// hideConsoleWindow is a no-op on non-Windows platforms.
func hideConsoleWindow(_ *exec.Cmd) {}
