package model

import "testing"

func TestAgentAntigravity(t *testing.T) {
	if AgentAntigravity != "antigravity" {
		t.Errorf("AgentAntigravity = %q, want %q", AgentAntigravity, "antigravity")
	}
}
