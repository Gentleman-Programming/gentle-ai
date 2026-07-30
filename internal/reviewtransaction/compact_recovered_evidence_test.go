package reviewtransaction

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestAccountingOnlyRecoveryImportsExactPredecessorEvidenceIntoValidatingSuccessor(t *testing.T) {
	repo := initSnapshotRepo(t)
	predecessor := accountingOnlyEscalatedState(t, repo, "accounting-only-predecessor")
	predecessorStore, predecessorRecord := persistEscalatedRecoveryFixture(t, repo, predecessor)

	successor := recoveredEvidenceSuccessor(t, repo, predecessor, "accounting-only-successor")
	const actor = "maintainer@example.com"
	const reason = "recover repository-derived correction after historical line-accounting escalation"
	authorization := compactRecoveryAuthorizationBinding(
		predecessor.LineageID, predecessorRecord.Revision, successor.InitialSnapshot.Identity, actor, reason,
	)
	recovered, err := RecoverCompactAuthority(context.Background(), repo, CompactRecoveryRequest{
		PredecessorLineageID: predecessor.LineageID, ExpectedPredecessorRevision: predecessorRecord.Revision,
		Successor: successor, Disposition: RecoveryEscalated, Reason: reason, Actor: actor,
		MaintainerAuthorization: authorization,
	})
	if err != nil {
		t.Fatal(err)
	}
	if recovered.State.State != StateValidating || recovered.State.Recovery == nil || recovered.State.Recovery.Evidence == nil {
		t.Fatalf("recovered state = %#v", recovered.State)
	}
	evidence := recovered.State.Recovery.Evidence
	if evidence.NativeCorrectionLines != 2 || evidence.SourceCorrectionAttempt.ActualLines != predecessor.CorrectionBudget+1 ||
		evidence.TargetedValidationRequest.ExpectedRevision != predecessorRecord.Revision ||
		evidence.TargetedValidationRequest.TargetIdentity != predecessor.InitialSnapshot.Identity ||
		evidence.TargetedValidationRequest.CorrectionTargetIdentity != predecessor.CurrentSnapshot.Identity ||
		evidence.ReviewEvidenceHash != compactReviewEvidenceHash(predecessor) ||
		!reflect.DeepEqual(recovered.State.LensResults, predecessor.LensResults) ||
		!reflect.DeepEqual(recovered.State.Classifications, predecessor.Classifications) ||
		!reflect.DeepEqual(recovered.State.Outcomes, predecessor.Outcomes) ||
		recovered.State.FixDeltaHash != predecessor.FixDeltaHash ||
		recovered.State.ActualCorrectionLines == nil || *recovered.State.ActualCorrectionLines != 2 ||
		len(recovered.State.CorrectionAttempts) != 0 || !snapshotsEqual(recovered.State.InitialSnapshot, recovered.State.CurrentSnapshot) {
		t.Fatalf("imported recovery evidence = %#v, state=%#v", evidence, recovered.State)
	}
	if err := validateCompactRecoveryEdge(predecessorRecord, recovered.State); err != nil {
		t.Fatalf("recovered edge: %v", err)
	}
	if err := recovered.State.CompleteVerification([]byte("successor-bound final verification passed\n"), true); err != nil {
		t.Fatal(err)
	}
	receipt, err := recovered.State.Receipt()
	if err != nil || receipt.FixDeltaHash != predecessor.FixDeltaHash || receipt.TerminalState != TerminalApproved {
		t.Fatalf("recovered receipt = %#v, err=%v", receipt, err)
	}

	unchangedPredecessor, err := predecessorStore.Load()
	if err != nil || unchangedPredecessor.Revision != predecessorRecord.Revision ||
		!compactStateEqual(unchangedPredecessor.State, predecessorRecord.State) {
		t.Fatalf("recovery mutated predecessor: err=%v before=%#v after=%#v", err, predecessorRecord, unchangedPredecessor)
	}
}

