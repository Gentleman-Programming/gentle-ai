package cli

// Issue #1881: a nested, non-gitignored git worktree (`git worktree add
// .wt/test`) made `review start` fail hard with "logical path is not
// canonical: \".wt/test/\"" — exit 1 and, in the plain form, an empty stdout.
// These tests pin the fix end to end: START succeeds with the nested worktree
// present and provably excludes the worktree's contents from the frozen
// changed-path manifest, and the one refusal that remains for the related
// shape (an embedded foreign repository) reaches a negotiated caller as a
// parseable typed failure envelope on stdout, never as a bare stderr line.

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestReviewStartExcludesNestedWorktreeFromFrozenManifest(t *testing.T) {
	repo := initReviewCLIRepo(t)
	runReviewCLIGit(t, repo, "worktree", "add", "-q", "-b", "nested-1881", filepath.Join(repo, ".wt", "test"))
	writeReviewStartCandidate(t, repo, "tracked.txt", "candidate\n", 0o644)
	writeReviewStartCandidate(t, repo, "extra-notes.txt", "untracked candidate file\n", 0o644)
	writeReviewStartCandidate(t, repo, ".wt/stray-note.txt", "beside the worktree, not inside it\n", 0o644)

	var output bytes.Buffer
	if err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", "nested-worktree-1881",
	}), &output); err != nil {
		t.Fatalf("negotiated review start with a nested non-ignored worktree: %v\n%s", err, output.String())
	}
	result := decodeNegotiatedReviewStart(t, output.Bytes())
	if result.ChangedPathManifest == nil {
		t.Fatalf("negotiated START carries no changed-path manifest:\n%s", output.String())
	}
	manifest := map[string]bool{}
	for _, entry := range *result.ChangedPathManifest {
		manifest[entry.Path] = true
		if entry.Path == ".wt/test" || strings.HasPrefix(entry.Path, ".wt/test/") {
			t.Fatalf("frozen manifest admitted nested worktree content %q", entry.Path)
		}
	}
	for _, want := range []string{"tracked.txt", "extra-notes.txt", ".wt/stray-note.txt"} {
		if !manifest[want] {
			t.Fatalf("frozen manifest %v is missing %q", *result.ChangedPathManifest, want)
		}
	}
}

func TestNegotiatedReviewStartEmbeddedRepositoryRefusalCarriesFailureEnvelope(t *testing.T) {
	repo := initReviewCLIRepo(t)
	runReviewCLIGit(t, repo, "init", "-q", filepath.Join(repo, "vendor", "embedded"))
	writeReviewStartCandidate(t, repo, "tracked.txt", "candidate\n", 0o644)

	var output bytes.Buffer
	err := RunReview([]string{
		"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--projection", "workspace",
		"--target", "sha256:0000000000000000000000000000000000000000000000000000000000000000",
	}, &output)
	if err == nil {
		t.Fatalf("negotiated review start admitted an embedded foreign repository:\n%s", output.String())
	}
	if output.Len() == 0 {
		t.Fatal("negotiated review start failed with an EMPTY stdout; a negotiated caller always receives a parseable envelope")
	}
	var failure ReviewIntegrationFailure
	if decodeErr := json.Unmarshal(output.Bytes(), &failure); decodeErr != nil {
		t.Fatalf("negotiated failure stdout is not a parseable envelope: %v\n%s", decodeErr, output.String())
	}
	if validateErr := failure.Validate(); validateErr != nil {
		t.Fatalf("negotiated failure envelope is invalid: %v\n%s", validateErr, output.String())
	}
	if failure.Code != "untracked_scope_undiscoverable" {
		t.Fatalf("negotiated failure code = %q, want untracked_scope_undiscoverable\n%s", failure.Code, output.String())
	}
	// The failure happens strictly before any authority is created, so the
	// envelope must say so instead of claiming an unknown mutation outcome.
	if failure.MutationOutcome != ReviewMutationNotStarted {
		t.Fatalf("negotiated failure mutation_outcome = %q, want not_started", failure.MutationOutcome)
	}
	if failure.Cause == "" {
		t.Fatal("negotiated failure envelope carries no cause prose for the blocking repository shape")
	}
}

func TestReviewStartKeepsSiblingLinkedWorktreeWorking(t *testing.T) {
	repo := initReviewCLIRepo(t)
	sibling := filepath.Join(t.TempDir(), "sibling-worktree")
	runReviewCLIGit(t, repo, "worktree", "add", "-q", "-b", "sibling-1881", sibling)
	writeReviewStartCandidate(t, repo, "tracked.txt", "candidate\n", 0o644)

	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "sibling-worktree-1881"}, &output); err != nil {
		t.Fatalf("review start with a sibling linked worktree: %v", err)
	}
	var started ReviewFacadeStartResult
	if err := json.Unmarshal(output.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	if started.State != reviewtransaction.StateReviewing && started.State != reviewtransaction.StateApproved {
		t.Fatalf("sibling-worktree start state = %q", started.State)
	}
}
