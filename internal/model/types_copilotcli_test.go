package model

import "testing"

func TestAgentCopilotCLI(t *testing.T) {
	// RED: this test references AgentCopilotCLI before it exists.
	// It verifies the constant has the expected string value.
	if got := AgentCopilotCLI; got != "copilot-cli" {
		t.Fatalf("AgentCopilotCLI = %q, want %q", got, "copilot-cli")
	}
}
