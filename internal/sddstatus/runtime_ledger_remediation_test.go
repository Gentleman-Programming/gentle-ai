package sddstatus

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestRuntimeRemediationFinishAtomicallyChargesEvidenceAndBinding(t *testing.T) {
	fixture := newRuntimeRemediationFixture(t, true)
	beforeRecords := countRuntimeRecords(t, fixture.store.Dir)
	request := fixture.finishRequest("finish-remediation")

	completed, err := fixture.store.Finish(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if completed.ActiveAttempt != nil || !completed.Complete || completed.NextAction != RuntimeActionComplete {
		t.Fatalf("completed remediation status = %#v", completed)
	}
	if completed.Binding == nil || completed.Binding.Lineage != fixture.successor.State.LineageID ||
		completed.BindingRevision == fixture.predecessorBinding.Revision {
		t.Fatalf("atomic successor binding = %#v", completed.Binding)
	}
	if completed.EvidenceRevision != request.EvidenceRevision || completed.CumulativeChangedLines == 0 {
		t.Fatalf("atomic remediation charge/evidence = %#v", completed)
	}
	last := completed.Attempts[len(completed.Attempts)-1]
	if last.Outcome != AttemptPassed || last.RemediatesEvidenceRevision != fixture.failedEvidence ||
		last.EvidenceRevision != request.EvidenceRevision || last.ChangedLines == 0 {
		t.Fatalf("completed remediation attempt = %#v", last)
	}
	if countRuntimeRecords(t, fixture.store.Dir) != beforeRecords+1 {
		t.Fatalf("atomic finish appended %d records, want one", countRuntimeRecords(t, fixture.store.Dir)-beforeRecords)
	}
	record, err := fixture.store.loadRecord(completed.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if record.Operation != runtimeOperationFinishRemediation || record.Finish == nil || record.Binding == nil ||
		record.Binding.LegacyImport != nil || record.Binding.ExpectedRevision != fixture.predecessorBinding.Revision ||
		record.Binding.Current.Lineage != fixture.successor.State.LineageID {
		t.Fatalf("atomic remediation record = %#v", record)
	}

	// Exact request replay is native-ledger-only. Once the single HEAD CAS is
	// committed, a later compact artifact failure cannot turn it into a second
	// external validation or append another record.
	successorStore, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), fixture.repo, fixture.successor.State.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(successorStore.ReceiptPath()); err != nil {
		t.Fatal(err)
	}
	replayed, err := fixture.store.Finish(context.Background(), request)
	if err != nil {
		t.Fatalf("exact remediation replay revalidated compact authority: %v", err)
	}
	if replayed.Revision != completed.Revision || countRuntimeRecords(t, fixture.store.Dir) != beforeRecords+1 {
		t.Fatalf("exact remediation replay = %#v records=%d", replayed, countRuntimeRecords(t, fixture.store.Dir))
	}
}

func TestRuntimeRemediationFinishImportsLegacyBindingInTheSameAtomicRecord(t *testing.T) {
	fixture := newRuntimeRemediationFixtureWithBinding(t, true, false)
	probe, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), fixture.repo, "legacy-remediation-probe")
	if err != nil {
		t.Fatal(err)
	}
	legacyPath := bindingPath(probe, fixture.predecessorBinding.Change)
	legacyBefore, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	beforeRecords := countRuntimeRecords(t, fixture.store.Dir)
	completed, err := fixture.store.Finish(context.Background(), fixture.finishRequest("finish-legacy-remediation"))
	if err != nil {
		t.Fatal(err)
	}
	if !completed.Complete || completed.Binding == nil || completed.Binding.Lineage != fixture.successor.State.LineageID {
		t.Fatalf("legacy atomic remediation status = %#v", completed)
	}
	record, err := fixture.store.loadRecord(completed.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if record.Operation != runtimeOperationFinishRemediation || record.Binding == nil || record.Binding.LegacyImport == nil ||
		record.Binding.LegacyImport.Binding.Revision != fixture.predecessorBinding.Revision ||
		record.Binding.Current.Lineage != fixture.successor.State.LineageID || countRuntimeRecords(t, fixture.store.Dir) != beforeRecords+1 {
		t.Fatalf("legacy atomic remediation record = %#v records=%d", record, countRuntimeRecords(t, fixture.store.Dir))
	}
	legacyAfter, readErr := os.ReadFile(legacyPath)
	if readErr != nil || string(legacyAfter) != string(legacyBefore) {
		t.Fatalf("legacy remediation dual-wrote compatibility binding: %q err=%v", legacyAfter, readErr)
	}
}

