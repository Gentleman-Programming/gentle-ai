package cli

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v5/internal/reviewtransaction"
)

// TestReviewCommittedRangeBoundaryMatrix distinguishes an explicit range from
// the provider's automatic range across successive real acknowledgements.
func TestReviewCommittedRangeBoundaryMatrix(t *testing.T) {
	for _, tc := range []struct {
		name     string
		escalate bool
	}{
		{name: "successive acknowledgements"},
		{name: "unresolved escalation", escalate: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reviewEnabledHome(t)
			repo := initReviewCLIRepo(t)
			runReviewCLIGit(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
			runReviewCLIGit(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
			commit := func(path, text string) string {
				writeReviewStartCandidate(t, repo, path, text, 0o644)
				runReviewCLIGit(t, repo, "add", "--", path)
				runReviewCLIGit(t, repo, "commit", "-qm", "update "+path)
				return strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
			}
			status := func(selectors ...string) ReviewTargetStatusResult {
				var output bytes.Buffer
				if err := RunReview(append([]string{"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2, "--next-transition"}, selectors...), &output); err != nil {
					t.Fatalf("STATUS %v: %v\n%s", selectors, err, output.String())
				}
				var result ReviewTargetStatusResult
				decodeStrictReviewJSON(t, output.Bytes(), &result)
				return result
			}
			assertStart := func(t *testing.T, result ReviewTargetStatusResult, base string, paths ...string) {
				t.Helper()
				if result.Authority != nil || result.NextTransition == nil || result.NextTransition.ReasonCode != "fresh_target_ready" ||
					result.NextTransition.Execute == nil || result.NextTransition.Execute.Operation != "review.start" {
					t.Fatalf("expected a fresh START without authority for %v; got action %s, transition %+v, authority %+v", paths, result.Action, result.NextTransition, result.Authority)
				}
				if actual := startTransitionArgumentValue(t, result, "base-ref"); actual != base || !slices.Equal(result.Projection.Paths, paths) {
					t.Errorf("START base-ref=%s paths=%v; want base-ref=%s paths=%v", actual, result.Projection.Paths, base, paths)
				}
				if actual := startTransitionArgumentValue(t, result, "committed-only"); actual != "true" {
					t.Errorf("START committed-only=%s; want true", actual)
				}
			}
			acknowledge := func(lineage, target string) {
				t.Helper()
				pending := status("--lineage", lineage)
				if pending.TargetIdentity != target || pending.NextTransition == nil || pending.NextTransition.ReasonCode != "approved_acknowledgement_required" || pending.NextTransition.Execute == nil {
					t.Fatalf("no acknowledgement for %s: %+v", target, pending.NextTransition)
				}
				args := []string{"acknowledge-approved"}
				for _, row := range pending.NextTransition.Execute.Arguments {
					args = append(args, row.Token)
				}
				var output bytes.Buffer
				if err := RunReview(args, &output); err != nil {
					t.Fatalf("acknowledge approved: %v\n%s", err, output.String())
				}
				after := status()
				if after.TargetIdentity != target || after.Authority != nil || after.NextTransition == nil || after.NextTransition.ReasonCode != "target_already_acknowledged" {
					t.Fatalf("acknowledged target %s not terminal: %+v", target, after.NextTransition)
				}
			}

			origin := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
			a := commit("README.md", "# Fixture\n\nA sample project.\n")
			first := status()
			assertStart(t, first, origin, "README.md")
			started := grantedDerivedRangeStart(t, repo, first)
			acknowledge(started.LineageID, first.TargetIdentity)

			// Operational README paths are non-passive even though their contents
			// are just text, so each slice can produce an observable START.
			b := commit("runtime/README.md", "# Example runtime\n\nThe sample uses mode A.\n")
			second := status()
			assertStart(t, second, a, "runtime/README.md")
			started = grantedDerivedRangeStart(t, repo, second)
			started.TargetIdentity = second.TargetIdentity
			var closureOutput bytes.Buffer
			if tc.escalate {
				captureCLIReviewerResultWithFindings(t, repo, started, 0, []facadeFinding{{
					Location: "runtime/README.md:3", Severity: "CRITICAL", Claim: "Sample mode could be documented incorrectly; attribution is unresolved",
					ProofRefs:     []string{"runtime/README.md:3 does not establish which revision changed the mode"},
					EvidenceClass: reviewtransaction.EvidenceInsufficient, CausalDisposition: reviewtransaction.CausalUnknown,
				}}, &closureOutput)
				var closure reviewLastEventClosureResult
				decodeStrictReviewJSON(t, closureOutput.Bytes(), &closure)
				if closure.State != reviewtransaction.StateEscalated || closure.Acknowledgement != nil {
					t.Fatalf("unresolved reviewer verdict = %s, want escalated without acknowledgement: %s", closure.State, closureOutput.String())
				}
			} else {
				captureCleanCLIReviewerResult(t, repo, started, 0, &closureOutput)
				acknowledge(started.LineageID, second.TargetIdentity)
			}

			c := commit("prompts/README.md", "# Example prompts\n\nA greeting example.\n")
			t.Run("explicit branch point", func(t *testing.T) {
				assertStart(t, status("--base-ref", origin, "--committed-only"), origin, "README.md", "prompts/README.md", "runtime/README.md")
				assessed := derivedRangeAssess(t, repo, origin)
				if assessed.Candidate.BaseRef != origin || assessed.ChangedPaths != 3 || assessed.Candidate.Consumed ||
					assessed.ReviewDue || assessed.ReviewDueReason != "under_budget" || assessed.NextTransition != nil {
					t.Fatalf("explicit ASSESS did not classify the full unconsumed range: %+v", assessed)
				}
			})
			t.Run("selectorless", func(t *testing.T) {
				if tc.escalate {
					assertStart(t, status(), a, "prompts/README.md", "runtime/README.md")
				} else {
					assertStart(t, status(), b, "prompts/README.md")
				}
			})
			if !tc.escalate {
				// Older selectorless offers could also have consumed A..C.
				// Freeze that exact historical scope through explicit STATUS.
				overlap := status("--base-ref", a, "--committed-only")
				assertStart(t, overlap, a, "prompts/README.md", "runtime/README.md")
				started = grantedDerivedRangeStart(t, repo, overlap)
				started.TargetIdentity = overlap.TargetIdentity
				captureCleanCLIReviewerResult(t, repo, started, 0, &bytes.Buffer{})
				acknowledge(started.LineageID, overlap.TargetIdentity)

				// The acknowledged A..C covers C; the next offer is only C..D,
				// never B..D, which would send C's reviewed change back.
				d := commit("runtime/example/README.md", "# Another sample\n")
				assertStart(t, status(), c, "runtime/example/README.md")

				// An explicit B..D acknowledgement beside A..C also covers D:
				// the next offer is D..E, not C..E.
				bd := status("--base-ref", b, "--committed-only")
				assertStart(t, bd, b, "prompts/README.md", "runtime/example/README.md")
				started = grantedDerivedRangeStart(t, repo, bd)
				started.TargetIdentity = bd.TargetIdentity
				captureCleanCLIReviewerResult(t, repo, started, 0, &bytes.Buffer{})
				acknowledge(started.LineageID, bd.TargetIdentity)
				e := commit("runtime/follow-up.md", "# Follow-up\n")
				assertStart(t, status(), d, "runtime/follow-up.md")

				// Any reached boundary may start the next acknowledgement, not
				// only the latest few: A..E also covers E, so F is offered alone.
				ae := status("--base-ref", a, "--committed-only")
				assertStart(t, ae, a, "prompts/README.md", "runtime/README.md", "runtime/example/README.md", "runtime/follow-up.md")
				started = grantedDerivedRangeStart(t, repo, ae)
				started.TargetIdentity = ae.TargetIdentity
				captureCleanCLIReviewerResult(t, repo, started, 0, &bytes.Buffer{})
				acknowledge(started.LineageID, ae.TargetIdentity)
				commit("runtime/later.md", "# Later\n")
				assertStart(t, status(), e, "runtime/later.md")
			}
		})
	}
}

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
