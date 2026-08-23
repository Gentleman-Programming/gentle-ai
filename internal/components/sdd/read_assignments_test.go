package sdd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

func TestReadCurrentModelAssignments(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "opencode.json")

	content := `{
  "agent": {
    "gentle-orchestrator": { "model": "anthropic:claude-sonnet-4-20250514" },
    "sdd-apply": { "model": "openai:gpt-4o" },
    "sdd-verify": { "model": "anthropic:claude-haiku-3-20240307" },
    "some-other-agent": { "model": "anthropic:claude-sonnet-4-20250514" }
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
		{"gentle-orchestrator", "anthropic", "claude-sonnet-4-20250514"},
		{"sdd-apply", "openai", "gpt-4o"},
		{"sdd-verify", "anthropic", "claude-haiku-3-20240307"},
		{"some-other-agent", "anthropic", "claude-sonnet-4-20250514"},
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

func TestDiscoverCustomAgents(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "opencode.json")

	content := `{
  "agent": {
    "gentle-orchestrator": { "model": "anthropic/claude-sonnet-4" },
    "sdd-apply": { "model": "openai/gpt-4o" },
    "z-custom-agent": { "model": "anthropic/claude-haiku-3" },
    "a-custom-agent": { "model": "openai/gpt-4o-mini" }
  }
}`
	if err := os.WriteFile(settingsPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	custom, err := DiscoverCustomAgents(settingsPath)
	if err != nil {
		t.Fatalf("DiscoverCustomAgents() error = %v", err)
	}

	want := []string{"a-custom-agent", "z-custom-agent"}
	if len(custom) != len(want) {
		t.Fatalf("DiscoverCustomAgents() len = %d, want %d; got %v", len(custom), len(want), custom)
	}
	for i := range want {
		if custom[i] != want[i] {
			t.Errorf("custom[%d] = %q, want %q", i, custom[i], want[i])
		}
	}
}

func TestDiscoverCustomAgentsExcludesReservedRolesAndDeduplicates(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "opencode.json")

	content := `{
  "agent": {
    "build": { "model": "openai/gpt-5" },
    "plan": { "model": "openai/gpt-5-mini" },
    "general": { "model": "openai/gpt-5" },
    "explore": { "model": "openai/gpt-5-mini" },
    "gentle-reviewer": { "model": "openai/gpt-5" },
    "gentle-worker": { "model": "openai/gpt-5-mini" },
    "sdd-orchestrator": { "model": "openai/gpt-5" },
    "review-validator": { "model": "openai/gpt-5-mini" },
    "a-custom-agent": { "model": "openai/gpt-5-mini" },
    "z-custom-agent": { "model": "openai/gpt-5" },
    "a-custom-agent": { "model": "openai/gpt-5" }
  }
}`
	if err := os.WriteFile(settingsPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	got, err := DiscoverCustomAgents(settingsPath)
	if err != nil {
		t.Fatalf("DiscoverCustomAgents() error = %v", err)
	}
	want := []string{"a-custom-agent", "z-custom-agent"}
	if len(got) != len(want) {
		t.Fatalf("DiscoverCustomAgents() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("DiscoverCustomAgents()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestReadCurrentModelAssignmentsIncludesReviewAgentsFromJSONC(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "opencode.json")
	content := `{
  // Reviewer models remain global across named profiles.
  "agent": {
    "review-risk": { "model": "anthropic/claude-sonnet-4" },
    "review-readability": { "model": "openai/gpt-5" },
    "review-reliability": { "model": "openai/gpt-5" },
    "review-resilience": { "model": "anthropic/claude-sonnet-4" },
    "review-refuter": { "model": "openai/gpt-5", "variant": "high" },
    "review-validator": { "model": "openai/gpt-5-mini" },
  },
}`
	if err := os.WriteFile(settingsPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	got, err := ReadCurrentModelAssignments(settingsPath)
	if err != nil {
		t.Fatalf("ReadCurrentModelAssignments() error = %v", err)
	}
	for _, agent := range []string{"review-risk", "review-readability", "review-reliability", "review-resilience", "review-refuter", "review-validator"} {
		if got[agent].ProviderID == "" || got[agent].ModelID == "" {
			t.Errorf("review assignment %q missing: %v", agent, got)
		}
	}
	if got["review-refuter"].Effort != "high" {
		t.Fatalf("review-refuter effort = %q, want high", got["review-refuter"].Effort)
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

func TestReadCurrentModelAssignmentsMapsLegacyOrchestratorToGentleOrchestrator(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "opencode.json")

	content := `{
  "agent": {
    "sdd-orchestrator": { "model": "anthropic:claude-opus-4-5" }
  }
}`
	if err := os.WriteFile(settingsPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	got, err := ReadCurrentModelAssignments(settingsPath)
	if err != nil {
		t.Fatalf("ReadCurrentModelAssignments() error = %v", err)
	}
	if _, exists := got["sdd-orchestrator"]; exists {
		t.Fatal("legacy sdd-orchestrator key should be normalized")
	}
	want := model.ModelAssignment{ProviderID: "anthropic", ModelID: "claude-opus-4-5"}
	if got["gentle-orchestrator"] != want {
		t.Fatalf("gentle-orchestrator assignment = %+v, want %+v", got["gentle-orchestrator"], want)
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
    "gentle-orchestrator": { "model": "anthropic:claude-opus-4-5" },
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

	// Only gentle-orchestrator has a model — only it should appear
	if len(got) != 1 {
		t.Errorf("ReadCurrentModelAssignments() len = %d, want 1; got %v", len(got), got)
	}

	a, ok := got["gentle-orchestrator"]
	if !ok {
		t.Fatal("gentle-orchestrator missing from result")
	}
	want := model.ModelAssignment{ProviderID: "anthropic", ModelID: "claude-opus-4-5"}
	if a != want {
		t.Errorf("gentle-orchestrator assignment = %+v, want %+v", a, want)
	}
}

func TestReadCurrentModelAssignmentsMalformedModelField(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "opencode.json")

	// Model without colon — should be skipped without error
	content := `{
  "agent": {
    "gentle-orchestrator": { "model": "no-colon-here" },
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

	// Malformed gentle-orchestrator skipped, sdd-apply parsed
	if _, ok := got["gentle-orchestrator"]; ok {
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
    "gentle-orchestrator": { "model": "zai-coding-plan/glm-5-turbo" }
  }
}`
	if err := os.WriteFile(settingsPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	got, err := ReadCurrentModelAssignments(settingsPath)
	if err != nil {
		t.Fatalf("ReadCurrentModelAssignments() error = %v", err)
	}

	a, ok := got["gentle-orchestrator"]
	if !ok {
		t.Fatal("gentle-orchestrator missing from result — slash-separated format not parsed")
	}
	if a.ProviderID != "zai-coding-plan" {
		t.Errorf("ProviderID = %q, want %q", a.ProviderID, "zai-coding-plan")
	}
	if a.ModelID != "glm-5-turbo" {
		t.Errorf("ModelID = %q, want %q", a.ModelID, "glm-5-turbo")
	}
}

// TestReadCurrentModelAssignmentsOpenRouterFreeModel verifies that model specs
// with multiple slashes and a colon (like "openrouter/qwen/qwen3.6-plus:free")
// are parsed correctly. The provider is everything before the FIRST separator
// (slash or colon), not before the colon. Issue #802.
func TestReadCurrentModelAssignmentsOpenRouterFreeModel(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "opencode.json")

	content := `{
  "agent": {
    "sdd-apply": { "model": "openrouter/qwen/qwen3.6-plus:free" }
  }
}`
	if err := os.WriteFile(settingsPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	got, err := ReadCurrentModelAssignments(settingsPath)
	if err != nil {
		t.Fatalf("ReadCurrentModelAssignments() error = %v", err)
	}

	a, ok := got["sdd-apply"]
	if !ok {
		t.Fatal("sdd-apply missing from result — OpenRouter free-model format not parsed")
	}
	if a.ProviderID != "openrouter" {
		t.Errorf("ProviderID = %q, want %q", a.ProviderID, "openrouter")
	}
	if a.ModelID != "qwen/qwen3.6-plus:free" {
		t.Errorf("ModelID = %q, want %q", a.ModelID, "qwen/qwen3.6-plus:free")
	}
}

// TestReadCurrentModelAssignmentsReadsVariant verifies that the
// variant field in an agent definition is populated on the returned
// ModelAssignment.Effort.
func TestReadCurrentModelAssignmentsReadsVariant(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "opencode.json")

	content := `{
  "agent": {
    "sdd-apply": { "model": "anthropic:claude-opus-4", "variant": "high" },
    "sdd-verify": { "model": "anthropic:claude-sonnet-4" }
  }
}`
	if err := os.WriteFile(settingsPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	got, err := ReadCurrentModelAssignments(settingsPath)
	if err != nil {
		t.Fatalf("ReadCurrentModelAssignments() error = %v", err)
	}

	a := got["sdd-apply"]
	if a.Effort != "high" {
		t.Errorf("sdd-apply Effort = %q, want %q", a.Effort, "high")
	}

	// Agent without variant must default to empty string.
	b := got["sdd-verify"]
	if b.Effort != "" {
		t.Errorf("sdd-verify Effort = %q, want empty string", b.Effort)
	}
}

// TestReadCurrentModelAssignmentsMixedSeparators verifies that a file containing
// agents with both colon and slash separators is parsed correctly (issue #152).
func TestReadCurrentModelAssignmentsMixedSeparators(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "opencode.json")

	content := `{
  "agent": {
    "gentle-orchestrator": { "model": "anthropic:claude-sonnet-4-20250514" },
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
		{"gentle-orchestrator", "anthropic", "claude-sonnet-4-20250514"},
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
