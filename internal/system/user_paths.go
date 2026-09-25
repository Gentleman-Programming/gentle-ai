package system

import (
	"path/filepath"
)

// AxiomDir returns the canonical user directory for Axiom.
// If AXIOM_STATE_DIR (or legacy GENTLE_AI_STATE_DIR) is set in the environment,
// it takes precedence. Otherwise, it defaults to filepath.Join(homeDir, ".axiom").
func AxiomDir(homeDir string) string {
	if custom := Getenv("AXIOM_STATE_DIR", "GENTLE_AI_STATE_DIR"); custom != "" {
		return custom
	}
	return filepath.Join(homeDir, ".axiom")
}

// StatePath returns the canonical path to state.json under AxiomDir(homeDir).
func StatePath(homeDir string) string {
	return filepath.Join(AxiomDir(homeDir), "state.json")
}

// CacheDir returns the canonical path to cache directory under AxiomDir(homeDir).
func CacheDir(homeDir string) string {
	return filepath.Join(AxiomDir(homeDir), "cache")
}

// BinDir returns the canonical path to bin directory under AxiomDir(homeDir).
func BinDir(homeDir string) string {
	return filepath.Join(AxiomDir(homeDir), "bin")
}

// LegacyDir returns the legacy Gentle AI directory (~/.gentle-ai).
func LegacyDir(homeDir string) string {
	return filepath.Join(homeDir, ".gentle-ai")
}

// LegacyStatePath returns the path to legacy state.json (~/.gentle-ai/state.json).
func LegacyStatePath(homeDir string) string {
	return filepath.Join(LegacyDir(homeDir), "state.json")
}

// LegacyCacheDir returns the path to legacy cache directory (~/.gentle-ai/cache).
func LegacyCacheDir(homeDir string) string {
	return filepath.Join(LegacyDir(homeDir), "cache")
}

// LegacyBinDir returns the path to legacy bin directory (~/.gentle-ai/bin).
func LegacyBinDir(homeDir string) string {
	return filepath.Join(LegacyDir(homeDir), "bin")
}
