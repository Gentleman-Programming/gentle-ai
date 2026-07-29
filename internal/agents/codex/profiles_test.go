package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteCodexProfiles_WritesModelAndEffort asserts that WriteCodexProfiles
// creates the three gentle-ai SDD profile files, each containing BOTH model
// and model_reasoning_effort TOML keys.
func TestWriteCodexProfiles_WritesModelAndEffort(t *testing.T) {
	dir := t.TempDir()

	assignments := []ProfileAssignment{
		{Profile: "sdd-strong", Model: "gpt-5.5", ReasoningEffort: "xhigh"},
		{Profile: "sdd-mid", Model: "gpt-5.5", ReasoningEffort: "high"},
		{Profile: "sdd-cheap", Model: "gpt-5.4-mini", ReasoningEffort: "low"},
	}

	changed, files, err := WriteCodexProfiles(dir, assignments)
	if err != nil {
		t.Fatalf("WriteCodexProfiles() error = %v", err)
	}
	if !changed {
		t.Fatal("WriteCodexProfiles() changed = false, want true on first write")
	}
	if len(files) != 3 {
		t.Fatalf("WriteCodexProfiles() returned %d files, want 3", len(files))
	}

	expected := []struct {
		name            string
		model           string
		reasoningEffort string
	}{
		{"sdd-strong.config.toml", "gpt-5.5", "xhigh"},
		{"sdd-mid.config.toml", "gpt-5.5", "high"},
		{"sdd-cheap.config.toml", "gpt-5.4-mini", "low"},
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
		wantEffort := `"` + tc.reasoningEffort + `"`
		if !strings.Contains(string(content), wantEffort) {
			t.Fatalf("profile %q: want model_reasoning_effort = %s; got:\n%s", tc.name, wantEffort, content)
		}
		if !strings.Contains(string(content), `model = `) {
			t.Fatalf("profile %q missing model key; got:\n%s", tc.name, content)
		}
		wantModel := `"` + tc.model + `"`
		if !strings.Contains(string(content), wantModel) {
			t.Fatalf("profile %q: want model = %s; got:\n%s", tc.name, wantModel, content)
		}
	}
}

// TestWriteCodexProfiles_DefaultFallback asserts that nil assignments use
// canonical Recommended defaults: sdd-strong=medium, sdd-mid=high, sdd-cheap=high.
func TestWriteCodexProfiles_DefaultFallback(t *testing.T) {
	dir := t.TempDir()

	changed, _, err := WriteCodexProfiles(dir, nil)
	if err != nil {
		t.Fatalf("WriteCodexProfiles(nil) error = %v", err)
	}
	if !changed {
		t.Fatal("WriteCodexProfiles(nil) changed = false, want true on first write")
	}

	// sdd-strong must have gpt-5.6-sol and effort=medium (Recommended default)
	strong, _ := os.ReadFile(filepath.Join(dir, "sdd-strong.config.toml"))
	if !strings.Contains(string(strong), `"gpt-5.6-sol"`) {
		t.Errorf("sdd-strong default model: want gpt-5.6-sol; got:\n%s", strong)
	}
	if !strings.Contains(string(strong), `"medium"`) {
		t.Errorf("sdd-strong default effort: want medium; got:\n%s", strong)
	}
	// sdd-mid must have gpt-5.6-terra and effort=medium (Recommended default)
	mid, _ := os.ReadFile(filepath.Join(dir, "sdd-mid.config.toml"))
	if !strings.Contains(string(mid), `"gpt-5.6-terra"`) {
		t.Errorf("sdd-mid default model: want gpt-5.6-terra; got:\n%s", mid)
	}
	if !strings.Contains(string(mid), `"high"`) {
		t.Errorf("sdd-mid default effort: want high; got:\n%s", mid)
	}
	// sdd-cheap must have gpt-5.6-luna and effort=high (Recommended default)
	cheap, _ := os.ReadFile(filepath.Join(dir, "sdd-cheap.config.toml"))
	if !strings.Contains(string(cheap), `"gpt-5.6-luna"`) {
		t.Errorf("sdd-cheap default model: want gpt-5.6-luna; got:\n%s", cheap)
	}
	if !strings.Contains(string(cheap), `"high"`) {
		t.Errorf("sdd-cheap default effort: want high; got:\n%s", cheap)
	}
}

// TestWriteCodexProfiles_Idempotent asserts that running WriteCodexProfiles
// twice with the same assignments produces no change on the second run and
// does not duplicate keys.
func TestWriteCodexProfiles_Idempotent(t *testing.T) {
	dir := t.TempDir()

	assignments := []ProfileAssignment{
		{Profile: "sdd-strong", Model: "gpt-5.5", ReasoningEffort: "high"},
		{Profile: "sdd-mid", Model: "gpt-5.5", ReasoningEffort: "medium"},
		{Profile: "sdd-cheap", Model: "gpt-5.4-mini", ReasoningEffort: "low"},
	}

	_, _, err := WriteCodexProfiles(dir, assignments)
	if err != nil {
		t.Fatalf("first WriteCodexProfiles() error = %v", err)
	}

	changed, _, err := WriteCodexProfiles(dir, assignments)
	if err != nil {
		t.Fatalf("second WriteCodexProfiles() error = %v", err)
	}
	if changed {
		t.Fatal("WriteCodexProfiles() changed = true on second run, want false (idempotent)")
	}

	// Verify no duplicate keys.
	for _, name := range []string{"sdd-strong.config.toml", "sdd-mid.config.toml", "sdd-cheap.config.toml"} {
		content, readErr := os.ReadFile(filepath.Join(dir, name))
		if readErr != nil {
			t.Fatalf("profile %q missing after second run: %v", name, readErr)
		}
		countEffort := strings.Count(string(content), "model_reasoning_effort")
		if countEffort != 1 {
			t.Fatalf("profile %q: expected 1 model_reasoning_effort key, got %d; content:\n%s", name, countEffort, content)
		}
		countModel := strings.Count(string(content), `model = `)
		if countModel != 1 {
			t.Fatalf("profile %q: expected 1 model key, got %d; content:\n%s", name, countModel, content)
		}
	}
}

// TestWriteCodexProfiles_ReturnsChangedPathsOnFirstWrite asserts that the
// returned changed flag is true and files slice has all 3 paths on first write.
func TestWriteCodexProfiles_ReturnsChangedPathsOnFirstWrite(t *testing.T) {
	dir := t.TempDir()

	changed, files, err := WriteCodexProfiles(dir, nil)
	if err != nil {
		t.Fatalf("WriteCodexProfiles() error = %v", err)
	}
	if !changed {
		t.Fatal("WriteCodexProfiles() changed = false on first write")
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
