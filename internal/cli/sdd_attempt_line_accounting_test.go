package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// baseAdvanceRepo is the #2536 shape: main holds a.txt, feature branches off
// it, and the attempt on feature will see main advance by a 500-line file and
// a rewrite of a.txt line one while it runs.
func baseAdvanceRepo(t *testing.T) (repo string, write func(name, content string)) {
	t.Helper()
	repo = initReviewCLIRepo(t)
	write = func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.txt", "one\ntwo\nthree\n")
	runReviewCLIGit(t, repo, "add", "a.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "a")
	runReviewCLIGit(t, repo, "branch", "-M", "main")
	runReviewCLIGit(t, repo, "checkout", "-qb", "feature")
	return repo, write
}

func advanceMain(t *testing.T, repo string, write func(name, content string)) {
	t.Helper()
	runReviewCLIGit(t, repo, "checkout", "-q", "main")
	write("big.txt", strings.Repeat("base line\n", 500))
	write("a.txt", "ONE\ntwo\nthree\n")
	runReviewCLIGit(t, repo, "add", "big.txt", "a.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "base advance")
	runReviewCLIGit(t, repo, "checkout", "-q", "feature")
}

func beginThenFinishPassed(t *testing.T, repo, change string, between func()) int {
	t.Helper()
	started := runSDDAttemptStatus(t, []string{
		"begin", "--cwd", repo, "--change", change, "--expected-revision=", "--request-id", change + "-begin",
		"--work-unit", "authored-only", "--evidence-goal", "prove the attempt charge excludes a base advance",
		"--max-attempts", "1", "--max-changed-lines", "200",
	})
	between()
	finished := runSDDAttemptStatus(t, []string{
		"finish", "--cwd", repo, "--change", change, "--expected-revision", started.Revision, "--request-id", change + "-finish",
		"--outcome", "passed", "--evidence-revision", cliAttemptHash('a'), "--harness-disposition", "reused",
		"--diagnosis", "focused checks passed", "--cleanup-evidence", "cleanup completed", "--process-evidence", "no descendants",
	})
	if len(finished.Attempts) != 1 || finished.DecisionRequired {
		t.Fatalf("finish CLI status = %#v", finished)
	}
	return finished.Attempts[0].ChangedLines
}

// TestRunSDDAttemptFinishChargesOnlyAuthoredLinesAfterBaseAdvance pins #2536
// through the real CLI route: a base advance that fast-forwards the worktree
// during the attempt is candidate identity, not authored work. The charge is
// the attempt's own edit of a.txt line three (two lines), not the
// begin-tree-to-finish-tree diff that also carries the advance.
func TestRunSDDAttemptFinishChargesOnlyAuthoredLinesAfterBaseAdvance(t *testing.T) {
	repo, write := baseAdvanceRepo(t)
	changed := beginThenFinishPassed(t, repo, "base-advance", func() {
		advanceMain(t, repo, write)
		runReviewCLIGit(t, repo, "merge", "-q", "--no-edit", "main")
		write("a.txt", "ONE\ntwo\nTHREE\n")
	})
	if changed != 2 {
		t.Fatalf("merged base advance was charged to the attempt: changed_lines=%d", changed)
	}
}

// TestRunSDDAttemptFinishKeepsUnresolvedAdvancePathsChargedFromBeginTree
// covers the merge-commit shape with a conflicting path: feature rewrote
// a.txt line one before the attempt began, so re-applying main's advance onto
// the begin tree cannot resolve a.txt. big.txt re-applies cleanly and is not
// charged; a.txt keeps its begin-tree charge (the resolved line one plus the
// authored line three, four lines) instead of the five-hundred-line advance.
func TestRunSDDAttemptFinishKeepsUnresolvedAdvancePathsChargedFromBeginTree(t *testing.T) {
	repo, write := baseAdvanceRepo(t)
	write("a.txt", "uno\ntwo\nthree\n")
	runReviewCLIGit(t, repo, "commit", "-qam", "feature line one")
	changed := beginThenFinishPassed(t, repo, "conflicting-advance", func() {
		advanceMain(t, repo, write)
		if err := exec.Command("git", "-C", repo, "merge", "-q", "--no-edit", "main").Run(); err == nil {
			t.Fatal("expected the merge to conflict on a.txt")
		}
		write("a.txt", "ONE\ntwo\nthree\n")
		runReviewCLIGit(t, repo, "add", "a.txt")
		runReviewCLIGit(t, repo, "-c", "core.editor=true", "commit", "-q", "--no-edit")
		write("a.txt", "ONE\ntwo\nTHREE\n")
	})
	if changed != 4 {
		t.Fatalf("conflicting base advance was charged wrongly: changed_lines=%d", changed)
	}
}

// TestRunSDDAttemptBeginRefusesZeroBudgets pins the #1947 residual: an explicit
// zero budget refuses exactly like a negative one instead of being silently
// replaced by the default, while an absent flag still receives the default.
func TestRunSDDAttemptBeginRefusesZeroBudgets(t *testing.T) {
	repo := initReviewCLIRepo(t)
	args := []string{
		"begin", "--cwd", repo, "--change", "zero-budget", "--expected-revision=", "--request-id", "zero-begin",
		"--work-unit", "zero", "--evidence-goal", "prove zero budgets refuse",
	}
	for _, tc := range []struct{ flag, want string }{
		{"--max-changed-lines", "max_changed_lines must be within 1..1000000"},
		{"--max-attempts", "max_attempts must be within 1..100"},
	} {
		err := RunSDDAttempt(append(append([]string{}, args...), tc.flag, "0"), &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s 0 was not refused with %q: %v", tc.flag, tc.want, err)
		}
	}
	status := runSDDAttemptStatus(t, args)
	if status.Objective == nil || status.Objective.MaxChangedLines != 200 || status.Objective.MaxAttempts != 2 {
		t.Fatalf("absent budgets did not default: %#v", status.Objective)
	}
}
