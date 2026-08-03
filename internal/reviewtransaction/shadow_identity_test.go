package reviewtransaction

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This file exercises shadowCandidateIdentity (shadow_identity.go) against
// rdd-candidate-identity's requirements: Canonical Identity Structure,
// Selector Normalization, Read-Only Resolution, and Deterministic
// Ambiguity/Failure Reporting, plus the design's Git-repository-selection
// threat (linked worktree / separate git dir / absolute vs relative path).

// TestShadowIdentityStructureIsComplete proves every resolved
// CandidateIdentity populates all five canonical fields deterministically
// and carries nothing selector-specific.
func TestShadowIdentityStructureIsComplete(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "workspace.txt", "workspace change\n")

	first, err := shadowCandidateIdentity(context.Background(), shadowSelector{
		Kind: shadowSelectorWorkspace, Repo: repo,
	})
	if err != nil {
		t.Fatalf("shadowCandidateIdentity() error = %v, want nil", err)
	}

	lease, err := OpenRepositoryIdentityLease(context.Background(), repo)
	if err != nil {
		t.Fatalf("OpenRepositoryIdentityLease: %v", err)
	}
	if first.RepositoryID == "" || first.RepositoryID != lease.Identity().RepositoryRef {
		t.Fatalf("RepositoryID = %q, want lease RepositoryRef %q", first.RepositoryID, lease.Identity().RepositoryRef)
	}
	if first.BaseTree == "" || first.CandidateTree == "" {
		t.Fatalf("BaseTree/CandidateTree not populated: %#v", first)
	}
	if first.ChangedPathsModesDigest == "" || !strings.HasPrefix(first.ChangedPathsModesDigest, "sha256:") {
		t.Fatalf("ChangedPathsModesDigest = %q, want a populated sha256: digest", first.ChangedPathsModesDigest)
	}
	if first.PolicyHash != "unknown" {
		t.Fatalf("PolicyHash = %q, want %q when no source is supplied", first.PolicyHash, "unknown")
	}

	// Deterministic: resolving the exact same selector again reproduces the
	// same five values.
	second, err := shadowCandidateIdentity(context.Background(), shadowSelector{
		Kind: shadowSelectorWorkspace, Repo: repo,
	})
	if err != nil {
		t.Fatalf("shadowCandidateIdentity() second call error = %v, want nil", err)
	}
	if first != second {
		t.Fatalf("resolution is not deterministic: first=%#v second=%#v", first, second)
	}
}

// TestShadowIdentityPolicyHashNeverFabricated proves policy_hash comes only
// from an explicit source and is "unknown" — never fabricated — when absent.
func TestShadowIdentityPolicyHashNeverFabricated(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)

	withoutSource, err := shadowCandidateIdentity(context.Background(), shadowSelector{
		Kind: shadowSelectorWorkspace, Repo: repo,
	})
	if err != nil {
		t.Fatalf("shadowCandidateIdentity() error = %v, want nil", err)
	}
	if withoutSource.PolicyHash != "unknown" {
		t.Fatalf("PolicyHash = %q, want %q", withoutSource.PolicyHash, "unknown")
	}

	known := "sha256:" + strings.Repeat("ab", 32)
	withSource, err := shadowCandidateIdentity(context.Background(), shadowSelector{
		Kind: shadowSelectorWorkspace, Repo: repo,
		PolicyHashSource: func() (string, bool) { return known, true },
	})
	if err != nil {
		t.Fatalf("shadowCandidateIdentity() error = %v, want nil", err)
	}
	if withSource.PolicyHash != known {
		t.Fatalf("PolicyHash = %q, want the supplied %q", withSource.PolicyHash, known)
	}

	unknownSource, err := shadowCandidateIdentity(context.Background(), shadowSelector{
		Kind: shadowSelectorWorkspace, Repo: repo,
		PolicyHashSource: func() (string, bool) { return "", false },
	})
	if err != nil {
		t.Fatalf("shadowCandidateIdentity() error = %v, want nil", err)
	}
	if unknownSource.PolicyHash != "unknown" {
		t.Fatalf("PolicyHash = %q, want %q when the source reports known=false", unknownSource.PolicyHash, "unknown")
	}
}

