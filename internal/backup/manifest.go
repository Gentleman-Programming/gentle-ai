package backup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
		return fmt.Sprintf("%s (%d files)", base, m.FileCount)
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

// DeleteBackup removes the entire backup directory.
func DeleteBackup(manifest Manifest) error {
	if manifest.RootDir == "" {
		return fmt.Errorf("backup has no root directory")
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
