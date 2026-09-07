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
			// Kind="" (legacy unknown) with Existed=false now preserves by default.
			// Set Kind=regular explicitly to test the deletion semantics for
			// explicit regular-file removals.
			{OriginalPath: removedPath, Existed: false, Kind: PathKindRegularFile},
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
	// Legacy manifests without Kind default to "safe" (preserve). Set
	// Kind=regular explicitly to test the deletion semantics for explicit
	// regular-file removals.
	createdFile := filepath.Join(home, "config", "extra.json")
	if err := os.WriteFile(createdFile, []byte("should be removed\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() created file error = %v", err)
	}
	manifest.Entries = append(manifest.Entries, ManifestEntry{
		OriginalPath: createdFile,
		Existed:      false,
		Kind:         PathKindRegularFile,
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
	// Kind=regular opts into the deletion semantics; Kind="" (legacy unknown)
	// is preserved by default under the #2021 path-kind classification.
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
			{OriginalPath: createdPath, Existed: false, Kind: PathKindRegularFile},
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

// TestIsPathUnderRoot_FailsClosedOnAncestorSymlink pins decode2's
// 2026-08-18 review concern: a leaf path that does not exist yet but
// has an in-root ancestor symlink to outside must refuse. The pre-fix
// implementation only EvalSymlinks'd the leaf (which fails when the
// leaf is missing) and accepted the textual prefix; the new walk-up-
// to-the-deepest-existing-ancestor logic catches the escape:
//
//	/r/foo -> /etc               (symlink in ancestor chain)
//	/r/foo/bar/newfile.json      (target leaf; does not exist yet)
//
// Writing /r/foo/bar/newfile.json would land in /etc/bar/newfile.json.
// isPathUnderRoot must return false so the restore refuses.
func TestIsPathUnderRoot_FailsClosedOnAncestorSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows")
	}

	root := t.TempDir()
	outside := t.TempDir()

	foo := filepath.Join(root, "foo")
	if err := os.Symlink(outside, foo); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	// The leaf path does NOT exist; the ancestor /r/foo IS a symlink to
	// outside the root.
	leafUnderEscape := filepath.Join(foo, "subdir", "newfile.json")
	if _, err := os.Lstat(leafUnderEscape); !os.IsNotExist(err) {
		t.Fatalf("precondition: leaf %q must not exist yet, got err=%v", leafUnderEscape, err)
	}

	if isPathUnderRoot(leafUnderEscape, root) {
		t.Errorf("isPathUnderRoot(%q, %q) = true; want false (ancestor %q is a symlink to outside %q)",
			leafUnderEscape, root, foo, outside)
	}

	// Sanity: a non-escaping path under root still passes.
	if !isPathUnderRoot(filepath.Join(root, "bar", "newfile.json"), root) {
		t.Errorf("isPathUnderRoot on a plain in-root path returned false; want true")
	}
}

// TestRestoreSymlinkCollisionDetectsTypeMismatch pins decode2's
// 2026-08-18 review of PR #2021: when a manifest entry declares a
// symlink-directory but the on-disk node is a regular file (or vice
// versa), the restore must refuse rather than silently accept what
// the disk happens to hold. A tampered manifest cannot lie about
// the node type and get the restore to clobber it.
func TestRestoreSymlinkCollisionDetectsTypeMismatch(t *testing.T) {
	home := t.TempDir()
	linkPath := filepath.Join(home, "link_dir")
	if err := os.WriteFile(linkPath, []byte("regular file, not a symlink"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	manifest := Manifest{
		Entries: []ManifestEntry{
			{
				OriginalPath: linkPath, Existed: true,
				Kind: PathKindSymlinkDirectory, LinkTarget: "../somewhere_safe",
			},
		},
	}

	service := RestoreService{Roots: []string{home}}
	err := service.Restore(manifest)
	if err == nil {
		t.Fatalf("Restore() expected error for symlink entry with regular-file on-disk node, got nil")
	}
	if !strings.Contains(err.Error(), "symlink-directory") || !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("Restore() error %q must call out the type mismatch", err)
	}

	// The on-disk regular file must be untouched.
	body, readErr := os.ReadFile(linkPath)
	if readErr != nil {
		t.Fatalf("ReadFile() on-disk regular file error = %v", readErr)
	}
	if string(body) != "regular file, not a symlink" {
		t.Fatalf("on-disk regular file was modified: %q", string(body))
	}
}

// TestRestoreSymlinkCollisionDetectsTargetMismatch pins decode2's
// 2026-08-18 review of PR #2021: when the manifest declares one
// LinkTarget but the on-disk symlink already points at something
// else, the restore refuses rather than redirects. Redirecting an
// already-existing symlink could silently move a real symlink (e.g.
// one pointed at a real codepath by an admin) to a manifest-claimed
// target that diverges from reality.
func TestRestoreSymlinkCollisionDetectsTargetMismatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows")
	}
	home := t.TempDir()

	// On-disk symlink points at an in-home target; manifest claims
	// a different target. The collision must fail closed.
	onDiskDir := filepath.Join(home, "on_disk_target")
	if err := os.MkdirAll(onDiskDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	linkPath := filepath.Join(home, "link_dir")
	if err := os.Symlink("on_disk_target", linkPath); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	manifest := Manifest{
		Entries: []ManifestEntry{
			{
				OriginalPath: linkPath, Existed: true,
				Kind: PathKindSymlinkDirectory, LinkTarget: "../somewhere_else",
			},
		},
	}

	service := RestoreService{Roots: []string{home}}
	err := service.Restore(manifest)
	if err == nil {
		t.Fatalf("Restore() expected error for symlink entry whose recorded target disagrees with on-disk symlink")
	}
	if !strings.Contains(err.Error(), "redirect") || !strings.Contains(err.Error(), "../somewhere_else") {
		t.Fatalf("Restore() error %q must call out the redirect hazard", err)
	}

	// On-disk symlink must still point at on_disk_target (we did not rewrite it).
	got, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Readlink() after Restore() error = %v", err)
	}
	if got != "on_disk_target" {
		t.Fatalf("on-disk symlink target = %q, want %q (restore must not have redirected it)", got, "on_disk_target")
	}
}

