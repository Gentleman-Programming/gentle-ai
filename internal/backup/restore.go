package backup

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/components/filemerge"
)

// UserHomeDirFn is the function used to resolve the user's home directory.
// Package-level var for testability — swapped in tests to use a temp directory.
var UserHomeDirFn = os.UserHomeDir

// isPathAllowedForRestore reports whether path is an absolute path that resides
// under a restore-safe root. Production config paths should normally live under
// the user's home directory; tests and rollback flows may operate under the
// process temp directory, so temp roots are also accepted.
//
// This still prevents arbitrary writes via tampered manifest OriginalPath
// fields such as system roots or unrelated user directories.
//
// Symlink note: if the path already exists on disk, EvalSymlinks is used to
// resolve the real path and re-check against the allowed root, preventing
// symlink escapes.
// If the path does not exist yet (typical during restore), only filepath.Clean
// is used — symlinks cannot be resolved for non-existent paths, so this
// limitation is accepted and documented here.
func isPathAllowedForRestore(path string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	clean := filepath.Clean(path)

	home, err := UserHomeDirFn()
	if err == nil && isPathUnderRestoreRoot(clean, filepath.Clean(home)) {
		return true
	}

	for _, root := range restoreTempRoots() {
		if isPathUnderRestoreRoot(clean, filepath.Clean(root)) {
			return true
		}
	}

	return false
}

func restoreTempRoots() []string {
	candidates := []string{os.TempDir()}
	for _, key := range []string{"GOTMPDIR", "TMP", "TEMP"} {
		if value := os.Getenv(key); value != "" {
			candidates = append(candidates, value)
		}
	}
	return candidates
}

func isPathUnderRestoreRoot(clean, rootClean string) bool {
	if rootClean == "" || clean == rootClean {
		return false
	}
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

		if !isPathAllowedForRestore(entry.OriginalPath) {
			return fmt.Errorf("manifest entry has invalid OriginalPath %q: must be an absolute path under the user home or temp directory", entry.OriginalPath)
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

		if !isPathAllowedForRestore(entry.OriginalPath) {
			return fmt.Errorf("manifest entry has invalid OriginalPath %q: must be an absolute path under the user home or temp directory", entry.OriginalPath)
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
	if !isPathAllowedForRestore(entry.OriginalPath) {
		return fmt.Errorf("manifest entry has invalid OriginalPath %q: must be an absolute path under the user home or temp directory", entry.OriginalPath)
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

	// Best-effort validation using in-memory content to avoid a second disk read.
	// A warning is logged for corrupted files but the restore is not aborted —
	// a warning is better than leaving the system in an inconsistent state.
	if warn := ValidateRestoredContent(entry.OriginalPath, content); warn != "" {
		log.Printf("backup: restore validation: %s", warn)
	}

	return nil
}
