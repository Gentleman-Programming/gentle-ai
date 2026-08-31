package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestExcludedUntrackedDeclarationSurvivesReviewingReentry is the #3120 shape:
// a lineage frozen with --untracked-scope exclude, then re-entered through the
// three routes a consumer actually runs. None of them may ask for the untracked
// selection again, because the frozen authority already holds that decision.
func TestExcludedUntrackedDeclarationSurvivesReviewingReentry(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc Candidate() int { return 1 }\n", 0o644)
	writeUndeclaredWorkspaceFile(t, repo, "scratch/mockup.txt", "kept outside the candidate\n", 0o644)
	started := runNegotiatedReviewStartExcludingUntracked(t, repo, "excluded-untracked-reentry")
	if started.Action != "created" || started.State != reviewtransaction.StateReviewing {
		t.Fatalf("START = %s/%s", started.Action, started.State)
	}

	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(previous) }()

	assertReentry := func(route string, args []string) {
		t.Helper()
		var output bytes.Buffer
		if err := RunReview(args, &output); err != nil {
			t.Fatalf("%s: %v\n%s", route, err, output.String())
		}
		var status ReviewTargetStatusResult
		decodeStrictReviewJSON(t, output.Bytes(), &status)
		if status.Authority == nil || status.Authority.LineageID != started.LineageID {
			t.Fatalf("%s bound %#v, want lineage %s", route, status.Authority, started.LineageID)
		}
		if status.NextTransition == nil || status.NextTransition.ReasonCode != "reviewer_results_required" {
			t.Fatalf("%s re-asked the frozen untracked decision: %#v", route, status.NextTransition)
		}
	}

	continuationArgs := func(result ReviewIntegrationStartResult) []string {
		t.Helper()
		if result.NextTransition == nil || result.NextTransition.Execute == nil {
			t.Fatalf("START published no status continuation: %#v", result.NextTransition)
		}
		fields := strings.Fields(result.NextTransition.Execute.Command)
		if len(fields) < 3 || fields[0] != "gentle-ai" || fields[1] != "review" {
			t.Fatalf("continuation command = %q", result.NextTransition.Execute.Command)
		}
		return fields[2:]
	}

	assertReentry("START-published continuation", continuationArgs(started))
	assertReentry("exact-lineage STATUS", []string{
		"status", "--contract", ReviewIntegrationContractV2, "--next-transition", "--lineage", started.LineageID,
	})

	replayed := runNegotiatedReviewStartExcludingUntracked(t, repo, started.LineageID)
	if replayed.Action != "replayed" || replayed.LineageID != started.LineageID {
		t.Fatalf("second START = %s/%s", replayed.Action, replayed.LineageID)
	}
	assertReentry("replayed START continuation", continuationArgs(replayed))
}
