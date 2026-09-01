package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
)

// UserHomeDirFn is the function used to resolve the user's home directory.
// Package-level var for testability — swapped in tests to use a temp directory.
var UserHomeDirFn = os.UserHomeDir

// isPathUnderRoot reports whether path is an absolute path that resides under
// root. This is used to prevent arbitrary file writes via tampered manifest
// OriginalPath fields: root must come from the caller (something it knows out
// of band, e.g. the home or workspace directory it actually installed into),
// never from the manifest itself — a tampered manifest would otherwise simply
// declare a wider root and walk straight through the guard.
//
// Symlink note: if the path already exists on disk, EvalSymlinks is used to
// resolve the real path and re-check against root, preventing symlink escapes.
// If the path does not exist yet (typical during restore), only filepath.Clean
// is used — symlinks cannot be resolved for non-existent paths, so this
// limitation is accepted and documented here.
func isPathUnderRoot(path, root string) bool {
	rootClean := filepath.Clean(root)
	if rootClean == "" || rootClean == "." {
		return false
	}
	clean := filepath.Clean(path)
	if !strings.HasPrefix(clean, rootClean+string(filepath.Separator)) {
		return false
	}
	// If the path exists, resolve symlinks and re-check to prevent symlink escapes.
	if resolved, err := filepath.EvalSymlinks(clean); err == nil {
		resolvedRoot, err := filepath.EvalSymlinks(rootClean)
		if err != nil {
			resolvedRoot = rootClean
		}
		return strings.HasPrefix(resolved, resolvedRoot+string(filepath.Separator))
	}
	// Path does not exist yet (file will be created by restore) — accept Clean-only check.
	return true
}

// isPathUnderRoots reports whether path resides under at least one of roots.
func isPathUnderRoots(path string, roots []string) bool {
	for _, root := range roots {
		if isPathUnderRoot(path, root) {
			return true
		}
	}
	return false
}

// invalidOriginalPathErr builds the refusal returned when a manifest entry's
// OriginalPath does not resolve under any allowed root. It names the roots
// actually validated against so a user who hits it can tell which boundary
// was crossed.
func invalidOriginalPathErr(originalPath string, roots []string) error {
	return fmt.Errorf("manifest entry has invalid OriginalPath %q: must be an absolute path under an allowed root (%s)", originalPath, strings.Join(roots, ", "))
}

// RestoreService restores a backup manifest, writing back or removing files
// at their OriginalPath.
//
// Roots restricts which directories a restore is allowed to write to or
// remove from. Every non-pre-existing manifest entry's OriginalPath must
// resolve under at least one Roots entry, or the restore refuses it.
//
// When Roots is empty, Restore falls back to the single directory returned by
// UserHomeDirFn — the historical, backward-compatible behavior. This is the
// correct default for standalone restores (`gentle-ai restore <id>` and the
// TUI "restore from list" screen): the backup being restored may be
// arbitrarily old, and the workspace root that was in effect when it was
// created is not something the current process can safely rediscover on its
// own, so it falls back to the one directory it explicitly owns.
//
// Callers that are still inside the same install/sync run that produced the
// manifest (e.g. pipeline rollback) know their own scope roots out of band
// and should set Roots explicitly — see rollbackRoots in internal/cli.
type RestoreService struct {
	Roots []string
}

// allowedRoots resolves the roots this restore may write under.
func (s RestoreService) allowedRoots() ([]string, error) {
	if len(s.Roots) > 0 {
		return s.Roots, nil
	}
	home, err := UserHomeDirFn()
	if err != nil {
		return nil, err
	}
	return []string{home}, nil
}

func (s RestoreService) Restore(manifest Manifest) error {
	if manifest.Compressed {
		return s.restoreCompressed(manifest)
	}
	return s.restorePlain(manifest)
}

