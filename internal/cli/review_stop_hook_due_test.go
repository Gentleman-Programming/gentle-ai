package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/reviewtransaction"
)

// stopHookAssessment runs the read-only assess command the ODD contract names
// and decodes its public envelope, so a test can compare the hook's behavior
// against the very review_due decision the contract tells agents to honor.
func stopHookAssessment(t *testing.T, repo, baseRef string) ReviewAssessmentResult {
	t.Helper()
	var output bytes.Buffer
	if err := RunReview([]string{"assess", "--cwd", repo, "--base-ref", baseRef, "--committed-only", "--json"}, &output); err != nil {
		t.Fatalf("review assess: %v\n%s", err, output.String())
	}
	var result ReviewAssessmentResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode review assess envelope: %v\n%s", err, output.String())
	}
	return result
}

// TestStopHookHonorsReviewDueUnderBudget reproduces the #4815 disagreement:
// a work-unit commit below the ~400-line slice budget classifies as medium,
// review assess reports review_due=false/under_budget ("record review_due_reason
// and continue"), and yet the Stop hook demanded the full consent + review
// round for that same candidate. The hook must honor the same review_due
// decision assess publishes: an under-budget slice stays pending in the slice
// and the hook stays silent.
func TestStopHookHonorsReviewDueUnderBudget(t *testing.T) {
	home := reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	runReviewCLIGit(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	runReviewCLIGit(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "refs/remotes/origin/main"))

	runReviewStopHookSessionStartTest(t, "under-budget-session", repo)

	writeReviewStartCandidate(t, repo, "notes/scratch.txt", "one work unit\n", 0o644)
	runReviewCLIGit(t, repo, "add", "notes/scratch.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "work unit one")

	assessment := stopHookAssessment(t, repo, base)
	if assessment.ReviewDue || assessment.ReviewDueReason != reviewAssessDueReasonUnderBudget {
		t.Fatalf("assess for the committed work unit = %v/%q, want false/%q: the fixture must reproduce the under-budget slice", assessment.ReviewDue, assessment.ReviewDueReason, reviewAssessDueReasonUnderBudget)
	}
	if assessment.Risk != "medium" {
		t.Fatalf("assess risk = %q, want medium", assessment.Risk)
	}

	payload := reviewStopHookTestPayload(t, "under-budget-stop", repo, false, nil)
	var hookOut, hookErr bytes.Buffer
	if err := runReviewStopHook([]string{"--agent", "claude-code"}, strings.NewReader(payload), &hookOut, &hookErr); err != nil {
		t.Fatalf("stop hook: %v\n%s%s", err, hookOut.String(), hookErr.String())
	}
	if strings.TrimSpace(hookOut.String()) != "" {
		t.Fatalf("stop hook demanded a review round assess says is under_budget (home %s): %s", home, hookOut.String())
	}
}

// TestStopHookStillRemindsWhenReviewDue pins the retention the under-budget
// silence must not weaken: a high-risk candidate (hot-path authentication
// change) is review_due=true/high_risk, and the hook keeps routing it through
// the selectorless STATUS preflight exactly as before.
func TestStopHookStillRemindsWhenReviewDue(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	runReviewCLIGit(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	runReviewCLIGit(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "refs/remotes/origin/main"))

	runReviewStopHookSessionStartTest(t, "high-risk-session", repo)

	writeReviewStartCandidate(t, repo, "internal/auth/login.go", "package auth\n", 0o644)
	runReviewCLIGit(t, repo, "add", "internal/auth/login.go")
	runReviewCLIGit(t, repo, "commit", "-qm", "auth change")

	assessment := stopHookAssessment(t, repo, base)
	if !assessment.ReviewDue || assessment.ReviewDueReason != reviewAssessDueReasonHighRisk {
		t.Fatalf("assess for the auth work unit = %v/%q, want true/%q", assessment.ReviewDue, assessment.ReviewDueReason, reviewAssessDueReasonHighRisk)
	}

	payload := reviewStopHookTestPayload(t, "high-risk-stop", repo, false, nil)
	var hookOut, hookErr bytes.Buffer
	if err := runReviewStopHook([]string{"--agent", "claude-code"}, strings.NewReader(payload), &hookOut, &hookErr); err != nil {
		t.Fatalf("stop hook: %v\n%s%s", err, hookOut.String(), hookErr.String())
	}
	if !strings.Contains(hookOut.String(), string(reviewtransaction.TargetBaseDiff)) && !strings.Contains(hookOut.String(), "unreviewed candidate") {
		t.Fatalf("stop hook stayed silent for a review_due=true candidate: %q", hookOut.String())
	}
}

// TestStopHookSilentForFreshPassiveCandidate pins the passive half of the same
// review_due gate: a fresh, never-reviewed candidate whose every authored path
// is passive documentation classifies review_due=false/passive ("passive needs
// no review"), so the hook stays silent exactly as it does for under_budget.
// The post-acknowledgement passive delta is a separate stop shipped by the
// acknowledged-predecessor work; this pins the fresh-candidate side.
func TestStopHookSilentForFreshPassiveCandidate(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	runReviewCLIGit(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	runReviewCLIGit(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "refs/remotes/origin/main"))

	runReviewStopHookSessionStartTest(t, "passive-session", repo)

	writeZeroLensDocumentationCandidate(t, repo)
	runReviewCLIGit(t, repo, "add", "docs/ordinary-guide.md")
	runReviewCLIGit(t, repo, "commit", "-qm", "documentation only")

	assessment := stopHookAssessment(t, repo, base)
	if assessment.ReviewDue || assessment.ReviewDueReason != reviewAssessDueReasonPassive {
		t.Fatalf("assess for the docs work unit = %v/%q, want false/%q", assessment.ReviewDue, assessment.ReviewDueReason, reviewAssessDueReasonPassive)
	}

	payload := reviewStopHookTestPayload(t, "passive-stop", repo, false, nil)
	var hookOut, hookErr bytes.Buffer
	if err := runReviewStopHook([]string{"--agent", "claude-code"}, strings.NewReader(payload), &hookOut, &hookErr); err != nil {
		t.Fatalf("stop hook: %v\n%s%s", err, hookOut.String(), hookErr.String())
	}
	if strings.TrimSpace(hookOut.String()) != "" {
		t.Fatalf("stop hook demanded a review round assess says is passive: %s", hookOut.String())
	}
}
