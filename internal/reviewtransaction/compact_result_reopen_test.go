package reviewtransaction

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestCompactReviewRejectsUnavailableInspectionButHistoricalStateRemainsReadable(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newCompactTestState(t, repo, "result-reopen-inspection")
	if len(state.SelectedLenses) != 1 {
		t.Fatalf("selected lenses = %v, want one medium-risk lens", state.SelectedLenses)
	}
	state, store := startReviewingCompactAuthority(t, repo, state)
	before, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	unavailable := compactLensCaptureRequest(t, store, state, 0)
	unavailable.Result.Evidence = []string{"Access denied; candidate was not inspected."}
	if _, err := store.CaptureAdmittedReviewerResult(t.Context(), unavailable); err == nil || !strings.Contains(err.Error(), "inspection was unavailable") {
		t.Fatalf("unavailable inspection capture error = %v", err)
	}
	afterRejected, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(afterRejected, before) || len(afterRejected.State.AdmittedRoleResults) != 0 {
		t.Fatal("rejected unavailable inspection mutated the capture slot")
	}
	captureCompactLens(t, store, state, 0)
	record := requireCompactRoleCount(t, store, 1)
	view, err := record.State.CompactReviewView()
	if err != nil {
		t.Fatal(err)
	}
	if err := record.State.CompleteReview(CompactReviewInput{LensResults: view.LensResults}); err != nil {
		t.Fatal(err)
	}
}

func TestReopenCompactReviewerResultsRemovesSelectedLensAndDependentRefuterFromRecord(t *testing.T) {
	repo, store, state := highRiskCaptureAuthority(t, "record-reopen-cas")
	for order := range state.SelectedLenses {
		captureCompactLens(t, store, state, order)
	}
	record := requireCompactRoleCount(t, store, len(state.SelectedLenses))
	if err := store.CaptureAdmittedRefuterResult(t.Context(), CompactAdmittedRefuterResultRequest{
		ExpectedRevision: record.State.CapturePhaseRevision, TargetIdentity: record.State.InitialSnapshot.Identity,
		RequestHash: hash("b"), Payload: []byte(`{"results":[]}`),
	}); err != nil {
		t.Fatal(err)
	}
	before := requireCompactRoleCount(t, store, len(state.SelectedLenses)+1)
	state = before.State
	view, err := state.CompactReviewView()
	if err != nil {
		t.Fatal(err)
	}
	if err := state.CompleteReview(CompactReviewInput{LensResults: view.LensResults, RefuterOutcomes: view.RefuterOutcomes}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(before.Revision, "review/complete-review", state); err != nil {
		t.Fatal(err)
	}
	before, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	request := CompactResultReopenRequest{
		LineageID: state.LineageID, ExpectedRevision: before.Revision, TargetIdentity: state.InitialSnapshot.Identity,
		Reason: "reviewer input was wrong", Actor: "maintainer", QuarantineLenses: []string{LensReliability, LensRisk},
	}
	plan, err := PrepareCompactResultReopen(t.Context(), repo, request)
	if err != nil {
		t.Fatalf("prepare record-only reopen: %v", err)
	}
	if !reflect.DeepEqual(plan.QuarantineLenses, []string{LensRisk, LensReliability}) || len(plan.Removed) != 3 || plan.Removed[0].Lens != LensRisk || plan.Removed[1].Lens != LensReliability {
		t.Fatalf("record-only reopen plan = %#v", plan)
	}
	request.MaintainerAuthorization = plan.RequiredMaintainerAuthorization
	wrong := request
	wrong.TargetIdentity = hash("wrong target")
	if _, err := ReopenCompactReviewerResults(t.Context(), repo, wrong); err == nil {
		t.Fatal("reopen accepted a wrong target binding")
	}
	wrong = request
	wrong.QuarantineLenses = []string{LensRisk, LensResilience}
	if _, err := ReopenCompactReviewerResults(t.Context(), repo, wrong); err == nil {
		t.Fatal("reopen accepted an authorization for another quarantine selection")
	}
	reopened, err := ReopenCompactReviewerResults(t.Context(), repo, request)
	if err != nil {
		t.Fatalf("apply record-only reopen: %v", err)
	}
	if reopened.State != StateReviewing || len(reopened.Removed) != 3 || reopened.Removed[0].Lens != LensRisk || reopened.Removed[1].Lens != LensReliability {
		t.Fatalf("record-only reopen = %#v", reopened)
	}
	after, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision == before.Revision || len(after.State.AdmittedRoleResults) != 2 || after.State.CapturePhaseRevision == before.State.CapturePhaseRevision {
		t.Fatalf("reopen did not atomically reduce five active values to two at a fresh capture phase: %#v", after.State)
	}
	for _, admitted := range after.State.AdmittedRoleResults {
		if admitted.Role == CompactRoleRefuter || admitted.Lens == LensRisk || admitted.Lens == LensReliability {
			t.Fatalf("reopen retained an invalid canonical value: %#v", admitted)
		}
		if original := before.State.AdmittedRoleResults[admitted.SelectedOrder]; !reflect.DeepEqual(admitted, original) {
			t.Fatalf("reopen rewrote retained lens tuple, phase, value, or hashes: got %#v, want %#v", admitted, original)
		}
	}
	replayed, err := ReopenCompactReviewerResults(t.Context(), repo, request)
	if err != nil || !replayed.Replayed || replayed.Revision != after.Revision {
		t.Fatalf("exact reopen replay = %#v, %v", replayed, err)
	}
	stale := request
	stale.Reason = "different stale request"
	if _, err := ReopenCompactReviewerResults(t.Context(), repo, stale); err == nil {
		t.Fatal("reopen accepted a stale non-replay request")
	}

	var historical CompactResultReopen
	if err := json.Unmarshal([]byte(`{"selected_lens":"review-risk"}`), &historical); err != nil {
		t.Fatalf("read historical singular reopen audit: %v", err)
	}
	lenses, err := compactResultReopenAuditQuarantineLenses(after.State, historical)
	if err != nil || !reflect.DeepEqual(lenses, []string{LensRisk}) {
		t.Fatalf("historical singular reopen lenses = %v, %v", lenses, err)
	}
}