func TestRuntimeRemediationFinishSupportsEngramWithoutOpenSpecRoot(t *testing.T) {
	fixture := newRuntimeEngramRemediationFixture(t)
	if _, err := os.Stat(filepath.Join(fixture.repo, "openspec")); !os.IsNotExist(err) {
		t.Fatalf("pure Engram fixture unexpectedly has an OpenSpec root: %v", err)
	}
	completed, err := fixture.store.Finish(context.Background(), fixture.finishRequest("finish-engram-remediation"))
	if err != nil {
		t.Fatal(err)
	}
	if !completed.Complete || completed.Binding == nil || completed.Binding.Lineage != fixture.successor.State.LineageID ||
		completed.BindingRevision == fixture.predecessorBinding.Revision {
		t.Fatalf("pure Engram remediation status = %#v", completed)
	}
	if _, err := os.Stat(filepath.Join(fixture.repo, "openspec")); !os.IsNotExist(err) {
		t.Fatalf("runtime remediation created or required an OpenSpec root: %v", err)
	}
}

func TestRuntimeRemediationRejectsSymlinkedOpenSpecInsteadOfEngramFallback(t *testing.T) {
	fixture := newRuntimeEngramRemediationFixture(t)
	outside := t.TempDir()
	seedReadyChange(t, outside, "engram-runtime", "- [x] linked\n")
	if err := os.Symlink(filepath.Join(outside, "openspec"), filepath.Join(fixture.repo, "openspec")); err != nil {
		t.Skipf("symlink fixture unavailable: %v", err)
	}
	before := countRuntimeRecords(t, fixture.store.Dir)
	_, err := fixture.store.Finish(context.Background(), fixture.finishRequest("reject-symlinked-openspec"))
	if err == nil || !strings.Contains(err.Error(), "outside repository") {
		t.Fatalf("symlinked OpenSpec successor error = %v", err)
	}
	assertRuntimeRemediationUnchanged(t, fixture, before)
}

func TestRuntimeRemediationFinishChargesButDoesNotSelectAnOverBudgetSuccessor(t *testing.T) {
	fixture := newRuntimeRemediationFixtureConfigured(t, true, true, 1, 2)
	beforeRecords := countRuntimeRecords(t, fixture.store.Dir)
	status, err := fixture.store.Finish(context.Background(), fixture.finishRequest("finish-over-budget-remediation"))
	if err != nil {
		t.Fatal(err)
	}
	if status.ActiveAttempt != nil || status.Complete || !status.DecisionRequired || status.NextAction != RuntimeActionReset {
		t.Fatalf("over-budget remediation status = %#v", status)
	}
	if status.Binding == nil || status.Binding.Revision != fixture.predecessorBinding.Revision ||
		status.Binding.Lineage == fixture.successor.State.LineageID {
		t.Fatalf("over-budget remediation selected successor binding = %#v", status.Binding)
	}
	last := status.Attempts[len(status.Attempts)-1]
	if !last.ChangedLineBudgetExceeded || last.Outcome != AttemptPassed || last.ChangedLines <= 1 ||
		last.RemediatesEvidenceRevision != fixture.failedEvidence || last.EvidenceRevision != runtimeTestHash('5') {
		t.Fatalf("over-budget charged attempt = %#v", last)
	}
	if countRuntimeRecords(t, fixture.store.Dir) != beforeRecords+1 {
		t.Fatalf("over-budget remediation records=%d want=%d", countRuntimeRecords(t, fixture.store.Dir), beforeRecords+1)
	}
	record, err := fixture.store.loadRecord(status.Revision)
	if err != nil || record.Operation != runtimeOperationFinishRemediation || record.Binding == nil || !record.Finish.ChangedLineBudgetExceeded {
		t.Fatalf("over-budget remediation record = %#v err=%v", record, err)
	}
}

