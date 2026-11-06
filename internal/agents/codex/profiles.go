package codex

import "path/filepath"

// SddProfilePaths identifies legacy profile paths for compatibility with
// existing installation records. It does not imply ownership of existing files
// or authorize deletion or future profile writes.
func SddProfilePaths(codexHomeDir string) []string {
	return []string{
		filepath.Join(codexHomeDir, "sdd-strong.config.toml"),
		filepath.Join(codexHomeDir, "sdd-mid.config.toml"),
		filepath.Join(codexHomeDir, "sdd-cheap.config.toml"),
	}
}
