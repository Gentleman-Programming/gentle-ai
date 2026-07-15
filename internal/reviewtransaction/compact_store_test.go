package reviewtransaction

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCompactLifecycleTransitionsAreCASBoundAndBudgetStable(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state, store, _ := approvedCompactCurrentChangesFixture(t, repo, "verify-remediation", []string{})
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	revision := record.Revision
	budget := state.CorrectionBudget
	if _, err := store.RecordVerifyFailure(revision, "sha256:"+strings.Repeat("a", 64)); err != nil {
		t.Fatalf("record verify failure: %v", err)
	}
	failed, err := store.Load()
	if err != nil || failed.State.State != StateVerifyFailed || failed.State.CorrectionBudget != budget {
		t.Fatalf("failed state = %#v, %v", failed, err)
	}
	if _, err := store.AuthorizeVerifyRemediation(failed.Revision, "sha256:"+strings.Repeat("a", 64)); err != nil {
		t.Fatalf("authorize remediation: %v", err)
	}
	remediating, _ := store.Load()
	if _, err := store.CompleteVerifyRemediation(remediating.Revision, "sha256:"+strings.Repeat("a", 64)); err != nil {
		t.Fatalf("complete remediation: %v", err)
	}
	if _, err := store.EscalateValidatorFailure(revision, "validator failure"); !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("stale competitor = %v, want concurrent update", err)
	}
}

func TestCompactStoreRecoverCreatesAuditableSuccessorWithoutChangingPredecessor(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	predecessor, predecessorStore, _ := approvedCompactRevisionFixture(t, repo, "recovery-approved")
	predecessorRecord, err := predecessorStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	predecessorRevision := predecessorRecord.Revision
	receiptBefore, err := os.ReadFile(predecessorStore.ReceiptPath())
	if err != nil {
		t.Fatal(err)
	}
	stateBefore, err := os.ReadFile(predecessorStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "changed scope\n")
	successor := newCompactTestState(t, repo, "recovery-approved-g2")
	successor.Generation = predecessor.Generation + 1
	recoveredAt := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	record, err := RecoverCompactAuthority(context.Background(), repo, CompactRecoveryRequest{
		PredecessorLineageID: predecessor.LineageID, ExpectedPredecessorRevision: predecessorRevision,
		Successor: successor, Disposition: RecoveryScopeChanged, Reason: "candidate scope changed after approval",
		Actor: "maintainer@example.com", RecoveredAt: recoveredAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.State.Recovery == nil || record.State.Recovery.PredecessorLineageID != predecessor.LineageID ||
		record.State.Recovery.PredecessorRevision != predecessorRevision || record.State.Recovery.Disposition != RecoveryScopeChanged ||
		record.State.Recovery.Actor != "maintainer@example.com" || !record.State.Recovery.RecoveredAt.Equal(recoveredAt) {
		t.Fatalf("recovery provenance = %#v", record.State.Recovery)
	}
	stateAfter, _ := os.ReadFile(predecessorStore.StatePath())
	receiptAfter, _ := os.ReadFile(predecessorStore.ReceiptPath())
	if !bytes.Equal(stateBefore, stateAfter) || !bytes.Equal(receiptBefore, receiptAfter) {
		t.Fatal("recovery changed predecessor state or receipt bytes")
	}
	retryRequest := CompactRecoveryRequest{
		PredecessorLineageID: predecessor.LineageID, ExpectedPredecessorRevision: predecessorRevision,
		Successor: successor, Disposition: RecoveryScopeChanged, Reason: "candidate scope changed after approval",
		Actor: "maintainer@example.com", RecoveredAt: recoveredAt,
	}
	retry, err := RecoverCompactAuthority(context.Background(), repo, retryRequest)
	if err != nil || retry.Revision != record.Revision || !compactStateEqual(retry.State, record.State) {
		t.Fatalf("exact recovery retry = %#v, %v", retry, err)
	}
	retryRequest.Reason = "different reason"
	if _, err := RecoverCompactAuthority(context.Background(), repo, retryRequest); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("conflicting recovery retry error = %v", err)
	}
	if _, err := RecoverCompactAuthority(context.Background(), repo, CompactRecoveryRequest{
		PredecessorLineageID: predecessor.LineageID, ExpectedPredecessorRevision: predecessorRevision,
		Successor: newCompactTestState(t, repo, "recovery-approved-fork"), Disposition: RecoveryScopeChanged,
		Reason: "second successor", Actor: "maintainer@example.com", RecoveredAt: recoveredAt,
	}); err == nil || !strings.Contains(err.Error(), "already has successor") {
		t.Fatalf("fork recovery error = %v", err)
	}
}

func TestRecoverCompactAuthorityRejectsProjectionChange(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	gitSnapshot(t, repo, "add", "--", "tracked.txt")
	state, store, _ := approvedCompactCurrentChangesFixture(t, repo, "recovery-staged-projection", []string{})
	state.InitialSnapshot.Projection = ProjectionStaged
	state.CurrentSnapshot.Projection = ProjectionStaged
	state.InitialSnapshot.Identity = snapshotIdentityForProjection(state.InitialSnapshot.Kind, state.InitialSnapshot.Projection, state.InitialSnapshot.BaseTree, state.InitialSnapshot.CandidateTree, state.InitialSnapshot.PathsDigest, state.InitialSnapshot.IntendedUntrackedProof, state.InitialSnapshot.IntendedUntracked, state.InitialSnapshot.LedgerIDs)
	state.CurrentSnapshot.Identity = snapshotIdentityForProjection(state.CurrentSnapshot.Kind, state.CurrentSnapshot.Projection, state.CurrentSnapshot.BaseTree, state.CurrentSnapshot.CandidateTree, state.CurrentSnapshot.PathsDigest, state.CurrentSnapshot.IntendedUntrackedProof, state.CurrentSnapshot.IntendedUntracked, state.CurrentSnapshot.LedgerIDs)
	record, payload, err := makeCompactRecord(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.StatePath(), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	receipt, err := state.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteCompactReceiptAtomic(store.ReceiptPath(), receipt); err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "new workspace scope\n")
	successor := newCompactTestState(t, repo, "recovery-workspace-projection")
	successor.Generation = state.Generation + 1
	if _, err := RecoverCompactAuthority(context.Background(), repo, CompactRecoveryRequest{PredecessorLineageID: state.LineageID, ExpectedPredecessorRevision: record.Revision, Successor: successor, Disposition: RecoveryScopeChanged, Reason: "scope changed", Actor: "maintainer"}); err == nil || !strings.Contains(err.Error(), "projection") {
		t.Fatalf("cross-projection recovery error = %v", err)
	}
}

func TestApprovedRecoveryTreatsBaseTreeMismatchAsScopeChange(t *testing.T) {
	snapshot := Snapshot{BaseTree: strings.Repeat("a", 40), CandidateTree: strings.Repeat("c", 40), PathsDigest: hash("1")}
	predecessor, successor := CompactState{State: StateApproved, CurrentSnapshot: snapshot}, CompactState{InitialSnapshot: snapshot}
	successor.InitialSnapshot.BaseTree = strings.Repeat("b", 40)
	predecessor.CurrentSnapshot.Kind, successor.InitialSnapshot.Kind = TargetCurrentChanges, TargetCurrentChanges
	if !compactRecoveryScopeChanged(predecessor.CurrentSnapshot, successor.InitialSnapshot) {
		t.Fatal("approved base-only mismatch was not recovery-eligible")
	}
	successor.InitialSnapshot.Kind = TargetFixDiff
	if compactRecoveryScopeChanged(predecessor.CurrentSnapshot, successor.InitialSnapshot) {
		t.Fatal("incompatible snapshot kinds created false base-only recovery")
	}
}

func TestCompactGateFinalRecheckRejectsConcurrentRecoverySuccessor(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state, store, receipt := approvedCompactCurrentChangesFixture(t, repo, "compact-recovery-race", []string{})
	gitSnapshot(t, repo, "add", "--", "tracked.txt")
	predecessor, _ := store.Load()
	originalHook := finalGateAuthorizationHook
	t.Cleanup(func() { finalGateAuthorizationHook = originalHook })
	finalGateAuthorizationHook = func() {
		finalGateAuthorizationHook = originalHook
		writeSnapshotFile(t, repo, "tracked.txt", "racing successor\n")
		successor := newCompactTestState(t, repo, "compact-recovery-race-g2")
		successor.Generation = state.Generation + 1
		request := CompactRecoveryRequest{PredecessorLineageID: state.LineageID, ExpectedPredecessorRevision: predecessor.Revision,
			Successor: successor, Disposition: RecoveryScopeChanged, Reason: "concurrent scope change", Actor: "maintainer"}
		if _, err := RecoverCompactAuthority(context.Background(), repo, request); !errors.Is(err, ErrConcurrentUpdate) {
			t.Fatalf("recovery during final recheck = %v", err)
		}
		writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	}
	got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePreCommit, LineageID: state.LineageID})
	if got.Result != GateAllow {
		t.Fatalf("concurrent recovery evaluation = %#v", got)
	}
}