// TestRestoreCompressedMixedEntries pins decode2's 2026-08-18 review
// of PR #2021: the restore path for compressed (tar.gz) backups must
// correctly handle a mix of entry kinds in one archive. Concretely:
// one regular-file entry, one directory entry, one symlink-directory
// entry whose on-disk node already exists (preserved), and one
// !Existed regular-file entry (must be removed after restore). All
// four entry kinds coexist in one compressed backup; the restore path
// must process each correctly without losing the per-entry semantics
// (no cross-entry contamination, every entry's OriginalPath is
// restored).
func TestRestoreCompressedMixedEntries(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows")
	}
	home := t.TempDir()
	// Override the home-root resolver so RestoreService accepts our
	// t.TempDir() tree as a legitimate target.
	origUserHomeDirFn := UserHomeDirFn
	t.Cleanup(func() { UserHomeDirFn = origUserHomeDirFn })
	UserHomeDirFn = func() (string, error) { return home, nil }

	// Source tree to snapshot:
	//   <home>/source/.config/app.json     (regular file, Existed=true)
	//   <home>/source/.config/sub/        (directory, Existed=true)
	//   <home>/source/.config/link_to_sub  (symlink-directory to ./sub, Existed=true)
	srcDir := filepath.Join(home, "source")
	if err := os.MkdirAll(filepath.Join(srcDir, ".config", "sub"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.config/sub) error = %v", err)
	}
	appJSON := filepath.Join(srcDir, ".config", "app.json")
	if err := os.WriteFile(appJSON, []byte("original app.json\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(app.json) error = %v", err)
	}
	linkToSub := filepath.Join(srcDir, ".config", "link_to_sub")
	if err := os.Symlink("sub", linkToSub); err != nil {
		t.Fatalf("Symlink(link_to_sub) error = %v", err)
	}

	// Snapshot creates a compressed archive under <home>/backup-mixed-entries/.
	snapshotter := Snapshotter{now: func() time.Time { return time.Now() }}
	manifest, err := snapshotter.Create(filepath.Join(home, "backup-mixed-entries"),
		[]string{appJSON, filepath.Join(srcDir, ".config", "sub"), linkToSub})
	if err != nil {
		t.Fatalf("Snapshotter.Create() error = %v", err)
	}
	if !manifest.Compressed {
		t.Fatalf("manifest.Compressed = false, want true (Snapshotter.Create with three files)")
	}

	// Add a !Existed entry so the restore removes it from disk.
	createdBySync := filepath.Join(home, "restore_root", "created_by_sync")
	if err := os.MkdirAll(filepath.Dir(createdBySync), 0o755); err != nil {
		t.Fatalf("MkdirAll(restore_root) error = %v", err)
	}
	if err := os.WriteFile(createdBySync, []byte("created-by-sync content\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(created_by_sync) error = %v", err)
	}
	manifest.Entries = append(manifest.Entries, ManifestEntry{
		OriginalPath: createdBySync,
		SnapshotPath: "files/created_by_sync",
		Existed:      false,
		Mode:         0o644,
		Kind:         PathKindRegularFile,
	})

	// Restore via the production service.
	service := RestoreService{}
	if err := service.Restore(manifest); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	// 1. Regular file restored with original content.
	gotApp, err := os.ReadFile(appJSON)
	if err != nil {
		t.Fatalf("ReadFile(app.json) error = %v", err)
	}
	if string(gotApp) != "original app.json\n" {
		t.Errorf(".config/app.json = %q, want %q", string(gotApp), "original app.json\n")
	}

	// 2. Directory entry created/preserved.
	subInfo, err := os.Stat(filepath.Join(srcDir, ".config", "sub"))
	if err != nil {
		t.Fatalf("Stat(.config/sub) error = %v", err)
	}
	if !subInfo.IsDir() {
		t.Errorf(".config/sub is %s, want directory", subInfo.Mode())
	}

	// 3. Symlink-directory was preserved as a symlink (because it was
	// Existed=true in the snapshot). The decode2 review addressed the
	// collision-detection branch, which is not exercised here -- an
	// Existed=true symlink that already exists on disk with a matching
	// LinkTarget stays put.
	linkInfo, err := os.Lstat(linkToSub)
	if err != nil {
		t.Fatalf("Lstat(.config/link_to_sub) error = %v", err)
	}
	if linkInfo.Mode()&os.ModeSymlink == 0 {
		t.Errorf(".config/link_to_sub is %s, want symlink", linkInfo.Mode())
	}
	if tgt, _ := os.Readlink(linkToSub); tgt != "sub" {
		t.Errorf(".config/link_to_sub target = %q, want %q", tgt, "sub")
	}

	// 4. !Existed entry removed from disk.
	if _, statErr := os.Lstat(createdBySync); !os.IsNotExist(statErr) {
		t.Errorf("created_by_sync should have been removed by !Existed branch, but stat err = %v", statErr)
	}
}
