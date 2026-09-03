package cli

// Issue #2586: `review start` with a TargetCurrentChanges candidate on a
// clean, fully-committed worktree, or a TargetBaseDiff candidate whose named
// base nets zero changed paths, freezes an EMPTY candidate (paths: []) that
// would pass risk assessment and mint an approved receipt inspecting
// nothing -- discoverable afterward as "governing" a later, genuinely
// unreviewed candidate sharing its final tree. The guard used to fire only
// on the negotiated route and only for TargetCurrentChanges; these tests pin
// the unified refusal that now fires on both routes and both target kinds,
// before any authority is created, naming --base-ref as the resolution
// instead of silently deriving one.

import (
	"bytes"
	"context"
	"os"
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
	if entries, readErr := os.ReadDir(reviewDefectReportDir(t, repo)); readErr == nil && len(entries) != 0 {
		t.Fatalf("preflight refusal wrote defect reports: %v", entries)
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
	reviewEnabledHome(t)
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
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "pending change\n", 0o644)

	result := runNegotiatedReviewStart(t, repo, "pending-changes")
	if result.ChangedFiles == 0 {
		t.Fatalf("negotiated start with pending changes produced an empty candidate: %#v", result)
	}
}

// TestDirectReviewStartRefusesEmptyCandidateWithoutAuthority is the direct-
// route repro from issue #2586: before this fix, a plain (non-negotiated)
// `review start` (no --contract) on a clean, fully-committed worktree
// created a lineage and closed it as an approved receipt that inspected
// nothing (base_tree == candidate_tree == HEAD) -- exactly the zero-delta
// receipt the issue reports being discovered as "governing" a later,
// genuinely unreviewed candidate that happens to share its final tree. The
// guard that used to fire only on the negotiated route now fires here too,
// before any authority is created.
func TestDirectReviewStartRefusesEmptyCandidateWithoutAuthority(t *testing.T) {
	repo := initReviewCLIRepo(t)

	var output bytes.Buffer
	err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "unnegotiated-empty"}, &output)
	if err == nil {
		t.Fatalf("direct review start admitted an empty candidate:\n%s", output.String())
	}
	if !strings.Contains(err.Error(), reviewStartEmptyCandidateHint) {
		t.Fatalf("direct empty-candidate refusal = %q, want it to contain %q", err.Error(), reviewStartEmptyCandidateHint)
	}
	if !strings.Contains(err.Error(), "--base-ref") {
		t.Fatalf("direct empty-candidate refusal = %q, want it to name --base-ref", err.Error())
	}

	stores, discoverErr := reviewtransaction.DiscoverCompactStores(context.Background(), repo)
	if discoverErr != nil {
		t.Fatal(discoverErr)
	}
	if len(stores) != 0 {
		t.Fatalf("refused direct-route empty-candidate START created authority: %#v", stores)
	}
}

// TestNegotiatedReviewStartRefusesEmptyBaseDiffCandidate proves the guard's
// #2586 extension: a TargetBaseDiff candidate whose named base nets zero
// changed paths (here, the named base IS the current HEAD -- a truly-empty
// diff) refuses identically to a clean TargetCurrentChanges candidate, on
// the negotiated route.
func TestNegotiatedReviewStartRefusesEmptyBaseDiffCandidate(t *testing.T) {
	repo := initReviewCLIRepo(t)

	var output bytes.Buffer
	err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", "empty-base-diff",
		"--base-ref", "HEAD",
	}), &output)
	if err == nil {
		t.Fatalf("negotiated review start admitted a zero-delta base-diff candidate:\n%s", output.String())
	}

	failure := decodeReviewIntegrationFailure(t, output.Bytes())
	if failure.Code != "empty_candidate_scope" {
		t.Fatalf("negotiated base-diff failure code = %q, want empty_candidate_scope\n%s", failure.Code, output.String())
	}
	if failure.Phase != "preflight" {
		t.Fatalf("negotiated base-diff failure phase = %q, want preflight\n%s", failure.Phase, output.String())
	}

	stores, discoverErr := reviewtransaction.DiscoverCompactStores(context.Background(), repo)
	if discoverErr != nil {
		t.Fatal(discoverErr)
	}
	if len(stores) != 0 {
		t.Fatalf("refused zero-delta base-diff START created authority: %#v", stores)
	}
}

// TestDirectReviewStartRefusesEmptyBaseDiffCandidate is the same proof on the
// direct (non-negotiated) route: issue #2586's guard extension applies to
// both target kinds on both routes, not only the negotiated one.
func TestDirectReviewStartRefusesEmptyBaseDiffCandidate(t *testing.T) {
	repo := initReviewCLIRepo(t)

	var output bytes.Buffer
	err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "unnegotiated-empty-base-diff", "--base-ref", "HEAD"}, &output)
	if err == nil {
		t.Fatalf("direct review start admitted a zero-delta base-diff candidate:\n%s", output.String())
	}
	if !strings.Contains(err.Error(), reviewStartEmptyCandidateHint) {
		t.Fatalf("direct base-diff refusal = %q, want it to contain %q", err.Error(), reviewStartEmptyCandidateHint)
	}

	stores, discoverErr := reviewtransaction.DiscoverCompactStores(context.Background(), repo)
	if discoverErr != nil {
		t.Fatal(discoverErr)
	}
	if len(stores) != 0 {
		t.Fatalf("refused direct-route zero-delta base-diff START created authority: %#v", stores)
	}
}