func TestCompactGateHoldsAuthorityLockThroughAllow(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state, store, receipt := approvedCompactCurrentChangesFixture(t, repo, "compact-allow-lock", []string{})
	gitSnapshot(t, repo, "add", "--", "tracked.txt")
	predecessor, _ := store.Load()
	writeSnapshotFile(t, repo, "tracked.txt", "successor\n")
	successor := newCompactTestState(t, repo, "compact-allow-lock-g2")
	successor.Generation = state.Generation + 1
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	original := finalCompactGateAllowHook
	t.Cleanup(func() { finalCompactGateAllowHook = original })
	finalCompactGateAllowHook = func() {
		_, err := RecoverCompactAuthority(context.Background(), repo, CompactRecoveryRequest{PredecessorLineageID: state.LineageID, ExpectedPredecessorRevision: predecessor.Revision, Successor: successor, Disposition: RecoveryScopeChanged, Reason: "race", Actor: "maintainer"})
		if !errors.Is(err, ErrConcurrentUpdate) {
			t.Fatalf("publication during GateAllow = %v", err)
		}
	}
	if got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePreCommit, LineageID: state.LineageID}); got.Result != GateAllow {
		t.Fatalf("gate result = %#v", got)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(store.Dir), successor.LineageID, "review-state.json")); !os.IsNotExist(err) {
		t.Fatalf("successor published after final check: %v", err)
	}
}

func TestCompactStoreRecoverRejectsIneligibleOrUnprovenPredecessor(t *testing.T) {
	tests := []struct {
		name        string
		disposition RecoveryDisposition
		prepare     func(t *testing.T, repo string, state *CompactState, store CompactStore, revision *string)
		authorizer  string
		want        string
	}{
		{name: "approved without scope change", disposition: RecoveryScopeChanged, want: "scope has not changed"},
		{name: "reviewing", disposition: RecoveryInvalidated, want: "requires an invalidated predecessor"},
		{name: "escalated without authorization", disposition: RecoveryEscalated, authorizer: "", want: "maintainer authorization"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
			var state CompactState
			var store CompactStore
			var revision string
			var err error
			if tt.name == "approved without scope change" {
				state, store, _ = approvedCompactCurrentChangesFixture(t, repo, "recovery-predecessor", []string{})
				record, loadErr := store.Load()
				if loadErr != nil {
					t.Fatal(loadErr)
				}
				revision = record.Revision
			} else {
				state = newCompactTestState(t, repo, "recovery-predecessor")
				store, _ = CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
				revision, err = store.Replace("", "review/start", state)
				if err != nil {
					t.Fatal(err)
				}
			}
			if tt.name == "escalated without authorization" {
				results := make([]LensResult, len(state.SelectedLenses))
				for index, lens := range state.SelectedLenses {
					results[index] = LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"reviewed"}}
				}
				if err = state.CompleteReview(CompactReviewInput{LensResults: results, Classifications: []FindingEvidence{}, RefuterOutcomes: []EvidenceResult{}}); err != nil {
					t.Fatal(err)
				}
				revision, err = store.Replace(revision, "review/complete-review", state)
				if err != nil {
					t.Fatal(err)
				}
				if err = state.CompleteVerification([]byte("failed verification"), false); err != nil {
					t.Fatal(err)
				}
				revision, err = store.Replace(revision, "review/complete-verification", state)
				if err != nil {
					t.Fatal(err)
				}
			}
			successor := newCompactTestState(t, repo, "recovery-successor")
			successor.Generation = state.Generation + 1
			_, err = RecoverCompactAuthority(context.Background(), repo, CompactRecoveryRequest{
				PredecessorLineageID: state.LineageID, ExpectedPredecessorRevision: revision, Successor: successor,
				Disposition: tt.disposition, Reason: "recover authority", Actor: "operator", MaintainerAuthorization: tt.authorizer,
			})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("recovery error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestStartCompactAuthorityBlocksSupersededApprovedPredecessor(t *testing.T) {
	repo := initSnapshotRepo(t)
	predecessor, store, _ := approvedCompactRevisionFixture(t, repo, "compact-start-a")
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "successor\n")
	successor := newCompactTestState(t, repo, "compact-start-b")
	successor.Generation = predecessor.Generation + 1
	if _, err := RecoverCompactAuthority(context.Background(), repo, CompactRecoveryRequest{
		PredecessorLineageID: predecessor.LineageID, ExpectedPredecessorRevision: record.Revision,
		Successor: successor, Disposition: RecoveryScopeChanged, Reason: "scope changed", Actor: "maintainer",
	}); err != nil {
		t.Fatal(err)
	}
	requested := newCompactRevisionState(t, repo, "compact-start-c")
	result, err := StartCompactAuthority(context.Background(), repo, CompactStartRequest{State: requested})
	if err != nil || result.Action != CompactStartCreated || result.Record.State.LineageID != requested.LineageID {
		t.Fatalf("start with superseded unrelated authority = %#v, %v", result, err)
	}
}

func TestStartCompactAuthorityKeepsStagedAndWorkspaceAuthoritiesDistinct(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	gitSnapshot(t, repo, "add", "--", "tracked.txt")

	staged := newCompactStartStateForTarget(t, repo, "compact-start-staged", Target{Kind: TargetCurrentChanges, Projection: ProjectionStaged, IntendedUntracked: []string{}})
	workspace := newCompactStartStateForTarget(t, repo, "compact-start-workspace", Target{Kind: TargetCurrentChanges, Projection: ProjectionWorkspace, IntendedUntracked: []string{}})
	if staged.InitialSnapshot.CandidateTree != workspace.InitialSnapshot.CandidateTree || staged.InitialSnapshot.Identity == workspace.InitialSnapshot.Identity {
		t.Fatalf("projection snapshots do not share tree with distinct identity: staged=%#v workspace=%#v", staged.InitialSnapshot, workspace.InitialSnapshot)
	}
	storeCompactStartAuthority(t, repo, staged)

	created, err := StartCompactAuthority(context.Background(), repo, CompactStartRequest{State: workspace})
	if err != nil || created.Action != CompactStartCreated || created.Record.State.LineageID != workspace.LineageID {
		t.Fatalf("workspace start against staged authority = %#v, %v", created, err)
	}
	replayed, err := StartCompactAuthority(context.Background(), repo, CompactStartRequest{State: staged})
	if err != nil || replayed.Action != CompactStartResumed || replayed.Record.State.LineageID != staged.LineageID {
		t.Fatalf("staged replay = %#v, %v", replayed, err)
	}
}

func TestStartCompactAuthoritySelectsProjectionSpecificBaseDiffAuthorityAfterCommit(t *testing.T) {
	repo := initSnapshotRepo(t)
	base := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	gitSnapshot(t, repo, "add", "--", "tracked.txt")

	staged := newCompactStartStateForTarget(t, repo, "compact-start-staged-base", Target{Kind: TargetCurrentChanges, Projection: ProjectionStaged, IntendedUntracked: []string{}})
	workspace := newCompactStartStateForTarget(t, repo, "compact-start-workspace-base", Target{Kind: TargetCurrentChanges, Projection: ProjectionWorkspace, IntendedUntracked: []string{}})
	if staged.InitialSnapshot.CandidateTree != workspace.InitialSnapshot.CandidateTree {
		t.Fatalf("same candidate tree required for projection selection: staged=%s workspace=%s", staged.InitialSnapshot.CandidateTree, workspace.InitialSnapshot.CandidateTree)
	}
	storeCompactStartAuthority(t, repo, staged)
	storeCompactStartAuthority(t, repo, workspace)
	gitSnapshot(t, repo, "commit", "-m", "deliver candidate")

	for _, tt := range []struct {
		name       string
		projection Projection
		want       string
	}{
		{name: "staged", projection: ProjectionStaged, want: staged.LineageID},
		{name: "workspace", projection: ProjectionWorkspace, want: workspace.LineageID},
	} {
		t.Run(tt.name, func(t *testing.T) {
			requested := newCompactStartStateForTarget(t, repo, "compact-start-"+tt.name+"-base-request", Target{Kind: TargetBaseDiff, Projection: tt.projection, BaseRef: base, IntendedUntracked: []string{}})
			result, err := StartCompactAuthority(context.Background(), repo, CompactStartRequest{State: requested})
			if err != nil || result.Action != CompactStartResumed || result.Record.State.LineageID != tt.want {
				t.Fatalf("%s base-diff authority selection = %#v, %v", tt.name, result, err)
			}
		})
	}
}

