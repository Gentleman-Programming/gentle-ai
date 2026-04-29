package sdd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestReadCurrentModelAssignments(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "opencode.json")

	content := `{
  "agent": {
    "sdd-orchestrator": { "model": "anthropic:claude-sonnet-4-20250514" },
    "sdd-apply": { "model": "openai:gpt-4o" },
    "sdd-verify": { "model": "anthropic:claude-haiku-3-20240307" },
    "gentleman": { "model": "anthropic:claude-sonnet-4-20250514" }
  }
}`
	if err := os.WriteFile(settingsPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	got, err := ReadCurrentModelAssignments(settingsPath)
	if err != nil {
		t.Fatalf("ReadCurrentModelAssignments() error = %v", err)
	}

	tests := []struct {
		phase      string
		providerID string
		modelID    string
	}{
		{"sdd-orchestrator", "anthropic", "claude-sonnet-4-20250514"},
		{"sdd-apply", "openai", "gpt-4o"},
		{"sdd-verify", "anthropic", "claude-haiku-3-20240307"},
	}

	for _, tt := range tests {
		a, ok := got[tt.phase]
		if !ok {
			t.Errorf("phase %q missing from result", tt.phase)
			continue
		}
		if a.ProviderID != tt.providerID {
			t.Errorf("phase %q: ProviderID = %q, want %q", tt.phase, a.ProviderID, tt.providerID)
		}
		if a.ModelID != tt.modelID {
			t.Errorf("phase %q: ModelID = %q, want %q", tt.phase, a.ModelID, tt.modelID)
		}
	}

	// "gentleman" is not an SDD phase — it should NOT be in the result
	if _, ok := got["gentleman"]; ok {
		t.Error("non-SDD agent 'gentleman' should not be in result")
	}
}

