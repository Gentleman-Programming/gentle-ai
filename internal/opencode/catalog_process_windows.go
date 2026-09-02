//go:build windows

package opencode

import "os/exec"

// configureProcessGroup keeps the default cancellation semantics on Windows:
// the direct child is killed on context cancellation. Process-tree kill on
// Windows requires Job Objects, which catalog discovery does not need today.
func configureProcessGroup(cmd *exec.Cmd) {}
