package screens

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestAgentOptionsShowsAntigravityOnly(t *testing.T) {
	options := AgentOptions()

	seenAntigravity := false

	for _, option := range options {
		if option == model.AgentID("antigravity-cli") {
			t.Fatal("AgentOptions() should not expose antigravity-cli as a separate TUI option")
		}
		if option == model.AgentAntigravity {
			seenAntigravity = true
		}
	}

	if !seenAntigravity {
		t.Fatal("AgentOptions() missing Antigravity option")
	}
}