func TestCompactStagedCorrectionAcceptsIndexFixDespiteWorkspaceDivergence(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nwrong\n")
	gitSnapshot(t, repo, "add", "--", "tracked.txt")
	state := newCompactStartStateForTarget(t, repo, "compact-staged-correction-accept", Target{Kind: TargetCurrentChanges, Projection: ProjectionStaged, IntendedUntracked: []string{}})
	finding := Finding{ID: "R3-001", Lens: strings.TrimPrefix(state.SelectedLenses[0], "review-"), Location: "tracked.txt:5", Severity: "CRITICAL", Claim: "wrong staged value", ProofRefs: []string{"candidate-only failure"}}
	results := make([]LensResult, len(state.SelectedLenses))
	for index, lens := range state.SelectedLenses {
		results[index] = LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"reviewed"}}
	}
	results[0].Findings = []Finding{finding}
	if err := state.CompleteReview(CompactReviewInput{LensResults: results, Classifications: []FindingEvidence{{FindingID: finding.ID, Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "changed hunk"}}, RefuterOutcomes: []EvidenceResult{}}); err != nil {
		t.Fatal(err)
	}
	if err := state.BeginCorrection(2); err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfixed\n")
	gitSnapshot(t, repo, "add", "--", "tracked.txt")
	writeSnapshotFile(t, repo, "tracked.txt", "unstaged workspace divergence\n")

	fix, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetFixDiff, Projection: ProjectionStaged, BaseRef: state.CurrentSnapshot.CandidateTree, IntendedUntracked: []string{}, LedgerIDs: state.FixFindingIDs})
	if err != nil {
		t.Fatal(err)
	}
	if got := gitSnapshot(t, repo, "show", fix.CandidateTree+":tracked.txt"); got != "base\none\ntwo\nthree\nfixed\n" {
		t.Fatalf("staged correction candidate = %q", got)
	}
	fixHash := FixDeltaHashForSnapshot(fix)
	validation := ScopedValidationResult{LedgerIDs: state.FixFindingIDs, FixCausedFindings: []Finding{}, FollowUps: []FollowUp{}, OriginalCriteria: ValidationCheck{Passed: true, EvidenceHash: hash("2"), FixDeltaHash: fixHash}, CorrectionRegression: ValidationCheck{Passed: true, EvidenceHash: hash("3"), FixDeltaHash: fixHash}}
	if err := state.CompleteCorrection(fix, 2, validation); err != nil {
		t.Fatalf("CompleteCorrection(staged fix) error = %v", err)
	}
	if state.State != StateValidating {
		t.Fatalf("staged correction state = %#v", state)
	}
}

func TestCompactStagedCorrectionRejectsWorkspaceSnapshotWithoutMutatingState(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "base\nwrong\n")
	gitSnapshot(t, repo, "add", "--", "tracked.txt")
	state := newCompactStartStateForTarget(t, repo, "compact-staged-correction", Target{Kind: TargetCurrentChanges, Projection: ProjectionStaged, IntendedUntracked: []string{}})
	finding := Finding{ID: "R3-001", Lens: strings.TrimPrefix(state.SelectedLenses[0], "review-"), Location: "tracked.txt:2", Severity: "CRITICAL", Claim: "wrong staged value", ProofRefs: []string{"candidate-only failure"}}
	results := make([]LensResult, len(state.SelectedLenses))
	for index, lens := range state.SelectedLenses {
		results[index] = LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"reviewed"}}
	}
	results[0].Findings = []Finding{finding}
	if err := state.CompleteReview(CompactReviewInput{LensResults: results, Classifications: []FindingEvidence{{FindingID: finding.ID, Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "changed hunk"}}, RefuterOutcomes: []EvidenceResult{}}); err != nil {
		t.Fatal(err)
	}
	if err := state.BeginCorrection(1); err != nil {
		t.Fatal(err)
	}
	before := state
	writeSnapshotFile(t, repo, "tracked.txt", "base\nfixed\n")
	workspaceFix, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetFixDiff, BaseRef: state.CurrentSnapshot.CandidateTree, IntendedUntracked: []string{}, LedgerIDs: state.FixFindingIDs})
	if err != nil {
		t.Fatal(err)
	}
	fixHash := FixDeltaHashForSnapshot(workspaceFix)
	validation := ScopedValidationResult{LedgerIDs: state.FixFindingIDs, FixCausedFindings: []Finding{}, FollowUps: []FollowUp{}, OriginalCriteria: ValidationCheck{Passed: true, EvidenceHash: hash("2"), FixDeltaHash: fixHash}, CorrectionRegression: ValidationCheck{Passed: true, EvidenceHash: hash("3"), FixDeltaHash: fixHash}}
	if err := state.CompleteCorrection(workspaceFix, 1, validation); err == nil || !strings.Contains(err.Error(), "projection") {
		t.Fatalf("workspace correction error = %v", err)
	}
	if !reflect.DeepEqual(state, before) {
		t.Fatalf("rejected workspace correction mutated staged state:\nbefore=%#v\nafter=%#v", before, state)
	}

	stagedFix, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetFixDiff, Projection: ProjectionStaged, BaseRef: state.CurrentSnapshot.CandidateTree, IntendedUntracked: []string{}, LedgerIDs: state.FixFindingIDs})
	if err != nil {
		t.Fatal(err)
	}
	fixHash = FixDeltaHashForSnapshot(stagedFix)
	validation.OriginalCriteria.FixDeltaHash, validation.CorrectionRegression.FixDeltaHash = fixHash, fixHash
	if err := state.CompleteCorrection(stagedFix, 0, validation); err == nil || !strings.Contains(err.Error(), "unchanged candidate") {
		t.Fatalf("unchanged staged correction error = %v", err)
	}
	if !reflect.DeepEqual(state, before) {
		t.Fatalf("rejected unchanged staged correction mutated state:\nbefore=%#v\nafter=%#v", before, state)
	}
}

func TestStartCompactAuthorityConcurrentlyConvergesOnOneEquivalentAuthority(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "historical candidate one\n")
	storeCompactStartAuthority(t, repo, newCompactTestState(t, repo, "compact-start-history-one"))
	writeSnapshotFile(t, repo, "tracked.txt", "historical candidate two\n")
	storeCompactStartAuthority(t, repo, newCompactTestState(t, repo, "compact-start-history-two"))
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	first := newCompactTestState(t, repo, "compact-start-first")
	second := first
	second.LineageID = "compact-start-second"

	type outcome struct {
		result CompactStartResult
		err    error
	}
	results := make(chan outcome, 2)
	for _, state := range []CompactState{first, second} {
		go func(state CompactState) {
			result, err := StartCompactAuthority(context.Background(), repo, CompactStartRequest{State: state})
			results <- outcome{result: result, err: err}
		}(state)
	}
	one, two := <-results, <-results
	if one.err != nil || two.err != nil {
		t.Fatalf("concurrent starts = %#v, %#v", one, two)
	}
	if one.result.Record.State.LineageID != two.result.Record.State.LineageID || one.result.Record.Revision != two.result.Record.Revision {
		t.Fatalf("concurrent starts diverged: %#v, %#v", one.result, two.result)
	}
	actions := map[CompactStartAction]int{one.result.Action: 1, two.result.Action: 1}
	if actions[CompactStartCreated] != 1 || actions[CompactStartResumed] != 1 {
		t.Fatalf("concurrent start actions = %#v", actions)
	}
	leaves, err := CompactAuthorityLeaves(context.Background(), repo)
	if err != nil || len(leaves) != 3 {
		t.Fatalf("concurrent start leaves = %#v, %v", leaves, err)
	}
}

func TestStartCompactAuthorityBlocksExistingLineageForUnrelatedTarget(t *testing.T) {
	repo := initSnapshotRepo(t)
	existing, store, _ := approvedCompactRevisionFixture(t, repo, "compact-start-existing-lineage")
	before, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	statePayload, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	receiptPayload, err := os.ReadFile(store.ReceiptPath())
	if err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "unrelated candidate\n")
	result, err := StartCompactAuthority(context.Background(), repo, CompactStartRequest{State: newCompactTestState(t, repo, existing.LineageID)})
	if err != nil || result.Action != CompactStartBlocked || result.Record.Revision != before.Revision {
		t.Fatalf("start against existing unrelated lineage = %#v, %v", result, err)
	}
	if payload, readErr := os.ReadFile(store.StatePath()); readErr != nil || !bytes.Equal(payload, statePayload) {
		t.Fatalf("existing lineage state changed: %q, %v", payload, readErr)
	}
	if payload, readErr := os.ReadFile(store.ReceiptPath()); readErr != nil || !bytes.Equal(payload, receiptPayload) {
		t.Fatalf("existing lineage receipt changed: %q, %v", payload, readErr)
	}
}

func TestStartCompactAuthorityCreatesWithUnrelatedHistoricalLeaves(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "historical candidate one\n")
	first := newCompactTestState(t, repo, "compact-start-original")
	firstStore, err := CompactAuthoritativeStore(context.Background(), repo, first.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := firstStore.Replace("", "review/start", first); err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "historical candidate two\n")
	second := newCompactTestState(t, repo, "compact-start-unrelated")
	secondStore, err := CompactAuthoritativeStore(context.Background(), repo, second.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := secondStore.Replace("", "review/start", second); err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "new candidate\n")
	requested := newCompactTestState(t, repo, "compact-start-new")

	result, err := StartCompactAuthority(context.Background(), repo, CompactStartRequest{State: requested})
	if err != nil || result.Action != CompactStartCreated || result.Record.State.LineageID != requested.LineageID {
		t.Fatalf("start with unrelated historical leaves = %#v, %v", result, err)
	}
	replay, err := StartCompactAuthority(context.Background(), repo, CompactStartRequest{State: requested})
	if err != nil || replay.Action != CompactStartResumed || replay.Record.Revision != result.Record.Revision {
		t.Fatalf("exact start replay = %#v, %v", replay, err)
	}
	leaves, err := CompactAuthorityLeaves(context.Background(), repo)
	if err != nil || len(leaves) != 3 {
		t.Fatalf("start leaves = %#v, %v", leaves, err)
	}
}

