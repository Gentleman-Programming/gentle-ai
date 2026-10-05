package agents

import (
	"path/filepath"
	"strings"
)

// JSONSettingsPath returns the adapter's native SettingsPath only when it is a
// JSON or JSONC file, and "" otherwise. Generic JSON settings writers and
// cleaners use it so native TOML/YAML settings (Kimi, Hermes) stay with their
// owners; SettingsPath itself keeps reporting the native file.
func JSONSettingsPath(homeDir string, adapter Adapter) string {
	path := adapter.SettingsPath(homeDir)
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json", ".jsonc":
		return path
	default:
		return ""
	}
}
