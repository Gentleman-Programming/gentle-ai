package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestNextTransitionV1ApprovedPendingAcknowledgementIsNotTerminalStop is
// issue #3940 (and the contract half of #3928): an approved lineage whose
// acknowledgement is still pending must route the v1 caller to the same
// review.acknowledge-approved execute transition v2 receives. The
// acknowledgement is not v2-specific, so native_stop_required was a wrong
// exit that stranded every v1 consumer one step before the burn.
func TestNextTransitionV1ApprovedPendingAcknowledgementIsNotTerminalStop(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "docs/approved.md", "# Approved\n", 0o644)
	var startOutput bytes.Buffer
	if err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", "v1-approved-pending",
	}), &startOutput); err != nil {
		t.Fatal(negotiatedReviewStartFailure(err, startOutput.String()))
	}
	started := decodeNegotiatedReviewStart(t, startOutput.Bytes())
	if started.State != reviewtransaction.StateApproved {
		t.Fatalf("v1 zero-lens START state = %q, want approved", started.State)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	pending, present := reviewtransaction.PendingApprovedCompactAcknowledgement(record)
	if !present {
		t.Fatalf("v1 zero-lens START left no pending acknowledgement: %#v", record.State.State)
	}

	var statusOutput bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--lineage", started.LineageID,
		"--contract", ReviewIntegrationContractV1, "--next-transition",
	}, &statusOutput); err != nil {
		t.Fatalf("v1 approved STATUS: %v\n%s", err, statusOutput.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, statusOutput.Bytes(), &status)
	transition := status.NextTransition
	if transition == nil || transition.Kind != reviewNextTransitionExecute || transition.ReasonCode != "approved_acknowledgement_required" ||
		transition.Execute == nil || transition.Execute.Operation != "review.acknowledge-approved" {
		t.Fatalf("v1 approved pending-acknowledgement transition = %#v, want execute review.acknowledge-approved", transition)
	}
	assertApprovedAcknowledgementTransition(t, transition.Execute, repo, started.LineageID, pending.TargetIdentity, pending.ExpectedRevision)
}

// TestNegotiatedStatusOverlayWithoutBaseRefIsInvalidRequestWithCause is issue
// #3935: a selector combination STATUS cannot honor is the caller's request to
// fix, so the negotiated envelope must say invalid_request with the cause and
// correct_request, never the cause-free operation_failed catch-all whose
// retry can never succeed.
func TestNegotiatedStatusOverlayWithoutBaseRefIsInvalidRequestWithCause(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	var output bytes.Buffer
	err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2, "--next-transition", "--workspace-overlay",
	}, &output)
	if err == nil {
		t.Fatalf("negotiated STATUS accepted --workspace-overlay without --base-ref:\n%s", output.String())
	}
	failure := decodeReviewIntegrationFailure(t, output.Bytes())
	if failure.Operation != "review.status" || failure.Code != reviewIntegrationInvalidRequestCode || failure.NextAction != "correct_request" {
		t.Fatalf("overlay-without-base STATUS failure = %#v, want invalid_request/correct_request", failure)
	}
	if !strings.Contains(failure.Cause, "--workspace-overlay") || !strings.Contains(failure.Cause, "--base-ref") ||
		!strings.Contains(failure.Cause, "gentle-ai review status") {
		t.Fatalf("overlay-without-base cause = %q, want the exact flag combination and the runnable STATUS continuation", failure.Cause)
	}
}

// TestNegotiatedStatusWithUnknownLineageFailsClosedFromForeignRepository is
// issue #3932: the review.status re-entry START emits binds its lineage
// through the opaque repository context START already published. Run from a
// process cwd inside an unrelated repository, it must fail closed with a typed
// refusal naming the lineage and the repository searched, never fall back to
// a fresh-target preflight of that repository. The same command from the
// owning repository resumes the reviewing authority.
func TestNegotiatedStatusWithUnknownLineageFailsClosedFromForeignRepository(t *testing.T) {
	reviewEnabledHome(t)
	owner := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, owner, "candidate.go", "package candidate\n\nfunc Candidate() int { return 6 }\n", 0o644)
	started := runNegotiatedReviewStart(t, owner, "continuation-foreign-cwd")
	execution := startStatusContinuationExecution(t, started, []ReviewTransitionArgument{
		{Name: "projection", Value: string(reviewtransaction.ProjectionWorkspace)},
	})
	if binding := execution.Arguments[3]; binding.Name != "repository-context" || started.RepositoryContext == nil || binding.Value != started.RepositoryContext.Handle {
		t.Fatalf("START continuation binding row = %#v, want the published repository context handle", execution.Arguments)
	}
	fields := strings.Fields(execution.Command)

	foreign := initReviewCLIRepo(t)
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(foreign); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(previous) }()
	var foreignOutput bytes.Buffer
	err = RunReview(fields[2:], &foreignOutput)
	if err == nil {
		t.Fatalf("continuation from a foreign repository succeeded instead of failing closed:\n%s", foreignOutput.String())
	}
	failure := decodeReviewIntegrationFailure(t, foreignOutput.Bytes())
	if failure.Operation != "review.status" || failure.Code != reviewIntegrationInvalidRequestCode || failure.NextAction != "correct_request" ||
		!strings.Contains(failure.Cause, started.LineageID) || !strings.Contains(failure.Cause, "gentle-ai review status --cwd") {
		t.Fatalf("foreign-cwd continuation failure = %#v, want invalid_request naming the lineage and the --cwd continuation", failure)
	}
	var preflight *reviewIntegrationPreflightError
	if !errors.As(err, &preflight) || !strings.Contains(preflight.Error(), foreign) {
		t.Fatalf("foreign-cwd refusal = %v, want it to name the searched repository %s", err, foreign)
	}
	if occupied, err := reviewtransaction.ExactReviewLineageOccupied(context.Background(), foreign, started.LineageID); err != nil || occupied {
		t.Fatalf("foreign repository lineage occupancy = %v, %v; the refusal must create nothing", occupied, err)
	}

	if err := os.Chdir(owner); err != nil {
		t.Fatal(err)
	}
	var ownerOutput bytes.Buffer
	if err := RunReview(fields[2:], &ownerOutput); err != nil {
		t.Fatalf("continuation from the owning repository: %v\n%s", err, ownerOutput.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, ownerOutput.Bytes(), &status)
	if status.Authority == nil || status.Authority.LineageID != started.LineageID || status.Authority.State != reviewtransaction.StateReviewing {
		t.Fatalf("owning-repository continuation authority = %#v, want lineage %q reviewing", status.Authority, started.LineageID)
	}
}
