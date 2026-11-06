package reviewtransaction

import (
	"context"
	"reflect"
	"testing"
)

// TestCompactCaptureMergesCanonicalResultIntoTheOnlyAuthority proves that a
// capture advances the live record revision without changing the capture phase,
// retains the admitted canonical value in the record, and creates no role slot.
func TestCompactReopenRemovesSelectedLensAndDependentRefuterFromTheOnlyAuthority(t *testing.T) {
	_, store, state := highRiskCaptureAuthority(t, "record-only-reopen")
	for order := range state.SelectedLenses {
		captureCompactLens(t, store, state, order)
	}
	record := requireCompactRoleCount(t, store, len(state.SelectedLenses))
	phase := record.State.CapturePhaseRevision
	if err := store.CaptureAdmittedRefuterResult(t.Context(), CompactAdmittedRefuterResultRequest{
		ExpectedRevision: phase, TargetIdentity: record.State.InitialSnapshot.Identity,
		RequestHash: hash("b"), Payload: []byte(`{"results":[]}`),
	}); err != nil {
		t.Fatal(err)
	}
	record = requireCompactRoleCount(t, store, len(state.SelectedLenses)+1)
	state = record.State
	view, err := state.CompactReviewView()
	if err != nil {
		t.Fatal(err)
	}
	if err := state.CompleteReview(CompactReviewInput{LensResults: view.LensResults, RefuterOutcomes: view.RefuterOutcomes}); err != nil {
		t.Fatal(err)
	}
	if state.State != StateValidating {
		t.Fatalf("completed reopened fixture state = %q, want %q", state.State, StateValidating)
	}
	original := cloneCompactAdmittedRoleResults(state.AdmittedRoleResults)

	next, removed, err := reopenCompactAdmittedRoleResults(state, []string{LensRisk})
	if err != nil {
		t.Fatal(err)
	}
	if len(next.AdmittedRoleResults) != 3 || len(removed) != 2 {
		t.Fatalf("reopen cardinality = %d values and %d removals, want 3 and 2", len(next.AdmittedRoleResults), len(removed))
	}
	if next.CapturePhaseRevision == phase {
		t.Fatal("reopen did not derive a fresh capture phase")
	}
	for _, result := range next.AdmittedRoleResults {
		if result.Role == CompactRoleRefuter || result.Lens == LensRisk {
			t.Fatalf("reopen retained invalidated role value %#v", result)
		}
		if result.Role == CompactRoleLens && result.CapturePhaseRevision != phase {
			t.Fatalf("retained lens phase = %q, want original phase %q", result.CapturePhaseRevision, phase)
		}
		if result.Role == CompactRoleLens && !reflect.DeepEqual(result, original[result.SelectedOrder]) {
			t.Fatalf("reopen rewrote retained lens tuple %#v, want %#v", result, original[result.SelectedOrder])
		}
	}
	for _, result := range removed {
		if result.Role == CompactRoleRefuter && len(result.Value) != 0 {
			t.Fatalf("reopen audit retained refuter payload %#v", result)
		}
	}

	withoutRefuter := state
	withoutRefuter.AdmittedRoleResults = append([]CompactAdmittedRoleResult(nil), state.AdmittedRoleResults[:len(state.AdmittedRoleResults)-1]...)
	noRefuter, noRefuterRemoved, err := reopenCompactAdmittedRoleResults(withoutRefuter, []string{LensRisk})
	if err != nil {
		t.Fatalf("reopen without refuter: %v", err)
	}
	if len(noRefuterRemoved) != 1 || noRefuterRemoved[0].Role != CompactRoleLens || len(noRefuter.AdmittedRoleResults) != 3 {
		t.Fatalf("no-refuter reopen = values %#v, removals %#v", noRefuter.AdmittedRoleResults, noRefuterRemoved)
	}
}

