package state

import (
	"fmt"
	"os"
	"path/filepath"
)

// HomeEnv names the folder that replaces ~/.gentle-ai for Gentle AI's own
// state: state.json, the managed-assets manifest and journal, telemetry
// state, install backups and the Pi CodeGraph manifest. A bundled Gentle
// Shell sets it to keep that state inside its prefix. It must be absolute.
const HomeEnv = "GENTLE_AI_HOME"

// Dir returns Gentle AI's state folder: the absolute HomeEnv value when set,
// otherwise <homeDir>/.gentle-ai. A relative value is rejected by
// ValidateHomeEnv at startup and never used here.
func Dir(homeDir string) string {
	if override := os.Getenv(HomeEnv); override != "" && filepath.IsAbs(override) {
		return filepath.Clean(override)
	}
	return filepath.Join(homeDir, StateDirName)
}

// BackupsDir returns the folder that holds install and sync backups.
func BackupsDir(homeDir string) string {
	return filepath.Join(Dir(homeDir), "backups")
}

// ValidateHomeEnv rejects a HomeEnv value that is not an absolute path.
func ValidateHomeEnv() error {
	if override := os.Getenv(HomeEnv); override != "" && !filepath.IsAbs(override) {
		return fmt.Errorf("%s must be an absolute path, got %q", HomeEnv, override)
	}
	return nil
}
