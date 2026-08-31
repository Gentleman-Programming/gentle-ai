package cli

import (
	"bytes"
	"context"
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

// TestStartStatusContinuationCarriesCwd is issue #3932: the review.status
// re-entry a reviewing START emits must bind the repository START received,
// exactly as the START and acknowledgement emissions do. Without --cwd a
// caller whose process cwd is another repository silently preflights that
// repository instead of resuming the frozen lineage.
func TestStartStatusContinuationCarriesCwd(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc Candidate() int { return 5 }\n", 0o644)
	started := runNegotiatedReviewStart(t, repo, "continuation-cwd")
	execution := startStatusContinuationExecution(t, started, []ReviewTransitionArgument{
		{Name: "projection", Value: string(reviewtransaction.ProjectionWorkspace)},
	})
	if len(execution.Arguments) == 0 || execution.Arguments[0].Name != "cwd" || execution.Arguments[0].Value != repo {
		t.Fatalf("START continuation arguments = %#v, want a leading --cwd=%s row", execution.Arguments, repo)
	}

	// Run the emitted command from an unrelated repository: the continuation
	// must still bind the repository START froze, never the process cwd.
	elsewhere := initReviewCLIRepo(t)
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(elsewhere); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(previous) }()
	fields := strings.Fields(execution.Command)
	if len(fields) < 3 || fields[0] != "gentle-ai" || fields[1] != "review" || fields[2] != "status" {
		t.Fatalf("continuation command = %q", execution.Command)
	}
	var statusOutput bytes.Buffer
	if err := RunReview(fields[2:], &statusOutput); err != nil {
		t.Fatalf("continuation STATUS from another cwd: %v\n%s", err, statusOutput.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, statusOutput.Bytes(), &status)
	if status.Authority == nil || status.Authority.LineageID != started.LineageID || status.Authority.State != reviewtransaction.StateReviewing {
		t.Fatalf("continuation STATUS from another cwd bound authority %#v, want lineage %q reviewing", status.Authority, started.LineageID)
	}
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
