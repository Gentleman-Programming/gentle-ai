package cli

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/backup"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

// RestoreManagedBackup keeps generic restore defaults unchanged. A managed
// standalone restore may also use the current validated Codex home when an
// entry belongs there. Authority comes from the environment, never the manifest;
// RestoreService still enforces containment and symlink-escape checks.
func RestoreManagedBackup(manifest backup.Manifest) error {
	homeDir, err := backup.UserHomeDirFn()
	if err != nil {
		return err
	}
	roots := []string{homeDir}
	codexRoot := system.CodexConfigDir(homeDir)
	codexRoots := []string{codexRoot}
	// Keep the environment spelling only when it resolves to the selected root.
	// An isolated home must never adopt the real user\'s CODEX_HOME override.
	if userHome, err := os.UserHomeDir(); err == nil && filepath.Clean(homeDir) == filepath.Clean(userHome) && os.Getenv("CODEX_HOME") != "" {
		if spelling, err := filepath.Abs(os.Getenv("CODEX_HOME")); err == nil {
			if canonical, err := filepath.EvalSymlinks(spelling); err == nil && canonical == codexRoot && spelling != codexRoot {
				codexRoots = append(codexRoots, spelling)
			}
		}
	}
	for _, entry := range manifest.Entries {
		for _, root := range codexRoots {
			if pathInsideCodexRoot(entry.OriginalPath, root) {
				if err := system.ValidateCodexHome(homeDir); err != nil {
					return err
				}
				roots = append(roots, codexRoots...)
				return (backup.RestoreService{Roots: roots}).Restore(manifest)
			}
		}
	}
	return (backup.RestoreService{Roots: roots}).Restore(manifest)
}

// pathInsideCodexRoot checks lexical strict containment for absolute paths.
// RestoreService performs the subsequent filesystem and symlink validation.
func pathInsideCodexRoot(path, root string) bool {
	if !filepath.IsAbs(path) || !filepath.IsAbs(root) {
		return false
	}
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
