package reviewtransaction

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestInvalidateApprovedCompactAuthorityPersistsNativeGateDenial(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "approved candidate\n")
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "approved candidate")
	state, store, _ := approvedCompactRevisionFixture(t, repo, "invalidate-approved-denied")
	before, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	base := trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD^"))
	branch := currentBranch(context.Background(), repo)
	remote := configurePublicationRemote(t, repo, branch)
	gitSnapshot(t, repo, "--git-dir", remote, "branch", "reviewed-base", base)

	record, evaluation, err := InvalidateApprovedCompactAuthority(context.Background(), repo, CompactApprovedInvalidationRequest{
		LineageID: state.LineageID, ExpectedRevision: before.Revision,
		Gate: NativeGateRequestInput{Gate: GatePrePR, LineageID: state.LineageID, BaseRef: "origin/" + branch},
	})
	if err != nil {
		t.Fatalf("invalidate approved authority: %v", err)
	}
	if evaluation.Result != GateInvalidated || record.State.State != StateInvalidated ||
		record.State.InvalidationReason != evaluation.Reason || record.Revision == before.Revision {
		t.Fatalf("approved invalidation = record %#v, evaluation %#v", record, evaluation)
	}
	if record.State.InvalidationEvidence == nil || record.State.InvalidationEvidence.Gate != GatePrePR ||
		!reflect.DeepEqual(record.State.InvalidationEvidence.Context, evaluation.Context) {
		t.Fatalf("approved invalidation evidence = %#v", record.State.InvalidationEvidence)
	}
	if !reflect.DeepEqual(record.State.LensResults, before.State.LensResults) ||
		record.State.EvidenceHash != before.State.EvidenceHash {
		t.Fatal("approved invalidation discarded frozen review evidence")
	}
	if _, err := os.Stat(store.ReceiptPath()); !os.IsNotExist(err) {
		t.Fatalf("invalidated authority retained an active receipt: %v", err)
	}
	loaded, err := store.Load()
	if err != nil || !reflect.DeepEqual(loaded, record) {
		t.Fatalf("persisted invalidation = %#v, %v", loaded, err)
	}
	replayed, replayEvaluation, err := InvalidateApprovedCompactAuthority(context.Background(), repo, CompactApprovedInvalidationRequest{
		LineageID: state.LineageID, ExpectedRevision: before.Revision,
		Gate: NativeGateRequestInput{Gate: GatePrePR, LineageID: state.LineageID, BaseRef: "origin/" + branch},
	})
	if err != nil || !reflect.DeepEqual(replayed, record) || !reflect.DeepEqual(replayEvaluation, evaluation) {
		t.Fatalf("exact approved invalidation replay = %#v, %#v, %v", replayed, replayEvaluation, err)
	}
	successor := newCompactTestState(t, repo, "invalidate-approved-denied-g2")
	successor.Generation = record.State.Generation + 1
	recovered, err := RecoverCompactAuthority(context.Background(), repo, CompactRecoveryRequest{
		PredecessorLineageID: record.State.LineageID, ExpectedPredecessorRevision: record.Revision,
		Successor: successor, Disposition: RecoveryInvalidated, Reason: "replace invalidated delivery", Actor: "maintainer",
	})
	if err != nil || recovered.State.Recovery == nil || recovered.State.Recovery.PredecessorRevision != record.Revision {
		t.Fatalf("recover invalidated approved predecessor = %#v, %v", recovered, err)
	}
}

