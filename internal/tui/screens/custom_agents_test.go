package screens_test

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agentbuilder"
	"github.com/gentleman-programming/gentle-ai/v4/internal/tui/screens"
)

func TestRenderCustomAgents_Views(t *testing.T) {
	agents := []agentbuilder.RegistryEntry{{Name: "agent-1", Title: "Agent One"}}
	out := screens.RenderCustomAgents(agents, 0, nil, true)
	if !strings.Contains(out, "agent-1") || screens.CustomAgentsOptionCount(agents) != 3 {
		t.Errorf("unexpected output: %s", out)
	}

	selected := map[string]bool{"agent-1": true}
	delOut := screens.RenderCustomAgentDelete(agents, selected, 0)
	if !strings.Contains(delOut, "[x]") || screens.CustomAgentDeleteOptionCount(agents) != 3 {
		t.Errorf("unexpected delete output: %s", delOut)
	}
}