func TestReadCurrentModelAssignmentsNoFile(t *testing.T) {
	got, err := ReadCurrentModelAssignments("/nonexistent/path/opencode.json")
	if err != nil {
		t.Fatalf("ReadCurrentModelAssignments() with missing file returned error = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("ReadCurrentModelAssignments() with missing file returned %d entries, want 0", len(got))
	}
}

func TestReadCurrentModelAssignmentsNoAgents(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "opencode.json")

	content := `{"model": "anthropic:claude-sonnet-4-20250514", "theme": "dark"}`
	if err := os.WriteFile(settingsPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	got, err := ReadCurrentModelAssignments(settingsPath)
	if err != nil {
		t.Fatalf("ReadCurrentModelAssignments() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ReadCurrentModelAssignments() = %v, want empty map", got)
	}
}

func TestReadCurrentModelAssignmentsPartialModels(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "opencode.json")

	// Some agents have model, some don't
	content := `{
  "agent": {
    "sdd-orchestrator": { "model": "anthropic:claude-opus-4-5" },
    "sdd-apply": { "prompt": "You are a coder" },
    "sdd-verify": {}
  }
}`
	if err := os.WriteFile(settingsPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	got, err := ReadCurrentModelAssignments(settingsPath)
	if err != nil {
		t.Fatalf("ReadCurrentModelAssignments() error = %v", err)
	}

	// Only sdd-orchestrator has a model — only it should appear
	if len(got) != 1 {
		t.Errorf("ReadCurrentModelAssignments() len = %d, want 1; got %v", len(got), got)
	}

	a, ok := got["sdd-orchestrator"]
	if !ok {
		t.Fatal("sdd-orchestrator missing from result")
	}
	want := model.ModelAssignment{ProviderID: "anthropic", ModelID: "claude-opus-4-5"}
	if a != want {
		t.Errorf("sdd-orchestrator assignment = %+v, want %+v", a, want)
	}
}

func TestReadCurrentModelAssignmentsMalformedModelField(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "opencode.json")

	// Model without colon — should be skipped without error
	content := `{
  "agent": {
    "sdd-orchestrator": { "model": "no-colon-here" },
    "sdd-apply": { "model": "anthropic:claude-sonnet-4-20250514" }
  }
}`
	if err := os.WriteFile(settingsPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	got, err := ReadCurrentModelAssignments(settingsPath)
	if err != nil {
		t.Fatalf("ReadCurrentModelAssignments() error = %v", err)
	}

	// Malformed sdd-orchestrator skipped, sdd-apply parsed
	if _, ok := got["sdd-orchestrator"]; ok {
		t.Error("malformed model 'no-colon-here' should be skipped")
	}
	a, ok := got["sdd-apply"]
	if !ok {
		t.Fatal("sdd-apply missing from result")
	}
	if a.ProviderID != "anthropic" || a.ModelID != "claude-sonnet-4-20250514" {
		t.Errorf("sdd-apply = %+v, want {anthropic, claude-sonnet-4-20250514}", a)
	}
}

// TestReadCurrentModelAssignmentsSlashSeparator verifies that custom provider
// models using slash format ("provider/model-id") are parsed correctly.
// Issue #152: OpenCode uses "zai-coding-plan/glm-5-turbo" for custom providers.
func TestReadCurrentModelAssignmentsSlashSeparator(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "opencode.json")

	content := `{
  "agent": {
    "sdd-orchestrator": { "model": "zai-coding-plan/glm-5-turbo" }
  }
}`
	if err := os.WriteFile(settingsPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	got, err := ReadCurrentModelAssignments(settingsPath)
	if err != nil {
		t.Fatalf("ReadCurrentModelAssignments() error = %v", err)
	}

	a, ok := got["sdd-orchestrator"]
	if !ok {
		t.Fatal("sdd-orchestrator missing from result — slash-separated format not parsed")
	}
	if a.ProviderID != "zai-coding-plan" {
		t.Errorf("ProviderID = %q, want %q", a.ProviderID, "zai-coding-plan")
	}
	if a.ModelID != "glm-5-turbo" {
		t.Errorf("ModelID = %q, want %q", a.ModelID, "glm-5-turbo")
	}
}

// TestReadCurrentModelAssignmentsMixedSeparators verifies that a file containing
// agents with both colon and slash separators is parsed correctly (issue #152).
func TestReadCurrentModelAssignmentsMixedSeparators(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "opencode.json")

	content := `{
  "agent": {
    "sdd-orchestrator": { "model": "anthropic:claude-sonnet-4-20250514" },
    "sdd-apply":        { "model": "zai-coding-plan/glm-5-turbo" },
    "sdd-verify":       { "model": "openai:gpt-4o" },
    "sdd-explore":      { "model": "custom-provider/some-model-v2" }
  }
}`
	if err := os.WriteFile(settingsPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	got, err := ReadCurrentModelAssignments(settingsPath)
	if err != nil {
		t.Fatalf("ReadCurrentModelAssignments() error = %v", err)
	}

	tests := []struct {
		phase      string
		providerID string
		modelID    string
	}{
		{"sdd-orchestrator", "anthropic", "claude-sonnet-4-20250514"},
		{"sdd-apply", "zai-coding-plan", "glm-5-turbo"},
		{"sdd-verify", "openai", "gpt-4o"},
		{"sdd-explore", "custom-provider", "some-model-v2"},
	}

	for _, tt := range tests {
		a, ok := got[tt.phase]
		if !ok {
			t.Errorf("phase %q missing from result", tt.phase)
			continue
		}
		if a.ProviderID != tt.providerID {
			t.Errorf("phase %q: ProviderID = %q, want %q", tt.phase, a.ProviderID, tt.providerID)
		}
		if a.ModelID != tt.modelID {
			t.Errorf("phase %q: ModelID = %q, want %q", tt.phase, a.ModelID, tt.modelID)
		}
	}
}

func TestReadCurrentModelAssignmentsDoesNotFabricateFromRootModel(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "opencode.json")

	content := `{"model":"openai/gpt-5","agent":{"gentleman":{"model":"openai/gpt-5"}}}`
	if err := os.WriteFile(settingsPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	got, err := ReadCurrentModelAssignments(settingsPath)
	if err != nil {
		t.Fatalf("ReadCurrentModelAssignments() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("ReadCurrentModelAssignments() fabricated SDD assignments from root model: %v", got)
	}
}
