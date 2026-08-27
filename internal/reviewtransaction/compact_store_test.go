package reviewtransaction

import (
	"context"
	"strings"
	"testing"
)

func TestCompactHistoricalFailedValidatorRequiresLocalAttempt(t *testing.T) {
	state := CompactState{
		State: StateCorrectionRequired,
		Recovery: &CompactRecoveryProvenance{
			ConsumedCorrectionAttempts: MaxCompactCorrectionAttempts,
		},
	}
	if compactHistoricalFailedValidator(state) {
		t.Fatal("recovery accounting without a local correction attempt must not be treated as a historical failed validator")
	}
}

func newCompactTestState(t *testing.T, repo, lineage string) CompactState {
	return newCompactTestStateWithIntended(t, repo, lineage, []string{})
}

func newCompactTestStateWithIntended(t *testing.T, repo, lineage string, intended []string) CompactState {
	t.Helper()
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: intended})
	if err != nil {
		t.Fatal(err)
	}
	risk, lines, err := (SnapshotBuilder{Repo: repo}).ClassifySnapshotRisk(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	lenses := []string{}
	if risk == RiskMedium {
		lenses = []string{LensReliability}
	} else if risk == RiskHigh {
		lenses = append([]string(nil), supportedLenses...)
	}
	state, err := NewCompactState(Start{
		LineageID: lineage, Mode: ModeOrdinaryBounded, Generation: 1, Snapshot: snapshot,
		PolicyHash: hash("1"), RiskLevel: risk, SelectedLenses: lenses, OriginalChangedLines: &lines,
	})
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func pendingCompactCorrection(t *testing.T, repo, lineage string) (CompactState, Snapshot) {
	t.Helper()
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfour\n")
	state := newCompactTestState(t, repo, lineage)
	finding := Finding{ID: "R3-001", Lens: strings.TrimPrefix(state.SelectedLenses[0], "review-"), Location: "tracked.txt:5", Severity: "CRITICAL", Claim: "wrong value", ProofRefs: []string{"candidate-only failure"}}
	if err := state.CompleteReview(CompactReviewInput{
		LensResults:     []LensResult{{Lens: state.SelectedLenses[0], Findings: []Finding{finding}, Evidence: []string{"reviewed once"}}},
		Classifications: []FindingEvidence{{FindingID: finding.ID, Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "changed hunk"}}, RefuterOutcomes: []EvidenceResult{},
	}); err != nil {
		t.Fatal(err)
	}
	if err := state.BeginCorrection(1); err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfixed\n")
	fix, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetFixDiff, BaseRef: state.CurrentSnapshot.CandidateTree, IntendedUntracked: state.InitialSnapshot.IntendedUntracked, LedgerIDs: state.FixFindingIDs})
	if err != nil {
		t.Fatal(err)
	}
	return state, fix
}

func correctedCompactTestState(t *testing.T, repo, lineage string) CompactState {
	return correctedCompactTestStateWithIntended(t, repo, lineage, []string{})
}

func correctedCompactTestStateWithIntended(t *testing.T, repo, lineage string, intended []string) CompactState {
	t.Helper()
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfour\n")
	for _, path := range intended {
		writeSnapshotFile(t, repo, path, "initial intended content\n")
	}
	state := newCompactTestStateWithIntended(t, repo, lineage, intended)
	finding := Finding{
		ID: "R3-001", Lens: "reliability", Location: "tracked.txt:5", Severity: "CRITICAL",
		Claim: "candidate returns the wrong terminal value", ProofRefs: []string{"differential test fails only on candidate"},
	}
	result := LensResult{Lens: LensReliability, Findings: []Finding{finding}, Evidence: []string{"focused differential test failed"}}
	if err := state.CompleteReview(CompactReviewInput{
		LensResults:     []LensResult{result},
		Classifications: []FindingEvidence{{FindingID: finding.ID, Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "changed hunk causes the failure"}},
		RefuterOutcomes: []EvidenceResult{},
	}); err != nil {
		t.Fatal(err)
	}
	if err := state.BeginCorrection(2); err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfixed\n")
	fix, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetFixDiff, BaseRef: state.InitialSnapshot.CandidateTree,
		IntendedUntracked: state.InitialSnapshot.IntendedUntracked, LedgerIDs: state.FixFindingIDs,
	})
	if err != nil {
		t.Fatal(err)
	}
	fixHash := FixDeltaHashForSnapshot(fix)
	validation := ScopedValidationResult{
		LedgerIDs: state.FixFindingIDs, FixCausedFindings: []Finding{}, FollowUps: []FollowUp{},
		OriginalCriteria:     ValidationCheck{EvidenceHash: hash("2"), FixDeltaHash: fixHash, Passed: true},
		CorrectionRegression: ValidationCheck{EvidenceHash: hash("3"), FixDeltaHash: fixHash, Passed: true},
	}
	if err := state.CompleteCorrectionVerification(fix, 2, bindTargetedValidationForTest(validation, fix)); err != nil {
		t.Fatal(err)
	}
	return state
}

func bindTargetedValidationForTest(validation ScopedValidationResult, fix Snapshot) ScopedValidationResult {
	validation.TargetedValidationRequestHash = hash("9")
	validation.CorrectionTargetIdentity = fix.Identity
	return validation
}
