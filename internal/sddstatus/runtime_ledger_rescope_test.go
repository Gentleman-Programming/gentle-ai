package sddstatus

import (
	"context"
	"errors"
	"strconv"
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
		Diagnosis:          "required scope measured 449 changed lines against a 400-line ceiling; reverted",
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

// TestRuntimeLedgerRescopeRefusesConsecutiveRescopeWithoutOwnAttempt proves
// #2830's ratified boundary: a rescope successor needs its own terminal
// attempt before it can itself be rescoped. Its predecessor's terminal attempt
// remains immutable provenance, not authority for another successor.
func TestRuntimeLedgerRescopeRefusesConsecutiveRescopeWithoutOwnAttempt(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "rescope-2830")
	if err != nil {
		t.Fatal(err)
	}

	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "2830-begin-a", WorkUnit: "objective-a",
		EvidenceGoal: "prove the original bounded objective", MaxAttempts: 3, MaxChangedLines: 90,
	})
	if err != nil {
		t.Fatal(err)
	}
	failed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "2830-finish-a", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('a'), Diagnosis: "objective A failed with the workspace unchanged",
		HarnessDisposition: HarnessReused, CleanupEvidence: "workspace remained unchanged",
		ProcessEvidence: "post-verification process scan found no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	if failed.CumulativeChangedLines != 0 || failed.DecisionRequired || failed.Complete || failed.NextAction != RuntimeActionBegin {
		t.Fatalf("failed objective A status = %#v", failed)
	}

	rescoped, err := store.Rescope(context.Background(), RescopeObjectiveRequest{
		ExpectedRevision: failed.Revision, RequestID: "2830-rescope-a-to-b",
		WorkUnit: "objective-b", EvidenceGoal: "prove the narrower successor objective",
		MaxAttempts: 3, MaxChangedLines: 60,
		Reason: "maintainer narrowed objective A into successor B", Actor: "maintainer",
	})
	if err != nil {
		t.Fatalf("first rescope was refused: %v", err)
	}
	if rescoped.Objective == nil || rescoped.Objective.WorkUnit != "objective-b" || rescoped.Objective.MaxAttempts != 3 ||
		rescoped.Objective.MaxChangedLines != 60 || rescoped.NextAction != RuntimeActionBegin || rescoped.ActiveAttempt != nil {
		t.Fatalf("rescoped objective B status = %#v", rescoped)
	}
	if len(rescoped.Attempts) != 1 || rescoped.Attempts[0].ObjectiveID == rescoped.Objective.ID {
		t.Fatalf("successor B must have no attempt of its own yet: %#v", rescoped)
	}

	beforeRevision := rescoped.Revision
	beforeRecords := countRuntimeRecords(t, store.Dir)
	_, err = store.Rescope(context.Background(), RescopeObjectiveRequest{
		ExpectedRevision: beforeRevision, RequestID: "2830-rescope-b-to-c-before-attempt",
		WorkUnit: "objective-c", EvidenceGoal: "prove the still narrower successor objective",
		MaxAttempts: 3, MaxChangedLines: 30,
		Reason: "maintainer attempted to rescope B before B has an attempt", Actor: "maintainer",
	})
	if !errors.Is(err, ErrRuntimeRescopeNotAllowed) {
		t.Errorf("consecutive rescope error = %v, want ErrRuntimeRescopeNotAllowed", err)
	}
	if records := countRuntimeRecords(t, store.Dir); records != beforeRecords {
		t.Errorf("refused consecutive rescope published a record: records=%d, want %d", records, beforeRecords)
	}
	status, statusErr := store.Status()
	if statusErr != nil {
		t.Errorf("status after refused consecutive rescope = %v", statusErr)
	} else if status.Revision != beforeRevision || status.Objective == nil || status.Objective.ID != rescoped.Objective.ID ||
		len(status.Attempts) != 1 || status.ActiveAttempt != nil || status.NextAction != RuntimeActionBegin {
		t.Errorf("refused consecutive rescope changed authoritative status: %#v", status)
	}
	if statusErr != nil {
		return
	}

	// Negative control: the same narrowing transition is admitted once objective
	// B owns a terminal attempt, so the refusal above cannot be a generic rescope
	// failure unrelated to objective ownership.
	startedB, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: status.Revision, RequestID: "2830-begin-b", WorkUnit: "objective-b",
		EvidenceGoal: "prove the narrower successor objective", MaxAttempts: 3, MaxChangedLines: 60,
	})
	if err != nil {
		t.Fatalf("begin objective B for negative control: %v", err)
	}
	failedB, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: startedB.Revision, RequestID: "2830-finish-b", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('b'), Diagnosis: "objective B failed with the workspace unchanged",
		HarnessDisposition: HarnessReused, CleanupEvidence: "workspace remained unchanged",
		ProcessEvidence: "post-verification process scan found no descendants",
	})
	if err != nil {
		t.Fatalf("finish objective B for negative control: %v", err)
	}
	accepted, err := store.Rescope(context.Background(), RescopeObjectiveRequest{
		ExpectedRevision: failedB.Revision, RequestID: "2830-rescope-b-to-c-after-attempt",
		WorkUnit: "objective-c", EvidenceGoal: "prove the still narrower successor objective",
		MaxAttempts: 3, MaxChangedLines: 30,
		Reason: "maintainer narrowed B only after B recorded its own attempt", Actor: "maintainer",
	})
	if err != nil {
		t.Fatalf("rescope after objective B's terminal attempt was refused: %v", err)
	}
	if accepted.Objective == nil || accepted.Objective.WorkUnit != "objective-c" || accepted.NextAction != RuntimeActionBegin {
		t.Fatalf("accepted post-attempt rescope status = %#v", accepted)
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
		Diagnosis:          "interrupted with the workspace unchanged",
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
		Diagnosis:          "interrupted with the workspace unchanged",
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

// TestRuntimeLedgerRescopeCarriedBudgetBindsAtAdmission is mutation proof
// (d), moved to its truthful boundary by #2804: carry-forward is not
// cosmetic, and the place it binds is rescope ADMISSION. Narrowing to a
// ceiling the carried CumulativeChangedLines already meets would publish a
// successor whose status advertises begin while its first acquire is refused
// budget-exhausted, whose reset is refused for zero drift, and whose own
// rescope cannot widen -- a wedge with no admitted continuation. That
// rescope is refused before mutation, and the runnable ceiling the refusal
// names is executed, not asserted as prose.
func TestRuntimeLedgerRescopeCarriedBudgetBindsAtAdmission(t *testing.T) {
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
		Diagnosis:          "interrupted after charging real lines, within budget",
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

	// A ceiling the carried charge already meets is refused with rescope's
	// own exhaustion sentinel, never a reused generic one, and refuses
	// BEFORE mutation.
	_, exhaustedErr := store.Rescope(context.Background(), RescopeObjectiveRequest{
		ExpectedRevision: interrupted.Revision, RequestID: "bind-rescope-1",
		WorkUnit: "narrower-consumed-scope", EvidenceGoal: "prove carried budget binds narrower",
		MaxAttempts: 3, MaxChangedLines: consumed - 1,
		Reason: "maintainer narrowed the ceiling below what is already consumed", Actor: "maintainer",
	})
	if !errors.Is(exhaustedErr, ErrRuntimeRescopeExhausted) {
		t.Fatalf("rescope with a ceiling below consumed history = %v, want ErrRuntimeRescopeExhausted", exhaustedErr)
	}
	if errors.Is(exhaustedErr, ErrRuntimeRescopeWidened) || errors.Is(exhaustedErr, ErrRuntimeRescopeNotAllowed) {
		t.Fatalf("exhaustion refusal reused a generic sentinel: %v", exhaustedErr)
	}
	for _, want := range []string{
		"--max-changed-lines " + strconv.Itoa(consumed-1),
		"carried " + strconv.Itoa(consumed),
		"at most 400",
	} {
		if !strings.Contains(exhaustedErr.Error(), want) {
			t.Fatalf("exhaustion refusal does not name %q: %v", want, exhaustedErr)
		}
	}
	status, statusErr := store.Status()
	if statusErr != nil || status.Revision != interrupted.Revision || countRuntimeRecords(t, store.Dir) != 2 {
		t.Fatalf("refused exhausted rescope mutated the ledger: status=%#v err=%v records=%d", status, statusErr, countRuntimeRecords(t, store.Dir))
	}

	// The runnable range the refusal names is executed: one line above the
	// carried charge is admitted, carries the charge forward exactly, and
	// its first begin proceeds instead of dying budget-exhausted.
	rescoped, err := store.Rescope(context.Background(), RescopeObjectiveRequest{
		ExpectedRevision: interrupted.Revision, RequestID: "bind-rescope-2",
		WorkUnit: "narrower-consumed-scope", EvidenceGoal: "prove carried budget binds narrower",
		MaxAttempts: 3, MaxChangedLines: consumed + 1,
		Reason: "maintainer narrowed the ceiling to the smallest runnable allowance", Actor: "maintainer",
	})
	if err != nil {
		t.Fatalf("the runnable rescope this refusal names was refused: %v", err)
	}
	if rescoped.CumulativeChangedLines != consumed || rescoped.DecisionRequired || rescoped.NextAction != RuntimeActionBegin {
		t.Fatalf("runnable rescope status = %#v", rescoped)
	}
	next, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: rescoped.Revision, RequestID: "bind-begin-2", WorkUnit: "narrower-consumed-scope",
		EvidenceGoal: "prove carried budget binds narrower", MaxAttempts: 3, MaxChangedLines: consumed + 1,
	})
	if err != nil {
		t.Fatalf("begin under the runnable rescoped ceiling was refused: %v", err)
	}
	if next.ActiveAttempt == nil || next.ActiveAttempt.ObjectiveID != rescoped.Objective.ID {
		t.Fatalf("post-rescope begin status = %#v", next)
	}
}

