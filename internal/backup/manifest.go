package backup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// BackupSource identifies what operation created a backup.
// New values may be added in future — consumers must handle unknown values gracefully.
type BackupSource string

const (
	// BackupSourceInstall indicates the backup was created before an install run.
	BackupSourceInstall BackupSource = "install"
	// BackupSourceSync indicates the backup was created before a sync run.
	BackupSourceSync BackupSource = "sync"
	// BackupSourceUpgrade indicates the backup was created before an upgrade run.
	BackupSourceUpgrade BackupSource = "upgrade"
)

// Label returns a human-readable string for the BackupSource.
// Unknown or empty sources return "unknown source" so old manifests display gracefully.
func (s BackupSource) Label() string {
	switch s {
	case BackupSourceInstall:
		return "install"
	case BackupSourceSync:
		return "sync"
	case BackupSourceUpgrade:
		return "upgrade"
	default:
		return "unknown source"
	}
}

type Manifest struct {
	ID        string          `json:"id"`
	CreatedAt time.Time       `json:"created_at"`
	RootDir   string          `json:"root_dir"`
	Entries   []ManifestEntry `json:"entries"`

	// Source identifies what operation created this backup.
	// Optional: omitted for backward-compatibility with old manifests.
	Source BackupSource `json:"source,omitempty"`

	// Description is a short human-readable note about the backup context.
	// Optional: omitted for backward-compatibility with old manifests.
	Description string `json:"description,omitempty"`

	// FileCount is the number of files that existed and were actually snapshotted.
	// Entries where Existed==false (files that did not exist at snapshot time) are
	// not counted. Optional: omitted when zero for backward-compatibility.
	FileCount int `json:"file_count,omitempty"`

	// CreatedByVersion is the gentle-ai version that created this backup.
	// Optional: omitted when empty for backward-compatibility with old manifests.
	CreatedByVersion string `json:"created_by_version,omitempty"`

	// Pinned marks the backup as protected from retention pruning.
	// Optional: omitted when false for backward-compatibility with old manifests.
	Pinned bool `json:"pinned,omitempty"`

	// Compressed indicates the backup files are stored as a tar.gz archive.
	// Optional: omitted when false for backward-compatibility with old manifests.
	Compressed bool `json:"compressed,omitempty"`

	// Checksum is the SHA-256 composite hash of the snapshotted files, used for deduplication.
	// Optional: omitted when empty for backward-compatibility with old manifests.
	Checksum string `json:"checksum,omitempty"`
}

// DisplayLabel returns a human-readable label for the backup suitable for display
// in the CLI restore list and TUI backup screen. It combines the source label and
// the formatted creation timestamp, and appends the file count when known.
//
// Old manifests without Source will show "unknown source" as a graceful fallback.
// Old manifests without FileCount will not show any file count.
func (m Manifest) DisplayLabel() string {
	base := fmt.Sprintf("%s — %s", m.Source.Label(), m.CreatedAt.Local().Format("2006-01-02 15:04"))
	if m.FileCount > 0 {
		base = fmt.Sprintf("%s (%d files)", base, m.FileCount)
	}
	if m.Pinned {
		return "[pinned] " + base
	}
	return base
}

type ManifestEntry struct {
	OriginalPath string `json:"original_path"`
	SnapshotPath string `json:"snapshot_path"`
	Existed      bool   `json:"existed"`
	Mode         uint32 `json:"mode,omitempty"`
}

func WriteManifest(path string, manifest Manifest) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create manifest directory %q: %w", path, err)
	}

	content, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	content = append(content, '\n')
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("write manifest %q: %w", path, err)
	}

	return nil
}

func ReadManifest(path string) (Manifest, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest %q: %w", path, err)
	}

	var manifest Manifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("unmarshal manifest %q: %w", path, err)
	}

	return manifest, nil
}

// backupRoot returns the expected parent directory for all backups.
func backupRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".gentle-ai", "backups"), nil
}

// BackupRootFn is the function used to resolve the backup root directory.
// Package-level var for testability — swapped in tests to use a temp directory.
// Exported so tests in other packages (e.g. internal/update/upgrade) can override it.
var BackupRootFn = backupRoot

// isRootDirUnderBackupRoot validates that dir is a direct or indirect subdirectory
// of the expected backup root (~/.gentle-ai/backups/). This prevents a tampered
// manifest with root_dir set to "/" or another sensitive path from deleting arbitrary files.
//
// Symlink note: if the path already exists on disk, EvalSymlinks is used to
// resolve the real path and re-check against the backup root, preventing symlink escapes.
// If the path does not exist yet, only filepath.Clean is used — this limitation is accepted
// and documented here, consistent with isPathUnderHome.
func isRootDirUnderBackupRoot(dir string) (bool, error) {
	root, err := BackupRootFn()
	if err != nil {
		return false, err
	}
	clean := filepath.Clean(dir)
	rootClean := filepath.Clean(root)
	if !strings.HasPrefix(clean, rootClean+string(filepath.Separator)) {
		return false, nil
	}
	// If the path exists, resolve symlinks and re-check to prevent symlink escapes.
	if resolved, err := filepath.EvalSymlinks(clean); err == nil {
		resolvedRoot, err := filepath.EvalSymlinks(rootClean)
		if err != nil {
			resolvedRoot = rootClean
		}
		return strings.HasPrefix(resolved, resolvedRoot+string(filepath.Separator)), nil
	}
	// Path does not exist yet — accept Clean-only check.
	return true, nil
}

// DeleteBackup removes the entire backup directory.
func DeleteBackup(manifest Manifest) error {
	if manifest.RootDir == "" {
		return fmt.Errorf("backup has no root directory")
	}
	ok, err := isRootDirUnderBackupRoot(manifest.RootDir)
	if err != nil {
		return fmt.Errorf("validate backup root dir: %w", err)
	}
	if !ok {
		return fmt.Errorf("backup RootDir %q is outside the expected backup directory — refusing to delete", manifest.RootDir)
	}
	return os.RemoveAll(manifest.RootDir)
}

// RenameBackup updates the backup's Description field in the manifest file.
// This does not rename the directory — it updates the human-readable description.
func RenameBackup(manifest Manifest, newDescription string) error {
	if manifest.RootDir == "" {
		return fmt.Errorf("backup has no root directory")
	}
	manifest.Description = newDescription
	manifestPath := filepath.Join(manifest.RootDir, ManifestFilename)
	return WriteManifest(manifestPath, manifest)
}

// TogglePin flips the Pinned field of the manifest and rewrites the manifest.json
// file inside the backup's RootDir. Pinned backups are excluded from retention
// pruning. Returns an error if RootDir is empty or the write fails.
func TogglePin(manifest Manifest) error {
	if manifest.RootDir == "" {
		return fmt.Errorf("backup has no root directory")
	}
	manifest.Pinned = !manifest.Pinned
	manifestPath := filepath.Join(manifest.RootDir, ManifestFilename)
	return WriteManifest(manifestPath, manifest)
}
