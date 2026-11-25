package reviewtransaction

import (
	"context"
	"slices"
)

// AcknowledgedPassivePredecessor reports whether a live committed base-diff
// candidate only adds passive content to a candidate that was already
// approved and acknowledged (issue #4739). After an acknowledgement, one more
// commit that touches a tracking note changes the target identity, and the
// full base-diff would otherwise be offered for review again although every
// reviewable byte in it was just approved.
//
// The walk finds the reviewed boundary through exact terminal consumption
// evidence. Only a delta from that boundary that classifies as low risk
// (passive content only) qualifies.
//
// The result may only suppress a fresh review offer. Terminal consumption is
// a deduplication fact, never authority: callers must not derive a receipt or
// a delivery gate from it, must prefer live authority, and must offer the
// review whenever this returns an error.
func AcknowledgedPassivePredecessor(ctx context.Context, repo string, live Snapshot) (bool, error) {
	boundary, ok, err := acknowledgedBoundaryCommit(ctx, repo, live, false)
	if err != nil || !ok {
		return false, err
	}
	return boundary.Passive, nil
}

// AcknowledgedBoundary reports the latest first-parent ancestor covered by
// contiguous acknowledgements from the candidate's base: the reviewed boundary.
type AcknowledgedBoundary struct {
	// Commit and Tree name the boundary commit and its tree: the base a
	// delta-scoped candidate derives from, and the --base-ref a START for it
	// must disclose.
	Commit string
	Tree   string
	// DeltaPaths is the change set from the boundary tree to the live
	// candidate tree. Empty only when every commit since the boundary was
	// empty: the boundary reaches the live content.
	DeltaPaths []string
	// Passive reports whether that delta can be suppressed as low risk. An
	// exact consumed range is selected instead, even if its content is passive.
	Passive bool
}

// AcknowledgedBoundaryCommit finds the newest consumed range from the
// candidate's base tree in HEAD's first-parent ancestry. From that checkpoint,
// it prefers an exact consumed range to the live tree before advancing to
// any consumed intermediate delta. ok is false when the bounded walk finds no
// original checkpoint; Git or tombstone faults keep the pre-existing offer.
//
// The result may only reshape or withhold a fresh review offer. Terminal
// consumption is a deduplication fact, never authority: callers must not
// derive a receipt or a delivery gate from it, must prefer live authority, and
// must keep a live lineage visible.
func AcknowledgedBoundaryCommit(ctx context.Context, repo string, live Snapshot) (AcknowledgedBoundary, bool, error) {
	return acknowledgedBoundaryCommit(ctx, repo, live, true)
}