// TestRuntimeLedgerRescopeRefusesExhaustedAttemptAllowance is #2804's
// write-time guard: rescope carries cumulative_attempts forward unchanged, so
// a successor whose max_attempts the carried count already meets has no
// runnable ordinal. Before this guard it was committed anyway: status
// advertised begin, acquire refused budget-exhausted, reset refused for zero
// drift, and a second rescope could not widen -- a published successor with
// no admitted continuation. The refusal now lands before mutation and names
// the exact runnable range, which is then executed.
func TestRuntimeLedgerRescopeRefusesExhaustedAttemptAllowance(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "rescope-2804")
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "2804-begin-1", WorkUnit: "failing-verification",
		EvidenceGoal: "independently verify the applied change", MaxAttempts: 2, MaxChangedLines: 400,
	})
	if err != nil {
		t.Fatal(err)
	}
	failed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "2804-finish-1", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('9'), Diagnosis: "verification failed with the workspace unchanged",
		HarnessDisposition: HarnessReused, CleanupEvidence: "verification harness exited cleanly",
		ProcessEvidence: "post-verification process scan found no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	if failed.CumulativeAttempts != 1 || failed.DecisionRequired || failed.Complete {
		t.Fatalf("pre-rescope failed status = %#v", failed)
	}

	// max_attempts equal to the carried cumulative_attempts is the exact
	// #2804 reproduction: narrowing-valid, and immediately exhausted.
	_, exhaustedErr := store.Rescope(context.Background(), RescopeObjectiveRequest{
		ExpectedRevision: failed.Revision, RequestID: "2804-rescope-1",
		WorkUnit: "narrower-correction", EvidenceGoal: "prove the bounded correction",
		MaxAttempts: 1, MaxChangedLines: 120,
		Reason: "maintainer narrowed to a correction objective", Actor: "maintainer",
	})
	if !errors.Is(exhaustedErr, ErrRuntimeRescopeExhausted) {
		t.Fatalf("exhausted-allowance rescope error = %v, want ErrRuntimeRescopeExhausted", exhaustedErr)
	}
	if errors.Is(exhaustedErr, ErrRuntimeRescopeWidened) || errors.Is(exhaustedErr, ErrRuntimeRescopeNotAllowed) {
		t.Fatalf("exhaustion refusal reused a generic sentinel: %v", exhaustedErr)
	}
	for _, want := range []string{"--max-attempts 1", "carried 1", "at most 2"} {
		if !strings.Contains(exhaustedErr.Error(), want) {
			t.Fatalf("exhaustion refusal does not name %q: %v", want, exhaustedErr)
		}
	}
	status, statusErr := store.Status()
	if statusErr != nil || status.Revision != failed.Revision || countRuntimeRecords(t, store.Dir) != 2 {
		t.Fatalf("refused exhausted rescope mutated the ledger: status=%#v err=%v records=%d", status, statusErr, countRuntimeRecords(t, store.Dir))
	}

	// The runnable allowance the refusal names is executed: one attempt above
	// the carried count is admitted and its first begin proceeds.
	rescoped, err := store.Rescope(context.Background(), RescopeObjectiveRequest{
		ExpectedRevision: failed.Revision, RequestID: "2804-rescope-2",
		WorkUnit: "narrower-correction", EvidenceGoal: "prove the bounded correction",
		MaxAttempts: 2, MaxChangedLines: 120,
		Reason: "maintainer narrowed to a runnable correction objective", Actor: "maintainer",
	})
	if err != nil {
		t.Fatalf("the runnable rescope this refusal names was refused: %v", err)
	}
	if rescoped.CumulativeAttempts != 1 || rescoped.DecisionRequired || rescoped.NextAction != RuntimeActionBegin {
		t.Fatalf("runnable rescope status = %#v", rescoped)
	}
	next, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: rescoped.Revision, RequestID: "2804-begin-2", WorkUnit: "narrower-correction",
		EvidenceGoal: "prove the bounded correction", MaxAttempts: 2, MaxChangedLines: 120,
	})
	if err != nil {
		t.Fatalf("begin under the runnable rescoped allowance was refused: %v", err)
	}
	if next.ActiveAttempt == nil || next.ActiveAttempt.ObjectiveID != rescoped.Objective.ID {
		t.Fatalf("post-rescope begin status = %#v", next)
	}
}

