package backup

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

// ─── Scope containment (issue #2451) ───────────────────────────────────────
//
// These tests pin the RestoreService.Roots contract: a workspace-scoped
// rollback must be able to restore/remove files under its workspace root
// (which is frequently not under the user home directory) without tripping
// the "must be an absolute path under the user home directory" refusal,
// while a path that escapes every allowed root — via traversal or a symlink
// — must still be refused, and the refusal must name the roots it checked.

// TestRestoreScope_WorkspaceRootRestoresAndRemovesWithoutError pins property 1:
// a workspace-scoped rollback restores a file under the workspace root (and
// removes a workspace-scoped file that did not exist at snapshot time)
// without error, when the workspace root is supplied via Roots the way
// rollbackRoots (internal/cli) supplies it.
func TestRestoreScope_WorkspaceRootRestoresAndRemovesWithoutError(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()

	origBackupRootFn := BackupRootFn
	t.Cleanup(func() { BackupRootFn = origBackupRootFn })
	BackupRootFn = func() (string, error) { return home, nil }

	// entry A: existed at snapshot time, under the workspace root — restore
	// must write the snapshotted content back.
	originalPath := filepath.Join(workspace, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(originalPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(originalPath, []byte("modified\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	snapshotPath := filepath.Join(home, "backup", "files", "settings.json")
	if err := os.MkdirAll(filepath.Dir(snapshotPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() snapshot error = %v", err)
	}
	if err := os.WriteFile(snapshotPath, []byte("original\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() snapshot error = %v", err)
	}

	// entry B: did not exist at snapshot time, under the workspace root —
	// restore must remove it (mirrors the engram MCP config scenario from #2451).
	createdPath := filepath.Join(workspace, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(createdPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(createdPath, []byte("written by the failed install\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	manifest := Manifest{
		RootDir: filepath.Join(home, "backup"),
		Entries: []ManifestEntry{
			{OriginalPath: originalPath, SnapshotPath: snapshotPath, Existed: true, Mode: 0o600},
			{OriginalPath: createdPath, Existed: false},
		},
	}

	// Roots mirrors internal/cli's rollbackRoots(homeDir, workspaceDir): the
	// scope root comes from the caller, never from manifest.RootDir.
	service := RestoreService{Roots: []string{home, workspace}}
	if err := service.Restore(manifest); err != nil {
		t.Fatalf("Restore() error = %v, want no error for a workspace-scoped rollback", err)
	}

	restored, err := os.ReadFile(originalPath)
	if err != nil {
		t.Fatalf("ReadFile() restored path error = %v", err)
	}
	if string(restored) != "original\n" {
		t.Fatalf("restored content = %q, want %q", string(restored), "original\n")
	}

	if _, statErr := os.Stat(createdPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected workspace-scoped created path %q to be removed, stat err = %v", createdPath, statErr)
	}
}

// TestRestoreScope_HomeRootUnchanged pins property 2: a home-scoped rollback
// (Roots containing only the home directory, exactly what rollbackRoots
// produces for a ScopeGlobal install) keeps restoring and removing exactly
// as it did before Roots existed.
func TestRestoreScope_HomeRootUnchanged(t *testing.T) {
	home := t.TempDir()

	origBackupRootFn := BackupRootFn
	t.Cleanup(func() { BackupRootFn = origBackupRootFn })
	BackupRootFn = func() (string, error) { return home, nil }

	originalPath := filepath.Join(home, "config", "settings.json")
	if err := os.MkdirAll(filepath.Dir(originalPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(originalPath, []byte("modified\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	snapshotPath := filepath.Join(home, "backup", "files", "settings.json")
	if err := os.MkdirAll(filepath.Dir(snapshotPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() snapshot error = %v", err)
	}
	if err := os.WriteFile(snapshotPath, []byte("original\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() snapshot error = %v", err)
	}

	manifest := Manifest{
		Entries: []ManifestEntry{
			{OriginalPath: originalPath, SnapshotPath: snapshotPath, Existed: true, Mode: 0o600},
		},
	}

	service := RestoreService{Roots: []string{home}}
	if err := service.Restore(manifest); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	restored, err := os.ReadFile(originalPath)
	if err != nil {
		t.Fatalf("ReadFile() restored path error = %v", err)
	}
	if string(restored) != "original\n" {
		t.Fatalf("restored content = %q, want %q", string(restored), "original\n")
	}
}

// TestRestoreScope_TraversalEscapingRootStillRefuses pins half of property 3:
// an OriginalPath containing ".." segments that clean outside every allowed
// root is refused, even though the raw string is textually prefixed by the
// workspace root.
func TestRestoreScope_TraversalEscapingRootStillRefuses(t *testing.T) {
	parent := t.TempDir()
	workspace := filepath.Join(parent, "workspace")
	secret := filepath.Join(parent, "secret")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("MkdirAll(workspace) error = %v", err)
	}
	if err := os.MkdirAll(secret, 0o755); err != nil {
		t.Fatalf("MkdirAll(secret) error = %v", err)
	}

	// Textually prefixed by workspace, but Clean resolves it into the sibling
	// "secret" directory — outside every allowed root.
	escapingPath := filepath.Join(workspace, "..", "secret", "payload.json")

	manifest := Manifest{
		Entries: []ManifestEntry{
			{OriginalPath: escapingPath, Existed: false},
		},
	}

	service := RestoreService{Roots: []string{workspace}}
	err := service.Restore(manifest)
	if err == nil {
		t.Fatalf("Restore() expected error for a path traversing outside the allowed root")
	}
	if !strings.Contains(err.Error(), "invalid OriginalPath") {
		t.Fatalf("Restore() error = %v, want an invalid OriginalPath refusal", err)
	}
}

// TestRestoreScope_SymlinkEscapingRootStillRefuses pins the other half of
// property 3: an OriginalPath that is textually under the allowed root but
// resolves through a symlink to somewhere outside every allowed root is
// refused — mirroring the existing symlink handling documented at the top
// of restore.go for the single-root case.
func TestRestoreScope_SymlinkEscapingRootStillRefuses(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows")
	}

	workspace := t.TempDir()
	outside := t.TempDir()

	outsideFile := filepath.Join(outside, "secret.json")
	if err := os.WriteFile(outsideFile, []byte("outside\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// workspace/link -> outside (escapes the allowed root via a symlink).
	linkPath := filepath.Join(workspace, "link")
	if err := os.Symlink(outside, linkPath); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	// Textually under workspace; resolves through the symlink to "outside".
	escapingPath := filepath.Join(linkPath, "secret.json")

	manifest := Manifest{
		Entries: []ManifestEntry{
			{OriginalPath: escapingPath, Existed: false},
		},
	}

	service := RestoreService{Roots: []string{workspace}}
	err := service.Restore(manifest)
	if err == nil {
		t.Fatalf("Restore() expected error for a path symlink-escaping the allowed root")
	}
	if !strings.Contains(err.Error(), "invalid OriginalPath") {
		t.Fatalf("Restore() error = %v, want an invalid OriginalPath refusal", err)
	}

	// The real file outside the allowed root must be untouched.
	content, readErr := os.ReadFile(outsideFile)
	if readErr != nil {
		t.Fatalf("ReadFile() outside file error = %v", readErr)
	}
	if string(content) != "outside\n" {
		t.Fatalf("outside file was modified: %q", string(content))
	}
}

// TestRestoreScope_RefusalNamesTheValidatedRoots pins property 4: the refusal
// message names the root(s) actually validated against, so a user who hits
// it can tell which boundary they crossed — both for a single implicit-home
// root and for an explicit multi-root (home + workspace) rollback.
func TestRestoreScope_RefusalNamesTheValidatedRoots(t *testing.T) {
	t.Run("implicit home root", func(t *testing.T) {
		home := t.TempDir()
		origUserHomeDirFn := UserHomeDirFn
		t.Cleanup(func() { UserHomeDirFn = origUserHomeDirFn })
		UserHomeDirFn = func() (string, error) { return home, nil }

		outsidePath := filepath.Join(t.TempDir(), "outside.json")
		manifest := Manifest{Entries: []ManifestEntry{{OriginalPath: outsidePath, Existed: false}}}

		err := RestoreService{}.Restore(manifest)
		if err == nil {
			t.Fatalf("Restore() expected error")
		}
		if !strings.Contains(err.Error(), home) {
			t.Fatalf("Restore() error = %v, want it to name the validated home root %q", err, home)
		}
	})

	t.Run("explicit home and workspace roots", func(t *testing.T) {
		home := t.TempDir()
		workspace := t.TempDir()
		outsidePath := filepath.Join(t.TempDir(), "outside.json")
		manifest := Manifest{Entries: []ManifestEntry{{OriginalPath: outsidePath, Existed: false}}}

		err := RestoreService{Roots: []string{home, workspace}}.Restore(manifest)
		if err == nil {
			t.Fatalf("Restore() expected error")
		}
		if !strings.Contains(err.Error(), home) || !strings.Contains(err.Error(), workspace) {
			t.Fatalf("Restore() error = %v, want it to name both validated roots %q and %q", err, home, workspace)
		}
	})
}
