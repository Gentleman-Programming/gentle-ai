package reviewtransaction

import (
	"context"
	"os"
	"strings"
	"testing"
)

// chainedRecoveryFixture reproduces gentle-ai issue #1422: a corrected and
// approved current-changes review (non-empty fix delta) is delivered as one
// commit, and its scope_changed recovery successor is created from the live
// repository after the delivery. The successor's pristine snapshot degenerates
// to base == candidate with empty genesis paths, so its receipt alone can
// never rebind the publication gates even though every tree matches live Git.
type chainedRecoveryFixture struct {
	repo      string
	remote    string
	branch    string
	baseRef   string
	root      CompactState
	rootStore CompactStore
	leaf      CompactState
	receipt   CompactReceipt
}

func chainedScopeRecoveryFixture(t *testing.T) *chainedRecoveryFixture {
	t.Helper()
	repo := initSnapshotRepo(t)
	branch := currentBranch(context.Background(), repo)
	remote := configurePublicationRemote(t, repo, branch)
	gitSnapshot(t, repo, "config", "branch."+branch+".remote", "origin")
	gitSnapshot(t, repo, "config", "branch."+branch+".merge", "refs/heads/"+branch)
	root := correctedCompactTestState(t, repo, "chained-recovery-root")
	persistCorrectedCompactFixture(t, repo, root)
	rootStore, err := CompactAuthoritativeStore(context.Background(), repo, root.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "reviewed delivery")
	leaf, receipt := recoverApprovedCompactSuccessor(t, repo, root.LineageID, "chained-recovery-r1", 2)
	return &chainedRecoveryFixture{
		repo: repo, remote: remote, branch: branch, baseRef: "origin/" + branch,
		root: root, rootStore: rootStore, leaf: leaf, receipt: receipt,
	}
}

// extendWithSecondDelivery reviews and delivers a second in-chain change so
// the recovery chain spans two delivery commits: root covers the first commit,
// the degenerate r1 covers nothing, and r2 covers the second commit.
func (fixture *chainedRecoveryFixture) extendWithSecondDelivery(t *testing.T) (CompactState, CompactReceipt) {
	t.Helper()
	writeSnapshotFile(t, fixture.repo, "deleted.txt", "second reviewed delivery\n")
	state, receipt := recoverApprovedCompactSuccessor(t, fixture.repo, fixture.leaf.LineageID, "chained-recovery-r2", 3)
	gitSnapshot(t, fixture.repo, "add", "deleted.txt")
	gitSnapshot(t, fixture.repo, "commit", "-m", "second reviewed delivery")
	return state, receipt
}

