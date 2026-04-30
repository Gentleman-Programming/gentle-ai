package backup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestManifestSourceLabel verifies that BackupSourceLabel returns the correct
// human-readable string for each BackupSource value, including the unknown
// fallback for old manifests without source metadata.
func TestManifestSourceLabel(t *testing.T) {
	tests := []struct {
		source BackupSource
		want   string
	}{
		{BackupSourceInstall, "install"},
		{BackupSourceSync, "sync"},
		{BackupSourceUpgrade, "upgrade"},
		{BackupSourceUninstall, "uninstall"},
		{BackupSource(""), "unknown source"},
		{BackupSource("other"), "unknown source"},
	}

	for _, tt := range tests {
		t.Run(string(tt.source), func(t *testing.T) {
			got := tt.source.Label()
			if got != tt.want {
				t.Errorf("Label() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestManifestDisplayLabel verifies that DisplayLabel returns a human-readable
// label combining the source and timestamp, and falls back gracefully for
// manifests without source metadata (backward-compatible old manifests).
func TestManifestDisplayLabel(t *testing.T) {
	ts := time.Date(2026, 3, 22, 15, 4, 5, 0, time.UTC)

	tests := []struct {
		name     string
		manifest Manifest
		contains string
	}{
		{
			name: "install source shows install label",
			manifest: Manifest{
				ID:        "20260322150405.000000000",
				CreatedAt: ts,
				Source:    BackupSourceInstall,
			},
			contains: "install",
		},
		{
			name: "sync source shows sync label",
			manifest: Manifest{
				ID:        "20260322150405.000000000",
				CreatedAt: ts,
				Source:    BackupSourceSync,
			},
			contains: "sync",
		},
		{
			name: "upgrade source shows upgrade label",
			manifest: Manifest{
				ID:        "20260322150405.000000000",
				CreatedAt: ts,
				Source:    BackupSourceUpgrade,
			},
			contains: "upgrade",
		},
		{
			name: "uninstall source shows uninstall label",
			manifest: Manifest{
				ID:        "20260322150405.000000000",
				CreatedAt: ts,
				Source:    BackupSourceUninstall,
			},
			contains: "uninstall",
		},
		{
			name: "no source falls back to unknown",
			manifest: Manifest{
				ID:        "20260322150405.000000000",
				CreatedAt: ts,
			},
			contains: "unknown source",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.manifest.DisplayLabel()
			if !strings.Contains(got, tt.contains) {
				t.Errorf("DisplayLabel() = %q, want string containing %q", got, tt.contains)
			}
		})
	}
}

// TestManifestSourceSerializationRoundTrip verifies that BackupSource and
// Description fields serialize to and deserialize from JSON correctly.
func TestManifestSourceSerializationRoundTrip(t *testing.T) {
	original := Manifest{
		ID:          "test-id",
		CreatedAt:   time.Date(2026, 3, 22, 15, 4, 5, 0, time.UTC),
		RootDir:     "/tmp/test",
		Source:      BackupSourceInstall,
		Description: "pre-install snapshot",
		Entries:     []ManifestEntry{},
	}

	data, err := json.MarshalIndent(original, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}

	var decoded Manifest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if decoded.Source != original.Source {
		t.Errorf("Source = %q, want %q", decoded.Source, original.Source)
	}
	if decoded.Description != original.Description {
		t.Errorf("Description = %q, want %q", decoded.Description, original.Description)
	}
}

// TestOldManifestRemainsReadable verifies backward-compatibility: a manifest
// JSON without the new metadata fields is still read correctly, with zero-value
// (empty) Source and Description — which DisplayLabel handles gracefully.
func TestOldManifestRemainsReadable(t *testing.T) {
	oldJSON := `{
  "id": "20260322150405.000000000",
  "created_at": "2026-03-22T15:04:05Z",
  "root_dir": "/home/user/.gentle-ai/backups/20260322150405.000000000",
  "entries": []
}`

	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(path, []byte(oldJSON), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	manifest, err := ReadManifest(path)
	if err != nil {
		t.Fatalf("ReadManifest() error = %v", err)
	}

	// New fields must be zero-valued when reading old manifests.
	if manifest.Source != "" {
		t.Errorf("Source = %q, want empty string for old manifest", manifest.Source)
	}
	if manifest.Description != "" {
		t.Errorf("Description = %q, want empty string for old manifest", manifest.Description)
	}

	// Fallback label must work without panicking.
	label := manifest.DisplayLabel()
	if !strings.Contains(label, "unknown source") {
		t.Errorf("DisplayLabel() = %q, want string containing 'unknown source'", label)
	}
}

// TestNewManifestOmitsEmptySourceFromJSON verifies that omitempty is respected:
// when Source is not set, it should not appear in the serialized JSON, keeping
// existing manifest files readable by older versions of gentle-ai.
func TestNewManifestOmitsEmptySourceFromJSON(t *testing.T) {
	m := Manifest{
		ID:        "test",
		CreatedAt: time.Now().UTC(),
		RootDir:   "/tmp",
		Entries:   []ManifestEntry{},
		// Source and Description intentionally omitted
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}

	jsonStr := string(data)
	if strings.Contains(jsonStr, `"source"`) {
		t.Errorf("JSON contains 'source' field but should omit it when empty: %s", jsonStr)
	}
	if strings.Contains(jsonStr, `"description"`) {
		t.Errorf("JSON contains 'description' field but should omit it when empty: %s", jsonStr)
	}
}

// TestManifestFileCountField verifies that FileCount is serialized correctly,
// omitted when zero (backward-compat), and reads back to the same value.
func TestManifestFileCountField(t *testing.T) {
	t.Run("non-zero FileCount round-trips via JSON", func(t *testing.T) {
		original := Manifest{
			ID:        "test-fc",
			CreatedAt: time.Date(2026, 3, 22, 15, 4, 5, 0, time.UTC),
			RootDir:   "/tmp/test",
			FileCount: 3,
			Entries:   []ManifestEntry{},
		}
		data, err := json.MarshalIndent(original, "", "  ")
		if err != nil {
			t.Fatalf("MarshalIndent() error = %v", err)
		}
		var decoded Manifest
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		if decoded.FileCount != 3 {
			t.Errorf("FileCount = %d, want 3", decoded.FileCount)
		}
	})

	t.Run("zero FileCount is omitted from JSON", func(t *testing.T) {
		m := Manifest{
			ID:        "test-fc-zero",
			CreatedAt: time.Now().UTC(),
			RootDir:   "/tmp",
			Entries:   []ManifestEntry{},
			// FileCount intentionally zero
		}
		data, err := json.MarshalIndent(m, "", "  ")
		if err != nil {
			t.Fatalf("MarshalIndent() error = %v", err)
		}
		if strings.Contains(string(data), `"file_count"`) {
			t.Errorf("JSON contains 'file_count' but should omit it when zero: %s", string(data))
		}
	})

	t.Run("old manifest without file_count reads as zero", func(t *testing.T) {
		oldJSON := `{
  "id": "old-no-fc",
  "created_at": "2026-03-22T15:04:05Z",
  "root_dir": "/home/user/.gentle-ai/backups/old",
  "entries": []
}`
		dir := t.TempDir()
		path := filepath.Join(dir, "manifest.json")
		if err := os.WriteFile(path, []byte(oldJSON), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		manifest, err := ReadManifest(path)
		if err != nil {
			t.Fatalf("ReadManifest() error = %v", err)
		}
		if manifest.FileCount != 0 {
			t.Errorf("FileCount = %d, want 0 for old manifest", manifest.FileCount)
		}
	})
}

// TestManifestCreatedByVersionField verifies that CreatedByVersion is serialized
// correctly, omitted when empty (backward-compat), and reads back to the same value.
func TestManifestCreatedByVersionField(t *testing.T) {
	t.Run("non-empty CreatedByVersion round-trips via JSON", func(t *testing.T) {
		original := Manifest{
			ID:               "test-ver",
			CreatedAt:        time.Date(2026, 3, 22, 15, 4, 5, 0, time.UTC),
			RootDir:          "/tmp/test",
			CreatedByVersion: "1.2.3",
			Entries:          []ManifestEntry{},
		}
		data, err := json.MarshalIndent(original, "", "  ")
		if err != nil {
			t.Fatalf("MarshalIndent() error = %v", err)
		}
		var decoded Manifest
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		if decoded.CreatedByVersion != "1.2.3" {
			t.Errorf("CreatedByVersion = %q, want %q", decoded.CreatedByVersion, "1.2.3")
		}
	})

	t.Run("empty CreatedByVersion is omitted from JSON", func(t *testing.T) {
		m := Manifest{
			ID:        "test-ver-empty",
			CreatedAt: time.Now().UTC(),
			RootDir:   "/tmp",
			Entries:   []ManifestEntry{},
			// CreatedByVersion intentionally empty
		}
		data, err := json.MarshalIndent(m, "", "  ")
		if err != nil {
			t.Fatalf("MarshalIndent() error = %v", err)
		}
		if strings.Contains(string(data), `"created_by_version"`) {
			t.Errorf("JSON contains 'created_by_version' but should omit it when empty: %s", string(data))
		}
	})

	t.Run("old manifest without created_by_version reads as empty string", func(t *testing.T) {
		oldJSON := `{
  "id": "old-no-ver",
  "created_at": "2026-03-22T15:04:05Z",
  "root_dir": "/home/user/.gentle-ai/backups/old",
  "entries": []
}`
		dir := t.TempDir()
		path := filepath.Join(dir, "manifest.json")
		if err := os.WriteFile(path, []byte(oldJSON), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		manifest, err := ReadManifest(path)
		if err != nil {
			t.Fatalf("ReadManifest() error = %v", err)
		}
		if manifest.CreatedByVersion != "" {
			t.Errorf("CreatedByVersion = %q, want empty string for old manifest", manifest.CreatedByVersion)
		}
	})
}

// TestManifestDisplayLabelIncludesFileCount verifies that DisplayLabel includes
// file count when FileCount > 0, and omits it gracefully when zero.
func TestManifestDisplayLabelIncludesFileCount(t *testing.T) {
	ts := time.Date(2026, 3, 22, 15, 4, 5, 0, time.UTC)
	tests := []struct {
		name       string
		manifest   Manifest
		wantCount  string // substring to check
		wantAbsent string // substring that must NOT appear
	}{
		{
			name: "non-zero FileCount shown in label",
			manifest: Manifest{
				ID:        "test-fc",
				CreatedAt: ts,
				Source:    BackupSourceInstall,
				FileCount: 5,
			},
			wantCount: "5",
		},
		{
			name: "zero FileCount not shown in label",
			manifest: Manifest{
				ID:        "test-fc-zero",
				CreatedAt: ts,
				Source:    BackupSourceInstall,
				FileCount: 0,
			},
			wantAbsent: "files",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.manifest.DisplayLabel()
			if tt.wantCount != "" && !strings.Contains(got, tt.wantCount) {
				t.Errorf("DisplayLabel() = %q, want string containing %q", got, tt.wantCount)
			}
			if tt.wantAbsent != "" && strings.Contains(got, tt.wantAbsent) {
				t.Errorf("DisplayLabel() = %q, must NOT contain %q when FileCount=0", got, tt.wantAbsent)
			}
		})
	}
}

// TestSnapshotterPopulatesFileCount verifies that the Snapshotter.Create() method
// automatically populates FileCount with the number of files that actually existed.
func TestSnapshotterPopulatesFileCount(t *testing.T) {
	home := t.TempDir()

	// Create two real files and one path that does NOT exist.
	file1 := filepath.Join(home, "config1.json")
	file2 := filepath.Join(home, "config2.json")
	missing := filepath.Join(home, "missing.json")

	if err := os.WriteFile(file1, []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile file1: %v", err)
	}
	if err := os.WriteFile(file2, []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile file2: %v", err)
	}

	snapshotDir := filepath.Join(home, "snap")
	snap := NewSnapshotter()
	manifest, err := snap.Create(snapshotDir, []string{file1, file2, missing})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Only file1 and file2 existed; missing did not.
	if manifest.FileCount != 2 {
		t.Errorf("FileCount = %d, want 2 (only existing files counted)", manifest.FileCount)
	}
}

// TestSnapshotterSkipsDirectories verifies that snapshot does not fail when a
// path in the list is a directory (e.g., a skills folder). The directory is
// recorded in the manifest as not-existed (skipped), not as an error.
func TestSnapshotterSkipsDirectories(t *testing.T) {
	home := t.TempDir()

	file1 := filepath.Join(home, "config.json")
	if err := os.WriteFile(file1, []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Create a directory that will be in the paths list.
	skillDir := filepath.Join(home, "skills", "my-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	snapshotDir := filepath.Join(home, "snap")
	snap := NewSnapshotter()
	manifest, err := snap.Create(snapshotDir, []string{file1, skillDir})
	if err != nil {
		t.Fatalf("Create() error = %v, want nil (directories should be skipped)", err)
	}

	// Only the file should count, not the directory.
	if manifest.FileCount != 1 {
		t.Errorf("FileCount = %d, want 1 (directory should not count)", manifest.FileCount)
	}
}

// TestDeleteBackup_Success verifies that DeleteBackup removes the backup directory.
func TestDeleteBackup_Success(t *testing.T) {
	dir := t.TempDir()
	// Override backupRootFn so validation accepts paths under t.TempDir().
	origBackupRootFn := BackupRootFn
	t.Cleanup(func() { BackupRootFn = origBackupRootFn })
	BackupRootFn = func() (string, error) { return dir, nil }

	backupDir := filepath.Join(dir, "backup-01")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// Write a dummy manifest file inside the backup dir.
	manifestPath := filepath.Join(backupDir, ManifestFilename)
	m := Manifest{
		ID:      "backup-01",
		RootDir: backupDir,
		Entries: []ManifestEntry{},
	}
	if err := WriteManifest(manifestPath, m); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}

	if err := DeleteBackup(m); err != nil {
		t.Fatalf("DeleteBackup() error = %v", err)
	}

	if _, err := os.Stat(backupDir); !os.IsNotExist(err) {
		t.Errorf("backup directory still exists after DeleteBackup")
	}
}

// TestDeleteBackup_EmptyRootDir verifies that DeleteBackup returns an error
// when the manifest has no RootDir set.
func TestDeleteBackup_EmptyRootDir(t *testing.T) {
	m := Manifest{
		ID:      "no-root",
		RootDir: "",
	}

	err := DeleteBackup(m)
	if err == nil {
		t.Fatalf("DeleteBackup() should return error for empty RootDir")
	}
}

// TestRenameBackup_Success verifies that RenameBackup updates the Description
// and returns no error.
func TestRenameBackup_Success(t *testing.T) {
	dir := t.TempDir()
	backupDir := filepath.Join(dir, "backup-02")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	m := Manifest{
		ID:          "backup-02",
		RootDir:     backupDir,
		Description: "original description",
		Entries:     []ManifestEntry{},
	}
	manifestPath := filepath.Join(backupDir, ManifestFilename)
	if err := WriteManifest(manifestPath, m); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}

	if err := RenameBackup(m, "new description"); err != nil {
		t.Fatalf("RenameBackup() error = %v", err)
	}
}

// TestRenameBackup_UpdatesManifestFile verifies that RenameBackup actually
// persists the new description into the manifest file on disk.
func TestRenameBackup_UpdatesManifestFile(t *testing.T) {
	dir := t.TempDir()
	backupDir := filepath.Join(dir, "backup-03")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	m := Manifest{
		ID:          "backup-03",
		RootDir:     backupDir,
		Description: "before rename",
		Entries:     []ManifestEntry{},
	}
	manifestPath := filepath.Join(backupDir, ManifestFilename)
	if err := WriteManifest(manifestPath, m); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}

	if err := RenameBackup(m, "after rename"); err != nil {
		t.Fatalf("RenameBackup() error = %v", err)
	}

	// Re-read the manifest and verify the description was updated.
	updated, err := ReadManifest(manifestPath)
	if err != nil {
		t.Fatalf("ReadManifest() error = %v", err)
	}
	if updated.Description != "after rename" {
		t.Errorf("Description = %q, want %q", updated.Description, "after rename")
	}
}

// TestDisplayLabelPin verifies that DisplayLabel prepends "[pinned]" when
// Manifest.Pinned is true, and produces no pin indicator when Pinned is false.
func TestDisplayLabelPin(t *testing.T) {
	ts := time.Date(2026, 3, 22, 15, 4, 5, 0, time.UTC)

	tests := []struct {
		name       string
		manifest   Manifest
		wantPrefix string // must be present in label
		wantAbsent string // must NOT be present
	}{
		{
			name: "pinned shows [pinned] prefix",
			manifest: Manifest{
				ID:        "pinned-backup",
				CreatedAt: ts,
				Source:    BackupSourceInstall,
				FileCount: 5,
				Pinned:    true,
			},
			wantPrefix: "[pinned]",
		},
		{
			name: "unpinned shows no pin indicator",
			manifest: Manifest{
				ID:        "unpinned-backup",
				CreatedAt: ts,
				Source:    BackupSourceInstall,
				FileCount: 5,
				Pinned:    false,
			},
			wantAbsent: "[pinned]",
		},
		{
			name: "pinned with zero FileCount still shows [pinned]",
			manifest: Manifest{
				ID:        "pinned-no-count",
				CreatedAt: ts,
				Source:    BackupSourceSync,
				FileCount: 0,
				Pinned:    true,
			},
			wantPrefix: "[pinned]",
			wantAbsent: "files",
		},
		{
			name: "pinned label still includes source",
			manifest: Manifest{
				ID:        "pinned-with-source",
				CreatedAt: ts,
				Source:    BackupSourceUpgrade,
				Pinned:    true,
			},
			wantPrefix: "[pinned]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.manifest.DisplayLabel()
			if tt.wantPrefix != "" && !strings.Contains(got, tt.wantPrefix) {
				t.Errorf("DisplayLabel() = %q, want string containing %q", got, tt.wantPrefix)
			}
			if tt.wantAbsent != "" && strings.Contains(got, tt.wantAbsent) {
				t.Errorf("DisplayLabel() = %q, must NOT contain %q", got, tt.wantAbsent)
			}
		})
	}
}

// TestTogglePin_PinsUnpinnedBackup verifies that TogglePin sets Pinned=true
// when the manifest currently has Pinned=false.
func TestTogglePin_PinsUnpinnedBackup(t *testing.T) {
	dir := t.TempDir()
	backupDir := filepath.Join(dir, "backup-toggle-01")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	m := Manifest{
		ID:      "backup-toggle-01",
		RootDir: backupDir,
		Pinned:  false,
		Entries: []ManifestEntry{},
	}
	if err := WriteManifest(filepath.Join(backupDir, ManifestFilename), m); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}

	if err := TogglePin(m); err != nil {
		t.Fatalf("TogglePin() error = %v", err)
	}

	// Re-read manifest from disk and verify Pinned is now true.
	updated, err := ReadManifest(filepath.Join(backupDir, ManifestFilename))
	if err != nil {
		t.Fatalf("ReadManifest() error = %v", err)
	}
	if !updated.Pinned {
		t.Errorf("Pinned = %v, want true after TogglePin on unpinned backup", updated.Pinned)
	}
}

// TestTogglePin_UnpinsPinnedBackup verifies that TogglePin sets Pinned=false
// when the manifest currently has Pinned=true.
func TestTogglePin_UnpinsPinnedBackup(t *testing.T) {
	dir := t.TempDir()
	backupDir := filepath.Join(dir, "backup-toggle-02")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	m := Manifest{
		ID:      "backup-toggle-02",
		RootDir: backupDir,
		Pinned:  true,
		Entries: []ManifestEntry{},
	}
	if err := WriteManifest(filepath.Join(backupDir, ManifestFilename), m); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}

	if err := TogglePin(m); err != nil {
		t.Fatalf("TogglePin() error = %v", err)
	}

	updated, err := ReadManifest(filepath.Join(backupDir, ManifestFilename))
	if err != nil {
		t.Fatalf("ReadManifest() error = %v", err)
	}
	if updated.Pinned {
		t.Errorf("Pinned = %v, want false after TogglePin on pinned backup", updated.Pinned)
	}
}

// TestTogglePin_PersistsToManifest verifies that TogglePin persists the Pinned
// change to manifest.json on disk (toggle twice → back to original value).
func TestTogglePin_PersistsToManifest(t *testing.T) {
	dir := t.TempDir()
	backupDir := filepath.Join(dir, "backup-toggle-03")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	m := Manifest{
		ID:      "backup-toggle-03",
		RootDir: backupDir,
		Pinned:  false,
		Entries: []ManifestEntry{},
	}
	manifestPath := filepath.Join(backupDir, ManifestFilename)
	if err := WriteManifest(manifestPath, m); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}

	// First toggle: false → true.
	if err := TogglePin(m); err != nil {
		t.Fatalf("first TogglePin() error = %v", err)
	}
	after1, err := ReadManifest(manifestPath)
	if err != nil {
		t.Fatalf("ReadManifest after 1st toggle: %v", err)
	}
	if !after1.Pinned {
		t.Errorf("after 1st toggle: Pinned = %v, want true", after1.Pinned)
	}

	// Second toggle: true → false (using the updated manifest).
	if err := TogglePin(after1); err != nil {
		t.Fatalf("second TogglePin() error = %v", err)
	}
	after2, err := ReadManifest(manifestPath)
	if err != nil {
		t.Fatalf("ReadManifest after 2nd toggle: %v", err)
	}
	if after2.Pinned {
		t.Errorf("after 2nd toggle: Pinned = %v, want false", after2.Pinned)
	}
}

// TestTogglePin_ErrorOnEmptyRootDir verifies that TogglePin returns an error
// when the manifest has an empty RootDir (cannot determine where to write).
func TestTogglePin_ErrorOnEmptyRootDir(t *testing.T) {
	m := Manifest{
		ID:     "no-root",
		Pinned: false,
		// RootDir intentionally empty
	}

	err := TogglePin(m)
	if err == nil {
		t.Fatal("TogglePin() should return error when RootDir is empty")
	}
}

// TestManifestNewFields verifies the three new Manifest fields (Pinned,
// Compressed, Checksum) introduced for the retention-policy feature:
//   - Old manifests (without these fields) parse with zero values (backward compat)
//   - Fields with non-zero values round-trip via JSON
//   - omitempty: zero-value fields are absent from serialized JSON
func TestManifestNewFields(t *testing.T) {
	t.Run("old JSON without new fields parses with zero values", func(t *testing.T) {
		oldJSON := `{
  "id": "old-no-new-fields",
  "created_at": "2026-03-22T15:04:05Z",
  "root_dir": "/home/user/.gentle-ai/backups/old",
  "entries": []
}`
		dir := t.TempDir()
		path := filepath.Join(dir, "manifest.json")
		if err := os.WriteFile(path, []byte(oldJSON), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		m, err := ReadManifest(path)
		if err != nil {
			t.Fatalf("ReadManifest() error = %v", err)
		}
		if m.Pinned {
			t.Errorf("Pinned = %v, want false for old manifest", m.Pinned)
		}
		if m.Compressed {
			t.Errorf("Compressed = %v, want false for old manifest", m.Compressed)
		}
		if m.Checksum != "" {
			t.Errorf("Checksum = %q, want empty string for old manifest", m.Checksum)
		}
	})

	t.Run("non-zero new fields round-trip via JSON", func(t *testing.T) {
		original := Manifest{
			ID:         "test-new-fields",
			CreatedAt:  time.Date(2026, 3, 22, 15, 4, 5, 0, time.UTC),
			RootDir:    "/tmp/test",
			Pinned:     true,
			Compressed: true,
			Checksum:   "abc123def456",
			Entries:    []ManifestEntry{},
		}
		data, err := json.MarshalIndent(original, "", "  ")
		if err != nil {
			t.Fatalf("MarshalIndent() error = %v", err)
		}
		var decoded Manifest
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		if !decoded.Pinned {
			t.Errorf("Pinned = %v, want true", decoded.Pinned)
		}
		if !decoded.Compressed {
			t.Errorf("Compressed = %v, want true", decoded.Compressed)
		}
		if decoded.Checksum != "abc123def456" {
			t.Errorf("Checksum = %q, want %q", decoded.Checksum, "abc123def456")
		}
	})

	t.Run("zero-value new fields are omitted from JSON (omitempty)", func(t *testing.T) {
		m := Manifest{
			ID:        "test-omitempty",
			CreatedAt: time.Now().UTC(),
			RootDir:   "/tmp",
			Entries:   []ManifestEntry{},
			// Pinned, Compressed, Checksum intentionally zero
		}
		data, err := json.MarshalIndent(m, "", "  ")
		if err != nil {
			t.Fatalf("MarshalIndent() error = %v", err)
		}
		jsonStr := string(data)
		if strings.Contains(jsonStr, `"pinned"`) {
			t.Errorf("JSON contains 'pinned' but should omit it when false: %s", jsonStr)
		}
		if strings.Contains(jsonStr, `"compressed"`) {
			t.Errorf("JSON contains 'compressed' but should omit it when false: %s", jsonStr)
		}
		if strings.Contains(jsonStr, `"checksum"`) {
			t.Errorf("JSON contains 'checksum' but should omit it when empty: %s", jsonStr)
		}
	})
}
