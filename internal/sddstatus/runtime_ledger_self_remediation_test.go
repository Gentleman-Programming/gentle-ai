package sddstatus

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestRuntimeSelfRemediationFinishBindsCorrectedApprovedAuthority reproduces
// the community-reported lifecycle deadlock (decode2, PR #1801): a corrected
// candidate passes every check and holds an approved, content-bound review on
// the SAME lineage that the runtime binding names, yet the passing bound
// finish used to demand a distinct recovery successor that can never legally
// exist — recover refuses an unchanged approved scope and a same-lineage
// successor, and invalidate correctly refuses healthy approved authority.
// The bound finish must accept the approved self-successor instead.
func TestRuntimeSelfRemediationFinishBindsCorrectedApprovedAuthority(t *testing.T) {
	fixture := newRuntimeSelfRemediationFixture(t)
	beforeRecords := countRuntimeRecords(t, fixture.store.Dir)
	request := fixture.finishRequest("finish-self-remediation")

	completed, err := fixture.store.Finish(context.Background(), request)
	if err != nil {
		t.Fatalf("approved self-successor finish is deadlocked: %v", err)
	}
	if completed.ActiveAttempt != nil || !completed.Complete || completed.NextAction != RuntimeActionComplete {
		t.Fatalf("completed self-remediation status = %#v", completed)
	}
	if completed.Binding == nil || completed.Binding.Lineage != fixture.lineage ||
		completed.Binding.Revision != fixture.binding.Revision {
		t.Fatalf("approved self-successor binding = %#v, want retained %s", completed.Binding, fixture.binding.Revision)
	}
	if completed.EvidenceRevision != request.EvidenceRevision {
		t.Fatalf("self-remediation evidence = %#v", completed)
	}
	last := completed.Attempts[len(completed.Attempts)-1]
	if last.Outcome != AttemptPassed || last.RemediatesEvidenceRevision != fixture.failedEvidence ||
		last.EvidenceRevision != request.EvidenceRevision || last.ChangedLines == 0 {
		t.Fatalf("completed self-remediation attempt = %#v", last)
	}
	// No new budget: the finish charges the existing objective exactly once.
	if completed.CumulativeAttempts != 2 || countRuntimeRecords(t, fixture.store.Dir) != beforeRecords+1 {
		t.Fatalf("self-remediation budget: attempts=%d records=%d want attempts=2 records=%d",
			completed.CumulativeAttempts, countRuntimeRecords(t, fixture.store.Dir), beforeRecords+1)
	}
	record, err := fixture.store.loadRecord(completed.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if record.Operation != runtimeOperationFinishRemediation || record.Finish == nil || record.Binding == nil ||
		record.Binding.ExpectedRevision != fixture.binding.Revision || record.Binding.Current.Lineage != fixture.lineage {
		t.Fatalf("self-remediation record = %#v", record)
	}

	// Exact replay converges idempotently without another record or mutation.
	replayed, err := fixture.store.Finish(context.Background(), request)
	if err != nil {
		t.Fatalf("exact self-remediation replay: %v", err)
	}
	if replayed.Revision != completed.Revision || countRuntimeRecords(t, fixture.store.Dir) != beforeRecords+1 {
		t.Fatalf("exact self-remediation replay = %#v records=%d", replayed, countRuntimeRecords(t, fixture.store.Dir))
	}
}

// TestRuntimeSelfRemediationTriangleGuardRailsHold pins the recovery legs of
// the reported triangle as CORRECT refusals: recovery cannot mint a
// successor from a healthy approved authority whose scope did not change,
// nor from a same-lineage successor, nor without an actually-invalidated
// predecessor. The self-successor finish is the only legal exit.
//
// A third leg -- "invalidate still refuses healthy approved authority" --
// was pinned here through InvalidateApprovedCompactAuthority before Wave 5
// (Gate Cutover) Slice 7 deleted it (design decision 2: invalidated is now
// a derived verdict, never a write this verb performs, so there is no
// write path left to refuse). The equivalent invariant -- a healthy
// approved candidate never reads as invalidated -- is exactly what
// TestReceiptFilePersistsAfterDerivedInvalidation_AllFiveGates (internal/cli)
// and the exact-candidate "allow" cells of the gate-boundary matrix
// (internal/reviewtransaction) already prove for every gate; nothing about
// self-remediation is specific to it.
func TestRuntimeSelfRemediationTriangleGuardRailsHold(t *testing.T) {
	fixture := newRuntimeSelfRemediationFixture(t)
	ctx := context.Background()
	predecessorStore, err := reviewtransaction.CompactAuthoritativeStore(ctx, fixture.repo, fixture.lineage)
	if err != nil {
		t.Fatal(err)
	}
	predecessor, err := predecessorStore.Load()
	if err != nil {
		t.Fatal(err)
	}

	t.Run("recover rejects a same-lineage successor", func(t *testing.T) {
		successor := newRuntimeCompactState(t, fixture.repo, fixture.lineage, predecessor.State.Generation+1)
		_, err := reviewtransaction.RecoverCompactAuthority(ctx, fixture.repo, reviewtransaction.CompactRecoveryRequest{
			PredecessorLineageID: fixture.lineage, ExpectedPredecessorRevision: predecessor.Revision,
			Successor: successor, Disposition: reviewtransaction.RecoveryScopeChanged,
			Reason: "self-successor probe", Actor: "runtime-ledger-test",
		})
		if err == nil || !strings.Contains(err.Error(), "distinct successor lineage") {
			t.Fatalf("same-lineage recover error = %v", err)
		}
	})

	t.Run("recover rejects an unchanged approved scope", func(t *testing.T) {
		successor := newRuntimeCompactState(t, fixture.repo, "runtime-self-distinct", predecessor.State.Generation+1)
		_, err := reviewtransaction.RecoverCompactAuthority(ctx, fixture.repo, reviewtransaction.CompactRecoveryRequest{
			PredecessorLineageID: fixture.lineage, ExpectedPredecessorRevision: predecessor.Revision,
			Successor: successor, Disposition: reviewtransaction.RecoveryScopeChanged,
			Reason: "unchanged scope probe", Actor: "runtime-ledger-test",
		})
		if err == nil || !strings.Contains(err.Error(), "approved predecessor scope has not changed") {
			t.Fatalf("unchanged scope recover error = %v", err)
		}
	})

	t.Run("recover still requires an invalidated predecessor", func(t *testing.T) {
		successor := newRuntimeCompactState(t, fixture.repo, "runtime-self-invalidated", predecessor.State.Generation+1)
		_, err := reviewtransaction.RecoverCompactAuthority(ctx, fixture.repo, reviewtransaction.CompactRecoveryRequest{
			PredecessorLineageID: fixture.lineage, ExpectedPredecessorRevision: predecessor.Revision,
			Successor: successor, Disposition: reviewtransaction.RecoveryInvalidated,
			Reason: "invalidated disposition probe", Actor: "runtime-ledger-test",
		})
		if err == nil || !strings.Contains(err.Error(), "recovery requires an invalidated predecessor") {
			t.Fatalf("invalidated disposition recover error = %v", err)
		}
	})
}

func TestRuntimeSelfRemediationFinishRejectsUnprovenSelfSuccessors(t *testing.T) {
	t.Run("corrected evidence must differ from the failed evidence", func(t *testing.T) {
		fixture := newRuntimeSelfRemediationFixture(t)
		request := fixture.finishRequest("finish-self-identical-evidence")
		request.EvidenceRevision = fixture.failedEvidence
		before := countRuntimeRecords(t, fixture.store.Dir)
		_, err := fixture.store.Finish(context.Background(), request)
		if err == nil || !strings.Contains(err.Error(), "distinct corrected evidence") {
			t.Fatalf("identical evidence self-remediation error = %v", err)
		}
		assertRuntimeSelfRemediationUnchanged(t, fixture, before)
	})

	t.Run("receipt must remain content-bound to the approved authority", func(t *testing.T) {
		fixture := newRuntimeSelfRemediationFixture(t)
		store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), fixture.repo, fixture.lineage)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(store.ReceiptPath()); err != nil {
			t.Fatal(err)
		}
		before := countRuntimeRecords(t, fixture.store.Dir)
		_, err = fixture.store.Finish(context.Background(), fixture.finishRequest("finish-self-missing-receipt"))
		if err == nil {
			t.Fatal("self-remediation accepted an approved authority without its receipt")
		}
		assertRuntimeSelfRemediationUnchanged(t, fixture, before)
	})

	t.Run("bound lineage must remain the compact recovery leaf", func(t *testing.T) {
		fixture := newRuntimeSelfRemediationFixture(t)
		// A later scope change hands the lineage a true recovery successor; the
		// stale self-successor must then be refused at the leaf boundary.
		write(t, filepath.Join(fixture.changeRoot, "design.md"), "# Design\n# post-remediation scope change\n")
		createRuntimeRecoverySuccessor(t, fixture.repo, fixture.lineage, "runtime-self-leaf-successor", false)
		err := validateRuntimeRemediationSelfSuccessor(context.Background(), fixture.repo, fixture.binding, fixture.binding)
		if err == nil || !strings.Contains(err.Error(), "compact recovery leaf") {
			t.Fatalf("superseded self-successor error = %v", err)
		}
	})
}

