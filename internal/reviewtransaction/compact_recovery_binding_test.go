package reviewtransaction

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
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
	repo       string
	remote     string
	branch     string
	baseRef    string
	root       CompactState
	rootStore  CompactStore
	leaf       CompactState
	receipt    CompactReceipt
	policyHash string
}

func chainedScopeRecoveryFixture(t *testing.T) *chainedRecoveryFixture {
	return chainedScopeRecoveryFixtureWithPolicy(t, hash("1"))
}

func chainedScopeRecoveryFixtureWithPolicy(t *testing.T, policyHash string) *chainedRecoveryFixture {
	t.Helper()
	repo := initSnapshotRepo(t)
	branch := currentBranch(context.Background(), repo)
	remote := configurePublicationRemote(t, repo, branch)
	gitSnapshot(t, repo, "config", "branch."+branch+".remote", "origin")
	gitSnapshot(t, repo, "config", "branch."+branch+".merge", "refs/heads/"+branch)
	root := correctedCompactTestState(t, repo, "chained-recovery-root")
	root.PolicyHash = policyHash
	persistCorrectedCompactFixture(t, repo, root)
	rootStore, err := CompactAuthoritativeStore(context.Background(), repo, root.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "reviewed delivery")
	leaf, receipt := recoverApprovedCompactSuccessorWithPolicy(t, repo, root.LineageID, "chained-recovery-r1", 2, policyHash)
	return &chainedRecoveryFixture{
		repo: repo, remote: remote, branch: branch, baseRef: "origin/" + branch,
		root: root, rootStore: rootStore, leaf: leaf, receipt: receipt, policyHash: policyHash,
	}
}

// extendWithSecondDelivery reviews and delivers a second in-chain change so
// the recovery chain spans two delivery commits: root covers the first commit,
// the degenerate r1 covers nothing, and r2 covers the second commit.
func (fixture *chainedRecoveryFixture) extendWithSecondDelivery(t *testing.T) (CompactState, CompactReceipt) {
	t.Helper()
	writeSnapshotFile(t, fixture.repo, "deleted.txt", "second reviewed delivery\n")
	state, receipt := recoverApprovedCompactSuccessorWithPolicy(t, fixture.repo, fixture.leaf.LineageID, "chained-recovery-r2", 3, fixture.policyHash)
	gitSnapshot(t, fixture.repo, "add", "deleted.txt")
	gitSnapshot(t, fixture.repo, "commit", "-m", "second reviewed delivery")
	return state, receipt
}

// recoverApprovedCompactSuccessor creates a scope_changed recovery successor
// from the live repository and approves it through the ordinary compact
// lifecycle. The successor itself always starts as a pristine reviewing
// authority; only gate binding may later compose the chain.
func recoverApprovedCompactSuccessor(t *testing.T, repo, predecessorLineage, lineage string, generation int) (CompactState, CompactReceipt) {
	return recoverApprovedCompactSuccessorWithPolicy(t, repo, predecessorLineage, lineage, generation, hash("1"))
}

func recoverApprovedCompactSuccessorWithPolicy(t *testing.T, repo, predecessorLineage, lineage string, generation int, policyHash string) (CompactState, CompactReceipt) {
	t.Helper()
	successor := newCompactTestState(t, repo, lineage)
	successor.Generation = generation
	successor.PolicyHash = policyHash
	return recoverApprovedCompactSuccessorState(t, repo, predecessorLineage, successor)
}

