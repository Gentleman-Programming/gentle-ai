package sddstatus

import (
	"context"
	"strings"
	"testing"
)

// TestRuntimeLedgerErrorsNameTheStatusRoute is the RED-first proof for the
// organic-dx Phase 3b discoverability sweep (task 3b.2). The five SDD ledger
// refusals a caller can hit while driving `sdd-attempt begin/finish/reset`
// already carry every fact needed to recover — the envelope
// `sdd-attempt status` returns already derives the correct next_action for
// each of these blocked states — but until now the error text itself named
// nothing, leaving an agent that only sees the error string with no route
// back into the negotiation. Each of these five sentinel messages must
// suffix the caller-side pointer to that status route.
//
// The pointer names `--cwd <repo> --change <change>` because the bare
// `gentle-ai sdd-attempt status` command is rejected by the CLI for missing
// required flags (internal/cli/sdd_attempt.go requires --cwd and --change).
// A continuation that fails when pasted is worse than none: it costs a round
// trip to discover. `<repo>`/`<change>` mirrors the exact placeholder
// convention already used for this same command in the SDD orchestrator
// prompt assets (e.g. internal/assets/generic/sdd-orchestrator.md).
func TestRuntimeLedgerErrorsNameTheStatusRoute(t *testing.T) {
	const pointer = "run `gentle-ai sdd-attempt status --cwd <repo> --change <change>` — its next_action names the continuation"
	for _, sentinel := range []error{
		ErrRuntimeBudgetExhausted,
		ErrRuntimeAttemptActive,
		ErrRuntimeNoActiveAttempt,
		ErrRuntimeObjectiveDone,
		ErrRuntimeNoObjective,
	} {
		if !strings.Contains(sentinel.Error(), pointer) {
			t.Errorf("sentinel %q does not name the sdd-attempt status route (want it to contain %q)", sentinel.Error(), pointer)
		}
	}
}

// TestStatusRouteActuallyResolvesEachSentinelState is the property guard that
// TestRuntimeLedgerErrorsNameTheStatusRoute above cannot be (#2471).
//
// That test asserts a FORM: the message contains the pointer to
// `sdd-attempt status`. The property that matters is different: pointing at
// status is a real continuation only when status's own next_action is the move
// that unblocks THIS caller. Three refusals passed the form guard while being
// useless, because status answered `begin` for a caller whose failed evidence
// proved the objective was wrong (#1974), and `complete` for a caller with more
// work to do (#2769). The form was right and the advice was worthless.
//
// So this drives each sentinel's real state and asserts that next_action names
// an operation that is genuinely admitted from it. It cannot be written as a
// table over sentinel strings; every case has to build the state, which is
// exactly why the cheap form guard was written instead and why both are kept:
// the form guard catches a refusal that names nothing, this one catches a
// refusal that names the wrong thing.
func TestStatusRouteActuallyResolvesEachSentinelState(t *testing.T) {
	t.Run("budget exhausted routes to a reset that is admitted", func(t *testing.T) {
		store := mustRuntimeStore(t, initRuntimeLedgerRepo(t), "route-exhausted")
		began, err := store.Begin(context.Background(), BeginAttemptRequest{
			RequestID: "exhausted-1", WorkUnit: "apply", EvidenceGoal: "prove it",
			MaxAttempts: 1, MaxChangedLines: 20,
		})
		if err != nil {
			t.Fatal(err)
		}
		exhausted, err := store.Finish(context.Background(), FinishAttemptRequest{
			ExpectedRevision: began.Revision, RequestID: "exhausted-finish", Outcome: AttemptFailed,
			EvidenceRevision: runtimeTestHash('1'), Diagnosis: "failed",
			HarnessDisposition: HarnessReused, CleanupEvidence: "clean", ProcessEvidence: "none",
		})
		if err != nil {
			t.Fatal(err)
		}
		if exhausted.NextAction != RuntimeActionReset {
			t.Fatalf("next_action = %q, want reset", exhausted.NextAction)
		}
		if _, err := store.Reset(context.Background(), ResetObjectiveRequest{
			ExpectedRevision: exhausted.Revision, RequestID: "exhausted-reset",
			Reason: "budget spent", Actor: "maintainer",
		}); err != nil {
			t.Fatalf("status named reset, but reset is refused from this state: %v", err)
		}
	})

	t.Run("active attempt routes to a settle that is admitted", func(t *testing.T) {
		store := mustRuntimeStore(t, initRuntimeLedgerRepo(t), "route-active")
		began, err := store.Begin(context.Background(), BeginAttemptRequest{
			RequestID: "active-1", WorkUnit: "apply", EvidenceGoal: "prove it",
			MaxAttempts: 2, MaxChangedLines: 20,
		})
		if err != nil {
			t.Fatal(err)
		}
		if began.NextAction != RuntimeActionFinish || began.ActiveAttempt == nil {
			t.Fatalf("next_action = %q with active = %v, want finish", began.NextAction, began.ActiveAttempt)
		}
		if _, err := store.Finish(context.Background(), FinishAttemptRequest{
			ExpectedRevision: began.Revision, RequestID: "active-finish", Outcome: AttemptFailed,
			EvidenceRevision: runtimeTestHash('2'), Diagnosis: "failed",
			HarnessDisposition: HarnessReused, CleanupEvidence: "clean", ProcessEvidence: "none",
		}); err != nil {
			t.Fatalf("status named finish, but finish is refused from this state: %v", err)
		}
	})

	t.Run("completed objective routes to an advance that is admitted", func(t *testing.T) {
		store := mustRuntimeStore(t, initRuntimeLedgerRepo(t), "route-complete")
		began, err := store.Begin(context.Background(), BeginAttemptRequest{
			RequestID: "complete-1", WorkUnit: "apply", EvidenceGoal: "prove it",
			MaxAttempts: 2, MaxChangedLines: 400,
		})
		if err != nil {
			t.Fatal(err)
		}
		passed, err := store.Finish(context.Background(), FinishAttemptRequest{
			ExpectedRevision: began.Revision, RequestID: "complete-finish", Outcome: AttemptPassed,
			EvidenceRevision: runtimeTestHash('3'), Diagnosis: "passed",
			HarnessDisposition: HarnessReused, CleanupEvidence: "clean", ProcessEvidence: "none",
		})
		if err != nil {
			t.Fatal(err)
		}
		if passed.NextAction != RuntimeActionComplete {
			t.Fatalf("next_action = %q, want complete", passed.NextAction)
		}
		// `complete` is honest for this caller and is NOT a dead end: the
		// successor objective is admitted, which is what #2769's refusal now
		// says out loud. This is the case the form guard could never express,
		// because the sentinel text is identical either way.
		if _, err := store.Begin(context.Background(), BeginAttemptRequest{
			ExpectedRevision: passed.Revision, RequestID: "complete-advance", WorkUnit: "verify",
			EvidenceGoal: "prove it", MaxAttempts: 2, MaxChangedLines: 400,
		}); err != nil {
			t.Fatalf("a completed objective must still admit a distinct-work-unit successor: %v", err)
		}
	})
}
