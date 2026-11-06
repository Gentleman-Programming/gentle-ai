package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildTrunkIntegrationSplitRepo reproduces the repository shape both #4639
// occurrences describe: refs/remotes/origin/HEAD names the remote default
// branch, which only advances when a release lands, while work is cut from and
// merged into a separate integration branch that the default branch lags
// behind. The returned develop SHA is the integration tip; the default branch
// never leaves the initial base commit.
func buildTrunkIntegrationSplitRepo(t *testing.T) (repo, develop string) {
	t.Helper()
	repo = initReviewCLIRepo(t)
	// Capture the default branch BEFORE cutting the integration branch: the
	// remote default branch (origin/HEAD) lags the integration branch in the
	// #4639 shape, and deriving it late would silently aim origin/HEAD at the
	// integration branch and void the reproduction.
	defaultBranch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
	runReviewCLIGit(t, repo, "checkout", "-qb", "develop")
	if err := os.WriteFile(filepath.Join(repo, "integration.md"), []byte("integration work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "integration.md")
	runReviewCLIGit(t, repo, "commit", "-qm", "integration work one")
	if err := os.WriteFile(filepath.Join(repo, "integration-two.md"), []byte("integration work two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "integration-two.md")
	runReviewCLIGit(t, repo, "commit", "-qm", "integration work two")
	main := defaultBranch
	develop = strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "refs/heads/develop"))
	runReviewCLIGit(t, repo, "update-ref", "refs/remotes/origin/"+defaultBranch, main)
	runReviewCLIGit(t, repo, "update-ref", "refs/remotes/origin/develop", develop)
	runReviewCLIGit(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/"+defaultBranch)
	return repo, develop
}

// TestNextTransitionCommittedRangeBasePrefersBranchUpstream reproduces the
// #4639 defect: a short-lived feature branch cut from the integration branch,
// tracking that integration branch, holds one small commit while the remote
// default branch lags whole releases behind. The derived committed-range START
// must name the merge-base with the branch's upstream, so the candidate is the
// change under review; deriving from the remote default branch instead spans
// every integration-branch commit since the last release and dies in
// preflight with lens_context_budget_exceeded before any review can start.
func TestNextTransitionCommittedRangeBasePrefersBranchUpstream(t *testing.T) {
	reviewEnabledHome(t)
	repo, develop := buildTrunkIntegrationSplitRepo(t)
	runReviewCLIGit(t, repo, "checkout", "-qb", "feature")
	if err := os.WriteFile(filepath.Join(repo, "feature.txt"), []byte("the change under review\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "feature.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "feature work")
	runReviewCLIGit(t, repo, "config", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*")
	runReviewCLIGit(t, repo, "config", "branch.feature.remote", "origin")
	runReviewCLIGit(t, repo, "config", "branch.feature.merge", "refs/heads/develop")

	status := derivedRangeTerminalStatus(t, repo)
	if status.NextTransition == nil || status.NextTransition.Execute == nil || status.NextTransition.Execute.Operation != "review.start" {
		t.Fatalf("no derived committed-range START for the tracked feature branch: %#v", status.NextTransition)
	}
	arguments, err := reviewTransitionArgumentMap(status.NextTransition.Execute.Arguments, status.NextTransition.Execute.Operation)
	if err != nil {
		t.Fatal(err)
	}
	if arguments["base-ref"] != develop {
		t.Fatalf("derived START base-ref = %q, want the merge-base with the branch's upstream (integration tip %q); a base on the stale default branch spans unrelated merged work (issue #4639)", arguments["base-ref"], develop)
	}
}

// TestNextTransitionCommittedRangeBaseRetainsDefaultBranchForLevelUpstream pins
// the retained #4412 behavior the upstream preference must not regress: when
// the branch is level with its own upstream (a fully pushed feature branch),
// the upstream range is empty, so derivation keeps the remote default branch
// and still offers the executable committed-range START instead of the
// unroutable empty_candidate_base_ref_required collect. The known limitation
// for a default branch that is not the integration branch is tracked by
// #4628 and addressed by making over-budget candidates actionable, not by
// dropping the derivation here.
func TestNextTransitionCommittedRangeBaseRetainsDefaultBranchForLevelUpstream(t *testing.T) {
	reviewEnabledHome(t)
	repo, _ := buildTrunkIntegrationSplitRepo(t)
	runReviewCLIGit(t, repo, "checkout", "-qb", "feature")
	if err := os.WriteFile(filepath.Join(repo, "feature.txt"), []byte("the change under review\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "feature.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "feature work")
	feature := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "refs/heads/feature"))
	runReviewCLIGit(t, repo, "update-ref", "refs/remotes/origin/feature", feature)
	runReviewCLIGit(t, repo, "config", "branch.feature.remote", "origin")
	runReviewCLIGit(t, repo, "config", "branch.feature.merge", "refs/heads/feature")

	status := derivedRangeTerminalStatus(t, repo)
	if status.NextTransition == nil || status.NextTransition.Execute == nil || status.NextTransition.Execute.Operation != "review.start" {
		t.Fatalf("level-upstream branch lost the derived committed-range START (must not regress #4412): %#v", status.NextTransition)
	}
	if arguments, err := reviewTransitionArgumentMap(status.NextTransition.Execute.Arguments, status.NextTransition.Execute.Operation); err != nil {
		t.Fatal(err)
	} else if arguments["base-ref"] == "" {
		t.Fatalf("derived START carries no base-ref: %#v", status.NextTransition.Execute.Arguments)
	}
}

// TestNextTransitionCommittedRangeBaseCrissCrossUpstreamKeepsDefaultBranch pins
// the upstream preference's ambiguity handling: when the upstream history has
// multiple merge-bases with HEAD, the upstream refinement declines and the
// derivation falls back to the remote default branch exactly as before, so an
// ambiguous upstream can never pick a scope the caller never named.
func TestNextTransitionCommittedRangeBaseCrissCrossUpstreamKeepsDefaultBranch(t *testing.T) {
	reviewEnabledHome(t)
	repo, _ := buildTrunkIntegrationSplitRepo(t)
	// Build a criss-cross between feature and its upstream develop: two
	// symmetric merges of one branch into the other.
	runReviewCLIGit(t, repo, "checkout", "-qb", "side")
	if err := os.WriteFile(filepath.Join(repo, "side.txt"), []byte("side\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "side.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "side work")
	runReviewCLIGit(t, repo, "checkout", "develop")
	if err := os.WriteFile(filepath.Join(repo, "develop-side.txt"), []byte("develop side\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "develop-side.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "develop side work")
	preMergeDevelop := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "develop"))
	runReviewCLIGit(t, repo, "merge", "--no-ff", "-qm", "merge side into develop", "side")
	runReviewCLIGit(t, repo, "checkout", "side")
	runReviewCLIGit(t, repo, "merge", "--no-ff", "-qm", "merge pre-merge develop into side", preMergeDevelop)
	runReviewCLIGit(t, repo, "checkout", "-B", "feature", "side")
	if err := os.WriteFile(filepath.Join(repo, "feature.txt"), []byte("the change under review\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "feature.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "feature work")
	merges := strings.TrimSpace(runReviewCLIGit(t, repo, "merge-base", "--all", "HEAD", "develop"))
	if strings.Count(strings.TrimSpace(merges), "\n") != 1 {
		t.Fatalf("fixture is not criss-cross: merge-base --all returned %q", merges)
	}
	runReviewCLIGit(t, repo, "config", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*")
	runReviewCLIGit(t, repo, "update-ref", "refs/remotes/origin/develop", "develop")
	runReviewCLIGit(t, repo, "config", "branch.feature.remote", "origin")
	runReviewCLIGit(t, repo, "config", "branch.feature.merge", "refs/heads/develop")

	status := derivedRangeTerminalStatus(t, repo)
	if status.NextTransition == nil || status.NextTransition.Execute == nil || status.NextTransition.Execute.Operation != "review.start" {
		t.Fatalf("criss-cross upstream lost the derived committed-range START (must keep the default-branch derivation): %#v", status.NextTransition)
	}
}
