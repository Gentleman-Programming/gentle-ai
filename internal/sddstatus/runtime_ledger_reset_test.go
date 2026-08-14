package sddstatus

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

func TestRuntimeLedgerExplicitResetStartsDistinctBudgetWithoutLosingHistory(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "reset-change")
	if err != nil {
		t.Fatal(err)
	}

	first, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "begin-one", WorkUnit: "browser-harness",
		EvidenceGoal: "prove runtime containment", MaxAttempts: 2, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	failed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: first.Revision, RequestID: "finish-one", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('1'), Diagnosis: "first bounded execution reproduced the harness defect",
		HarnessDisposition: HarnessReused, CleanupEvidence: "first process group exited cleanly",
		ProcessEvidence: "first process scan found no surviving descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: failed.Revision, RequestID: "begin-two", WorkUnit: "browser-harness",
		EvidenceGoal: "prove runtime containment", MaxAttempts: 2, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Objective == nil || second.Objective == nil || first.Objective.ID != second.Objective.ID ||
		second.Objective.Generation != 1 || second.CumulativeAttempts != 2 || second.LifetimeAttempts != 2 {
		t.Fatalf("continued objective status = %#v", second)
	}
	exhausted, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: second.Revision, RequestID: "finish-two", Outcome: AttemptInterrupted,
		Diagnosis:          "transport ended after bounded process evidence was captured",
		HarnessDisposition: HarnessInvalidated, CleanupEvidence: "second process group cleanup completed",
		ProcessEvidence: "second process scan found no surviving descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !exhausted.DecisionRequired || exhausted.NextAction != RuntimeActionReset {
		t.Fatalf("exhausted objective status = %#v", exhausted)
	}
	oldObjectiveID := exhausted.Objective.ID

	resetRequest := ResetObjectiveRequest{
		ExpectedRevision: exhausted.Revision, RequestID: "reset-one",
		Reason: "runtime evidence scope changed after the bounded harness diagnosis",
		Actor:  "maintainer",
	}
	reset, err := store.Reset(context.Background(), resetRequest)
	if err != nil {
		t.Fatal(err)
	}
	if reset.Objective != nil || reset.ActiveAttempt != nil || reset.CumulativeAttempts != 0 ||
		reset.CumulativeChangedLines != 0 || reset.LifetimeAttempts != 2 || reset.NextOrdinal != 3 ||
		reset.ObjectiveGeneration != 1 || reset.DecisionRequired || reset.Complete || reset.NextAction != RuntimeActionBegin {
		t.Fatalf("reset status = %#v", reset)
	}
	if reset.LastReset == nil || reset.LastReset.Revision != reset.Revision || reset.LastReset.PreviousObjectiveID != oldObjectiveID ||
		reset.LastReset.Reason != resetRequest.Reason || reset.LastReset.Actor != resetRequest.Actor {
		t.Fatalf("reset audit context = %#v", reset.LastReset)
	}
	replayedReset, err := store.Reset(context.Background(), resetRequest)
	if err != nil || replayedReset.Revision != reset.Revision || countRuntimeRecords(t, store.Dir) != 5 {
		t.Fatalf("reset exact replay = %#v err=%v records=%d", replayedReset, err, countRuntimeRecords(t, store.Dir))
	}

	third, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: reset.Revision, RequestID: "begin-three", WorkUnit: "post-reset-proof",
		EvidenceGoal: "prove runtime containment", MaxAttempts: 2, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if third.Objective == nil || third.Objective.Generation != 2 || third.Objective.ID == oldObjectiveID ||
		third.CumulativeAttempts != 1 || third.LifetimeAttempts != 3 || third.NextOrdinal != 4 || len(third.Attempts) != 3 {
		t.Fatalf("post-reset objective status = %#v", third)
	}
	if third.Attempts[0].ObjectiveID != oldObjectiveID || third.Attempts[1].ObjectiveID != oldObjectiveID ||
		third.Attempts[2].ObjectiveID != third.Objective.ID {
		t.Fatalf("objective history = %#v", third.Attempts)
	}
}

