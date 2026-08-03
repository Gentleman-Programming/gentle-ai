package sddstatus

import (
	"context"
	"errors"
	"testing"
)

// runtimeAdvanceFixture settles one passed apply objective so each test starts
// from the exact terminal state the reported deadlock reproduces.
type runtimeAdvanceFixture struct {
	repo   string
	store  RuntimeStore
	passed RuntimeStatus
}

const (
	advanceApplyWorkUnit    = "migration-schema-implementation"
	advanceApplyGoal        = "implement the migration schema"
	advanceVerifyWorkUnit   = "requirements-runtime-verification"
	advanceVerifyGoal       = "independently verify implementation against spec tasks"
	advanceApplyMaxAttempts = 3
	advanceApplyMaxLines    = 20
	// The successor budget deliberately differs from the apply budget in both
	// dimensions, so an assertion on it cannot pass while the predecessor's
	// budget is carried over.
	advanceVerifyMaxAttempts = 2
	advanceVerifyMaxLines    = 30
)

func newRuntimeAdvanceFixture(t *testing.T, change string) runtimeAdvanceFixture {
	t.Helper()
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, change)
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "advance-apply-begin", WorkUnit: advanceApplyWorkUnit,
		EvidenceGoal: advanceApplyGoal, MaxAttempts: advanceApplyMaxAttempts, MaxChangedLines: advanceApplyMaxLines,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendRuntimeLedgerFile(t, repo, "migration-schema\n")
	passed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "advance-apply-finish", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('a'), Diagnosis: "apply gates passed",
		HarnessDisposition: HarnessReused, CleanupEvidence: "apply cleanup completed",
		ProcessEvidence: "apply process scan found no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !passed.Complete || passed.ActiveAttempt != nil || passed.NextAction != RuntimeActionComplete {
		t.Fatalf("apply settlement did not reach the reported terminal state: %#v", passed)
	}
	return runtimeAdvanceFixture{repo: repo, store: store, passed: passed}
}

func (fixture runtimeAdvanceFixture) verifyRequest(requestID string) BeginAttemptRequest {
	return BeginAttemptRequest{
		ExpectedRevision: fixture.passed.Revision, RequestID: requestID, WorkUnit: advanceVerifyWorkUnit,
		EvidenceGoal: advanceVerifyGoal, MaxAttempts: advanceVerifyMaxAttempts, MaxChangedLines: advanceVerifyMaxLines,
	}
}

