package backup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// overrideHomeForBackup swaps UserHomeDirFn and BackupRootFn to dir,
// restoring the originals at test cleanup. Mirrors the inline pattern in
// restore_test.go but factored out so this file's tests stay readable.
func overrideHomeForBackup(t *testing.T, dir string) {
	t.Helper()
	origHome := UserHomeDirFn
	origRoot := BackupRootFn
	UserHomeDirFn = func() (string, error) { return dir, nil }
	BackupRootFn = func() (string, error) { return dir, nil }
	t.Cleanup(func() {
		UserHomeDirFn = origHome
		BackupRootFn = origRoot
	})
}

func newSnapshotter() Snapshotter {
	return Snapshotter{now: func() time.Time { return time.Now() }}
}

// TestSnapshotterClassifiesPreExistingDirectory verifies that a pre-existing
// empty directory is snapshotted with Kind=directory, Existed=true, and
// restore does not remove it (acceptance criterion 2).
func TestSnapshotterClassifiesPreExistingDirectory(t *testing.T) {
	home := t.TempDir()
	overrideHomeForBackup(t, home)
	dir := filepath.Join(home, "existing-dir")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	m, err := newSnapshotter().Create(filepath.Join(home, "backup"), []string{dir})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got := len(m.Entries); got != 1 {
		t.Fatalf("entries = %d, want 1", got)
	}
	e := m.Entries[0]
	if e.Kind != PathKindDirectory || !e.Existed {
		t.Fatalf("entry = %+v, want Kind=directory Existed=true", e)
	}
	if err := os.Remove(dir); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := (RestoreService{}).Restore(m); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected dir to remain after restore")
	}
}

// TestSnapshotterClassifiesPreExistingSymlinkDirectory verifies that a
// pre-existing symlink-to-directory is snapshotted with Kind=symlink_directory
// and LinkTarget set, and restore recreates the symlink if missing
// (acceptance criterion 3).
func TestSnapshotterClassifiesPreExistingSymlinkDirectory(t *testing.T) {
	home := t.TempDir()
	overrideHomeForBackup(t, home)
	dir := filepath.Join(home, "existing-dir")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	link := filepath.Join(home, "existing-dir-link")
	if err := os.Symlink("existing-dir", link); err != nil {
		t.Skipf("Symlink not available: %v", err)
	}
	m, err := newSnapshotter().Create(filepath.Join(home, "backup"), []string{dir, link})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	var se ManifestEntry
	for _, e := range m.Entries {
		if e.OriginalPath == link {
			se = e
			break
		}
	}
	if se.OriginalPath == "" {
		t.Fatalf("symlink entry not found")
	}
	if se.Kind != PathKindSymlinkDirectory || se.LinkTarget != "existing-dir" || !se.Existed {
		t.Fatalf("symlink entry = %+v, want Kind=symlink_directory LinkTarget=existing-dir Existed=true", se)
	}
	if err := os.Remove(link); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := (RestoreService{}).Restore(m); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected symlink after restore")
	}
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if target != "existing-dir" {
		t.Fatalf("target = %q, want existing-dir", target)
	}
}

// TestRestoreRejectsUnsafeSymlinkTarget covers acceptance criterion 3:
// absolute and parent-traversal targets in a manifest's LinkTarget are
// refused by restore, so a tampered manifest cannot point outside the
// user home (covers both rejection paths via table-driven subtests).
func TestRestoreRejectsUnsafeSymlinkTarget(t *testing.T) {
	home := t.TempDir()
	overrideHomeForBackup(t, home)
	cases := []struct {
		name   string
		target string
		errMsg string
	}{
		{"absolute", "/etc/passwd", "absolute targets are not allowed"},
		{"parent-traversal", "../etc/passwd", ".."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := (RestoreService{}).Restore(Manifest{
				Compressed: false,
				Entries: []ManifestEntry{{
					OriginalPath: filepath.Join(home, "link"),
					Existed:      true,
					Kind:         PathKindSymlinkDirectory,
					LinkTarget:   tc.target,
					Mode:         uint32(os.ModeSymlink | 0o777),
				}},
			})
			if err == nil {
				t.Fatal("Restore() expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.errMsg) {
				t.Fatalf("error = %q, want to contain %q", err.Error(), tc.errMsg)
			}
		})
	}
}

