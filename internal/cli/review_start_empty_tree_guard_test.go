package cli

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

// TestReviewStartRefusesWhenEmptyBaseTree verifies that RunReview("start")
// does NOT create authority when the snapshot has an empty base_tree.
// The agent reported exactly this scenario: `action: created` with
// `base_tree: None` and `candidate_tree: None`.
func TestReviewStartRefusesWhenEmptyBaseTree(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "file.txt", "content\n", 0o644)
	lineage := "start-empty-base-tree"

	// Override reviewFacadeBuildStartSnapshot to return a snapshot with
	// empty base_tree. This simulates the bug path where trees become empty.
	oldBuildFn := reviewFacadeBuildStartSnapshot
	reviewFacadeBuildStartSnapshot = func(ctx context.Context, builder reviewtransaction.SnapshotBuilder, target reviewtransaction.Target) (reviewtransaction.Snapshot, error) {
		// Get the real snapshot first
		snap, err := builder.Build(ctx, target)
		if err != nil {
			return reviewtransaction.Snapshot{}, err
		}
		// Tamper with the snapshot to have empty base_tree
		snap.BaseTree = ""
		return snap, nil
	}
	t.Cleanup(func() { reviewFacadeBuildStartSnapshot = oldBuildFn })

	var output bytes.Buffer
	if err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2,
		"--cwd", repo, "--lineage", lineage,
	}), &output); err == nil {
		t.Fatal("expected failure when base_tree is empty, got nil")
	}

	failure := decodeReviewIntegrationFailure(t, output.Bytes())
	if failure.Phase != "pre_native" {
		t.Fatalf("phase = %q, want pre_native", failure.Phase)
	}
	if failure.MutationOutcome != ReviewMutationNotStarted {
		t.Fatalf("MutationOutcome = %q, want %q", failure.MutationOutcome, ReviewMutationNotStarted)
	}
	// pre_native failures are not retry-safe; the maintainer must correct
	// the request before attempting again.
	if failure.RetrySafe {
		t.Error("unexpected retry-safe failure")
	}
}

// TestReviewStartRefusesWhenEmptyCandidateTree verifies that RunReview("start")
// does NOT create authority when the snapshot has an empty candidate_tree.
func TestReviewStartRefusesWhenEmptyCandidateTree(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "file.txt", "content\n", 0o644)
	lineage := "start-empty-candidate-tree"

	oldBuildFn := reviewFacadeBuildStartSnapshot
	reviewFacadeBuildStartSnapshot = func(ctx context.Context, builder reviewtransaction.SnapshotBuilder, target reviewtransaction.Target) (reviewtransaction.Snapshot, error) {
		snap, err := builder.Build(ctx, target)
		if err != nil {
			return reviewtransaction.Snapshot{}, err
		}
		snap.CandidateTree = ""
		return snap, nil
	}
	t.Cleanup(func() { reviewFacadeBuildStartSnapshot = oldBuildFn })

	var output bytes.Buffer
	if err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2,
		"--cwd", repo, "--lineage", lineage,
	}), &output); err == nil {
		t.Fatal("expected failure when candidate_tree is empty, got nil")
	}

	failure := decodeReviewIntegrationFailure(t, output.Bytes())
	if failure.Phase != "pre_native" {
		t.Fatalf("phase = %q, want pre_native", failure.Phase)
	}
	if failure.MutationOutcome != ReviewMutationNotStarted {
		t.Fatalf("MutationOutcome = %q, want %q", failure.MutationOutcome, ReviewMutationNotStarted)
	}
	// pre_native failures are not retry-safe; the maintainer must correct
	// the request before attempting again.
	if failure.RetrySafe {
		t.Error("unexpected retry-safe failure")
	}
}

// TestReviewStartNormalFlowStillCreatesAuthority verifies that normal START
// (with valid trees) still creates authority correctly after adding the
// empty-tree guards.
func TestReviewStartNormalFlowStillCreatesAuthority(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "file.txt", "content\n", 0o644)
	lineage := "start-normal-flow"

	var output bytes.Buffer
	if err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2,
		"--cwd", repo, "--lineage", lineage,
	}), &output); err != nil {
		t.Fatalf("normal START failed: %v\nOutput: %s", err, output.String())
	}

	result := decodeNegotiatedReviewStart(t, output.Bytes())
	if result.Action != "created" {
		t.Errorf("action = %q, want 'created'", result.Action)
	}
	if result.BaseTree == "" {
		t.Error("base_tree is empty in successful START")
	}
	if result.CandidateTree == "" {
		t.Error("candidate_tree is empty in successful START")
	}

	// Verify authority was actually created
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatalf("resolve authority: %v", err)
	}
	statePath := store.StatePath()
	if _, statErr := os.Stat(statePath); statErr != nil {
		t.Fatalf("authority file not created: %s", statePath)
	}
}

// TestReviewStartNoAuthorityOnEmptyBaseTree verifies that when START fails
// due to empty base_tree, no authority store is left behind on disk.
func TestReviewStartNoAuthorityOnEmptyBaseTree(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "file.txt", "content\n", 0o644)
	lineage := "start-empty-base-tree-no-authority"

	oldBuildFn := reviewFacadeBuildStartSnapshot
	reviewFacadeBuildStartSnapshot = func(ctx context.Context, builder reviewtransaction.SnapshotBuilder, target reviewtransaction.Target) (reviewtransaction.Snapshot, error) {
		snap, err := builder.Build(ctx, target)
		if err != nil {
			return reviewtransaction.Snapshot{}, err
		}
		snap.BaseTree = ""
		return snap, nil
	}
	t.Cleanup(func() { reviewFacadeBuildStartSnapshot = oldBuildFn })

	var output bytes.Buffer
	if err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2,
		"--cwd", repo, "--lineage", lineage,
	}), &output); err == nil {
		t.Fatal("expected failure when base_tree is empty")
	}

	// Verify no authority store was created
	stores, err := reviewtransaction.DiscoverCompactStores(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(stores) != 0 {
		t.Fatalf("empty-base-tree START persisted authority stores: %#v", stores)
	}
}