type runtimeSelfRemediationFixture struct {
	repo           string
	changeRoot     string
	lineage        string
	store          RuntimeStore
	binding        ReviewBinding
	failedEvidence string
	active         RuntimeStatus
	postBind       RuntimeStatus
}

// newRuntimeSelfRemediationFixture drives decode2's exact timeline: failed
// evidence is recorded, the bounded correction lands on the same lineage, and
// that lineage holds the approved, content-bound, gate-allow review of the
// corrected candidate before the passing bound finish.
func newRuntimeSelfRemediationFixture(t *testing.T) runtimeSelfRemediationFixture {
	t.Helper()
	repo := t.TempDir()
	changeRoot := seedReadyChange(t, repo, "runtime-self-remediation", "- [x] 1.1 Done\n")
	runSDDStatusGit(t, repo, "init", "-q")
	runSDDStatusGit(t, repo, "config", "user.email", "status@example.com")
	runSDDStatusGit(t, repo, "config", "user.name", "Status Test")
	runSDDStatusGit(t, repo, "add", ".")
	runSDDStatusGit(t, repo, "commit", "-qm", "base")

	store := mustRuntimeStore(t, repo, "runtime-self-remediation")
	initial, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: initial.Revision, RequestID: "self-remediation-begin-1", WorkUnit: "runtime-self-remediation",
		EvidenceGoal: "repair failed verification evidence", MaxAttempts: 3, MaxChangedLines: 40,
	})
	if err != nil {
		t.Fatal(err)
	}
	failedEvidence := runtimeTestHash('4')
	failed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: first.Revision, RequestID: "self-remediation-finish-1", Outcome: AttemptFailed,
		EvidenceRevision: failedEvidence, Diagnosis: "failed verification reproduced before bounded self remediation",
		HarnessDisposition: HarnessReused, CleanupEvidence: "predecessor cleanup completed",
		ProcessEvidence: "predecessor process scan found no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	active, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: failed.Revision, RequestID: "self-remediation-begin-2", WorkUnit: "runtime-self-remediation",
		EvidenceGoal: "repair failed verification evidence", MaxAttempts: 3, MaxChangedLines: 40,
	})
	if err != nil {
		t.Fatal(err)
	}
	// The bounded correction lands during the attempt, so the finish candidate
	// tree differs from the begin candidate tree.
	write(t, filepath.Join(changeRoot, "tasks.md"), "- [x] 1.1 Done\n# bounded self remediation\n")
	// The same lineage reviews and approves the corrected candidate.
	lineage := "runtime-self-lineage"
	createApprovedRuntimeAuthority(t, repo, lineage, 1)
	binding, err := BindApprovedReview(context.Background(), repo, "runtime-self-remediation", lineage, "")
	if err != nil {
		t.Fatal(err)
	}
	postBind, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if postBind.Binding == nil || postBind.Binding.Revision != binding.Revision || postBind.EvidenceRevision != failedEvidence {
		t.Fatalf("self-remediation fixture status = %#v", postBind)
	}
	return runtimeSelfRemediationFixture{
		repo: repo, changeRoot: changeRoot, lineage: lineage, store: store,
		binding: binding, failedEvidence: failedEvidence, active: active, postBind: postBind,
	}
}

