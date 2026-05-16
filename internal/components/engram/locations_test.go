package engram

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSuggestLocations_DefaultFirst(t *testing.T) {
	home := t.TempDir()

	locs := SuggestLocations(home, "")
	if len(locs) == 0 {
		t.Fatal("SuggestLocations returned no locations")
	}
	// Without a current dir the default should be first.
	want := filepath.Clean(DefaultDir(home))
	if locs[0].Path != want {
		t.Errorf("first location = %q, want default %q", locs[0].Path, want)
	}
}

func TestSuggestLocations_CurrentFirst(t *testing.T) {
	home := t.TempDir()
	current := filepath.Join(home, "custom-engram")

	locs := SuggestLocations(home, current)
	if len(locs) == 0 {
		t.Fatal("SuggestLocations returned no locations")
	}
	if locs[0].Path != filepath.Clean(current) {
		t.Errorf("first location = %q, want current %q", locs[0].Path, current)
	}
	if !locs[0].IsCurrent {
		t.Error("first location IsCurrent should be true")
	}
}

func TestSuggestLocations_DefaultNotCurrentWhenCustomDirSet(t *testing.T) {
	home := t.TempDir()
	custom := filepath.Join(home, "custom-engram")

	locs := SuggestLocations(home, custom)

	defaultPath := filepath.Clean(DefaultDir(home))
	for _, loc := range locs {
		if loc.Path == defaultPath && loc.IsCurrent {
			t.Errorf("default dir %q should NOT be IsCurrent when a custom dir is set", defaultPath)
		}
	}
}

func TestSuggestLocations_NoDuplicates(t *testing.T) {
	home := t.TempDir()
	// When current == default, we should get exactly one entry for that path.
	defaultDir := DefaultDir(home)
	locs := SuggestLocations(home, defaultDir)

	seen := map[string]int{}
	for _, l := range locs {
		seen[l.Path]++
	}
	for path, count := range seen {
		if count > 1 {
			t.Errorf("path %q appears %d times, want 1", path, count)
		}
	}
}

func TestSuggestLocationsWithKnown_IncludesKnownAndMarksActive(t *testing.T) {
	home := t.TempDir()
	current := filepath.Join(home, "copied-engram")
	known := []string{current, filepath.Join(home, "archive-engram")}

	locs := SuggestLocationsWithKnown(home, current, known)

	if locs[0].Path != filepath.Clean(current) || !locs[0].IsCurrent {
		t.Fatalf("first location = %+v, want current known dir marked active", locs[0])
	}
	foundArchive := false
	for _, loc := range locs {
		if loc.Path == filepath.Clean(known[1]) {
			foundArchive = true
		}
	}
	if !foundArchive {
		t.Fatalf("known non-active dir %q missing from suggestions: %+v", known[1], locs)
	}
}

func TestSuggestLocationsWithKnown_MarksNonEmptyDirsAsHasData(t *testing.T) {
	home := t.TempDir()
	found := filepath.Join(home, "found-engram")
	if err := os.MkdirAll(found, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(found, "data.bin"), []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	locs := SuggestLocationsWithKnown(home, "", []string{found})

	for _, loc := range locs {
		if loc.Path == filepath.Clean(found) {
			if !loc.HasData {
				t.Fatalf("known dir with content should be marked HasData")
			}
			return
		}
	}
	t.Fatalf("known dir %q missing from suggestions: %+v", found, locs)
}

func TestBuildLabel_HomePrefix(t *testing.T) {
	home := "/home/user"
	path := filepath.Join(home, ".engram")
	label := buildLabel(path, home, -1)
	if label != filepath.Join("~", ".engram") {
		t.Errorf("buildLabel = %q, want %q", label, filepath.Join("~", ".engram"))
	}
}

func TestBuildLabel_WithFreeSpace(t *testing.T) {
	home := "/home/user"
	path := filepath.Join(home, ".engram")
	label := buildLabel(path, home, 1024*1024*1024) // 1 GiB
	want := filepath.Join("~", ".engram") + "  (1.0 GiB free)"
	if label != want {
		t.Errorf("buildLabel = %q, want %q", label, want)
	}
}

func TestBuildLabel_NoHomePrefix(t *testing.T) {
	home := "/home/user"
	path := "/external/drive/engram"
	label := buildLabel(path, home, -1)
	if label != path {
		t.Errorf("buildLabel = %q, want %q", label, path)
	}
}
