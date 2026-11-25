package agentguidance_test

import (
	"os"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v5/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v5/internal/components/agentguidance"
	"github.com/gentleman-programming/gentle-ai/v5/internal/components/antigravityrules"
	"github.com/gentleman-programming/gentle-ai/v5/internal/model"
)

// TestAntigravityOrchestratorRoutesODDToInstalledAgents verifies that Antigravity
// orchestrator guidance routes delegated ODD work to the installed gentle-ai-*
// role agents (preserving configured model tiers) while keeping delegation thresholds intact (#5993).
func TestAntigravityOrchestratorRoutesODDToInstalledAgents(t *testing.T) {
	content, err := assets.Read("antigravity/orchestrator.md")
	if err != nil {
		t.Fatalf("assets.Read(antigravity/orchestrator.md) error = %v", err)
	}

	for _, want := range []string{
		"gentle-ai-explore",
		"gentle-ai-worker",
		"gentle-ai-verify",
		"invoke_subagent",
		"define_subagent",
		"Evidence budget rule",
		"Write rule",
		"Verification rule",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("antigravity/orchestrator.md missing %q", want)
		}
	}

	home := t.TempDir()
	if _, err := agentguidance.InjectRoutingWithOptions(home, model.AgentAntigravity, agentguidance.RoutingOptions{}); err != nil {
		t.Fatalf("InjectRoutingWithOptions(antigravity) error = %v", err)
	}
	stagingBytes, err := os.ReadFile(antigravityrules.StagingPath(home))
	if err != nil {
		t.Fatalf("os.ReadFile(stagingPath) error = %v", err)
	}
	rules := antigravityrules.Render(string(stagingBytes))
	var orchestratorParts []string
	for _, rule := range rules {
		if strings.Contains(rule.Name, "orchestrator") {
			orchestratorParts = append(orchestratorParts, rule.Content)
		}
	}
	combined := strings.Join(orchestratorParts, "\n")
	for _, role := range []string{"gentle-ai-explore", "gentle-ai-worker", "gentle-ai-verify"} {
		if !strings.Contains(combined, role) {
			t.Errorf("rendered Antigravity orchestrator rules missing installed role %q", role)
		}
	}
}
