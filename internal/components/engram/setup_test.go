package engram

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestParseSetupModeDefaultsToSupported(t *testing.T) {
	tests := []string{"", "invalid", "  weird  "}
	for _, value := range tests {
		if got := ParseSetupMode(value); got != SetupModeSupported {
			t.Fatalf("ParseSetupMode(%q) = %q, want %q", value, got, SetupModeSupported)
		}
	}
}

func TestParseSetupModeValues(t *testing.T) {
	if got := ParseSetupMode("off"); got != SetupModeOff {
		t.Fatalf("ParseSetupMode(off) = %q, want %q", got, SetupModeOff)
	}
	if got := ParseSetupMode("supported"); got != SetupModeSupported {
		t.Fatalf("ParseSetupMode(supported) = %q, want %q", got, SetupModeSupported)
	}
	if got := ParseSetupMode("opencode"); got != SetupModeOpenCode {
		t.Fatalf("ParseSetupMode(opencode) = %q, want %q", got, SetupModeOpenCode)
	}
}

func TestParseSetupStrict(t *testing.T) {
	truthy := []string{"1", "true", "TRUE", "yes", "on"}
	for _, value := range truthy {
		if !ParseSetupStrict(value) {
			t.Fatalf("ParseSetupStrict(%q) = false, want true", value)
		}
	}

	falsy := []string{"", "0", "false", "no", "off", "nah"}
	for _, value := range falsy {
		if ParseSetupStrict(value) {
			t.Fatalf("ParseSetupStrict(%q) = true, want false", value)
		}
	}
}

func TestSetupAgentSlug(t *testing.T) {
	tests := []struct {
		agent model.AgentID
		slug  string
		ok    bool
	}{
		{model.AgentOpenCode, "opencode", true},
		{model.AgentClaudeCode, "claude-code", true},
		{model.AgentGeminiCLI, "gemini-cli", true},
		{model.AgentCodex, "codex", true},
		{model.AgentAntigravity, "gemini-cli", true},
		{model.AgentWindsurf, "windsurf", true},
		{model.AgentQwenCode, "", false},
		{model.AgentCursor, "", false},
		{model.AgentVSCodeCopilot, "", false},
	}

	for _, tt := range tests {
		slug, ok := SetupAgentSlug(tt.agent)
		if slug != tt.slug || ok != tt.ok {
			t.Fatalf("SetupAgentSlug(%q) = (%q, %v), want (%q, %v)", tt.agent, slug, ok, tt.slug, tt.ok)
		}
	}
}

func TestShouldAttemptSetup(t *testing.T) {
	if ShouldAttemptSetup(SetupModeOff, model.AgentOpenCode) {
		t.Fatal("ShouldAttemptSetup(off, opencode) = true, want false")
	}
	if !ShouldAttemptSetup(SetupModeOpenCode, model.AgentOpenCode) {
		t.Fatal("ShouldAttemptSetup(opencode, opencode) = false, want true")
	}
	if ShouldAttemptSetup(SetupModeOpenCode, model.AgentGeminiCLI) {
		t.Fatal("ShouldAttemptSetup(opencode, gemini-cli) = true, want false")
	}
	if !ShouldAttemptSetup(SetupModeSupported, model.AgentClaudeCode) {
		t.Fatal("ShouldAttemptSetup(supported, claude-code) = false, want true")
	}
	if ShouldAttemptSetup(SetupModeSupported, model.AgentQwenCode) {
		t.Fatal("ShouldAttemptSetup(supported, qwen-code) = true, want false")
	}
	if ShouldAttemptSetup(SetupModeSupported, model.AgentCursor) {
		t.Fatal("ShouldAttemptSetup(supported, cursor) = true, want false")
	}
}
