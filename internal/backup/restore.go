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

// BackupRootFn resolves the backup root directory for SnapshotPath validation.
// Package-level var for testability.
var BackupRootFn = func() (string, error) {
	home, err := UserHomeDirFn()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".gentle-ai", "backups"), nil
}

func isPathWithinBase(path string, base string) bool {
	cleanPath := filepath.Clean(path)
	cleanBase := filepath.Clean(base)

	if cleanPath == cleanBase {
		return true
	}

	return strings.HasPrefix(cleanPath, cleanBase+string(filepath.Separator))
}

func nearestExistingAncestor(path string) string {
	current := filepath.Clean(path)
	for {
		if _, err := os.Lstat(current); err == nil {
			return current
		}

		parent := filepath.Dir(current)
		if parent == current {
			return current
		}
		current = parent
	}
}

// isPathUnderHome reports whether path is an absolute path that resides under
// the current user's home directory. This is used to prevent arbitrary file
// writes via tampered manifest OriginalPath fields.
//
// Symlink note: this check resolves existing ancestors (not just the final
// file) to prevent escapes through parent-directory symlinks when the leaf
// file does not exist yet.
func isPathUnderHome(path string) bool {
	home, err := UserHomeDirFn()
	if err != nil {
		return false
	}
	clean := filepath.Clean(path)
	homeClean := filepath.Clean(home)
	if !filepath.IsAbs(clean) || !isPathWithinBase(clean, homeClean) {
		return false
	}

	resolvedHome, err := filepath.EvalSymlinks(homeClean)
	if err != nil {
		resolvedHome = homeClean
	}

	// Resolve the nearest existing ancestor of the destination's parent directory
	// so symlink escapes in intermediate directories are detected before writes.
	parent := filepath.Dir(clean)
	ancestor := nearestExistingAncestor(parent)
	resolvedAncestor, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		return false
	}
	if !isPathWithinBase(resolvedAncestor, resolvedHome) {
		return false
	}

	// If the destination exists, resolve it too for leaf-symlink protection.
	if _, err := os.Lstat(clean); err == nil {
		resolvedPath, err := filepath.EvalSymlinks(clean)
		if err != nil {
			return false
		}
		return isPathWithinBase(resolvedPath, resolvedHome)
	}

	return true
}

// isRootDirUnderBackupRoot reports whether rootDir is an absolute path under
// the configured backup root and does not escape through symlinks.
func isRootDirUnderBackupRoot(rootDir string) (bool, error) {
	return isPathUnderBackupRoot(rootDir)
}

// isPathUnderBackupRoot reports whether path is an absolute path under the
// configured backup root after resolving symlinks on both sides.
func isPathUnderBackupRoot(path string) (bool, error) {
	backupRoot, err := BackupRootFn()
	if err != nil {
		return false, fmt.Errorf("resolve backup root: %w", err)
	}

	cleanRoot := filepath.Clean(path)
	cleanBackupRoot := filepath.Clean(backupRoot)

	if !filepath.IsAbs(cleanRoot) {
		return false, nil
	}

	resolvedBackupRoot, err := filepath.EvalSymlinks(cleanBackupRoot)
	if err != nil {
		resolvedBackupRoot = cleanBackupRoot
	}

	resolvedRoot, err := filepath.EvalSymlinks(cleanRoot)
	if err != nil {
		return false, nil
	}

	return isPathWithinBase(resolvedRoot, resolvedBackupRoot), nil
}

func cleanupCreatedPath(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if info.IsDir() {
		return os.RemoveAll(path)
	}

	err = os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
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

	ok, err := isRootDirUnderBackupRoot(manifest.RootDir)
	if err != nil {
		return fmt.Errorf("validate compressed manifest RootDir %q: %w", manifest.RootDir, err)
	}
	if !ok {
		return fmt.Errorf("compressed manifest has invalid RootDir %q: must be under the backup root directory", manifest.RootDir)
	}

	archivePath := filepath.Join(manifest.RootDir, ArchiveFilename)
	ok, err = isPathUnderBackupRoot(archivePath)
	if err != nil {
		return fmt.Errorf("validate archive path %q: %w", archivePath, err)
	}
	if !ok {
		return fmt.Errorf("compressed manifest has invalid archive path %q: must be under the backup root directory", archivePath)
	}

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
			resolvedSnapshot := filepath.Clean(filepath.Join(tempDir, filepath.FromSlash(entry.SnapshotPath)))
			if !isPathWithinBase(resolvedSnapshot, tempDir) {
				return fmt.Errorf("manifest entry %q has SnapshotPath %q escaping restore extraction directory", entry.OriginalPath, entry.SnapshotPath)
			}

			resolvedEntry := ManifestEntry{
				OriginalPath: entry.OriginalPath,
				SnapshotPath: resolvedSnapshot,
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
		if err := cleanupCreatedPath(entry.OriginalPath); err != nil {
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
		if err := cleanupCreatedPath(entry.OriginalPath); err != nil {
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
		ok, err := isPathUnderBackupRoot(entry.SnapshotPath)
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
