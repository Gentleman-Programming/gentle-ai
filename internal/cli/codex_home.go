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
	roots := restoreRootsForManifest(homeDir, manifest)
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
			if pathStrictlyInside(entry.OriginalPath, root) {
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

// StandaloneRestoreRoots returns the directories a standalone restore of
// manifest may write under: the home root, plus this process's working
// directory when a manifest entry actually resolves inside it.
//
// Both standalone entries use it — `gentle-ai restore` through
// RestoreManagedBackup and the TUI restore screen through app.tuiRestore — so a
// snapshot whose entries live in a workspace outside the home root (the
// workspace .mcp.json and project settings a workspace-scoped uninstall backs
// up) is restorable again. The uninstaller keeps those same roots in its
// Result; a standalone restore has no Result, so it learns the workspace from
// its own environment and never from the manifest.
func StandaloneRestoreRoots(manifest backup.Manifest) ([]string, error) {
	homeDir, err := backup.UserHomeDirFn()
	if err != nil {
		return nil, err
	}
	return restoreRootsForManifest(homeDir, manifest), nil
}

// restoreRootsForManifest keeps the historical home root and adds the working
// directory only when it differs from the home root and an entry resolves
// strictly inside it. It mirrors uninstall.restoreRootsForPlan, which adds the
// workspace root only when the plan's own backup targets reach it.
func restoreRootsForManifest(homeDir string, manifest backup.Manifest) []string {
	roots := []string{homeDir}
	workspaceDir, ok := workspaceRestoreRoot(homeDir, manifest)
	// guard:population workspace-restore-root too-loose: Only this process's working directory, when a manifest entry resolves strictly inside it and it differs from the home root, extends standalone restore roots; every other entry is still refused by RestoreService.
	if ok {
		roots = append(roots, workspaceDir)
	}
	return roots
}

// workspaceRestoreRoot reports the working directory a restore of manifest
// legitimately spans. The value comes from this process, so a tampered manifest
// cannot widen the roots: an entry can only match a directory this process is
// already running in.
func workspaceRestoreRoot(homeDir string, manifest backup.Manifest) (string, bool) {
	workspaceDir, err := os.Getwd()
	if err != nil {
		return "", false
	}
	workspaceDir, err = filepath.Abs(workspaceDir)
	if err != nil {
		return "", false
	}
	if filepath.Clean(workspaceDir) == filepath.Clean(homeDir) {
		return "", false
	}
	for _, entry := range manifest.Entries {
		if pathStrictlyInside(entry.OriginalPath, workspaceDir) {
			return workspaceDir, true
		}
	}
	return "", false
}

// pathStrictlyInside checks lexical strict containment for absolute paths.
// RestoreService performs the subsequent filesystem and symlink validation.
func pathStrictlyInside(path, root string) bool {
	if !filepath.IsAbs(path) || !filepath.IsAbs(root) {
		return false
	}
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
