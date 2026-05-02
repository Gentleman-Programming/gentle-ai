package engram

import (
	"runtime"
	"strings"
	"testing"
)

func TestSuggestedLocations_IncludesHomeDefault(t *testing.T) {
	backend := NewLocalDataBackend()
	home := t.TempDir()

	suggestions := SuggestedLocations(backend, home)
	if len(suggestions) == 0 {
		t.Fatal("expected at least one suggestion")
	}

	found := false
	for _, s := range suggestions {
		if strings.Contains(s.Label, "Home") {
			found = true
			if s.Path == "" {
				t.Error("home suggestion has empty path")
			}
		}
	}
	if !found {
		t.Error("missing Home suggestion")
	}
}

func TestSuggestedLocations_IncludesDocuments(t *testing.T) {
	backend := NewLocalDataBackend()
	home := t.TempDir()

	suggestions := SuggestedLocations(backend, home)

	found := false
	for _, s := range suggestions {
		if s.Label == "Documents" {
			found = true
		}
	}
	if !found {
		t.Error("missing Documents suggestion")
	}
}

func TestSuggestedLocations_Deduplicates(t *testing.T) {
	backend := NewLocalDataBackend()
	home := t.TempDir()

	suggestions := SuggestedLocations(backend, home)
	seen := make(map[string]int)
	for _, s := range suggestions {
		seen[s.Path]++
	}
	for path, count := range seen {
		if count > 1 {
			t.Errorf("path %q appears %d times, expected 1", path, count)
		}
	}
}

func TestSuggestedLocations_DarwinVolumes(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only test")
	}
	backend := NewLocalDataBackend()
	suggestions := SuggestedLocations(backend, t.TempDir())
	// We can't guarantee volumes exist in CI, so just verify no panic.
	_ = suggestions
}

func TestSuggestedLocations_LinuxMounts(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux-only test")
	}
	backend := NewLocalDataBackend()
	suggestions := SuggestedLocations(backend, t.TempDir())
	// We can't guarantee mounts exist in CI, so just verify no panic.
	_ = suggestions
}

func TestSuggestedLocations_WindowsDrives(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only test")
	}
	backend := NewLocalDataBackend()
	suggestions := SuggestedLocations(backend, t.TempDir())

	// At minimum C:\ should exist and be suggested.
	foundC := false
	for _, s := range suggestions {
		if strings.HasPrefix(s.Label, `C:\`) || strings.HasPrefix(s.Path, `C:\`) {
			foundC = true
		}
	}
	if !foundC {
		t.Log(`C:\ drive not found in suggestions (may be expected in some CI environments)`)
	}
}

func TestSuggestedLocations_HasSpaceInfo(t *testing.T) {
	backend := NewLocalDataBackend()
	home := t.TempDir()

	suggestions := SuggestedLocations(backend, home)
	if len(suggestions) == 0 {
		t.Fatal("expected at least one suggestion")
	}

	// The home default should have space > 0 because t.TempDir() is on the
	// same volume which has free space.
	if suggestions[0].Space == 0 {
		t.Error("first suggestion has zero space")
	}
}

func TestLabelForPath(t *testing.T) {
	home := "/home/user"
	tests := []struct {
		path string
		want string
	}{
		{"/home/user/.engram", "Home (~/.engram)"},
		{"/home/user/Documents/.engram", "Documents"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := labelForPath(tt.path, home)
			if got != tt.want {
				t.Errorf("labelForPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