func TestStartCompactAuthorityResumesMatchingAuthorityAmongUnrelatedLeaves(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "unrelated one\n")
	storeCompactStartAuthority(t, repo, newCompactTestState(t, repo, "compact-start-unrelated-one"))
	writeSnapshotFile(t, repo, "tracked.txt", "requested candidate\n")
	existing := newCompactTestState(t, repo, "compact-start-matching")
	storeCompactStartAuthority(t, repo, existing)
	writeSnapshotFile(t, repo, "tracked.txt", "unrelated two\n")
	storeCompactStartAuthority(t, repo, newCompactTestState(t, repo, "compact-start-unrelated-two"))
	writeSnapshotFile(t, repo, "tracked.txt", "requested candidate\n")
	requested := newCompactTestState(t, repo, "compact-start-replay")

	resumed, err := StartCompactAuthority(context.Background(), repo, CompactStartRequest{State: requested})
	if err != nil || resumed.Action != CompactStartResumed || resumed.Record.State.LineageID != existing.LineageID {
		t.Fatalf("resume matching authority = %#v, %v", resumed, err)
	}
	conflicting := requested
	conflicting.PolicyHash = hash("2")
	blocked, err := StartCompactAuthority(context.Background(), repo, CompactStartRequest{State: conflicting})
	if err != nil || blocked.Action != CompactStartBlocked || blocked.Record.State.LineageID != existing.LineageID {
		t.Fatalf("same candidate metadata conflict = %#v, %v", blocked, err)
	}
}

func TestStartCompactAuthorityReusesApprovedReceiptAmongUnrelatedLeaves(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "approved candidate\n")
	approved, _, _ := approvedCompactCurrentChangesFixture(t, repo, "compact-start-approved", []string{})
	writeSnapshotFile(t, repo, "tracked.txt", "unrelated candidate\n")
	storeCompactStartAuthority(t, repo, newCompactTestState(t, repo, "compact-start-approved-unrelated"))
	writeSnapshotFile(t, repo, "tracked.txt", "approved candidate\n")
	requested := newCompactTestState(t, repo, "compact-start-approved-request")

	result, err := StartCompactAuthority(context.Background(), repo, CompactStartRequest{State: requested})
	if err != nil || result.Action != CompactStartReuseReceipt || result.Record.State.LineageID != approved.LineageID {
		t.Fatalf("reuse approved receipt = %#v, %v", result, err)
	}
}

func TestStartCompactAuthorityReusesPersistedV2ApprovedReceipt(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "approved candidate\n")
	approved, store, receipt := approvedCompactCurrentChangesFixture(t, repo, "compact-start-v2-approved", []string{})
	approved.Schema = LegacyCompactStateSchema
	_, payload, err := makeCompactRecord(approved)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.StatePath(), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	receipt.Schema, receipt.FinalScope = LegacyCompactReceiptSchema, nil
	if err := WriteCompactReceiptAtomic(store.ReceiptPath(), receipt); err != nil {
		t.Fatal(err)
	}

	result, err := StartCompactAuthority(context.Background(), repo, CompactStartRequest{State: newCompactTestState(t, repo, "compact-start-v2-request")})
	if err != nil || result.Action != CompactStartReuseReceipt || result.Record.State.LineageID != approved.LineageID {
		t.Fatalf("reuse persisted v2 approved receipt = %#v, %v", result, err)
	}
}

func TestStartCompactAuthorityResumesAuthorizedCorrectionContinuation(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "base\nwrong\n")
	state := newCompactTestState(t, repo, "compact-start-correction")
	store := storeCompactStartAuthority(t, repo, state)
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	finding := Finding{ID: "R3-001", Lens: "reliability", Location: "tracked.txt:2", Severity: "CRITICAL", Claim: "wrong value", ProofRefs: []string{"candidate-only failure"}}
	if err := state.CompleteReview(CompactReviewInput{LensResults: []LensResult{{Lens: LensReliability, Findings: []Finding{finding}, Evidence: []string{"reviewed"}}}, Classifications: []FindingEvidence{{FindingID: finding.ID, Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "changed hunk causes failure"}}, RefuterOutcomes: []EvidenceResult{}}); err != nil {
		t.Fatal(err)
	}
	revision, err := store.Replace(record.Revision, "review/complete-review", state)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.BeginCorrection(1); err != nil {
		t.Fatal(err)
	}
	revision, err = store.Replace(revision, "review/begin-fix", state)
	if err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "base\n")
	fix, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetFixDiff, BaseRef: state.CurrentSnapshot.CandidateTree, IntendedUntracked: state.InitialSnapshot.IntendedUntracked, LedgerIDs: state.FixFindingIDs})
	if err != nil {
		t.Fatal(err)
	}
	fixHash := FixDeltaHashForSnapshot(fix)
	if err := state.CompleteCorrection(fix, 1, ScopedValidationResult{LedgerIDs: state.FixFindingIDs, FixCausedFindings: []Finding{}, FollowUps: []FollowUp{}, OriginalCriteria: ValidationCheck{Passed: false, EvidenceHash: hash("2"), FixDeltaHash: fixHash}, CorrectionRegression: ValidationCheck{Passed: false, EvidenceHash: hash("3"), FixDeltaHash: fixHash}}); err != nil {
		t.Fatal(err)
	}
	revision, err = store.Replace(revision, "review/complete-fix", state)
	if err != nil {
		t.Fatal(err)
	}
	if state.ProposedCorrectionLines != nil || state.CurrentSnapshot.CandidateTree != fix.CandidateTree {
		t.Fatalf("failed correction state = %#v", state)
	}
	requested := newCompactTestState(t, repo, "compact-start-correction-request")
	result, err := StartCompactAuthority(context.Background(), repo, CompactStartRequest{State: requested})
	if err != nil || result.Action != CompactStartResumed || result.Record.State.LineageID != state.LineageID || result.Record.Revision != revision {
		t.Fatalf("authorized correction continuation = %#v, %v", result, err)
	}
	leaves, err := CompactAuthorityLeaves(context.Background(), repo)
	if err != nil || len(leaves) != 1 {
		t.Fatalf("correction replay leaves = %#v, %v", leaves, err)
	}
}

func TestStartCompactAuthorityCreatesForSameCandidateWithDifferentBaseAndPathScope(t *testing.T) {
	repo := initSnapshotRepo(t)
	base := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	writeSnapshotFile(t, repo, "tracked.txt", "committed candidate\n")
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "candidate")

	existing := newCompactStartStateForTarget(t, repo, "compact-start-base-scope-existing", Target{Kind: TargetBaseDiff, BaseRef: base})
	storeCompactStartAuthority(t, repo, existing)
	requested := newCompactTestState(t, repo, "compact-start-base-scope-request")

	result, err := StartCompactAuthority(context.Background(), repo, CompactStartRequest{State: requested})
	if err != nil || result.Action != CompactStartCreated || result.Record.State.LineageID != requested.LineageID {
		t.Fatalf("same candidate with different base/path scope = %#v, %v", result, err)
	}
}

func TestStartCompactAuthorityCreatesForSameCandidateWithDifferentIntendedProof(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "other.txt", "tracked\n")
	gitSnapshot(t, repo, "add", "other.txt")
	gitSnapshot(t, repo, "commit", "-m", "add another tracked file")

	existing := newCompactStartStateForTarget(t, repo, "compact-start-intended-existing", Target{Kind: TargetBaseDiff, BaseRef: "HEAD", IntendedUntracked: []string{"tracked.txt"}})
	storeCompactStartAuthority(t, repo, existing)
	requested := newCompactStartStateForTarget(t, repo, "compact-start-intended-request", Target{Kind: TargetBaseDiff, BaseRef: "HEAD", IntendedUntracked: []string{"other.txt"}})

	result, err := StartCompactAuthority(context.Background(), repo, CompactStartRequest{State: requested})
	if err != nil || result.Action != CompactStartCreated || result.Record.State.LineageID != requested.LineageID {
		t.Fatalf("same candidate with different intended identity = %#v, %v", result, err)
	}
}

func TestStartCompactAuthorityResumesEquivalentCurrentChangesAndBaseDiff(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "new.txt", "candidate\n")
	existing := newCompactTestStateWithIntended(t, repo, "compact-start-current-existing", []string{"new.txt"})
	storeCompactStartAuthority(t, repo, existing)
	requested := newCompactStartStateForTarget(t, repo, "compact-start-base-diff-request", Target{Kind: TargetBaseDiff, BaseRef: "HEAD", IntendedUntracked: []string{"new.txt"}})

	result, err := StartCompactAuthority(context.Background(), repo, CompactStartRequest{State: requested})
	if err != nil || result.Action != CompactStartResumed || result.Record.State.LineageID != existing.LineageID {
		t.Fatalf("equivalent current-changes/base-diff start = %#v, %v", result, err)
	}
}

