package catalog

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestAllAgentsIncludesOpenClaw(t *testing.T) {
	agents := AllAgents()

	for _, agent := range agents {
		if agent.ID != model.AgentOpenClaw {
			continue
		}

		if agent.Name != "OpenClaw" {
			t.Fatalf("OpenClaw Name = %q, want OpenClaw", agent.Name)
		}

		if agent.Tier != model.TierFull {
			t.Fatalf("OpenClaw Tier = %q, want %q", agent.Tier, model.TierFull)
		}

		if agent.ConfigPath != "~/.openclaw" {
			t.Fatalf("OpenClaw ConfigPath = %q, want ~/.openclaw", agent.ConfigPath)
		}

		return
	}

	t.Fatalf("AllAgents() missing %s", model.AgentOpenClaw)
}

func TestAllAgentsIncludesPiCodingAgent(t *testing.T) {
	agents := AllAgents()

	for _, agent := range agents {
		if agent.ID != model.AgentPiCodingAgent {
			continue
		}

		if agent.Name != "Pi coding agent" {
			t.Fatalf("Pi Name = %q, want Pi coding agent", agent.Name)
		}

		if agent.Tier != model.TierPartial {
			t.Fatalf("Pi Tier = %q, want %q", agent.Tier, model.TierPartial)
		}

		if agent.ConfigPath != "~/.pi/agent" {
			t.Fatalf("Pi ConfigPath = %q, want ~/.pi/agent", agent.ConfigPath)
		}

		return
	}

	t.Fatalf("AllAgents() missing %s", model.AgentPiCodingAgent)
}

func TestIsSupportedAgentAcceptsOpenClaw(t *testing.T) {
	if !IsSupportedAgent(model.AgentOpenClaw) {
		t.Fatalf("IsSupportedAgent(%q) = false, want true", model.AgentOpenClaw)
	}
}

func TestIsSupportedAgentAcceptsPiCodingAgent(t *testing.T) {
	if !IsSupportedAgent(model.AgentPiCodingAgent) {
		t.Fatalf("IsSupportedAgent(%q) = false, want true", model.AgentPiCodingAgent)
	}
}
