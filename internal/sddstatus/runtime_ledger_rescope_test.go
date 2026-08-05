package sddstatus

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestRuntimeLedgerRescopeRecoversZeroDriftDeadlock is the RED-then-GREEN
// reproduction of #2298: attempt 1 discovers mid-work that the required
// scope exceeds the ceiling, reverts every temporary change back to the
// exact original candidate (zero drift), and settles interrupted with 0
// changed lines. Status then shows one remaining ordinal, decision_required
// false, complete false, next_action begin -- but begin can only repeat the
// SAME oversized objective (any changed param hits ErrRuntimeObjectiveChange)
// and reset is ALSO refused (ErrRuntimeResetNotAllowed: no drift, not
// decision-required, not complete). That dead end is the RED baseline this
// test captures before proving AUDITED NARROWING RESCOPE clears it.
func TestRuntimeLedgerRescopeRecoversZeroDriftDeadlock(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "rescope-2298")
	if err != nil {
		t.Fatal(err)
	}

	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "oversized-begin-1", WorkUnit: "oversized-scope",
		EvidenceGoal: "prove bounded apply scope", MaxAttempts: 2, MaxChangedLines: 400,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Attempt 1 discovers the required scope exceeds the ceiling and reverts
	// every temporary change back to the exact original candidate before
	// settling: zero drift, zero changed lines.
	interrupted, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "oversized-finish-1", Outcome: AttemptInterrupted,
		EvidenceRevision: runtimeTestHash('1'), Diagnosis: "required scope measured 449 changed lines against a 400-line ceiling; reverted",
		HarnessDisposition: HarnessInvalidated, CleanupEvidence: "every temporary change was reverted before settling",
		ProcessEvidence: "post-revert process scan found no surviving descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	if interrupted.CumulativeChangedLines != 0 || interrupted.DecisionRequired || interrupted.Complete ||
		interrupted.NextAction != RuntimeActionBegin || interrupted.CumulativeAttempts != 1 || interrupted.LifetimeAttempts != 1 {
		t.Fatalf("pre-rescope interrupted status = %#v", interrupted)
	}
	oldObjectiveID := interrupted.Objective.ID

	// RED: begin against a narrower ceiling is refused as a changed objective.
	_, err = store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: interrupted.Revision, RequestID: "oversized-begin-2", WorkUnit: "oversized-scope",
		EvidenceGoal: "prove bounded apply scope", MaxAttempts: 2, MaxChangedLines: 100,
	})
	if !errors.Is(err, ErrRuntimeObjectiveChange) {
		t.Fatalf("narrower begin error = %v, want ErrRuntimeObjectiveChange", err)
	}
	if !strings.Contains(err.Error(), "gentle-ai sdd-attempt rescope") || strings.Contains(err.Error(), "gentle-ai sdd-attempt reset") {
		t.Fatalf("dead-end refusal does not name rescope (and only rescope): %v", err)
	}

	// RED: reset is refused too -- the candidate has not drifted.
	_, err = store.Reset(context.Background(), ResetObjectiveRequest{
		ExpectedRevision: interrupted.Revision, RequestID: "oversized-reset-1",
		Reason: "attempted elective reset with no drift", Actor: "maintainer",
	})
	if !errors.Is(err, ErrRuntimeResetNotAllowed) {
		t.Fatalf("reset error = %v, want ErrRuntimeResetNotAllowed", err)
	}
	afterRedRecords := countRuntimeRecords(t, store.Dir)
	if afterRedRecords != 2 {
		t.Fatalf("RED probes mutated the ledger: records=%d, want 2", afterRedRecords)
	}

	// GREEN: a maintainer-authorized narrower rescope closes the dead end.
	rescopeRequest := RescopeObjectiveRequest{
		ExpectedRevision: interrupted.Revision, RequestID: "oversized-rescope-1",
		WorkUnit: "narrower-apply-scope", EvidenceGoal: "prove a 100-line bounded slice",
		MaxAttempts: 2, MaxChangedLines: 100,
		Reason: "maintainer split the oversized objective into a narrower successor", Actor: "maintainer",
	}
	rescoped, err := store.Rescope(context.Background(), rescopeRequest)
	if err != nil {
		t.Fatalf("maintainer rescope refused the zero-drift dead end: %v", err)
	}
	if rescoped.Objective == nil || rescoped.Objective.ID == oldObjectiveID || rescoped.Objective.WorkUnit != "narrower-apply-scope" ||
		rescoped.Objective.MaxAttempts != 2 || rescoped.Objective.MaxChangedLines != 100 || rescoped.ObjectiveGeneration != 2 {
		t.Fatalf("rescoped objective = %#v", rescoped.Objective)
	}
	// Mutation proof (a): carry-forward counters are numerically UNCHANGED,
	// never zeroed -- exact value assertions, not just "not zero".
	if rescoped.CumulativeAttempts != 1 || rescoped.CumulativeChangedLines != 0 ||
		rescoped.LifetimeAttempts != 1 || rescoped.LifetimeChangedLines != 0 {
		t.Fatalf("rescope did not carry cumulative/lifetime counters forward unchanged: %#v", rescoped)
	}
	if rescoped.DecisionRequired || rescoped.Complete || rescoped.NextAction != RuntimeActionBegin || rescoped.ActiveAttempt != nil {
		t.Fatalf("post-rescope status = %#v", rescoped)
	}
	if rescoped.LastRescope == nil || rescoped.LastRescope.Revision != rescoped.Revision ||
		rescoped.LastRescope.PreviousObjectiveID != oldObjectiveID || rescoped.LastRescope.Reason != rescopeRequest.Reason ||
		rescoped.LastRescope.Actor != rescopeRequest.Actor || rescoped.LastRescope.MaxChangedLines != 100 {
		t.Fatalf("rescope audit context = %#v", rescoped.LastRescope)
	}
	// History is immutable: the reverted attempt still belongs to the OLD
	// objective ID, and no attempt yet exists under the new one.
	if len(rescoped.Attempts) != 1 || rescoped.Attempts[0].ObjectiveID != oldObjectiveID {
		t.Fatalf("rescope lost or mutated immutable attempt history: %#v", rescoped.Attempts)
	}

	// Exact replay is idempotent.
	replayed, err := store.Rescope(context.Background(), rescopeRequest)
	if err != nil || replayed.Revision != rescoped.Revision || countRuntimeRecords(t, store.Dir) != 3 {
		t.Fatalf("rescope replay = %#v err=%v records=%d", replayed, err, countRuntimeRecords(t, store.Dir))
	}

	// The landmine: the very next begin under the rescoped objective must
	// succeed normally (fresh candidate capture validated against the
	// objective's own recorded InitialCandidate*, not a terminal-provenance
	// chase through the OLD objective's attempts).
	next, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: rescoped.Revision, RequestID: "oversized-begin-3", WorkUnit: "narrower-apply-scope",
		EvidenceGoal: "prove a 100-line bounded slice", MaxAttempts: 2, MaxChangedLines: 100,
	})
	if err != nil {
		t.Fatalf("begin immediately after rescope was refused: %v", err)
	}
	if next.ActiveAttempt == nil || next.ActiveAttempt.Ordinal != 2 || next.ActiveAttempt.ObjectiveID != rescoped.Objective.ID ||
		next.CumulativeAttempts != 2 || next.LifetimeAttempts != 2 || next.NextOrdinal != 3 {
		t.Fatalf("post-rescope begin status = %#v", next)
	}
}