// TestProjectLegacyExhaustedSuccessorIsScopedToTheFreshRescopeWedge pins the
// projection's scope instead of asserting it in prose: only the exact
// publication wedge -- the last transition is the rescope that opened the
// current objective, and the successor has no attempt of its own -- may be
// converted to decision-required. Every other exhausted shape must keep its
// replayed projection, so an objective legitimately holding unused allowance
// is never silently converted into a demanded reset.
func TestProjectLegacyExhaustedSuccessorIsScopedToTheFreshRescopeWedge(t *testing.T) {
	objective := &RuntimeObjective{ID: "objective-b", MaxAttempts: 1, MaxChangedLines: 10}

	// Exhausted with no rescope transition at all: not the wedge.
	noRescope := RuntimeStatus{Objective: objective, CumulativeAttempts: 1, NextAction: RuntimeActionBegin}
	projectLegacyExhaustedSuccessor(&noRescope)
	if noRescope.DecisionRequired || noRescope.NextAction != RuntimeActionBegin {
		t.Fatalf("non-rescope exhausted state was converted: %#v", noRescope)
	}

	// Exhausted successor that already owns an attempt: not the wedge either
	// (the pre-guard writer could never publish this -- its begin was refused).
	ownAttempt := RuntimeStatus{
		Objective: objective, CumulativeAttempts: 1, NextAction: RuntimeActionBegin,
		LastRescope: &RuntimeRescope{ObjectiveID: objective.ID},
		Attempts:    []RuntimeAttempt{{ObjectiveID: objective.ID, Outcome: AttemptFailed}},
	}
	projectLegacyExhaustedSuccessor(&ownAttempt)
	if ownAttempt.DecisionRequired || ownAttempt.NextAction != RuntimeActionBegin {
		t.Fatalf("successor with its own attempt was converted: %#v", ownAttempt)
	}

	// The publication wedge itself projects to decision-required/reset.
	wedge := RuntimeStatus{
		Objective: objective, CumulativeAttempts: 1, NextAction: RuntimeActionBegin,
		LastRescope: &RuntimeRescope{ObjectiveID: objective.ID},
		Attempts:    []RuntimeAttempt{{ObjectiveID: "objective-a", Outcome: AttemptFailed}},
	}
	projectLegacyExhaustedSuccessor(&wedge)
	if !wedge.DecisionRequired || wedge.NextAction != RuntimeActionReset {
		t.Fatalf("the publication wedge did not project: %#v", wedge)
	}
}

