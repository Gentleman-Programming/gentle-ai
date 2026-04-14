package catalog

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestIsSupportedAgentIncludesKiroAndQwen(t *testing.T) {
	tests := []struct {
		name  string
		agent model.AgentID
	}{
		{name: "kiro", agent: model.AgentKiroIDE},
		{name: "qwen", agent: model.AgentQwenCode},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !IsSupportedAgent(tt.agent) {
				t.Fatalf("IsSupportedAgent(%q) = false, want true", tt.agent)
			}
		})
	}
}

func TestAllAgentsIncludesKiroAndQwen(t *testing.T) {
	agents := AllAgents()

	want := map[model.AgentID]bool{
		model.AgentKiroIDE:  false,
		model.AgentQwenCode: false,
	}

	for _, agent := range agents {
		if _, ok := want[agent.ID]; ok {
			want[agent.ID] = true
		}
	}

	for agent, found := range want {
		if !found {
			t.Fatalf("AllAgents() missing %q", agent)
		}
	}
}
