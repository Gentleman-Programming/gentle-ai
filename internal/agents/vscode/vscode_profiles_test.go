package vscode

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestVSCodeModelID_KnownProviders(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		modelID  string
		want     string
	}{
		{
			name:     "anthropic claude-sonnet-4",
			provider: "anthropic",
			modelID:  "claude-sonnet-4-20250514",
			want:     "Claude Sonnet 4 (copilot)",
		},
		{
			name:     "anthropic claude-opus-4-5",
			provider: "anthropic",
			modelID:  "claude-opus-4-5-20250514",
			want:     "Claude Opus 4.5 (copilot)",
		},
		{
			name:     "anthropic claude-haiku-4-5",
			provider: "anthropic",
			modelID:  "claude-haiku-4-5-20250514",
			want:     "Claude Haiku 4.5 (copilot)",
		},
		{
			name:     "openai gpt-4o",
			provider: "openai",
			modelID:  "gpt-4o-2024-11-20",
			want:     "GPT 4o (copilot)",
		},
		{
			name:     "openai gpt-4o-mini",
			provider: "openai",
			modelID:  "gpt-4o-mini",
			want:     "GPT 4o Mini (copilot)",
		},
		{
			name:     "openai gpt-4.1",
			provider: "openai",
			modelID:  "gpt-4.1-2025-04-14",
			want:     "GPT 4.1 (copilot)",
		},
		{
			name:     "openai gpt-4.1-mini",
			provider: "openai",
			modelID:  "gpt-4.1-mini",
			want:     "GPT 4.1 Mini (copilot)",
		},
		{
			name:     "google gemini-2.5-pro",
			provider: "google",
			modelID:  "gemini-2.5-pro-preview-05-06",
			want:     "Gemini 2.5 Pro (copilot)",
		},
		{
			name:     "google gemini-2.5-flash",
			provider: "google",
			modelID:  "gemini-2.5-flash-preview-05-20",
			want:     "Gemini 2.5 Flash (copilot)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := VSCodeModelID(model.ModelAssignment{ProviderID: tt.provider, ModelID: tt.modelID})
			if got != tt.want {
				t.Fatalf("VSCodeModelID({%s, %s}) = %q, want %q", tt.provider, tt.modelID, got, tt.want)
			}
		})
	}
}

func TestVSCodeModelID_UnknownProvider_FallsBack(t *testing.T) {
	got := VSCodeModelID(model.ModelAssignment{ProviderID: "unknown", ModelID: "cheap-model"})
	if got != "unknown/cheap-model" {
		t.Fatalf("VSCodeModelID({unknown, cheap-model}) = %q, want %q", got, "unknown/cheap-model")
	}
}

func TestVSCodeModelID_EmptyModelID_ReturnsEmpty(t *testing.T) {
	got := VSCodeModelID(model.ModelAssignment{ProviderID: "anthropic", ModelID: ""})
	if got != "" {
		t.Fatalf("VSCodeModelID({anthropic, ''}) = %q, want empty string", got)
	}
}

func TestGenerateAgentFile_DefaultProfile(t *testing.T) {
	profile := model.Profile{
		Name: "",
		PhaseAssignments: map[string]model.ModelAssignment{
			"sdd-apply": {ProviderID: "anthropic", ModelID: "claude-sonnet-4-20250514"},
		},
	}

	content := GenerateAgentFile("sdd-apply", profile)

	// Must have YAML frontmatter markers
	if !strings.HasPrefix(content, "---\n") {
		t.Fatalf("Agent file must start with YAML frontmatter, got: %q", content[:min(50, len(content))])
	}
	if !strings.Contains(content, "\n---\n") {
		t.Fatalf("Agent file must have closing YAML frontmatter marker")
	}

	// Must contain required fields
	if !strings.Contains(content, "name: sdd-apply") {
		t.Fatalf("Agent file must contain 'name: sdd-apply', got:\n%s", content)
	}
	if !strings.Contains(content, "description:") {
		t.Fatalf("Agent file must contain 'description:' field")
	}
	if !strings.Contains(content, "readonly:") {
		t.Fatalf("Agent file must contain 'readonly:' field")
	}
	if !strings.Contains(content, "background:") {
		t.Fatalf("Agent file must contain 'background:' field")
	}
	if !strings.Contains(content, "user-invocable:") {
		t.Fatalf("Agent file must contain 'user-invocable:' field")
	}

	// Must have model mapping for known provider
	if !strings.Contains(content, "model: \"Claude Sonnet 4 (copilot)\"") {
		t.Fatalf("Agent file must contain resolved model name, got:\n%s", content)
	}
}

func TestGenerateAgentFile_NamedProfile(t *testing.T) {
	profile := model.Profile{
		Name: "cheap",
		PhaseAssignments: map[string]model.ModelAssignment{
			"sdd-apply": {ProviderID: "openai", ModelID: "gpt-4o-mini"},
		},
	}

	content := GenerateAgentFile("sdd-apply", profile)

	// Named profile should include profile name in the agent name
	if !strings.Contains(content, "name: sdd-apply-cheap") {
		t.Fatalf("Named profile agent must have suffixed name, got:\n%s", content)
	}
}

