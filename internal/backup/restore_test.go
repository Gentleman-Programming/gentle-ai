package backup

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRestoreCompressedRejectsSnapshotPathTraversal(t *testing.T) {
	home := t.TempDir()
	origUserHomeDirFn := UserHomeDirFn
	origBackupRootFn := BackupRootFn
	t.Cleanup(func() { UserHomeDirFn = origUserHomeDirFn })
	t.Cleanup(func() { BackupRootFn = origBackupRootFn })
	UserHomeDirFn = func() (string, error) { return home, nil }
	BackupRootFn = func() (string, error) { return home, nil }

	backupDir := filepath.Join(home, "backup")
	originalPath := filepath.Join(home, "config", "settings.json")
	if err := os.MkdirAll(filepath.Dir(originalPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(originalPath, []byte("original\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() original error = %v", err)
	}

	snapshotter := Snapshotter{now: func() time.Time { return time.Now() }}
	manifest, err := snapshotter.Create(backupDir, []string{originalPath})
	if err != nil {
		t.Fatalf("Snapshotter.Create() error = %v", err)
	}

	if err := os.WriteFile(originalPath, []byte("modified\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() modified error = %v", err)
	}

	if len(manifest.Entries) != 1 {
		t.Fatalf("entries len = %d, want 1", len(manifest.Entries))
	}

	manifest.Entries[0].SnapshotPath = "../gentle-ai-restore-escape.json"
	escapePath := filepath.Join(os.TempDir(), "gentle-ai-restore-escape.json")
	t.Cleanup(func() { _ = os.Remove(escapePath) })
	if err := os.WriteFile(escapePath, []byte("malicious\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() escape file error = %v", err)
	}

	service := RestoreService{}
	if err := service.Restore(manifest); err == nil {
		t.Fatalf("Restore() expected error for traversal SnapshotPath")
	}

	got, err := os.ReadFile(originalPath)
	if err != nil {
		t.Fatalf("ReadFile() original path error = %v", err)
	}
	if string(got) != "modified\n" {
		t.Fatalf("original content changed = %q, want %q", string(got), "modified\n")
	}
}

func TestBackupRootFnUsesUserHomeDirFn(t *testing.T) {
	origUserHomeDirFn := UserHomeDirFn
	origBackupRootFn := BackupRootFn
	t.Cleanup(func() {
		UserHomeDirFn = origUserHomeDirFn
		BackupRootFn = origBackupRootFn
	})

	home := t.TempDir()
	UserHomeDirFn = func() (string, error) { return home, nil }
	BackupRootFn = origBackupRootFn

	backupRoot, err := BackupRootFn()
	if err != nil {
		t.Fatalf("BackupRootFn() error = %v", err)
	}

	want := filepath.Join(home, ".gentle-ai", "backups")
	if backupRoot != want {
		t.Fatalf("BackupRootFn() = %q, want %q", backupRoot, want)
	}
}

func TestBackupRootFnPropagatesUserHomeDirError(t *testing.T) {
	origUserHomeDirFn := UserHomeDirFn
	origBackupRootFn := BackupRootFn
	t.Cleanup(func() {
		UserHomeDirFn = origUserHomeDirFn
		BackupRootFn = origBackupRootFn
	})

	UserHomeDirFn = func() (string, error) { return "", errors.New("boom") }
	BackupRootFn = origBackupRootFn

	if _, err := BackupRootFn(); err == nil {
		t.Fatalf("BackupRootFn() expected error when UserHomeDirFn fails")
	}
}

func TestRestoreRejectsOriginalPathWhenParentSymlinkEscapesHome(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()

	origUserHomeDirFn := UserHomeDirFn
	origBackupRootFn := BackupRootFn
	t.Cleanup(func() {
		UserHomeDirFn = origUserHomeDirFn
		BackupRootFn = origBackupRootFn
	})

	UserHomeDirFn = func() (string, error) { return home, nil }
	BackupRootFn = func() (string, error) { return home, nil }

	symlinkParent := filepath.Join(home, "linked")
	if err := os.Symlink(outside, symlinkParent); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(outside, "nested"), 0o755); err != nil {
		t.Fatalf("MkdirAll() outside nested dir error = %v", err)
	}

	originalPath := filepath.Join(symlinkParent, "nested", "settings.json")
	snapshotPath := filepath.Join(home, "backup", "files", "settings.json")
	if err := os.MkdirAll(filepath.Dir(snapshotPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() snapshot dir error = %v", err)
	}
	if err := os.WriteFile(snapshotPath, []byte("safe\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() snapshot error = %v", err)
	}

	service := RestoreService{}
	err := service.Restore(Manifest{Entries: []ManifestEntry{{
		OriginalPath: originalPath,
		SnapshotPath: snapshotPath,
		Existed:      true,
		Mode:         0o600,
	}}})
	if err == nil {
		t.Fatalf("Restore() expected error when OriginalPath parent symlink escapes home")
	}

	escapedWritePath := filepath.Join(outside, "nested", "settings.json")
	if _, statErr := os.Stat(escapedWritePath); !os.IsNotExist(statErr) {
		t.Fatalf("expected no write outside home via symlink parent, stat err = %v", statErr)
	}
}

func TestRestoreCompressedRejectsRootDirOutsideBackupRoot(t *testing.T) {
	home := t.TempDir()
	backupRoot := filepath.Join(home, ".gentle-ai", "backups")
	outsideBackupDir := t.TempDir()

	origUserHomeDirFn := UserHomeDirFn
	origBackupRootFn := BackupRootFn
	t.Cleanup(func() {
		UserHomeDirFn = origUserHomeDirFn
		BackupRootFn = origBackupRootFn
	})

	UserHomeDirFn = func() (string, error) { return home, nil }
	BackupRootFn = func() (string, error) { return backupRoot, nil }

	sourcePath := filepath.Join(home, "config", "settings.json")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("MkdirAll() source dir error = %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("original\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() source error = %v", err)
	}

	snapshotter := Snapshotter{now: func() time.Time { return time.Now() }}
	manifest, err := snapshotter.Create(outsideBackupDir, []string{sourcePath})
	if err != nil {
		t.Fatalf("Snapshotter.Create() error = %v", err)
	}
	if !manifest.Compressed {
		t.Fatalf("expected compressed manifest")
	}

	if err := os.WriteFile(sourcePath, []byte("mutated\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() mutate source error = %v", err)
	}

	service := RestoreService{}
	if err := service.Restore(manifest); err == nil {
		t.Fatalf("Restore() expected error for RootDir outside backup root")
	}

	got, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("ReadFile() source after restore error = %v", err)
	}
	if string(got) != "mutated\n" {
		t.Fatalf("source content changed = %q, want %q", string(got), "mutated\n")
	}
}

func TestRestoreCompressedAcceptsRootDirWhenBackupRootIsSymlinkAlias(t *testing.T) {
	home := t.TempDir()
	backupRoot := filepath.Join(home, ".gentle-ai", "backups")
	outsideBackupDir := t.TempDir()

	origUserHomeDirFn := UserHomeDirFn
	origBackupRootFn := BackupRootFn
	t.Cleanup(func() {
		UserHomeDirFn = origUserHomeDirFn
		BackupRootFn = origBackupRootFn
	})

	UserHomeDirFn = func() (string, error) { return home, nil }
	BackupRootFn = func() (string, error) { return backupRoot, nil }

	if err := os.MkdirAll(filepath.Dir(backupRoot), 0o755); err != nil {
		t.Fatalf("MkdirAll() backup root parent error = %v", err)
	}
	if err := os.Symlink(outsideBackupDir, backupRoot); err != nil {
		t.Fatalf("Symlink() backup root error = %v", err)
	}

	sourcePath := filepath.Join(home, "config", "settings.json")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("MkdirAll() source dir error = %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("original\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() source error = %v", err)
	}

	snapshotter := Snapshotter{now: func() time.Time { return time.Now() }}
	manifest, err := snapshotter.Create(outsideBackupDir, []string{sourcePath})
	if err != nil {
		t.Fatalf("Snapshotter.Create() error = %v", err)
	}
	if !manifest.Compressed {
		t.Fatalf("expected compressed manifest")
	}

	manifest.RootDir = backupRoot

	if err := os.WriteFile(sourcePath, []byte("mutated\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() mutate source error = %v", err)
	}

	service := RestoreService{}
	if err := service.Restore(manifest); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	got, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("ReadFile() source after restore error = %v", err)
	}
	if string(got) != "original\n" {
		t.Fatalf("source content = %q, want %q", string(got), "original\n")
	}
}

