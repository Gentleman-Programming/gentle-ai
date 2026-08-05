package reviewtransaction

import (
	"context"
	"strings"
	"testing"
)

// TestCompactReleaseScopeRecoveryParentCount proves issue-1816:
// BuildReleaseScopeSnapshot accepts an ordinary one-parent delivery commit as
// HEAD (squash or rebase merges are one-parent commits), not only a
// two-parent merge commit, while still refusing a root commit (zero
// parents) with a typed message. compactReleaseScopeRecovery
// (validateCompactRecoveryEdge's RecoveryScopeChanged/StateApproved release
// branch) remains the real authority gate and is unchanged by this relaxation.
func TestCompactReleaseScopeRecoveryParentCount(t *testing.T) {
	t.Run("one-parent delivery commit is accepted", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		writeSnapshotFile(t, repo, "tracked.txt", "delivery candidate\n")
		gitSnapshot(t, repo, "add", "--", "tracked.txt")
		gitSnapshot(t, repo, "commit", "-m", "squash-merged delivery")
		parents := strings.Fields(gitSnapshot(t, repo, "rev-list", "--parents", "-n", "1", "HEAD"))
		if len(parents) != 2 {
			t.Fatalf("fixture HEAD is not an ordinary one-parent commit: %v", parents)
		}

		snapshot, err := BuildReleaseScopeSnapshot(context.Background(), repo)
		if err != nil {
			t.Fatalf("release-scope snapshot over a one-parent delivery commit rejected: %v", err)
		}
		wantBase := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD^1^{tree}"))
		wantCandidate := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD^{tree}"))
		if snapshot.Kind != TargetBaseDiff || snapshot.BaseTree != wantBase || snapshot.CandidateTree != wantCandidate {
			t.Fatalf("one-parent release-scope snapshot = %#v, want base %s candidate %s", snapshot, wantBase, wantCandidate)
		}
	})

	t.Run("root commit is still refused", func(t *testing.T) {
		repo := initUnbornSnapshotRepo(t)
		writeSnapshotFile(t, repo, "tracked.txt", "root candidate\n")
		gitSnapshot(t, repo, "add", "--", "tracked.txt")
		gitSnapshot(t, repo, "commit", "-m", "root commit")
		parents := strings.Fields(gitSnapshot(t, repo, "rev-list", "--parents", "-n", "1", "HEAD"))
		if len(parents) != 1 {
			t.Fatalf("fixture HEAD is not a root commit: %v", parents)
		}

		if _, err := BuildReleaseScopeSnapshot(context.Background(), repo); err == nil {
			t.Fatal("release-scope snapshot over a root commit (zero parents) was admitted")
		}
	})
}