func recoverApprovedCompactSuccessorState(t *testing.T, repo, predecessorLineage string, successor CompactState) (CompactState, CompactReceipt) {
	t.Helper()
	predecessorStore, err := CompactAuthoritativeStore(context.Background(), repo, predecessorLineage)
	if err != nil {
		t.Fatal(err)
	}
	predecessorRecord, err := predecessorStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	record, err := RecoverCompactAuthority(context.Background(), repo, CompactRecoveryRequest{
		PredecessorLineageID: predecessorLineage, ExpectedPredecessorRevision: predecessorRecord.Revision,
		Successor: successor, Disposition: RecoveryScopeChanged,
		Reason: "delivery scope changed after approval", Actor: "maintainer@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	state := record.State
	store, err := CompactAuthoritativeStore(context.Background(), repo, successor.LineageID)
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

func overlappingRecoveryFixture(t *testing.T) *chainedRecoveryFixture {
	t.Helper()
	repo := initSnapshotRepo(t)
	branch := currentBranch(context.Background(), repo)
	remote := configurePublicationRemote(t, repo, branch)
	gitSnapshot(t, repo, "config", "branch."+branch+".remote", "origin")
	gitSnapshot(t, repo, "config", "branch."+branch+".merge", "refs/heads/"+branch)
	base := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))

	writeSnapshotFile(t, repo, "tracked.txt", "intermediate\n")
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "intermediate boundary")
	intermediate := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))

	writeSnapshotFile(t, repo, "deleted.txt", "final reviewed content\n")
	gitSnapshot(t, repo, "add", "deleted.txt")
	gitSnapshot(t, repo, "commit", "-m", "final reviewed candidate")

	root := newCompactStartStateForTarget(t, repo, "overlapping-recovery-root", Target{
		Kind: TargetBaseDiff, BaseRef: base, IntendedUntracked: []string{},
	})
	root, _ = persistApprovedCompactState(t, repo, root)
	successor := newCompactStartStateForTarget(t, repo, "overlapping-recovery-leaf", Target{
		Kind: TargetBaseDiff, BaseRef: intermediate, IntendedUntracked: []string{},
	})
	successor.Generation = 2
	leaf, receipt := recoverApprovedCompactSuccessorState(t, repo, root.LineageID, successor)
	rootStore, err := CompactAuthoritativeStore(context.Background(), repo, root.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	return &chainedRecoveryFixture{
		repo: repo, remote: remote, branch: branch, baseRef: "origin/" + branch,
		root: root, rootStore: rootStore, leaf: leaf, receipt: receipt,
	}
}

func TestCompactPrePRRecoveryChainPreservesCumulativeAuthority(t *testing.T) {
	fixture := overlappingRecoveryFixture(t)
	binding, ok, err := deriveCompactRecoveryBinding(context.Background(), fixture.repo, fixture.leaf)
	if err != nil || !ok {
		t.Fatalf("derive overlapping scope recovery binding: ok=%t err=%v", ok, err)
	}
	if binding.BaseTree != fixture.root.InitialSnapshot.BaseTree ||
		!equalStrings(binding.GenesisPaths, fixture.root.GenesisPaths) || len(binding.Members) != 2 {
		t.Fatalf("overlapping scope recovery binding = %#v", binding)
	}
	got := EvaluateCompactGate(context.Background(), fixture.repo, fixture.receipt, NativeGateRequestInput{
		Gate: GatePrePR, LineageID: fixture.leaf.LineageID, BaseRef: fixture.baseRef,
	})
	if got.Result != GateAllow || !got.Context.BaseRelationshipValid || got.Context.Denial != nil {
		t.Fatalf("overlapping scope recovery pre-pr = %#v", got)
	}
	got = EvaluateCompactGate(context.Background(), fixture.repo, fixture.receipt, NativeGateRequestInput{
		Gate: GatePrePush, LineageID: fixture.leaf.LineageID, BaseRef: fixture.baseRef,
	})
	if got.Result != GateAllow || !got.Context.BaseRelationshipValid || got.Context.Denial != nil {
		t.Fatalf("overlapping scope recovery pre-push = %#v", got)
	}
}

func TestCompactOverlappingRecoveryRebindRejectsScopeOutsideCumulativeAuthority(t *testing.T) {
	repo := initSnapshotRepo(t)
	branch := currentBranch(context.Background(), repo)
	configurePublicationRemote(t, repo, branch)
	base := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	writeSnapshotFile(t, repo, "transient.txt", "outside cumulative authority\n")
	gitSnapshot(t, repo, "add", "transient.txt")
	gitSnapshot(t, repo, "commit", "-m", "transient intermediate path")
	intermediate := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	if err := os.Remove(filepath.Join(repo, "transient.txt")); err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "deleted.txt", "final reviewed content\n")
	gitSnapshot(t, repo, "add", "-A")
	gitSnapshot(t, repo, "commit", "-m", "final reviewed candidate")

	root := newCompactStartStateForTarget(t, repo, "overlap-outside-root", Target{Kind: TargetBaseDiff, BaseRef: base, IntendedUntracked: []string{}})
	root, _ = persistApprovedCompactState(t, repo, root)
	successor := newCompactStartStateForTarget(t, repo, "overlap-outside-leaf", Target{Kind: TargetBaseDiff, BaseRef: intermediate, IntendedUntracked: []string{}})
	successor.Generation = 2
	leaf, receipt := recoverApprovedCompactSuccessorState(t, repo, root.LineageID, successor)
	if _, ok, err := deriveCompactRecoveryBinding(context.Background(), repo, leaf); err != nil || ok {
		t.Fatalf("outside-scope overlap binding: ok=%t err=%v", ok, err)
	}
	got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{
		Gate: GatePrePR, LineageID: leaf.LineageID, BaseRef: "origin/" + branch,
	})
	if got.Result == GateAllow {
		t.Fatalf("outside-scope overlap rebind = %#v", got)
	}
}

