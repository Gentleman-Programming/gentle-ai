package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestManifestFileIsNonEmptyAndSorted loads the checked-in manifest and asserts
// it is non-empty, sorted, and de-duplicated.
func TestManifestFileIsNonEmptyAndSorted(t *testing.T) {
	path := filepath.Join(".", "release-core-v1.manifest")
	manifest, err := LoadReleaseManifest(path)
	if err != nil {
		t.Fatalf("LoadReleaseManifest(%q) error = %v", path, err)
	}
	if len(manifest.Journeys) == 0 {
		t.Fatal("manifest is empty; CI gate requires at least one journey ID")
	}
	// Sorted check
	for i := 1; i < len(manifest.Journeys); i++ {
		if manifest.Journeys[i-1] >= manifest.Journeys[i] {
			t.Fatalf("manifest journeys are not strictly sorted: at index %d, %q >= %q",
				i-1, manifest.Journeys[i-1], manifest.Journeys[i])
		}
	}
	// De-duplicated check (sorted implies adjacent duplicates if any)
	for i := 1; i < len(manifest.Journeys); i++ {
		if manifest.Journeys[i-1] == manifest.Journeys[i] {
			t.Fatalf("manifest contains duplicate ID: %q", manifest.Journeys[i])
		}
	}
}

// TestLoadReleaseManifestRejectsEmptyFile asserts the loader rejects a manifest
// with an empty journey list.
func TestLoadReleaseManifestRejectsEmptyFile(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "empty-manifest.yaml")
	if err := os.WriteFile(tmp, []byte("journeys: []\n"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	_, err := LoadReleaseManifest(tmp)
	if err == nil {
		t.Fatal("LoadReleaseManifest did not reject empty journeys list")
	}
}

// TestLoadReleaseManifestRejectsDuplicates asserts the loader rejects a manifest
// containing duplicate IDs even if the file is otherwise valid YAML.
func TestLoadReleaseManifestRejectsDuplicates(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "dup-manifest.yaml")
	content := `journeys:
  - j01-docs-happy-path
  - j01-docs-happy-path
  - j05-gate-without-any-review
`
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	_, err := LoadReleaseManifest(tmp)
	if err == nil {
		t.Fatal("LoadReleaseManifest did not reject duplicate IDs")
	}
}

// TestLoadReleaseManifestRejectsUnknownKeys asserts the loader rejects a manifest
// with unknown top-level keys when KnownFields strictness is enabled.
func TestLoadReleaseManifestRejectsUnknownKeys(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "unknown-key-manifest.yaml")
	content := `journeys:
  - j01-docs-happy-path
unknown_field: ["should-not-appear"]
`
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	_, err := LoadReleaseManifest(tmp)
	if err == nil {
		t.Fatal("LoadReleaseManifest did not reject unknown keys")
	}
}

// TestLoadReleaseManifestRejectsMissingFile asserts the loader returns an error
// when the manifest path does not exist.
func TestLoadReleaseManifestRejectsMissingFile(t *testing.T) {
	_, err := LoadReleaseManifest("/nonexistent/release-core-v1.manifest")
	if err == nil {
		t.Fatal("LoadReleaseManifest did not reject a missing file")
	}
}

// TestLoadReleaseManifestRejectsUnsorted asserts the loader rejects a manifest
// whose IDs are not in lexicographic order.
func TestLoadReleaseManifestRejectsUnsorted(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "unsorted-manifest.yaml")
	content := `journeys:
  - j05-gate-without-any-review
  - j01-docs-happy-path
`
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	_, err := LoadReleaseManifest(tmp)
	if err == nil {
		t.Fatal("LoadReleaseManifest did not reject unsorted IDs")
	}
}
