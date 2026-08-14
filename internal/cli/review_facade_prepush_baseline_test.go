package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestBuildCompactPrePushDiscoveryBaselineCarriesTheBaseTree is issue #3176.
//
// The #1886 fast path compares the baseline against each leaf's
// CurrentSnapshot.BaseTree, which SnapshotBuilder derives as
// `rev-parse <ref>^{tree}`. The original baseline carried the push base
// COMMIT oid instead, so the equality could never hold and the skip never
// fired — the optimization shipped permanently inert. This test derives both
// sides from a real repository, which is exactly what the original
// synthetic-string tests failed to do.
func TestBuildCompactPrePushDiscoveryBaselineCarriesTheBaseTree(t *testing.T) {
	repo := initReviewCLIRepo(t)
	ctx := context.Background()

	// A bare remote with the current branch pushed and tracking configured:
	// the push base the baseline must resolve.
	remote := filepath.Join(t.TempDir(), "origin.git")
	if _, err := runGit(ctx, repo, "init", "--bare", remote); err != nil {
		t.Fatal(err)
	}
	if _, err := runGit(ctx, repo, "remote", "add", "origin", remote); err != nil {
		t.Fatal(err)
	}
	branch, err := runGit(ctx, repo, "symbolic-ref", "--short", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runGit(ctx, repo, "push", "--set-upstream", "origin", branch); err != nil {
		t.Fatal(err)
	}

	// Unpushed work on top, so the live push candidate is non-empty.
	if err := os.WriteFile(filepath.Join(repo, "unpushed.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runGit(ctx, repo, "add", "unpushed.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := runGit(ctx, repo, "commit", "-m", "unpushed candidate"); err != nil {
		t.Fatal(err)
	}

	baseline, err := BuildCompactPrePushDiscoveryBaseline(ctx, repo)
	if err != nil {
		t.Fatalf("BuildCompactPrePushDiscoveryBaseline() error = %v", err)
	}

	// The leaf side of the comparison, built the way production leaves are:
	// a BaseDiff snapshot whose BaseTree is `rev-parse <base>^{tree}`.
	leaf, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).BuildStoredSnapshot(ctx, reviewtransaction.Target{
		Kind:              reviewtransaction.TargetBaseDiff,
		BaseRef:           "origin/" + branch,
		IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if baseline.PushBaseTree != leaf.BaseTree {
		t.Fatalf("baseline push base %q does not match a real leaf's BaseTree %q; the fast-path equality can never fire (#3176)", baseline.PushBaseTree, leaf.BaseTree)
	}
	if len(baseline.Paths) != 1 || baseline.Paths[0] != "unpushed.txt" {
		t.Fatalf("baseline live paths = %v, want [unpushed.txt]", baseline.Paths)
	}

	// End to end: a disjoint BaseDiff leaf on the same base is now provably
	// unrelated to the live push candidate and gets skipped.
	state := reviewtransaction.CompactState{
		Schema:          reviewtransaction.CompactStateSchema,
		GenesisPaths:    []string{"somewhere/else.txt"},
		CurrentSnapshot: reviewtransaction.Snapshot{Kind: reviewtransaction.TargetBaseDiff, BaseTree: leaf.BaseTree},
	}
	if !CompactLeafProvablyUnrelatedToPrePushCandidate(state, baseline.PushBaseTree, baseline.Paths) {
		t.Fatal("disjoint same-base leaf was not skipped against a real-repository baseline (#3176)")
	}
}
