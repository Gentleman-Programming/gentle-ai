package sddstatus

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
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
		BeginAttemptRequest: BeginAttemptRequest{
			RequestID: "compact-advance-verify", WorkUnit: advanceVerifyWorkUnit, EvidenceGoal: advanceVerifyGoal,
			MaxAttempts: advanceVerifyMaxAttempts, MaxChangedLines: advanceVerifyMaxLines,
		},
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
		BeginAttemptRequest: BeginAttemptRequest{
			RequestID: "compact-advance-same-scope", WorkUnit: advanceApplyWorkUnit, EvidenceGoal: advanceApplyGoal,
			MaxAttempts: advanceApplyMaxAttempts, MaxChangedLines: advanceApplyMaxLines,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.State != CompactStateComplete || result.Token != "" ||
		countRuntimeRecords(t, fixture.store.Dir) != before {
		t.Fatalf("settled-scope acquire = %#v records=%d", result, countRuntimeRecords(t, fixture.store.Dir))
	}
}

// ---------------------------------------------------------------------------
// Phase 0 regression gate (qa-orchestrator-v2 #3282, tasks 0.1/0.2)
// ---------------------------------------------------------------------------

// updateRuntimeLedgerVocabularyLessGolden regenerates the golden fixtures
// TestRuntimeLedgerVocabularyLessChainReplaysByteIdentically compares
// against, following the same -update convention as
// internal/components/golden_test.go and
// internal/cli/review_new_lineage_switch_off_golden_test.go.
var updateRuntimeLedgerVocabularyLessGolden = flag.Bool("update", false, "update the vocabulary-less runtime ledger golden fixtures")

const (
	vocabularyLessGoldenChange     = "qa-orchestrator-v2-regression-gate"
	vocabularyLessGoldenApplyWork  = "implement-migration-schema"
	vocabularyLessGoldenApplyGoal  = "prove the vocabulary-less chain stays byte-identical"
	vocabularyLessGoldenVerifyWork = "verify-migration-schema"
	vocabularyLessGoldenVerifyGoal = "independently verify implementation against spec tasks"
)