// TestShadowIdentitySelectorsConverge proves the four selector variants
// normalize into the same CandidateIdentity shape without a per-selector
// lifecycle: staged and workspace converge when they reference identical
// tree content, and committed-range resolves the range's effective base and
// candidate trees canonically.
func TestShadowIdentitySelectorsConverge(t *testing.T) {
	requireSnapshotGit(t)

	t.Run("staged and workspace converge on identical tree content", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		writeSnapshotFile(t, repo, "converge.txt", "staged and unchanged further\n")
		gitSnapshot(t, repo, "add", "--", "converge.txt")
		// No further unstaged edits: the index tree and the workspace tree
		// are identical, so workspace and staged selectors must converge.

		workspace, err := shadowCandidateIdentity(context.Background(), shadowSelector{
			Kind: shadowSelectorWorkspace, Repo: repo,
		})
		if err != nil {
			t.Fatalf("workspace selector: %v", err)
		}
		staged, err := shadowCandidateIdentity(context.Background(), shadowSelector{
			Kind: shadowSelectorStaged, Repo: repo,
		})
		if err != nil {
			t.Fatalf("staged selector: %v", err)
		}
		if workspace != staged {
			t.Fatalf("staged and workspace selectors diverged: workspace=%#v staged=%#v", workspace, staged)
		}
	})

	t.Run("committed-range resolves the range's effective base and candidate trees", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		first := trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD"))
		firstTree := trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD^{tree}"))
		writeSnapshotFile(t, repo, "range.txt", "second commit\n")
		gitSnapshot(t, repo, "add", "--", "range.txt")
		gitSnapshot(t, repo, "commit", "-m", "second")
		second := trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD"))
		secondTree := trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD^{tree}"))

		identity, err := shadowCandidateIdentity(context.Background(), shadowSelector{
			Kind: shadowSelectorCommittedRange, Repo: repo, Revision: first + ".." + second,
		})
		if err != nil {
			t.Fatalf("committed-range selector: %v", err)
		}
		if identity.BaseTree != firstTree || identity.CandidateTree != secondTree {
			t.Fatalf("committed-range identity = %#v, want base=%q candidate=%q", identity, firstTree, secondTree)
		}
	})
}

// TestShadowIdentityRepositorySelectionThreat proves the resolver's
// repository_id always equals the lease's own RepositoryRef — for a linked
// worktree (whose .git is a separate-git-dir redirect to the common dir) and
// for absolute vs relative repo path aliases of the same repository — rather
// than an independently (and possibly divergent) derived value.
func TestShadowIdentityRepositorySelectionThreat(t *testing.T) {
	requireSnapshotGit(t)

	t.Run("linked worktree resolves to its own lease RepositoryRef", func(t *testing.T) {
		primary, linked := initRepositoryIdentityLeaseWorktree(t)
		for _, repo := range []string{primary, linked} {
			lease, err := OpenRepositoryIdentityLease(context.Background(), repo)
			if err != nil {
				t.Fatalf("OpenRepositoryIdentityLease(%s): %v", repo, err)
			}
			identity, err := shadowCandidateIdentity(context.Background(), shadowSelector{
				Kind: shadowSelectorWorkspace, Repo: repo,
			})
			if err != nil {
				t.Fatalf("shadowCandidateIdentity(%s): %v", repo, err)
			}
			if identity.RepositoryID != lease.Identity().RepositoryRef {
				t.Fatalf("RepositoryID = %q, want lease RepositoryRef %q for %s", identity.RepositoryID, lease.Identity().RepositoryRef, repo)
			}
		}
	})

	t.Run("absolute and relative repo path aliases resolve identically", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		workingDirectory, err := os.Getwd()
		if err != nil {
			t.Fatalf("os.Getwd: %v", err)
		}
		relative, relativeErr := filepath.Rel(workingDirectory, repo)
		if relativeErr != nil {
			t.Skip("relative alias form is unavailable on this platform")
		}

		absoluteIdentity, err := shadowCandidateIdentity(context.Background(), shadowSelector{
			Kind: shadowSelectorWorkspace, Repo: repo,
		})
		if err != nil {
			t.Fatalf("absolute path selector: %v", err)
		}
		relativeIdentity, err := shadowCandidateIdentity(context.Background(), shadowSelector{
			Kind: shadowSelectorWorkspace, Repo: relative,
		})
		if err != nil {
			t.Fatalf("relative path selector: %v", err)
		}
		if absoluteIdentity != relativeIdentity {
			t.Fatalf("absolute and relative path aliases diverged: absolute=%#v relative=%#v", absoluteIdentity, relativeIdentity)
		}
	})
}

// TestShadowIdentityReadOnlyResolution proves the resolver leaves repository
// state untouched (Requirement: Read-Only Resolution) and that repeated
// resolution of the same selector is idempotent.
func TestShadowIdentityReadOnlyResolution(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "readonly.txt", "untouched by resolution\n")
	gitSnapshot(t, repo, "add", "--", "readonly.txt")

	headBefore := trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	statusBefore := gitSnapshot(t, repo, "status", "--porcelain")

	first, err := shadowCandidateIdentity(context.Background(), shadowSelector{Kind: shadowSelectorStaged, Repo: repo})
	if err != nil {
		t.Fatalf("shadowCandidateIdentity() first call: %v", err)
	}
	second, err := shadowCandidateIdentity(context.Background(), shadowSelector{Kind: shadowSelectorStaged, Repo: repo})
	if err != nil {
		t.Fatalf("shadowCandidateIdentity() second call: %v", err)
	}

	headAfter := trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	statusAfter := gitSnapshot(t, repo, "status", "--porcelain")

	if headBefore != headAfter {
		t.Fatalf("HEAD changed: before=%q after=%q", headBefore, headAfter)
	}
	if statusBefore != statusAfter {
		t.Fatalf("working tree/index status changed: before=%q after=%q", statusBefore, statusAfter)
	}
	if first != second {
		t.Fatalf("resolution is not idempotent: first=%#v second=%#v", first, second)
	}
}