func TestIsRootDirUnderBackupRootAcceptsCanonicalRootWhenBackupRootIsSymlink(t *testing.T) {
	home := t.TempDir()
	realBackupRoot := t.TempDir()
	backupRootAlias := filepath.Join(home, ".gentle-ai", "backups")

	origBackupRootFn := BackupRootFn
	t.Cleanup(func() { BackupRootFn = origBackupRootFn })

	if err := os.MkdirAll(filepath.Dir(backupRootAlias), 0o755); err != nil {
		t.Fatalf("MkdirAll() backup root alias parent error = %v", err)
	}
	if err := os.Symlink(realBackupRoot, backupRootAlias); err != nil {
		t.Fatalf("Symlink() backup root alias error = %v", err)
	}

	BackupRootFn = func() (string, error) { return backupRootAlias, nil }

	rootDir := filepath.Join(realBackupRoot, "snapshot-2026")
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() rootDir error = %v", err)
	}

	ok, err := isRootDirUnderBackupRoot(rootDir)
	if err != nil {
		t.Fatalf("isRootDirUnderBackupRoot() error = %v", err)
	}
	if !ok {
		t.Fatalf("isRootDirUnderBackupRoot() = false, want true for canonical root under symlinked backup root")
	}
}

func TestRestoreAcceptsSnapshotPathWhenBackupRootIsSymlinkAlias(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()

	origUserHomeDirFn := UserHomeDirFn
	origBackupRootFn := BackupRootFn
	t.Cleanup(func() {
		UserHomeDirFn = origUserHomeDirFn
		BackupRootFn = origBackupRootFn
	})

	backupRoot := filepath.Join(home, ".gentle-ai", "backups")
	UserHomeDirFn = func() (string, error) { return home, nil }
	BackupRootFn = func() (string, error) { return backupRoot, nil }

	if err := os.MkdirAll(filepath.Dir(backupRoot), 0o755); err != nil {
		t.Fatalf("MkdirAll() backup root parent error = %v", err)
	}
	if err := os.Symlink(outside, backupRoot); err != nil {
		t.Fatalf("Symlink() backup root error = %v", err)
	}

	originalPath := filepath.Join(home, "config", "app.json")
	if err := os.MkdirAll(filepath.Dir(originalPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() original dir error = %v", err)
	}
	if err := os.WriteFile(originalPath, []byte("mutated\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() original error = %v", err)
	}

	snapshotPath := filepath.Join(backupRoot, "files", "app.json")
	if err := os.MkdirAll(filepath.Dir(snapshotPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() snapshot dir error = %v", err)
	}
	if err := os.WriteFile(snapshotPath, []byte("outside\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() snapshot error = %v", err)
	}

	service := RestoreService{}
	err := service.Restore(Manifest{Entries: []ManifestEntry{{
		OriginalPath: originalPath,
		SnapshotPath: snapshotPath,
		Existed:      true,
		Mode:         0o600,
	}}})
	if err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	got, err := os.ReadFile(originalPath)
	if err != nil {
		t.Fatalf("ReadFile() original after restore error = %v", err)
	}
	if string(got) != "outside\n" {
		t.Fatalf("original content = %q, want %q", string(got), "outside\n")
	}
}

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

func TestRestoreRemovesCreatedDirectoryWhenExistedFalse(t *testing.T) {
	home := t.TempDir()
	origUserHomeDirFn := UserHomeDirFn
	origBackupRootFn := BackupRootFn
	t.Cleanup(func() {
		UserHomeDirFn = origUserHomeDirFn
		BackupRootFn = origBackupRootFn
	})
	UserHomeDirFn = func() (string, error) { return home, nil }
	BackupRootFn = func() (string, error) { return home, nil }

	snapshotPath := filepath.Join(home, "backup", "files", "settings.json")
	if err := os.MkdirAll(filepath.Dir(snapshotPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() snapshot dir error = %v", err)
	}
	if err := os.WriteFile(snapshotPath, []byte("old\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() snapshot error = %v", err)
	}

	originalPath := filepath.Join(home, "config", "settings.json")
	if err := os.MkdirAll(filepath.Dir(originalPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() original dir error = %v", err)
	}
	if err := os.WriteFile(originalPath, []byte("new\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() original error = %v", err)
	}

	createdDir := filepath.Join(home, "config", "plugins")
	if err := os.MkdirAll(createdDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() created dir error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(createdDir, "temp.txt"), []byte("temp\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() created dir nested file error = %v", err)
	}

	manifest := Manifest{Entries: []ManifestEntry{
		{OriginalPath: originalPath, SnapshotPath: snapshotPath, Existed: true, Mode: 0o600},
		{OriginalPath: createdDir, Existed: false},
	}}

	service := RestoreService{}
	if err := service.Restore(manifest); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	if _, err := os.Stat(createdDir); !os.IsNotExist(err) {
		t.Fatalf("expected created directory %q to be removed, got stat err = %v", createdDir, err)
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
	origBackupRootFn := BackupRootFn
	t.Cleanup(func() {
		UserHomeDirFn = origUserHomeDirFn
		BackupRootFn = origBackupRootFn
	})
	UserHomeDirFn = func() (string, error) { return home, nil }
	BackupRootFn = func() (string, error) { return home, nil }

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
	origBackupRootFn := BackupRootFn
	t.Cleanup(func() {
		UserHomeDirFn = origUserHomeDirFn
		BackupRootFn = origBackupRootFn
	})
	UserHomeDirFn = func() (string, error) { return home, nil }
	BackupRootFn = func() (string, error) { return home, nil }

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

func TestRestoreCompressedRejectsArchivePathSymlinkEscapingBackupRoot(t *testing.T) {
	home := t.TempDir()
	backupRoot := filepath.Join(home, ".gentle-ai", "backups")

	origUserHomeDirFn := UserHomeDirFn
	origBackupRootFn := BackupRootFn
	t.Cleanup(func() {
		UserHomeDirFn = origUserHomeDirFn
		BackupRootFn = origBackupRootFn
	})

	UserHomeDirFn = func() (string, error) { return home, nil }
	BackupRootFn = func() (string, error) { return backupRoot, nil }

	backupDir := filepath.Join(backupRoot, "backup-01")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() backup dir error = %v", err)
	}

	originalPath := filepath.Join(home, "config", "settings.json")
	if err := os.MkdirAll(filepath.Dir(originalPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() original dir error = %v", err)
	}
	if err := os.WriteFile(originalPath, []byte("mutated\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() original error = %v", err)
	}

	payloadFile := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(payloadFile, []byte("outside\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() payload error = %v", err)
	}

	outsideArchive := filepath.Join(t.TempDir(), ArchiveFilename)
	if err := CreateArchive(outsideArchive, []ArchiveEntry{{
		RelPath:    "files/config/settings.json",
		SourcePath: payloadFile,
		Mode:       0o600,
	}}); err != nil {
		t.Fatalf("CreateArchive() error = %v", err)
	}

	archivePath := filepath.Join(backupDir, ArchiveFilename)
	if err := os.Symlink(outsideArchive, archivePath); err != nil {
		t.Fatalf("Symlink() archive error = %v", err)
	}

	manifest := Manifest{
		RootDir:    backupDir,
		Compressed: true,
		Entries: []ManifestEntry{{
			OriginalPath: originalPath,
			SnapshotPath: "files/config/settings.json",
			Existed:      true,
			Mode:         0o600,
		}},
	}

	service := RestoreService{}
	if err := service.Restore(manifest); err == nil {
		t.Fatal("Restore() expected error when archive path escapes backup root via symlink")
	}

	got, err := os.ReadFile(originalPath)
	if err != nil {
		t.Fatalf("ReadFile() original error = %v", err)
	}
	if string(got) != "mutated\n" {
		t.Fatalf("original content changed = %q, want %q", string(got), "mutated\n")
	}
}

// TestRestoreCompressedRemovesCreatedFiles verifies that entries with Existed=false
// in a compressed backup cause the file at OriginalPath to be deleted (BKUP-T32).
func TestRestoreCompressedRemovesCreatedFiles(t *testing.T) {
	home := t.TempDir()
	// Override UserHomeDirFn so validation accepts paths under t.TempDir().
	origUserHomeDirFn := UserHomeDirFn
	origBackupRootFn := BackupRootFn
	t.Cleanup(func() {
		UserHomeDirFn = origUserHomeDirFn
		BackupRootFn = origBackupRootFn
	})
	UserHomeDirFn = func() (string, error) { return home, nil }
	BackupRootFn = func() (string, error) { return home, nil }

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

func TestRestoreCompressedRemovesCreatedDirectoryWhenExistedFalse(t *testing.T) {
	home := t.TempDir()
	origUserHomeDirFn := UserHomeDirFn
	origBackupRootFn := BackupRootFn
	t.Cleanup(func() {
		UserHomeDirFn = origUserHomeDirFn
		BackupRootFn = origBackupRootFn
	})
	UserHomeDirFn = func() (string, error) { return home, nil }
	BackupRootFn = func() (string, error) { return home, nil }

	backupDir := filepath.Join(home, "backup")
	srcFile := filepath.Join(home, "config", "kept.json")
	if err := os.MkdirAll(filepath.Dir(srcFile), 0o755); err != nil {
		t.Fatalf("MkdirAll() source dir error = %v", err)
	}
	if err := os.WriteFile(srcFile, []byte("data\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() source error = %v", err)
	}

	snapshotter := Snapshotter{now: func() time.Time { return time.Now() }}
	manifest, err := snapshotter.Create(backupDir, []string{srcFile})
	if err != nil {
		t.Fatalf("Snapshotter.Create() error = %v", err)
	}

	createdDir := filepath.Join(home, "config", "after")
	if err := os.MkdirAll(createdDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() created dir error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(createdDir, "temp.txt"), []byte("temp\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() created dir nested file error = %v", err)
	}
	manifest.Entries = append(manifest.Entries, ManifestEntry{
		OriginalPath: createdDir,
		Existed:      false,
	})

	service := RestoreService{}
	if err := service.Restore(manifest); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	if _, err := os.Stat(createdDir); !os.IsNotExist(err) {
		t.Fatalf("expected created directory %q to be removed, got stat err = %v", createdDir, err)
	}
}
