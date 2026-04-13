package backup

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRestoreRestoresExistingAndRemovesCreated(t *testing.T) {
	home := t.TempDir()
	// Override fns so validation accepts paths under t.TempDir().
	origUserHomeDirFn := UserHomeDirFn
	origBackupRootFn := BackupRootFn
	t.Cleanup(func() {
		UserHomeDirFn = origUserHomeDirFn
		BackupRootFn = origBackupRootFn
	})
	UserHomeDirFn = func() (string, error) { return home, nil }
	BackupRootFn = func() (string, error) { return home, nil }

	originalPath := filepath.Join(home, "config", "settings.json")
	if err := os.MkdirAll(filepath.Dir(originalPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(originalPath, []byte("new\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	removedPath := filepath.Join(home, "config", "extra.json")
	if err := os.WriteFile(removedPath, []byte("temporary\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() removed path error = %v", err)
	}

	snapshotPath := filepath.Join(home, "backup", "files", "settings.json")
	if err := os.MkdirAll(filepath.Dir(snapshotPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() snapshot error = %v", err)
	}
	if err := os.WriteFile(snapshotPath, []byte("old\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() snapshot error = %v", err)
	}

	manifest := Manifest{
		Entries: []ManifestEntry{
			{OriginalPath: originalPath, SnapshotPath: snapshotPath, Existed: true, Mode: 0o600},
			{OriginalPath: removedPath, Existed: false},
		},
	}

	service := RestoreService{}
	if err := service.Restore(manifest); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	restored, err := os.ReadFile(originalPath)
	if err != nil {
		t.Fatalf("ReadFile() restored path error = %v", err)
	}
	if string(restored) != "old\n" {
		t.Fatalf("restored content = %q", string(restored))
	}

	if _, err := os.Stat(removedPath); !os.IsNotExist(err) {
		t.Fatalf("expected removed path %q to be deleted, err = %v", removedPath, err)
	}
}

func TestRestoreFailsWhenSnapshotMissing(t *testing.T) {
	tmpDir := t.TempDir()
	// Override fns so validation accepts paths under t.TempDir().
	origUserHomeDirFn := UserHomeDirFn
	origBackupRootFn := BackupRootFn
	t.Cleanup(func() {
		UserHomeDirFn = origUserHomeDirFn
		BackupRootFn = origBackupRootFn
	})
	UserHomeDirFn = func() (string, error) { return tmpDir, nil }
	BackupRootFn = func() (string, error) { return tmpDir, nil }

	service := RestoreService{}
	err := service.Restore(Manifest{Entries: []ManifestEntry{{
		OriginalPath: filepath.Join(tmpDir, "out.json"),
		SnapshotPath: filepath.Join(tmpDir, "missing.json"),
		Existed:      true,
		Mode:         0o644,
	}}})

	if err == nil {
		t.Fatalf("Restore() expected error for missing snapshot")
	}
}

// TestRestoreCompressedBackup verifies that Restore() correctly extracts files
// from a tar.gz archive when manifest.Compressed == true (BKUP-T31).
func TestRestoreCompressedBackup(t *testing.T) {
	home := t.TempDir()
	// Override fns so validation accepts paths under t.TempDir().
	origUserHomeDirFn := UserHomeDirFn
	t.Cleanup(func() { UserHomeDirFn = origUserHomeDirFn })
	UserHomeDirFn = func() (string, error) { return home, nil }

	backupDir := filepath.Join(home, "backup")

	// Create a source file to snapshot.
	srcFile := filepath.Join(home, "config", "settings.json")
	if err := os.MkdirAll(filepath.Dir(srcFile), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(srcFile, []byte("original content\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Use Snapshotter to create a compressed backup — this produces snapshot.tar.gz
	// and sets Compressed=true + relative SnapshotPaths in the manifest.
	snapshotter := Snapshotter{now: func() time.Time { return time.Now() }}
	manifest, err := snapshotter.Create(backupDir, []string{srcFile})
	if err != nil {
		t.Fatalf("Snapshotter.Create() error = %v", err)
	}
	if !manifest.Compressed {
		t.Fatalf("expected Compressed=true, got false")
	}

	// Overwrite the source file so we can verify restore brought back the original.
	if err := os.WriteFile(srcFile, []byte("modified content\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() overwrite error = %v", err)
	}

	service := RestoreService{}
	if err := service.Restore(manifest); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	restored, err := os.ReadFile(srcFile)
	if err != nil {
		t.Fatalf("ReadFile() after restore error = %v", err)
	}
	if string(restored) != "original content\n" {
		t.Fatalf("restored content = %q, want %q", string(restored), "original content\n")
	}
}

// TestRestoreUncompressedBackup verifies backward compatibility: old-style backups
// with Compressed==false (plain files on disk) still restore correctly (BKUP-T30).
func TestRestoreUncompressedBackup(t *testing.T) {
	home := t.TempDir()
	// Override fns so validation accepts paths under t.TempDir().
	origUserHomeDirFn := UserHomeDirFn
	origBackupRootFn := BackupRootFn
	t.Cleanup(func() {
		UserHomeDirFn = origUserHomeDirFn
		BackupRootFn = origBackupRootFn
	})
	UserHomeDirFn = func() (string, error) { return home, nil }
	BackupRootFn = func() (string, error) { return home, nil }

	originalPath := filepath.Join(home, "config", "app.json")
	if err := os.MkdirAll(filepath.Dir(originalPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(originalPath, []byte("modified\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	snapshotPath := filepath.Join(home, "backup", "files", "app.json")
	if err := os.MkdirAll(filepath.Dir(snapshotPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() snapshot dir error = %v", err)
	}
	if err := os.WriteFile(snapshotPath, []byte("original\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() snapshot error = %v", err)
	}

	// Manifest with Compressed=false (zero value) — old-style plain files.
	manifest := Manifest{
		Compressed: false,
		Entries: []ManifestEntry{
			{OriginalPath: originalPath, SnapshotPath: snapshotPath, Existed: true, Mode: 0o600},
		},
	}

	service := RestoreService{}
	if err := service.Restore(manifest); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	got, err := os.ReadFile(originalPath)
	if err != nil {
		t.Fatalf("ReadFile() after restore error = %v", err)
	}
	if string(got) != "original\n" {
		t.Fatalf("restored content = %q, want %q", string(got), "original\n")
	}
}

// TestRestoreCompressedMultipleFiles triangulates the compressed restore path
// with more than one file, ensuring the loop resolves all relative paths correctly.
func TestRestoreCompressedMultipleFiles(t *testing.T) {
	home := t.TempDir()
	// Override UserHomeDirFn so validation accepts paths under t.TempDir().
	origUserHomeDirFn := UserHomeDirFn
	t.Cleanup(func() { UserHomeDirFn = origUserHomeDirFn })
	UserHomeDirFn = func() (string, error) { return home, nil }

	backupDir := filepath.Join(home, "backup")

	fileA := filepath.Join(home, "config", "a.json")
	fileB := filepath.Join(home, "config", "b.json")
	if err := os.MkdirAll(filepath.Dir(fileA), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(fileA, []byte("content-a\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() a error = %v", err)
	}
	if err := os.WriteFile(fileB, []byte("content-b\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() b error = %v", err)
	}

	snapshotter := Snapshotter{now: func() time.Time { return time.Now() }}
	manifest, err := snapshotter.Create(backupDir, []string{fileA, fileB})
	if err != nil {
		t.Fatalf("Snapshotter.Create() error = %v", err)
	}

	// Overwrite both files.
	if err := os.WriteFile(fileA, []byte("dirty-a\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() overwrite a error = %v", err)
	}
	if err := os.WriteFile(fileB, []byte("dirty-b\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() overwrite b error = %v", err)
	}

	service := RestoreService{}
	if err := service.Restore(manifest); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	gotA, err := os.ReadFile(fileA)
	if err != nil {
		t.Fatalf("ReadFile(a) error = %v", err)
	}
	if string(gotA) != "content-a\n" {
		t.Fatalf("fileA restored content = %q, want %q", string(gotA), "content-a\n")
	}

	gotB, err := os.ReadFile(fileB)
	if err != nil {
		t.Fatalf("ReadFile(b) error = %v", err)
	}
	if string(gotB) != "content-b\n" {
		t.Fatalf("fileB restored content = %q, want %q", string(gotB), "content-b\n")
	}
}

// TestRestoreCompressed_MissingArchive verifies that Restore returns an error
// when the manifest has Compressed==true but snapshot.tar.gz does not exist.
func TestRestoreCompressed_MissingArchive(t *testing.T) {
	home := t.TempDir()
	backupDir := filepath.Join(home, "backup-no-archive")
	// Create the backup directory but do NOT create snapshot.tar.gz inside it.
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	manifest := Manifest{
		RootDir:    backupDir,
		Compressed: true,
		Entries: []ManifestEntry{
			{
				OriginalPath: filepath.Join(home, "config", "settings.json"),
				SnapshotPath: "files/config/settings.json",
				Existed:      true,
				Mode:         0o644,
			},
		},
	}

	service := RestoreService{}
	err := service.Restore(manifest)
	if err == nil {
		t.Fatal("Restore() should return error when snapshot.tar.gz is missing")
	}
}

// TestRestoreCompressedRemovesCreatedFiles verifies that entries with Existed=false
// in a compressed backup cause the file at OriginalPath to be deleted (BKUP-T32).
func TestRestoreCompressedRemovesCreatedFiles(t *testing.T) {
	home := t.TempDir()
	// Override UserHomeDirFn so validation accepts paths under t.TempDir().
	origUserHomeDirFn := UserHomeDirFn
	t.Cleanup(func() { UserHomeDirFn = origUserHomeDirFn })
	UserHomeDirFn = func() (string, error) { return home, nil }

	backupDir := filepath.Join(home, "backup")

	// Create a real file to snapshot (so the archive is valid).
	srcFile := filepath.Join(home, "config", "kept.json")
	if err := os.MkdirAll(filepath.Dir(srcFile), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(srcFile, []byte("data\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	snapshotter := Snapshotter{now: func() time.Time { return time.Now() }}
	manifest, err := snapshotter.Create(backupDir, []string{srcFile})
	if err != nil {
		t.Fatalf("Snapshotter.Create() error = %v", err)
	}

	// Add an entry that was NOT in the original snapshot (Existed=false).
	// This simulates a file created AFTER backup — restore should remove it.
	createdFile := filepath.Join(home, "config", "extra.json")
	if err := os.WriteFile(createdFile, []byte("should be removed\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() created file error = %v", err)
	}
	manifest.Entries = append(manifest.Entries, ManifestEntry{
		OriginalPath: createdFile,
		Existed:      false,
	})

	service := RestoreService{}
	if err := service.Restore(manifest); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	if _, statErr := os.Stat(createdFile); !os.IsNotExist(statErr) {
		t.Fatalf("expected %q to be removed after restore, got stat err = %v", createdFile, statErr)
	}
}
