package cli

import (
	"bytes"
	"context"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

// TestStartRefusesEmptyBaseTree verifies that START refuses to create authority
// when the snapshot reports an empty base_tree, even if the build succeeded.
// This guards against the agent-reported "base_tree: None" failure pattern.
func TestStartRefusesEmptyBaseTree(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "candidate\n", 0o644)
	lineage := "start-empty-base-tree"

	originalBuild := reviewFacadeBuildStartSnapshot
	reviewFacadeBuildStartSnapshot = func(ctx context.Context, builder reviewtransaction.SnapshotBuilder, target reviewtransaction.Target) (reviewtransaction.Snapshot, error) {
		// Return a snapshot with empty BaseTree but valid CandidateTree.
		snapshot, err := originalBuild(ctx, builder, target)
		if err != nil {
			return snapshot, err
		}
		snapshot.BaseTree = ""
		return snapshot, nil
	}
	t.Cleanup(func() { reviewFacadeBuildStartSnapshot = originalBuild })

	var output bytes.Buffer
	if err := RunReview(boundNegotiatedStartArgs(t, []string{"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo, "--lineage", lineage}), &output); err == nil {
		t.Fatal("START with empty base_tree unexpectedly succeeded")
	}
	failure := decodeReviewIntegrationFailure(t, output.Bytes())
	if failure.Phase != "pre_native" {
		t.Fatalf("phase = %q, want pre_native", failure.Phase)
	}
	if failure.MutationOutcome != ReviewMutationNotStarted {
		t.Fatalf("MutationOutcome = %q, want %q", failure.MutationOutcome, ReviewMutationNotStarted)
	}
	if !failure.RetrySafe {
		t.Error("expected retry-safe failure")
	}
	if !failure.RetrySafe || failure.Replayability != reviewtransaction.ReplayabilityNotReplayable ||
		failure.NextAction != "correct_request" || failure.LineageID != lineage {
		t.Fatalf("empty-base-tree failure = %#v", failure)
	}
}

// TestStartRefusesEmptyCandidateTree verifies that START refuses to create
// authority when the snapshot reports an empty candidate_tree, even if the
// build succeeded. This guards against the agent-reported
// "candidate_tree: None" failure pattern.
func TestStartRefusesEmptyCandidateTree(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "candidate\n", 0o644)
	lineage := "start-empty-candidate-tree"

	originalBuild := reviewFacadeBuildStartSnapshot
	reviewFacadeBuildStartSnapshot = func(ctx context.Context, builder reviewtransaction.SnapshotBuilder, target reviewtransaction.Target) (reviewtransaction.Snapshot, error) {
		snapshot, err := originalBuild(ctx, builder, target)
		if err != nil {
			return snapshot, err
		}
		snapshot.CandidateTree = ""
		return snapshot, nil
	}
	t.Cleanup(func() { reviewFacadeBuildStartSnapshot = originalBuild })

	var output bytes.Buffer
	if err := RunReview(boundNegotiatedStartArgs(t, []string{"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo, "--lineage", lineage}), &output); err == nil {
		t.Fatal("START with empty candidate_tree unexpectedly succeeded")
	}
	failure := decodeReviewIntegrationFailure(t, output.Bytes())
	if failure.Phase != "pre_native" {
		t.Fatalf("phase = %q, want pre_native", failure.Phase)
	}
	if failure.MutationOutcome != ReviewMutationNotStarted {
		t.Fatalf("MutationOutcome = %q, want %q", failure.MutationOutcome, ReviewMutationNotStarted)
	}
	if !failure.RetrySafe {
		t.Error("expected retry-safe failure")
	}
}

// TestStartNoAuthorityCreatedOnEmptyTree verifies that when START fails due
// to empty trees, no authority store is left behind on disk.
func TestStartNoAuthorityCreatedOnEmptyTree(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "candidate\n", 0o644)
	lineage := "start-empty-tree-no-authority"

	originalBuild := reviewFacadeBuildStartSnapshot
	reviewFacadeBuildStartSnapshot = func(ctx context.Context, builder reviewtransaction.SnapshotBuilder, target reviewtransaction.Target) (reviewtransaction.Snapshot, error) {
		snapshot, err := originalBuild(ctx, builder, target)
		if err != nil {
			return snapshot, err
		}
		snapshot.BaseTree = ""
		return snapshot, nil
	}
	t.Cleanup(func() { reviewFacadeBuildStartSnapshot = originalBuild })

	var output bytes.Buffer
	if err := RunReview(boundNegotiatedStartArgs(t, []string{"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo, "--lineage", lineage}), &output); err == nil {
		t.Fatal("START unexpectedly succeeded")
	}
	failure := decodeReviewIntegrationFailure(t, output.Bytes())
	if failure.MutationOutcome != ReviewMutationNotStarted {
		t.Fatalf("MutationOutcome = %q, want %q", failure.MutationOutcome, ReviewMutationNotStarted)
	}

	// Verify no authority store was created.
	stores, err := reviewtransaction.DiscoverCompactStores(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(stores) != 0 {
		t.Fatalf("empty-tree START persisted authority stores: %#v", stores)
	}
}