func TestRuntimeRemediationFinishRejectsIncompleteStaleAndUnrelatedAuthority(t *testing.T) {
	t.Run("successor required for a changed bound passing attempt", func(t *testing.T) {
		fixture := newRuntimeRemediationFixture(t, true)
		request := fixture.finishRequest("finish-without-successor")
		request.ExpectedBindingRevision = ""
		request.SuccessorLineageID = ""
		request.RemediatesEvidenceRevision = ""
		before := countRuntimeRecords(t, fixture.store.Dir)
		_, err := fixture.store.Finish(context.Background(), request)
		if !errors.Is(err, ErrRuntimeRemediationSuccessorRequired) {
			t.Fatalf("missing successor error = %T %v", err, err)
		}
		assertRuntimeRemediationUnchanged(t, fixture, before)
	})

	t.Run("stale populated binding revision", func(t *testing.T) {
		fixture := newRuntimeRemediationFixture(t, true)
		request := fixture.finishRequest("finish-stale-binding")
		request.ExpectedBindingRevision = runtimeTestHash('9')
		before := countRuntimeRecords(t, fixture.store.Dir)
		_, err := fixture.store.Finish(context.Background(), request)
		var conflict *BindingRevisionConflictError
		if !errors.As(err, &conflict) || conflict.Current != fixture.predecessorBinding.Revision {
			t.Fatalf("stale populated binding error = %T %#v", err, err)
		}
		assertRuntimeRemediationUnchanged(t, fixture, before)
	})

	t.Run("failed evidence mismatch", func(t *testing.T) {
		fixture := newRuntimeRemediationFixture(t, true)
		request := fixture.finishRequest("finish-wrong-evidence")
		request.RemediatesEvidenceRevision = runtimeTestHash('8')
		before := countRuntimeRecords(t, fixture.store.Dir)
		_, err := fixture.store.Finish(context.Background(), request)
		if err == nil || !strings.Contains(err.Error(), "failed evidence revision") {
			t.Fatalf("mismatched failed evidence error = %v", err)
		}
		assertRuntimeRemediationUnchanged(t, fixture, before)
	})

	t.Run("unapproved recovery descendant", func(t *testing.T) {
		fixture := newRuntimeRemediationFixture(t, false)
		before := countRuntimeRecords(t, fixture.store.Dir)
		_, err := fixture.store.Finish(context.Background(), fixture.finishRequest("finish-unapproved-successor"))
		if err == nil || !strings.Contains(err.Error(), "not approved") {
			t.Fatalf("unapproved successor error = %v", err)
		}
		assertRuntimeRemediationUnchanged(t, fixture, before)
	})

	t.Run("approved authority outside recovery chain", func(t *testing.T) {
		fixture := newRuntimeRemediationFixture(t, true)
		unrelated := createApprovedRuntimeAuthority(t, fixture.repo, "unrelated-approved", 1)
		request := fixture.finishRequest("finish-unrelated-successor")
		request.SuccessorLineageID = unrelated.State.LineageID
		before := countRuntimeRecords(t, fixture.store.Dir)
		_, err := fixture.store.Finish(context.Background(), request)
		if err == nil || !strings.Contains(err.Error(), "recovery descendant") {
			t.Fatalf("unrelated successor error = %v", err)
		}
		assertRuntimeRemediationUnchanged(t, fixture, before)
	})

	t.Run("candidate changes during final authorization", func(t *testing.T) {
		fixture := newRuntimeRemediationFixture(t, true)
		before := countRuntimeRecords(t, fixture.store.Dir)
		originalHook := runtimeRemediationFinalAuthorizationHook
		runtimeRemediationFinalAuthorizationHook = func() {
			write(t, filepath.Join(fixture.changeRoot, "tasks.md"), "- [x] 1.1 Done\n# bounded remediation\n# authorization race\n")
		}
		t.Cleanup(func() { runtimeRemediationFinalAuthorizationHook = originalHook })
		_, err := fixture.store.Finish(context.Background(), fixture.finishRequest("finish-authorization-race"))
		runtimeRemediationFinalAuthorizationHook = originalHook
		if err == nil || !strings.Contains(err.Error(), "changed before native commit") {
			t.Fatalf("final authorization race error = %v", err)
		}
		assertRuntimeRemediationUnchanged(t, fixture, before)
	})
}

