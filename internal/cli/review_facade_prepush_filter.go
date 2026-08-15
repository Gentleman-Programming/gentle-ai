package cli

import (
	"context"
	"errors"
	"os/exec"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// CompactPrePushDiscoveryBaseline is the shared, gate-scoped live resolution a
// gate discovery pass needs ONCE per unqualified GatePrePush call, so it can
// cheaply prove that an accumulated terminal leaf cannot govern the live push
// without paying AssessCompactGateTarget's full per-leaf git-subprocess cost for
// every historical lineage (issue #1886: bounded pre-push discovery).
//
// Mirrors CompactPreCommitDiscoveryBaseline but for pre-push, where the live
// push base is the commit the remote tracking branch points to, resolved via
// the configured upstream ref.
type CompactPrePushDiscoveryBaseline struct {
	// PushBaseTree is the TREE oid of the remote tracking base (the push
	// base). It must be a tree, not the commit, because leaf snapshots store
	// BaseTree as `rev-parse <ref>^{tree}` — issue #3176: comparing the
	// commit oid here made the fast path permanently inert.
	PushBaseTree string
	Paths        []string // live changed paths between the push base and HEAD
}

// BuildCompactPrePushDiscoveryBaseline resolves the baseline once. A non-nil
// error means the fast path is unavailable for this discovery pass; callers
// must fall back to full per-leaf assessment for every candidate.
func BuildCompactPrePushDiscoveryBaseline(ctx context.Context, repo string) (CompactPrePushDiscoveryBaseline, error) {
	// Resolve the push base commit using the same logic as selectPrePushBoundary.
	baseRef, err := resolvePrePushBaseCommit(ctx, repo)
	if err != nil {
		return CompactPrePushDiscoveryBaseline{}, err
	}
	// Build the TargetBaseDiff snapshot: base -> HEAD
	snapshot, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).BuildStoredSnapshot(ctx, reviewtransaction.Target{
		Kind:              reviewtransaction.TargetBaseDiff,
		BaseRef:           baseRef,
		IntendedUntracked: []string{},
	})
	if err != nil {
		return CompactPrePushDiscoveryBaseline{}, err
	}
	return CompactPrePushDiscoveryBaseline{PushBaseTree: snapshot.BaseTree, Paths: snapshot.Paths}, nil
}

// resolvePrePushBaseCommit resolves the push base commit (the commit the remote
// tracking branch points to). This mirrors the logic of selectPrePushBoundary
// but is inlined here to avoid exporting private functions.
func resolvePrePushBaseCommit(ctx context.Context, repo string) (string, error) {
	branch, err := runGit(ctx, repo, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return "", err
	}
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return "", errors.New("configured upstream is unavailable on detached HEAD; pass --base-ref <remote>/<branch>") // refusal:by-design world-action: the operator must be on a branch or provide an explicit --base-ref
	}

	upstream, err := runGit(ctx, repo, "for-each-ref", "--format=%(upstream:remotename)%00%(upstream:remoteref)", "refs/heads/"+branch)
	if err != nil {
		return "", err
	}
	parts := strings.SplitN(strings.TrimSpace(upstream), "\x00", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", errors.New("configured upstream is missing or ambiguous; pass --base-ref <remote>/<branch>") // refusal:by-design world-action: the operator must configure a valid tracking upstream or provide an explicit --base-ref
	}
	remote, ref := parts[0], parts[1]
	remoteBranch := remote + "/" + strings.TrimPrefix(ref, "refs/heads/")

	commit, err := runGit(ctx, repo, "rev-parse", "--verify", "--quiet", remoteBranch+"^{commit}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(commit), nil
}

// runGit runs a git command and returns the stdout trimmed.
func runGit(ctx context.Context, repo string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// CompactLeafProvablyUnrelatedToPrePushCandidate reports whether state provably
// cannot govern the live pre-push candidate described by liveBaseTree and
// liveChangedPaths. Mirrors the byte-identity argument from
// CompactLeafProvablyUnrelatedToPreCommitBaseline but for pre-push so we do not
// pay for an extra SnapshotBuilder.Build per leaf.
//
// Returns true (skip the leaf's AssessCompactGateTarget call) when ALL hold:
//   - Kind == TargetBaseDiff
//   - GenesisPaths != nil
//   - CurrentSnapshot.BaseTree == liveBaseTree
//   - state.GenesisPaths are disjoint from liveChangedPaths
//
// Returns false otherwise (and for TargetBaseWorkspaceOverlay, TargetCurrentChanges,
// TargetFixDiff, TargetExactRevision, GenesisPaths == nil).
//
// The caller is responsible for ensuring state is valid (e.g., via state.Validate())
// before calling this predicate. This function assumes the CompactState has already
// passed validation in the caller's context (discoverCompactFacadeGateReview calls
// store.Load() which validates before this predicate is reached).
func CompactLeafProvablyUnrelatedToPrePushCandidate(state reviewtransaction.CompactState, liveBaseTree string, liveChangedPaths []string) bool {
	current := state.CurrentSnapshot
	if current.Kind != reviewtransaction.TargetBaseDiff {
		return false
	}
	if len(state.GenesisPaths) == 0 {
		return false
	}
	if current.BaseTree != liveBaseTree {
		return false
	}
	return pathsAreDisjoint(state.GenesisPaths, liveChangedPaths)
}

// pathsAreDisjoint reports whether the two path sets have no element in common.
// Empty live paths are treated as a special case: an empty path set is considered
// disjoint from any genesis set because an empty candidate cannot be related to
// any non-empty genesis scope.
func pathsAreDisjoint(genesisPaths, livePaths []string) bool {
	if len(livePaths) == 0 {
		return true
	}
	if len(genesisPaths) == 0 {
		return false
	}
	// Build a set of genesis paths for O(1) lookup.
	known := make(map[string]struct{}, len(genesisPaths))
	for _, p := range genesisPaths {
		known[p] = struct{}{}
	}
	// Check if any live path is in the genesis set.
	for _, p := range livePaths {
		if _, ok := known[p]; ok {
			return false
		}
	}
	return true
}
