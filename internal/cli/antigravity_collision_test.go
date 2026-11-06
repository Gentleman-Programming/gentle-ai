package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

func TestAntigravityCollisionCheckIncludesGeminiCLI(t *testing.T) {
	checks := antigravityCollisionCheck([]model.AgentID{model.AgentGeminiCLI, model.AgentAntigravity})
	if len(checks) != 1 {
		t.Fatalf("antigravityCollisionCheck() len = %d, want 1", len(checks))
	}

	check := checks[0]
	if !check.Soft {
		t.Fatal("antigravityCollisionCheck() should return a soft warning")
	}
	if check.ID != "verify:antigravity:rules-collision" {
		t.Fatalf("check ID = %q, want verify:antigravity:rules-collision", check.ID)
	}

	err := check.Run(context.Background())
	if err == nil {
		t.Fatal("check.Run() error = nil, want warning message")
	}
	message := err.Error()
	for _, want := range []string{
		"~/.gemini/config/rules/gentle-ai-*.md",
		"also loads ~/.gemini/GEMINI.md, which Gemini CLI owns",
		"Keep Gemini CLI selected only if you also use it",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("warning message missing %q; got:\n%s", want, message)
		}
	}
	// Antigravity no longer writes GEMINI.md (#5608), so the warning must not
	// claim the agents share or overwrite each other's sections there.
	for _, stale := range []string{"write rules to ~/.gemini/GEMINI.md", "controls the shared gentle-ai:agent-routing section"} {
		if strings.Contains(message, stale) || strings.Contains(check.Description, "share") {
			t.Fatalf("warning still describes the retired shared GEMINI.md write (%q): %s / %s", stale, check.Description, message)
		}
	}
	if strings.Contains(message, "gentle-ai:sdd-orchestrator") || strings.Contains(message, "SDD orchestrator") {
		t.Fatalf("warning message advertises retired SDD guidance: %s", message)
	}
}

func TestAntigravityCollisionCheckNoWarningWithoutGemini(t *testing.T) {
	checks := antigravityCollisionCheck([]model.AgentID{model.AgentAntigravity})
	if len(checks) != 0 {
		t.Fatalf("antigravityCollisionCheck() len = %d, want 0", len(checks))
	}
}
