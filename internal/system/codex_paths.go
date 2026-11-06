package system

import (
	"fmt"
	"os"
	"path/filepath"
)

// CodexConfigDir returns Codex's global configuration root. CODEX_HOME applies
// only to the real user home; callers supplying an isolated home keep their
// configuration inside it. Relative values are resolved against the process cwd.
// Mutating callers must use ValidateCodexHome before writing: an invalid override
// is never silently redirected to ~/.codex.
func CodexConfigDir(homeDir string) string {
	root, _ := resolveCodexHome(homeDir)
	return root
}

// ValidateCodexHome rejects an invalid explicit CODEX_HOME without creating it.
// The default ~/.codex may be absent because install can create that directory.
func ValidateCodexHome(homeDir string) error {
	_, err := resolveCodexHome(homeDir)
	return err
}

// resolveCodexHome validates and canonicalizes an applicable override without writes.
// Isolated homes and an unset or empty override use the local .codex fallback.
func resolveCodexHome(homeDir string) (string, error) {
	fallback := filepath.Join(homeDir, ".codex")
	override := os.Getenv("CODEX_HOME")
	if override == "" {
		return fallback, nil
	}
	userHome, err := os.UserHomeDir()
	if err != nil || filepath.Clean(homeDir) != filepath.Clean(userHome) {
		return fallback, nil
	}
	root, err := filepath.Abs(override)
	if err != nil {
		return override, fmt.Errorf("resolve CODEX_HOME %q: %w", override, err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return root, fmt.Errorf("validate CODEX_HOME %q: %w", override, err)
	}
	if !info.IsDir() {
		return root, fmt.Errorf("validate CODEX_HOME %q: not a directory", override)
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		return root, fmt.Errorf("canonicalize CODEX_HOME %q: %w", override, err)
	}
	return canonical, nil
}
