package opencode

import (
	"os"
	"path/filepath"
	"strings"
)

func ConfigPath(homeDir string) string {
	if xdgConfigHome := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); filepath.IsAbs(xdgConfigHome) {
		userHome, err := os.UserHomeDir()
		if err == nil && filepath.Clean(homeDir) == filepath.Clean(userHome) {
			return filepath.Join(xdgConfigHome, "opencode")
		}
	}
	return filepath.Join(homeDir, ".config", "opencode")
}
