package agentbuilder

import (
	"context"
	"errors"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

// ─── MockEngine tests ────────────────────────────────────────────────────────

func TestMockEngine_Agent_ReturnsConfiguredID(t *testing.T) {
	mock := &MockEngine{AgentIDVal: model.AgentClaudeCode}
	if got := mock.Agent(); got != model.AgentClaudeCode {
		t.Errorf("Agent() = %q, want %q", got, model.AgentClaudeCode)
	}
}

func TestMockEngine_Available_ReturnsConfiguredValue(t *testing.T) {
	tests := []struct {
		available bool
	}{
		{true},
		{false},
	}
	for _, tt := range tests {
		mock := &MockEngine{IsAvailable: tt.available}
		if got := mock.Available(); got != tt.available {
			t.Errorf("Available() = %v, want %v", got, tt.available)
		}
	}
}

func TestMockEngine_Generate_ReturnsConfiguredResponse(t *testing.T) {
	mock := &MockEngine{
		Output:      "generated content",
		IsAvailable: true,
	}
	out, err := mock.Generate(context.Background(), "some prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "generated content" {
		t.Errorf("Generate() = %q, want %q", out, "generated content")
	}
}

func TestMockEngine_Generate_ReturnsConfiguredError(t *testing.T) {
	expectedErr := errors.New("generation failed")
	mock := &MockEngine{
		Err:         expectedErr,
		IsAvailable: true,
	}
	_, err := mock.Generate(context.Background(), "some prompt")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("error = %v, want %v", err, expectedErr)
	}
}

// ─── NewEngine tests ─────────────────────────────────────────────────────────

func TestNewEngine_ClaudeCode_ReturnsClaudeEngine(t *testing.T) {
	engine := NewEngine(model.AgentClaudeCode)
	if engine == nil {
		t.Fatal("expected non-nil engine for claude-code")
	}
	if engine.Agent() != model.AgentClaudeCode {
		t.Errorf("Agent() = %q, want %q", engine.Agent(), model.AgentClaudeCode)
	}
}

func TestNewEngine_OpenCode_ReturnsOpenCodeEngine(t *testing.T) {
	engine := NewEngine(model.AgentOpenCode)
	if engine == nil {
		t.Fatal("expected non-nil engine for opencode")
	}
	if engine.Agent() != model.AgentOpenCode {
		t.Errorf("Agent() = %q, want %q", engine.Agent(), model.AgentOpenCode)
	}
}

func TestNewEngine_GeminiCLI_ReturnsGeminiEngine(t *testing.T) {
	engine := NewEngine(model.AgentGeminiCLI)
	if engine == nil {
		t.Fatal("expected non-nil engine for gemini-cli")
	}
	if engine.Agent() != model.AgentGeminiCLI {
		t.Errorf("Agent() = %q, want %q", engine.Agent(), model.AgentGeminiCLI)
	}
}

func TestNewEngine_Codex_ReturnsCodexEngine(t *testing.T) {
	engine := NewEngine(model.AgentCodex)
	if engine == nil {
		t.Fatal("expected non-nil engine for codex")
	}
	if engine.Agent() != model.AgentCodex {
		t.Errorf("Agent() = %q, want %q", engine.Agent(), model.AgentCodex)
	}
}

func TestNewEngine_Unknown_ReturnsNil(t *testing.T) {
	engine := NewEngine(model.AgentID("unknown-agent"))
	if engine != nil {
		t.Errorf("expected nil engine for unknown agent, got %v", engine)
	}
}

func TestNewEngine_Cursor_ReturnsNil(t *testing.T) {
	// cursor, vscode-copilot, etc. are not supported generation engines.
	engine := NewEngine(model.AgentCursor)
	if engine != nil {
		t.Errorf("expected nil engine for cursor (not a generation engine), got %v", engine)
	}
}

// ─── Engine command construction tests ───────────────────────────────────────
// These test the Agent() method (which identifies the CLI binary used)
// without actually executing anything.

func TestClaudeEngine_AgentID(t *testing.T) {
	e := &ClaudeEngine{}
	if e.Agent() != model.AgentClaudeCode {
		t.Errorf("ClaudeEngine.Agent() = %q, want %q", e.Agent(), model.AgentClaudeCode)
	}
}

func TestOpenCodeEngine_AgentID(t *testing.T) {
	e := &OpenCodeEngine{}
	if e.Agent() != model.AgentOpenCode {
		t.Errorf("OpenCodeEngine.Agent() = %q, want %q", e.Agent(), model.AgentOpenCode)
	}
}

func TestGeminiEngine_AgentID(t *testing.T) {
	e := &GeminiEngine{}
	if e.Agent() != model.AgentGeminiCLI {
		t.Errorf("GeminiEngine.Agent() = %q, want %q", e.Agent(), model.AgentGeminiCLI)
	}
}

func TestCodexEngine_AgentID(t *testing.T) {
	e := &CodexEngine{}
	if e.Agent() != model.AgentCodex {
		t.Errorf("CodexEngine.Agent() = %q, want %q", e.Agent(), model.AgentCodex)
	}
}

// TestAllSupportedEngines verifies that all supported engine IDs produce non-nil engines.
func TestAllSupportedEngines(t *testing.T) {
	supported := []model.AgentID{
		model.AgentClaudeCode,
		model.AgentOpenCode,
		model.AgentGeminiCLI,
		model.AgentCodex,
	}
	for _, id := range supported {
		t.Run(string(id), func(t *testing.T) {
			engine := NewEngine(id)
			if engine == nil {
				t.Errorf("NewEngine(%q) returned nil", id)
			}
		})
	}
}
