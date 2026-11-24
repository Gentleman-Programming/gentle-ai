package agentguidance_test

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v5/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v5/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v5/internal/components/agentguidance"
	"github.com/gentleman-programming/gentle-ai/v5/internal/components/reviewassets"
	"github.com/gentleman-programming/gentle-ai/v5/internal/model"
)

// harness-continuity S10 parity: rendered text names the thresholds and guards
// the runtime actually enforces, without changing any threshold. These are
// shipped-policy checks, not model-compliance proof.

// TestRenderRoutingNamesPiMeasuredExplorationGate: Pi measures exploration
// (lib/exploration-budget.ts: a one-shot notice past 3 lookups or 40,000
// output bytes) and the context backstop (lib/context-backstop.ts: the lower of
// 150k tokens or 15% of the window), so its routing names those values. Every
// other runtime keeps the unmeasured approximate wording unchanged.
func TestRenderRoutingNamesPiMeasuredExplorationGate(t *testing.T) {
	t.Parallel()

	piWant := []string{
		"understanding that needs more evidence or more than 3 lookups takes one read-only explorer",
		"needs more than 3 lookups (the runtime's exploration notice fires past 3 lookups or 40,000 output bytes), delegate one read-only explorer",
		"- **Long-session backstop:** when the runtime signals it, at the lower of approximately 150k parent-context tokens or 15% of the context window, pause and delegate the next bounded unit.",
	}
	otherWant := []string{
		"more than approximately 5 sequential lookups takes one read-only explorer",
		"needs more than approximately 5 sequential lookups, delegate one read-only explorer",
		"at approximately 150k parent-context tokens, pause and delegate the next bounded unit. This is advisory context guidance, not mechanically observed or enforced",
	}
	for _, agent := range catalog.AllAgents() {
		rendered, err := agentguidance.RenderRouting(agent.ID)
		if err != nil {
			t.Fatalf("RenderRouting(%q) error = %v", agent.ID, err)
		}
		want, absent := otherWant, piWant
		if agent.ID == model.AgentPi {
			want, absent = piWant, otherWant
		}
		for _, clause := range want {
			if !strings.Contains(rendered, clause) {
				t.Errorf("%s routing is missing %q", agent.ID, clause)
			}
		}
		for _, clause := range absent {
			if strings.Contains(rendered, clause) {
				t.Errorf("%s routing carries the other runtime's threshold wording %q", agent.ID, clause)
			}
		}
	}
}

// TestPiReviewContractNamesTheRealPreDeliveryGuard: a review outcome still
// never authorizes delivery, and the text states the Pi RDD-on guard
// (lib/review-delivery-preflight.ts) instead of implying delivery is free.
func TestPiReviewContractNamesTheRealPreDeliveryGuard(t *testing.T) {
	t.Parallel()

	contract, err := reviewassets.ReviewExecutionContractFor(model.AgentPi)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Commit, push, PR, and release remain separate human decisions under ordinary repository policy.",
		"A review outcome is informational and never authorizes delivery.",
		"With RDD on, the pre-delivery guard still blocks a foreground push or PR until the committed range is assessed and any due review is resolved, and never runs it beside candidate-changing or background commands.",
	} {
		if !strings.Contains(contract, want) {
			t.Errorf("Pi review contract is missing %q", want)
		}
	}
}

// TestResearchLifecycleBatchesIndependentProductQuestions: S4 lets
// independent decisions batch when the interface supports it.
func TestResearchLifecycleBatchesIndependentProductQuestions(t *testing.T) {
	t.Parallel()

	lifecycle, err := assets.Read("skills/_shared/research-lifecycle.md")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(lifecycle, "one focused product question at a time") {
		t.Error("research lifecycle still serializes independent product questions")
	}
	for _, want := range []string{
		"Ask focused product questions, batching independent ones when the interface supports it, and wait for the answers;",
		"do not choose for the user or repeat settled decisions",
	} {
		if !strings.Contains(lifecycle, want) {
			t.Errorf("research lifecycle is missing %q", want)
		}
	}
}

// TestSmallGenericStopListIsScopedToTheAffectedWork: the small variant has no
// shared section, so its own stop list must carry the canon's scope.
func TestSmallGenericStopListIsScopedToTheAffectedWork(t *testing.T) {
	t.Parallel()

	rendered, err := agentguidance.RenderOrchestratorWithSource(model.AgentVSCodeCopilot, reviewassets.ReviewExecutionContractFor, "small")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered, "Orchestrator Instructions (Small Model)") {
		t.Fatal("small capability did not select the small variant")
	}
	if strings.Contains(rendered, "Stop only for") {
		t.Error("small variant keeps the unscoped stop list")
	}
	for _, want := range []string{
		// The canon names the exhausted limit beside the other stops.
		"An open human decision, missing consent, a denial, an exhausted limit, an explicit stop, or a failure with no permitted route left halts only the work it names; independent authorized work continues.",
		// PRESERVE: the guards beside it.
		"Do not infer permission for publishing, destructive actions, unrelated scope or native consent from this continuity rule.",
		"Report a concrete blocker and the exact decision needed rather than requesting a generic \"continue\".",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("small variant is missing %q", want)
		}
	}
}

// TestCodexOrchestratorNamesExistingDelegationGates: a Codex install has no
// "hard gate"; its delegation stops are the Delegation Triggers it carries.
func TestCodexOrchestratorNamesExistingDelegationGates(t *testing.T) {
	t.Parallel()

	rendered, err := agentguidance.RenderOrchestratorWithSource(model.AgentCodex, reviewassets.ReviewExecutionContractFor, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, missing := range []string{"hard gate", "hard-gate", "hard delegation gate", "delegation-required", "triggered the gate"} {
		if strings.Contains(strings.ToLower(rendered), missing) {
			t.Errorf("Codex orchestrator names a nonexistent %q", missing)
		}
	}
	for _, want := range []string{
		"#### Delegation Triggers",
		"stop and tell the user that a fired Delegation Trigger requires delegation before continuing.",
		"Do not present inline execution past a fired Delegation Trigger as normal behavior.",
		"It is not the default and it is not a bypass for a fired Delegation Trigger.",
		"When a Delegation Trigger fires and sub-agent tooling is unavailable:",
		"1. Stop the delegated work whose Delegation Trigger fired.",
		"narrow the task so no Delegation Trigger fires before implementation continues.",
		"Do not run triggered work inline as a fallback.",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("Codex orchestrator is missing %q", want)
		}
	}
}