// TestRuntimeLedgerLegacyExhaustedRescopeReplaysToDecisionRequired is #2804's
// recovery half: builds before the write-time guard PUBLISHED exhausted
// successors, and prevention alone does not repair them (the fresh occurrence
// on #2804 says exactly that). The immutable record stays valid -- replay
// never rewrites history -- but the projected state must tell the truth: an
// objective with no runnable ordinal is a maintainer decision, exactly as
// applyRuntimeConsecutiveRescopeRepairEvent already projects for the same
// shape. Status stops advertising a begin acquire refuses, and the reset that
// decision-required admits is executed all the way to a wider fresh budget.
func TestRuntimeLedgerLegacyExhaustedRescopeReplaysToDecisionRequired(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "rescope-2804-legacy")
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "legacy-begin-1", WorkUnit: "failing-verification",
		EvidenceGoal: "independently verify the applied change", MaxAttempts: 2, MaxChangedLines: 400,
	})
	if err != nil {
		t.Fatal(err)
	}
	failed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "legacy-finish-1", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('8'), Diagnosis: "verification failed with the workspace unchanged",
		HarnessDisposition: HarnessReused, CleanupEvidence: "verification harness exited cleanly",
		ProcessEvidence: "post-verification process scan found no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	objective := failed.Objective
	last := failed.Attempts[len(failed.Attempts)-1]

	// Publish the exact record a pre-guard writer committed for the #2804
	// occurrence: internally consistent, truthfully narrowing, and already
	// exhausted (max_attempts equal to the carried cumulative_attempts).
	generation := failed.ObjectiveGeneration + 1
	legacyObjectiveID := runtimeObjectiveID(store.Change, "narrower-correction", "prove the bounded correction", last.FinishCandidateIdentity, generation)
	request := RescopeObjectiveRequest{
		ExpectedRevision: failed.Revision, RequestID: "legacy-rescope-1",
		WorkUnit: "narrower-correction", EvidenceGoal: "prove the bounded correction",
		MaxAttempts: 1, MaxChangedLines: 120,
		Reason: "maintainer narrowed to a correction objective", Actor: "maintainer",
	}
	legacyRecord := runtimeRecord{
		Schema: runtimeRecordSchema, Change: store.Change, PreviousRevision: failed.Revision,
		Operation: runtimeOperationRescope, RequestID: request.RequestID,
		RequestDigest: runtimeValueHash("gentle-ai.sdd-runtime-rescope-request/v1", request),
		Rescope: &runtimeRescopeEvent{
			PreviousObjectiveID: objective.ID, PreviousGeneration: objective.Generation,
			PreviousMaxAttempts: objective.MaxAttempts, PreviousMaxChangedLines: objective.MaxChangedLines,
			RescopeCandidateIdentity: last.FinishCandidateIdentity, RescopeCandidateTree: last.FinishCandidateTree,
			ObjectiveID: legacyObjectiveID, ObjectiveGeneration: generation,
			WorkUnit: request.WorkUnit, EvidenceGoal: request.EvidenceGoal,
			MaxAttempts: request.MaxAttempts, MaxChangedLines: request.MaxChangedLines,
			Reason: request.Reason, Actor: request.Actor,
		},
	}
	revision, payload, err := runtimeRecordRevision(legacyRecord)
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

	// The wedge is gone: replay projects the truth instead of an unusable
	// begin. History, the successor objective, and the carried charges all
	// survive untouched.
	status, err := store.Status()
	if err != nil {
		t.Fatalf("replaying the published legacy exhausted rescope failed: %v", err)
	}
	if !status.DecisionRequired || status.Complete || status.NextAction != RuntimeActionReset {
		t.Fatalf("legacy exhausted successor status = %#v, want decision-required with next_action reset", status)
	}
	if status.Objective == nil || status.Objective.ID != legacyObjectiveID || status.Objective.MaxAttempts != 1 ||
		status.CumulativeAttempts != 1 || len(status.Attempts) != 1 {
		t.Fatalf("legacy exhausted successor projection = %#v", status)
	}

	// Begin stays refused -- truthfully now, with status in agreement.
	if _, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: status.Revision, RequestID: "legacy-begin-2", WorkUnit: "narrower-correction",
		EvidenceGoal: "prove the bounded correction", MaxAttempts: 1, MaxChangedLines: 120,
	}); !errors.Is(err, ErrRuntimeBudgetExhausted) {
		t.Fatalf("begin under the legacy exhausted successor = %v, want ErrRuntimeBudgetExhausted", err)
	}

	// The decision-required reset is admitted and opens the wider fresh
	// budget the wedged reporter could never reach.
	after, err := store.Reset(context.Background(), ResetObjectiveRequest{
		ExpectedRevision: status.Revision, RequestID: "legacy-reset-1",
		Reason: "recover the published exhausted successor with a fresh budget", Actor: "maintainer",
	})
	if err != nil {
		t.Fatalf("reset of the legacy exhausted successor was refused: %v", err)
	}
	wider, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: after.Revision, RequestID: "legacy-begin-3", WorkUnit: "recovered-correction",
		EvidenceGoal: "prove the recovered correction", MaxAttempts: 2, MaxChangedLines: 400,
	})
	if err != nil {
		t.Fatalf("begin after recovering the legacy exhausted successor was refused: %v", err)
	}
	if wider.Objective == nil || wider.Objective.MaxAttempts != 2 || wider.CumulativeAttempts != 1 {
		t.Fatalf("post-recovery objective = %#v", wider)
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
		Diagnosis:          "interrupted after charging a real line",
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

// TestRuntimeLedgerZeroDriftResetRefusalNamesBothExits is #1974's reproduction.
// The reporter reached a failed verification objective with the candidate
// unchanged and budget remaining, ran reset, and concluded the lifecycle was
// deadlocked. It was not: rescope already owned that transition. The refusal
// they received named neither it nor the route to a wider successor budget, and
// status answered next_action: begin, which for failed verification evidence is
// the one continuation that cannot help.
//
// Both named exits are executed here, not just matched as strings, because a
// refusal that names a command nothing can run is worse than one that names
// nothing at all.
func TestRuntimeLedgerZeroDriftResetRefusalNamesBothExits(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "reset-exit-1974")
	if err != nil {
		t.Fatal(err)
	}

	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "verify-begin-1", WorkUnit: "independent-verification",
		EvidenceGoal: "independently verify the applied change", MaxAttempts: 2, MaxChangedLines: 40,
	})
	if err != nil {
		t.Fatal(err)
	}
	failed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "verify-finish-1", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('7'), Diagnosis: "verification failed with the workspace unchanged",
		HarnessDisposition: HarnessInvalidated, CleanupEvidence: "verification harness exited cleanly",
		ProcessEvidence: "post-verification process scan found no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	if failed.DecisionRequired || failed.Complete || failed.NextAction != RuntimeActionBegin || failed.CumulativeAttempts != 1 {
		t.Fatalf("pre-reset failed-verification status = %#v", failed)
	}

	_, resetErr := store.Reset(context.Background(), ResetObjectiveRequest{
		ExpectedRevision: failed.Revision, RequestID: "verify-reset-1",
		Reason: "failed evidence proves implementation must remediate", Actor: "maintainer",
	})
	if !errors.Is(resetErr, ErrRuntimeResetNotAllowed) {
		t.Fatalf("zero-drift reset error = %v, want ErrRuntimeResetNotAllowed", resetErr)
	}
	for _, want := range []string{
		"gentle-ai sdd-attempt rescope",
		"--expected-revision " + strconv.Quote(failed.Revision),
		"at most 40",
		"decision-required",
	} {
		if !strings.Contains(resetErr.Error(), want) {
			t.Fatalf("zero-drift reset refusal does not name %q: %v", want, resetErr)
		}
	}

	// Exit 1 runs: the rescope the refusal names is admitted at this exact
	// revision, with the ceiling it advertises.
	rescoped, err := store.Rescope(context.Background(), RescopeObjectiveRequest{
		ExpectedRevision: failed.Revision, RequestID: "verify-rescope-1",
		WorkUnit: "verification-remediation", EvidenceGoal: "remediate the verification gap",
		MaxAttempts: 2, MaxChangedLines: 40,
		Reason: "failed evidence names the remediation", Actor: "maintainer",
	})
	if err != nil {
		t.Fatalf("the rescope this refusal names was refused: %v", err)
	}
	if rescoped.Objective == nil || rescoped.Objective.WorkUnit != "verification-remediation" ||
		rescoped.CumulativeAttempts != 1 {
		t.Fatalf("rescoped objective = %#v", rescoped)
	}
}

