package cli

// Issue: negotiated `review.start` with TargetCurrentChanges on a clean,
// fully-committed worktree freezes an EMPTY candidate (base_tree ==
// candidate_tree == HEAD, paths: []), passes risk assessment and
// pre-commit, then fails late and misleadingly at pre-push ("reviewed
// delivery is not exactly one commit from its reviewed base"). These tests
// pin a typed preflight refusal that stops this flow before any authority
// is created, naming --base-ref as the resolution instead of silently
// deriving one.

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestNegotiatedReviewStartRefusesEmptyCandidateWithoutAuthority proves the
// core fix: a clean, fully-committed worktree refuses negotiated START with
// a typed, schema-valid preflight refusal, and persists nothing.
func TestNegotiatedReviewStartRefusesEmptyCandidateWithoutAuthority(t *testing.T) {
	repo := initReviewCLIRepo(t)

	var output bytes.Buffer
	err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", "empty-candidate",
	}), &output)
	if err == nil {
		t.Fatalf("negotiated review start admitted an empty candidate:\n%s", output.String())
	}

	failure := decodeReviewIntegrationFailure(t, output.Bytes())
	assertReviewFailureMatchesPublishedSchema(t, failure)

	if failure.Code != "empty_candidate_scope" {
		t.Fatalf("negotiated failure code = %q, want empty_candidate_scope\n%s", failure.Code, output.String())
	}
	if failure.Phase != "preflight" {
		t.Fatalf("negotiated failure phase = %q, want preflight\n%s", failure.Phase, output.String())
	}
	if failure.MutationOutcome != ReviewMutationNotStarted {
		t.Fatalf("negotiated failure mutation_outcome = %q, want not_started\n%s", failure.MutationOutcome, output.String())
	}
	if len(failure.RequiredInputs) != 1 || failure.RequiredInputs[0] != "base_ref" {
		t.Fatalf("negotiated failure required_inputs = %v, want [base_ref]\n%s", failure.RequiredInputs, output.String())
	}
	if failure.NextAction != "correct_request" {
		t.Fatalf("negotiated failure next_action = %q, want correct_request\n%s", failure.NextAction, output.String())
	}

	stores, discoverErr := reviewtransaction.DiscoverCompactStores(context.Background(), repo)
	if discoverErr != nil {
		t.Fatal(discoverErr)
	}
	if len(stores) != 0 {
		t.Fatalf("refused empty-candidate START created authority: %#v", stores)
	}
}

// TestNegotiatedEmptyCandidateRefusalNamesBaseRefWithoutDerivingIt proves the
// refusal text names --base-ref as the way forward and never silently
// derives or proposes a base ref such as HEAD~1.
func TestNegotiatedEmptyCandidateRefusalNamesBaseRefWithoutDerivingIt(t *testing.T) {
	repo := initReviewCLIRepo(t)

	var output bytes.Buffer
	err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", "empty-candidate-cause",
	}), &output)
	if err == nil {
		t.Fatalf("negotiated review start admitted an empty candidate:\n%s", output.String())
	}

	failure := decodeReviewIntegrationFailure(t, output.Bytes())
	if failure.Cause != reviewStartEmptyCandidateHint {
		t.Fatalf("negotiated failure cause = %q, want %q", failure.Cause, reviewStartEmptyCandidateHint)
	}
	if !strings.Contains(failure.Cause, "--base-ref") {
		t.Fatalf("negotiated failure cause = %q, want it to name --base-ref", failure.Cause)
	}
	if strings.Contains(failure.Cause, "HEAD~1") || strings.Contains(failure.Cause, "HEAD^") {
		t.Fatalf("negotiated failure cause auto-derives a base ref: %q", failure.Cause)
	}
}

// TestNegotiatedStartWithExplicitBaseRefOverCommittedWorkStillStarts proves
// the refusal only fires for an actually empty manifest: an explicit
// --base-ref naming a real prior commit still builds a non-empty candidate
// and proceeds normally.
func TestNegotiatedStartWithExplicitBaseRefOverCommittedWorkStillStarts(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "second commit\n", 0o644)
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "second")

	var output bytes.Buffer
	if err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", "explicit-base-ref",
		"--base-ref", "HEAD~1",
	}), &output); err != nil {
		t.Fatalf("negotiated start with an explicit base ref over committed work: %v\n%s", err, output.String())
	}
	result := decodeNegotiatedReviewStart(t, output.Bytes())
	if result.ChangedFiles == 0 {
		t.Fatalf("negotiated start with an explicit base ref over committed work produced an empty candidate: %#v", result)
	}
}

// TestNegotiatedStartWithPendingChangesIsUnaffected proves the guard never
// fires when the worktree actually has pending changes.
func TestNegotiatedStartWithPendingChangesIsUnaffected(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "pending change\n", 0o644)

	result := runNegotiatedReviewStart(t, repo, "pending-changes")
	if result.ChangedFiles == 0 {
		t.Fatalf("negotiated start with pending changes produced an empty candidate: %#v", result)
	}
}

// TestUnnegotiatedEmptyCandidateStartKeepsItsHint is a regression pin: the
// plain (non-negotiated) review start path on a clean tree must keep
// emitting its existing hint unchanged by the negotiated-only guard added
// by this change.
func TestUnnegotiatedEmptyCandidateStartKeepsItsHint(t *testing.T) {
	repo := initReviewCLIRepo(t)

	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "unnegotiated-empty"}, &output); err != nil {
		t.Fatalf("unnegotiated review start on a clean worktree: %v\n%s", err, output.String())
	}
	var started ReviewFacadeStartResult
	if err := json.Unmarshal(output.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	if started.ChangedFiles != 0 {
		t.Fatalf("unnegotiated clean-worktree start changed_files = %d, want 0", started.ChangedFiles)
	}
	if started.Hint != reviewStartEmptyCandidateHint {
		t.Fatalf("unnegotiated empty-candidate start hint = %q, want %q", started.Hint, reviewStartEmptyCandidateHint)
	}
}
