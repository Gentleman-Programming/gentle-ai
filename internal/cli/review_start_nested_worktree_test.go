package cli

// Issue #1881: a nested, non-gitignored git worktree (`git worktree add
// .wt/test`) made `review start` fail hard with "logical path is not
// canonical: \".wt/test/\"" -- exit 1 and, in the plain form, an empty stdout.
// These tests pin the fix end to end: START succeeds with the nested worktree
// present and provably excludes the worktree's contents from the frozen
// changed-path manifest.
//
// Issue #2394 removed the second half of the original story. START no longer
// enumerates untracked workspace content at all, so neither a nested worktree
// nor an embedded foreign repository can reach the candidate, and the
// untracked-scope refusal that used to fire here has nothing left to refuse.
// The refusal still exists where the question is still asked: the delivery
// gates, which do inspect the live worktree.

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
	writeReviewStartCandidate(t, repo, "extra-notes.txt", "declared candidate file\n", 0o644)
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

// TestNegotiatedReviewStartIgnoresEmbeddedForeignRepository replaces the test
// that pinned the opposite outcome. An embedded foreign clone used to block
// START, because the sweep tried to make its directory part of the candidate
// and Git refuses to enumerate inside it. #2394 stopped the sweep, so an
// embedded clone is now ordinary undeclared workspace content: START succeeds
// and the clone never appears in the frozen manifest.
func TestNegotiatedReviewStartIgnoresEmbeddedForeignRepository(t *testing.T) {
	repo := initReviewCLIRepo(t)
	runReviewCLIGit(t, repo, "init", "-q", filepath.Join(repo, "vendor", "embedded"))
	writeReviewStartCandidate(t, repo, "tracked.txt", "candidate\n", 0o644)

	var output bytes.Buffer
	if err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", "embedded-repository-2394",
	}), &output); err != nil {
		t.Fatalf("negotiated review start beside an embedded foreign repository: %v\n%s", err, output.String())
	}
	result := decodeNegotiatedReviewStart(t, output.Bytes())
	if result.ChangedPathManifest == nil {
		t.Fatalf("negotiated START carries no changed-path manifest:\n%s", output.String())
	}
	for _, entry := range *result.ChangedPathManifest {
		if strings.HasPrefix(entry.Path, "vendor/") {
			t.Fatalf("frozen manifest admitted embedded repository content %q", entry.Path)
		}
	}
	if len(*result.ChangedPathManifest) != 1 || (*result.ChangedPathManifest)[0].Path != "tracked.txt" {
		t.Fatalf("frozen manifest = %#v, want only the declared change", *result.ChangedPathManifest)
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