func TestRuntimeLedgerResetRequiresTerminalObjectiveAndExactCAS(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "reset-guard")
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "begin-guard", WorkUnit: "guarded-runtime",
		EvidenceGoal: "prove guarded reset", MaxAttempts: 3, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Reset(context.Background(), ResetObjectiveRequest{
		ExpectedRevision: started.Revision, RequestID: "reset-active", Reason: "attempted active reset", Actor: "maintainer",
	})
	if !errors.Is(err, ErrRuntimeAttemptActive) {
		t.Fatalf("active reset error = %v", err)
	}
	failed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "finish-guard", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('3'), Diagnosis: "guarded harness failed before its budget was exhausted",
		HarnessDisposition: HarnessReused, CleanupEvidence: "guarded cleanup completed",
		ProcessEvidence: "guarded process scan found no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Reset(context.Background(), ResetObjectiveRequest{
		ExpectedRevision: failed.Revision, RequestID: "reset-early", Reason: "attempted early reset", Actor: "maintainer",
	})
	if !errors.Is(err, ErrRuntimeResetNotAllowed) {
		t.Fatalf("early reset error = %v", err)
	}
	_, err = store.Reset(context.Background(), ResetObjectiveRequest{
		ExpectedRevision: started.Revision, RequestID: "reset-stale", Reason: "attempted stale reset", Actor: "maintainer",
	})
	var conflict *RuntimeRevisionConflictError
	if !errors.As(err, &conflict) || conflict.Current != failed.Revision {
		t.Fatalf("stale reset error = %T %#v", err, err)
	}
	status, statusErr := store.Status()
	if statusErr != nil || status.Revision != failed.Revision || countRuntimeRecords(t, store.Dir) != 2 {
		t.Fatalf("denied resets mutated ledger: status=%#v err=%v records=%d", status, statusErr, countRuntimeRecords(t, store.Dir))
	}
}

func TestRuntimeLedgerReplaysPreGenerationBeginRecords(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "generation-compat")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := captureRuntimeCandidate(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	request := BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "legacy-begin", WorkUnit: "legacy-work-unit",
		EvidenceGoal: "replay pre-generation runtime authority", MaxAttempts: 2, MaxChangedLines: 20,
	}
	record := runtimeRecord{
		Schema: runtimeRecordSchema, Change: store.Change, Operation: runtimeOperationBegin,
		RequestID: request.RequestID, RequestDigest: runtimeValueHash("gentle-ai.sdd-runtime-begin-request/v1", request),
		Begin: &runtimeBeginEvent{
			ObjectiveID: legacyRuntimeObjectiveID(store.Change, request.EvidenceGoal), WorkUnit: request.WorkUnit,
			EvidenceGoal: request.EvidenceGoal, MaxAttempts: request.MaxAttempts, MaxChangedLines: request.MaxChangedLines,
			Ordinal: 1, BeginCandidateIdentity: snapshot.Identity, BeginCandidateTree: snapshot.CandidateTree,
		},
	}
	if err := validateRuntimeRecordShape(record); err != nil {
		t.Fatalf("pre-generation record rejected: %v", err)
	}
	revision, payload, err := runtimeRecordRevision(record)
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
	status, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if status.Objective == nil || status.Objective.Generation != 1 || status.ObjectiveGeneration != 1 ||
		status.ActiveAttempt == nil || status.ActiveAttempt.ObjectiveGeneration != 1 || status.LifetimeAttempts != 1 {
		t.Fatalf("pre-generation replay status = %#v", status)
	}
}