func TestRuntimeBoundFinishCompletesUnchangedCandidateWithoutReplacingBinding(t *testing.T) {
	fixture := newRuntimeUnchangedBindingFixture(t, "unchanged-bound")
	request := fixture.finishRequest("unchanged-finish")
	beforeRecords := countRuntimeRecords(t, fixture.store.Dir)
	completed, err := fixture.store.Finish(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !completed.Complete || completed.ActiveAttempt != nil || completed.NextAction != RuntimeActionComplete ||
		completed.Binding == nil || completed.Binding.Revision != fixture.binding.Revision ||
		completed.CumulativeChangedLines != 0 {
		t.Fatalf("unchanged bound completion = %#v", completed)
	}
	replayed, err := fixture.store.Finish(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Revision != completed.Revision || countRuntimeRecords(t, fixture.store.Dir) != beforeRecords+1 {
		t.Fatalf("unchanged exact replay = %#v records=%d", replayed, countRuntimeRecords(t, fixture.store.Dir))
	}
}

func TestRuntimeBoundUnchangedFinishPostHeadFailureIsReplayable(t *testing.T) {
	fixture := newRuntimeUnchangedBindingFixture(t, "unchanged-bound-fault")
	request := fixture.finishRequest("unchanged-finish-fault")
	beforeRecords := countRuntimeRecords(t, fixture.store.Dir)
	originalSync := runtimeSyncDirectory
	var fail atomic.Bool
	fail.Store(true)
	runtimeSyncDirectory = func(path string) error {
		if filepath.Clean(path) == filepath.Clean(fixture.store.Dir) && fail.CompareAndSwap(true, false) {
			return errors.New("simulated unchanged finish directory sync failure")
		}
		return originalSync(path)
	}
	t.Cleanup(func() { runtimeSyncDirectory = originalSync })

	_, err := fixture.store.Finish(context.Background(), request)
	runtimeSyncDirectory = originalSync
	var publication *RuntimePublicationError
	if !errors.As(err, &publication) || !publication.Committed {
		t.Fatalf("post-HEAD unchanged finish error = %T %v", err, err)
	}
	committed, statusErr := fixture.store.Status()
	if statusErr != nil || !committed.Complete || committed.Binding == nil ||
		committed.Binding.Revision != fixture.binding.Revision || committed.CumulativeChangedLines != 0 ||
		countRuntimeRecords(t, fixture.store.Dir) != beforeRecords+1 {
		t.Fatalf("post-HEAD unchanged finish status = %#v err=%v records=%d", committed, statusErr, countRuntimeRecords(t, fixture.store.Dir))
	}
	replayed, err := fixture.store.Finish(context.Background(), request)
	if err != nil || replayed.Revision != committed.Revision || countRuntimeRecords(t, fixture.store.Dir) != beforeRecords+1 {
		t.Fatalf("post-HEAD unchanged finish replay = %#v err=%v records=%d", replayed, err, countRuntimeRecords(t, fixture.store.Dir))
	}
}

func TestRuntimeRemediationFinishCrashWindowsRemainAtomicAndReplayable(t *testing.T) {
	t.Run("pre HEAD failure", func(t *testing.T) {
		fixture := newRuntimeRemediationFixture(t, true)
		request := fixture.finishRequest("finish-pre-head-fault")
		before := countRuntimeRecords(t, fixture.store.Dir)
		originalReplace := runtimeReplaceHead
		var fail atomic.Bool
		fail.Store(true)
		runtimeReplaceHead = func(source, destination string) error {
			if fail.CompareAndSwap(true, false) {
				return errors.New("simulated remediation HEAD failure")
			}
			return originalReplace(source, destination)
		}
		t.Cleanup(func() { runtimeReplaceHead = originalReplace })

		_, err := fixture.store.Finish(context.Background(), request)
		runtimeReplaceHead = originalReplace
		if err == nil || !strings.Contains(err.Error(), "remediation HEAD failure") {
			t.Fatalf("pre-HEAD remediation error = %v", err)
		}
		assertRuntimeRemediationUnchanged(t, fixture, before+1)

		completed, err := fixture.store.Finish(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		if !completed.Complete || completed.Binding == nil || completed.Binding.Lineage != fixture.successor.State.LineageID ||
			countRuntimeRecords(t, fixture.store.Dir) != before+1 {
			t.Fatalf("pre-HEAD exact retry = %#v records=%d", completed, countRuntimeRecords(t, fixture.store.Dir))
		}
	})

	t.Run("post HEAD directory sync failure", func(t *testing.T) {
		fixture := newRuntimeRemediationFixture(t, true)
		request := fixture.finishRequest("finish-post-head-fault")
		before := countRuntimeRecords(t, fixture.store.Dir)
		originalSync := runtimeSyncDirectory
		var fail atomic.Bool
		fail.Store(true)
		runtimeSyncDirectory = func(path string) error {
			if filepath.Clean(path) == filepath.Clean(fixture.store.Dir) && fail.CompareAndSwap(true, false) {
				return errors.New("simulated remediation directory sync failure")
			}
			return originalSync(path)
		}
		t.Cleanup(func() { runtimeSyncDirectory = originalSync })

		_, err := fixture.store.Finish(context.Background(), request)
		runtimeSyncDirectory = originalSync
		var publication *RuntimePublicationError
		if !errors.As(err, &publication) || !publication.Committed {
			t.Fatalf("post-HEAD remediation error = %T %v", err, err)
		}
		committed, statusErr := fixture.store.Status()
		if statusErr != nil || !committed.Complete || committed.Binding == nil ||
			committed.Binding.Lineage != fixture.successor.State.LineageID || countRuntimeRecords(t, fixture.store.Dir) != before+1 {
			t.Fatalf("post-HEAD committed status = %#v err=%v", committed, statusErr)
		}
		replayed, err := fixture.store.Finish(context.Background(), request)
		if err != nil || replayed.Revision != committed.Revision || countRuntimeRecords(t, fixture.store.Dir) != before+1 {
			t.Fatalf("post-HEAD exact replay = %#v err=%v records=%d", replayed, err, countRuntimeRecords(t, fixture.store.Dir))
		}
	})
}

func TestRuntimeRemediationFinishCASAllowsOnlyOneAtomicSuccessor(t *testing.T) {
	fixture := newRuntimeRemediationFixture(t, true)
	before := countRuntimeRecords(t, fixture.store.Dir)
	const writers = 16
	start := make(chan struct{})
	var successes atomic.Int32
	var unexpectedMu sync.Mutex
	var unexpected []error
	var wait sync.WaitGroup
	for index := 0; index < writers; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			request := fixture.finishRequest("finish-concurrent-" + string(rune('a'+index)))
			_, err := fixture.store.Finish(context.Background(), request)
			if err == nil {
				successes.Add(1)
				return
			}
			if !errors.Is(err, ErrRuntimeRevisionConflict) && !errors.Is(err, ErrRuntimeConcurrentUpdate) &&
				!errors.Is(err, ErrRuntimeNoActiveAttempt) {
				unexpectedMu.Lock()
				unexpected = append(unexpected, err)
				unexpectedMu.Unlock()
			}
		}(index)
	}
	close(start)
	wait.Wait()
	if successes.Load() != 1 || len(unexpected) != 0 {
		t.Fatalf("concurrent remediation writers: successes=%d unexpected=%v", successes.Load(), unexpected)
	}
	status, err := fixture.store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if !status.Complete || status.Binding == nil || status.Binding.Lineage != fixture.successor.State.LineageID ||
		countRuntimeRecords(t, fixture.store.Dir) != before+1 {
		t.Fatalf("concurrent atomic remediation status = %#v records=%d", status, countRuntimeRecords(t, fixture.store.Dir))
	}
}

type runtimeRemediationFixture struct {
	repo               string
	changeRoot         string
	store              RuntimeStore
	predecessorBinding ReviewBinding
	failedEvidence     string
	active             RuntimeStatus
	successor          reviewtransaction.CompactRecord
}

type runtimeUnchangedBindingFixture struct {
	store   RuntimeStore
	binding ReviewBinding
	active  RuntimeStatus
}

func newRuntimeUnchangedBindingFixture(t *testing.T, change string) runtimeUnchangedBindingFixture {
	t.Helper()
	repo := t.TempDir()
	changeRoot := seedReadyChange(t, repo, change, "- [x] 1.1 Done\n")
	lineage := change + "-approved"
	writeApprovedCompactAuthorityForChange(t, repo, changeRoot, lineage)
	binding, err := BindApprovedReview(context.Background(), repo, change, lineage, "")
	if err != nil {
		t.Fatal(err)
	}
	store := mustRuntimeStore(t, repo, change)
	bound, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	active, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: bound.Revision, RequestID: change + "-begin", WorkUnit: change,
		EvidenceGoal: "verify unchanged approved candidate", MaxAttempts: 2, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	return runtimeUnchangedBindingFixture{store: store, binding: binding, active: active}
}

func (fixture runtimeUnchangedBindingFixture) finishRequest(requestID string) FinishAttemptRequest {
	return FinishAttemptRequest{
		ExpectedRevision: fixture.active.Revision, RequestID: requestID, Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('c'), Diagnosis: "approved candidate passed unchanged verification",
		HarnessDisposition: HarnessReused, CleanupEvidence: "unchanged verification cleanup completed",
		ProcessEvidence: "unchanged verification process scan found no descendants",
	}
}

func newRuntimeRemediationFixture(t *testing.T, approveSuccessor bool) runtimeRemediationFixture {
	return newRuntimeRemediationFixtureConfigured(t, approveSuccessor, true, 40, 1)
}

func newRuntimeRemediationFixtureWithBinding(t *testing.T, approveSuccessor, nativeBinding bool) runtimeRemediationFixture {
	return newRuntimeRemediationFixtureConfigured(t, approveSuccessor, nativeBinding, 40, 1)
}

func newRuntimeEngramRemediationFixture(t *testing.T) runtimeRemediationFixture {
	t.Helper()
	repo := initRuntimeLedgerRepo(t)
	predecessor := createApprovedRuntimeAuthority(t, repo, "engram-runtime-predecessor", 1)
	predecessorBinding := runtimeBindingFromCompactRecord(t, repo, "engram-runtime", predecessor)
	store := mustRuntimeStore(t, repo, "engram-runtime")
	bound, err := store.bindPreparedReview(context.Background(), BindReviewRequest{
		ExpectedBindingRevision: "", RequestID: "bind-engram-runtime", LineageID: predecessorBinding.Lineage,
	}, func() (ReviewBinding, error) { return predecessorBinding, nil })
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: bound.Revision, RequestID: "engram-remediation-begin-1", WorkUnit: "engram-runtime",
		EvidenceGoal: "repair failed Engram verification evidence", MaxAttempts: 3, MaxChangedLines: 40,
	})
	if err != nil {
		t.Fatal(err)
	}
	failedEvidence := runtimeTestHash('d')
	failed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: first.Revision, RequestID: "engram-remediation-finish-1", Outcome: AttemptFailed,
		EvidenceRevision: failedEvidence, Diagnosis: "pure Engram verification failed before remediation",
		HarnessDisposition: HarnessReused, CleanupEvidence: "Engram predecessor cleanup completed",
		ProcessEvidence: "Engram predecessor process scan found no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	active, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: failed.Revision, RequestID: "engram-remediation-begin-2", WorkUnit: "engram-runtime",
		EvidenceGoal: "repair failed Engram verification evidence", MaxAttempts: 3, MaxChangedLines: 40,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendRuntimeLedgerFile(t, repo, "bounded Engram remediation\n")
	successor := createRuntimeRecoverySuccessor(t, repo, predecessor.State.LineageID, "engram-runtime-successor", true)
	return runtimeRemediationFixture{
		repo: repo, store: store, predecessorBinding: predecessorBinding,
		failedEvidence: failedEvidence, active: active, successor: successor,
	}
}

