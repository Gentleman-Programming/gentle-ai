package openrecord

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeManifestFile(t *testing.T, dir string, entries []manifestEntry) {
	t.Helper()
	m := manifest{Version: manifestVersion, Emitter: "openrecord@test", WithQmd: true, Entries: entries}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, manifestName), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestPruneStaleEntries covers the three documented cases: a stale entry
// whose on-disk content still matches its recorded hash is removed; a stale
// entry that was locally edited is kept; an absent or corrupt manifest is
// treated as empty (no deletions, no crash).
func TestPruneStaleEntries(t *testing.T) {
	t.Run("stale entry with matching hash is removed", func(t *testing.T) {
		dir := t.TempDir()
		content := []byte("stale skill body")
		if err := os.WriteFile(filepath.Join(dir, "stale.md"), content, 0o644); err != nil {
			t.Fatal(err)
		}
		writeManifestFile(t, dir, []manifestEntry{{Path: "stale.md", Hash: hashContent(content)}})

		if err := pruneStaleEntries(dir, manifest{}); err != nil {
			t.Fatalf("pruneStaleEntries() error = %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "stale.md")); !os.IsNotExist(err) {
			t.Fatalf("stale.md was not removed: err = %v", err)
		}
	})

	t.Run("stale entry with a changed hash is kept", func(t *testing.T) {
		dir := t.TempDir()
		originalHash := hashContent([]byte("original body"))
		editedContent := []byte("locally edited body")
		if err := os.WriteFile(filepath.Join(dir, "edited.md"), editedContent, 0o644); err != nil {
			t.Fatal(err)
		}
		writeManifestFile(t, dir, []manifestEntry{{Path: "edited.md", Hash: originalHash}})

		if err := pruneStaleEntries(dir, manifest{}); err != nil {
			t.Fatalf("pruneStaleEntries() error = %v", err)
		}
		got, err := os.ReadFile(filepath.Join(dir, "edited.md"))
		if err != nil {
			t.Fatalf("edited.md was removed despite local edits: %v", err)
		}
		if string(got) != string(editedContent) {
			t.Fatalf("edited.md content changed: got %q", got)
		}
	})

	t.Run("an entry still shipped is never pruned", func(t *testing.T) {
		dir := t.TempDir()
		content := []byte("still shipped")
		if err := os.WriteFile(filepath.Join(dir, "kept.md"), content, 0o644); err != nil {
			t.Fatal(err)
		}
		hash := hashContent(content)
		writeManifestFile(t, dir, []manifestEntry{{Path: "kept.md", Hash: hash}})

		shipped := manifest{Entries: []manifestEntry{{Path: "kept.md", Hash: hash}}}
		if err := pruneStaleEntries(dir, shipped); err != nil {
			t.Fatalf("pruneStaleEntries() error = %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "kept.md")); err != nil {
			t.Fatalf("kept.md was removed despite still being shipped: %v", err)
		}
	})

	t.Run("no manifest or corrupt manifest is treated as empty", func(t *testing.T) {
		dir := t.TempDir()
		if err := pruneStaleEntries(dir, manifest{}); err != nil {
			t.Fatalf("pruneStaleEntries() with no manifest error = %v", err)
		}

		if err := os.WriteFile(filepath.Join(dir, manifestName), []byte("{not json"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := pruneStaleEntries(dir, manifest{}); err != nil {
			t.Fatalf("pruneStaleEntries() with a corrupt manifest error = %v", err)
		}
	})
}

// TestLoadManifestUnrecognisedVersion pins that a manifest this build does
// not understand reads as empty rather than misread, matching openrecord's
// own tolerance.
func TestLoadManifestUnrecognisedVersion(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, manifestName), []byte(`{"version":99,"entries":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := loadManifest(dir); len(got.Entries) != 0 || got.Version != 0 {
		t.Fatalf("loadManifest() = %+v, want the zero value for an unrecognised version", got)
	}
}

func TestEmittedPaths(t *testing.T) {
	dir := t.TempDir()
	writeManifestFile(t, dir, []manifestEntry{{Path: "a/SKILL.md", Hash: "x"}, {Path: "b/SKILL.md", Hash: "y"}})

	got := EmittedPaths(dir)
	if len(got) != 2 || got[0] != "a/SKILL.md" || got[1] != "b/SKILL.md" {
		t.Fatalf("EmittedPaths() = %v, want [a/SKILL.md b/SKILL.md]", got)
	}

	if got := EmittedPaths(t.TempDir()); len(got) != 0 {
		t.Fatalf("EmittedPaths() on an empty dir = %v, want none", got)
	}
}
