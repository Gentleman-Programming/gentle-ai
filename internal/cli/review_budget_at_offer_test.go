package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeBigCommittedWorkUnit commits one work unit whose frozen diff is large
// but individually reviewable: about 150 KB of generated Go, safely under the
// 200 KiB per-runtime context budget, so the over-budget condition in these
// tests comes from the accumulated range, not from any single commit.
func writeBigCommittedWorkUnit(t *testing.T, repo, path, label string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(repo, filepath.Dir(path)), 0o755); err != nil {
		t.Fatal(err)
	}
	var body strings.Builder
	body.WriteString("package big\n\n")
	for i := 0; i < 1500; i++ {
		body.WriteString("// " + label + " generated commentary line that inflates the frozen diff evidence toward the runtime budget\n")
	}
	if err := os.WriteFile(filepath.Join(repo, path), []byte(body.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", path)
	runReviewCLIGit(t, repo, "commit", "-qm", label+" work unit")
	return strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
}

// buildTwoUnitOverBudgetRepo builds the #4790 shape: two committed work units
// whose accumulated base..HEAD frozen diff exceeds the per-runtime context
// budget while each unit alone fits, on a repository whose remote default
// branch names the committed range unambiguously. Returns the parent of HEAD
// (the first work unit's commit, HEAD's direct parent), the slice base the
// typed stop must name.
func buildTwoUnitOverBudgetRepo(t *testing.T) (repo, sliceBase string) {
	t.Helper()
	repo = initReviewCLIRepo(t)
	runReviewCLIGit(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	runReviewCLIGit(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	first := writeBigCommittedWorkUnit(t, repo, "internal/big/first.go", "first")
	writeBigCommittedWorkUnit(t, repo, "internal/big/second.go", "second")
	return repo, first
}

// TestNegotiatedStatusRefusesOverBudgetCommittedRangeStart pins the #4790
// contract: the selectorless STATUS offer site validates the frozen-evidence
// budget BEFORE the START is offered, so an over-budget candidate never
// reaches a consent question or a reviewer slot. The typed stop names the
// executable commit-range slice START (one work unit: HEAD's direct parent),
// and that printed continuation must actually run -- it reaches the consent
// question because the sliced candidate fits the budget.
func TestNegotiatedStatusRefusesOverBudgetCommittedRangeStart(t *testing.T) {
	reviewEnabledHome(t)
	repo, sliceBase := buildTwoUnitOverBudgetRepo(t)

	var output bytes.Buffer
	if err := RunReview([]string{"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2, "--agent", "claude-code", "--next-transition"}, &output); err != nil {
		t.Fatalf("status: %v\n%s", err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	validatePublishedReviewSchema(t, compileWholeNativeStatusSchema(t, "status-v9.schema.json"), output.Bytes())

	if status.NextTransition == nil {
		t.Fatal("STATUS returned no next_transition")
	}
	if status.NextTransition.Kind != reviewNextTransitionStop || status.NextTransition.ReasonCode != "fresh_candidate_runtime_budget_exceeded" {
		t.Fatalf("over-budget candidate offered %q/%q instead of the fresh_candidate_runtime_budget_exceeded stop; the offer site must not route an uncollectable candidate into consent", status.NextTransition.Kind, status.NextTransition.ReasonCode)
	}
	if status.NextTransition.Continuation == nil {
		t.Fatal("over-budget stop carries no slice continuation")
	}
	continuation := status.NextTransition.Continuation.Command
	if !strings.Contains(continuation, "--base-ref="+sliceBase) || !strings.Contains(continuation, "--committed-only=true") {
		t.Fatalf("slice continuation does not name the one-work-unit range (base-ref %s): %s", sliceBase, continuation)
	}

	// Runnability: the named exit must execute exactly as printed. Running it
	// reaches the consent question because the sliced candidate fits.
	invocation := strings.TrimPrefix(continuation, "gentle-ai review ")
	var runOut bytes.Buffer
	if err := RunReview(strings.Fields(invocation), &runOut); err != nil {
		t.Fatalf("slice continuation refused: %v\n%s", err, runOut.String())
	}
	question := decodeConsentQuestion(t, runOut.Bytes())
	if question.Action != "consent_required" {
		t.Fatalf("slice continuation did not reach consent: %#v", question)
	}
}

// TestNegotiatedStatusStillOffersFittingCommittedRange pins the probe's
// fail-open side: a candidate whose frozen evidence fits the runtime budget
// keeps today's offer exactly as before, so the offer-site bound can never
// false-positive a reviewable candidate.
func TestNegotiatedStatusStillOffersFittingCommittedRange(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	runReviewCLIGit(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	runReviewCLIGit(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	if err := os.MkdirAll(filepath.Join(repo, "internal/big"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "internal/big/small.go"), []byte("package big\n\n// one small change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "internal/big/small.go")
	runReviewCLIGit(t, repo, "commit", "-qm", "small work unit")

	status := derivedRangeTerminalStatus(t, repo)
	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionExecute ||
		status.NextTransition.Execute == nil || status.NextTransition.Execute.Operation != "review.start" {
		t.Fatalf("fitting candidate lost the derived committed-range START: %#v", status.NextTransition)
	}
}

// TestStopHookRoutesOverBudgetCandidateToSlice pins the hook half: the Stop
// hook must not go silent for an over-budget pending candidate (that would
// hollow out RDD enforcement), and it must not route into the blocked
// full-range START either. The reminder names the slice continuation.
func TestStopHookRoutesOverBudgetCandidateToSlice(t *testing.T) {
	reviewEnabledHome(t)
	repo, _ := buildTwoUnitOverBudgetRepo(t)

	runReviewStopHookSessionStartTest(t, "over-budget-session", repo)

	payload := reviewStopHookTestPayload(t, "over-budget-stop", repo, false, nil)
	var hookOut, hookErr bytes.Buffer
	if err := runReviewStopHook([]string{"--agent", "claude-code"}, strings.NewReader(payload), &hookOut, &hookErr); err != nil {
		t.Fatalf("stop hook: %v\n%s%s", err, hookOut.String(), hookErr.String())
	}
	if strings.TrimSpace(hookOut.String()) == "" {
		t.Fatal("stop hook stayed silent for a pending over-budget candidate")
	}
	if !strings.Contains(hookOut.String(), "fresh_candidate_runtime_budget_exceeded") ||
		!strings.Contains(hookOut.String(), "--committed-only=true") {
		t.Fatalf("stop hook reminder does not route to the slice continuation: %s", hookOut.String())
	}
}
