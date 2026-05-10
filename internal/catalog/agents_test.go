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

func TestIsSupportedAgentAcceptsOpenClaw(t *testing.T) {
	if !IsSupportedAgent(model.AgentOpenClaw) {
		t.Fatalf("IsSupportedAgent(%q) = false, want true", model.AgentOpenClaw)
	}
}