func TestInvalidateApprovedCompactAuthorityRefusesHealthyGateUnderLock(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "approved candidate\n")
	state, store, _ := approvedCompactCurrentChangesFixture(t, repo, "invalidate-approved-healthy", []string{})
	gitSnapshot(t, repo, "add", "tracked.txt")
	beforeState, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	beforeReceipt, err := os.ReadFile(store.ReceiptPath())
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}

	originalHook := finalCompactGateAllowHook
	t.Cleanup(func() { finalCompactGateAllowHook = originalHook })
	finalCompactGateAllowHook = func() {
		contender, lockErr := acquireStoreLock(store.lockPath)
		if contender != nil {
			_ = contender.release()
		}
		if !errors.Is(lockErr, ErrConcurrentUpdate) {
			t.Fatalf("native derivation did not hold compact LOCK: %v", lockErr)
		}
	}

	_, evaluation, err := InvalidateApprovedCompactAuthority(context.Background(), repo, CompactApprovedInvalidationRequest{
		LineageID: state.LineageID, ExpectedRevision: record.Revision,
		Gate: NativeGateRequestInput{Gate: GatePreCommit, LineageID: state.LineageID},
	})
	if err == nil || !strings.Contains(err.Error(), "healthy approved authority") || evaluation.Result != GateAllow {
		t.Fatalf("healthy approved invalidation = evaluation %#v, error %v", evaluation, err)
	}
	afterState, _ := os.ReadFile(store.StatePath())
	afterReceipt, _ := os.ReadFile(store.ReceiptPath())
	if !reflect.DeepEqual(afterState, beforeState) || !reflect.DeepEqual(afterReceipt, beforeReceipt) {
		t.Fatal("refused healthy invalidation changed authority bytes")
	}
}

func TestInvalidateApprovedCompactAuthorityRejectsIncompleteReleaseRequestWithoutMutation(t *testing.T) {
	repo := initSnapshotRepo(t)
	state, store, _ := approvedCompactRevisionFixture(t, repo, "invalidate-approved-release-request")
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	beforeState, _ := os.ReadFile(store.StatePath())
	beforeReceipt, _ := os.ReadFile(store.ReceiptPath())

	_, evaluation, err := InvalidateApprovedCompactAuthority(context.Background(), repo, CompactApprovedInvalidationRequest{
		LineageID: state.LineageID, ExpectedRevision: record.Revision,
		Gate: NativeGateRequestInput{Gate: GateRelease, LineageID: state.LineageID},
	})
	if err == nil || !strings.Contains(err.Error(), "release gate requires complete independent release evidence") || evaluation.Result != "" {
		t.Fatalf("incomplete release invalidation = evaluation %#v, error %v", evaluation, err)
	}
	afterState, _ := os.ReadFile(store.StatePath())
	afterReceipt, _ := os.ReadFile(store.ReceiptPath())
	if !reflect.DeepEqual(afterState, beforeState) || !reflect.DeepEqual(afterReceipt, beforeReceipt) {
		t.Fatal("incomplete release request changed approved authority")
	}
}

func TestInvalidateApprovedCompactAuthorityPreservesAuthorityOnGitInfrastructureFailure(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "reviewed candidate\n")
	writeSnapshotFile(t, repo, "intended.txt", "reviewed untracked\n")
	state := newCompactStartStateForTarget(t, repo, "invalidate-approved-infra", Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{"intended.txt"}})
	state, store, _ := approvedCompactStateFixture(t, repo, state)
	gitSnapshot(t, repo, "add", "tracked.txt", "intended.txt")
	gitSnapshot(t, repo, "commit", "-m", "reviewed candidate")
	writeSnapshotFile(t, repo, "deleted.txt", "next slice in progress\n")
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	beforeState, _ := os.ReadFile(store.StatePath())
	beforeReceipt, _ := os.ReadFile(store.ReceiptPath())

	originalStarter := gitProcessTreeStarter
	t.Cleanup(func() { gitProcessTreeStarter = originalStarter })
	gitProcessTreeStarter = func(command *exec.Cmd) (func() error, error) {
		for _, arg := range command.Args {
			if arg == "--error-unmatch" {
				return nil, errors.New("job object creation rejected")
			}
		}
		return originalStarter(command)
	}

	_, evaluation, err := InvalidateApprovedCompactAuthority(context.Background(), repo, CompactApprovedInvalidationRequest{
		LineageID: state.LineageID, ExpectedRevision: record.Revision,
		Gate: NativeGateRequestInput{Gate: GatePostApply, LineageID: state.LineageID},
	})
	var control *GitProcessControlError
	if err == nil || evaluation.Result != GateInvalidated || !errors.As(evaluation.Cause, &control) {
		t.Fatalf("infrastructure invalidation = evaluation %#v, error %v", evaluation, err)
	}
	afterState, _ := os.ReadFile(store.StatePath())
	afterReceipt, _ := os.ReadFile(store.ReceiptPath())
	if !reflect.DeepEqual(afterState, beforeState) || !reflect.DeepEqual(afterReceipt, beforeReceipt) {
		t.Fatal("infrastructure failure changed approved authority")
	}
}