func acknowledgedBoundaryCommit(ctx context.Context, repo string, live Snapshot, preferExact bool) (AcknowledgedBoundary, bool, error) {
	if live.Kind != TargetBaseDiff || len(live.IntendedUntracked) != 0 || len(live.Paths) == 0 {
		return AcknowledgedBoundary{}, false, nil
	}
	defer StartStatusPhase(ctx, StatusPhasePredecessorConsumptionAncestry)()
	builder := SnapshotBuilder{Repo: repo}
	root, err := builder.repositoryRoot(ctx)
	if err != nil {
		return AcknowledgedBoundary{}, false, err
	}
	builder.Repo = root
	// The ancestry is HEAD's, so the candidate must be exactly HEAD's tree.
	headTree, err := builder.resolveTree(ctx, "HEAD")
	if err != nil || headTree != live.CandidateTree {
		return AcknowledgedBoundary{}, false, err
	}
	ancestors, err := selectorlessCommittedBaseDiffAncestors(ctx, root)
	if err != nil {
		return AcknowledgedBoundary{}, false, err
	}
	if len(ancestors) == 0 {
		return AcknowledgedBoundary{}, false, nil
	}
	// Every candidate range checks a tombstone in the same repository. Resolve
	// its authority directory once and revalidate the Git identity before
	// using a matched boundary; a changed repository must not suppress review.
	lease, err := openRepositoryIdentityLeaseAtRoot(ctx, root)
	if err != nil {
		return AcknowledgedBoundary{}, false, err
	}
	authority := lease.Identity()
	consumptionRoot := reviewAuthorityRootIn(authority.GitCommonDir)
	confirm := func(boundary AcknowledgedBoundary) (AcknowledgedBoundary, bool, error) {
		if err := lease.Validate(ctx); err != nil {
			return AcknowledgedBoundary{}, false, err
		}
		return boundary, true, nil
	}
	consumedRange := func(baseTree, candidateTree string) ([]string, bool, error) {
		paths, err := builder.changedPaths(ctx, baseTree, candidateTree)
		if err != nil {
			return nil, false, err
		}
		identity := IdentityForComponents(TargetBaseDiff, live.Projection, baseTree, candidateTree, digestPaths(paths))
		consumed, err := compactTargetConsumedAtRoot(consumptionRoot, authority.RepositoryRoot, identity)
		return paths, consumed, err
	}
	// The walk starts at the branch point. Later slices may START from any
	// reached boundary (A..B, A..C, B..E): check every reached boundary newest
	// first and advance only on its first exact acknowledgement.
	// A branch point older than the window still starts the walk by its tree.
	ancestor := selectorlessCommittedBaseDiffAncestor{tree: live.BaseTree}
	start := slices.IndexFunc(ancestors, func(entry selectorlessCommittedBaseDiffAncestor) bool { return entry.tree == live.BaseTree })
	if start < 0 {
		start = len(ancestors)
	} else {
		ancestor = ancestors[start]
	}
	reached := []selectorlessCommittedBaseDiffAncestor{ancestor}
	for next, checked := start-1, live.BaseTree; ; next-- {
		// A legacy acknowledgement may cover HEAD from this boundary,
		// even when a shorter prefix was also consumed. Check once per
		// boundary before advancing, and keep the exact matched base.
		if preferExact && checked != ancestor.tree && ancestor.tree != live.CandidateTree {
			checked = ancestor.tree
			paths, consumed, err := consumedRange(ancestor.tree, live.CandidateTree)
			if err != nil {
				return AcknowledgedBoundary{}, false, err
			}
			if consumed {
				return confirm(AcknowledgedBoundary{Commit: ancestor.commit, Tree: ancestor.tree, DeltaPaths: paths})
			}
		}
		if next < 0 {
			break
		}
		descendant := ancestors[next]
		if descendant.tree == ancestor.tree {
			continue
		}
		for i := len(reached) - 1; i >= 0; i-- {
			if base := reached[i]; base.tree == descendant.tree {
				continue
			}
			_, consumed, err := consumedRange(reached[i].tree, descendant.tree)
			if err != nil {
				return AcknowledgedBoundary{}, false, err
			}
			if consumed {
				reached, ancestor = append(reached, descendant), descendant
				break
			}
		}
	}
	if ancestor.tree == live.BaseTree {
		return AcknowledgedBoundary{}, false, nil
	}
	deltaPaths, err := builder.changedPaths(ctx, ancestor.tree, live.CandidateTree)
	if err != nil {
		return AcknowledgedBoundary{}, false, err
	}
	if len(deltaPaths) == 0 {
		// Every commit since the boundary was empty: the boundary reaches
		// the live content, so there is no delta to classify.
		return confirm(AcknowledgedBoundary{Commit: ancestor.commit, Tree: ancestor.tree})
	}
	delta := Snapshot{
		Kind: TargetBaseDiff, Projection: live.Projection,
		BaseTree: ancestor.tree, CandidateTree: live.CandidateTree,
		GeneratedPathInterpretation: live.GeneratedPathInterpretation,
		Paths:                       deltaPaths, PathsDigest: digestPaths(deltaPaths),
	}
	assessment, err := builder.AssessSnapshotRisk(ctx, delta)
	if err != nil {
		return AcknowledgedBoundary{}, false, err
	}
	// guard:population acknowledged-passive-delta too-loose: only a committed HEAD base-diff whose first-parent boundary is reached by contiguous, exact consumed identities from the same base tree is reported; the shared risk assessor classifies the remaining delta, and lookup failures keep the offer
	return confirm(AcknowledgedBoundary{
		Commit: ancestor.commit, Tree: ancestor.tree,
		DeltaPaths: deltaPaths, Passive: assessment.Level == RiskLow,
	})
}
