package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteCodexProfiles_Written asserts that WriteCodexProfiles creates the
// three gentle-ai SDD profile files in the given Codex home directory, each
// containing a non-empty model_reasoning_effort key.
func TestWriteCodexProfiles_Written(t *testing.T) {
	dir := t.TempDir()

	changed, files, err := WriteCodexProfiles(dir)
	if err != nil {
		t.Fatalf("WriteCodexProfiles() error = %v", err)
	}
	if !changed {
		t.Fatal("WriteCodexProfiles() changed = false, want true on first write")
	}

	expected := []struct {
		name            string
		reasoningEffort string
	}{
		{"sdd-strong.config.toml", "xhigh"},
		{"sdd-mid.config.toml", "high"},
		{"sdd-cheap.config.toml", "low"},
	}

	if len(files) != len(expected) {
		t.Fatalf("WriteCodexProfiles() returned %d files, want %d", len(files), len(expected))
	}

	for _, tc := range expected {
		path := filepath.Join(dir, tc.name)
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("profile file %q not created: %v", tc.name, readErr)
		}
		if !strings.Contains(string(content), `model_reasoning_effort`) {
			t.Fatalf("profile %q missing model_reasoning_effort key; got:\n%s", tc.name, content)
		}
		want := `"` + tc.reasoningEffort + `"`
		if !strings.Contains(string(content), want) {
			t.Fatalf("profile %q: want model_reasoning_effort = %s; got:\n%s", tc.name, want, content)
		}
	}
}

// TestWriteCodexProfiles_Idempotent asserts that running WriteCodexProfiles
// twice produces no change on the second run and does not duplicate keys.
func TestWriteCodexProfiles_Idempotent(t *testing.T) {
	dir := t.TempDir()

	_, _, err := WriteCodexProfiles(dir)
	if err != nil {
		t.Fatalf("first WriteCodexProfiles() error = %v", err)
	}

	changed, _, err := WriteCodexProfiles(dir)
	if err != nil {
		t.Fatalf("second WriteCodexProfiles() error = %v", err)
	}
	if changed {
		t.Fatal("WriteCodexProfiles() changed = true on second run, want false (idempotent)")
	}

	// Verify no duplicate keys by checking the count.
	for _, name := range []string{"sdd-strong.config.toml", "sdd-mid.config.toml", "sdd-cheap.config.toml"} {
		content, readErr := os.ReadFile(filepath.Join(dir, name))
		if readErr != nil {
			t.Fatalf("profile %q missing after second run: %v", name, readErr)
		}
		count := strings.Count(string(content), "model_reasoning_effort")
		if count != 1 {
			t.Fatalf("profile %q: expected 1 model_reasoning_effort key, got %d; content:\n%s", name, count, content)
		}
	}
}

// TestWriteCodexProfiles_ReturnsAllThreePaths asserts that the returned files
// slice contains paths for all three profile files.
func TestWriteCodexProfiles_ReturnsAllThreePaths(t *testing.T) {
	dir := t.TempDir()

	_, files, err := WriteCodexProfiles(dir)
	if err != nil {
		t.Fatalf("WriteCodexProfiles() error = %v", err)
	}

	wantNames := []string{"sdd-strong.config.toml", "sdd-mid.config.toml", "sdd-cheap.config.toml"}
	for _, name := range wantNames {
		want := filepath.Join(dir, name)
		found := false
		for _, f := range files {
			if f == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("WriteCodexProfiles() files does not contain %q; got %v", want, files)
		}
	}
}