func TestAccountingOnlyRecoveryFailsClosedOrStartsFreshWhenEvidenceDoesNotMatch(t *testing.T) {
	t.Run("stale predecessor revision", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		predecessor := accountingOnlyEscalatedState(t, repo, "accounting-stale-predecessor")
		_, record := persistEscalatedRecoveryFixture(t, repo, predecessor)
		successor := recoveredEvidenceSuccessor(t, repo, predecessor, "accounting-stale-successor")
		request := CompactRecoveryRequest{
			PredecessorLineageID: predecessor.LineageID, ExpectedPredecessorRevision: hash("8"), Successor: successor,
			Disposition: RecoveryEscalated, Reason: "stale", Actor: "maintainer@example.com",
			MaintainerAuthorization: compactRecoveryAuthorizationBinding(predecessor.LineageID, record.Revision, successor.InitialSnapshot.Identity, "maintainer@example.com", "stale"),
		}
		if _, err := RecoverCompactAuthority(context.Background(), repo, request); !errorsIsConcurrentUpdate(err) {
			t.Fatalf("stale recovery error = %v", err)
		}
	})

	t.Run("policy mismatch cannot import", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		predecessor := accountingOnlyEscalatedState(t, repo, "accounting-policy-predecessor")
		_, record := persistEscalatedRecoveryFixture(t, repo, predecessor)
		successor := recoveredEvidenceSuccessor(t, repo, predecessor, "accounting-policy-successor")
		successor.PolicyHash = hash("7")
		const actor, reason = "maintainer@example.com", "policy mismatch"
		_, err := RecoverCompactAuthority(context.Background(), repo, CompactRecoveryRequest{
			PredecessorLineageID: predecessor.LineageID, ExpectedPredecessorRevision: record.Revision, Successor: successor,
			Disposition: RecoveryEscalated, Reason: reason, Actor: actor,
			MaintainerAuthorization: compactRecoveryAuthorizationBinding(predecessor.LineageID, record.Revision, successor.InitialSnapshot.Identity, actor, reason),
		})
		if err == nil || !strings.Contains(err.Error(), "target has not changed") {
			t.Fatalf("policy-mismatched evidence import error = %v", err)
		}
	})

	t.Run("changed bytes use fresh review", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		predecessor := accountingOnlyEscalatedState(t, repo, "accounting-changed-predecessor")
		_, record := persistEscalatedRecoveryFixture(t, repo, predecessor)
		if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nchanged again\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		successor := recoveredEvidenceSuccessor(t, repo, predecessor, "accounting-changed-successor")
		const actor, reason = "maintainer@example.com", "changed candidate recovery"
		recovered, err := RecoverCompactAuthority(context.Background(), repo, CompactRecoveryRequest{
			PredecessorLineageID: predecessor.LineageID, ExpectedPredecessorRevision: record.Revision, Successor: successor,
			Disposition: RecoveryEscalated, Reason: reason, Actor: actor,
			MaintainerAuthorization: compactRecoveryAuthorizationBinding(predecessor.LineageID, record.Revision, successor.InitialSnapshot.Identity, actor, reason),
		})
		if err != nil {
			t.Fatal(err)
		}
		if recovered.State.State != StateReviewing || recovered.State.Recovery.Evidence != nil || len(recovered.State.LensResults) != 0 {
			t.Fatalf("changed target reused predecessor evidence: %#v", recovered.State)
		}
	})
}

func accountingOnlyEscalatedState(t *testing.T, repo, lineage string) CompactState {
	t.Helper()
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfour\n")
	state := newCompactTestState(t, repo, lineage)
	if state.CorrectionBudget < 2 || len(state.SelectedLenses) != 1 {
		t.Fatalf("fixture risk/budget = %q/%d", state.RiskLevel, state.CorrectionBudget)
	}
	finding := Finding{
		ID: "R3-001", Lens: "reliability", Location: "tracked.txt:5", Severity: "CRITICAL",
		Claim: "candidate retains the wrong value", ProofRefs: []string{"candidate-only differential failure"},
	}
	if err := state.CompleteReview(CompactReviewInput{
		LensResults:     []LensResult{{Lens: state.SelectedLenses[0], Findings: []Finding{finding}, Evidence: []string{"reviewed exact candidate tree"}}},
		Classifications: []FindingEvidence{{FindingID: finding.ID, Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "changed hunk"}},
		RefuterOutcomes: []EvidenceResult{},
	}); err != nil {
		t.Fatal(err)
	}
	if err := state.BeginCorrection(2); err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfixed\n")
	fix, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetFixDiff, Projection: state.InitialSnapshot.Projection, BaseRef: state.CurrentSnapshot.CandidateTree,
		IntendedUntracked: state.InitialSnapshot.IntendedUntracked, LedgerIDs: state.FixFindingIDs,
	})
	if err != nil {
		t.Fatal(err)
	}
	fixHash := FixDeltaHashForSnapshot(fix)
	validation := bindTargetedValidationForTest(ScopedValidationResult{
		LedgerIDs: state.FixFindingIDs, FixCausedFindings: []Finding{}, FollowUps: []FollowUp{},
		OriginalCriteria:     ValidationCheck{EvidenceHash: hash("2"), FixDeltaHash: fixHash, Passed: true},
		CorrectionRegression: ValidationCheck{EvidenceHash: hash("3"), FixDeltaHash: fixHash, Passed: true},
	}, fix)
	if err := state.CompleteCorrection(fix, state.CorrectionBudget+1, validation); err != nil {
		t.Fatal(err)
	}
	if !compactAccountingOnlyEscalation(state) {
		t.Fatalf("fixture is not accounting-only escalation: %#v", state)
	}
	return state
}

func persistEscalatedRecoveryFixture(t *testing.T, repo string, state CompactState) (CompactStore, CompactRecord) {
	t.Helper()
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, payload, err := makeCompactRecord(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(store.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(store.StatePath(), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	receipt, err := state.Receipt()
	if err != nil {
		t.Fatalf("persist escalated receipt: %v", err)
	}
	if err := WriteCompactReceiptAtomic(store.ReceiptPath(), receipt); err != nil {
		t.Fatalf("persist escalated receipt: %v", err)
	}
	return store, record
}

func recoveredEvidenceSuccessor(t *testing.T, repo string, predecessor CompactState, lineage string) CompactState {
	t.Helper()
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetCurrentChanges, Projection: predecessor.InitialSnapshot.Projection,
		IntendedUntracked: predecessor.InitialSnapshot.IntendedUntracked,
	})
	if err != nil {
		t.Fatal(err)
	}
	risk, lines, err := (SnapshotBuilder{Repo: repo}).ClassifySnapshotRisk(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewCompactState(Start{
		LineageID: lineage, Mode: ModeOrdinaryBounded, Generation: predecessor.Generation + 1,
		Snapshot: snapshot, PolicyHash: predecessor.PolicyHash, RiskLevel: risk,
		SelectedLenses: append([]string(nil), predecessor.SelectedLenses...), OriginalChangedLines: &lines,
	})
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func errorsIsConcurrentUpdate(err error) bool {
	return err != nil && strings.Contains(err.Error(), ErrConcurrentUpdate.Error())
}
