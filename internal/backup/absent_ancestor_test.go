package backup

import (
	"os"
	"path/filepath"
	"testing"
)

// A regular user file where a planned path expects a directory means nothing
// can exist below it. Snapshot and restore must treat those paths as absent
// and leave the blocking file alone instead of failing the whole operation.

// writeBlockedTargets creates root/.claude/gentle-ai as a user regular file and
// returns planned paths below it, a module and the ledger name.
func writeBlockedTargets(t *testing.T, root string) (blocker string, targets []string) {
	t.Helper()
	blocker = filepath.Join(root, ".claude", "gentle-ai")
	if err := os.MkdirAll(filepath.Dir(blocker), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blocker, []byte("user file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	moduleDir := filepath.Join(blocker, "orchestrator")
	return blocker, []string{
		filepath.Join(moduleDir, "orchestrator-delegation.md"),
		filepath.Join(moduleDir, ".gentle-ai-orchestrator-module-ownership.json"),
	}
}

func assertFileBytes(t *testing.T, path, want string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("%s is no longer a regular file: %v", path, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != want {
		t.Fatalf("%s = %q, %v; want %q", path, got, err, want)
	}
}

func TestSnapshotAndRestoreTreatRegularFileAncestorAsAbsence(t *testing.T) {
	for _, tc := range []struct {
		name      string
		withCore  bool
		wantCompr bool
	}{
		{name: "plain", withCore: false, wantCompr: false},
		{name: "compressed", withCore: true, wantCompr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			blocker, targets := writeBlockedTargets(t, root)
			core := filepath.Join(root, ".claude", "CLAUDE.md")
			paths := targets
			if tc.withCore {
				if err := os.WriteFile(core, []byte("core\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				paths = append([]string{core}, targets...)
			}

			manifest, err := NewSnapshotter().Create(filepath.Join(t.TempDir(), "snap"), paths)
			if err != nil {
				t.Fatalf("Create() error = %v; want paths below a regular file recorded as absent", err)
			}
			if manifest.Compressed != tc.wantCompr || len(manifest.Entries) != len(paths) {
				t.Fatalf("manifest compressed=%v entries=%d; want %v and %d", manifest.Compressed, len(manifest.Entries), tc.wantCompr, len(paths))
			}
			for _, entry := range manifest.Entries[len(paths)-len(targets):] {
				if entry.Existed || entry.Kind != PathKindRegularFile {
					t.Fatalf("entry %s existed=%v kind=%q; want an absent regular file target", entry.OriginalPath, entry.Existed, entry.Kind)
				}
			}

			if err := (RestoreService{Roots: []string{root}}).Restore(manifest); err != nil {
				t.Fatalf("Restore() error = %v; want absent targets below a regular file to be a no-op", err)
			}
			assertFileBytes(t, blocker, "user file\n")
			if tc.withCore {
				assertFileBytes(t, core, "core\n")
			}
		})
	}
}

// TestRestoreAbsentEntryUnderRegularFileAncestorIsNoop isolates the restore
// side from snapshot creation with a manifest recording one absent target.
func TestRestoreAbsentEntryUnderRegularFileAncestorIsNoop(t *testing.T) {
	root := t.TempDir()
	blocker, targets := writeBlockedTargets(t, root)
	manifest := Manifest{Entries: []ManifestEntry{{OriginalPath: targets[0], Kind: PathKindRegularFile}}}

	if err := (RestoreService{Roots: []string{root}}).Restore(manifest); err != nil {
		t.Fatalf("Restore() error = %v; want a no-op", err)
	}
	assertFileBytes(t, blocker, "user file\n")
}