func runtimeBindingFromCompactRecord(t *testing.T, repo, change string, record reviewtransaction.CompactRecord) ReviewBinding {
	t.Helper()
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, record.State.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(store.ReceiptPath())
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := record.State.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	evaluation := reviewtransaction.EvaluateCompactGate(context.Background(), repo, receipt, reviewtransaction.NativeGateRequestInput{
		Gate: reviewtransaction.GatePostApply, LineageID: record.State.LineageID,
	})
	if evaluation.Result != reviewtransaction.GateAllow {
		t.Fatalf("compact binding gate = %#v, want allow", evaluation)
	}
	binding := ReviewBinding{
		Schema: reviewBindingSchema, Change: change, Lineage: record.State.LineageID,
		AuthorityRevision: record.Revision, ReceiptHash: bindingHash(payload), GateContext: evaluation.Context,
	}
	binding.Revision = bindingDigest(binding)
	return binding
}

func newRuntimeRemediationFixtureConfigured(t *testing.T, approveSuccessor, nativeBinding bool, maxChangedLines, remediationLines int) runtimeRemediationFixture {
	t.Helper()
	repo := t.TempDir()
	changeRoot := seedReadyChange(t, repo, "runtime-remediation", "- [x] 1.1 Done\n")
	writeApprovedCompactAuthorityForChange(t, repo, changeRoot, "runtime-predecessor")
	var predecessorBinding ReviewBinding
	var err error
	if nativeBinding {
		predecessorBinding, err = BindApprovedReview(context.Background(), repo, "runtime-remediation", "runtime-predecessor", "")
	} else {
		predecessorBinding, err = prepareApprovedReviewBinding(context.Background(), repo, repo, "runtime-remediation", "runtime-predecessor")
		if err == nil {
			writeRuntimeLegacyBinding(t, repo, predecessorBinding)
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	store := mustRuntimeStore(t, repo, "runtime-remediation")
	bound, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: bound.Revision, RequestID: "remediation-begin-1", WorkUnit: "runtime-remediation",
		EvidenceGoal: "repair failed verification evidence", MaxAttempts: 3, MaxChangedLines: maxChangedLines,
	})
	if err != nil {
		t.Fatal(err)
	}
	failedEvidence := runtimeTestHash('4')
	failed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: first.Revision, RequestID: "remediation-finish-1", Outcome: AttemptFailed,
		EvidenceRevision: failedEvidence, Diagnosis: "failed verification reproduced before bounded remediation",
		HarnessDisposition: HarnessReused, CleanupEvidence: "predecessor cleanup completed",
		ProcessEvidence: "predecessor process scan found no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	active, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: failed.Revision, RequestID: "remediation-begin-2", WorkUnit: "runtime-remediation",
		EvidenceGoal: "repair failed verification evidence", MaxAttempts: 3, MaxChangedLines: maxChangedLines,
	})
	if err != nil {
		t.Fatal(err)
	}
	content := "- [x] 1.1 Done\n"
	for index := 0; index < remediationLines; index++ {
		content += "# bounded remediation " + string(rune('a'+index)) + "\n"
	}
	write(t, filepath.Join(changeRoot, "tasks.md"), content)
	successor := createRuntimeRecoverySuccessor(t, repo, "runtime-predecessor", "runtime-successor", approveSuccessor)
	return runtimeRemediationFixture{
		repo: repo, changeRoot: changeRoot, store: store, predecessorBinding: predecessorBinding,
		failedEvidence: failedEvidence, active: active, successor: successor,
	}
}

