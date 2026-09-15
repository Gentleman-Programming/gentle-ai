package opencode

import (
	"os"
	"path/filepath"
	"strings"
)

func ConfigPath(homeDir string) string {
	userHome, err := os.UserHomeDir()
	if err == nil && filepath.Clean(homeDir) != filepath.Clean(userHome) {
		// homeDir is a workspace root (install --scope workspace), not the
		// user's real home directory. OpenCode's project-local convention is
		// <workspace>/.opencode -- confirmed by OpenCode's own installer
		// output for other components (e.g. openspec's .opencode/skills,
		// .opencode/commands). The XDG ~/.config/opencode nesting below
		// applies only to the global scope, where homeDir really is the
		// user's home directory.
		return filepath.Join(homeDir, ".opencode")
	}

	if xdgConfigHome := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); filepath.IsAbs(xdgConfigHome) {
		return filepath.Join(xdgConfigHome, "opencode")
	}
	return filepath.Join(homeDir, ".config", "opencode")
}