// TestRuntimeLedgerRescopeNarrowsFailedVerificationToTestOnlyRemediation is
// #2296 part 2's distinct scenario reusing the exact same rescope mechanism:
// after an admitted failed independent verification (zero drift once
// settled), a maintainer-authorized bounded test-only remediation objective
// with a narrower scope has no legal transition today, for the identical
// structural reason as #2298.
func TestRuntimeLedgerRescopeNarrowsFailedVerificationToTestOnlyRemediation(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "rescope-2296-part2")
	if err != nil {
		t.Fatal(err)
	}

	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "verify-begin-1", WorkUnit: "independent-verification",
		EvidenceGoal: "independently verify the applied change", MaxAttempts: 2, MaxChangedLines: 400,
	})
	if err != nil {
		t.Fatal(err)
	}
	failed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "verify-finish-1", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('2'), Diagnosis: "independent verification failed with the workspace unchanged",
		HarnessDisposition: HarnessReused, CleanupEvidence: "verification harness exited cleanly",
		ProcessEvidence: "post-verification process scan found no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	if failed.CumulativeChangedLines != 0 || failed.DecisionRequired || failed.Complete || failed.NextAction != RuntimeActionBegin {
		t.Fatalf("pre-rescope failed-verification status = %#v", failed)
	}

	// Both dead-end probes reproduce, exactly as in #2298.
	if _, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: failed.Revision, RequestID: "verify-begin-2", WorkUnit: "independent-verification",
		EvidenceGoal: "independently verify the applied change", MaxAttempts: 2, MaxChangedLines: 50,
	}); !errors.Is(err, ErrRuntimeObjectiveChange) {
		t.Fatalf("narrower begin error = %v, want ErrRuntimeObjectiveChange", err)
	}
	if _, err := store.Reset(context.Background(), ResetObjectiveRequest{
		ExpectedRevision: failed.Revision, RequestID: "verify-reset-1", Reason: "attempted elective reset", Actor: "maintainer",
	}); !errors.Is(err, ErrRuntimeResetNotAllowed) {
		t.Fatalf("reset error = %v, want ErrRuntimeResetNotAllowed", err)
	}

	rescoped, err := store.Rescope(context.Background(), RescopeObjectiveRequest{
		ExpectedRevision: failed.Revision, RequestID: "verify-rescope-1",
		WorkUnit: "test-only-remediation", EvidenceGoal: "bounded test-only remediation of the verification gap",
		MaxAttempts: 2, MaxChangedLines: 50,
		Reason: "maintainer authorized a bounded test-only remediation narrower than the failed verification scope", Actor: "maintainer",
	})
	if err != nil {
		t.Fatalf("maintainer rescope refused the failed-verification dead end: %v", err)
	}
	if rescoped.Objective == nil || rescoped.Objective.WorkUnit != "test-only-remediation" ||
		rescoped.Objective.MaxAttempts != 2 || rescoped.Objective.MaxChangedLines != 50 {
		t.Fatalf("rescoped verification-remediation objective = %#v", rescoped.Objective)
	}
	if rescoped.CumulativeAttempts != 1 || rescoped.CumulativeChangedLines != 0 {
		t.Fatalf("rescope did not carry forward verification history unchanged: %#v", rescoped)
	}

	next, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: rescoped.Revision, RequestID: "verify-begin-3", WorkUnit: "test-only-remediation",
		EvidenceGoal: "bounded test-only remediation of the verification gap", MaxAttempts: 2, MaxChangedLines: 50,
	})
	if err != nil {
		t.Fatalf("begin after verification rescope was refused: %v", err)
	}
	if next.ActiveAttempt == nil || next.ActiveAttempt.ObjectiveID != rescoped.Objective.ID {
		t.Fatalf("post-rescope verification begin status = %#v", next)
	}
}