func TestInvalidateApprovedCompactAuthorityPersistsBoundSemanticDenial(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "approved candidate\n")
	state, store, _ := approvedCompactCurrentChangesFixture(t, repo, "invalidate-approved-untracked", []string{})
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "outside.txt", "outside frozen scope\n")

	invalidated, evaluation, err := InvalidateApprovedCompactAuthority(context.Background(), repo, CompactApprovedInvalidationRequest{
		LineageID: state.LineageID, ExpectedRevision: record.Revision,
		Gate: NativeGateRequestInput{Gate: GatePostApply, LineageID: state.LineageID},
	})
	if err != nil {
		t.Fatalf("persist semantic denial: %v", err)
	}
	if evaluation.Result != GateInvalidated || evaluation.Cause != nil || evaluation.Context.Gate != GatePostApply ||
		evaluation.Context.LineageID != state.LineageID || evaluation.Context.Generation != state.Generation ||
		evaluation.Context.StoreRevision != record.Revision || invalidated.State.State != StateInvalidated {
		t.Fatalf("bound semantic denial = record %#v, evaluation %#v", invalidated, evaluation)
	}
	if invalidated.State.InvalidationEvidence == nil || !reflect.DeepEqual(invalidated.State.InvalidationEvidence.Context, evaluation.Context) {
		t.Fatalf("persisted semantic evidence = %#v", invalidated.State.InvalidationEvidence)
	}
	if _, err := os.Stat(store.ReceiptPath()); !os.IsNotExist(err) {
		t.Fatalf("semantic invalidation retained receipt: %v", err)
	}
}
func TestInvalidateApprovedCompactAuthorityPersistsStagedIntendedUntrackedDenial(t *testing.T) {
	// Reproduces issue #1269: an approved current-changes lineage whose frozen
	// intended-untracked path is later staged/tracked. The native gate derives
	// an invalidated result from the buildCompactLifecycleSnapshot failure path
	// ("intended-untracked path is already tracked"), which is a semantic scope
	// denial rather than an infrastructure fault. Post-apply reaches this path
	// because the staged path leaves DiscoverIntendedUntracked empty, so the
	// earlier untracked-scope guard passes and the failure surfaces only at
	// lifecycle snapshot derivation. The invalidation MUST persist so the
	// authority becomes recovery-eligible instead of staying "approved".
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "intended.txt", "reviewed untracked\n")
	state, store, _ := approvedCompactCurrentChangesFixture(t, repo, "invalidate-approved-staged-intended", []string{"intended.txt"})
	before, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	// Stage the frozen intended-untracked path so it is tracked in the index but
	// absent from HEAD: the exact live state issue #1269 reports.
	gitSnapshot(t, repo, "add", "--", "intended.txt")

	record, evaluation, err := InvalidateApprovedCompactAuthority(context.Background(), repo, CompactApprovedInvalidationRequest{
		LineageID: state.LineageID, ExpectedRevision: before.Revision,
		Gate: NativeGateRequestInput{Gate: GatePostApply, LineageID: state.LineageID},
	})
	if err != nil {
		t.Fatalf("persist staged intended-untracked denial: %v", err)
	}
	if evaluation.Result != GateInvalidated || evaluation.Cause != nil {
		t.Fatalf("staged intended-untracked denial = evaluation %#v", evaluation)
	}
	if !strings.Contains(evaluation.Reason, "current repository target cannot be derived") ||
		!strings.Contains(evaluation.Reason, "already tracked") {
		t.Fatalf("denial did not surface from lifecycle snapshot derivation: %q", evaluation.Reason)
	}
	if evaluation.Context.Denial == nil || evaluation.Context.Denial.Stage != "target-derivation" {
		t.Fatalf("semantic denial lacked target-derivation stage: %#v", evaluation.Context.Denial)
	}
	// Authority must terminally transition to invalidated and drop the receipt.
	if record.State.State != StateInvalidated || record.Revision == before.Revision ||
		record.State.InvalidationReason != evaluation.Reason {
		t.Fatalf("staged intended-untracked invalidation = record %#v", record)
	}
	if record.State.InvalidationEvidence == nil ||
		!reflect.DeepEqual(record.State.InvalidationEvidence.Context, evaluation.Context) {
		t.Fatalf("persisted staged-intended evidence = %#v", record.State.InvalidationEvidence)
	}
	if _, err := os.Stat(store.ReceiptPath()); !os.IsNotExist(err) {
		t.Fatalf("staged intended-untracked invalidation retained receipt: %v", err)
	}
	loaded, err := store.Load()
	if err != nil || !reflect.DeepEqual(loaded, record) {
		t.Fatalf("persisted staged-intended invalidation = %#v, %v", loaded, err)
	}
	// The invalidated authority must be recovery-eligible.
	successor := newCompactTestState(t, repo, "invalidate-approved-staged-intended-g2")
	successor.Generation = record.State.Generation + 1
	recovered, err := RecoverCompactAuthority(context.Background(), repo, CompactRecoveryRequest{
		PredecessorLineageID: record.State.LineageID, ExpectedPredecessorRevision: record.Revision,
		Successor: successor, Disposition: RecoveryInvalidated, Reason: "replace invalidated delivery", Actor: "maintainer",
	})
	if err != nil || recovered.State.Recovery == nil || recovered.State.Recovery.PredecessorRevision != record.Revision {
		t.Fatalf("recover staged intended-untracked invalidated predecessor = %#v, %v", recovered, err)
	}
}