func TestRuntimeLedgerCASAllowsOnlyOneConcurrentBudgetReset(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "concurrent-reset")
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "begin-reset", WorkUnit: "concurrent-reset",
		EvidenceGoal: "prove one reset", MaxAttempts: 1, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	exhausted, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "finish-reset", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('4'), Diagnosis: "bounded attempt exhausted the objective",
		HarnessDisposition: HarnessReused, CleanupEvidence: "concurrent reset cleanup completed",
		ProcessEvidence: "concurrent reset process scan found no descendants",
	})
	if err != nil || !exhausted.DecisionRequired {
		t.Fatalf("prepare concurrent reset = %#v err=%v", exhausted, err)
	}

	const writers = 16
	start := make(chan struct{})
	var successes atomic.Int32
	var wait sync.WaitGroup
	var unexpectedMu sync.Mutex
	var unexpected []error
	for index := 0; index < writers; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			_, resetErr := store.Reset(context.Background(), ResetObjectiveRequest{
				ExpectedRevision: exhausted.Revision, RequestID: fmt.Sprintf("reset-%d", index),
				Reason: "concurrent explicit reset", Actor: "maintainer",
			})
			if resetErr == nil {
				successes.Add(1)
				return
			}
			if !errors.Is(resetErr, ErrRuntimeConcurrentUpdate) && !errors.Is(resetErr, ErrRuntimeRevisionConflict) {
				unexpectedMu.Lock()
				unexpected = append(unexpected, resetErr)
				unexpectedMu.Unlock()
			}
		}(index)
	}
	close(start)
	wait.Wait()
	if successes.Load() != 1 || len(unexpected) != 0 {
		t.Fatalf("concurrent resets: successes=%d unexpected=%v", successes.Load(), unexpected)
	}
	status, err := store.Status()
	if err != nil || status.Objective != nil || status.ObjectiveGeneration != 1 || status.LifetimeAttempts != 1 || countRuntimeRecords(t, store.Dir) != 3 {
		t.Fatalf("concurrent reset status = %#v err=%v records=%d", status, err, countRuntimeRecords(t, store.Dir))
	}
}

