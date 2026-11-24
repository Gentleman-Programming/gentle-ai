package agentguidance_test

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v5/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v5/internal/components/agentguidance"
	"github.com/gentleman-programming/gentle-ai/v5/internal/components/reviewassets"
	"github.com/gentleman-programming/gentle-ai/v5/internal/model"
)

// harness-continuity S3/C6: missing proof forbids a completion claim, never
// authorized implementation, so no rendered orchestrator lists "unavailable
// required proof" as a reason to stop the work. The capable variants state the
// canonical continuity sentence in their Authorized work continuity section,
// in place of the old unscoped stop list. The small generic variant carries no
// shared section by design, so only the absence applies to it. Shipped-policy
// text through the public renderer, not model-compliance proof.
func TestRenderedOrchestratorNeverStopsForUnavailableProof(t *testing.T) {
	t.Parallel()

	const (
		unscopedProofStop = "unavailable required proof"
		canonicalContinue = "Authorized work continues: runnable in-scope steps run"
		smallModelHeading = "Orchestrator Instructions (Small Model)"
	)

	for _, agent := range catalog.AllAgents() {
		if agent.ID == model.AgentPi || agent.ID == model.AgentConductor {
			// Pi's prompt is owned by Gentle Shell; Conductor is catalog-only.
			continue
		}
		for _, capability := range []string{"", "small"} {
			name := string(agent.ID) + "/capable"
			if capability != "" {
				name = string(agent.ID) + "/" + capability
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				rendered, err := agentguidance.RenderOrchestratorWithSource(agent.ID, reviewassets.ReviewExecutionContractFor, capability)
				if err != nil {
					t.Fatalf("RenderOrchestratorWithSource(%q, %q) error = %v", agent.ID, capability, err)
				}
				if strings.Contains(rendered, unscopedProofStop) {
					t.Errorf("rendered %s orchestrator still stops for %q", name, unscopedProofStop)
				}
				if strings.Contains(rendered, smallModelHeading) {
					return
				}
				section := authorizedContinuitySection(rendered)
				if section == "" {
					t.Fatalf("rendered %s orchestrator has no Authorized work continuity section", name)
				}
				if !strings.Contains(section, canonicalContinue) {
					t.Errorf("rendered %s Authorized work continuity is missing the canonical continuity sentence %q", name, canonicalContinue)
				}
				if strings.Contains(section, "Stop only for") {
					t.Errorf("rendered %s Authorized work continuity keeps the restated stop list", name)
				}
				// PRESERVE: the guards the canonical sentence does not carry.
				for _, guard := range []string{
					"Do not infer permission for publishing, destructive actions, unrelated scope or native consent from this continuity rule.",
					"Report a concrete blocker and the exact decision needed rather than requesting a generic \"continue\".",
					"end the turn when no independent in-scope work remains",
					"Retain its identity and resume on its completion event without asking the human",
					"never poll or claim completion while checks remain pending",
				} {
					if !strings.Contains(section, guard) {
						t.Errorf("rendered %s Authorized work continuity lost %q", name, guard)
					}
				}
			})
		}
	}
}

// authorizedContinuitySection returns the Authorized work continuity body of a
// rendered orchestrator, up to the next heading of the same or a higher level.
func authorizedContinuitySection(content string) string {
	const heading = "### Authorized work continuity"
	start := strings.Index(content, heading)
	if start < 0 {
		return ""
	}
	rest := content[start+len(heading):]
	end := len(rest)
	for _, next := range []string{"\n### ", "\n## ", "\n# "} {
		if at := strings.Index(rest, next); at >= 0 && at < end {
			end = at
		}
	}
	return rest[:end]
}