// TestRuntimeLedgerExhaustedAttemptsAdmitTheResetForAWiderScope is the second
// exit #1974's refusal now names, and the reason the narrow-only rescope rule
// is not itself a deadlock: a caller who needs a successor budget WIDER than
// the failed objective's ceiling spends the remaining attempts honestly, and
// the run that exhausts them reaches decision-required, where reset opens a
// fresh budget of any size.
func TestRuntimeLedgerExhaustedAttemptsAdmitTheResetForAWiderScope(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "reset-exit-1974-wider")
	if err != nil {
		t.Fatal(err)
	}

	revision := ""
	for ordinal, request := range []string{"verify-1", "verify-2"} {
		started, beginErr := store.Begin(context.Background(), BeginAttemptRequest{
			ExpectedRevision: revision, RequestID: request + "-begin", WorkUnit: "independent-verification",
			EvidenceGoal: "independently verify the applied change", MaxAttempts: 2, MaxChangedLines: 40,
		})
		if beginErr != nil {
			t.Fatalf("begin %d: %v", ordinal+1, beginErr)
		}
		finished, finishErr := store.Finish(context.Background(), FinishAttemptRequest{
			ExpectedRevision: started.Revision, RequestID: request + "-finish", Outcome: AttemptFailed,
			EvidenceRevision: runtimeTestHash(byte('a' + ordinal)), Diagnosis: "verification failed with the workspace unchanged",
			HarnessDisposition: HarnessInvalidated, CleanupEvidence: "verification harness exited cleanly",
			ProcessEvidence: "post-verification process scan found no descendants",
		})
		if finishErr != nil {
			t.Fatalf("finish %d: %v", ordinal+1, finishErr)
		}
		revision = finished.Revision
		if ordinal == 1 && (!finished.DecisionRequired || finished.NextAction != RuntimeActionReset) {
			t.Fatalf("exhausting the attempt budget did not reach decision-required: %#v", finished)
		}
	}

	after, err := store.Reset(context.Background(), ResetObjectiveRequest{
		ExpectedRevision: revision, RequestID: "wider-reset-1",
		Reason: "verification exhausted its budget; remediation needs a wider scope", Actor: "maintainer",
	})
	if err != nil {
		t.Fatalf("reset at decision-required was refused: %v", err)
	}

	// The fresh objective may exceed the exhausted one's ceiling, which is the
	// capability rescope structurally cannot provide.
	wider, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: after.Revision, RequestID: "wider-begin-1", WorkUnit: "implementation-remediation",
		EvidenceGoal: "remediate what the failed verification named", MaxAttempts: 2, MaxChangedLines: 400,
	})
	if err != nil {
		t.Fatalf("begin with a wider budget after reset was refused: %v", err)
	}
	if wider.Objective == nil || wider.Objective.MaxChangedLines != 400 {
		t.Fatalf("post-reset objective = %#v", wider.Objective)
	}
	if wider.LifetimeAttempts != 3 {
		t.Fatalf("reset laundered lifetime attempts: %#v", wider)
	}
}