// TestRuntimeLedgerRescopeRefusesWideningMaxChangedLines is mutation proof
// (b): a widened max_changed_lines is refused with rescope's OWN sentinel,
// never a reused generic one.
func TestRuntimeLedgerRescopeRefusesWideningMaxChangedLines(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "rescope-widen-lines")
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "widen-begin-1", WorkUnit: "bounded-scope",
		EvidenceGoal: "prove widening refusal", MaxAttempts: 2, MaxChangedLines: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	interrupted, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "widen-finish-1", Outcome: AttemptInterrupted,
		EvidenceRevision: runtimeTestHash('3'), Diagnosis: "interrupted with the workspace unchanged",
		HarnessDisposition: HarnessInvalidated, CleanupEvidence: "no executor process was ever spawned",
		ProcessEvidence: "pre-launch process scan found no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Rescope(context.Background(), RescopeObjectiveRequest{
		ExpectedRevision: interrupted.Revision, RequestID: "widen-rescope-1",
		WorkUnit: "bounded-scope", EvidenceGoal: "prove widening refusal",
		MaxAttempts: 2, MaxChangedLines: 101,
		Reason: "attempted a widened ceiling", Actor: "maintainer",
	})
	if !errors.Is(err, ErrRuntimeRescopeWidened) {
		t.Fatalf("widened max-changed-lines rescope error = %v, want ErrRuntimeRescopeWidened", err)
	}
	if errors.Is(err, ErrRuntimeResetNotAllowed) || errors.Is(err, ErrRuntimeObjectiveChange) {
		t.Fatalf("widening refusal reused a generic sentinel: %v", err)
	}

	// max_attempts is narrowing-only too (this slice's documented choice: the
	// ratified decision only names max_changed_lines explicitly, but an
	// uncontrolled max_attempts widen would be the identical attempt-count
	// laundering the decision rejected, so both are narrowing-only).
	_, err = store.Rescope(context.Background(), RescopeObjectiveRequest{
		ExpectedRevision: interrupted.Revision, RequestID: "widen-rescope-2",
		WorkUnit: "bounded-scope", EvidenceGoal: "prove widening refusal",
		MaxAttempts: 3, MaxChangedLines: 100,
		Reason: "attempted a widened attempt budget", Actor: "maintainer",
	})
	if !errors.Is(err, ErrRuntimeRescopeWidened) {
		t.Fatalf("widened max-attempts rescope error = %v, want ErrRuntimeRescopeWidened", err)
	}
	status, statusErr := store.Status()
	if statusErr != nil || status.Revision != interrupted.Revision || countRuntimeRecords(t, store.Dir) != 2 {
		t.Fatalf("refused widenings mutated the ledger: status=%#v err=%v records=%d", status, statusErr, countRuntimeRecords(t, store.Dir))
	}
}

