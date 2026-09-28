package model

import "testing"

func TestSupportsReceiptDrivenDevelopmentIsTheClosedRDDRuntimeSet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		agent AgentID
		want  bool
	}{
		{AgentClaudeCode, true},
		{AgentCodex, true},
		{AgentOpenCode, true},
		{AgentPi, true},
		{AgentKilocode, false},
		{AgentGeminiCLI, false},
		{AgentCursor, false},
		{AgentVSCodeCopilot, false},
		{AgentAntigravity, false},
		{AgentWindsurf, false},
		{AgentKimi, false},
		{AgentQwenCode, false},
		{AgentKiroIDE, false},
		{AgentOpenClaw, false},
		{AgentTrae, false},
		{AgentHermes, false},
		{AgentID("unknown-runtime"), false},
	}
	for _, tt := range tests {
		t.Run(string(tt.agent), func(t *testing.T) {
			t.Parallel()
			if got := SupportsReceiptDrivenDevelopment(tt.agent); got != tt.want {
				t.Fatalf("SupportsReceiptDrivenDevelopment(%q) = %v, want %v", tt.agent, got, tt.want)
			}
		})
	}
}
