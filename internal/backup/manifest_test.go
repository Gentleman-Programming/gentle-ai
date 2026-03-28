package backup

import (
	"encoding/json"
	"os"
	"path/filepath"
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
			if !containsStr(got, tt.contains) {
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
	if !containsStr(label, "unknown source") {
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
	if containsStr(jsonStr, `"source"`) {
		t.Errorf("JSON contains 'source' field but should omit it when empty: %s", jsonStr)
	}
	if containsStr(jsonStr, `"description"`) {
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
		if containsStr(string(data), `"file_count"`) {
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
		if containsStr(string(data), `"created_by_version"`) {
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
			if tt.wantCount != "" && !containsStr(got, tt.wantCount) {
				t.Errorf("DisplayLabel() = %q, want string containing %q", got, tt.wantCount)
			}
			if tt.wantAbsent != "" && containsStr(got, tt.wantAbsent) {
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

// TestDeleteBackup_Success verifies that DeleteBackup removes the backup directory.
func TestDeleteBackup_Success(t *testing.T) {
	dir := t.TempDir()
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

func containsStr(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
