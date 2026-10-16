package reviewassets

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// TestReviewContractsTellTheAgentToSelectItsLenses is verify-always-rdd-high
// S8: every rendered review contract tells the agent to pick the pertinent 4R
// lenses by what it touched and how, defines each lens by consequence, and
// shows where the selection travels (STATUS flags, or the Pi facade input).
func TestReviewContractsTellTheAgentToSelectItsLenses(t *testing.T) {
	for _, agent := range []model.AgentID{model.AgentClaudeCode, model.AgentCodex, model.AgentOpenCode, model.AgentPi} {
		t.Run(string(agent), func(t *testing.T) {
			contract := ContractFor(agent)
			required := []string{
				"## Lens selection",
				"Pick every lens pertinent to what you touched and how you touched it, and only those",
				"`risk` when a mistake could expose, lose, or irreversibly change something",
				"`resilience` when it changes behavior under failure",
				"`readability` when it changes what others read or depend on",
				"`reliability` when it changes logic or observable behavior",
				"Requirement compliance is verify's job before review",
			}
			if agent == model.AgentPi {
				required = append(required, `"lenses":[...],"lensesReason":"<what you touched and how>"`)
			} else {
				required = append(required, `--next-transition --lenses <r,...> --lenses-reason "<what you touched and how>"`, "`start_lens_selection`")
			}
			for _, want := range required {
				if !strings.Contains(contract, want) {
					t.Fatalf("%s review contract omits %q", agent, want)
				}
			}
			if strings.Contains(contract, "A deterministic blocker needs no refuter; inferential blockers share one read-only refuter batch.") {
				t.Fatalf("%s review contract still says every deterministic blocker skips the refuter", agent)
			}
		})
	}
}

// TestReviewContractsEnterReviewOnlyWhenDue is verify-always-rdd-high S2: the
// rendered entry rule starts a review only for a review_due (high-risk)
// candidate or an explicit user request, never once per candidate.
func TestReviewContractsEnterReviewOnlyWhenDue(t *testing.T) {
	for _, agent := range []model.AgentID{model.AgentClaudeCode, model.AgentCodex, model.AgentOpenCode, model.AgentPi} {
		t.Run(string(agent), func(t *testing.T) {
			contract := ContractFor(agent)
			for _, want := range []string{
				"Review is an extra outside view for high-risk work, never a per-candidate ritual.",
				"or the user explicitly asks for a review of this candidate; otherwise do not start one, because verify already covered it by the risk tier.",
			} {
				if !strings.Contains(contract, want) {
					t.Fatalf("%s entry rule omits %q", agent, want)
				}
			}
			for _, banned := range []string{"Enter this lifecycle once per candidate", "Do this once per candidate", "never skip the preflight because the user did not ask for a review"} {
				if strings.Contains(strings.ReplaceAll(contract, "Never skip", "never skip"), banned) {
					t.Fatalf("%s entry rule still reviews every candidate (%q)", agent, banned)
				}
			}
		})
	}
}
