package reviewtransaction

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestCompleteCorrectionVerificationIsAtomicAndCandidateBound(t *testing.T) {
	repo := initSnapshotRepo(t)
	state, fix := pendingCompactCorrection(t, repo, "atomic-correction-verification")
	before := state
	fixHash := FixDeltaHashForSnapshot(fix)
	validation := bindTargetedValidationForTest(ScopedValidationResult{
		LedgerIDs: append([]string(nil), state.FixFindingIDs...), FixCausedFindings: []Finding{}, FollowUps: []FollowUp{},
		OriginalCriteria:     ValidationCheck{EvidenceHash: hash("2"), FixDeltaHash: fixHash, Passed: true},
		CorrectionRegression: ValidationCheck{EvidenceHash: hash("3"), FixDeltaHash: fixHash, Passed: true},
	}, fix)
	if err := state.CompleteCorrectionVerification(fix, 1, validation); err != nil {
		t.Fatal(err)
	}
	if state.State != StateApproved || len(state.CorrectionAttempts) != 1 || state.CumulativeCorrectionLines != 1 ||
		!snapshotsEqual(state.CurrentSnapshot, fix) || !snapshotsEqual(state.CorrectionAttempts[0].Snapshot, fix) {
		t.Fatalf("terminal targeted-validator correction state = %#v", state)
	}
	if _, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetFixDiff, BaseRef: before.CurrentSnapshot.CandidateTree, IntendedUntracked: before.InitialSnapshot.IntendedUntracked, LedgerIDs: before.FixFindingIDs}); err != nil {
		t.Fatal(err)
	}
}

func TestCompleteCorrectionVerificationEscalatesConclusiveFailedValidator(t *testing.T) {
	repo := initSnapshotRepo(t)
	state, fix := pendingCompactCorrection(t, repo, "failed-targeted-validator")
	fixHash := FixDeltaHashForSnapshot(fix)
	validation := bindTargetedValidationForTest(ScopedValidationResult{
		LedgerIDs: append([]string(nil), state.FixFindingIDs...), FixCausedFindings: []Finding{}, FollowUps: []FollowUp{},
		OriginalCriteria:     ValidationCheck{EvidenceHash: hash("2"), FixDeltaHash: fixHash, Passed: false},
		CorrectionRegression: ValidationCheck{EvidenceHash: hash("3"), FixDeltaHash: fixHash, Passed: true},
	}, fix)
	if err := state.CompleteCorrectionVerification(fix, 1, validation); err != nil {
		t.Fatalf("complete failed validator verification: %v", err)
	}
	if state.State != StateEscalated || len(state.CorrectionAttempts) != 1 || state.CorrectionAttempts[0].OriginalCriteria.Passed ||
		state.CorrectionAttempts[0].CorrectionTargetIdentity != fix.Identity || !snapshotsEqual(state.CurrentSnapshot, fix) {
		t.Fatalf("failed targeted-validator correction state = %#v", state)
	}
}

func TestCompleteCorrectionVerificationPreservesFullCandidateScope(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "base\nwrong\n")
	writeSnapshotFile(t, repo, "deleted.txt", "reviewed companion\n")
	state := newCompactTestState(t, repo, "full-candidate-correction")
	state, store := startReviewingCompactAuthority(t, repo, state)
	if !reflect.DeepEqual(state.GenesisPaths, []string{"deleted.txt", "tracked.txt"}) {
		t.Fatalf("genesis paths = %v", state.GenesisPaths)
	}
	finding := Finding{
		ID: "R3-001", Lens: strings.TrimPrefix(state.SelectedLenses[0], "review-"), Location: "tracked.txt:2", Severity: "CRITICAL",
		Claim: "wrong value", ProofRefs: []string{"candidate-only failure"}, EvidenceClass: EvidenceDeterministic, CausalDisposition: CausalIntroduced,
	}
	results := make([]LensResult, len(state.SelectedLenses))
	for index, lens := range state.SelectedLenses {
		results[index] = LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"reviewed"}}
		if index == 0 {
			results[index].Findings = []Finding{finding}
		}
	}
	state, _ = captureAndCompleteCompactReview(t, store, state, CompactReviewInput{
		LensResults:     results,
		Classifications: []FindingEvidence{{FindingID: finding.ID, Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "changed hunk causes failure"}},
		RefuterOutcomes: []EvidenceResult{},
	})
	if err := state.BeginCorrection(1); err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "base\nfixed\n")
	builder := SnapshotBuilder{Repo: repo}
	fix, err := builder.Build(context.Background(), Target{Kind: TargetFixDiff, BaseRef: state.CurrentSnapshot.CandidateTree, IntendedUntracked: state.InitialSnapshot.IntendedUntracked, LedgerIDs: state.FixFindingIDs})
	if err != nil {
		t.Fatal(err)
	}
	full, err := builder.BuildCorrectedCandidate(context.Background(), state.InitialSnapshot, fix)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fix.Paths, []string{"tracked.txt"}) || !reflect.DeepEqual(full.Paths, state.GenesisPaths) || fix.PathsDigest == full.PathsDigest {
		t.Fatalf("correction paths = %v (%s), full paths = %v (%s)", fix.Paths, fix.PathsDigest, full.Paths, full.PathsDigest)
	}
	fixHash := FixDeltaHashForSnapshot(fix)
	validation := bindTargetedValidationForTest(ScopedValidationResult{
		LedgerIDs: state.FixFindingIDs, FixCausedFindings: []Finding{}, FollowUps: []FollowUp{},
		OriginalCriteria:     ValidationCheck{EvidenceHash: hash("2"), FixDeltaHash: fixHash, Passed: true},
		CorrectionRegression: ValidationCheck{EvidenceHash: hash("3"), FixDeltaHash: fixHash, Passed: true},
	}, fix)
	if err := state.CompleteCorrectionVerification(fix, 1, validation, full); err != nil {
		t.Fatal(err)
	}
	if state.State != StateApproved || !snapshotsEqual(state.CurrentSnapshot, full) ||
		!snapshotsEqual(state.CorrectionAttempts[0].Snapshot, fix) ||
		!reflect.DeepEqual(state.CurrentSnapshot.Paths, state.GenesisPaths) {
		t.Fatalf("terminal full-candidate correction state = %#v, correction = %#v", state.CurrentSnapshot, state.CorrectionAttempts[0].Snapshot)
	}
}
