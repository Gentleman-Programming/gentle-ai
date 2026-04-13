package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/components/filemerge"
)

// UserHomeDirFn is the function used to resolve the user's home directory.
// Package-level var for testability — swapped in tests to use a temp directory.
var UserHomeDirFn = os.UserHomeDir

// isPathUnderHome reports whether path is an absolute path that resides under
// the current user's home directory. This is used to prevent arbitrary file
// writes via tampered manifest OriginalPath fields.
//
// Symlink note: if the path already exists on disk, EvalSymlinks is used to
// resolve the real path and re-check against home, preventing symlink escapes.
// If the path does not exist yet (typical during restore), only filepath.Clean
// is used — symlinks cannot be resolved for non-existent paths, so this
// limitation is accepted and documented here.
func isPathUnderHome(path string) bool {
	home, err := UserHomeDirFn()
	if err != nil {
		return false
	}
	clean := filepath.Clean(path)
	homeClean := filepath.Clean(home)
	if !strings.HasPrefix(clean, homeClean+string(filepath.Separator)) {
		return false
	}
	// If the path exists, resolve symlinks and re-check to prevent symlink escapes.
	if resolved, err := filepath.EvalSymlinks(clean); err == nil {
		resolvedHome, err := filepath.EvalSymlinks(homeClean)
		if err != nil {
			resolvedHome = homeClean
		}
		return strings.HasPrefix(resolved, resolvedHome+string(filepath.Separator))
	}
	// Path does not exist yet (file will be created by restore) — accept Clean-only check.
	return true
}

type RestoreService struct{}

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
			if err := restoreEntry(resolvedEntry, true); err != nil {
				return err
			}
			continue
		}

		if !filepath.IsAbs(entry.OriginalPath) || !isPathUnderHome(entry.OriginalPath) {
			return fmt.Errorf("manifest entry has invalid OriginalPath %q: must be an absolute path under the user home directory", entry.OriginalPath)
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
	for _, entry := range manifest.Entries {
		if entry.Existed {
			if err := restoreEntry(entry, false); err != nil {
				return err
			}
			continue
		}

		if !filepath.IsAbs(entry.OriginalPath) || !isPathUnderHome(entry.OriginalPath) {
			return fmt.Errorf("manifest entry has invalid OriginalPath %q: must be an absolute path under the user home directory", entry.OriginalPath)
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
func restoreEntry(entry ManifestEntry, trustedSnapshot bool) error {
	if !filepath.IsAbs(entry.OriginalPath) || !isPathUnderHome(entry.OriginalPath) {
		return fmt.Errorf("manifest entry has invalid OriginalPath %q: must be an absolute path under the user home directory", entry.OriginalPath)
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