// TestRuntimeLedgerVocabularyLessChainReplaysByteIdentically is Phase 0 of
// qa-orchestrator-v2 (#3282): the regression gate that blocks every
// runtime_ledger.go edit in later phases. It hand-authors an ordinary
// Begin->Finish->Begin(advance)->Finish chain with NO stage vocabulary --
// today's only real usage pattern -- directly through runtimeRecord/
// runtimeBeginEvent/runtimeFinishEvent/runtimeAdvanceEvent and the package's
// own runtimeObjectiveID/runtimeValueHash/runtimeRecordRevision helpers
// (never through a live git repo), so every persisted byte is a pure
// function of fixed literal inputs -- no wall clock, no host-specific
// temp-directory path (BeginWorktree is deliberately left "", the same
// legacy/no-binding-recorded shape #2296 already defines for chains that
// predate that field), and therefore fully reproducible across machines and
// runs. It asserts the exact bytes of each immutable record, plus the exact
// bytes of the final replayed RuntimeStatus, are byte-identical to goldens
// captured from the pre-change binary.
//
// This test is the safety net, not a test of new behavior: there is no
// StageVocabulary/StagePosition/ApprovalRevision field yet. Once Phase 2/3
// add those fields as omitempty (design decision D1), this exact test must
// keep passing unmodified for a chain that never sets them -- any field
// that leaks a non-empty value into a vocabulary-less chain, or any change
// to runtimeObjectiveAdvanceAdmissible's/applyRuntimeAdvanceEvent's ordering
// for a vocabulary-less predecessor, fails this test first.
func TestRuntimeLedgerVocabularyLessChainReplaysByteIdentically(t *testing.T) {
	const change = vocabularyLessGoldenChange
	store := RuntimeStore{Dir: t.TempDir(), Change: change}

	applyCandidateIdentity := "sha256:" + strings.Repeat("a", 64)
	applyCandidateTree := strings.Repeat("a", 40)
	applyGeneration := 1
	applyObjectiveID := runtimeObjectiveID(change, vocabularyLessGoldenApplyWork, vocabularyLessGoldenApplyGoal, applyCandidateIdentity, applyGeneration)
	beginApplyRequest := BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "golden-apply-begin", WorkUnit: vocabularyLessGoldenApplyWork,
		EvidenceGoal: vocabularyLessGoldenApplyGoal, MaxAttempts: 3, MaxChangedLines: 20,
	}
	record1 := runtimeRecord{
		Schema: runtimeRecordSchema, Change: change, PreviousRevision: "", Operation: runtimeOperationBegin,
		RequestID: beginApplyRequest.RequestID,
		RequestDigest: runtimeValueHash("gentle-ai.sdd-runtime-begin-request/v1", beginApplyRequest),
		Begin: &runtimeBeginEvent{
			ObjectiveID: applyObjectiveID, ObjectiveGeneration: applyGeneration, WorkUnit: beginApplyRequest.WorkUnit,
			EvidenceGoal: beginApplyRequest.EvidenceGoal, MaxAttempts: beginApplyRequest.MaxAttempts,
			MaxChangedLines: beginApplyRequest.MaxChangedLines, Ordinal: 1,
			BeginCandidateIdentity: applyCandidateIdentity, BeginCandidateTree: applyCandidateTree,
		},
	}
	revision1, payload1 := writeGoldenRuntimeRecord(t, store, record1)
	assertRuntimeLedgerGoldenBytes(t, "01-begin-apply.golden.json", payload1)

	applyFinishCandidateIdentity := "sha256:" + strings.Repeat("b", 64)
	applyFinishCandidateTree := strings.Repeat("b", 40)
	finishApplyRequest := FinishAttemptRequest{
		ExpectedRevision: revision1, RequestID: "golden-apply-finish", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('c'), Diagnosis: "apply gates passed",
		HarnessDisposition: HarnessReused, CleanupEvidence: "apply cleanup completed",
		ProcessEvidence: "apply process scan found no descendants",
	}
	record2 := runtimeRecord{
		Schema: runtimeRecordSchema, Change: change, PreviousRevision: revision1, Operation: runtimeOperationFinish,
		RequestID: finishApplyRequest.RequestID,
		RequestDigest: runtimeValueHash("gentle-ai.sdd-runtime-finish-request/v1", finishApplyRequest),
		Finish: &runtimeFinishEvent{
			Ordinal: 1, FinishCandidateIdentity: applyFinishCandidateIdentity, FinishCandidateTree: applyFinishCandidateTree,
			Outcome: finishApplyRequest.Outcome, ChangedLines: 1, EvidenceRevision: finishApplyRequest.EvidenceRevision,
			Diagnosis: finishApplyRequest.Diagnosis, HarnessDisposition: finishApplyRequest.HarnessDisposition,
			CleanupEvidence: finishApplyRequest.CleanupEvidence, ProcessEvidence: finishApplyRequest.ProcessEvidence,
		},
	}
	revision2, payload2 := writeGoldenRuntimeRecord(t, store, record2)
	assertRuntimeLedgerGoldenBytes(t, "02-finish-apply.golden.json", payload2)

	verifyGeneration := 2
	// The verify objective's candidate continues from the passed apply
	// attempt's finish candidate -- the same continuity a live advance
	// produces -- though replay's objective==nil begin branch (taken right
	// after an advance clears Objective) does not itself re-check it.
	verifyObjectiveID := runtimeObjectiveID(change, vocabularyLessGoldenVerifyWork, vocabularyLessGoldenVerifyGoal, applyFinishCandidateIdentity, verifyGeneration)
	beginVerifyRequest := BeginAttemptRequest{
		ExpectedRevision: revision2, RequestID: "golden-verify-begin", WorkUnit: vocabularyLessGoldenVerifyWork,
		EvidenceGoal: vocabularyLessGoldenVerifyGoal, MaxAttempts: 2, MaxChangedLines: 30,
	}
	record3 := runtimeRecord{
		Schema: runtimeRecordSchema, Change: change, PreviousRevision: revision2, Operation: runtimeOperationAdvance,
		RequestID: beginVerifyRequest.RequestID,
		RequestDigest: runtimeValueHash("gentle-ai.sdd-runtime-begin-request/v1", beginVerifyRequest),
		Begin: &runtimeBeginEvent{
			ObjectiveID: verifyObjectiveID, ObjectiveGeneration: verifyGeneration, WorkUnit: beginVerifyRequest.WorkUnit,
			EvidenceGoal: beginVerifyRequest.EvidenceGoal, MaxAttempts: beginVerifyRequest.MaxAttempts,
			MaxChangedLines: beginVerifyRequest.MaxChangedLines, Ordinal: 2,
			BeginCandidateIdentity: applyFinishCandidateIdentity, BeginCandidateTree: applyFinishCandidateTree,
		},
		Advance: &runtimeAdvanceEvent{
			PreviousObjectiveID: applyObjectiveID, PreviousGeneration: applyGeneration,
			PreviousWorkUnit: vocabularyLessGoldenApplyWork,
		},
	}
	revision3, payload3 := writeGoldenRuntimeRecord(t, store, record3)
	assertRuntimeLedgerGoldenBytes(t, "03-begin-advance-verify.golden.json", payload3)

	verifyFinishCandidateIdentity := "sha256:" + strings.Repeat("e", 64)
	verifyFinishCandidateTree := strings.Repeat("e", 40)
	finishVerifyRequest := FinishAttemptRequest{
		ExpectedRevision: revision3, RequestID: "golden-verify-finish", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('d'), Diagnosis: "verify gates passed",
		HarnessDisposition: HarnessReused, CleanupEvidence: "verify cleanup completed",
		ProcessEvidence: "verify process scan found no descendants",
	}
	record4 := runtimeRecord{
		Schema: runtimeRecordSchema, Change: change, PreviousRevision: revision3, Operation: runtimeOperationFinish,
		RequestID: finishVerifyRequest.RequestID,
		RequestDigest: runtimeValueHash("gentle-ai.sdd-runtime-finish-request/v1", finishVerifyRequest),
		Finish: &runtimeFinishEvent{
			Ordinal: 2, FinishCandidateIdentity: verifyFinishCandidateIdentity, FinishCandidateTree: verifyFinishCandidateTree,
			Outcome: finishVerifyRequest.Outcome, ChangedLines: 1, EvidenceRevision: finishVerifyRequest.EvidenceRevision,
			Diagnosis: finishVerifyRequest.Diagnosis, HarnessDisposition: finishVerifyRequest.HarnessDisposition,
			CleanupEvidence: finishVerifyRequest.CleanupEvidence, ProcessEvidence: finishVerifyRequest.ProcessEvidence,
		},
	}
	revision4, payload4 := writeGoldenRuntimeRecord(t, store, record4)
	assertRuntimeLedgerGoldenBytes(t, "04-finish-verify.golden.json", payload4)

	if countRuntimeRecords(t, store.Dir) != 4 {
		t.Fatalf("vocabulary-less golden chain wrote %d records, want 4", countRuntimeRecords(t, store.Dir))
	}

	// Status() always replays from the persisted records (store.load(), never
	// an in-memory cache), so this is a genuine replay assertion over the
	// exact bytes written above, exercised through the CURRENT
	// load()/applyRuntimeRecord/applyRuntimeBeginEvent/
	// applyRuntimeAdvanceEvent/applyRuntimeFinishEvent/
	// validateRuntimeRecordShape code.
	replayed, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Revision != revision4 || !replayed.Complete {
		t.Fatalf("replayed terminal status = %#v, want complete at %q", replayed, revision4)
	}
	statusJSON, err := json.MarshalIndent(replayed, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	statusJSON = append(statusJSON, '\n')
	assertRuntimeLedgerGoldenBytes(t, "05-final-status.golden.json", statusJSON)
}

