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

func TestAllAgentsIncludesHermes(t *testing.T) {
	agents := AllAgents()

	for _, agent := range agents {
		if agent.ID != model.AgentHermes {
			continue
		}

		if agent.Name != "Hermes" {
			t.Fatalf("Hermes Name = %q, want Hermes", agent.Name)
		}

		if agent.Tier != model.TierFull {
			t.Fatalf("Hermes Tier = %q, want %q", agent.Tier, model.TierFull)
		}

		if agent.ConfigPath != "~/.hermes" {
			t.Fatalf("Hermes ConfigPath = %q, want ~/.hermes", agent.ConfigPath)
		}

		return
	}

	t.Fatalf("AllAgents() missing %s", model.AgentHermes)
}

func TestIsSupportedAgentAcceptsHermes(t *testing.T) {
	if !IsSupportedAgent(model.AgentHermes) {
		t.Fatalf("IsSupportedAgent(%q) = false, want true", model.AgentHermes)
	}
}

func TestAllAgentsIncludesFreebuff(t *testing.T) {
	agents := AllAgents()

	for _, agent := range agents {
		if agent.ID != model.AgentFreebuff {
			continue
		}

		if agent.Name != "Freebuff" {
			t.Fatalf("Freebuff Name = %q, want Freebuff", agent.Name)
		}

		if agent.Tier != model.TierFull {
			t.Fatalf("Freebuff Tier = %q, want %q", agent.Tier, model.TierFull)
		}

		if agent.ConfigPath != "~/.agents" {
			t.Fatalf("Freebuff ConfigPath = %q, want ~/.agents", agent.ConfigPath)
		}

		return
	}

	t.Fatalf("AllAgents() missing %s", model.AgentFreebuff)
}

func TestIsSupportedAgentAcceptsFreebuff(t *testing.T) {
	if !IsSupportedAgent(model.AgentFreebuff) {
		t.Fatalf("IsSupportedAgent(%q) = false, want true", model.AgentFreebuff)
	}
}
