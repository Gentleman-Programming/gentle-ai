package screens

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestRenderABEngine_NonEmpty(t *testing.T) {
	engines := []model.AgentID{model.AgentClaudeCode, model.AgentOpenCode}
	out := RenderABEngine(engines, 0)
	if out == "" {
		t.Fatal("RenderABEngine returned empty string")
	}
}

func TestRenderABEngine_HeadingPresent(t *testing.T) {
	engines := []model.AgentID{model.AgentClaudeCode}
	out := RenderABEngine(engines, 0)
	if !strings.Contains(out, "Choose Your AI Engine") {
		t.Errorf("heading not found; output:\n%s", out)
	}
}

func TestRenderABEngine_EngineLabelsPresent(t *testing.T) {
	engines := []model.AgentID{model.AgentClaudeCode, model.AgentGeminiCLI}
	out := RenderABEngine(engines, 0)
	if !strings.Contains(out, string(model.AgentClaudeCode)) {
		t.Errorf("claude-code label not found; output:\n%s", out)
	}
	if !strings.Contains(out, string(model.AgentGeminiCLI)) {
		t.Errorf("gemini-cli label not found; output:\n%s", out)
	}
}

func TestRenderABEngine_BackOptionPresent(t *testing.T) {
	engines := []model.AgentID{model.AgentClaudeCode}
	out := RenderABEngine(engines, 0)
	if !strings.Contains(out, "Back") {
		t.Errorf("Back option not found; output:\n%s", out)
	}
}

func TestRenderABEngine_EmptyEngines_ShowsWarning(t *testing.T) {
	out := RenderABEngine([]model.AgentID{}, 0)
	if !strings.Contains(out, "No supported AI agent") {
		t.Errorf("expected warning for no engines; output:\n%s", out)
	}
}

func TestABEngineOptions_IncludesBack(t *testing.T) {
	engines := []model.AgentID{model.AgentClaudeCode, model.AgentOpenCode}
	opts := ABEngineOptions(engines)
	if len(opts) != 3 { // 2 engines + Back
		t.Errorf("len(opts) = %d, want 3", len(opts))
	}
	if opts[len(opts)-1] != "Back" {
		t.Errorf("last option = %q, want 'Back'", opts[len(opts)-1])
	}
}
