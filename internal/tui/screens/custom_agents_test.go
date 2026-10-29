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

// TestRenderCustomAgents_SanitizesControlCharacters pins that registry-sourced
// labels cannot forge menu content or issue terminal control sequences.
func TestRenderCustomAgents_SanitizesControlCharacters(t *testing.T) {
	injection := "evil\x1b[31mINJECTED\nnext-row\x07"
	agents := []agentbuilder.RegistryEntry{{Name: injection, Title: injection}}

	for _, out := range []string{
		screens.RenderCustomAgents(agents, 0, nil, true),
		screens.RenderCustomAgentDelete(agents, nil, 0),
	} {
		if strings.Contains(out, "\x1b") {
			t.Errorf("ANSI escape sequence survived rendering: %q", out)
		}
		if strings.ContainsRune(out, '\x07') {
			t.Errorf("BEL control character survived rendering: %q", out)
		}
		if !strings.Contains(out, "evilINJECTEDnext-row") {
			t.Errorf("sanitized label not rendered as a single printable token: %q", out)
		}
	}
}