// recoverApprovedCompactSuccessor creates a scope_changed recovery successor
// from the live repository and approves it through the ordinary compact
// lifecycle. The successor itself always starts as a pristine reviewing
// authority; only gate binding may later compose the chain.
func recoverApprovedCompactSuccessor(t *testing.T, repo, predecessorLineage, lineage string, generation int) (CompactState, CompactReceipt) {
	t.Helper()
	predecessorStore, err := CompactAuthoritativeStore(context.Background(), repo, predecessorLineage)
	if err != nil {
		t.Fatal(err)
	}
	predecessorRecord, err := predecessorStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	successor := newCompactTestState(t, repo, lineage)
	successor.Generation = generation
	record, err := RecoverCompactAuthority(context.Background(), repo, CompactRecoveryRequest{
		PredecessorLineageID: predecessorLineage, ExpectedPredecessorRevision: predecessorRecord.Revision,
		Successor: successor, Disposition: RecoveryScopeChanged,
		Reason: "delivery scope changed after approval", Actor: "maintainer@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	state := record.State
	store, err := CompactAuthoritativeStore(context.Background(), repo, lineage)
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
	revision, err := store.Replace(record.Revision, "review/complete-review", state)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.CompleteVerification([]byte("independent verification passed\n"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(revision, "review/complete-verification", state); err != nil {
		t.Fatal(err)
	}
	receipt, err := state.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteCompactReceiptAtomic(store.ReceiptPath(), receipt); err != nil {
		t.Fatal(err)
	}
	return state, receipt
}

func TestCompactPrePushBindsChainedScopeRecoveryDelivery(t *testing.T) {
	fixture := chainedScopeRecoveryFixture(t)
	got := EvaluateCompactGate(context.Background(), fixture.repo, fixture.receipt, NativeGateRequestInput{
		Gate: GatePrePush, LineageID: fixture.leaf.LineageID, BaseRef: fixture.baseRef,
	})
	if got.Result != GateAllow || !got.Context.BaseRelationshipValid {
		t.Fatalf("chained scope recovery pre-push = %#v", got)
	}
	if got.Context.FixDeltaHash == EmptyFixDeltaHash || !validSHA256(got.Context.FixDeltaHash) {
		t.Fatalf("chained scope recovery pre-push fix delta = %q", got.Context.FixDeltaHash)
	}
}

func TestCompactPrePRBindsChainedScopeRecoveryDelivery(t *testing.T) {
	fixture := chainedScopeRecoveryFixture(t)
	state, receipt := fixture.extendWithSecondDelivery(t)
	got := EvaluateCompactGate(context.Background(), fixture.repo, receipt, NativeGateRequestInput{
		Gate: GatePrePR, LineageID: state.LineageID, BaseRef: fixture.baseRef,
	})
	if got.Result != GateAllow || !got.Context.BaseRelationshipValid || got.Context.Denial != nil {
		t.Fatalf("chained scope recovery pre-pr = %#v", got)
	}
	if got.Context.FixDeltaHash == EmptyFixDeltaHash || !validSHA256(got.Context.FixDeltaHash) {
		t.Fatalf("chained scope recovery pre-pr fix delta = %q", got.Context.FixDeltaHash)
	}
	precommit := EvaluateCompactGate(context.Background(), fixture.repo, receipt, NativeGateRequestInput{
		Gate: GatePreCommit, LineageID: state.LineageID,
	})
	if precommit.Result != GateAllow {
		t.Fatalf("chained scope recovery pre-commit = %#v", precommit)
	}
}

func TestCompactGateAssessmentBindsChainedScopeRecovery(t *testing.T) {
	t.Run("pre-push", func(t *testing.T) {
		fixture := chainedScopeRecoveryFixture(t)
		store, err := CompactAuthoritativeStore(context.Background(), fixture.repo, fixture.leaf.LineageID)
		if err != nil {
			t.Fatal(err)
		}
		record, err := store.Load()
		if err != nil {
			t.Fatal(err)
		}
		assessment, err := AssessCompactGateTarget(context.Background(), fixture.repo, record.State, NativeGateRequestInput{
			Gate: GatePrePush, LineageID: record.State.LineageID, BaseRef: fixture.baseRef,
		})
		if err != nil || assessment.Applicability != CompactGateTargetExact {
			t.Fatalf("chained recovery pre-push assessment = %q, %v", assessment.Applicability, err)
		}
	})
	t.Run("pre-pr", func(t *testing.T) {
		fixture := chainedScopeRecoveryFixture(t)
		state, _ := fixture.extendWithSecondDelivery(t)
		store, err := CompactAuthoritativeStore(context.Background(), fixture.repo, state.LineageID)
		if err != nil {
			t.Fatal(err)
		}
		record, err := store.Load()
		if err != nil {
			t.Fatal(err)
		}
		assessment, err := AssessCompactGateTarget(context.Background(), fixture.repo, record.State, NativeGateRequestInput{
			Gate: GatePrePR, LineageID: record.State.LineageID, BaseRef: fixture.baseRef,
		})
		if err != nil || assessment.Applicability != CompactGateTargetExact {
			t.Fatalf("chained recovery pre-pr assessment = %q, %v", assessment.Applicability, err)
		}
	})
}

func TestCompactPrePushDeriveFailureCarriesReceiptDenialContext(t *testing.T) {
	repo := initSnapshotRepo(t)
	branch := currentBranch(context.Background(), repo)
	configurePublicationRemote(t, repo, branch)
	gitSnapshot(t, repo, "config", "branch."+branch+".remote", "origin")
	gitSnapshot(t, repo, "config", "branch."+branch+".merge", "refs/heads/"+branch)
	state := correctedCompactTestState(t, repo, "compact-derive-denial-context")
	receipt := persistCorrectedCompactFixture(t, repo, state)
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "corrected delivery")
	gitSnapshot(t, repo, "commit", "--allow-empty", "-m", "unreviewed extra commit")
	got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{
		Gate: GatePrePush, LineageID: state.LineageID, BaseRef: "origin/" + branch,
	})
	if got.Result != GateInvalidated || !strings.Contains(got.Reason, "reviewed delivery is not exactly one commit") {
		t.Fatalf("underivable pre-push delivery = %#v", got)
	}
	if got.Context.LineageID != state.LineageID || got.Context.BaseTree != receipt.BaseTree ||
		got.Context.CandidateTree != receipt.FinalCandidateTree || got.Context.FixDeltaHash != receipt.FixDeltaHash ||
		got.Context.Denial == nil {
		t.Fatalf("underivable pre-push denial context = %#v", got.Context)
	}
}

func TestCompactChainedRecoveryRebindFailsClosedWithoutBoundPredecessorReceipt(t *testing.T) {
	fixture := chainedScopeRecoveryFixture(t)
	if err := os.Remove(fixture.rootStore.ReceiptPath()); err != nil {
		t.Fatal(err)
	}
	got := EvaluateCompactGate(context.Background(), fixture.repo, fixture.receipt, NativeGateRequestInput{
		Gate: GatePrePush, LineageID: fixture.leaf.LineageID, BaseRef: fixture.baseRef,
	})
	if got.Result == GateAllow {
		t.Fatalf("receiptless predecessor rebind = %#v", got)
	}
}

func TestCompactChainedRecoveryRebindRejectsExtraDeliveryCommit(t *testing.T) {
	fixture := chainedScopeRecoveryFixture(t)
	gitSnapshot(t, fixture.repo, "commit", "--allow-empty", "-m", "unreviewed extra commit")
	got := EvaluateCompactGate(context.Background(), fixture.repo, fixture.receipt, NativeGateRequestInput{
		Gate: GatePrePush, LineageID: fixture.leaf.LineageID, BaseRef: fixture.baseRef,
	})
	if got.Result == GateAllow {
		t.Fatalf("multi-commit chained recovery delivery = %#v", got)
	}
}

func TestCompactChainedRecoveryRebindRejectsUncoveredPublicationPath(t *testing.T) {
	fixture := chainedScopeRecoveryFixture(t)
	state, receipt := fixture.extendWithSecondDelivery(t)
	writeSnapshotFile(t, fixture.repo, "outside.txt", "unreviewed\n")
	gitSnapshot(t, fixture.repo, "add", "outside.txt")
	gitSnapshot(t, fixture.repo, "commit", "-m", "unreviewed path")
	got := EvaluateCompactGate(context.Background(), fixture.repo, receipt, NativeGateRequestInput{
		Gate: GatePrePR, LineageID: state.LineageID, BaseRef: fixture.baseRef,
	})
	if got.Result == GateAllow {
		t.Fatalf("uncovered publication path rebind = %#v", got)
	}
}

func TestCompactChainedRecoveryRebindRejectsAdvancedBoundary(t *testing.T) {
	fixture := chainedScopeRecoveryFixture(t)
	state, receipt := fixture.extendWithSecondDelivery(t)
	side := t.TempDir()
	gitSnapshot(t, fixture.repo, "clone", fixture.remote, side)
	gitSnapshot(t, side, "config", "user.email", "side@example.com")
	gitSnapshot(t, side, "config", "user.name", "Side")
	writeSnapshotFile(t, side, "base-only.txt", "unrelated boundary advance\n")
	gitSnapshot(t, side, "add", "base-only.txt")
	gitSnapshot(t, side, "commit", "-m", "boundary advance")
	gitSnapshot(t, side, "push", "origin", "HEAD:"+fixture.branch)
	gitSnapshot(t, fixture.repo, "fetch", "origin")
	got := EvaluateCompactGate(context.Background(), fixture.repo, receipt, NativeGateRequestInput{
		Gate: GatePrePR, LineageID: state.LineageID, BaseRef: fixture.baseRef,
	})
	if got.Result == GateAllow {
		t.Fatalf("advanced boundary rebind = %#v", got)
	}
}
