package pi

import (
	"fmt"
	"os"
	"path/filepath"
)

// GentlePiConfigDirEnv names gentle-pi's config folder itself, the folder
// gentle-pi reads as GENTLE_PI_CONFIG_HOME (default ~/.pi/gentle-ai). A
// global install writes persona.json and background-subagents.json directly
// in it. It must be absolute. GENTLE_PI_CONFIG_HOME keeps its existing
// gentle-ai meaning (the parent of that folder) for current callers.
const GentlePiConfigDirEnv = "GENTLE_AI_PI_CONFIG_DIR"

// GentlePiConfigDir returns gentle-pi's global config folder: the absolute
// GentlePiConfigDirEnv value when set, otherwise <homeDir>/.pi/gentle-ai.
func GentlePiConfigDir(homeDir string) string {
	if dir, ok := GentlePiConfigDirOverride(); ok {
		return dir
	}
	return filepath.Join(ConfigPath(homeDir), "gentle-ai")
}

// GentlePiConfigDirOverride returns the absolute GentlePiConfigDirEnv value.
// A relative value is rejected by ValidateGentlePiConfigDirEnv at startup
// and never used here.
func GentlePiConfigDirOverride() (string, bool) {
	dir := os.Getenv(GentlePiConfigDirEnv)
	if dir == "" || !filepath.IsAbs(dir) {
		return "", false
	}
	return filepath.Clean(dir), true
}

// ValidateGentlePiConfigDirEnv rejects a GentlePiConfigDirEnv value that is
// not an absolute path.
func ValidateGentlePiConfigDirEnv() error {
	if dir := os.Getenv(GentlePiConfigDirEnv); dir != "" && !filepath.IsAbs(dir) {
		return fmt.Errorf("%s must be an absolute path, got %q", GentlePiConfigDirEnv, dir)
	}
	return nil
}