func TestStartCompactAuthorityReusesCommittedCorrectedReceipt(t *testing.T) {
	repo := initSnapshotRepo(t)
	state := correctedCompactTestState(t, repo, "compact-start-corrected-approved")
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
	if err := os.WriteFile(store.StatePath(), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	receipt, err := state.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteCompactReceiptAtomic(store.ReceiptPath(), receipt); err != nil {
		t.Fatal(err)
	}
	if record.State.State != StateApproved {
		t.Fatalf("corrected fixture state = %s", record.State.State)
	}
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "corrected candidate")
	requested := newCompactStartStateForTarget(t, repo, "compact-start-corrected-request", Target{Kind: TargetBaseDiff, BaseRef: state.InitialSnapshot.BaseTree, IntendedUntracked: state.InitialSnapshot.IntendedUntracked})

	result, err := StartCompactAuthority(context.Background(), repo, CompactStartRequest{State: requested})
	if err != nil || result.Action != CompactStartReuseReceipt || result.Record.State.LineageID != state.LineageID {
		t.Fatalf("committed corrected receipt reuse = %#v, %v", result, err)
	}
}

func TestStartCompactAuthorityBlocksMultipleMatchingAuthorities(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "shared candidate\n")
	first := newCompactTestState(t, repo, "compact-start-shared-one")
	storeCompactStartAuthority(t, repo, first)
	second := first
	second.LineageID = "compact-start-shared-two"
	storeCompactStartAuthority(t, repo, second)
	requested := first
	requested.LineageID = "compact-start-shared-request"

	result, err := StartCompactAuthority(context.Background(), repo, CompactStartRequest{State: requested})
	if err != nil || result.Action != CompactStartBlocked || result.Record.State.LineageID != first.LineageID {
		t.Fatalf("multiple matching authorities = %#v, %v", result, err)
	}
	leaves, err := CompactAuthorityLeaves(context.Background(), repo)
	if err != nil || len(leaves) != 2 {
		t.Fatalf("multiple matching authorities created a lineage: %#v, %v", leaves, err)
	}
}

func TestStartCompactAuthorityBlocksInvalidReceiptAndCorruptUnrelatedStore(t *testing.T) {
	t.Run("invalid approved receipt", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		_, store, _ := approvedCompactRevisionFixture(t, repo, "compact-start-invalid-receipt")
		if err := os.WriteFile(store.ReceiptPath(), []byte("invalid\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		result, err := StartCompactAuthority(context.Background(), repo, CompactStartRequest{State: newCompactRevisionState(t, repo, "compact-start-invalid-receipt-request")})
		if err != nil || result.Action != CompactStartBlocked || result.Record.State.LineageID != "compact-start-invalid-receipt" {
			t.Fatalf("invalid receipt start = %#v, %v", result, err)
		}
	})
	t.Run("corrupt unrelated store", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		writeSnapshotFile(t, repo, "tracked.txt", "historical candidate\n")
		store := storeCompactStartAuthority(t, repo, newCompactTestState(t, repo, "compact-start-corrupt-history"))
		if err := os.WriteFile(store.StatePath(), []byte("{"), 0o644); err != nil {
			t.Fatal(err)
		}
		writeSnapshotFile(t, repo, "tracked.txt", "new candidate\n")
		if _, err := StartCompactAuthority(context.Background(), repo, CompactStartRequest{State: newCompactTestState(t, repo, "compact-start-corrupt-request")}); err == nil {
			t.Fatal("corrupt unrelated store allowed a fresh authority")
		}
	})
}

func TestCompactStoreReplacesCurrentStateWithCASAndExactRetry(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newCompactTestState(t, repo, "compact-cas")
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.Replace("", "review/start", state)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(store.Dir, "events")); !os.IsNotExist(err) {
		t.Fatalf("compact store created event history: %v", err)
	}
	results := make([]LensResult, len(state.SelectedLenses))
	for index, lens := range state.SelectedLenses {
		results[index] = LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"review completed"}}
	}
	if err := state.CompleteReview(CompactReviewInput{LensResults: results, Classifications: []FindingEvidence{}, RefuterOutcomes: []EvidenceResult{}}); err != nil {
		t.Fatal(err)
	}
	second, err := store.Replace(first, "review/complete-review", state)
	if err != nil || second == first {
		t.Fatalf("compact replacement = %q, %v", second, err)
	}
	if retry, err := store.Replace(first, "review/complete-review", state); err != nil || retry != second {
		t.Fatalf("exact compact retry = %q, %v", retry, err)
	}
	forged := state
	forged.PolicyHash = hash("f")
	if _, err := store.Replace(first, "review/complete-review", forged); !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("stale expected revision error = %v", err)
	}
	if _, err := store.Replace(second, "review/complete-verification", forged); !errors.Is(err, ErrInvalidSuccessor) {
		t.Fatalf("illegal compact successor error = %v", err)
	}
	loaded, err := store.Load()
	if err != nil || loaded.Revision != second || !compactStateEqual(loaded.State, state) {
		t.Fatalf("loaded compact authority = %#v, %v", loaded, err)
	}
}

func TestCompactCorrectionRetriesWithinFrozenBudgetAndFindingScope(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfour\n")
	state := newCompactTestState(t, repo, "compact-iterative-correction")
	finding := Finding{ID: "R3-001", Location: "tracked.txt:5", Severity: "CRITICAL", Claim: "wrong value", ProofRefs: []string{"candidate-only failure"}}
	result := LensResult{Lens: state.SelectedLenses[0], Findings: []Finding{finding}, Evidence: []string{"reviewed once"}}
	if err := state.CompleteReview(CompactReviewInput{LensResults: []LensResult{result}, Classifications: []FindingEvidence{{FindingID: finding.ID, Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "changed hunk"}}, RefuterOutcomes: []EvidenceResult{}}); err != nil {
		t.Fatal(err)
	}
	initialLenses := append([]LensResult(nil), state.LensResults...)

	complete := func(content string, passed bool) error {
		if err := state.BeginCorrection(1); err != nil {
			return err
		}
		writeSnapshotFile(t, repo, "tracked.txt", content)
		fix, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetFixDiff, BaseRef: state.CurrentSnapshot.CandidateTree, IntendedUntracked: state.InitialSnapshot.IntendedUntracked, LedgerIDs: state.FixFindingIDs})
		if err != nil {
			return err
		}
		fixHash := FixDeltaHashForSnapshot(fix)
		return state.CompleteCorrection(fix, 1, ScopedValidationResult{LedgerIDs: []string{finding.ID}, FixCausedFindings: []Finding{}, FollowUps: []FollowUp{},
			OriginalCriteria: ValidationCheck{EvidenceHash: hash("2"), FixDeltaHash: fixHash, Passed: passed}, CorrectionRegression: ValidationCheck{EvidenceHash: hash("3"), FixDeltaHash: fixHash, Passed: passed}})
	}
	if err := complete("base\none\ntwo\nthree\nfirst-fix\n", false); err != nil {
		t.Fatal(err)
	}
	if state.State != StateCorrectionRequired || state.CumulativeCorrectionLines != 1 || len(state.CorrectionAttempts) != 1 {
		t.Fatalf("failed attempt state = %#v", state)
	}
	if err := complete("base\none\ntwo\nthree\nfixed\n", true); err != nil {
		t.Fatal(err)
	}
	if state.State != StateValidating || state.CumulativeCorrectionLines != 2 || len(state.CorrectionAttempts) != 2 || !reflect.DeepEqual(state.LensResults, initialLenses) || !reflect.DeepEqual(state.FixFindingIDs, []string{finding.ID}) {
		t.Fatalf("successful retry state = %#v", state)
	}
	before := state
	state.State, state.ProposedCorrectionLines = StateCorrectionRequired, nil
	if err := state.BeginCorrection(state.CorrectionBudget); err != nil || state.State != StateEscalated {
		t.Fatalf("cumulative overflow = %#v, %v", state, err)
	}
	state = before
}