// TestRuntimeLedgerRescopeReplayRefusesForgedWidenedRecord is mutation proof
// (c): validateRuntimeRecordShape alone only proves a rescope event is
// internally self-consistent (max <= its own claimed "previous"). A forged
// record that lies about "previous" to make a real widen look like a narrow
// passes that shape check -- this test proves replay independently
// recomputes narrowing against the REPLAYED objective and rejects it anyway.
func TestRuntimeLedgerRescopeReplayRefusesForgedWidenedRecord(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "rescope-forged-replay")
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "forge-begin-1", WorkUnit: "forge-scope",
		EvidenceGoal: "prove forged replay rejection", MaxAttempts: 2, MaxChangedLines: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	interrupted, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "forge-finish-1", Outcome: AttemptInterrupted,
		EvidenceRevision: runtimeTestHash('4'), Diagnosis: "interrupted with the workspace unchanged",
		HarnessDisposition: HarnessInvalidated, CleanupEvidence: "no executor process was ever spawned",
		ProcessEvidence: "pre-launch process scan found no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	objective := interrupted.Objective
	last := interrupted.Attempts[len(interrupted.Attempts)-1]

	generation := interrupted.ObjectiveGeneration + 1
	forgedObjectiveID := runtimeObjectiveID(store.Change, "forge-scope", "prove forged replay rejection", last.FinishCandidateIdentity, generation)
	request := RescopeObjectiveRequest{
		ExpectedRevision: interrupted.Revision, RequestID: "forge-rescope-1",
		WorkUnit: "forge-scope", EvidenceGoal: "prove forged replay rejection",
		MaxAttempts: 2, MaxChangedLines: 300,
		Reason: "forged carry-forward claiming a widened previous ceiling", Actor: "attacker",
	}
	forgedRecord := runtimeRecord{
		Schema: runtimeRecordSchema, Change: store.Change, PreviousRevision: interrupted.Revision,
		Operation: runtimeOperationRescope, RequestID: request.RequestID,
		RequestDigest: runtimeValueHash("gentle-ai.sdd-runtime-rescope-request/v1", request),
		Rescope: &runtimeRescopeEvent{
			PreviousObjectiveID: objective.ID, PreviousGeneration: objective.Generation,
			// Forged: the record CLAIMS the previous ceiling was 9000 (so its
			// own 300 looks like a narrowing), but the real replayed
			// objective's ceiling is only 100.
			PreviousMaxAttempts: objective.MaxAttempts, PreviousMaxChangedLines: 9000,
			RescopeCandidateIdentity: last.FinishCandidateIdentity, RescopeCandidateTree: last.FinishCandidateTree,
			ObjectiveID: forgedObjectiveID, ObjectiveGeneration: generation,
			WorkUnit: request.WorkUnit, EvidenceGoal: request.EvidenceGoal,
			MaxAttempts: request.MaxAttempts, MaxChangedLines: request.MaxChangedLines,
			Reason: request.Reason, Actor: request.Actor,
		},
	}
	// The forged record IS internally self-consistent: shape validation alone
	// cannot see the lie, because 300 <= the record's own claimed 9000.
	if err := validateRuntimeRecordShape(forgedRecord); err != nil {
		t.Fatalf("forged record unexpectedly failed shape validation: %v", err)
	}

	revision, payload, err := runtimeRecordRevision(forgedRecord)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ensureDirectories(); err != nil {
		t.Fatal(err)
	}
	if err := store.publishRecord(revision, payload); err != nil {
		t.Fatal(err)
	}
	if err := store.publishHead(revision); err != nil {
		t.Fatal(err)
	}

	_, err = store.Status()
	if err == nil || !strings.Contains(err.Error(), "widens") {
		t.Fatalf("replay of forged widened rescope = %v, want a rejection naming the widened budget", err)
	}
}