func TestCompactOverlappingRecoveryRebindRejectsBaseOutsidePublicationAncestry(t *testing.T) {
	repo := initSnapshotRepo(t)
	branch := currentBranch(context.Background(), repo)
	configurePublicationRemote(t, repo, branch)
	base := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	gitSnapshot(t, repo, "checkout", "-b", "unpublished-recovery-base")
	writeSnapshotFile(t, repo, "tracked.txt", "unpublished base\n")
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "unpublished recovery base")
	unpublishedBase := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	gitSnapshot(t, repo, "checkout", branch)
	writeSnapshotFile(t, repo, "tracked.txt", "published intermediate\n")
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "published intermediate")
	writeSnapshotFile(t, repo, "deleted.txt", "final reviewed content\n")
	gitSnapshot(t, repo, "add", "deleted.txt")
	gitSnapshot(t, repo, "commit", "-m", "final reviewed candidate")

	root := newCompactStartStateForTarget(t, repo, "overlap-order-root", Target{Kind: TargetBaseDiff, BaseRef: base, IntendedUntracked: []string{}})
	root, _ = persistApprovedCompactState(t, repo, root)
	successor := newCompactStartStateForTarget(t, repo, "overlap-order-leaf", Target{Kind: TargetBaseDiff, BaseRef: unpublishedBase, IntendedUntracked: []string{}})
	successor.Generation = 2
	leaf, receipt := recoverApprovedCompactSuccessorState(t, repo, root.LineageID, successor)
	if _, ok, err := deriveCompactRecoveryBinding(context.Background(), repo, leaf); err != nil || !ok {
		t.Fatalf("immutable off-ancestry overlap binding: ok=%t err=%v", ok, err)
	}
	got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{
		Gate: GatePrePR, LineageID: leaf.LineageID, BaseRef: "origin/" + branch,
	})
	if got.Result == GateAllow {
		t.Fatalf("off-ancestry overlap rebind = %#v", got)
	}
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
	original := finalCompactGateAllowHook
	t.Cleanup(func() { finalCompactGateAllowHook = original })
	finalCompactGateAllowHook = func() { _ = os.Remove(fixture.rootStore.ReceiptPath()) }
	got := EvaluateCompactGate(context.Background(), fixture.repo, fixture.receipt, NativeGateRequestInput{
		Gate: GatePrePush, LineageID: fixture.leaf.LineageID, BaseRef: fixture.baseRef,
	})
	if got.Result == GateAllow {
		t.Fatalf("receiptless predecessor rebind = %#v", got)
	}
}