// TestRuntimeLedgerWidenedRescopeRefusalNamesTheExhaustRoute is #2769 A, the
// complement of #1974: that caller reached for reset and heard nothing about
// rescope, this one reaches for rescope and hears only that a wider budget is
// refused. Status answers begin, which repeats the ceiling they just called
// too small. The exhaust-then-reset route is executed here, not asserted as
// prose, because a refusal naming a route nothing can walk is worse than one
// naming nothing.
func TestRuntimeLedgerWidenedRescopeRefusalNamesTheExhaustRoute(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "widened-2769")
	if err != nil {
		t.Fatal(err)
	}

	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "widen-begin-1", WorkUnit: "independent-verification",
		EvidenceGoal: "independently verify the applied change", MaxAttempts: 2, MaxChangedLines: 40,
	})
	if err != nil {
		t.Fatal(err)
	}
	failed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "widen-finish-1", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('3'), Diagnosis: "verification failed with the workspace unchanged",
		HarnessDisposition: HarnessInvalidated, CleanupEvidence: "verification harness exited cleanly",
		ProcessEvidence: "post-verification process scan found no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	if failed.NextAction != RuntimeActionBegin {
		t.Fatalf("status must still answer begin for this state: %#v", failed)
	}

	_, widenErr := store.Rescope(context.Background(), RescopeObjectiveRequest{
		ExpectedRevision: failed.Revision, RequestID: "widen-rescope-1",
		WorkUnit: "implementation-remediation", EvidenceGoal: "remediate what verification named",
		MaxAttempts: 2, MaxChangedLines: 400,
		Reason: "remediation needs more than the verification ceiling", Actor: "maintainer",
	})
	if !errors.Is(widenErr, ErrRuntimeRescopeWidened) {
		t.Fatalf("widened rescope error = %v, want ErrRuntimeRescopeWidened", widenErr)
	}
	for _, want := range []string{"1 remaining attempt", "decision-required", "gentle-ai sdd-attempt reset"} {
		if !strings.Contains(widenErr.Error(), want) {
			t.Fatalf("widened rescope refusal does not name %q: %v", want, widenErr)
		}
	}

	// Walk the route the refusal names: spend the last attempt, land on
	// decision-required, reset, and open the wider budget rescope refused.
	last, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: failed.Revision, RequestID: "widen-begin-2", WorkUnit: "independent-verification",
		EvidenceGoal: "independently verify the applied change", MaxAttempts: 2, MaxChangedLines: 40,
	})
	if err != nil {
		t.Fatal(err)
	}
	exhausted, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: last.Revision, RequestID: "widen-finish-2", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('4'), Diagnosis: "verification failed again with the workspace unchanged",
		HarnessDisposition: HarnessInvalidated, CleanupEvidence: "verification harness exited cleanly",
		ProcessEvidence: "post-verification process scan found no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !exhausted.DecisionRequired {
		t.Fatalf("the route the refusal names did not reach decision-required: %#v", exhausted)
	}
	after, err := store.Reset(context.Background(), ResetObjectiveRequest{
		ExpectedRevision: exhausted.Revision, RequestID: "widen-reset-1",
		Reason: "remediation needs a wider scope than verification allowed", Actor: "maintainer",
	})
	if err != nil {
		t.Fatalf("the reset this refusal names was refused: %v", err)
	}
	wider, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: after.Revision, RequestID: "widen-begin-3", WorkUnit: "implementation-remediation",
		EvidenceGoal: "remediate what verification named", MaxAttempts: 2, MaxChangedLines: 400,
	})
	if err != nil {
		t.Fatalf("the wider budget the refusal promises was refused: %v", err)
	}
	if wider.Objective == nil || wider.Objective.MaxChangedLines != 400 {
		t.Fatalf("post-reset objective = %#v", wider.Objective)
	}
}

