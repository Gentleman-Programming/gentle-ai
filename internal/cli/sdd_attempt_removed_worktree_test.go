package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// #2661 at the CLI boundary: with the bound worktree gone, acquire's block and
// the passed settle's refusal name the interrupted settle, which succeeds.
func TestRunSDDAttemptRemovedWorktreeNamesAndAdmitsInterruptedSettle(t *testing.T) {
	repo := initReviewCLIRepo(t)
	linked := filepath.Join(t.TempDir(), "linked")
	runReviewCLIGit(t, repo, "worktree", "add", "-q", "--detach", linked)
	const change = "removed-worktree"
	acquired, _ := runCompactSDDAttempt(t, compactAcquireArgs(linked, change, "linked-acquire", 2))
	if err := os.RemoveAll(linked); err != nil || acquired.State != "proceed" {
		t.Fatalf("linked acquire = %#v, remove err = %v", acquired, err)
	}
	runReviewCLIGit(t, repo, "worktree", "prune")
	blocked, _ := runCompactSDDAttempt(t, compactAcquireArgs(repo, change, "main-acquire", 2))
	refused, _ := runCompactSDDAttempt(t, compactSettleArgs(repo, change, acquired.Token, "main-settle-passed", "passed"))
	for _, tt := range []struct {
		label  string
		result compactAttemptOutput
		reason string
	}{{"acquire", blocked, "active_attempt"}, {"passed settle", refused, "worktree_mismatch"}} {
		if tt.result.State != "blocked" || tt.result.Reason != tt.reason {
			t.Fatalf("%s after removal = %#v, want blocked(%s)", tt.label, tt.result, tt.reason)
		}
		assertExitNames(t, tt.result.Exit, "no longer exists", "--outcome interrupted", "`gentle-ai sdd-attempt settle --cwd")
	}
	settled, _ := runCompactSDDAttempt(t, compactSettleArgsWithEvidence(repo, change, acquired.Token, "main-settle-interrupted", "interrupted", ""))
	status := runSDDAttemptStatus(t, []string{"status", "--cwd", repo, "--change", change})
	if settled.State != "proceed" || status.ActiveAttempt != nil {
		t.Fatalf("interrupted settle after removal = %#v, active = %#v; want proceed and no active attempt", settled, status.ActiveAttempt)
	}
}
