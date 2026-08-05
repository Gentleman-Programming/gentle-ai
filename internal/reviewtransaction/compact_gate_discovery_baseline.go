package reviewtransaction

import "context"

// CompactPreCommitDiscoveryBaseline is the shared, gate-scoped live
// resolution a gate discovery pass needs ONCE per unqualified GatePreCommit
// call, so it can cheaply prove that an accumulated terminal leaf cannot
// govern the live candidate without paying AssessCompactGateTarget's full
// per-leaf git-subprocess cost for every historical lineage (organic-dx
// Phase 3d: bound discovery cost as lineages accumulate).
//
// It is byte-identical, not approximate, to what AssessCompactGateTarget
// resolves for any ORDINARY (non "exact recommit") pre-commit leaf:
//   - buildCompactLifecycleSnapshot always forces IntendedUntracked to empty
//     for GatePreCommit's staged projection (compact_gate.go:531-533), so a
//     leaf's own IntendedUntracked never changes its resolved live snapshot;
//   - validateCompactReceiptMirrorScope always short-circuits to nil for a
//     staged projection, so the receipt-mirror check can never turn this
//     into a scope-changed result;
//   - lifecycleTargetForGate always resolves GatePostApply/GatePreCommit to
//     TargetCurrentChanges regardless of the leaf's OWN CurrentSnapshot.Kind
//     (gate.go:1250-1257), except TargetFixDiff/TargetBaseWorkspaceOverlay,
//     which take an earlier, different branch entirely and are excluded by
//     CompactLeafProvablyUnrelatedToPreCommitBaseline below;
//   - the only leaf-specific branch is the "exact recommit" case
//     (compact_gate.go's buildCompactGateRequestWithPushBase: headTree ==
//     current.CandidateTree), which resolves through TargetBaseDiff and
//     BuildStoredSnapshot instead. HeadTree is captured here so callers can
//     exclude any leaf that could take that branch.
type CompactPreCommitDiscoveryBaseline struct {
	HeadTree      string
	Paths         []string
	CandidateTree string
}

// BuildCompactPreCommitDiscoveryBaseline resolves the baseline once. A
// non-nil error means the fast path is unavailable for this discovery pass;
// callers must fall back to full per-leaf assessment for every candidate.
func BuildCompactPreCommitDiscoveryBaseline(ctx context.Context, repo string) (CompactPreCommitDiscoveryBaseline, error) {
	headTree, _, err := (SnapshotBuilder{Repo: repo}).resolveCurrentChangesBase(ctx, ProjectionStaged)
	if err != nil {
		return CompactPreCommitDiscoveryBaseline{}, err
	}
	snapshot, _, err := buildCompactLifecycleSnapshot(ctx, repo, GateRequest{
		Schema: GateRequestSchema, Gate: GatePreCommit,
		Target: Target{Kind: TargetCurrentChanges, Projection: ProjectionStaged, IntendedUntracked: []string{}},
	})
	if err != nil {
		return CompactPreCommitDiscoveryBaseline{}, err
	}
	return CompactPreCommitDiscoveryBaseline{HeadTree: headTree, Paths: snapshot.Paths, CandidateTree: snapshot.CandidateTree}, nil
}

// CompactLeafProvablyUnrelatedToPreCommitBaseline reports whether state
// provably cannot govern the live pre-commit candidate baseline describes,
// using only data already loaded in state (no additional git subprocess
// work). Eligibility is restricted to the leaves the type doc proves resolve
// byte-identically to baseline, so a true result here always matches the
// CompactGateTargetUnrelated AssessCompactGateTarget(GatePreCommit) itself
// would compute -- it never guesses.
func CompactLeafProvablyUnrelatedToPreCommitBaseline(state CompactState, baseline CompactPreCommitDiscoveryBaseline) bool {
	if err := state.Validate(); err != nil {
		return false
	}
	current := state.CurrentSnapshot
	if current.Kind == TargetFixDiff || current.Kind == TargetBaseWorkspaceOverlay {
		return false
	}
	if current.CandidateTree == baseline.HeadTree || current.CandidateTree == baseline.CandidateTree {
		return false
	}
	return classifyCompactPathSetRelation(state.GenesisPaths, baseline.Paths) == compactPathsDisjoint
}