func (fixture runtimeSelfRemediationFixture) finishRequest(requestID string) FinishAttemptRequest {
	return FinishAttemptRequest{
		ExpectedRevision: fixture.postBind.Revision, RequestID: requestID, Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('7'), Diagnosis: "bounded self remediation passed corrected verification",
		HarnessDisposition: HarnessReused, CleanupEvidence: "self remediation cleanup completed",
		ProcessEvidence:            "self remediation process scan found no descendants",
		ExpectedBindingRevision:    fixture.binding.Revision,
		SuccessorLineageID:         fixture.lineage,
		RemediatesEvidenceRevision: fixture.failedEvidence,
	}
}

func assertRuntimeSelfRemediationUnchanged(t *testing.T, fixture runtimeSelfRemediationFixture, wantRecords int) {
	t.Helper()
	status, err := fixture.store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if status.Revision != fixture.postBind.Revision || status.ActiveAttempt == nil || status.Complete ||
		status.Binding == nil || status.Binding.Revision != fixture.binding.Revision ||
		countRuntimeRecords(t, fixture.store.Dir) != wantRecords {
		t.Fatalf("failed self-remediation mutated authority: status=%#v records=%d want=%d",
			status, countRuntimeRecords(t, fixture.store.Dir), wantRecords)
	}
}