func TestCompactRecoveryDirectLeafRejectsUnreviewedChangeRevert(t *testing.T) {
	fixture := chainedScopeRecoveryFixture(t)
	gitSnapshot(t, fixture.repo, "push", "origin", "HEAD:"+fixture.branch)
	state, receipt := fixture.extendWithSecondDelivery(t)
	writeSnapshotFile(t, fixture.repo, "outside.txt", "unreviewed\n")
	gitSnapshot(t, fixture.repo, "add", "outside.txt")
	gitSnapshot(t, fixture.repo, "commit", "-m", "unreviewed intermediate")
	if err := os.Remove(filepath.Join(fixture.repo, "outside.txt")); err != nil {
		t.Fatal(err)
	}
	gitSnapshot(t, fixture.repo, "add", "-A")
	gitSnapshot(t, fixture.repo, "commit", "-m", "revert unreviewed intermediate")
	got := EvaluateCompactGate(context.Background(), fixture.repo, receipt, NativeGateRequestInput{
		Gate: GatePrePR, LineageID: state.LineageID, BaseRef: fixture.baseRef,
	})
	if got.Result == GateAllow {
		t.Fatalf("direct leaf with unreviewed change/revert = %#v", got)
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

func TestCompactPrePRRecoveryChainRejectsMovingAdvertisedBoundary(t *testing.T) {
	fixture := newAttestedRecoveryAdvanceFixture(t)
	unattested := fixture.input
	unattested.PrePRCIAttestation = ""
	if got := EvaluateCompactGate(context.Background(), fixture.recovery.repo, fixture.receipt, unattested); got.Result == GateAllow {
		t.Fatalf("unattested recovery-chain base advance = %#v", got)
	}

	got := EvaluateCompactGate(context.Background(), fixture.recovery.repo, fixture.receipt, fixture.input)
	if got.Result == GateAllow {
		t.Fatalf("attested recovery-chain base advance = %#v", got)
	}
}

func TestCompactPrePRRecoveryAdvanceAttestationCannotRescueInvalidFullChain(t *testing.T) {
	fixture := newAttestedRecoveryAdvanceFixture(t)
	writeSnapshotFile(t, fixture.recovery.repo, "outside.txt", "unreviewed transient\n")
	gitSnapshot(t, fixture.recovery.repo, "add", "outside.txt")
	gitSnapshot(t, fixture.recovery.repo, "commit", "-m", "unreviewed transient")
	if err := os.Remove(filepath.Join(fixture.recovery.repo, "outside.txt")); err != nil {
		t.Fatal(err)
	}
	gitSnapshot(t, fixture.recovery.repo, "add", "-A")
	gitSnapshot(t, fixture.recovery.repo, "commit", "-m", "revert unreviewed transient")

	got := EvaluateCompactGate(context.Background(), fixture.recovery.repo, fixture.receipt, fixture.input)
	if got.Result == GateAllow {
		t.Fatalf("attestation rescued invalid full recovery delivery chain = %#v", got)
	}
}

func TestCompactPrePRNormalAdvanceRevalidatesArtifactsAtFinalAuthorization(t *testing.T) {
	for _, tt := range []struct {
		name string
		path func(*compatiblePrePRFixture) string
	}{
		{name: "policy", path: func(fixture *compatiblePrePRFixture) string { return fixture.policyPath }},
		{name: "attestation", path: func(fixture *compatiblePrePRFixture) string { return fixture.attestationPath }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newCompatiblePrePRFixture(t, "delivery.txt", "base-only.txt")
			state, receipt := approvedCompactPrePRFixture(t, fixture)
			hookRan := false
			originalHook := finalGateAuthorizationHook
			finalGateAuthorizationHook = func() {
				finalGateAuthorizationHook = originalHook
				hookRan = true
				if err := os.Remove(tt.path(fixture)); err != nil {
					t.Fatal(err)
				}
			}
			t.Cleanup(func() { finalGateAuthorizationHook = originalHook })

			got := EvaluateCompactGate(context.Background(), fixture.repo, receipt, NativeGateRequestInput{
				Gate: GatePrePR, LineageID: state.LineageID, BaseRef: "main", PolicyArtifact: fixture.policyPath, PrePRCIAttestation: fixture.attestationPath,
			})
			if !hookRan || got.Result != GateInvalidated {
				t.Fatalf("%s mutation during final authorization = %#v", tt.name, got)
			}
		})
	}
}

type attestedRecoveryAdvanceFixture struct {
	recovery *chainedRecoveryFixture
	state    CompactState
	receipt  CompactReceipt
	input    NativeGateRequestInput
}

func newAttestedRecoveryAdvanceFixture(t *testing.T) *attestedRecoveryAdvanceFixture {
	t.Helper()
	requireMergeTreeWriteTree(t)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	policyPayload := []byte("pre_pr_ci_issuer: trusted-ci\npre_pr_ci_ed25519_public_key: " + base64.StdEncoding.EncodeToString(publicKey) + "\n")
	policyPath := filepath.Join(dir, "policy.md")
	if err := os.WriteFile(policyPath, policyPayload, 0o644); err != nil {
		t.Fatal(err)
	}
	recovery := chainedScopeRecoveryFixtureWithPolicy(t, hashArtifactPayload(policyPayload))
	state, receipt := recovery.extendWithSecondDelivery(t)
	newBase := advanceChainedRecoveryBoundary(t, recovery)
	mergedOutput, err := runGit(context.Background(), recovery.repo, nil, nil, "merge-tree", "--write-tree", newBase, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	mergedFields := strings.Fields(string(mergedOutput))
	if len(mergedFields) == 0 {
		t.Fatal("compatible recovery advance did not produce a merged tree")
	}
	attestation := prePRCIAttestation{
		Schema: prePRCIAttestationSchema, Issuer: "trusted-ci", MergedTree: mergedFields[0], Status: "success",
	}
	attestation.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, prePRCIAttestationPreimage(attestation)))
	payload, err := json.Marshal(attestation)
	if err != nil {
		t.Fatal(err)
	}
	attestationPath := filepath.Join(dir, "attestation.json")
	if err := os.WriteFile(attestationPath, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	return &attestedRecoveryAdvanceFixture{
		recovery: recovery, state: state, receipt: receipt,
		input: NativeGateRequestInput{
			Gate: GatePrePR, LineageID: state.LineageID, BaseRef: recovery.baseRef,
			PolicyArtifact: policyPath, PrePRCIAttestation: attestationPath,
		},
	}
}

func advanceChainedRecoveryBoundary(t *testing.T, fixture *chainedRecoveryFixture) string {
	t.Helper()
	side := t.TempDir()
	gitSnapshot(t, fixture.repo, "clone", fixture.remote, side)
	gitSnapshot(t, side, "config", "user.email", "side@example.com")
	gitSnapshot(t, side, "config", "user.name", "Side")
	writeSnapshotFile(t, side, "base-only.txt", "unrelated boundary advance\n")
	gitSnapshot(t, side, "add", "base-only.txt")
	gitSnapshot(t, side, "commit", "-m", "boundary advance")
	gitSnapshot(t, side, "push", "origin", "HEAD:"+fixture.branch)
	gitSnapshot(t, fixture.repo, "fetch", "origin")
	return strings.TrimSpace(gitSnapshot(t, side, "rev-parse", "HEAD"))
}

// TestCompactFinalRecoveryDeliveryRefusalNamesItsCause locks the second half
// of the named-continuation defect: when the composed-chain delivery check the
// evaluation relied on genuinely fails under the final authorization lock, the
// verifier's specific reason must survive onto the evaluation. Discarding it
// with `!= nil` left an operator with a generic sentence naming no cause and
// no continuation, which is a defect even when the refusal itself is right.
func TestCompactFinalRecoveryDeliveryRefusalNamesItsCause(t *testing.T) {
	fixture := chainedScopeRecoveryFixture(t)
	base := strings.TrimSpace(gitSnapshot(t, fixture.repo, "rev-parse", "HEAD~1"))
	object := filepath.Join(fixture.repo, ".git", "objects", base[:2], base[2:])
	payload, err := os.ReadFile(object)
	if err != nil {
		t.Fatalf("read publication base commit object: %v", err)
	}
	original := finalCompactGateAllowHook
	t.Cleanup(func() {
		finalCompactGateAllowHook = original
		_ = os.WriteFile(object, payload, 0o444)
	})
	// Everything the evaluation compared under the lock is already proven
	// unchanged, so the only way this defensive re-verification fails is a
	// repository read that stops answering. Remove the publication base commit
	// exactly between the two calls.
	finalCompactGateAllowHook = func() {
		finalCompactGateAllowHook = original
		if err := os.Remove(object); err != nil {
			t.Fatal(err)
		}
	}
	got := EvaluateCompactGate(context.Background(), fixture.repo, fixture.receipt, NativeGateRequestInput{
		Gate: GatePrePush, LineageID: fixture.leaf.LineageID, BaseRef: fixture.baseRef,
	})
	if got.Result != GateInvalidated {
		t.Fatalf("unreadable publication base at final authorization = %#v", got)
	}
	if !strings.HasPrefix(got.Reason, "compact recovery delivery changed during final authorization") {
		t.Fatalf("final recovery delivery refusal reason = %q", got.Reason)
	}
	if got.Cause == nil {
		t.Fatalf("final recovery delivery refusal discarded its cause: %#v", got)
	}
	if !strings.Contains(got.Reason, got.Cause.Error()) {
		t.Fatalf("final recovery delivery reason %q does not name its cause %q", got.Reason, got.Cause)
	}
}