func TestInvalidateApprovedCompactAuthorityAcceptsCanonicalZeroLensReceipt(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "approved candidate\n")
	state := newCompactTestState(t, repo, "invalidate-approved-zero-lens")
	state, store, _ := approvedCompactStateFixture(t, repo, state)
	record, _ := store.Load()
	writeSnapshotFile(t, repo, "outside.txt", "outside scope\n")

	invalidated, _, err := InvalidateApprovedCompactAuthority(context.Background(), repo, CompactApprovedInvalidationRequest{
		LineageID: state.LineageID, ExpectedRevision: record.Revision,
		Gate: NativeGateRequestInput{Gate: GatePostApply, LineageID: state.LineageID},
	})
	if err != nil || invalidated.State.State != StateInvalidated {
		t.Fatalf("zero-lens receipt invalidation = %#v, %v", invalidated, err)
	}
}
func TestInvalidateApprovedCompactAuthorityPreservesAuthorityWhenFinalReleaseDerivationFails(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "candidate")
	state, store, _ := approvedCompactRevisionFixture(t, repo, "invalidate-approved-release-infra")
	record, _ := store.Load()
	beforeState, _ := os.ReadFile(store.StatePath())
	beforeReceipt, _ := os.ReadFile(store.ReceiptPath())
	dir := t.TempDir()
	input := NativeGateRequestInput{Gate: GateRelease, LineageID: state.LineageID}
	for path, target := range map[string]*string{
		"configuration": &input.ReleaseConfiguration, "generated": &input.ReleaseGenerated,
		"provenance": &input.ReleaseProvenance, "boundary": &input.ReleasePublicationBoundary,
		"freshness": &input.ReleaseEvidenceFreshness,
	} {
		*target = filepath.Join(dir, path)
		if err := os.WriteFile(*target, []byte(path+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	originalHook, originalStarter := finalGateAuthorizationHook, gitProcessTreeStarter
	t.Cleanup(func() { finalGateAuthorizationHook, gitProcessTreeStarter = originalHook, originalStarter })
	finalGateAuthorizationHook = func() {
		calls := 0
		gitProcessTreeStarter = func(command *exec.Cmd) (func() error, error) {
			if strings.Contains(strings.Join(command.Args, " "), "rev-parse --verify") {
				calls++
				if calls == 2 {
					return nil, errors.New("job object creation rejected")
				}
			}
			return originalStarter(command)
		}
	}
	_, evaluation, err := InvalidateApprovedCompactAuthority(context.Background(), repo, CompactApprovedInvalidationRequest{
		LineageID: state.LineageID, ExpectedRevision: record.Revision, Gate: input,
	})
	var control *GitProcessControlError
	if err == nil || !errors.As(evaluation.Cause, &control) {
		t.Fatalf("final release derivation = %#v, %v", evaluation, err)
	}
	afterState, _ := os.ReadFile(store.StatePath())
	afterReceipt, _ := os.ReadFile(store.ReceiptPath())
	if !reflect.DeepEqual(afterState, beforeState) || !reflect.DeepEqual(afterReceipt, beforeReceipt) {
		t.Fatal("release infrastructure failure changed approved authority")
	}
}
func TestInvalidateApprovedCompactAuthorityRederivesSemanticDenialBeforePersist(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "approved candidate\n")
	state, store, _ := approvedCompactCurrentChangesFixture(t, repo, "invalidate-approved-race", []string{})
	record, _ := store.Load()
	writeSnapshotFile(t, repo, "outside.txt", "outside scope\n")
	beforeState, _ := os.ReadFile(store.StatePath())
	beforeReceipt, _ := os.ReadFile(store.ReceiptPath())
	originalHook := finalCompactInvalidationHook
	t.Cleanup(func() { finalCompactInvalidationHook = originalHook })
	finalCompactInvalidationHook = func() { writeSnapshotFile(t, repo, "late.txt", "concurrent change\n") }

	_, evaluation, err := InvalidateApprovedCompactAuthority(context.Background(), repo, CompactApprovedInvalidationRequest{
		LineageID: state.LineageID, ExpectedRevision: record.Revision,
		Gate: NativeGateRequestInput{Gate: GatePostApply, LineageID: state.LineageID},
	})
	if err == nil || evaluation.Result != GateInvalidated {
		t.Fatalf("concurrent denial rederivation = %#v, %v", evaluation, err)
	}
	afterState, _ := os.ReadFile(store.StatePath())
	afterReceipt, _ := os.ReadFile(store.ReceiptPath())
	if !reflect.DeepEqual(afterState, beforeState) || !reflect.DeepEqual(afterReceipt, beforeReceipt) {
		t.Fatal("concurrent denial change mutated authority")
	}
}

func approvedCompactStateFixture(t *testing.T, repo string, state CompactState) (CompactState, CompactStore, CompactReceipt) {
	t.Helper()
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.Replace("", "review/start", state)
	if err != nil {
		t.Fatal(err)
	}
	results := make([]LensResult, len(state.SelectedLenses))
	for index, lens := range state.SelectedLenses {
		results[index] = LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"review completed"}}
	}
	if err := state.CompleteReview(CompactReviewInput{LensResults: results, Classifications: []FindingEvidence{}, RefuterOutcomes: []EvidenceResult{}}); err != nil {
		t.Fatal(err)
	}
	revision, err = store.Replace(revision, "review/complete-review", state)
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
	return state, store, receipt
}
