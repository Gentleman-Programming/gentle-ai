package system

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// HermesConfigDir returns the Hermes Agent home directory for the current
// platform. See HermesConfigDirFor for the resolution order.
func HermesConfigDir(homeDir string) string {
	return HermesConfigDirFor(homeDir, runtime.GOOS)
}

// HermesConfigDirFor returns the Hermes Agent home directory for goos, using
// the same order Hermes itself uses:
//
//  1. HERMES_HOME, when set.
//  2. %LOCALAPPDATA%\hermes on native Windows.
//  3. ~/.hermes on Linux and macOS.
//
// Environment overrides apply only when homeDir is the real user home, so a
// caller supplying an isolated home keeps every Hermes path inside it.
// %LOCALAPPDATA% is honored only when it lies inside homeDir (its default
// location), so redirecting HOME/USERPROFILE, as tests and sandboxes do, never
// leaks writes into the ambient profile.
func HermesConfigDirFor(homeDir, goos string) string {
	realHome := isRealUserHome(homeDir)

	if realHome {
		if override := strings.TrimSpace(os.Getenv("HERMES_HOME")); override != "" {
			if abs, err := filepath.Abs(override); err == nil {
				return abs
			}
			return override
		}
	}

	if goos == "windows" {
		localAppData := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
		if realHome && filepath.IsAbs(localAppData) && isWithin(homeDir, localAppData) {
			return filepath.Join(localAppData, "hermes")
		}
		return filepath.Join(homeDir, "AppData", "Local", "hermes")
	}

	return filepath.Join(homeDir, ".hermes")
}

func isRealUserHome(homeDir string) bool {
	userHome, err := os.UserHomeDir()
	return err == nil && filepath.Clean(homeDir) == filepath.Clean(userHome)
}

// isWithin reports whether path is root or lies inside it.
func isWithin(root, path string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
