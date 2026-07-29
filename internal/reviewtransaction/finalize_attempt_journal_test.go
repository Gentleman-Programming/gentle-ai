package reviewtransaction

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
)

func TestFinalizeAttemptJournalBindsExactRequestAndPreservesLegacyAbsence(t *testing.T) {
	repo := initSnapshotRepo(t)
	state := newCompactTestState(t, repo, "finalize-attempt-journal")
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.Replace("", "review/start", state)
	if err != nil {
		t.Fatal(err)
	}
	if pending, err := store.PendingFinalizeAttempt(); err != nil || pending != nil {
		t.Fatalf("legacy missing journal = %#v, %v", pending, err)
	}
	request := finalizeAttemptTestRequest(state.LineageID, revision, "evidence-one")
	first, replay, err := store.BeginFinalizeAttempt(context.Background(), request)
	if err != nil || replay || first.Request.RequestDigest != request.RequestDigest {
		t.Fatalf("begin attempt = %#v, replay %v, err %v", first, replay, err)
	}
	if err := store.RecordFinalizeAttemptTransition(request.RequestDigest, "review/complete-review", revision); err != nil {
		t.Fatal(err)
	}
	second, replay, err := store.BeginFinalizeAttempt(context.Background(), request)
	if err != nil || !replay || len(second.Transitions) != 1 {
		t.Fatalf("exact replay = %#v, replay %v, err %v", second, replay, err)
	}
	changed := finalizeAttemptTestRequest(state.LineageID, revision, "evidence-two")
	if _, _, err := store.BeginFinalizeAttempt(context.Background(), changed); !errors.As(err, new(*FinalizeAttemptReplayMismatchError)) {
		t.Fatalf("changed request error = %v", err)
	}
	changedRevision := request
	changedRevision.ExpectedRevision = FinalizeAttemptValueDigest("revision", "changed")
	changedRevision.RequestDigest = FinalizeAttemptRequestDigest(changedRevision)
	if _, _, err := store.ReconcileFinalizeAttempt(context.Background(), changedRevision); !errors.As(err, new(*FinalizeAttemptReplayMismatchError)) {
		t.Fatalf("changed revision error = %v", err)
	}
}

func TestFinalizeAttemptJournalAcceptsPostRenameAmbiguityOnlyWhenExactBytesPersisted(t *testing.T) {
	repo := initSnapshotRepo(t)
	state := newCompactTestState(t, repo, "finalize-post-rename")
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.Replace("", "review/start", state)
	if err != nil {
		t.Fatal(err)
	}
	original := writeFinalizeAttemptAtomic
	writeFinalizeAttemptAtomic = func(path string, payload []byte, mode os.FileMode) error {
		if err := writeAtomic(path, payload, mode); err != nil {
			return err
		}
		return fmt.Errorf("injected post-rename interruption")
	}
	t.Cleanup(func() { writeFinalizeAttemptAtomic = original })
	request := finalizeAttemptTestRequest(state.LineageID, revision, "evidence")
	if _, replay, err := store.BeginFinalizeAttempt(context.Background(), request); err != nil || replay {
		t.Fatalf("post-rename reconciliation = replay %v, err %v", replay, err)
	}
}

func TestFinalizeAttemptPendingStatusRequiresExplicitReconciliationWithoutLockMutation(t *testing.T) {
	repo := initSnapshotRepo(t)
	state := newCompactTestState(t, repo, "finalize-status")
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.Replace("", "review/start", state)
	if err != nil {
		t.Fatal(err)
	}
	request := finalizeAttemptTestRequest(state.LineageID, revision, "evidence")
	request.CandidateDigest = FinalizeAttemptValueDigest("candidate", state.CurrentSnapshot)
	request.RequestDigest = FinalizeAttemptRequestDigest(request)
	if _, _, err := store.BeginFinalizeAttempt(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	status, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{Target: Target{Kind: TargetCurrentChanges, Projection: state.InitialSnapshot.Projection, IntendedUntracked: []string{}}})
	if err != nil || status.Action != TargetStatusActionReconcileFinalize || status.Replayability != ReplayabilityStatusRequired {
		t.Fatalf("pending status = %#v, %v", status, err)
	}
}