func (fixture runtimeRemediationFixture) finishRequest(requestID string) FinishAttemptRequest {
	return FinishAttemptRequest{
		ExpectedRevision: fixture.active.Revision, RequestID: requestID, Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('5'), Diagnosis: "bounded remediation passed successor verification",
		HarnessDisposition: HarnessReused, CleanupEvidence: "remediation cleanup completed",
		ProcessEvidence:            "remediation process scan found no descendants",
		ExpectedBindingRevision:    fixture.predecessorBinding.Revision,
		SuccessorLineageID:         fixture.successor.State.LineageID,
		RemediatesEvidenceRevision: fixture.failedEvidence,
	}
}

func assertRuntimeRemediationUnchanged(t *testing.T, fixture runtimeRemediationFixture, wantRecords int) {
	t.Helper()
	status, err := fixture.store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if status.Revision != fixture.active.Revision || status.ActiveAttempt == nil || status.Complete ||
		status.Binding == nil || status.Binding.Revision != fixture.predecessorBinding.Revision ||
		countRuntimeRecords(t, fixture.store.Dir) != wantRecords {
		t.Fatalf("failed remediation mutated authority: status=%#v records=%d want=%d", status, countRuntimeRecords(t, fixture.store.Dir), wantRecords)
	}
}

func createRuntimeRecoverySuccessor(t *testing.T, repo, predecessorLineage, successorLineage string, approve bool) reviewtransaction.CompactRecord {
	t.Helper()
	predecessorStore, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, predecessorLineage)
	if err != nil {
		t.Fatal(err)
	}
	predecessor, err := predecessorStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	successor := newRuntimeCompactState(t, repo, successorLineage, predecessor.State.Generation+1)
	recovered, err := reviewtransaction.RecoverCompactAuthority(context.Background(), repo, reviewtransaction.CompactRecoveryRequest{
		PredecessorLineageID: predecessorLineage, ExpectedPredecessorRevision: predecessor.Revision,
		Successor: successor, Disposition: reviewtransaction.RecoveryScopeChanged,
		Reason: "bounded SDD remediation changed the reviewed candidate", Actor: "runtime-ledger-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !approve {
		return recovered
	}
	return completeRuntimeCompactAuthority(t, repo, recovered)
}

