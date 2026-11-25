package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
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

func TestNextTransitionCommittedRangeBaseStaleFork(t *testing.T) {
	for _, name := range []string{"fresh_upstream", "shorter_incomparable_range", "upstream_not_in_head", "reverted_upstream_format", "chained_tips", "incomparable_tips"} {
		t.Run(name, func(t *testing.T) {
			reviewEnabledHome(t)
			repo := initReviewCLIRepo(t)
			runReviewCLIGit(t, repo, "branch", "-M", "main")
			const originRef = "refs/remotes/origin/main"
			const upstreamRef = "refs/remotes/upstream/main"
			commitFile := func(name, content string) {
				writeReviewStartCandidate(t, repo, name, content, 0o644)
				runReviewCLIGit(t, repo, "add", name)
				runReviewCLIGit(t, repo, "commit", "-qm", name)
			}
			rev := func(ref string) string {
				return strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", ref))
			}
			if name == "reverted_upstream_format" {
				commitFile("format.txt", "base\n")
			}
			root := rev("HEAD")
			if name == "shorter_incomparable_range" {
				commitFile("fork-default.txt", "fork-only default work\n")
			}
			origin := rev("HEAD")
			runReviewCLIGit(t, repo, "remote", "add", "origin", "origin.git")
			runReviewCLIGit(t, repo, "remote", "add", "upstream", "upstream.git")
			runReviewCLIGit(t, repo, "update-ref", originRef, origin)
			runReviewCLIGit(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", originRef)
			runReviewCLIGit(t, repo, "switch", "-qc", "upstream-default", root)
			for i := 1; i <= 12; i++ {
				commitFile("upstream.txt", fmt.Sprintf("upstream change %d\n", i))
			}
			switch name {
			case "reverted_upstream_format":
				commitFile("tracked.txt", "    base\n")
				commitFile("format.txt", "    base\n")
			case "chained_tips":
				// The earlier tip sorts first. Both tips differ from the feature in tracked.txt only.
				commitFile("tracked.txt", "    base\n")
				runReviewCLIGit(t, repo, "remote", "add", "mirror", "mirror.git")
				runReviewCLIGit(t, repo, "update-ref", "refs/remotes/mirror/main", "HEAD")
				commitFile("tracked.txt", "base\n")
			case "incomparable_tips":
				commitFile("upstream-extra.txt", "extra upstream path\n")
			}
			upstream := rev("HEAD")
			runReviewCLIGit(t, repo, "update-ref", upstreamRef, upstream)
			runReviewCLIGit(t, repo, "switch", "-qc", "feature", "--no-track", upstreamRef)
			if name == "chained_tips" {
				commitFile("tracked.txt", "base\nthe change under review\n")
			} else {
				commitFile("feature.txt", "the change under review\n")
			}
			want := upstream
			switch name {
			case "shorter_incomparable_range":
				// Both tips are in HEAD's history, but neither contains the other.
				runReviewCLIGit(t, repo, "merge", "--no-ff", "-qm", "merge fork default", origin)
				if got := strings.TrimSpace(runReviewCLIGit(t, repo, "merge-base", origin, upstream)); got != root {
					t.Fatalf("fixture merge-base = %q, want common root %q", got, root)
				}
				count := func(ref string) int {
					return len(strings.Fields(runReviewCLIGit(t, repo, "rev-list", ref+"..HEAD")))
				}
				if other, original := count(upstream), count(origin); other >= original {
					t.Fatalf("fixture upstream has %d commits to review, want fewer than origin's %d", other, original)
				}
				want = origin
			case "upstream_not_in_head":
				runReviewCLIGit(t, repo, "switch", "-q", "upstream-default")
				commitFile("upstream.txt", "upstream moved after the fork point\n")
				runReviewCLIGit(t, repo, "update-ref", upstreamRef, "HEAD")
				runReviewCLIGit(t, repo, "switch", "-q", "feature")
				want = origin
			case "reverted_upstream_format":
				runReviewCLIGit(t, repo, "revert", "--no-commit", origin+".."+upstream)
				runReviewCLIGit(t, repo, "commit", "-qm", "revert upstream changes")
			case "incomparable_tips":
				// Neither tip subsumes the other, even though upstream leaves fewer changed paths.
				runReviewCLIGit(t, repo, "switch", "-qc", "mirror-default", origin)
				commitFile("mirror.txt", "independent mirror path\n")
				runReviewCLIGit(t, repo, "remote", "add", "mirror", "mirror.git")
				runReviewCLIGit(t, repo, "update-ref", "refs/remotes/mirror/main", "HEAD")
				runReviewCLIGit(t, repo, "switch", "-q", "feature")
				runReviewCLIGit(t, repo, "merge", "--no-ff", "-qm", "merge mirror default", "mirror-default")
				want = origin
			}
			// A fully pushed feature has no unpushed range against its tracking ref.
			runReviewCLIGit(t, repo, "update-ref", "refs/remotes/origin/feature", "HEAD")
			runReviewCLIGit(t, repo, "branch", "--set-upstream-to=origin/feature")

			status := derivedRangeTerminalStatus(t, repo)
			if status.NextTransition == nil || status.NextTransition.Execute == nil || status.NextTransition.Execute.Operation != "review.start" {
				t.Fatalf("selectorless STATUS did not offer START: %#v", status.NextTransition)
			}
			if got := startTransitionArgumentValue(t, status, "committed-only"); got != "true" {
				t.Errorf("offered START committed-only = %q, want true", got)
			}
			if got := startTransitionArgumentValue(t, status, "base-ref"); got != want {
				t.Errorf("offered START base-ref = %q, want %q (origin %q, upstream fork point %q)", got, want, origin, upstream)
			}
			if name == "reverted_upstream_format" {
				// The reversal was authored on the feature branch and must stay visible, even though it expands the diff.
				if paths, want := status.Projection.Paths, []string{"feature.txt", "format.txt", "tracked.txt", "upstream.txt"}; !slices.Equal(paths, want) {
					t.Errorf("offered START changes %q, want %q", paths, want)
				}
			}
			if name == "fresh_upstream" {
				explicit := negotiatedStartStatusForContract(t, repo, ReviewIntegrationContractV2, "--base-ref", originRef, "--committed-only")
				// Explicit STATUS already freezes the caller's base as a tree in START.
				if got, want := startTransitionArgumentValue(t, explicit, "base-ref"), rev(originRef+"^{tree}"); got != want {
					t.Errorf("START base-ref after explicit STATUS = %q, want the explicitly selected tree %q", got, want)
				}
			}
		})
	}
}