// A passed apply objective must not make the change-level ledger terminal for a
// distinct downstream work unit. The successor opens its own generation and
// budget while the completed apply attempts stay immutable and uncharged twice.
func TestRuntimeLedgerAdvancesDistinctWorkUnitAfterPassedObjective(t *testing.T) {
	fixture := newRuntimeAdvanceFixture(t, "objective-advance")
	before := countRuntimeRecords(t, fixture.store.Dir)
	request := fixture.verifyRequest("advance-verify-begin")

	advanced, err := fixture.store.Begin(context.Background(), request)
	if err != nil {
		t.Fatalf("distinct verification work unit was refused after a passed apply: %v", err)
	}
	if advanced.Complete || advanced.DecisionRequired || advanced.ActiveAttempt == nil ||
		advanced.ActiveAttempt.Ordinal != 2 || advanced.NextAction != RuntimeActionFinish {
		t.Fatalf("advanced status = %#v", advanced)
	}
	if advanced.Objective == nil || advanced.Objective.WorkUnit != advanceVerifyWorkUnit ||
		advanced.Objective.EvidenceGoal != advanceVerifyGoal || advanced.Objective.Generation != 2 ||
		advanced.Objective.ID == fixture.passed.Objective.ID || advanced.ObjectiveGeneration != 2 {
		t.Fatalf("advanced objective = %#v", advanced.Objective)
	}
	// The successor must hold the budget IT requested, not the one the passed
	// predecessor was bound to.
	if advanced.Objective.MaxAttempts != advanceVerifyMaxAttempts || advanced.Objective.MaxChangedLines != advanceVerifyMaxLines ||
		advanced.Objective.MaxAttempts == fixture.passed.Objective.MaxAttempts ||
		advanced.Objective.MaxChangedLines == fixture.passed.Objective.MaxChangedLines {
		t.Fatalf("successor inherited the predecessor budget: %#v", advanced.Objective)
	}
	if advanced.CumulativeAttempts != 1 || advanced.CumulativeChangedLines != 0 ||
		advanced.LifetimeAttempts != 2 || advanced.LifetimeChangedLines != fixture.passed.LifetimeChangedLines {
		t.Fatalf("advance laundered or double-charged budget: %#v", advanced)
	}
	if len(advanced.Attempts) != 2 || advanced.Attempts[0].Outcome != AttemptPassed ||
		advanced.Attempts[0].WorkUnit != advanceApplyWorkUnit ||
		advanced.Attempts[0].EvidenceRevision != runtimeTestHash('a') {
		t.Fatalf("advance mutated completed apply attempts: %#v", advanced.Attempts)
	}
	// Reset is the maintainer abandonment path and must not be implied here,
	// while the completed apply evidence stays auditable after the live
	// per-objective field is cleared for the successor scope.
	if advanced.LastReset != nil {
		t.Fatalf("advance recorded a maintainer reset: %#v", advanced.LastReset)
	}
	if advanced.LastAdvance == nil || advanced.LastAdvance.PreviousObjectiveID != fixture.passed.Objective.ID ||
		advanced.LastAdvance.PreviousGeneration != 1 || advanced.LastAdvance.PreviousWorkUnit != advanceApplyWorkUnit ||
		advanced.LastAdvance.PreviousEvidenceRevision != runtimeTestHash('a') {
		t.Fatalf("advance succession record = %#v", advanced.LastAdvance)
	}
	if advanced.EvidenceRevision != "" {
		t.Fatalf("successor objective inherited apply evidence: %q", advanced.EvidenceRevision)
	}
	if countRuntimeRecords(t, fixture.store.Dir) != before+1 {
		t.Fatalf("advance wrote %d records, want one", countRuntimeRecords(t, fixture.store.Dir)-before)
	}

	replayed, err := fixture.store.Begin(context.Background(), request)
	if err != nil || replayed.Revision != advanced.Revision || countRuntimeRecords(t, fixture.store.Dir) != before+1 {
		t.Fatalf("advance replay = %#v err=%v records=%d", replayed, err, countRuntimeRecords(t, fixture.store.Dir))
	}
}

// Advance is strictly narrower than reset: it may not reopen the scope that
// already passed, because that objective genuinely is complete.
func TestRuntimeLedgerRefusesAdvanceIntoTheSameWorkUnit(t *testing.T) {
	fixture := newRuntimeAdvanceFixture(t, "objective-advance-same-scope")
	before := countRuntimeRecords(t, fixture.store.Dir)

	for _, test := range []struct {
		name    string
		request BeginAttemptRequest
	}{
		{name: "identical scope", request: BeginAttemptRequest{
			ExpectedRevision: fixture.passed.Revision, RequestID: "advance-same-scope", WorkUnit: advanceApplyWorkUnit,
			EvidenceGoal: advanceApplyGoal, MaxAttempts: advanceApplyMaxAttempts, MaxChangedLines: advanceApplyMaxLines,
		}},
		{name: "restated evidence goal", request: BeginAttemptRequest{
			ExpectedRevision: fixture.passed.Revision, RequestID: "advance-restated-goal", WorkUnit: advanceApplyWorkUnit,
			EvidenceGoal: "implement the migration schema once more", MaxAttempts: advanceApplyMaxAttempts,
			MaxChangedLines: advanceApplyMaxLines,
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := fixture.store.Begin(context.Background(), test.request)
			if !errors.Is(err, ErrRuntimeObjectiveDone) {
				t.Fatalf("same-work-unit advance error = %T %v, want ErrRuntimeObjectiveDone", err, err)
			}
			status, statusErr := fixture.store.Status()
			if statusErr != nil || status.Revision != fixture.passed.Revision || !status.Complete ||
				countRuntimeRecords(t, fixture.store.Dir) != before {
				t.Fatalf("refused advance mutated authority: status=%#v err=%v records=%d",
					status, statusErr, countRuntimeRecords(t, fixture.store.Dir))
			}
		})
	}
}