// restoreCompressed handles backups where Compressed==true.
// It extracts the tar.gz archive into a temp directory, then restores each
// entry by resolving the relative SnapshotPath inside that temp directory.
func (s RestoreService) restoreCompressed(manifest Manifest) error {
	roots, err := s.allowedRoots()
	if err != nil {
		return fmt.Errorf("resolve restore roots: %w", err)
	}

	tempDir, err := os.MkdirTemp("", "gentle-ai-restore-*")
	if err != nil {
		return fmt.Errorf("create temp restore dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	archivePath := filepath.Join(manifest.RootDir, ArchiveFilename)
	if _, err := ExtractArchive(archivePath, tempDir); err != nil {
		return fmt.Errorf("extract archive %q: %w", archivePath, err)
	}

	for _, entry := range manifest.Entries {
		if entry.Existed {
			// SnapshotPath must be relative inside the archive (e.g. "files/.config/foo.json").
			// An absolute path would cause filepath.Join to ignore tempDir, reading from
			// the live filesystem instead of the extraction directory.
			if filepath.IsAbs(entry.SnapshotPath) {
				return fmt.Errorf("manifest entry %q has absolute SnapshotPath %q, expected relative", entry.OriginalPath, entry.SnapshotPath)
			}
			resolvedEntry := ManifestEntry{
				OriginalPath: entry.OriginalPath,
				SnapshotPath: filepath.Join(tempDir, filepath.FromSlash(entry.SnapshotPath)),
				Existed:      true,
				Mode:         entry.Mode,
			}
			if err := restoreEntry(resolvedEntry, true, roots); err != nil {
				return err
			}
			continue
		}

		if !filepath.IsAbs(entry.OriginalPath) || !isPathUnderRoots(entry.OriginalPath, roots) {
			return invalidOriginalPathErr(entry.OriginalPath, roots)
		}
		if err := os.Remove(entry.OriginalPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove path %q: %w", entry.OriginalPath, err)
		}
	}

	return nil
}

// restorePlain handles old-style backups where Compressed==false.
// SnapshotPath is an absolute path to a plain file on disk.
func (s RestoreService) restorePlain(manifest Manifest) error {
	roots, err := s.allowedRoots()
	if err != nil {
		return fmt.Errorf("resolve restore roots: %w", err)
	}

	for _, entry := range manifest.Entries {
		if entry.Existed {
			if err := restoreEntry(entry, false, roots); err != nil {
				return err
			}
			continue
		}

		if !filepath.IsAbs(entry.OriginalPath) || !isPathUnderRoots(entry.OriginalPath, roots) {
			return invalidOriginalPathErr(entry.OriginalPath, roots)
		}
		if err := os.Remove(entry.OriginalPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove path %q: %w", entry.OriginalPath, err)
		}
	}

	return nil
}

// restoreEntry writes the snapshot file at entry.SnapshotPath back to entry.OriginalPath.
// trustedSnapshot must be true when SnapshotPath has already been resolved to a safe
// temp directory (compressed restores), skipping the isRootDirUnderBackupRoot check.
// It must be false for plain restores where SnapshotPath comes directly from the manifest
// and must be validated against the backup root to prevent arbitrary file reads.
func restoreEntry(entry ManifestEntry, trustedSnapshot bool, roots []string) error {
	if !filepath.IsAbs(entry.OriginalPath) || !isPathUnderRoots(entry.OriginalPath, roots) {
		return invalidOriginalPathErr(entry.OriginalPath, roots)
	}

	// Validate SnapshotPath is under the backup root to prevent reading arbitrary
	// files from the filesystem via a tampered manifest (e.g. SnapshotPath: "/etc/shadow").
	// Skip this check for trusted snapshots (compressed restores) where SnapshotPath
	// has already been resolved to a safe temp directory by restoreCompressed.
	if !trustedSnapshot {
		ok, err := isRootDirUnderBackupRoot(entry.SnapshotPath)
		if err != nil || !ok {
			return fmt.Errorf("manifest entry has invalid SnapshotPath %q: must be under the backup root directory", entry.SnapshotPath)
		}
	}

	content, err := os.ReadFile(entry.SnapshotPath)
	if err != nil {
		return fmt.Errorf("read snapshot file %q: %w", entry.SnapshotPath, err)
	}

	if err := os.MkdirAll(filepath.Dir(entry.OriginalPath), 0o755); err != nil {
		return fmt.Errorf("create restore directory for %q: %w", entry.OriginalPath, err)
	}

	if _, err := filemerge.WriteFileAtomic(entry.OriginalPath, content, os.FileMode(entry.Mode)); err != nil {
		return fmt.Errorf("restore path %q: %w", entry.OriginalPath, err)
	}

	return nil
}
