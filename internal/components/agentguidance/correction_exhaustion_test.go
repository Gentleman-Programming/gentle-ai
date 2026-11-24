package agentguidance_test

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v5/internal/assets"
)

// orchestrator-owns-correction-exhaustion (S1–S4) shipped-policy checks through
// the public renders and the installed worker asset. They prove prompt text,
// not model compliance.
const (
	// The wording is compact because the gate is always-on core: Antigravity's
	// always-on rules must stay within AlwaysOnBudgetBytes without demoting the
	// routing core (S4: never raise prompt budgets).
	//
	// S1: the writer's exhausted limit ends its cycle, not the unit, and the
	// parent, not the user, decides the next step.
	exhaustionEndsCycle = "A writer's exhausted limit ends its cycle, not the unit: you, not the user, name and act on the loop's cause:"
	// S2: Gentle AI has no delegation modes, so the route is mode-free: fix
	// inline only when small and understood, else launch a new bounded
	// writer with the diagnosis and a new hypothesis, never a repeat.
	exhaustionRoute    = "fix inline only when small and understood, else launch a new bounded writer with the diagnosis and a new hypothesis, never a repeat;"
	exhaustionNoRepeat = "a recurring cause or no hypothesis blocks the unit."
	// S1 PRESERVE: still-open verifier blockers keep ending the unit partial.
	verifierBlockersPartial = "Blockers still open after that leave the unit `status: partial`, naming any genuine human decision; independent authorized work continues."

	// S1/S3 worker side.
	workerKeepsLimit      = "Make one correction attempt per failing check, and a second only if the same check still fails after a real fix; then stop and return `status: partial` with the failing command and its output, never looping."
	workerDiagnosedCause  = "Name in `summary` the loop's diagnosed cause (for example a wrong test, a wrong hypothesis, missing context, a setup or environment error, or a real blocker). Exhaustion ends this cycle, not the unit: the parent decides the next step from your evidence."
	workerREDConstruction = "Making a new test fail for the expected reason is RED construction, not a fix correction: its own harness or setup errors get the setup bound (one self-fix), separate from the fix's limit; if the test still cannot reproduce the failure, return `status: partial` naming a reproduction blocker, not exhausted corrections."
)

// TestRenderedGateGivesTheParentTheExhaustedWorkerLoop: every rendered
// orchestrator owns the next step after a worker's exhausted limit, routes it
// without naming delegation modes Gentle AI does not have, forbids repeating the attempt, and keeps the existing
// correction bound and separate budgets (S4).
func TestRenderedGateGivesTheParentTheExhaustedWorkerLoop(t *testing.T) {
	t.Parallel()

	for name, rendered := range orchestratorVariants(t) {
		for _, want := range []string{
			exhaustionEndsCycle,
			exhaustionRoute,
			exhaustionNoRepeat,
			verifierBlockersPartial,
			// S4 PRESERVE: the bound and the separate budgets are unchanged.
			"A second correction runs only when the recheck shows the same blocker still failing; a new finding never earns one.",
			separateBudgets,
		} {
			if count := strings.Count(rendered, want); count != 1 {
				t.Errorf("%s orchestrator carries %q %d times, want 1", name, want, count)
			}
		}
		for _, banned := range []string{"explicit human authorization", "human-controlled", "a writer's self-review under the same bound"} {
			if strings.Contains(rendered, banned) {
				t.Errorf("%s orchestrator still says %q", name, banned)
			}
		}
		bounds := correctionBoundsBullet(rendered)
		if bounds == "" {
			t.Errorf("%s orchestrator has no Correction bounds bullet", name)
		}
		for _, mode := range []string{"full delegation", "organic", "Organic", "delegation mode", "relaunch"} {
			if strings.Contains(bounds, mode) {
				t.Errorf("%s Correction bounds names %q (no delegation modes in Gentle AI; launch, never relaunch)", name, mode)
			}
		}
	}
}

// TestWorkerReturnsExhaustionWithACauseAndSeparatesREDConstruction: the
// worker keeps its two-attempt limit, returns a diagnosed cause that ends only
// its cycle, and treats RED construction under the setup bound.
func TestWorkerReturnsExhaustionWithACauseAndSeparatesREDConstruction(t *testing.T) {
	t.Parallel()

	worker, err := assets.Read("opencode/agents/gentle-ai-worker.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{workerKeepsLimit, workerDiagnosedCause, workerREDConstruction} {
		if count := strings.Count(worker, want); count != 1 {
			t.Errorf("gentle-ai-worker carries %q %d times, want 1", want, count)
		}
	}
	for _, banned := range []string{"explicit human authorization", "human-controlled", "fix it inline"} {
		if strings.Contains(worker, banned) {
			t.Errorf("gentle-ai-worker still says %q", banned)
		}
	}
}

// correctionBoundsBullet returns the rendered Correction bounds bullet line.
func correctionBoundsBullet(rendered string) string {
	for _, line := range strings.Split(rendered, "\n") {
		if strings.HasPrefix(line, "- **Correction bounds**:") {
			return line
		}
	}
	return ""
}
