package agentguidance_test

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/agentguidance"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/reviewassets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// harness-continuity Delivery B (S6, S7, S8) shipped-policy checks through the
// public renders and installed assets. They prove prompt text, not model
// compliance.
const (
	// S6: open blockers end the unit as an honest partial, never a generic
	// terminal decision license, and every budget keeps its own count.
	openBlockersPartial = "leave the unit `status: partial`, naming any genuine human decision; independent authorized work continues."
	separateBudgets     = "Worker, setup, recheck, native, and user budgets stay separate; changed state earns a fresh check, not a reset."

	// S8: phase labels stay automatic; the tool only refines them.
	optionalPhaseTool = "`gentle_odd_phase` is an optional refinement for phases tools cannot show (`authorizing`, `researching`, `deciding`, `closing`), never a per-phase obligation"

	// S7: the oversized-delivery menu belongs to publishing, not to a local
	// work-unit commit, and the feature document stays canonical.
	publishOnlyDelivery = "apply the chosen strategy only when the user asks to publish (push or PR), never before a local work-unit commit"
	piStepSixProjection = "mirror the file and project the visible `todo` at each task transition"
	stepSixProjection   = "mirror the file at each task transition"
)

// orchestratorVariants renders every non-Pi orchestrator in both capability
// variants, exactly as installation selects them.
func orchestratorVariants(t *testing.T) map[string]string {
	t.Helper()
	variants := map[string]string{}
	for _, agent := range catalog.AllAgents() {
		if agent.ID == model.AgentPi || agent.ID == model.AgentConductor {
			continue // Pi's prompt is owned by Gentle Shell; Conductor is catalog-only.
		}
		for _, capability := range []string{"", "small"} {
			rendered, err := agentguidance.RenderOrchestratorWithSource(agent.ID, reviewassets.ReviewExecutionContractFor, capability)
			if err != nil {
				t.Fatalf("RenderOrchestratorWithSource(%q, %q) error = %v", agent.ID, capability, err)
			}
			variants[string(agent.ID)+"/"+map[string]string{"": "capable", "small": "small"}[capability]] = rendered
		}
	}
	return variants
}

// TestRenderedVerificationGateEndsOpenBlockersAsPartial: the correction bound
// still allows one correction and a same-blocker second, but what stays open
// afterwards leaves the unit partial while independent authorized work
// continues; no rendered orchestrator keeps the generic terminal phrase.
func TestRenderedVerificationGateEndsOpenBlockersAsPartial(t *testing.T) {
	t.Parallel()

	for name, rendered := range orchestratorVariants(t) {
		if strings.Contains(rendered, "**Needs your decision**") {
			t.Errorf("%s orchestrator keeps the generic **Needs your decision** terminal result", name)
		}
		for _, want := range []string{
			"**Correction bounds**: verifier blockers get one correction batch that fixes every reported blocker",
			"A second correction runs only when the recheck shows the same blocker still failing; a new finding never earns one.",
			"Blockers still open after that, and a writer's self-review under the same bound, " + openBlockersPartial,
			separateBudgets,
		} {
			if !strings.Contains(rendered, want) {
				t.Errorf("%s orchestrator verification gate is missing %q", name, want)
			}
		}
	}
}

// TestRenderedOrchestratorsCarryNoKeyLearningsRitual: S8 retires the generic
// closing block every delegated prompt had to request; memory-owned learning
// (a worker's own Engram save when the parent supplies a project) stays.
func TestRenderedOrchestratorsCarryNoKeyLearningsRitual(t *testing.T) {
	t.Parallel()

	for name, rendered := range orchestratorVariants(t) {
		if strings.Contains(rendered, "Key Learnings") {
			t.Errorf("%s orchestrator still carries the Key Learnings ritual", name)
		}
	}
	for _, path := range []string{
		"opencode/agents/gentle-ai-explore.md",
		"opencode/agents/gentle-ai-worker.md",
		"opencode/agents/gentle-ai-verify.md",
	} {
		content, err := assets.Read(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(content, "Key Learnings") {
			t.Errorf("%s still asks for a Key Learnings block", path)
		}
	}
	// PRESERVE: the worker keeps its memory-owned learning rule.
	worker, err := assets.Read("opencode/agents/gentle-ai-worker.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(worker, "Use `mem_save` only when the parent supplies a validated project name") {
		t.Error("gentle-ai-worker lost its memory-owned learning rule")
	}
}

// TestRenderRoutingKeepsPhaseToolOptional: Pi's label is inferred from tool
// activity; the phase tool only refines it and is never an obligation per
// phase. No other runtime names the tool.
func TestRenderRoutingKeepsPhaseToolOptional(t *testing.T) {
	t.Parallel()

	for _, agent := range catalog.AllAgents() {
		rendered, err := agentguidance.RenderRouting(agent.ID)
		if err != nil {
			t.Fatalf("RenderRouting(%q) error = %v", agent.ID, err)
		}
		if agent.ID != model.AgentPi {
			if strings.Contains(rendered, "gentle_odd_phase") {
				t.Errorf("%s routing names the Pi-only phase tool", agent.ID)
			}
			continue
		}
		for _, want := range []string{
			"the Gentle Shell prompt label is inferred automatically from the primary session's tool activity",
			optionalPhaseTool,
			"call it from the primary session only when its ODD phase actually changes, never per tool call or on a fixed cadence",
			"It drives the Gentle Shell prompt label only.",
		} {
			if !strings.Contains(rendered, want) {
				t.Errorf("Pi routing phase reporting is missing %q", want)
			}
		}
		if strings.Contains(rendered, "use it to refine that label") {
			t.Error("Pi routing keeps the phase tool as an instruction to use")
		}
	}
}

// TestRenderRoutingAsksDeliveryStrategyOnlyToPublish: an oversized feature
// keeps committing local work units; the delivery strategy is applied when the
// user asks to publish, slice boundaries are still recorded, and the feature
// document stays the canonical state its mirror and todo project.
func TestRenderRoutingAsksDeliveryStrategyOnlyToPublish(t *testing.T) {
	t.Parallel()

	for _, agent := range catalog.AllAgents() {
		rendered, err := agentguidance.RenderRouting(agent.ID)
		if err != nil {
			t.Fatalf("RenderRouting(%q) error = %v", agent.ID, err)
		}
		for _, want := range []string{
			publishOnlyDelivery,
			"record slice boundaries, which commits each pull request holds, in the feature document",
			"The file is the source: a mirror that differs from the file just written is refreshed from the file, never a blocker.",
		} {
			if !strings.Contains(rendered, want) {
				t.Errorf("%s routing is missing %q", agent.ID, want)
			}
		}
		if strings.Contains(rendered, "apply the chosen strategy before the next commit") {
			t.Errorf("%s routing still applies the delivery strategy before a local commit", agent.ID)
		}
		step := stepSixProjection
		if agent.ID == model.AgentPi {
			step = piStepSixProjection
		}
		if !strings.Contains(rendered, "Check an item off only after its outcome and checks were observed; "+step+".") {
			t.Errorf("%s routing step 6 is missing its projection rule %q", agent.ID, step)
		}
	}
}
