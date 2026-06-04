package catalog

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestAllAgentsIncludesPi(t *testing.T) {
	agents := AllAgents()

	for _, agent := range agents {
		if agent.ID != model.AgentPi {
			continue
		}

		if agent.Name != "Pi" {
			t.Fatalf("Pi Name = %q, want Pi", agent.Name)
		}

		if agent.Tier != model.TierFull {
			t.Fatalf("Pi Tier = %q, want %q", agent.Tier, model.TierFull)
		}

		if agent.ConfigPath != "~/.pi" {
			t.Fatalf("Pi ConfigPath = %q, want ~/.pi", agent.ConfigPath)
		}

		return
	}

	t.Fatalf("AllAgents() missing %s", model.AgentPi)
}

func TestAllAgentsIncludesAntigravity(t *testing.T) {
	agents := AllAgents()

	for _, agent := range agents {
		if agent.ID != model.AgentAntigravity {
			continue
		}

		if agent.Name != "Google Antigravity" {
			t.Fatalf("Antigravity Name = %q, want Google Antigravity", agent.Name)
		}

		if agent.Tier != model.TierFull {
			t.Fatalf("Antigravity Tier = %q, want %q", agent.Tier, model.TierFull)
		}

		if agent.ConfigPath != "~/.gemini/antigravity-cli" {
			t.Fatalf("Antigravity ConfigPath = %q, want ~/.gemini/antigravity-cli", agent.ConfigPath)
		}

		return
	}

	t.Fatalf("AllAgents() missing %s", model.AgentAntigravity)
}

func TestIsSupportedAgentAcceptsPi(t *testing.T) {
	if !IsSupportedAgent(model.AgentPi) {
		t.Fatalf("IsSupportedAgent(%q) = false, want true", model.AgentPi)
	}
}

func TestIsSupportedAgent_False(t *testing.T) {
	if IsSupportedAgent("non-existent-agent") {
		t.Error("IsSupportedAgent(non-existent-agent) = true, want false")
	}
	if IsSupportedAgent("") {
		t.Error("IsSupportedAgent(empty) = true, want false")
	}
}

func TestMVPAgents(t *testing.T) {
	agents := MVPAgents()
	if len(agents) != 2 {
		t.Fatalf("MVPAgents() returned %d agents, want 2", len(agents))
	}

	hasClaude := false
	hasOpenCode := false

	for _, a := range agents {
		if a.ID == model.AgentClaudeCode {
			hasClaude = true
		}
		if a.ID == model.AgentOpenCode {
			hasOpenCode = true
		}
	}

	if !hasClaude || !hasOpenCode {
		t.Errorf("MVPAgents did not return expected set: %+v", agents)
	}
}

func TestIsMVPAgent(t *testing.T) {
	tests := []struct {
		agent model.AgentID
		want  bool
	}{
		{model.AgentClaudeCode, true},
		{model.AgentOpenCode, true},
		{model.AgentPi, false},
		{model.AgentAntigravity, false},
		{"unknown-agent", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.agent), func(t *testing.T) {
			if got := IsMVPAgent(tt.agent); got != tt.want {
				t.Errorf("IsMVPAgent(%q) = %t, want %t", tt.agent, got, tt.want)
			}
		})
	}
}
