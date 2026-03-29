package cli

// backup_metadata_test.go — verifies that the real backup creation paths
// (install, sync, upgrade) emit Source, Description, and CreatedByVersion
// metadata into the manifest file on disk.
//
// Verify gap: prior tests only validated JSON round-trip for manifest fields in
// the backup package (unit tests) but no test confirmed that prepareBackupStep
// actually writes those fields when running from the install or sync paths.

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/backup"
)

// TestPrepareBackupStep_InstallWritesMetadataToManifest verifies that when
// prepareBackupStep runs with source=install and an appVersion, the manifest
// file on disk contains the Source, Description, and CreatedByVersion fields.
//
// This is a TDD-first runtime evidence test: it exercises the actual
// prepareBackupStep.Run() path using real Snapshotter and real filesystem I/O.
func TestPrepareBackupStep_InstallWritesMetadataToManifest(t *testing.T) {
	home := t.TempDir()

	// Create a real config file to snapshot so FileCount > 0.
	configPath := filepath.Join(home, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"test": true}`), 0o644); err != nil {
		t.Fatalf("WriteFile config: %v", err)
	}

	snapshotDir := filepath.Join(home, ".gentle-ai", "backups",
		time.Now().UTC().Format("20060102150405.000000000"))
	state := &runtimeState{}

	step := prepareBackupStep{
		id:          "prepare:backup-snapshot",
		snapshotter: backup.NewSnapshotter(),
		snapshotDir: snapshotDir,
		targets:     []string{configPath},
		state:       state,
		source:      backup.BackupSourceInstall,
		description: "pre-install snapshot",
		appVersion:  "1.2.3",
	}

	if err := step.Run(); err != nil {
		t.Fatalf("prepareBackupStep.Run() error = %v", err)
	}

	// Read the manifest file that was written to disk.
	manifestPath := filepath.Join(snapshotDir, backup.ManifestFilename)
	manifest, err := backup.ReadManifest(manifestPath)
	if err != nil {
		t.Fatalf("ReadManifest() error = %v", err)
	}

	// Verify metadata fields are present in the on-disk manifest.
	if manifest.Source != backup.BackupSourceInstall {
		t.Errorf("manifest.Source = %q, want %q", manifest.Source, backup.BackupSourceInstall)
	}
	if manifest.Description != "pre-install snapshot" {
		t.Errorf("manifest.Description = %q, want %q", manifest.Description, "pre-install snapshot")
	}
	if manifest.CreatedByVersion != "1.2.3" {
		t.Errorf("manifest.CreatedByVersion = %q, want %q", manifest.CreatedByVersion, "1.2.3")
	}
	// FileCount must be 1 (only configPath existed).
	if manifest.FileCount != 1 {
		t.Errorf("manifest.FileCount = %d, want 1", manifest.FileCount)
	}

	// State manifest must also carry the metadata (used for rollback).
	if state.manifest.Source != backup.BackupSourceInstall {
		t.Errorf("state.manifest.Source = %q, want install", state.manifest.Source)
	}
}

// TestPrepareBackupStep_SyncWritesMetadataToManifest verifies the sync path
// emits Source=sync and CreatedByVersion into the manifest on disk.
func TestPrepareBackupStep_SyncWritesMetadataToManifest(t *testing.T) {
	home := t.TempDir()

	configPath := filepath.Join(home, "opencode.json")
	if err := os.WriteFile(configPath, []byte(`{"model": "claude"}`), 0o644); err != nil {
		t.Fatalf("WriteFile config: %v", err)
	}

	snapshotDir := filepath.Join(home, ".gentle-ai", "backups",
		time.Now().UTC().Format("20060102150405.000000001"))
	state := &runtimeState{}

	step := prepareBackupStep{
		id:          "prepare:backup-snapshot",
		snapshotter: backup.NewSnapshotter(),
		snapshotDir: snapshotDir,
		targets:     []string{configPath},
		state:       state,
		source:      backup.BackupSourceSync,
		description: "pre-sync snapshot",
		appVersion:  "2.0.0",
	}

	if err := step.Run(); err != nil {
		t.Fatalf("prepareBackupStep.Run() for sync error = %v", err)
	}

	manifestPath := filepath.Join(snapshotDir, backup.ManifestFilename)
	manifest, err := backup.ReadManifest(manifestPath)
	if err != nil {
		t.Fatalf("ReadManifest() error = %v", err)
	}

	if manifest.Source != backup.BackupSourceSync {
		t.Errorf("manifest.Source = %q, want %q", manifest.Source, backup.BackupSourceSync)
	}
	if manifest.CreatedByVersion != "2.0.0" {
		t.Errorf("manifest.CreatedByVersion = %q, want 2.0.0", manifest.CreatedByVersion)
	}
	if manifest.Description != "pre-sync snapshot" {
		t.Errorf("manifest.Description = %q, want %q", manifest.Description, "pre-sync snapshot")
	}
}

// TestPrepareBackupStep_NoMetadataWhenSourceEmpty verifies backward-compatible
// behavior: when source and appVersion are empty, the manifest fields are
// omitted (zero-values), keeping old-manifest compatibility intact.
func TestPrepareBackupStep_NoMetadataWhenSourceEmpty(t *testing.T) {
	home := t.TempDir()

	configPath := filepath.Join(home, "cursor.json")
	if err := os.WriteFile(configPath, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("WriteFile config: %v", err)
	}

	snapshotDir := filepath.Join(home, ".gentle-ai", "backups",
		time.Now().UTC().Format("20060102150405.000000002"))
	state := &runtimeState{}

	step := prepareBackupStep{
		id:          "prepare:backup-snapshot",
		snapshotter: backup.NewSnapshotter(),
		snapshotDir: snapshotDir,
		targets:     []string{configPath},
		state:       state,
		// source, description, and appVersion intentionally left empty.
	}

	if err := step.Run(); err != nil {
		t.Fatalf("prepareBackupStep.Run() without metadata error = %v", err)
	}

	manifestPath := filepath.Join(snapshotDir, backup.ManifestFilename)
	manifest, err := backup.ReadManifest(manifestPath)
	if err != nil {
		t.Fatalf("ReadManifest() error = %v", err)
	}

	// When no source or version is provided, fields stay empty.
	if manifest.Source != "" {
		t.Errorf("manifest.Source = %q, want empty when not provided", manifest.Source)
	}
	if manifest.CreatedByVersion != "" {
		t.Errorf("manifest.CreatedByVersion = %q, want empty when not provided", manifest.CreatedByVersion)
	}
}