func TestCompactZeroLineFailuresReachAttemptCap(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfour\n")
	state := newCompactTestState(t, repo, "compact-zero-attempt-cap")
	finding := Finding{ID: "R3-001", Location: "tracked.txt:5", Severity: "CRITICAL", Claim: "wrong", ProofRefs: []string{"proof"}}
	if err := state.CompleteReview(CompactReviewInput{LensResults: []LensResult{{Lens: state.SelectedLenses[0], Findings: []Finding{finding}, Evidence: []string{"reviewed"}}}, Classifications: []FindingEvidence{{FindingID: finding.ID, Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "proof"}}, RefuterOutcomes: []EvidenceResult{}}); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < MaxCompactCorrectionAttempts; attempt++ {
		if err := state.BeginCorrection(1); err != nil {
			t.Fatal(err)
		}
		mode := os.FileMode(0o755)
		if attempt%2 != 0 {
			mode = 0o644
		}
		if err := os.Chmod(filepath.Join(repo, "tracked.txt"), mode); err != nil {
			t.Fatal(err)
		}
		fix, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetFixDiff, BaseRef: state.CurrentSnapshot.CandidateTree, IntendedUntracked: state.InitialSnapshot.IntendedUntracked, LedgerIDs: state.FixFindingIDs})
		if err != nil {
			t.Fatal(err)
		}
		fixHash := FixDeltaHashForSnapshot(fix)
		validation := ScopedValidationResult{LedgerIDs: state.FixFindingIDs, FixCausedFindings: []Finding{}, FollowUps: []FollowUp{}, OriginalCriteria: ValidationCheck{EvidenceHash: hash("2"), FixDeltaHash: fixHash}, CorrectionRegression: ValidationCheck{EvidenceHash: hash("3"), FixDeltaHash: fixHash}}
		if err := state.CompleteCorrection(fix, 0, validation); err != nil {
			t.Fatal(err)
		}
	}
	if state.State != StateEscalated || len(state.CorrectionAttempts) != MaxCompactCorrectionAttempts {
		t.Fatalf("zero-line cap state = %#v", state)
	}
}

func TestCompactStoreFailsClosedForCorruptionAndIgnoresInvalidTempState(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newCompactTestState(t, repo, "compact-corruption")
	store, _ := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	revision, err := store.Replace("", "review/start", state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.Dir, ".atomic-interrupted"), []byte("not authority"), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil || loaded.Revision != revision {
		t.Fatalf("invalid temp displaced authority: %#v, %v", loaded, err)
	}
	payload, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	var record map[string]any
	if err := json.Unmarshal(payload, &record); err != nil {
		t.Fatal(err)
	}
	record["revision"] = hash("a")
	corrupt, _ := json.Marshal(record)
	if err := os.WriteFile(store.StatePath(), corrupt, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("corrupt compact state error = %v", err)
	}
}

