package opencode

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	settingsJSON  = "opencode.json"
	settingsJSONC = "opencode.jsonc"
)

// ConfigPath returns the OpenCode global configuration directory for homeDir.
func ConfigPath(homeDir string) string {
	if xdgConfigHome := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); filepath.IsAbs(xdgConfigHome) {
		userHome, err := os.UserHomeDir()
		if err == nil && filepath.Clean(homeDir) == filepath.Clean(userHome) {
			return filepath.Join(xdgConfigHome, "opencode")
		}
	}
	return filepath.Join(homeDir, ".config", "opencode")
}

// SettingsSourcePaths returns global OpenCode settings sources in merge order.
// Later sources take precedence over earlier sources.
func SettingsSourcePaths(configDir string) []string {
	return []string{
		filepath.Join(configDir, settingsJSON),
		filepath.Join(configDir, settingsJSONC),
	}
}

// ManagedSettingsPath chooses the OpenCode settings file Gentle AI manages.
// Existing JSONC takes precedence, followed by existing JSON; new installs keep
// the historical JSON default.
func ManagedSettingsPath(configDir string) string {
	paths := SettingsSourcePaths(configDir)
	if _, err := os.Stat(paths[1]); err == nil {
		return paths[1]
	}
	if _, err := os.Stat(paths[0]); err == nil {
		return paths[0]
	}
	return paths[0]
}

func settingsSourcesForPath(path string) []string {
	switch filepath.Base(path) {
	case settingsJSON, settingsJSONC:
		return SettingsSourcePaths(filepath.Dir(path))
	default:
		return []string{path}
	}
}