// TestLegacyManifestWithUnknownKindDoesNotDelete covers acceptance
// criterion 4: a manifest entry with Kind=PathKindUnknown (the legacy
// zero value, written before PathKind existed) and Existed=false does
// NOT delete the file at OriginalPath — the safe-compat policy.
func TestLegacyManifestWithUnknownKindDoesNotDelete(t *testing.T) {
	home := t.TempDir()
	overrideHomeForBackup(t, home)
	keepFile := filepath.Join(home, "pre-existing", "keepme.txt")
	if err := os.MkdirAll(filepath.Dir(keepFile), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	const body = "i was here before the backup\n"
	if err := os.WriteFile(keepFile, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := (RestoreService{}).Restore(Manifest{
		Compressed: false,
		Entries: []ManifestEntry{{
			OriginalPath: keepFile,
			Existed:      false,
			Kind:         PathKindUnknown,
		}},
	}); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	got, err := os.ReadFile(keepFile)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != body {
		t.Fatalf("content = %q, want %q", got, body)
	}
}

// TestRestoreRejectsExistingOutsideRootSymlink verifies that restore refuses
// a pre-existing symlink that resolves outside the allowed root without
// modifying the symlink.
func TestRestoreRejectsExistingOutsideRootSymlink(t *testing.T) {
	home := t.TempDir()
	overrideHomeForBackup(t, home)
	outside := t.TempDir()

	link := filepath.Join(home, "pre-existing-link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("Symlink not available: %v", err)
	}
	beforeTarget, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink before restore: %v", err)
	}

	err = (RestoreService{}).Restore(Manifest{
		Compressed: false,
		Entries: []ManifestEntry{{
			OriginalPath: link,
			Existed:      true,
			Kind:         PathKindSymlinkDirectory,
			LinkTarget:   filepath.Base(outside),
			Mode:         uint32(os.ModeSymlink | 0o777),
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid OriginalPath") {
		t.Fatalf("Restore() error = %v, want invalid OriginalPath refusal", err)
	}

	gotTarget, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if gotTarget != beforeTarget {
		t.Fatalf("Readlink target = %q, want %q", gotTarget, beforeTarget)
	}
}

// TestRestoreCreatesMissingParentDirForSymlink covers the follow-up:
// when a symlink needs recreation and its parent directory does not
// exist, restore must create the parent before calling os.Symlink —
// otherwise a fresh restore whose parent was cleaned up aborts with
// ENOENT from os.Symlink.
func TestRestoreCreatesMissingParentDirForSymlink(t *testing.T) {
	home := t.TempDir()
	overrideHomeForBackup(t, home)

	// Pre-existing target at home/target-dir; the symlink's own parent
	// home/missing-parent does NOT exist. Manifest LinkTarget points
	// back up one level to the target, which keeps it inside the
	// allowed roots after resolution.
	if err := os.MkdirAll(filepath.Join(home, "target-dir"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	link := filepath.Join(home, "missing-parent", "link")

	// Probe symlink availability on this platform before driving the
	// actual scenario — restore's recreation branch calls os.Symlink.
	probe := filepath.Join(home, ".probe-symlink")
	if err := os.Symlink("x", probe); err != nil {
		t.Skipf("Symlink not available: %v", err)
	}
	os.Remove(probe)

	err := (RestoreService{}).Restore(Manifest{
		Compressed: false,
		Entries: []ManifestEntry{{
			OriginalPath: link,
			Existed:      true,
			Kind:         PathKindSymlinkDirectory,
			LinkTarget:   filepath.Join("..", "target-dir"),
			Mode:         uint32(os.ModeSymlink | 0o777),
		}},
	})
	if err != nil {
		t.Fatalf("Restore() error = %v, want nil when missing parent dir can be created", err)
	}

	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected symlink at %q, got mode %v", link, info.Mode())
	}
	if got, err := os.Readlink(link); err != nil {
		t.Fatalf("Readlink: %v", err)
	} else if got != filepath.Join("..", "target-dir") {
		t.Fatalf("Readlink target = %q, want %q", got, filepath.Join("..", "target-dir"))
	}
}
