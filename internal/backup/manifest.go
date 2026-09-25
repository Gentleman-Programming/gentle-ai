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

// BackupOrigin identifies the product-owned root from which a backup was
// discovered. It is runtime metadata, not part of the persisted snapshot
// contract: legacy manifests do not contain a product-origin field.
type BackupOrigin string

const (
	BackupOriginUnknown  BackupOrigin = ""
	BackupOriginAxiom    BackupOrigin = "axiom"
	BackupOriginGentleAI BackupOrigin = "gentle-ai"
)

// Label returns the human-readable product origin for a discovered backup.
func (o BackupOrigin) Label() string {
	switch o {
	case BackupOriginAxiom:
		return "Axiom"
	case BackupOriginGentleAI:
		return "Gentle AI histórico"
	default:
		return "origen no identificado"
	}
}

const (
	// BackupSourceInstall indicates the backup was created before an install run.
	BackupSourceInstall BackupSource = "install"
	// BackupSourceSync indicates the backup was created before a sync run.
	BackupSourceSync BackupSource = "sync"
	// BackupSourceUpgrade indicates the backup was created before an upgrade run.
	BackupSourceUpgrade BackupSource = "upgrade"
	// BackupSourceUninstall indicates the backup was created before an uninstall run.
	BackupSourceUninstall BackupSource = "uninstall"
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
	case BackupSourceUninstall:
		return "uninstall"
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

	// Origin is assigned by the backup-listing boundary from the root where
	// this manifest was discovered. It is intentionally transient so listing
	// backups never migrates or rewrites a user's existing manifest.
	Origin BackupOrigin `json:"-"`
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

// PathKind classifies a backed-up path for restore. The empty value is the
// legacy "unknown" state — persisted manifests written before this field
// existed carry it implicitly, and the restore path applies a safe
// compatibility policy (see RestoreService for details).
type PathKind string

const (
	// PathKindUnknown is the legacy default. Restore treats unknown
	// Existed==false entries as preserve-only (do not delete) so we
	// never destroy a path whose original type we cannot prove.
	PathKindUnknown PathKind = ""

	// PathKindRegularFile marks a regular file. Snapshotted into the
	// archive and restored by reading the archive entry.
	PathKindRegularFile PathKind = "regular"

	// PathKindDirectory marks an empty directory. Not archived.
	// Restore ensures the directory still exists; never deletes a
	// pre-existing directory even if the install/sync that produced
	// this snapshot created it.
	PathKindDirectory PathKind = "directory"

	// PathKindSymlinkDirectory marks a relative symlink to a
	// directory. Not archived. Restore validates LinkTarget is a safe
	// relative path and recreates the symlink if it is missing.
	PathKindSymlinkDirectory PathKind = "symlink_directory"
)

type ManifestEntry struct {
	OriginalPath string   `json:"original_path"`
	SnapshotPath string   `json:"snapshot_path"`
	Existed      bool     `json:"existed"`
	Mode         uint32   `json:"mode,omitempty"`
	Kind         PathKind `json:"kind,omitempty"`

	// LinkTarget is the recorded symlink target. Only set when Kind
	// is PathKindSymlinkDirectory. Restore validates that it is a safe
	// relative path (no leading '/', no '..' segments that escape).
	LinkTarget string `json:"link_target,omitempty"`
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

// BackupRootFor returns the canonical backup root directory
// (<home>/.axiom/backups) for the given home directory. It is the single
// owning accessor for the canonical backup root literal: production code
// outside this package MUST resolve the backup root through this function
// (or BackupRootFn) instead of constructing the path with its own
// filepath.Join literal.
func BackupRootFor(home string) string {
	return filepath.Join(home, ".axiom", "backups")
}

// backupRoot returns the expected parent directory for all backups (~/.axiom/backups).
func backupRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return BackupRootFor(home), nil
}

// LegacyBackupRootFor returns the legacy backup root directory
// (<home>/.gentle-ai/backups) for the given home directory. It is the single
// owning accessor for the legacy backup root literal, kept for the
// permanent read-side compatibility fallback [D-03, D-12]: it is never used
// to create new backups, only to keep resolving ones created before the
// canonical root existed.
func LegacyBackupRootFor(home string) string {
	return filepath.Join(home, ".gentle-ai", "backups")
}

// BackupRoots returns every backup root that a reader must scan for the
// given home directory, canonical root first: [BackupRootFor(home),
// LegacyBackupRootFor(home)]. Order is part of the contract — callers can
// identify canonical and historical backups consistently, but must not treat
// manifest IDs as globally unique across these roots.
func BackupRoots(home string) []string {
	return []string{BackupRootFor(home), LegacyBackupRootFor(home)}
}

// BackupOriginForRoot identifies one of the supported backup roots. Unknown
// roots remain unknown rather than being guessed from manifest metadata.
func BackupOriginForRoot(home, root string) BackupOrigin {
	cleanRoot := filepath.Clean(root)
	switch cleanRoot {
	case filepath.Clean(BackupRootFor(home)):
		return BackupOriginAxiom
	case filepath.Clean(LegacyBackupRootFor(home)):
		return BackupOriginGentleAI
	default:
		return BackupOriginUnknown
	}
}

// legacyBackupRoot returns the legacy parent directory for Gentle AI backups (~/.gentle-ai/backups).
func legacyBackupRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return LegacyBackupRootFor(home), nil
}

// BackupRootFn is the function used to resolve the backup root directory.
// Package-level var for testability — swapped by this package's own tests to
// point at a temp directory (manifest_test.go, restore_test.go,
// retention_test.go, snapshot_dir_fsync_test.go). No other package overrides
// it: production code elsewhere resolves the backup root through
// BackupRootFor/LegacyBackupRootFor/BackupRoots instead.
var BackupRootFn = backupRoot

func isDirUnderRoot(dir, root string) bool {
	clean := filepath.Clean(dir)
	rootClean := filepath.Clean(root)
	if !strings.HasPrefix(clean, rootClean+string(filepath.Separator)) {
		return false
	}
	if resolved, err := filepath.EvalSymlinks(clean); err == nil {
		resolvedRoot, err := filepath.EvalSymlinks(rootClean)
		if err != nil {
			resolvedRoot = rootClean
		}
		return strings.HasPrefix(resolved, resolvedRoot+string(filepath.Separator))
	}
	return true
}

// isRootDirUnderBackupRoot validates that dir is a direct or indirect subdirectory
// of the expected backup root (~/.axiom/backups/) or the legacy root (~/.gentle-ai/backups/).
// This prevents a tampered manifest with root_dir set to "/" or another sensitive path
// from deleting arbitrary files.
func isRootDirUnderBackupRoot(dir string) (bool, error) {
	root, err := BackupRootFn()
	if err != nil {
		return false, err
	}
	if isDirUnderRoot(dir, root) {
		return true, nil
	}
	if legacy, err := legacyBackupRoot(); err == nil && isDirUnderRoot(dir, legacy) {
		return true, nil
	}
	return false, nil
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
