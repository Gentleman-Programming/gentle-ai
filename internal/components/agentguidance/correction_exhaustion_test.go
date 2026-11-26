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
	// The worker-local stop ends its cycle, not the unit; the parent, not
	// the user, diagnoses and routes the failure without resetting the cycle.
	workerStopEndsCycle = "A worker-local stop (same cause after a real fix, no new hypothesis) ends its cycle, not the unit."
	workerStopRoute     = "You diagnose, not the user: fix inline only if small and clear; else launch a bounded writer with the cause and new hypothesis."
	workerStopNoReset   = "Never repeat or reset. Without a new route, the unit stays blocked."
	// S1 PRESERVE: still-open verifier blockers keep ending the unit partial.
	verifierBlockersPartial = "Blockers still open after that leave the unit `status: partial`, naming any genuine human decision; independent authorized work continues."

	workerProgress = "A failing check is an obstacle: keep correcting while each attempt names a cause from the output and makes a change to address it. Never repeat the same change or stop because of an attempt count. RED construction is separate from fix correction: harness or setup errors get one self-fix; if the test still cannot reproduce the failure, return `status: partial` with the reproduction blocker and its evidence."
	workerStop     = "Stop correcting and return `status: partial` only if the same cause fails again after a real fix and no new hypothesis exists, passing would require weakening, skipping or rewriting the check, or the fix needs work outside the allowed surfaces or crosses an explicit limit."
	workerHandoff  = "Return the failing command and its output, the remaining implementation work, the missing proof, and your diagnosed cause with evidence. Name the cause in `summary`. That ends your cycle, not the unit: the parent decides the next step from your evidence. Do not start a fresh correction cycle or treat a new session as a reset."
)

// Every rendered orchestrator owns the next step after a worker-local stop
// and keeps the separate verifier and review limits.
func TestRenderedGateGivesTheParentTheExhaustedWorkerLoop(t *testing.T) {
	t.Parallel()

	for name, rendered := range orchestratorVariants(t) {
		for _, want := range []string{
			workerStopEndsCycle,
			workerStopRoute,
			workerStopNoReset,
			verifierBlockersPartial,
			// S4 PRESERVE: the bound and the separate budgets are unchanged.
			"A second correction runs only when the recheck shows the same blocker still failing; a new finding never earns one.",
			separateBudgets,
		} {
			if count := strings.Count(rendered, want); count != 1 {
				t.Errorf("%s orchestrator carries %q %d times, want 1", name, want, count)
			}
		}
		for _, banned := range []string{"explicit human authorization", "human-controlled", "a writer's self-review under the same bound", "A writer's exhausted limit", "exhausted worker-local correction limit"} {
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

// The worker corrects while making progress, hands back partial with evidence,
// and keeps the RED construction setup limit.
func TestWorkerReturnsExhaustionWithACauseAndSeparatesREDConstruction(t *testing.T) {
	t.Parallel()

	worker, err := assets.Read("opencode/agents/gentle-ai-worker.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{workerProgress, workerStop, workerHandoff} {
		if count := strings.Count(worker, want); count != 1 {
			t.Errorf("gentle-ai-worker carries %q %d times, want 1", want, count)
		}
	}
	for _, banned := range []string{"explicit human authorization", "human-controlled", "fix it inline", "one correction attempt per failing check"} {
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
