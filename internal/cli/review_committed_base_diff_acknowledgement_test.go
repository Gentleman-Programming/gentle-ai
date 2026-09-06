package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestSelectorlessStatusResumesCommittedRangeApprovedAcknowledgement pins
// gentle-pi#569: a committed-range (base-diff, committed-only) review that
// reaches approved must still offer its exact `review.acknowledge-approved`
// continuation when STATUS is queried selector-free (no --base-ref, no
// --committed-only) — the shape the public acknowledgement operation actually
// issues before executing the returned transition. Before the fix, the
// selectorless refresh silently defaulted to a current-changes target, which
// projects an empty diff on the clean approved worktree and reports the
// approved lineage as unrelated instead of offering its acknowledgement.
func TestSelectorlessStatusResumesCommittedRangeApprovedAcknowledgement(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo := initReviewCLIRepo(t)
	const baseRef = "frozen-base"
	const lineage = "committed-range-approved-acknowledgement"
	runReviewCLIGit(t, repo, "branch", baseRef, "HEAD")
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\nfunc value() int {\n\treturn 1\n}\n", 0o644)
	runReviewCLIGit(t, repo, "add", "candidate.go")
	runReviewCLIGit(t, repo, "commit", "-qm", "candidate")

	startedBytes, err := runLegacyFacadeStartForTestBytes(t, []string{
		"--cwd", repo, "--lineage", lineage, "--base-ref", baseRef, "--committed-only",
	})
	if err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, startedBytes, &started)
	if len(started.SelectedLenses) == 0 {
		t.Fatalf("committed base-diff START selected no lenses: %#v", started)
	}

	// Drive every lens capture clean, so the lineage reaches approved and
	// mints a pending acknowledgement.
	for order := 0; order < len(started.SelectedLenses)-1; order++ {
		captureCleanCLIReviewerResult(t, repo, started, order, &bytes.Buffer{})
	}
	var closureOutput bytes.Buffer
	captureCleanCLIReviewerResult(t, repo, started, len(started.SelectedLenses)-1, &closureOutput)

	var closure reviewLastEventClosureResult
	decodeStrictReviewJSON(t, closureOutput.Bytes(), &closure)
	if closure.State != reviewtransaction.StateApproved || closure.Acknowledgement == nil {
		t.Fatalf("committed base-diff final capture closure = %#v, want approved with a pending acknowledgement", closure)
	}
	wantAcknowledgement, err := reviewTransitionArgumentMap(closure.Acknowledgement.Arguments)
	if err != nil {
		t.Fatal(err)
	}

	// The named branch is mutable and irrelevant after approval; the
	// selectorless refresh must resume the frozen tree, never the ref.
	runReviewCLIGit(t, repo, "branch", "-f", baseRef, "HEAD")

	// This is the exact selectorless shape the public acknowledge-approved
	// operation issues: no --base-ref, no --committed-only, no --workspace-overlay.
	var statusOutput bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2, "--next-transition", "--lineage", lineage,
	}, &statusOutput); err != nil {
		t.Fatalf("selectorless committed-range approved status: %v\n%s", err, statusOutput.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, statusOutput.Bytes(), &status)
	if status.Authority == nil || status.Authority.LineageID != lineage || status.Authority.State != reviewtransaction.StateApproved {
		t.Fatalf("selectorless committed-range status authority = %#v, want approved lineage %q", status.Authority, lineage)
	}
	if status.NextTransition == nil || status.NextTransition.Execute == nil ||
		status.NextTransition.Execute.Operation != "review.acknowledge-approved" {
		t.Fatalf("selectorless committed-range status next transition = %#v, want review.acknowledge-approved", status.NextTransition)
	}
	gotAcknowledgement, err := reviewTransitionArgumentMap(status.NextTransition.Execute.Arguments)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"lineage", "target", "expected-revision", "token"} {
		if gotAcknowledgement[name] != wantAcknowledgement[name] {
			t.Fatalf("selectorless status acknowledge-approved argument %q = %q, want %q (from the terminal capture closure)", name, gotAcknowledgement[name], wantAcknowledgement[name])
		}
	}

	// The acknowledgement STATUS offered must actually succeed.
	statusArgs := []string{strings.TrimPrefix(status.NextTransition.Execute.Operation, "review.")}
	for _, argument := range status.NextTransition.Execute.Arguments {
		statusArgs = append(statusArgs, argument.Token)
	}
	var ackOutput bytes.Buffer
	if err := RunReview(statusArgs, &ackOutput); err != nil {
		t.Fatalf("run status-offered acknowledge-approved: %v\n%s", err, ackOutput.String())
	}

	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	assertApprovedCompactAuthorityBurned(t, store, lineage)
}
