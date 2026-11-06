package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

// TestNextTransitionDerivedRangeAdvancesAcknowledgedBoundary reproduces the
// #4815 expected-behavior half tracked in #5638: once a committed candidate is
// approved and acknowledged, the reviewed boundary advances to that commit, so
// a NON-passive follow-up is offered as the delta since the acknowledged
// boundary -- not as the whole range from the original branch point, which
// re-reviews every byte the prior receipt already covered.
func TestNextTransitionDerivedRangeAdvancesAcknowledgedBoundary(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	runReviewCLIGit(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	runReviewCLIGit(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	originalBase := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "refs/remotes/origin/main"))

	// First work unit: a documentation candidate, zero-lens, closed approved
	// and acknowledged through the standard negotiated flow.
	writeZeroLensDocumentationCandidate(t, repo)
	runReviewCLIGit(t, repo, "add", "docs/ordinary-guide.md")
	runReviewCLIGit(t, repo, "commit", "-qm", "documentation candidate")
	offered := derivedRangeTerminalStatus(t, repo)
	if offered.NextTransition == nil || offered.NextTransition.Execute == nil {
		t.Fatalf("no committed-range START for the first work unit: %#v", offered.NextTransition)
	}
	if baseRef := startTransitionArgumentValue(t, offered, "base-ref"); baseRef != originalBase {
		t.Fatalf("first work unit base-ref = %q, want the branch point %q", baseRef, originalBase)
	}
	args := []string{"start"}
	for _, argument := range offered.NextTransition.Execute.Arguments {
		args = append(args, argument.Token)
	}
	var output bytes.Buffer
	if err := RunReview(args, &output); err != nil {
		t.Fatalf("execute offered START: %v\n%s", err, output.String())
	}
	pending := derivedRangeTerminalStatus(t, repo)
	if pending.NextTransition == nil || pending.NextTransition.ReasonCode != "approved_acknowledgement_required" {
		t.Fatalf("first work unit did not reach pending acknowledgement: %#v", pending.NextTransition)
	}
	args = []string{"acknowledge-approved"}
	for _, argument := range pending.NextTransition.Execute.Arguments {
		args = append(args, argument.Token)
	}
	output.Reset()
	if err := RunReview(args, &output); err != nil {
		t.Fatal(err)
	}
	consumed, err := reviewtransaction.CompactTargetConsumed(t.Context(), repo, offered.TargetIdentity)
	if err != nil || !consumed {
		t.Fatalf("first work unit consumption = %v, %v", consumed, err)
	}
	boundary := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))

	// Second work unit: executable code after the acknowledged boundary. The
	// reviewed boundary advances to the acknowledged commit, so the derived
	// candidate is the delta since that boundary -- its --base-ref names the
	// boundary commit, never the original branch point.
	writeReviewStartCandidate(t, repo, "cmd/app/main.go", "package main\n\nfunc main() {}\n", 0o644)
	runReviewCLIGit(t, repo, "add", "cmd/app/main.go")
	runReviewCLIGit(t, repo, "commit", "-qm", "code follow-up")
	changed := derivedRangeTerminalStatus(t, repo)
	if changed.NextTransition == nil || changed.NextTransition.Execute == nil || changed.NextTransition.Execute.Operation != "review.start" {
		t.Fatalf("non-passive follow-up after the boundary not offered: %#v", changed.NextTransition)
	}
	if baseRef := startTransitionArgumentValue(t, changed, "base-ref"); baseRef != boundary {
		t.Fatalf("follow-up base-ref = %q, want the acknowledged boundary commit %q; re-reviewing the acknowledged range is the #4815 boundary drift (issue #5638)", baseRef, boundary)
	}
}

// TestNextTransitionAcknowledgedBoundaryReachesHead pins the #4628 half: on a
// branch whose every commit since the acknowledged boundary is empty, the
// reviewed boundary reaches HEAD, so the stale accumulated-span offer is
// replaced by the truthful authority-free stop and the Stop hook stays silent.
func TestNextTransitionAcknowledgedBoundaryReachesHead(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	runReviewCLIGit(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	runReviewCLIGit(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")

	writeZeroLensDocumentationCandidate(t, repo)
	runReviewCLIGit(t, repo, "add", "docs/ordinary-guide.md")
	runReviewCLIGit(t, repo, "commit", "-qm", "documentation candidate")
	offered := derivedRangeTerminalStatus(t, repo)
	if offered.NextTransition == nil || offered.NextTransition.Execute == nil {
		t.Fatalf("no committed-range START: %#v", offered.NextTransition)
	}
	args := []string{"start"}
	for _, argument := range offered.NextTransition.Execute.Arguments {
		args = append(args, argument.Token)
	}
	var output bytes.Buffer
	if err := RunReview(args, &output); err != nil {
		t.Fatalf("execute offered START: %v\n%s", err, output.String())
	}
	pending := derivedRangeTerminalStatus(t, repo)
	args = []string{"acknowledge-approved"}
	for _, argument := range pending.NextTransition.Execute.Arguments {
		args = append(args, argument.Token)
	}
	output.Reset()
	if err := RunReview(args, &output); err != nil {
		t.Fatal(err)
	}

	// An empty commit after the acknowledgement: the boundary reaches HEAD.
	runReviewCLIGit(t, repo, "commit", "--allow-empty", "-qm", "empty follow-up")
	reaches := derivedRangeTerminalStatus(t, repo)
	if reaches.NextTransition == nil || reaches.NextTransition.Kind != reviewNextTransitionStop ||
		reaches.NextTransition.ReasonCode != "acknowledged_boundary_reaches_head" {
		t.Fatalf("empty commits after the acknowledgement did not stop at the boundary: %#v", reaches.NextTransition)
	}
	if reaches.NextTransition.Continuation != nil || reaches.Authority != nil {
		t.Fatalf("boundary stop carries authority or continuation: %#v", reaches.NextTransition)
	}
	payload := reviewStopHookTestPayload(t, "reaches-head-session", repo, false, nil)
	var hookOut, hookErr bytes.Buffer
	if err := runReviewStopHook([]string{"--agent", "claude-code"}, strings.NewReader(payload), &hookOut, &hookErr); err != nil {
		t.Fatalf("stop hook: %v\n%s%s", err, hookOut.String(), hookErr.String())
	}
	if strings.TrimSpace(hookOut.String()) != "" {
		t.Fatalf("stop hook reminded for a boundary that reaches HEAD: %s", hookOut.String())
	}
}