// TestRuntimeLedgerRescopeCarriedBudgetBindsTheNextBegin is mutation proof
// (d): carry-forward is not cosmetic. Rescoping to a ceiling already at or
// below the carried-forward CumulativeChangedLines must bind -- the very
// next begin is refused budget-exhausted.
func TestRuntimeLedgerRescopeCarriedBudgetBindsTheNextBegin(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "rescope-budget-binds")
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "bind-begin-1", WorkUnit: "consumed-scope",
		EvidenceGoal: "prove carried budget binds", MaxAttempts: 3, MaxChangedLines: 400,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendRuntimeLedgerFile(t, repo, strings.Repeat("consumed-line\n", 50))
	interrupted, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "bind-finish-1", Outcome: AttemptInterrupted,
		EvidenceRevision: runtimeTestHash('5'), Diagnosis: "interrupted after charging real lines, within budget",
		HarnessDisposition: HarnessInvalidated, CleanupEvidence: "cleanup completed with the charge left in place",
		ProcessEvidence: "post-interruption process scan found no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	if interrupted.CumulativeChangedLines == 0 || interrupted.DecisionRequired || interrupted.NextAction != RuntimeActionBegin {
		t.Fatalf("pre-rescope consumed-budget status = %#v", interrupted)
	}
	consumed := interrupted.CumulativeChangedLines

	// The candidate has not drifted further since Finish: rescope is
	// admissible even though the OBJECTIVE already carries real charges.
	rescoped, err := store.Rescope(context.Background(), RescopeObjectiveRequest{
		ExpectedRevision: interrupted.Revision, RequestID: "bind-rescope-1",
		WorkUnit: "narrower-consumed-scope", EvidenceGoal: "prove carried budget binds narrower",
		MaxAttempts: 3, MaxChangedLines: consumed - 1,
		Reason: "maintainer narrowed the ceiling below what is already consumed", Actor: "maintainer",
	})
	if err != nil {
		t.Fatalf("rescope with a ceiling below consumed history was refused: %v", err)
	}
	if rescoped.CumulativeChangedLines != consumed {
		t.Fatalf("rescope did not carry the consumed charge forward exactly: got %d, want %d", rescoped.CumulativeChangedLines, consumed)
	}

	_, err = store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: rescoped.Revision, RequestID: "bind-begin-2", WorkUnit: "narrower-consumed-scope",
		EvidenceGoal: "prove carried budget binds narrower", MaxAttempts: 3, MaxChangedLines: consumed - 1,
	})
	if !errors.Is(err, ErrRuntimeBudgetExhausted) {
		t.Fatalf("begin after a rescope whose ceiling is already consumed = %v, want ErrRuntimeBudgetExhausted", err)
	}
}