// TestRuntimeLedgerResetRecoversDriftDeadlockAfterInterruptedAttempt reproduces
// the community-reported lifecycle dead-end (GinoL221, Refresh 4 c2b91ac9):
// an attempt closes interrupted before launch with zero changed lines, an
// authorized maintainer correction then changes the candidate with no active
// attempt outstanding, and both `begin` (candidate drift) and the historical
// `reset` precondition (decision-required-or-complete only) refuse. An
// explicit maintainer reset must recover this scope without laundering the
// per-objective budget: history, lifetime charges, and CAS discipline all
// survive.
func TestRuntimeLedgerResetRecoversDriftDeadlockAfterInterruptedAttempt(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "drift-deadlock")
	if err != nil {
		t.Fatal(err)
	}

	beginRequest := BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "deadlock-begin-1", WorkUnit: "sdd-objective",
		EvidenceGoal: "prove recovery continuation", MaxAttempts: 2, MaxChangedLines: 400,
	}
	started, err := store.Begin(context.Background(), beginRequest)
	if err != nil {
		t.Fatal(err)
	}

	// Attempt 1 closes interrupted BEFORE the executor ever launched: no
	// workspace bytes changed, so the native line charge is exactly zero.
	interrupted, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "deadlock-finish-1", Outcome: AttemptInterrupted,
		Diagnosis:          "transport closed before the executor was launched",
		HarnessDisposition: HarnessInvalidated, CleanupEvidence: "no executor process was ever spawned",
		ProcessEvidence: "pre-launch process scan found no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	if interrupted.CumulativeChangedLines != 0 || interrupted.DecisionRequired || interrupted.Complete ||
		interrupted.NextAction != RuntimeActionBegin || interrupted.CumulativeAttempts != 1 || interrupted.LifetimeAttempts != 1 {
		t.Fatalf("pre-drift interrupted status = %#v", interrupted)
	}

	// A maintainer-approved correction changes the candidate (e.g. fixing a
	// circular plan dependency) without ever launching an executor.
	appendRuntimeLedgerFile(t, repo, "corrected-plan-dependency\n")

	retryBegin := BeginAttemptRequest{
		ExpectedRevision: interrupted.Revision, RequestID: "deadlock-begin-2", WorkUnit: "sdd-objective",
		EvidenceGoal: "prove recovery continuation", MaxAttempts: 2, MaxChangedLines: 400,
	}
	if _, err := store.Begin(context.Background(), retryBegin); !errors.Is(err, ErrRuntimeObjectiveChange) {
		t.Fatalf("drifted begin error = %v, want ErrRuntimeObjectiveChange", err)
	}
	afterBlockedBegin, statusErr := store.Status()
	if statusErr != nil || afterBlockedBegin.Revision != interrupted.Revision || countRuntimeRecords(t, store.Dir) != 2 {
		t.Fatalf("blocked drift begin mutated authority: status=%#v err=%v records=%d", afterBlockedBegin, statusErr, countRuntimeRecords(t, store.Dir))
	}

	resetRequest := ResetObjectiveRequest{
		ExpectedRevision: interrupted.Revision, RequestID: "deadlock-reset-1",
		Reason: "maintainer corrected a circular plan dependency", Actor: "maintainer",
	}
	reset, err := store.Reset(context.Background(), resetRequest)
	if err != nil {
		t.Fatalf("maintainer reset refused the drift deadlock: %v", err)
	}
	if reset.Objective != nil || reset.ActiveAttempt != nil || reset.DecisionRequired || reset.Complete ||
		reset.NextAction != RuntimeActionBegin {
		t.Fatalf("post-reset status = %#v", reset)
	}
	// Lifetime charges and the objective generation counter survive the
	// reset: the budget is not laundered by resetting.
	if reset.CumulativeAttempts != 0 || reset.CumulativeChangedLines != 0 ||
		reset.LifetimeAttempts != 1 || reset.LifetimeChangedLines != 0 || reset.ObjectiveGeneration != 1 {
		t.Fatalf("reset laundered or lost budget accounting: %#v", reset)
	}
	if len(reset.Attempts) != 1 || reset.Attempts[0].Outcome != AttemptInterrupted {
		t.Fatalf("reset lost immutable attempt history: %#v", reset.Attempts)
	}
	if reset.LastReset == nil || reset.LastReset.Reason != resetRequest.Reason || reset.LastReset.Actor != resetRequest.Actor {
		t.Fatalf("reset audit context = %#v", reset.LastReset)
	}

	// begin now proceeds against the corrected, maintainer-authorized candidate.
	next, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: reset.Revision, RequestID: "deadlock-begin-3", WorkUnit: "sdd-objective",
		EvidenceGoal: "prove recovery continuation", MaxAttempts: 2, MaxChangedLines: 400,
	})
	if err != nil {
		t.Fatalf("begin after recovery reset still refused: %v", err)
	}
	if next.Objective == nil || next.Objective.Generation != 2 || next.CumulativeAttempts != 1 ||
		next.LifetimeAttempts != 2 || next.ActiveAttempt == nil {
		t.Fatalf("post-recovery begin status = %#v", next)
	}

	// A fresh reset request against the now-stale expected revision must
	// fail CAS rather than silently succeed against a newer generation.
	_, err = store.Reset(context.Background(), ResetObjectiveRequest{
		ExpectedRevision: interrupted.Revision, RequestID: "deadlock-reset-1-stale",
		Reason: "attempted reset against a stale revision", Actor: "maintainer",
	})
	var conflict *RuntimeRevisionConflictError
	if !errors.As(err, &conflict) || conflict.Current != next.Revision {
		t.Fatalf("stale reset after a new generation began = %T %#v, want RuntimeRevisionConflictError(%q)", err, err, next.Revision)
	}
}

// TestRuntimeLedgerDriftResetStillRefusesWithActiveAttempt confirms the
// widened reset precondition never admits a reset while an attempt is
// active, even when the eventual terminal outcome would have been eligible.
func TestRuntimeLedgerDriftResetStillRefusesWithActiveAttempt(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "drift-active-guard")
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "active-begin-1", WorkUnit: "sdd-objective",
		EvidenceGoal: "prove active guard", MaxAttempts: 2, MaxChangedLines: 400,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendRuntimeLedgerFile(t, repo, "mid-attempt-change\n")
	_, err = store.Reset(context.Background(), ResetObjectiveRequest{
		ExpectedRevision: started.Revision, RequestID: "active-reset-1",
		Reason: "attempted reset while active", Actor: "maintainer",
	})
	if !errors.Is(err, ErrRuntimeAttemptActive) {
		t.Fatalf("active drift reset error = %v, want ErrRuntimeAttemptActive", err)
	}
}

