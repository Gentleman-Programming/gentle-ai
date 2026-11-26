package agentguidance_test

import (
	"os"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v5/internal/agents/antigravity"
	"github.com/gentleman-programming/gentle-ai/v5/internal/components/agentguidance"
	"github.com/gentleman-programming/gentle-ai/v5/internal/components/antigravityrules"
	"github.com/gentleman-programming/gentle-ai/v5/internal/components/engram"
	"github.com/gentleman-programming/gentle-ai/v5/internal/components/persona"
	"github.com/gentleman-programming/gentle-ai/v5/internal/model"
)

func TestInstalledAntigravityCloseFitsAlwaysOnCore(t *testing.T) {
	home := t.TempDir()
	adapter := antigravity.NewAdapter()
	engram.SetLookPathForTest(t, "/opt/homebrew/bin/engram", "")
	if _, err := persona.Inject(home, adapter, model.PersonaGentleman); err != nil {
		t.Fatal(err)
	}
	if _, err := engram.Inject(home, adapter); err != nil {
		t.Fatal(err)
	}
	options := agentguidance.RoutingOptions{}
	if _, err := agentguidance.InjectRoutingWithOptions(home, model.AgentAntigravity, options); err != nil {
		t.Fatal(err)
	}
	if _, err := agentguidance.InjectStrictTDDWithOptions(home, model.AgentAntigravity, false, options); err != nil {
		t.Fatal(err)
	}
	staging, err := os.ReadFile(adapter.SystemPromptFile(home))
	if err != nil {
		t.Fatal(err)
	}

	const alwaysOn = "---\ntrigger: always_on\n---\n\n"
	rules := antigravityrules.Render(string(staging))
	largest, inline := 0, 0
	for _, rule := range rules {
		largest = max(largest, len(rule.Content))
		if strings.HasPrefix(rule.Content, alwaysOn) {
			inline += len(rule.Content)
		}
	}
	t.Logf("Antigravity largest rule: %d of %d bytes; all always-on rules: %d of %d bytes", largest, antigravityrules.MaxRuleFileBytes, inline, antigravityrules.AlwaysOnBudgetBytes)
	if largest > antigravityrules.MaxRuleFileBytes || inline > antigravityrules.AlwaysOnBudgetBytes {
		t.Error("installed Antigravity rules exceed a byte budget")
	}

	coreBytes := 0
	for _, rule := range rules {
		body, on := strings.CutPrefix(rule.Content, alwaysOn)
		if !on {
			_, body, _ = strings.Cut(rule.Content, "\n---\n\n")
		}
		if strings.Contains(body, "7. **Close.**") {
			coreBytes += len(alwaysOn) + len(body)
			if !on {
				t.Errorf("%s: required Antigravity core is %d bytes; always-on budget is %d", rule.Name, coreBytes, antigravityrules.AlwaysOnBudgetBytes)
			}
			t.Logf("Antigravity required core: %d of %d bytes", coreBytes, antigravityrules.AlwaysOnBudgetBytes)
			return
		}
		// These three sections precede agent-routing in the core priority.
		if strings.HasSuffix(rule.Name, "-persona.md") || strings.HasSuffix(rule.Name, "-engram-protocol.md") || strings.HasSuffix(rule.Name, "-orchestrator-part1.md") {
			coreBytes += len(alwaysOn) + len(body)
		}
	}
	t.Fatal("installed Antigravity rules have no ODD Close step")
}