func TestGenerateAgentFile_NoModelAssignment_OmitsField(t *testing.T) {
	profile := model.Profile{
		Name:              "",
		PhaseAssignments:  map[string]model.ModelAssignment{},
	}

	content := GenerateAgentFile("sdd-apply", profile)

	// When no model assignment for the phase, model field must be absent or omitted
	frontmatterEnd := strings.Index(content[4:], "\n---\n")
	if frontmatterEnd == -1 {
		t.Fatalf("Cannot find closing frontmatter marker")
	}
	frontmatter := content[4 : frontmatterEnd+4]

	if strings.Contains(frontmatter, "model:") {
		t.Fatalf("When no model assignment, model field must be absent from frontmatter, got:\n%s", frontmatter)
	}
}

func TestVSCodeModelID_AllDesignMappings(t *testing.T) {
	// Verify every mapping from the design doc
	designMappings := []struct {
		modelSubstr string
		expected    string
	}{
		{"claude-sonnet-4", "Claude Sonnet 4 (copilot)"},
		{"claude-opus-4-5", "Claude Opus 4.5 (copilot)"},
		{"claude-haiku-4-5", "Claude Haiku 4.5 (copilot)"},
		{"gemini-2.5-pro", "Gemini 2.5 Pro (copilot)"},
		{"gemini-2.5-flash", "Gemini 2.5 Flash (copilot)"},
		{"gpt-4.1", "GPT 4.1 (copilot)"},
		{"gpt-4.1-mini", "GPT 4.1 Mini (copilot)"},
		{"gpt-4o", "GPT 4o (copilot)"},
		{"gpt-4o-mini", "GPT 4o Mini (copilot)"},
	}

	for _, dm := range designMappings {
		t.Run(dm.modelSubstr, func(t *testing.T) {
			got := VSCodeModelID(model.ModelAssignment{ProviderID: "any", ModelID: dm.modelSubstr})
			if got != dm.expected {
				t.Fatalf("VSCodeModelID({any, %s}) = %q, want %q", dm.modelSubstr, got, dm.expected)
			}
		})
	}
}

func TestRemoveVSCodeProfileAgents_RemovesOnlyGentleAIAssets(t *testing.T) {
	agentsDir := t.TempDir()

	// Create mixed files: gentle-ai SDD files + user files
	gentleAISDD := []string{
		"sdd-init-cheap.agent.md",
		"sdd-explore-cheap.agent.md",
		"sdd-propose-cheap.agent.md",
		"sdd-spec-cheap.agent.md",
		"sdd-design-cheap.agent.md",
		"sdd-tasks-cheap.agent.md",
		"sdd-apply-cheap.agent.md",
		"sdd-verify-cheap.agent.md",
		"sdd-archive-cheap.agent.md",
		"sdd-onboard-cheap.agent.md",
	}
	userFiles := []string{
		"my-custom.agent.md",
		"helper.agent.md",
		"notes.txt",
	}

	for _, f := range gentleAISDD {
		if err := os.WriteFile(filepath.Join(agentsDir, f), []byte("content"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", f, err)
		}
	}
	for _, f := range userFiles {
		if err := os.WriteFile(filepath.Join(agentsDir, f), []byte("user content"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", f, err)
		}
	}

	err := RemoveVSCodeProfileAgents(agentsDir, "cheap")
	if err != nil {
		t.Fatalf("RemoveVSCodeProfileAgents() error = %v", err)
	}

	// All gentle-ai SDD files should be removed
	for _, f := range gentleAISDD {
		path := filepath.Join(agentsDir, f)
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Errorf("file %q should have been removed but still exists", f)
		}
	}

	// User files should be preserved
	for _, f := range userFiles {
		path := filepath.Join(agentsDir, f)
		if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
			t.Errorf("user file %q should have been preserved but was removed", f)
		}
	}
}

func TestRemoveVSCodeProfileAgents_RejectsDefaultProfile(t *testing.T) {
	for _, name := range []string{"", "default"} {
		t.Run(name, func(t *testing.T) {
			err := RemoveVSCodeProfileAgents(t.TempDir(), name)
			if err == nil {
				t.Fatalf("RemoveVSCodeProfileAgents(%q) should return error for default profile", name)
			}
			if !strings.Contains(err.Error(), "cannot remove default profile") {
				t.Fatalf("RemoveVSCodeProfileAgents(%q) error = %q, want 'cannot remove default profile'", name, err.Error())
			}
		})
	}
}

func TestRemoveVSCodeProfileAgents_SilentlySkipsMissingFiles(t *testing.T) {
	agentsDir := t.TempDir()
	// No files exist at all — should not error
	err := RemoveVSCodeProfileAgents(agentsDir, "nonexistent")
	if err != nil {
		t.Fatalf("RemoveVSCodeProfileAgents() on empty dir error = %v", err)
	}
}

func TestRemoveVSCodeProfileAgents_NonexistentDirIsNoop(t *testing.T) {
	err := RemoveVSCodeProfileAgents(filepath.Join(t.TempDir(), "does-not-exist"), "cheap")
	if err != nil {
		t.Fatalf("RemoveVSCodeProfileAgents() on nonexistent dir error = %v", err)
	}
}