func TestCompactDiscoveryIgnoresOnlyUnpublishedCrashResidue(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newCompactTestState(t, repo, "compact-published")
	store, _ := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if _, err := store.Replace("", "review/start", state); err != nil {
		t.Fatal(err)
	}
	residue, _ := CompactAuthoritativeStore(context.Background(), repo, "compact-crash-residue")
	if err := os.MkdirAll(residue.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(residue.Dir, ".atomic-interrupted"), []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	leaves, err := CompactAuthorityLeaves(context.Background(), repo)
	if err != nil || len(leaves) != 1 || leaves[0].lineageID != state.LineageID {
		t.Fatalf("leaves with crash residue = %#v, %v", leaves, err)
	}
	if err := os.WriteFile(residue.StatePath(), []byte("corrupt published authority"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CompactAuthorityLeaves(context.Background(), repo); err == nil {
		t.Fatal("corrupt published authority was hidden as residue")
	}
}

func TestCompactActualCumulativeOverflowPersistsTerminalAttempt(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfour\n")
	state := correctedCompactTestState(t, repo, "compact-cumulative-overflow")
	prior := CompactCorrectionAttempt{Snapshot: state.CurrentSnapshot, ProposedLines: 1, ActualLines: state.CorrectionBudget - 1, FixDeltaHash: state.FixDeltaHash, OriginalCriteria: *state.OriginalCriteria, CorrectionRegression: *state.CorrectionRegression}
	state.State, state.EvidenceHash = StateCorrectionRequired, ""
	state.CorrectionAttempts, state.CumulativeCorrectionLines = []CompactCorrectionAttempt{prior}, state.CorrectionBudget-1
	state.FixDeltaHash, state.ActualCorrectionLines = EmptyFixDeltaHash, nil
	state.OriginalCriteria, state.CorrectionRegression, state.ProposedCorrectionLines = nil, nil, nil
	if err := state.BeginCorrection(1); err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nchanged\nexpanded\n")
	fix, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetFixDiff, BaseRef: state.CurrentSnapshot.CandidateTree, IntendedUntracked: state.InitialSnapshot.IntendedUntracked, LedgerIDs: state.FixFindingIDs})
	if err != nil {
		t.Fatal(err)
	}
	actual, _ := (SnapshotBuilder{Repo: repo}).ChangedLines(context.Background(), fix)
	fixHash := FixDeltaHashForSnapshot(fix)
	validation := ScopedValidationResult{LedgerIDs: state.FixFindingIDs, FixCausedFindings: []Finding{}, FollowUps: []FollowUp{}, OriginalCriteria: ValidationCheck{EvidenceHash: hash("2"), FixDeltaHash: fixHash, Passed: true}, CorrectionRegression: ValidationCheck{EvidenceHash: hash("3"), FixDeltaHash: fixHash, Passed: true}}
	if err := state.CompleteCorrection(fix, actual, validation); err != nil {
		t.Fatal(err)
	}
	if state.State != StateEscalated || state.CumulativeCorrectionLines <= state.CorrectionBudget || len(state.CorrectionAttempts) != 2 {
		t.Fatalf("overflow state = %#v", state)
	}
	_, payload, err := makeCompactRecord(state)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseCompactRecord(payload, state.LineageID); err != nil {
		t.Fatalf("persisted overflow record: %v", err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfixed\n")
	if err := state.BeginCorrection(1); err == nil {
		t.Fatal("overflow lineage resumed after reducing the diff")
	}
}

func TestCompactStoreRejectsForgedServiceTokenRiskDowngrade(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "neutral/service-token.ts", "export const token = 'candidate'\n")
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{"neutral/service-token.ts"}})
	if err != nil {
		t.Fatal(err)
	}
	lines, err := (SnapshotBuilder{Repo: repo}).ChangedLines(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewCompactState(Start{
		LineageID: "compact-service-token-forgery", Mode: ModeOrdinaryBounded, Generation: 1,
		Snapshot: snapshot, PolicyHash: hash("1"), RiskLevel: RiskMedium,
		SelectedLenses: []string{LensReliability}, OriginalChangedLines: &lines,
	})
	if err != nil {
		t.Fatal(err)
	}
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace("", "review/start", state); err == nil || !errors.Is(err, ErrInvalidSuccessor) {
		t.Fatalf("forged medium service-token state error = %v, want invalid successor", err)
	}
	for _, lenses := range [][]string{{LensRisk}, {LensReliability, LensReadability, LensResilience, LensRisk}} {
		if _, err := NewCompactState(Start{
			LineageID: "compact-service-token-invalid-high", Mode: ModeOrdinaryBounded, Generation: 1,
			Snapshot: snapshot, PolicyHash: hash("1"), RiskLevel: RiskHigh,
			SelectedLenses: lenses, OriginalChangedLines: &lines,
		}); err == nil {
			t.Fatalf("invalid high-risk lenses %v were accepted", lenses)
		}
	}
}

func TestCompactStateRejectsChecksumValidImpossibleSemantics(t *testing.T) {
	repo := initSnapshotRepo(t)
	valid := correctedCompactTestState(t, repo, "compact-semantic-invalid")
	clean := valid
	clean.LensResults = append([]LensResult(nil), valid.LensResults...)
	clean.LensResults[0].Findings = append([]Finding(nil), valid.LensResults[0].Findings...)
	clean.CurrentSnapshot = clean.InitialSnapshot
	clean.FixDeltaHash = EmptyFixDeltaHash
	clean.FixFindingIDs = []string{}
	clean.Classifications = map[string]FindingEvidence{}
	clean.Outcomes = map[string]EvidenceOutcome{}
	clean.Findings = []Finding{}
	clean.LensResults[0].Findings = []Finding{}
	clean.LensResults[0].ResultHash = LensResultHash(clean.LensResults[0])
	clean.ProposedCorrectionLines = nil
	clean.ActualCorrectionLines = nil
	clean.OriginalCriteria = nil
	clean.CorrectionRegression = nil

	tests := []struct {
		name   string
		mutate func(*CompactState)
	}{
		{name: "findings differ from lens concatenation", mutate: func(state *CompactState) { state.Findings = []Finding{} }},
		{name: "severe classification missing", mutate: func(state *CompactState) { delete(state.Classifications, state.FixFindingIDs[0]) }},
		{name: "severe outcome missing", mutate: func(state *CompactState) { delete(state.Outcomes, state.FixFindingIDs[0]) }},
		{name: "unsupported evidence class", mutate: func(state *CompactState) {
			item := state.Classifications[state.FixFindingIDs[0]]
			item.Class = EvidenceClass("invented")
			state.Classifications[state.FixFindingIDs[0]] = item
		}},
		{name: "unsupported outcome", mutate: func(state *CompactState) { state.Outcomes[state.FixFindingIDs[0]] = EvidenceOutcome("invented") }},
		{name: "corroborated causal finding omitted from fix IDs", mutate: func(state *CompactState) { state.FixFindingIDs = []string{} }},
		{name: "arbitrary fix delta hash", mutate: func(state *CompactState) { state.FixDeltaHash = hash("f") }},
		{name: "approved correction has no completed correction", mutate: func(state *CompactState) {
			state.CurrentSnapshot = state.InitialSnapshot
			state.FixDeltaHash = EmptyFixDeltaHash
			state.ProposedCorrectionLines = nil
			state.ActualCorrectionLines = nil
			state.OriginalCriteria = nil
			state.CorrectionRegression = nil
		}},
		{name: "corrected state uses wrong fix base", mutate: func(state *CompactState) { state.CurrentSnapshot.BaseTree = state.InitialSnapshot.BaseTree }},
		{name: "corrected state uses wrong ledger IDs", mutate: func(state *CompactState) { state.CurrentSnapshot.LedgerIDs = []string{"OTHER"} }},
		{name: "approved correction has failed targeted check", mutate: func(state *CompactState) { state.OriginalCriteria.Passed = false }},
		{name: "unknown causality is not escalated", mutate: func(state *CompactState) {
			*state = clean
			finding := valid.Findings[0]
			state.Findings = []Finding{finding}
			state.LensResults[0].Findings = []Finding{finding}
			state.LensResults[0].ResultHash = LensResultHash(state.LensResults[0])
			state.Classifications = map[string]FindingEvidence{finding.ID: {FindingID: finding.ID, Class: EvidenceDeterministic, Causality: CausalUnknown, Proof: "causality is unresolved"}}
			state.Outcomes = map[string]EvidenceOutcome{finding.ID: OutcomeInconclusive}
		}},
		{name: "insufficient evidence is not escalated", mutate: func(state *CompactState) {
			*state = clean
			finding := valid.Findings[0]
			state.Findings = []Finding{finding}
			state.LensResults[0].Findings = []Finding{finding}
			state.LensResults[0].ResultHash = LensResultHash(state.LensResults[0])
			state.Classifications = map[string]FindingEvidence{finding.ID: {FindingID: finding.ID, Class: EvidenceInsufficient, Causality: CausalIntroduced, Proof: "evidence remains insufficient"}}
			state.Outcomes = map[string]EvidenceOutcome{finding.ID: OutcomeInconclusive}
		}},
		{name: "non-severe finding enters correction", mutate: func(state *CompactState) {
			state.Findings[0].Severity = "INFO"
			state.LensResults[0].Findings[0].Severity = "INFO"
			state.LensResults[0].ResultHash = LensResultHash(state.LensResults[0])
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := valid
			state.LensResults = append([]LensResult(nil), valid.LensResults...)
			state.LensResults[0].Findings = append([]Finding(nil), valid.LensResults[0].Findings...)
			state.Findings = append([]Finding(nil), valid.Findings...)
			state.Classifications = cloneClassifications(valid.Classifications)
			state.Outcomes = cloneOutcomes(valid.Outcomes)
			state.FixFindingIDs = append([]string(nil), valid.FixFindingIDs...)
			if valid.OriginalCriteria != nil {
				original, regression := *valid.OriginalCriteria, *valid.CorrectionRegression
				state.OriginalCriteria, state.CorrectionRegression = &original, &regression
			}
			tt.mutate(&state)
			state.InitialSnapshot.Identity = snapshotIdentity(state.InitialSnapshot.Kind, state.InitialSnapshot.BaseTree, state.InitialSnapshot.CandidateTree, state.InitialSnapshot.PathsDigest, state.InitialSnapshot.IntendedUntrackedProof, state.InitialSnapshot.IntendedUntracked, state.InitialSnapshot.LedgerIDs)
			state.CurrentSnapshot.Identity = snapshotIdentity(state.CurrentSnapshot.Kind, state.CurrentSnapshot.BaseTree, state.CurrentSnapshot.CandidateTree, state.CurrentSnapshot.PathsDigest, state.CurrentSnapshot.IntendedUntrackedProof, state.CurrentSnapshot.IntendedUntracked, state.CurrentSnapshot.LedgerIDs)
			record, payload, err := makeCompactRecord(state)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := parseCompactRecord(payload, state.LineageID); err == nil || strings.Contains(err.Error(), "checksum mismatch") {
				t.Fatalf("checksum-valid impossible state parse error = %v", err)
			}
			store, _ := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
			if err := os.MkdirAll(store.Dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(store.StatePath(), payload, 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Load(); err == nil || strings.Contains(err.Error(), "checksum mismatch") {
				t.Fatalf("checksum-valid impossible current load error = %v", err)
			}
			_ = os.RemoveAll(store.Dir)
			transport := CompactTransport{Schema: CompactTransportSchema, Record: record}
			transport.BundleDigest = compactTransportDigest(transport)
			transportPayload, _ := json.Marshal(transport)
			if _, err := ParseCompactTransport(transportPayload); err == nil || strings.Contains(err.Error(), "checksum mismatch") {
				t.Fatalf("checksum-valid impossible transport parse error = %v", err)
			}
			if _, err := ImportCompactTransport(context.Background(), repo, transport); err == nil || strings.Contains(err.Error(), "checksum mismatch") {
				t.Fatalf("checksum-valid impossible import error = %v", err)
			}
		})
	}
}

func TestCompactStoreRejectsConcurrentLockedWriter(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newCompactTestState(t, repo, "compact-locked")
	store, _ := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	lock, err := acquireStoreLock(store.lockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.release()
	if _, err := store.Replace("", "review/start", state); !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("concurrent compact writer error = %v", err)
	}
}

func TestCompactTransportRoundTripRecoversEquivalentCurrentAuthority(t *testing.T) {
	source := initSnapshotRepo(t)
	writeSnapshotFile(t, source, "tracked.txt", "candidate\n")
	gitSnapshot(t, source, "add", "tracked.txt")
	gitSnapshot(t, source, "commit", "-m", "candidate")
	state := newCompactRevisionState(t, source, "compact-transport")
	store, _ := CompactAuthoritativeStore(context.Background(), source, state.LineageID)
	if _, err := store.Replace("", "review/start", state); err != nil {
		t.Fatal(err)
	}
	results := make([]LensResult, len(state.SelectedLenses))
	for index, lens := range state.SelectedLenses {
		results[index] = LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"review completed"}}
	}
	if err := state.CompleteReview(CompactReviewInput{LensResults: results, Classifications: []FindingEvidence{}, RefuterOutcomes: []EvidenceResult{}}); err != nil {
		t.Fatal(err)
	}
	record, _ := store.Load()
	if _, err := store.Replace(record.Revision, "review/complete-review", state); err != nil {
		t.Fatal(err)
	}
	if err := state.CompleteVerification([]byte("tests pass\n"), true); err != nil {
		t.Fatal(err)
	}
	record, _ = store.Load()
	if _, err := store.Replace(record.Revision, "review/complete-verification", state); err != nil {
		t.Fatal(err)
	}
	receipt, _ := state.Receipt()
	if err := WriteCompactReceiptAtomic(store.ReceiptPath(), receipt); err != nil {
		t.Fatal(err)
	}
	transport, err := store.ExportTransport()
	if err != nil {
		t.Fatal(err)
	}
	if transport.Receipt == nil {
		t.Fatalf("compact transport = %#v", transport)
	}

	destination := filepath.Join(t.TempDir(), "clone")
	gitSnapshot(t, source, "clone", "--no-local", source, destination)
	imported, err := ImportCompactTransport(context.Background(), destination, transport)
	if err != nil {
		t.Fatal(err)
	}
	destinationStore, _ := CompactAuthoritativeStore(context.Background(), destination, state.LineageID)
	destinationTransport, err := destinationStore.ExportTransport()
	if err != nil {
		t.Fatal(err)
	}
	if imported.Revision != transport.Record.Revision || !reflect.DeepEqual(destinationTransport.Record, transport.Record) || !reflect.DeepEqual(destinationTransport.Receipt, transport.Receipt) {
		t.Fatalf("compact transport round trip changed authority")
	}
	if _, err := os.Stat(filepath.Join(destinationStore.Dir, "events")); !os.IsNotExist(err) {
		t.Fatalf("compact import reconstructed event history: %v", err)
	}
}

func TestCompactStagedTransportRoundTripRejectsProjectionTampering(t *testing.T) {
	source := initSnapshotRepo(t)
	writeSnapshotFile(t, source, "tracked.txt", "staged candidate\n")
	gitSnapshot(t, source, "add", "--", "tracked.txt")
	state := newCompactStartStateForTarget(t, source, "compact-staged-transport", Target{Kind: TargetCurrentChanges, Projection: ProjectionStaged, IntendedUntracked: []string{}})
	store, err := CompactAuthoritativeStore(context.Background(), source, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.Replace("", "review/start", state)
	if err != nil {
		t.Fatal(err)
	}
	results := make([]LensResult, len(state.SelectedLenses))
	for index, lens := range state.SelectedLenses {
		results[index] = LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"reviewed"}}
	}
	if err := state.CompleteReview(CompactReviewInput{LensResults: results, Classifications: []FindingEvidence{}, RefuterOutcomes: []EvidenceResult{}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(revision, "review/complete-review", state); err != nil {
		t.Fatal(err)
	}
	if err := state.CompleteVerification([]byte("tests pass\n"), true); err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(record.Revision, "review/complete-verification", state); err != nil {
		t.Fatal(err)
	}
	receipt, err := state.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Projection != ProjectionStaged {
		t.Fatalf("staged receipt projection = %q", receipt.Projection)
	}
	if err := WriteCompactReceiptAtomic(store.ReceiptPath(), receipt); err != nil {
		t.Fatal(err)
	}
	gitSnapshot(t, source, "commit", "-qm", "staged candidate")
	transport, err := store.ExportTransport()
	if err != nil {
		t.Fatal(err)
	}
	if transport.Record.State.InitialSnapshot.Projection != ProjectionStaged || transport.Receipt == nil || transport.Receipt.Projection != ProjectionStaged {
		t.Fatalf("staged transport = %#v", transport)
	}
	clone := filepath.Join(t.TempDir(), "clone")
	gitSnapshot(t, source, "clone", "--no-local", source, clone)
	if _, err := ImportCompactTransport(context.Background(), clone, transport); err != nil {
		t.Fatal(err)
	}
	tampered := transport
	tamperedReceipt := *transport.Receipt
	tamperedReceipt.Projection = ProjectionWorkspace
	tampered.Receipt = &tamperedReceipt
	tampered.BundleDigest = compactTransportDigest(tampered)
	payload, err := json.Marshal(tampered)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseCompactTransport(payload); err == nil || !strings.Contains(err.Error(), "receipt does not match state") {
		t.Fatalf("projection-tampered transport error = %v", err)
	}
}

func TestCompactTransportImportRejectsWrongDeliveredTreeAndScope(t *testing.T) {
	source := initSnapshotRepo(t)
	state := correctedCompactTestState(t, source, "compact-transport-binding")
	gitSnapshot(t, source, "add", "tracked.txt")
	gitSnapshot(t, source, "commit", "-m", "corrected candidate")
	tests := []struct {
		name   string
		mutate func(*CompactState)
		want   string
	}{
		{name: "wrong delivered tree", want: "delivered tree", mutate: func(candidate *CompactState) {
			candidate.CurrentSnapshot.CandidateTree = candidate.InitialSnapshot.BaseTree
			candidate.FixDeltaHash = FixDeltaHashForSnapshot(candidate.CurrentSnapshot)
		}},
		{name: "wrong delivered path scope", want: "path scope", mutate: func(candidate *CompactState) {
			candidate.InitialSnapshot.Paths = []string{"other.txt"}
			candidate.InitialSnapshot.PathsDigest = digestPaths(candidate.InitialSnapshot.Paths)
			candidate.GenesisPaths = append([]string(nil), candidate.InitialSnapshot.Paths...)
			candidate.CurrentSnapshot.Paths = []string{"other.txt"}
			candidate.CurrentSnapshot.PathsDigest = digestPaths(candidate.CurrentSnapshot.Paths)
			candidate.FixDeltaHash = FixDeltaHashForSnapshot(candidate.CurrentSnapshot)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := state
			candidate.InitialSnapshot.Paths = append([]string(nil), state.InitialSnapshot.Paths...)
			candidate.CurrentSnapshot.Paths = append([]string(nil), state.CurrentSnapshot.Paths...)
			candidate.GenesisPaths = append([]string(nil), state.GenesisPaths...)
			tt.mutate(&candidate)
			candidate.InitialSnapshot.Identity = snapshotIdentity(candidate.InitialSnapshot.Kind, candidate.InitialSnapshot.BaseTree, candidate.InitialSnapshot.CandidateTree, candidate.InitialSnapshot.PathsDigest, candidate.InitialSnapshot.IntendedUntrackedProof, candidate.InitialSnapshot.IntendedUntracked, candidate.InitialSnapshot.LedgerIDs)
			candidate.CurrentSnapshot.Identity = snapshotIdentity(candidate.CurrentSnapshot.Kind, candidate.CurrentSnapshot.BaseTree, candidate.CurrentSnapshot.CandidateTree, candidate.CurrentSnapshot.PathsDigest, candidate.CurrentSnapshot.IntendedUntrackedProof, candidate.CurrentSnapshot.IntendedUntracked, candidate.CurrentSnapshot.LedgerIDs)
			candidate.OriginalCriteria.FixDeltaHash = candidate.FixDeltaHash
			candidate.CorrectionRegression.FixDeltaHash = candidate.FixDeltaHash
			if err := candidate.Validate(); err != nil {
				t.Fatalf("test candidate must remain checksum-valid and semantically self-consistent: %v", err)
			}
			record, _, err := makeCompactRecord(candidate)
			if err != nil {
				t.Fatal(err)
			}
			receipt, err := candidate.Receipt()
			if err != nil {
				t.Fatal(err)
			}
			transport := CompactTransport{Schema: CompactTransportSchema, Record: record, Receipt: &receipt}
			transport.BundleDigest = compactTransportDigest(transport)
			clone := filepath.Join(t.TempDir(), "clone")
			gitSnapshot(t, source, "clone", "--no-local", source, clone)
			if _, err := ImportCompactTransport(context.Background(), clone, transport); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("wrong compact delivery import error = %v", err)
			}
		})
	}
}

func TestCompactDiagnosticTraceContainsMetadataOnly(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newCompactTestState(t, repo, "compact-trace")
	store, _ := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	store.TracePath = filepath.Join(t.TempDir(), "trace.jsonl")
	if _, err := store.Replace("", "review/start", state); err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(store.TracePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "initial_snapshot") || strings.Contains(string(payload), "findings") || !strings.Contains(string(payload), `"operation":"review/start"`) {
		t.Fatalf("diagnostic trace contains authority snapshot or lacks metadata: %s", payload)
	}
}

func TestCompactLifecycleComplexityMeasurements(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "candidate")
	_, compactStore, _ := approvedCompactRevisionFixture(t, repo, "compact-measurement")
	compactFiles, compactBytes := authorityFileMetrics(t, compactStore.Dir)

	legacyTransaction, legacyReceipt, _ := nativeGateFixture(t, repo, "legacy-measurement")
	legacyStore, err := AuthoritativeStore(context.Background(), repo, legacyTransaction.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	appendApprovedStoreChain(t, legacyStore, legacyTransaction)
	if err := WriteReceiptAtomic(filepath.Join(legacyStore.Dir, "artifacts", "receipt.json"), legacyReceipt); err != nil {
		t.Fatal(err)
	}
	legacyFiles, legacyBytes := authorityFileMetrics(t, legacyStore.Dir)

	if compactFiles != 2 || legacyFiles <= compactFiles || compactBytes >= legacyBytes {
		t.Fatalf("authority metrics legacy=%d files/%d bytes compact=%d files/%d bytes", legacyFiles, legacyBytes, compactFiles, compactBytes)
	}
	t.Logf("authority metrics: legacy v1=%d files/%d bytes; compact v2=%d files/%d bytes; semantic states=12->5; counters=12->0; clean writes=6->3; corrected writes=9->5", legacyFiles, legacyBytes, compactFiles, compactBytes)
}

func authorityFileMetrics(t *testing.T, root string) (int, int64) {
	t.Helper()
	files := 0
	var bytes int64
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() == "LOCK" || strings.HasPrefix(entry.Name(), ".atomic-") {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		files++
		bytes += info.Size()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files, bytes
}

func storeCompactStartAuthority(t *testing.T, repo string, state CompactState) CompactStore {
	t.Helper()
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace("", "review/start", state); err != nil {
		t.Fatal(err)
	}
	return store
}

func newCompactStartStateForTarget(t *testing.T, repo, lineage string, target Target) CompactState {
	t.Helper()
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), target)
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
	state, err := NewCompactState(Start{LineageID: lineage, Mode: ModeOrdinaryBounded, Generation: 1, Snapshot: snapshot, PolicyHash: hash("1"), RiskLevel: risk, SelectedLenses: lenses, OriginalChangedLines: &lines})
	if err != nil {
		t.Fatal(err)
	}
	return state
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

func correctedCompactTestState(t *testing.T, repo, lineage string) CompactState {
	t.Helper()
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfour\n")
	state := newCompactTestState(t, repo, lineage)
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
	if err := state.CompleteCorrection(fix, 2, validation); err != nil {
		t.Fatal(err)
	}
	if err := state.CompleteVerification([]byte("tests pass\n"), true); err != nil {
		t.Fatal(err)
	}
	// Preserve the legacy compact fixture shape for backward-compatibility tests.
	state.CorrectionAttempts, state.CumulativeCorrectionLines = nil, 0
	return state
}

func newCompactRevisionState(t *testing.T, repo, lineage string) CompactState {
	t.Helper()
	commit := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetExactRevision, Revision: commit})
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
	state, err := NewCompactState(Start{LineageID: lineage, Mode: ModeOrdinaryBounded, Generation: 1, Snapshot: snapshot, PolicyHash: hash("1"), RiskLevel: risk, SelectedLenses: lenses, OriginalChangedLines: &lines})
	if err != nil {
		t.Fatal(err)
	}
	return state
}
