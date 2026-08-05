package system

import (
	"os"
	"os/exec"
)

// EnsureCommandDir pins cmd to a working directory that still exists on disk.
//
// A child process inherits the parent's working directory whenever cmd.Dir is
// empty. If that directory was deleted underneath the parent (for example a
// removed Git worktree or temporary directory), Node.js-based children crash
// during bootstrap with "ENOENT: process.cwd failed ... uv_cwd" and the Go
// toolchain refuses to run at all (issue #2148).
//
// When the inherited working directory is healthy, or cmd.Dir is already set,
// this is a no-op, so relative-path behavior is unchanged. When the inherited
// working directory is gone, the child runs in the user's home directory,
// falling back to the system temporary directory.
func EnsureCommandDir(cmd *exec.Cmd) {
	if cmd == nil || cmd.Dir != "" {
		return
	}
	if wd, err := os.Getwd(); err == nil && dirExists(wd) {
		return
	}
	if home, err := os.UserHomeDir(); err == nil && dirExists(home) {
		cmd.Dir = home
		return
	}
	if tmp := os.TempDir(); dirExists(tmp) {
		cmd.Dir = tmp
	}
}

// dirExists reports whether path names an existing directory.
func dirExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
