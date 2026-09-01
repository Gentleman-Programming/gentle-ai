package sddstatus

import (
	"strings"
	"testing"
)

// TestExhaustedAttemptBudgetBlocksApplyOnly is #2902's reproduction.
//
// The reporter's change was DONE: every implementation task complete, the
// merged candidate green on repository CI, receipt-driven review off both
// globally and clone-locally. What remained was one historical sdd-attempt
// objective sitting at blocked(maintainer_decision) from changed-line
// accounting on work that had already landed.
//
// Status projected that historical blocker onto Apply, Verify AND Archive, and
// offered only resolve-blockers. The change could never be verified and never
// be archived.
//
// The attempt ledger governs one thing: whether a bounded implementation work
// unit may OPEN. An exhausted budget is a true and permanent answer to that
// question, so it blocks Apply. It says nothing about whether independent
// verification of the finished change may run, or whether a verified change
// may close. Those have their own dependencies, and those dependencies are the
// ones entitled to block them: a change whose budget was exhausted mid-flight
// still has incomplete tasks and no passing verify, so Verify and Archive stay
// blocked on their own merits. Nothing is laundered by letting them answer for
// themselves.
func TestExhaustedAttemptBudgetBlocksApplyOnly(t *testing.T) {
	status := &Status{
		RuntimeStatus: &RuntimeStatus{
			Change: "budget-exhausted", DecisionRequired: true, NextAction: RuntimeActionReset,
		},
		Dependencies: Dependencies{
			Apply: DependencyAllDone, Verify: DependencyAllDone, Archive: DependencyReady,
		},
	}
	applyNativeRuntimeRouting(status)

	if status.Dependencies.Apply != DependencyBlocked {
		t.Fatalf("Apply = %q, want %q: an exhausted budget genuinely does stop the next work unit", status.Dependencies.Apply, DependencyBlocked)
	}
	if status.Dependencies.Verify == DependencyBlocked {
		t.Fatal("Verify is blocked by a historical attempt-budget decision; the attempt ledger governs whether a work unit may open, not whether a finished change may be verified (#2902)")
	}
	if status.Dependencies.Archive == DependencyBlocked {
		t.Fatal("Archive is blocked by a historical attempt-budget decision; a change whose work is complete and verified must be able to close (#2902)")
	}
	if status.NextRecommended == "resolve-blockers" {
		t.Fatalf("NextRecommended = %q; with verify done and archive ready, the ledger's budget decision is not what this change should do next (#2902)", status.NextRecommended)
	}

	// The blocker stays visible and auditable. Silencing it would be the
	// opposite defect.
	if len(status.BlockedReasons) == 0 {
		t.Fatal("the attempt-budget decision vanished from BlockedReasons; it must stay auditable, it just must not gate verify and archive")
	}
	if !strings.Contains(strings.Join(status.BlockedReasons, "\n"), "maintainer_decision") {
		t.Fatalf("BlockedReasons does not name the decision: %v", status.BlockedReasons)
	}
}

// TestExhaustedAttemptBudgetStillStopsUnfinishedWork is the other half: when
// the work is NOT done, the ordinary dependencies still block, and scoping the
// attempt blocker to Apply must not have loosened them.
func TestExhaustedAttemptBudgetStillStopsUnfinishedWork(t *testing.T) {
	status := &Status{
		RuntimeStatus: &RuntimeStatus{
			Change: "budget-exhausted-midflight", DecisionRequired: true, NextAction: RuntimeActionReset,
		},
		Dependencies: Dependencies{
			Apply: DependencyReady, Verify: DependencyReady, Archive: DependencyReady,
		},
	}
	applyNativeRuntimeRouting(status)

	if status.Dependencies.Apply != DependencyBlocked {
		t.Fatalf("Apply = %q, want %q", status.Dependencies.Apply, DependencyBlocked)
	}
	if status.NextRecommended != "resolve-blockers" {
		t.Fatalf("NextRecommended = %q, want resolve-blockers: apply is blocked and the change is not finished, so the budget decision IS what to do next", status.NextRecommended)
	}
}