// An exhausted or failed objective stays maintainer territory: advance never
// launders a budget that decision_required is holding.
func TestRuntimeLedgerRefusesAdvanceWhenDecisionIsRequired(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "objective-advance-exhausted")
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "exhausted-begin", WorkUnit: advanceApplyWorkUnit,
		EvidenceGoal: advanceApplyGoal, MaxAttempts: 1, MaxChangedLines: advanceApplyMaxLines,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendRuntimeLedgerFile(t, repo, "partial\n")
	exhausted, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "exhausted-finish", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('b'), Diagnosis: "apply failed and exhausted its budget",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed",
		ProcessEvidence: "process scan found no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !exhausted.DecisionRequired || exhausted.Complete {
		t.Fatalf("fixture did not reach decision_required: %#v", exhausted)
	}
	before := countRuntimeRecords(t, store.Dir)

	_, err = store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: exhausted.Revision, RequestID: "exhausted-advance", WorkUnit: advanceVerifyWorkUnit,
		EvidenceGoal: advanceVerifyGoal, MaxAttempts: advanceVerifyMaxAttempts, MaxChangedLines: advanceVerifyMaxLines,
	})
	if !errors.Is(err, ErrRuntimeBudgetExhausted) {
		t.Fatalf("advance past decision_required error = %T %v, want ErrRuntimeBudgetExhausted", err, err)
	}
	if countRuntimeRecords(t, store.Dir) != before {
		t.Fatalf("refused advance wrote %d records", countRuntimeRecords(t, store.Dir)-before)
	}
}

// The reported defect surfaced through the compact projection: acquire returned
// a bare `{"state":"complete"}` with no token, so the verifier could not launch.
func TestCompactAcquireProceedsForDistinctWorkUnitAfterPassedObjective(t *testing.T) {
	fixture := newRuntimeAdvanceFixture(t, "compact-objective-advance")
	before := countRuntimeRecords(t, fixture.store.Dir)
	request := CompactAcquireRequest{
		RequestID: "compact-advance-verify", WorkUnit: advanceVerifyWorkUnit, EvidenceGoal: advanceVerifyGoal,
		MaxAttempts: advanceVerifyMaxAttempts, MaxChangedLines: advanceVerifyMaxLines,
	}

	result, err := fixture.store.Acquire(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != CompactStateProceed || result.Token == "" || result.Reason != "" {
		t.Fatalf("verification acquire = %#v, want proceed with a token", result)
	}
	status, statusErr := fixture.store.Status()
	if statusErr != nil {
		t.Fatal(statusErr)
	}
	if status.ActiveAttempt == nil || status.ActiveAttempt.WorkUnit != advanceVerifyWorkUnit ||
		result.Token != status.Revision || countRuntimeRecords(t, fixture.store.Dir) != before+1 {
		t.Fatalf("acquire token/status = %#v %#v records=%d", result, status, countRuntimeRecords(t, fixture.store.Dir))
	}

	replayed, err := fixture.store.Acquire(context.Background(), request)
	if err != nil || replayed != result || countRuntimeRecords(t, fixture.store.Dir) != before+1 {
		t.Fatalf("acquire replay = %#v err=%v records=%d", replayed, err, countRuntimeRecords(t, fixture.store.Dir))
	}
}

func TestCompactAcquireStaysCompleteForTheSettledWorkUnit(t *testing.T) {
	fixture := newRuntimeAdvanceFixture(t, "compact-objective-advance-same-scope")
	before := countRuntimeRecords(t, fixture.store.Dir)

	result, err := fixture.store.Acquire(context.Background(), CompactAcquireRequest{
		RequestID: "compact-advance-same-scope", WorkUnit: advanceApplyWorkUnit, EvidenceGoal: advanceApplyGoal,
		MaxAttempts: advanceApplyMaxAttempts, MaxChangedLines: advanceApplyMaxLines,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.State != CompactStateComplete || result.Token != "" ||
		countRuntimeRecords(t, fixture.store.Dir) != before {
		t.Fatalf("settled-scope acquire = %#v records=%d", result, countRuntimeRecords(t, fixture.store.Dir))
	}
}
