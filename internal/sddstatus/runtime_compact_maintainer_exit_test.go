package sddstatus

import (
	"context"
	"strings"
	"testing"
)

// TestMaintainerDecisionExitNamesNoRefusedOperation is #2530's regression guard.
//
// The maintainer_decision exit used to name "rescope or reset the objective".
// That text is emitted from exactly one call site — the DecisionRequired arm of
// compactTerminalState — and runtimeObjectiveRescopeStructurallyPermitted
// refuses every decision-required objective. So the advertised exit named an
// operation the runtime is guaranteed to reject in the only state that can
// produce it, which is the dead end #2530 reported.
//
// The assertion is deliberately tied to the admissibility predicates rather
// than to a fixed string: the exit may name an operation only when that
// operation would actually be accepted for this same status.
func TestMaintainerDecisionExitNamesNoRefusedOperation(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	seedReadyChange(t, repo, "maintainer-exit", "- [ ] 1.1 Work\n")
	store := mustRuntimeStore(t, repo, "maintainer-exit")

	active, err := store.Begin(context.Background(), BeginAttemptRequest{
		RequestID: "begin-maintainer-exit", WorkUnit: "apply-auth",
		EvidenceGoal: "prove the auth runtime", MaxAttempts: 1, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: active.Revision, RequestID: "finish-maintainer-exit", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('7'), Diagnosis: "bounded runtime reproduced the failure",
		HarnessDisposition: HarnessReused, CleanupEvidence: "runtime process group exited",
		ProcessEvidence: "post-run scan found no descendants",
	}); err != nil {
		t.Fatal(err)
	}

	status, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}

	// Guard the premise: this really is the state that emits the exit.
	if !status.DecisionRequired {
		t.Fatalf("precondition lost: a budget-exhausted failed attempt no longer requires a maintainer decision: %#v", status)
	}
	if status.NextAction != RuntimeActionReset {
		t.Fatalf("precondition lost: next_action = %q, want %q", status.NextAction, RuntimeActionReset)
	}

	// The two predicates that decide which operation may legally be named.
	rescopeAdmissible := runtimeObjectiveRescopeStructurallyPermitted(status)
	resetAdmissible := store.runtimeObjectiveResetAdmissible(context.Background(), status)
	if rescopeAdmissible {
		t.Fatalf("precondition lost: rescope became admissible for a decision-required objective, so #2530's contradiction no longer exists")
	}
	if !resetAdmissible {
		t.Fatalf("precondition lost: reset is not admissible either, leaving the state with no continuation at all")
	}

	exit := compactBlockedExitText(CompactBlockMaintainerDecision, "")
	if exit == "" {
		t.Fatal("maintainer_decision shipped a blocked result with no exit at all")
	}

	// A message may name a command only when running it resolves the block.
	if strings.Contains(exit, "rescope") {
		t.Fatalf("maintainer_decision exit names rescope, which runtimeObjectiveRescopeStructurallyPermitted refuses for every decision-required objective:\n%s", exit)
	}

	// It must still hand the caller somewhere runnable: status publishes the
	// admissible continuation as next_action, so the exit points there.
	if !strings.Contains(exit, "gentle-ai sdd-attempt status") {
		t.Fatalf("maintainer_decision exit stopped naming the status command that carries the real continuation:\n%s", exit)
	}
	if !strings.Contains(exit, "next_action") {
		t.Fatalf("maintainer_decision exit does not point at next_action, so the caller must still guess the operation:\n%s", exit)
	}
}