func createApprovedRuntimeAuthority(t *testing.T, repo, lineage string, generation int) reviewtransaction.CompactRecord {
	t.Helper()
	state := newRuntimeCompactState(t, repo, lineage, generation)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.Replace("", "review/start", state)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil || record.Revision != revision {
		t.Fatalf("load independent compact authority: record=%#v err=%v", record, err)
	}
	return completeRuntimeCompactAuthority(t, repo, record)
}

func newRuntimeCompactState(t *testing.T, repo, lineage string, generation int) reviewtransaction.CompactState {
	t.Helper()
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	snapshot, err := builder.Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	risk, lines, err := builder.ClassifySnapshotRisk(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	lenses := []string{}
	if risk == reviewtransaction.RiskMedium {
		lenses = []string{reviewtransaction.LensReliability}
	} else if risk == reviewtransaction.RiskHigh {
		lenses = []string{reviewtransaction.LensRisk, reviewtransaction.LensResilience, reviewtransaction.LensReadability, reviewtransaction.LensReliability}
	}
	state, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: lineage, Mode: reviewtransaction.ModeOrdinaryBounded, Generation: generation,
		Snapshot: snapshot, PolicyHash: runtimeTestHash('6'), RiskLevel: risk,
		SelectedLenses: lenses, OriginalChangedLines: &lines,
	})
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func completeRuntimeCompactAuthority(t *testing.T, repo string, record reviewtransaction.CompactRecord) reviewtransaction.CompactRecord {
	t.Helper()
	state := record.State
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	results := make([]reviewtransaction.LensResult, len(state.SelectedLenses))
	for index, lens := range state.SelectedLenses {
		results[index] = reviewtransaction.LensResult{Lens: lens, Findings: []reviewtransaction.Finding{}, Evidence: []string{"review complete"}}
	}
	if err := state.CompleteReview(reviewtransaction.CompactReviewInput{
		LensResults: results, Classifications: []reviewtransaction.FindingEvidence{}, RefuterOutcomes: []reviewtransaction.EvidenceResult{},
	}); err != nil {
		t.Fatal(err)
	}
	revision, err := store.Replace(record.Revision, "review/complete-review", state)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.CompleteVerification([]byte("runtime remediation verification passed\n"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(revision, "review/complete-verification", state); err != nil {
		t.Fatal(err)
	}
	receipt, err := state.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := reviewtransaction.WriteCompactReceiptAtomic(store.ReceiptPath(), receipt); err != nil {
		t.Fatal(err)
	}
	completed, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	return completed
}
