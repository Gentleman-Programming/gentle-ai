package rtk

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestAgentFlags(t *testing.T) {
	tests := []struct {
		name     string
		agentID  model.AgentID
		wantFlag string
	}{
		{
			name:     "claude-code returns empty string (default)",
			agentID:  model.AgentClaudeCode,
			wantFlag: "",
		},
		{
			name:     "opencode returns --opencode",
			agentID:  model.AgentOpenCode,
			wantFlag: "--opencode",
		},
		{
			name:     "gemini-cli returns --gemini",
			agentID:  model.AgentGeminiCLI,
			wantFlag: "--gemini",
		},
		{
			name:     "cursor returns --agent cursor",
			agentID:  model.AgentCursor,
			wantFlag: "--agent cursor",
		},
		{
			name:     "vscode-copilot returns --copilot",
			agentID:  model.AgentVSCodeCopilot,
			wantFlag: "--copilot",
		},
		{
			name:     "codex returns --codex",
			agentID:  model.AgentCodex,
			wantFlag: "--codex",
		},
		{
			name:     "windsurf returns --agent windsurf",
			agentID:  model.AgentWindsurf,
			wantFlag: "--agent windsurf",
		},
		{
			name:     "antigravity returns empty (no support)",
			agentID:  model.AgentAntigravity,
			wantFlag: "",
		},
		{
			name:     "unknown agent returns empty",
			agentID:  model.AgentID("unknown"),
			wantFlag: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AgentFlags(tt.agentID)
			if got != tt.wantFlag {
				t.Errorf("AgentFlags(%q) = %q, want %q", tt.agentID, got, tt.wantFlag)
			}
		})
	}
}

func TestSupportsHook(t *testing.T) {
	tests := []struct {
		name    string
		agentID model.AgentID
		want    bool
	}{
		{name: "claude-code supports hooks", agentID: model.AgentClaudeCode, want: true},
		{name: "opencode supports hooks", agentID: model.AgentOpenCode, want: true},
		{name: "gemini-cli supports hooks", agentID: model.AgentGeminiCLI, want: true},
		{name: "cursor supports hooks", agentID: model.AgentCursor, want: true},
		{name: "vscode-copilot supports hooks", agentID: model.AgentVSCodeCopilot, want: true},
		{name: "codex supports hooks", agentID: model.AgentCodex, want: true},
		{name: "windsurf supports hooks", agentID: model.AgentWindsurf, want: true},
		{name: "antigravity does NOT support hooks", agentID: model.AgentAntigravity, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SupportsHook(tt.agentID)
			if got != tt.want {
				t.Errorf("SupportsHook(%q) = %v, want %v", tt.agentID, got, tt.want)
			}
		})
	}
}