// TestRuntimeLedgerRescopeRequiresPreconditions guards the structural
// boundary directly: an active attempt, a missing objective, and a
// non-terminal (drifted-refused-elsewhere) state must all still refuse.
func TestRuntimeLedgerRescopeRequiresPreconditions(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "rescope-preconditions")
	if err != nil {
		t.Fatal(err)
	}

	// A store with no objective at all (no HEAD yet) cannot legally supply the
	// exact expected revision rescope requires, so the "no objective" shape is
	// reproduced the same way #2298's dead end actually recovers from it: an
	// objective closed by Reset, leaving status.Objective nil with a real HEAD
	// revision to reference.
	preObjective, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "pre-objective-begin-1", WorkUnit: "pre-objective-scope",
		EvidenceGoal: "prove no-objective guard", MaxAttempts: 1, MaxChangedLines: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	exhausted, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: preObjective.Revision, RequestID: "pre-objective-finish-1", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('7'), Diagnosis: "failed to exhaust the one-attempt budget",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed",
		ProcessEvidence: "process scan found no descendants",
	})
	if err != nil || !exhausted.DecisionRequired {
		t.Fatalf("prepare no-objective guard = %#v err=%v", exhausted, err)
	}
	reset, err := store.Reset(context.Background(), ResetObjectiveRequest{
		ExpectedRevision: exhausted.Revision, RequestID: "pre-objective-reset-1",
		Reason: "close the objective before proving the no-objective guard", Actor: "maintainer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Rescope(context.Background(), RescopeObjectiveRequest{
		ExpectedRevision: reset.Revision, RequestID: "no-objective-rescope-1", WorkUnit: "w", EvidenceGoal: "g",
		MaxAttempts: 1, MaxChangedLines: 1, Reason: "no objective yet", Actor: "maintainer",
	}); !errors.Is(err, ErrRuntimeNoObjective) {
		t.Fatalf("rescope with no objective error = %v, want ErrRuntimeNoObjective", err)
	}

	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: reset.Revision, RequestID: "active-begin-1", WorkUnit: "active-scope",
		EvidenceGoal: "prove active guard", MaxAttempts: 2, MaxChangedLines: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Rescope(context.Background(), RescopeObjectiveRequest{
		ExpectedRevision: started.Revision, RequestID: "active-rescope-1", WorkUnit: "active-scope",
		EvidenceGoal: "prove active guard", MaxAttempts: 2, MaxChangedLines: 50,
		Reason: "attempted rescope while active", Actor: "maintainer",
	}); !errors.Is(err, ErrRuntimeAttemptActive) {
		t.Fatalf("rescope with an active attempt error = %v, want ErrRuntimeAttemptActive", err)
	}

	appendRuntimeLedgerFile(t, repo, "drift-after-begin\n")
	interrupted, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "active-finish-1", Outcome: AttemptInterrupted,
		EvidenceRevision: runtimeTestHash('6'), Diagnosis: "interrupted after charging a real line",
		HarnessDisposition: HarnessInvalidated, CleanupEvidence: "cleanup completed",
		ProcessEvidence: "process scan found no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	appendRuntimeLedgerFile(t, repo, "further-drift-after-finish\n")
	if _, err := store.Rescope(context.Background(), RescopeObjectiveRequest{
		ExpectedRevision: interrupted.Revision, RequestID: "drifted-rescope-1", WorkUnit: "active-scope",
		EvidenceGoal: "prove active guard", MaxAttempts: 2, MaxChangedLines: 50,
		Reason: "attempted rescope on a drifted candidate", Actor: "maintainer",
	}); !errors.Is(err, ErrRuntimeRescopeNotAllowed) {
		t.Fatalf("rescope on a drifted candidate error = %v, want ErrRuntimeRescopeNotAllowed", err)
	}
}