func TestFinalizeAttemptJournalResumesOnlyTheContentBoundRequestAfterPlannedCommit(t *testing.T) {
	repo := initSnapshotRepo(t)
	state := newCompactTestState(t, repo, "finalize-planned-commit")
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := store.Replace("", "review/start", state)
	if err != nil {
		t.Fatal(err)
	}
	request := finalizeAttemptTestRequest(state.LineageID, entry, "evidence")
	attempt, replay, err := store.BeginFinalizeAttempt(context.Background(), request)
	if err != nil || replay {
		t.Fatalf("begin = %#v, replay %v, err %v", attempt, replay, err)
	}
	next := state
	if err := next.CompleteReview(CompactReviewInput{LensResults: []LensResult{}, Classifications: []FindingEvidence{}, RefuterOutcomes: []EvidenceResult{}}); err != nil {
		t.Fatal(err)
	}
	planned, err := store.PlanFinalizeAttemptTransition(request.RequestDigest, "review/complete-review", entry, next)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := store.Replace(entry, "review/complete-review", next)
	if err != nil {
		t.Fatal(err)
	}
	if committed != planned {
		t.Fatalf("committed revision = %q, want planned %q", committed, planned)
	}
	resumed, replay, err := store.ReconcileFinalizeAttempt(context.Background(), request)
	if err != nil || !replay || resumed.Request.RequestDigest != request.RequestDigest || len(resumed.Transitions) != 1 {
		t.Fatalf("exact replay after commit = %#v, replay %v, err %v", resumed, replay, err)
	}
	for name, mutate := range map[string]func(*FinalizeAttemptRequest){
		"reviewer results": func(value *FinalizeAttemptRequest) {
			value.ReviewerResultsDigest = FinalizeAttemptValueDigest("reviewer-results", []string{"changed"})
		},
		"validation": func(value *FinalizeAttemptRequest) {
			value.ValidationDigest = FinalizeAttemptValueDigest("validation", "changed")
		},
		"candidate": func(value *FinalizeAttemptRequest) {
			value.CandidateDigest = FinalizeAttemptValueDigest("candidate", "changed")
		},
		"unrelated revision": func(value *FinalizeAttemptRequest) {
			value.ExpectedRevision = FinalizeAttemptValueDigest("revision", "unrelated")
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := request
			mutate(&changed)
			changed.RequestDigest = FinalizeAttemptRequestDigest(changed)
			if _, _, err := store.ReconcileFinalizeAttempt(context.Background(), changed); !errors.As(err, new(*FinalizeAttemptReplayMismatchError)) {
				t.Fatalf("changed %s reconciled: %v", name, err)
			}
		})
	}
}

