package catalog

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestAllAgentsIncludesPIWithNonParityLabels(t *testing.T) {
	agents := AllAgents()

	var pi *Agent
	for i := range agents {
		if agents[i].ID == model.AgentPiCodingAgent {
			pi = &agents[i]
			break
		}
	}

	if pi == nil {
		t.Fatalf("AllAgents() missing %q", model.AgentPiCodingAgent)
	}

	if pi.ConfigPath != "~/.pi/agent" {
		t.Fatalf("PI ConfigPath = %q, want %q", pi.ConfigPath, "~/.pi/agent")
	}

	wantLabels := []string{"experimental", "non-parity"}
	if len(pi.CapabilityLabels) != len(wantLabels) {
		t.Fatalf("PI CapabilityLabels len = %d, want %d (%v)", len(pi.CapabilityLabels), len(wantLabels), wantLabels)
	}
	for idx, want := range wantLabels {
		if pi.CapabilityLabels[idx] != want {
			t.Fatalf("PI CapabilityLabels[%d] = %q, want %q", idx, pi.CapabilityLabels[idx], want)
		}
	}
}

func TestMVPAgentsDoNotIncludePI(t *testing.T) {
	for _, agent := range MVPAgents() {
		if agent.ID == model.AgentPiCodingAgent {
			t.Fatalf("MVPAgents() should not include %q", model.AgentPiCodingAgent)
		}
	}
}