// writeGoldenRuntimeRecord computes the exact immutable bytes gentle-ai
// would persist for record (via runtimeRecordRevision, the same function
// RuntimeStore.mutate uses at publication), writes them under
// store.Dir/records, advances HEAD to the new revision (the same bytes
// publishHead writes: "sha256:<64 hex>\n"), and returns the revision and the
// exact persisted payload for golden comparison.
func writeGoldenRuntimeRecord(t *testing.T, store RuntimeStore, record runtimeRecord) (string, []byte) {
	t.Helper()
	revision, payload, err := runtimeRecordRevision(record)
	if err != nil {
		t.Fatal(err)
	}
	recordsDir := filepath.Join(store.Dir, "records")
	if err := os.MkdirAll(recordsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(recordsDir, strings.TrimPrefix(revision, "sha256:")+".json")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.Dir, "HEAD"), []byte(revision+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return revision, payload
}

func assertRuntimeLedgerGoldenBytes(t *testing.T, name string, actual []byte) {
	t.Helper()
	goldenPath := filepath.Join("testdata", "runtime_ledger_vocabulary_less_chain", name)
	if *updateRuntimeLedgerVocabularyLessGolden {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, actual, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("updated vocabulary-less runtime ledger golden: %s", goldenPath)
		return
	}
	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read vocabulary-less runtime ledger golden %q: %v (regenerate with `go test ./internal/sddstatus/... -run TestRuntimeLedgerVocabularyLessChainReplaysByteIdentically -update`)", goldenPath, err)
	}
	if !bytes.Equal(actual, expected) {
		t.Fatalf("vocabulary-less runtime ledger chain diverged from golden %s (this test gates ALL later runtime_ledger.go phases -- see qa-orchestrator-v2 Phase 0/2/3 in sdd/qa-orchestrator-v2/tasks):\ngot:\n%s\nwant:\n%s",
			name, string(actual), string(expected))
	}
}
