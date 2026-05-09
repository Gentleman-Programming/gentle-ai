package engram

import (
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