func TestFinalizeAttemptJournalPublicationAndCompletionMarkersRemainRetryableAfterWriteFailure(t *testing.T) {
	repo := initSnapshotRepo(t)
	state := newCompactTestState(t, repo, "finalize-marker-retry")
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.Replace("", "review/start", state)
	if err != nil {
		t.Fatal(err)
	}
	request := finalizeAttemptTestRequest(state.LineageID, revision, "evidence")
	if _, _, err := store.BeginFinalizeAttempt(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	original := writeFinalizeAttemptAtomic
	writeFinalizeAttemptAtomic = func(string, []byte, os.FileMode) error { return errors.New("injected marker write failure") }
	t.Cleanup(func() { writeFinalizeAttemptAtomic = original })
	if err := store.MarkFinalizeAttemptReceiptPublished(request.RequestDigest); err == nil {
		t.Fatal("publication marker write succeeded")
	}
	pending, err := store.PendingFinalizeAttempt()
	if err != nil || pending == nil || pending.ReceiptPublished || pending.Completed {
		t.Fatalf("failed publication marker hid pending attempt: %#v, %v", pending, err)
	}
	writeFinalizeAttemptAtomic = original
	if err := store.MarkFinalizeAttemptReceiptPublished(request.RequestDigest); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteFinalizeAttempt(request.RequestDigest); err != nil {
		t.Fatal(err)
	}
	if pending, err := store.PendingFinalizeAttempt(); err != nil || pending != nil {
		t.Fatalf("completed marker remained pending: %#v, %v", pending, err)
	}
}

func TestFinalizeAttemptJournalCompletedRequestRemainsIdempotent(t *testing.T) {
	repo := initSnapshotRepo(t)
	state := newCompactTestState(t, repo, "finalize-completed-replay")
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.Replace("", "review/start", state)
	if err != nil {
		t.Fatal(err)
	}
	request := finalizeAttemptTestRequest(state.LineageID, revision, "evidence")
	if _, _, err := store.BeginFinalizeAttempt(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteFinalizeAttempt(request.RequestDigest); err != nil {
		t.Fatal(err)
	}
	attempt, replay, err := store.ReconcileFinalizeAttempt(context.Background(), request)
	if err != nil || !replay || !attempt.Completed || attempt.Request.RequestDigest != request.RequestDigest {
		t.Fatalf("completed replay = %#v, replay %v, err %v", attempt, replay, err)
	}
}

func TestFinalizeAttemptJournalDoesNotHideDirectorySyncFailure(t *testing.T) {
	repo := initSnapshotRepo(t)
	state := newCompactTestState(t, repo, "finalize-sync-failure")
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.Replace("", "review/start", state)
	if err != nil {
		t.Fatal(err)
	}
	original := syncReviewDirectory
	syncReviewDirectory = func(string) error { return errors.New("injected directory sync failure") }
	t.Cleanup(func() { syncReviewDirectory = original })
	request := finalizeAttemptTestRequest(state.LineageID, revision, "evidence")
	if _, _, err := store.BeginFinalizeAttempt(context.Background(), request); err == nil {
		t.Fatal("directory sync failure was accepted because journal bytes were readable")
	}
	syncReviewDirectory = original
	attempt, replay, err := store.ReconcileFinalizeAttempt(context.Background(), request)
	if err != nil || !replay || attempt.Request.RequestDigest != request.RequestDigest {
		t.Fatalf("retry after directory sync failure = %#v, replay %v, err %v", attempt, replay, err)
	}
}

func TestFinalizeAttemptCompletionSyncFailureIsSafelyReplayable(t *testing.T) {
	repo := initSnapshotRepo(t)
	state := newCompactTestState(t, repo, "finalize-completion-sync")
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.Replace("", "review/start", state)
	if err != nil {
		t.Fatal(err)
	}
	request := finalizeAttemptTestRequest(state.LineageID, revision, "evidence")
	if _, _, err := store.BeginFinalizeAttempt(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	original := syncReviewDirectory
	syncReviewDirectory = func(string) error { return errors.New("injected completion sync failure") }
	t.Cleanup(func() { syncReviewDirectory = original })
	if err := store.CompleteFinalizeAttempt(request.RequestDigest); err == nil {
		t.Fatal("nonterminal completion sync failure was acknowledged")
	}
	syncReviewDirectory = original
	if pending, err := store.PendingFinalizeAttempt(); err != nil || pending != nil {
		t.Fatalf("completed attempt remained pending: %#v, %v", pending, err)
	}
}

func finalizeAttemptTestRequest(lineage, revision, evidence string) FinalizeAttemptRequest {
	request := FinalizeAttemptRequest{
		LineageID: lineage, ExpectedRevision: revision,
		CandidateDigest:          FinalizeAttemptValueDigest("candidate", "candidate"),
		ReviewerResultsDigest:    FinalizeAttemptValueDigest("reviewer-results", []string{}),
		CorrectionForecastDigest: FinalizeAttemptValueDigest("correction-forecast", 0),
		ValidationDigest:         FinalizeAttemptValueDigest("validation", nil),
		RefuterDigest:            FinalizeAttemptValueDigest("refuter", nil),
		EvidenceDigest:           FinalizeAttemptValueDigest("evidence", evidence),
		FailedDigest:             FinalizeAttemptValueDigest("failed", false),
	}
	request.RequestDigest = FinalizeAttemptRequestDigest(request)
	return request
}