// TestShadowIdentityAmbiguousSelectorReturnsFullSet proves a selector that
// legitimately matches more than one equally applicable live target returns
// every candidate rather than picking the most recent one, and performs no
// mutation while ambiguous.
func TestShadowIdentityAmbiguousSelectorReturnsFullSet(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	base := trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	writeSnapshotFile(t, repo, "staged-ambiguous.txt", "staged\n")
	gitSnapshot(t, repo, "add", "--", "staged-ambiguous.txt")

	headBefore := trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD"))

	_, err := shadowCandidateIdentity(context.Background(), shadowSelector{
		Kind: shadowSelectorStaged, Repo: repo, BaseRef: base, LedgerIDs: []string{"ledger-1"},
	})
	var ambiguity *shadowIdentityAmbiguity
	if !errors.As(err, &ambiguity) {
		t.Fatalf("error = %T %v, want *shadowIdentityAmbiguity", err, err)
	}
	if len(ambiguity.Candidates) != 2 {
		t.Fatalf("Candidates = %d, want 2 equally applicable candidates: %#v", len(ambiguity.Candidates), ambiguity.Candidates)
	}
	if ambiguity.Candidates[0] == ambiguity.Candidates[1] {
		t.Fatalf("ambiguous candidates were not distinct: %#v", ambiguity.Candidates)
	}
	for _, candidate := range ambiguity.Candidates {
		if candidate.RepositoryID != ambiguity.RepositoryID {
			t.Fatalf("candidate RepositoryID = %q, want ambiguity RepositoryID %q", candidate.RepositoryID, ambiguity.RepositoryID)
		}
	}

	headAfter := trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	if headBefore != headAfter {
		t.Fatalf("HEAD changed while resolving an ambiguous selector: before=%q after=%q", headBefore, headAfter)
	}
}

// TestShadowIdentityUnresolvableSelectorFailsClosed proves an unresolvable
// selector returns a typed failure carrying whatever evidence the resolver
// could gather, and never guesses a candidate identity.
func TestShadowIdentityUnresolvableSelectorFailsClosed(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)

	_, err := shadowCandidateIdentity(context.Background(), shadowSelector{
		Kind: shadowSelectorCommittedRange, Repo: repo, Revision: "not-a-real-revision",
	})
	var failure *shadowIdentityFailure
	if !errors.As(err, &failure) {
		t.Fatalf("error = %T %v, want *shadowIdentityFailure", err, err)
	}
	if failure.RepositoryID == "" {
		t.Fatalf("failure = %#v, want RepositoryID evidence gathered before the failure", failure)
	}

	// A selector missing every field a live target needs is also
	// unresolvable, not a silently-guessed default.
	_, err = shadowCandidateIdentity(context.Background(), shadowSelector{
		Kind: shadowSelectorCommittedRange, Repo: repo,
	})
	if !errors.As(err, &failure) {
		t.Fatalf("error = %T %v, want *shadowIdentityFailure for an empty committed-range selector", err, err)
	}
}

// TestShadowIdentityPiOverlaySelectorIsOutOfWave1Scope proves the resolver
// never claims to resolve a gentle-pi protocol-1.1 overlay selector as a
// supported Wave 1 selector (spec.md "Wave 1 Selector Scope" -> "Pi overlay
// selector is explicitly out of scope"). shadowSelectorKind is a closed
// 4-value enum; an unrecognized kind falls through shadowSelectorTarget's
// default case and fails closed, never fabricating a CandidateIdentity.
func TestShadowIdentityPiOverlaySelectorIsOutOfWave1Scope(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)

	identity, err := shadowCandidateIdentity(context.Background(), shadowSelector{
		Kind: shadowSelectorKind("gentle-pi-protocol-1.1-overlay"), Repo: repo, BaseRef: "HEAD",
	})
	var failure *shadowIdentityFailure
	if !errors.As(err, &failure) {
		t.Fatalf("error = %T %v, want *shadowIdentityFailure for an out-of-scope pi-overlay selector", err, err)
	}
	if !strings.Contains(failure.Reason, "unsupported shadow selector kind") {
		t.Fatalf("failure.Reason = %q, want it to name the selector kind as unsupported", failure.Reason)
	}
	if identity != (CandidateIdentity{}) {
		t.Fatalf("identity = %#v, want zero value: resolver must not fabricate a tuple for an out-of-scope selector", identity)
	}
}
