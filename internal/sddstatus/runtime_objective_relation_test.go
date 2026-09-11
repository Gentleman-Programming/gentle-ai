package sddstatus

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestResetIndependentSuppressesTheObligationObservably is #4024's truthful
// exit: the maintainer declares --objective-relation independent on the
// EXACT reset that closes the failed objective (the original GH #4024
// scenario -- objective A discovers a missing prerequisite Y and cannot
// itself repair the failure). Acquiring the causally independent prerequisite
// afterward must disclose no obligation, must record the suppression
// observably (never silently), and must settle passed with no remediation
// claim.
func TestResetIndependentSuppressesTheObligationObservably(t *testing.T) {
	ctx := context.Background()
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "independent-declared")
	store.ReviewDisabled = true

	appendRuntimeLedgerFile(t, repo, "objective A work\n")
	first, err := store.Begin(ctx, BeginAttemptRequest{
		RequestID: "a-begin", WorkUnit: "task-a-needs-missing-prereq",
		EvidenceGoal: "implement task A", MaxAttempts: 1, MaxChangedLines: 400,
	})
	if err != nil {
		t.Fatal(err)
	}
	failedEvidence := runtimeTestHash('a')
	failed, err := store.Finish(ctx, FinishAttemptRequest{
		ExpectedRevision: first.Revision, RequestID: "a-finish", Outcome: AttemptFailed,
		EvidenceRevision: failedEvidence, Diagnosis: "discovered missing prerequisite Y",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed", ProcessEvidence: "no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	failedObjectiveID := failed.Attempts[0].ObjectiveID
	// The SAME maintainer authorization reset already requires (Reason+Actor)
	// carries the declaration; no separate consent path exists for it.
	if _, err := store.Reset(ctx, ResetObjectiveRequest{
		ExpectedRevision: failed.Revision, RequestID: "audited-reset",
		Reason: "open independent prerequisite Y", Actor: "maintainer",
		Relation: RuntimeObjectiveRelationIndependent,
	}); err != nil {
		t.Fatal(err)
	}

	acquired, err := store.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: BeginAttemptRequest{
		RequestID: "b-acquire", WorkUnit: "implement-prerequisite-y",
		EvidenceGoal: "add missing prerequisite Y, unrelated to task A", MaxAttempts: 2, MaxChangedLines: 400,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if acquired.State != CompactStateProceed {
		t.Fatalf("acquire = %#v, want state=proceed", acquired)
	}
	if acquired.SettleObligation != "" {
		t.Fatalf("acquire disclosed an obligation the declared-independent successor cannot truthfully owe:\n%s", acquired.SettleObligation)
	}
	if acquired.SuppressedObligation == nil {
		t.Fatal("suppression happened silently instead of being recorded observably (#4024)")
	}
	if acquired.SuppressedObligation.Reason != "declared_independent" ||
		acquired.SuppressedObligation.EvidenceRevision != failedEvidence ||
		acquired.SuppressedObligation.ObjectiveID != failedObjectiveID {
		t.Fatalf("suppressed_obligation = %#v, want reason declared_independent, evidence %s, objective %s",
			acquired.SuppressedObligation, failedEvidence, failedObjectiveID)
	}

	appendRuntimeLedgerFile(t, repo, "objective B work\n")
	settled, err := store.Settle(ctx, CompactSettleRequest{
		Token: acquired.Token, RequestID: "b-settle", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('b'), Diagnosis: "prerequisite Y implemented",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed", ProcessEvidence: "no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	if settled.State != CompactStateProceed && settled.State != CompactStateComplete {
		t.Fatalf("settling the declared-independent prerequisite = %#v, want proceed or complete -- there is no truthful remediation claim to make", settled)
	}
}

// TestResetDefaultRelationStillInheritsTheObligation is the companion
// boundary: omitting --objective-relation (or passing "remediation"
// explicitly) preserves today's #1974 slice 2 behavior exactly -- the
// successor still owes the chain's unremediated failure, and settling passed
// without declaring the remediation is still refused.
func TestResetDefaultRelationStillInheritsTheObligation(t *testing.T) {
	ctx := context.Background()
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "reset-remediation-default")
	store.ReviewDisabled = true

	appendRuntimeLedgerFile(t, repo, "objective work\n")
	first, err := store.Begin(ctx, BeginAttemptRequest{
		RequestID: "a-begin", WorkUnit: "verify", EvidenceGoal: "independent verification",
		MaxAttempts: 1, MaxChangedLines: 400,
	})
	if err != nil {
		t.Fatal(err)
	}
	failedEvidence := runtimeTestHash('c')
	failed, err := store.Finish(ctx, FinishAttemptRequest{
		ExpectedRevision: first.Revision, RequestID: "a-finish", Outcome: AttemptFailed,
		EvidenceRevision: failedEvidence, Diagnosis: "verification failed",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed", ProcessEvidence: "no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	// No --objective-relation at all: the zero value is the default.
	reset, err := store.Reset(ctx, ResetObjectiveRequest{
		ExpectedRevision: failed.Revision, RequestID: "audited-reset",
		Reason: "retry the same verification under a fresh generation", Actor: "maintainer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if reset.LastReset == nil || reset.LastReset.Relation != "" {
		t.Fatalf("default reset recorded a non-empty relation: %#v", reset.LastReset)
	}
	// The on-disk record omits the field entirely (omitempty), so it is
	// byte-identical to a record a pre-#4024 binary would have written --
	// this IS the legacy shape, not a simulation of it.
	raw, err := os.ReadFile(filepath.Join(store.Dir, "records", strings.TrimPrefix(reset.Revision, "sha256:")+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"relation":`) {
		t.Fatalf("default-relation reset record is not byte-compatible with a legacy record:\n%s", raw)
	}

	acquired, err := store.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: BeginAttemptRequest{
		RequestID: "b-acquire", WorkUnit: "verify", EvidenceGoal: "independent verification",
		MaxAttempts: 2, MaxChangedLines: 400,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if acquired.SettleObligation == "" {
		t.Fatal("acquire dropped the obligation for a default-relation (remediation) retry")
	}
	if acquired.SuppressedObligation != nil {
		t.Fatalf("acquire suppressed an obligation that should have been inherited: %#v", acquired.SuppressedObligation)
	}

	appendRuntimeLedgerFile(t, repo, "the retry\n")
	plain, err := store.Settle(ctx, CompactSettleRequest{
		Token: acquired.Token, RequestID: "b-settle-plain", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('d'), Diagnosis: "retry passed",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed", ProcessEvidence: "no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	if plain.State != CompactStateBlocked {
		t.Fatalf("a default-relation retry settled %v with no remediation claim; the force demand must still apply", plain.State)
	}
}

// TestLegacyResetReplaysAsRemediation replays a reset record from BEFORE
// #4024's Relation field existed (constructed here by stripping the key a
// current write would still omit for the empty/default case, then reopening
// a fresh store to force a full from-genesis replay). A legacy reset must
// replay exactly as RuntimeObjectiveRelationRemediation: the successor still
// inherits the chain's unremediated failure.
func TestSupersedeRequiresDistinctCausalScope(t *testing.T) {
	for _, test := range []struct {
		name, workUnit, evidenceGoal string
		wantErr                      bool
	}{
		{"same scope", "verify-a", "verify A", true},
		{"work-unit-only", "verify-b", "verify A", false},
		{"evidence-goal-only", "verify-a", "verify B", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			repo := initRuntimeLedgerRepo(t)
			store := mustRuntimeStore(t, repo, "supersede-scope-"+test.name)
			started, err := store.Begin(ctx, BeginAttemptRequest{RequestID: "begin", WorkUnit: "verify-a", EvidenceGoal: "verify A", MaxAttempts: 2, MaxChangedLines: 40})
			if err != nil {
				t.Fatal(err)
			}
			failed, err := store.Finish(ctx, FinishAttemptRequest{ExpectedRevision: started.Revision, RequestID: "finish", Outcome: AttemptFailed, EvidenceRevision: runtimeTestHash('a'), Diagnosis: "failed", HarnessDisposition: HarnessReused, CleanupEvidence: "unchanged", ProcessEvidence: "none"})
			if err != nil {
				t.Fatal(err)
			}
			status, err := store.Supersede(ctx, SupersedeObjectiveRequest{ExpectedRevision: failed.Revision, RequestID: "supersede", WorkUnit: test.workUnit, EvidenceGoal: test.evidenceGoal, MaxAttempts: 3, MaxChangedLines: 80, Reason: "distinct successor", Actor: "maintainer"})
			if test.wantErr {
				if !errors.Is(err, ErrRuntimeSupersedeNotAllowed) {
					t.Fatalf("same-scope supersede = %v", err)
				}
				unchanged, statusErr := store.Status()
				if statusErr != nil || unchanged.Revision != failed.Revision || countRuntimeRecords(t, store.Dir) != 2 {
					t.Fatalf("same-scope supersede mutated ledger: status=%#v err=%v", unchanged, statusErr)
				}
				return
			}
			if err != nil || status.Objective.WorkUnit != test.workUnit || status.Objective.EvidenceGoal != test.evidenceGoal {
				t.Fatalf("distinct supersede = %#v, %v", status, err)
			}
		})
	}
}

func TestSupersedeSameScopePreservesCompleteObjectiveRefusal(t *testing.T) {
	ctx, repo := context.Background(), initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "supersede-complete-precedence")
	started, err := store.Begin(ctx, BeginAttemptRequest{RequestID: "begin", WorkUnit: "verify-a", EvidenceGoal: "verify A", MaxAttempts: 1, MaxChangedLines: 40})
	if err != nil {
		t.Fatal(err)
	}
	complete, err := store.Finish(ctx, FinishAttemptRequest{ExpectedRevision: started.Revision, RequestID: "finish", Outcome: AttemptPassed, EvidenceRevision: runtimeTestHash('c'), Diagnosis: "passed", HarnessDisposition: HarnessReused, CleanupEvidence: "unchanged", ProcessEvidence: "none"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Supersede(ctx, SupersedeObjectiveRequest{ExpectedRevision: complete.Revision, RequestID: "supersede", WorkUnit: "verify-a", EvidenceGoal: "verify A", MaxAttempts: 3, MaxChangedLines: 80, Reason: "same scope", Actor: "maintainer"})
	if !errors.Is(err, ErrRuntimeObjectiveDone) {
		t.Fatalf("complete same-scope supersede = %v, want ErrRuntimeObjectiveDone", err)
	}
}

func TestSupersedeReplayRejectsSameCausalScope(t *testing.T) {
	ctx, repo := context.Background(), initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "supersede-same-scope-replay")
	started, err := store.Begin(ctx, BeginAttemptRequest{RequestID: "begin", WorkUnit: "verify-a", EvidenceGoal: "verify A", MaxAttempts: 2, MaxChangedLines: 40})
	if err != nil {
		t.Fatal(err)
	}
	failed, err := store.Finish(ctx, FinishAttemptRequest{ExpectedRevision: started.Revision, RequestID: "finish", Outcome: AttemptFailed, EvidenceRevision: runtimeTestHash('a'), Diagnosis: "failed", HarnessDisposition: HarnessReused, CleanupEvidence: "unchanged", ProcessEvidence: "none"})
	if err != nil {
		t.Fatal(err)
	}
	objective, last := failed.Objective, failed.Attempts[len(failed.Attempts)-1]
	fresh, err := captureRuntimeCandidate(ctx, repo, []string{})
	if err != nil {
		t.Fatal(err)
	}
	request := SupersedeObjectiveRequest{ExpectedRevision: failed.Revision, RequestID: "forged", WorkUnit: objective.WorkUnit, EvidenceGoal: objective.EvidenceGoal, MaxAttempts: 3, MaxChangedLines: 80, Reason: "forged budget bypass", Actor: "attacker"}
	normalized, err := normalizeSupersedeObjectiveRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	event := &runtimeSupersedeEvent{PreviousObjectiveID: objective.ID, PreviousGeneration: objective.Generation, PreviousMaxAttempts: objective.MaxAttempts, PreviousMaxChangedLines: objective.MaxChangedLines, SupersedeCandidateIdentity: fresh.Identity, SupersedeCandidateTree: last.FinishCandidateTree, ObjectiveID: runtimeObjectiveID(store.Change, request.WorkUnit, request.EvidenceGoal, fresh.Identity, failed.ObjectiveGeneration+1), ObjectiveGeneration: failed.ObjectiveGeneration + 1, WorkUnit: request.WorkUnit, EvidenceGoal: request.EvidenceGoal, MaxAttempts: request.MaxAttempts, MaxChangedLines: request.MaxChangedLines, Reason: request.Reason, Actor: request.Actor}
	record := runtimeRecord{Schema: runtimeRecordSchema, Change: store.Change, PreviousRevision: failed.Revision, Operation: runtimeOperationSupersede, RequestID: request.RequestID, RequestDigest: runtimeValueHash("gentle-ai.sdd-runtime-supersede-request/v1", normalized), Supersede: event}
	if err := validateRuntimeRecordShape(record); err != nil {
		t.Fatalf("same-scope record failed shape validation: %v", err)
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
	if _, err = store.Status(); err == nil || !strings.Contains(err.Error(), "objective_supersede_same_scope") {
		t.Fatalf("replay of forged same-scope supersede = %v", err)
	}
}

func TestSupersedeClosesDistinctZeroDriftRemediationWithoutFalseNarrowing(t *testing.T) {
	ctx := context.Background()
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "supersede-4024")
	started, err := store.Begin(ctx, BeginAttemptRequest{RequestID: "a-begin", WorkUnit: "verify-a", EvidenceGoal: "verify A", MaxAttempts: 2, MaxChangedLines: 40})
	if err != nil {
		t.Fatal(err)
	}
	if _, e := store.Supersede(ctx, SupersedeObjectiveRequest{ExpectedRevision: started.Revision, RequestID: "active", WorkUnit: "b", EvidenceGoal: "b", MaxAttempts: 3, MaxChangedLines: 80, Reason: "x", Actor: "x"}); !errors.Is(e, ErrRuntimeAttemptActive) {
		t.Fatalf("active supersede = %v", e)
	}
	failedEvidence := runtimeTestHash('a')
	failed, err := store.Finish(ctx, FinishAttemptRequest{ExpectedRevision: started.Revision, RequestID: "a-finish", Outcome: AttemptFailed, EvidenceRevision: failedEvidence, Diagnosis: "A found a distinct remediation", HarnessDisposition: HarnessReused, CleanupEvidence: "unchanged", ProcessEvidence: "none"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Begin(ctx, BeginAttemptRequest{ExpectedRevision: failed.Revision, RequestID: "changed-acquire", WorkUnit: "remediate-b", EvidenceGoal: "repair distinct B", MaxAttempts: 2, MaxChangedLines: 40}); !errors.Is(err, ErrRuntimeObjectiveChange) || !strings.Contains(err.Error(), "sdd-attempt supersede") || !strings.Contains(err.Error(), "sdd-attempt rescope") {
		t.Fatalf("changed objective route = %v", err)
	}
	request := SupersedeObjectiveRequest{ExpectedRevision: failed.Revision, RequestID: "supersede-a-b", WorkUnit: "remediate-b", EvidenceGoal: "repair distinct B", MaxAttempts: 3, MaxChangedLines: 80, Reason: "maintainer authorized distinct remediation", Actor: "maintainer"}
	superseded, err := store.Supersede(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if superseded.LastSupersede == nil || superseded.Objective.WorkUnit != "remediate-b" || superseded.CumulativeAttempts != 1 || superseded.LifetimeAttempts != 1 {
		t.Fatalf("supersede = %#v", superseded)
	}
	replayed, err := store.Supersede(ctx, request)
	if err != nil || replayed.Revision != superseded.Revision {
		t.Fatalf("replay = %#v, %v", replayed, err)
	}
	_, err = store.Supersede(ctx, SupersedeObjectiveRequest{ExpectedRevision: failed.Revision, RequestID: "supersede-other", WorkUnit: "x", EvidenceGoal: "x", MaxAttempts: 1, MaxChangedLines: 1, Reason: "x", Actor: "x"})
	if !errors.Is(err, ErrRuntimeRevisionConflict) {
		t.Fatalf("CAS = %v", err)
	}
	acquired, err := store.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: BeginAttemptRequest{RequestID: "b-acquire", WorkUnit: "remediate-b", EvidenceGoal: "repair distinct B", MaxAttempts: 3, MaxChangedLines: 80}})
	if err != nil || acquired.SettleObligation == "" {
		t.Fatalf("remediation inheritance = %#v, %v", acquired, err)
	}
}
func TestSupersedeIndependentSuppressesFailedEvidence(t *testing.T) {
	ctx, repo := context.Background(), initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "supersede-independent")
	started, err := store.Begin(ctx, BeginAttemptRequest{RequestID: "a", WorkUnit: "verify", EvidenceGoal: "verify", MaxAttempts: 2, MaxChangedLines: 40})
	if err != nil {
		t.Fatal(err)
	}
	failed, err := store.Finish(ctx, FinishAttemptRequest{ExpectedRevision: started.Revision, RequestID: "af", Outcome: AttemptFailed, EvidenceRevision: runtimeTestHash('b'), Diagnosis: "distinct prerequisite", HarnessDisposition: HarnessReused, CleanupEvidence: "unchanged", ProcessEvidence: "none"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Supersede(ctx, SupersedeObjectiveRequest{ExpectedRevision: failed.Revision, RequestID: "ab", WorkUnit: "prerequisite", EvidenceGoal: "independent", MaxAttempts: 3, MaxChangedLines: 80, Reason: "independent prerequisite", Actor: "maintainer", Relation: RuntimeObjectiveRelationIndependent})
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: BeginAttemptRequest{RequestID: "b", WorkUnit: "prerequisite", EvidenceGoal: "independent", MaxAttempts: 3, MaxChangedLines: 80}})
	if err != nil || result.SettleObligation != "" || result.SuppressedObligation == nil || result.SuppressedObligation.Reason != "declared_independent" {
		t.Fatalf("independent successor = %#v, %v", result, err)
	}
}

func TestLegacyResetReplaysAsRemediation(t *testing.T) {
	ctx := context.Background()
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "legacy-reset-replay")
	store.ReviewDisabled = true

	appendRuntimeLedgerFile(t, repo, "objective work\n")
	first, err := store.Begin(ctx, BeginAttemptRequest{
		RequestID: "a-begin", WorkUnit: "verify", EvidenceGoal: "independent verification",
		MaxAttempts: 1, MaxChangedLines: 400,
	})
	if err != nil {
		t.Fatal(err)
	}
	failedEvidence := runtimeTestHash('e')
	failed, err := store.Finish(ctx, FinishAttemptRequest{
		ExpectedRevision: first.Revision, RequestID: "a-finish", Outcome: AttemptFailed,
		EvidenceRevision: failedEvidence, Diagnosis: "verification failed",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed", ProcessEvidence: "no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	reset, err := store.Reset(ctx, ResetObjectiveRequest{
		ExpectedRevision: failed.Revision, RequestID: "legacy-reset",
		Reason: "retry the same verification under a fresh generation", Actor: "maintainer",
	})
	if err != nil {
		t.Fatal(err)
	}
	recordPath := filepath.Join(store.Dir, "records", strings.TrimPrefix(reset.Revision, "sha256:")+".json")
	raw, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"relation":`) {
		t.Fatalf("precondition failed: the default-relation record already carries a relation key, cannot stand in for a legacy record:\n%s", raw)
	}

	// A fresh store instance replays this exact on-disk chain from genesis --
	// there is no in-memory state left over from the write above.
	reopened := mustRuntimeStore(t, repo, "legacy-reset-replay")
	replayed, err := reopened.Status()
	if err != nil {
		t.Fatal(err)
	}
	if replayed.LastReset == nil ||
		(replayed.LastReset.Relation != RuntimeObjectiveRelationRemediation && replayed.LastReset.Relation != "") {
		t.Fatalf("legacy reset replayed with relation %q, want empty/remediation", replayed.LastReset.Relation)
	}

	acquired, err := reopened.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: BeginAttemptRequest{
		RequestID: "b-acquire", WorkUnit: "verify", EvidenceGoal: "independent verification",
		MaxAttempts: 2, MaxChangedLines: 400,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if acquired.SettleObligation == "" {
		t.Fatal("a legacy reset (no recorded relation) failed to inherit the obligation on replay")
	}
	if acquired.SuppressedObligation != nil {
		t.Fatalf("a legacy reset was replayed as independent instead of remediation: %#v", acquired.SuppressedObligation)
	}
}