// TestRuntimeLedgerDriftResetRequiresExactCAS confirms the widened reset
// precondition still enforces the exact expected revision.
func TestRuntimeLedgerDriftResetRequiresExactCAS(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "drift-cas-guard")
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "cas-begin-1", WorkUnit: "sdd-objective",
		EvidenceGoal: "prove exact CAS", MaxAttempts: 2, MaxChangedLines: 400,
	})
	if err != nil {
		t.Fatal(err)
	}
	interrupted, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "cas-finish-1", Outcome: AttemptInterrupted,
		Diagnosis:          "transport closed before the executor was launched",
		HarnessDisposition: HarnessInvalidated, CleanupEvidence: "no executor process was ever spawned",
		ProcessEvidence: "pre-launch process scan found no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	appendRuntimeLedgerFile(t, repo, "corrected-plan-dependency\n")
	_, err = store.Reset(context.Background(), ResetObjectiveRequest{
		ExpectedRevision: started.Revision, RequestID: "cas-reset-1",
		Reason: "attempted stale reset", Actor: "maintainer",
	})
	var conflict *RuntimeRevisionConflictError
	if !errors.As(err, &conflict) || conflict.Current != interrupted.Revision {
		t.Fatalf("stale drift reset error = %T %#v, want RuntimeRevisionConflictError(%q)", err, err, interrupted.Revision)
	}
}

// TestRuntimeLedgerDriftResetRequiresMaintainerAuthorization confirms an
// empty actor is refused even when the drift-eligible precondition holds.
func TestRuntimeLedgerDriftResetRequiresMaintainerAuthorization(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "drift-authorization-guard")
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "auth-begin-1", WorkUnit: "sdd-objective",
		EvidenceGoal: "prove maintainer authorization", MaxAttempts: 2, MaxChangedLines: 400,
	})
	if err != nil {
		t.Fatal(err)
	}
	interrupted, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "auth-finish-1", Outcome: AttemptInterrupted,
		Diagnosis:          "transport closed before the executor was launched",
		HarnessDisposition: HarnessInvalidated, CleanupEvidence: "no executor process was ever spawned",
		ProcessEvidence: "pre-launch process scan found no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	appendRuntimeLedgerFile(t, repo, "corrected-plan-dependency\n")
	_, err = store.Reset(context.Background(), ResetObjectiveRequest{
		ExpectedRevision: interrupted.Revision, RequestID: "auth-reset-1",
		Reason: "attempted reset without an actor", Actor: "",
	})
	if err == nil {
		t.Fatal("reset without maintainer actor unexpectedly succeeded")
	}
}

// TestRuntimeLedgerResetWithoutDriftStillRequiresTerminalScope guards the
// narrower scope of the fix: an early elective reset after a terminal
// failed/interrupted attempt, with the candidate unchanged (begin would
// still succeed), must remain refused so the per-objective budget cannot be
// laundered by resetting instead of retrying.
func TestRuntimeLedgerResetWithoutDriftStillRequiresTerminalScope(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "no-drift-guard")
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "no-drift-begin-1", WorkUnit: "sdd-objective",
		EvidenceGoal: "prove undrifted reset refusal", MaxAttempts: 3, MaxChangedLines: 400,
	})
	if err != nil {
		t.Fatal(err)
	}
	interrupted, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "no-drift-finish-1", Outcome: AttemptInterrupted,
		Diagnosis:          "transport closed before the executor was launched",
		HarnessDisposition: HarnessInvalidated, CleanupEvidence: "no executor process was ever spawned",
		ProcessEvidence: "pre-launch process scan found no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Reset(context.Background(), ResetObjectiveRequest{
		ExpectedRevision: interrupted.Revision, RequestID: "no-drift-reset-1",
		Reason: "attempted early elective reset with no drift", Actor: "maintainer",
	})
	if !errors.Is(err, ErrRuntimeResetNotAllowed) {
		t.Fatalf("undrifted early reset error = %v, want ErrRuntimeResetNotAllowed", err)
	}
}