func TestCompactCaptureMergesCanonicalResultIntoTheOnlyAuthority(t *testing.T) {
	fixture := newCompactReviewerCaptureFixture(t, "record-only-capture")
	phase := fixture.state.CapturePhaseRevision

	first, err := fixture.store.CaptureAdmittedReviewerResult(t.Context(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	record, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if record.Revision == phase {
		t.Fatal("capture did not advance the live authority revision")
	}
	if record.State.CapturePhaseRevision != phase {
		t.Fatalf("capture phase = %q, want stable phase %q", record.State.CapturePhaseRevision, phase)
	}
	if len(record.State.AdmittedRoleResults) != 1 {
		t.Fatalf("admitted role results = %d, want one", len(record.State.AdmittedRoleResults))
	}
	stored := record.State.AdmittedRoleResults[0]
	if stored.Role != CompactRoleLens || stored.Lens != LensReliability || stored.ResultHash != first.ResultHash || stored.ArtifactDigest == "" || len(stored.Value) == 0 {
		t.Fatalf("stored admitted role result = %#v", stored)
	}
	replayed, err := fixture.store.CaptureAdmittedReviewerResult(context.Background(), fixture.request)
	if err != nil {
		t.Fatalf("exact replay: %v", err)
	}
	after, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if replayed.ResultHash != first.ResultHash || after.Revision != record.Revision || len(after.State.AdmittedRoleResults) != 1 {
		t.Fatalf("exact replay changed authority: replay=%#v record=%#v", replayed, after)
	}
}

func TestCompactReviewViewDerivesLensResultFromActiveAuthority(t *testing.T) {
	fixture := newCompactReviewerCaptureFixture(t, "derived-view-active-authority")
	if _, err := fixture.store.CaptureAdmittedReviewerResult(t.Context(), fixture.request); err != nil {
		t.Fatal(err)
	}
	record := requireCompactRoleCount(t, fixture.store, 1)
	state := record.State
	state.State = StateValidating
	if err := state.Validate(); err != nil {
		t.Fatalf("admitted capture fixture must remain readable: %v", err)
	}

	view, err := state.CompactReviewView()
	if err != nil {
		t.Fatal(err)
	}
	if len(view.LensResults) != 1 || view.LensResults[0].Evidence[0] != fixture.request.Result.Evidence[0] {
		t.Fatalf("derived lens result = %#v, want active admitted value %#v", view.LensResults, fixture.request.Result)
	}
}

func TestCompactActiveAuthorityFollowsFiveThreeFourFiveSixSequence(t *testing.T) {
	repo, store, state := highRiskCaptureAuthority(t, "active-five-three-four-five-six")
	for order := range state.SelectedLenses {
		captureCompactLens(t, store, state, order)
	}
	record := requireCompactRoleCount(t, store, 4)
	refuter := CompactAdmittedRefuterResultRequest{ExpectedRevision: record.State.CapturePhaseRevision, TargetIdentity: record.State.InitialSnapshot.Identity, RequestHash: hash("a"), Payload: []byte(`{"results":[]}`)}
	if err := store.CaptureAdmittedRefuterResult(t.Context(), refuter); err != nil {
		t.Fatal(err)
	}
	record = requireCompactRoleCount(t, store, 5)
	beforeReopen := record
	record = reopenOneCapturedLens(t, repo, store, record, LensRisk)
	requireCompactRoleCount(t, store, 3)
	assertActiveLensOrders(t, record.State, false, true, true, true)
	for _, retained := range record.State.AdmittedRoleResults {
		if retained.Role != CompactRoleLens {
			continue
		}
		if retained.CapturePhaseRevision != beforeReopen.State.CapturePhaseRevision {
			t.Fatalf("retained lens phase = %q, want original phase %q", retained.CapturePhaseRevision, beforeReopen.State.CapturePhaseRevision)
		}
		active, found, err := record.State.ActiveAdmittedLensResult(retained.SelectedOrder)
		if err != nil || !found || !reflect.DeepEqual(active, retained) {
			t.Fatalf("retained active lens slot = %#v, found=%t, err=%v", active, found, err)
		}
	}
	finding := Finding{
		ID: "R1-001", Lens: LensRisk, Location: "tracked.txt:1", Severity: "CRITICAL",
		Claim: "candidate needs correction", ProofRefs: []string{"candidate-only failure"},
		EvidenceClass: EvidenceDeterministic, CausalDisposition: CausalIntroduced,
	}
	captureCompactLens(t, store, record.State, 0, finding)
	record = requireCompactRoleCount(t, store, 4)
	assertActiveLensOrders(t, record.State, true, true, true, true)
	refuter.ExpectedRevision, refuter.RequestHash = record.State.CapturePhaseRevision, hash("b")
	if err := store.CaptureAdmittedRefuterResult(t.Context(), refuter); err != nil {
		t.Fatal(err)
	}
	record = requireCompactRoleCount(t, store, 5)

	view, err := record.State.CompactReviewView()
	if err != nil {
		t.Fatal(err)
	}
	if err := record.State.CompleteReview(CompactReviewInput{
		LensResults: view.LensResults, Classifications: []FindingEvidence{view.Classifications[finding.ID]},
	}); err != nil {
		t.Fatal(err)
	}
	assertActiveLensOrders(t, record.State, true, true, true, true)
	revision, err := store.Replace(record.Revision, "review/complete-review", record.State)
	if err != nil {
		t.Fatal(err)
	}
	if err := record.State.BeginCorrection(1); err != nil {
		t.Fatal(err)
	}
	assertActiveLensOrders(t, record.State, true, true, true, true)
	stale := record.State
	stale.AdmittedRoleResults = cloneCompactAdmittedRoleResults(record.State.AdmittedRoleResults)
	stale.AdmittedRoleResults[0].CapturePhaseRevision = hash("unexplained stale active lens phase")
	if _, _, err := stale.ActiveAdmittedLensResult(0); err == nil {
		t.Fatal("unexplained stale active lens phase was accepted")
	}
	if _, err := store.Replace(revision, "review/begin-fix", record.State); err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "fixed\n")
	fix, err := (SnapshotBuilder{Repo: repo}).Build(t.Context(), Target{Kind: TargetFixDiff, BaseRef: record.State.CurrentSnapshot.CandidateTree, IntendedUntracked: []string{}, LedgerIDs: record.State.FixFindingIDs})
	if err != nil {
		t.Fatal(err)
	}
	request, err := BuildTargetedValidationRequestFromSnapshot(t.Context(), repo, record.State, record.State.CapturePhaseRevision, fix)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CaptureAdmittedTargetedValidatorResult(t.Context(), CompactAdmittedTargetedValidatorResultRequest{ExpectedRequest: request, Payload: []byte(`{"outcome":"passed"}`)}); err != nil {
		t.Fatal(err)
	}
	record = requireCompactRoleCount(t, store, 6)
	roles := map[CompactRole]int{}
	for _, entry := range record.State.AdmittedRoleResults {
		roles[entry.Role]++
	}
	if roles[CompactRoleLens] != 4 || roles[CompactRoleRefuter] != 1 || roles[CompactRoleTargetedValidator] != 1 {
		t.Fatalf("six-role authority = %#v", roles)
	}
	view, err = record.State.CompactReviewView()
	if err != nil || len(view.LensResults) != 4 || len(view.RefuterOutcomes) != 0 || view.TargetedValidatorOutcome != "passed" {
		t.Fatalf("six-role derived view = %#v, %v", view, err)
	}
}

func assertActiveLensOrders(t *testing.T, state CompactState, want ...bool) {
	t.Helper()
	for order, expected := range want {
		_, found, err := state.ActiveAdmittedLensResult(order)
		if err != nil || found != expected {
			t.Fatalf("active lens order %d = found=%t err=%v, want found=%t", order, found, err, expected)
		}
	}
}

func highRiskCaptureAuthority(t *testing.T, lineage string) (string, CompactStore, CompactState) {
	t.Helper()
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newCompactTestState(t, repo, lineage)
	policy := "frozen test policy"
	state.RiskLevel, state.SelectedLenses = RiskHigh, append([]string(nil), supportedLenses...)
	state.PolicyHash, state.FrozenPolicyContent = compactPolicyContentHash(policy), &policy
	state, store := startReviewingCompactAuthority(t, repo, state)
	return repo, store, state
}

func captureCompactLens(t *testing.T, store CompactStore, state CompactState, order int, findings ...Finding) {
	t.Helper()
	if _, err := store.CaptureAdmittedReviewerResult(t.Context(), compactLensCaptureRequest(t, store, state, order, findings...)); err != nil {
		t.Fatal(err)
	}
}

func reopenOneCapturedLens(t *testing.T, repo string, store CompactStore, record CompactRecord, lens string) CompactRecord {
	t.Helper()
	if record.State.State == StateReviewing {
		view, err := record.State.CompactReviewView()
		if err != nil {
			t.Fatal(err)
		}
		if err := record.State.CompleteReview(CompactReviewInput{LensResults: view.LensResults}); err != nil {
			t.Fatal(err)
		}
		record.Revision, err = store.Replace(record.Revision, "review/complete-review", record.State)
		if err != nil {
			t.Fatal(err)
		}
	}
	request := CompactResultReopenRequest{LineageID: record.State.LineageID, ExpectedRevision: record.Revision, TargetIdentity: record.State.InitialSnapshot.Identity, Reason: "reviewer input was wrong", Actor: "maintainer", QuarantineLenses: []string{lens}}
	plan, err := PrepareCompactResultReopen(t.Context(), repo, request)
	if err != nil {
		t.Fatal(err)
	}
	request.MaintainerAuthorization = plan.RequiredMaintainerAuthorization
	if _, err := ReopenCompactReviewerResults(t.Context(), repo, request); err != nil {
		t.Fatal(err)
	}
	record, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func requireCompactRoleCount(t *testing.T, store CompactStore, want int) CompactRecord {
	t.Helper()
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := len(record.State.AdmittedRoleResults); got != want || got > compactMaxAdmittedRoleResults {
		t.Fatalf("active role count = %d, want %d within global bound %d", got, want, compactMaxAdmittedRoleResults)
	}
	return record
}