// TestRuntimeLedgerCompleteObjectiveRefusalNamesTheSuccessor is #2769 B. A
// completed objective refused a repeated begin with "objective is complete"
// and pointed at status, whose next_action is `complete`: true, and useless to
// a caller with more work on this change. Advance was one flag away the whole
// time, and the code comment above the refusal already said so.
func TestRuntimeLedgerCompleteObjectiveRefusalNamesTheSuccessor(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "complete-2769")
	if err != nil {
		t.Fatal(err)
	}

	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "done-begin-1", WorkUnit: "apply-the-change",
		EvidenceGoal: "apply the approved change", MaxAttempts: 2, MaxChangedLines: 400,
	})
	if err != nil {
		t.Fatal(err)
	}
	passed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "done-finish-1", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('5'), Diagnosis: "the change applied and its evidence passed",
		HarnessDisposition: HarnessReused, CleanupEvidence: "harness exited cleanly",
		ProcessEvidence: "post-run process scan found no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !passed.Complete || passed.NextAction != RuntimeActionComplete {
		t.Fatalf("post-pass status = %#v", passed)
	}

	_, doneErr := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: passed.Revision, RequestID: "done-begin-2", WorkUnit: "apply-the-change",
		EvidenceGoal: "apply the approved change", MaxAttempts: 2, MaxChangedLines: 400,
	})
	if !errors.Is(doneErr, ErrRuntimeObjectiveDone) {
		t.Fatalf("repeated begin error = %v, want ErrRuntimeObjectiveDone", doneErr)
	}
	for _, want := range []string{"--work-unit", "advance", "gentle-ai sdd-attempt reset"} {
		if !strings.Contains(doneErr.Error(), want) {
			t.Fatalf("complete-objective refusal does not name %q: %v", want, doneErr)
		}
	}

	// Exit 1 runs: the same begin with only --work-unit changed is admitted.
	advanced, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: passed.Revision, RequestID: "done-begin-3", WorkUnit: "verify-the-change",
		EvidenceGoal: "apply the approved change", MaxAttempts: 2, MaxChangedLines: 400,
	})
	if err != nil {
		t.Fatalf("the advance this refusal names was refused: %v", err)
	}
	if advanced.ActiveAttempt == nil || advanced.Objective == nil || advanced.Objective.WorkUnit != "verify-the-change" {
		t.Fatalf("advanced objective = %#v", advanced)
	}
}
