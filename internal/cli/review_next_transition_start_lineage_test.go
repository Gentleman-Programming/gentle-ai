package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestReviewStatusNamesAStartLineageThatCanActuallyBeStarted is the
// discoverability proof for the worst half of journey
// j39-sdd-remediation-stranded-successor: a next transition that is named, runs
// exactly as printed, exits 0, and changes nothing.
//
// The lineage name a `review start` derives when the operator names none is a
// function of the target identity alone, so once a lineage has been started for
// a target that name is taken for good. When the leaf that superseded it froze
// a different target, negotiated status correctly reports the live candidate as
// governed by nothing and routes to `review start` — but the command it printed
// carried no --lineage, so the store found the superseded record occupying the
// derived name and answered blocked-scope-action at exit 0.
//
// A named continuation that runs and does nothing is the most expensive
// refusal in this product, because the operator then trusts it. This test runs
// the printed transition verbatim and requires authority to exist afterwards.
func TestReviewStatusNamesAStartLineageThatCanActuallyBeStarted(t *testing.T) {
	repo := newSupersededDerivedLineageRepo(t)

	var statusOut bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV1, "--next-transition",
	}, &statusOut); err != nil {
		t.Fatalf("review status: %v\n%s", err, statusOut.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, statusOut.Bytes(), &status)
	if status.Applicability != reviewtransaction.TargetApplicabilityUnrelated {
		t.Fatalf("fixture no longer reproduces the shape: applicability = %q", status.Applicability)
	}
	transition := status.NextTransition
	if transition == nil || transition.Kind != reviewNextTransitionExecute || transition.Execute == nil ||
		transition.Execute.Operation != "review.start" {
		t.Fatalf("status routed to %#v, not an executable start", transition)
	}

	// Run the printed transition exactly as printed, with no repair.
	arguments := []string{"start", "--cwd", repo}
	for _, argument := range transition.Execute.Arguments {
		if argument.Token == "" {
			t.Fatalf("argument %q carried no runnable token", argument.Name)
		}
		arguments = append(arguments, argument.Token)
	}
	var started bytes.Buffer
	if err := RunReview(arguments, &started); err != nil {
		t.Fatalf("the transition the status named does not run: review %v: %v\n%s", arguments, err, started.String())
	}
	var result ReviewIntegrationStartResult
	decodeStrictReviewJSON(t, started.Bytes(), &result)
	if result.Action == string(reviewtransaction.CompactStartBlocked) {
		t.Fatalf("the transition the status named ran, exited 0 and changed nothing:\ncommand: %s\nresult: %s",
			transition.Execute.Command, started.String())
	}
	if result.Action != string(reviewtransaction.CompactStartCreated) || result.State != reviewtransaction.StateReviewing {
		t.Fatalf("the transition the status named produced %#v", result)
	}
	if !strings.Contains(transition.Execute.Command, "--lineage="+result.LineageID) {
		t.Fatalf("the printed command does not name the lineage it created: %s", transition.Execute.Command)
	}
}

// newSupersededDerivedLineageRepo builds the exact shape: an approved lineage
// carrying the derived name for the live target, a recovery successor that
// superseded it, and a live candidate that is back to what the predecessor
// approved and that the successor's frozen target no longer matches.
func newSupersededDerivedLineageRepo(t *testing.T) string {
	t.Helper()
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	writeCLIAttemptFile(t, filepath.Join(repo, "docs", "attempt.md"), "# attempt\n\nplain prose.\n")
	runReviewCLIGit(t, repo, "add", ".")

	var started bytes.Buffer
	if err := RunReview([]string{"start", "--cwd", repo}, &started); err != nil {
		t.Fatalf("review start: %v\n%s", err, started.String())
	}
	var startResult ReviewFacadeStartResult
	decodeStrictReviewJSON(t, started.Bytes(), &startResult)
	derived := startResult.LineageID
	if want := reviewDerivedStartLineage(startResult.TargetIdentity); want != "" && derived != want {
		t.Fatalf("start derived lineage %q, want %q", derived, want)
	}
	var finalized bytes.Buffer
	if err := RunReview([]string{"finalize", "--cwd", repo}, &finalized); err != nil {
		t.Fatalf("review finalize: %v\n%s", err, finalized.String())
	}

	widened := filepath.Join(repo, "docs", "widened.md")
	writeCLIAttemptFile(t, widened, "# widened\n")
	runReviewCLIGit(t, repo, "add", widened)
	createCLIRecoverySuccessor(t, repo, derived, "superseding-successor")
	runReviewCLIGit(t, repo, "rm", "-q", "--cached", widened)
	if err := os.Remove(widened); err != nil {
		t.Fatal(err)
	}
	return repo
}
