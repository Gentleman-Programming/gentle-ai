package reviewtransaction

import "context"

// AcknowledgedPassivePredecessor reports whether a live committed base-diff
// candidate only adds passive content to a candidate that was already
// approved and acknowledged (issue #4739). After an acknowledgement, one more
// commit that touches a tracking note changes the target identity, and the
// full base-diff would otherwise be offered for review again although every
// reviewable byte in it was just approved.
//
// The walk follows HEAD's first-parent ancestry back to the candidate's base
// tree, recomputes each ancestor's base-diff identity against that same base,
// and stops at the nearest ancestor with terminal consumption evidence. Only
// a delta from that ancestor that classifies as low risk (passive content
// only) qualifies.
//
// The result may only suppress a fresh review offer. Terminal consumption is
// a deduplication fact, never authority: callers must not derive a receipt or
// a delivery gate from it, must prefer live authority, and must offer the
// review whenever this returns an error.
func AcknowledgedPassivePredecessor(ctx context.Context, repo string, live Snapshot) (bool, error) {
	boundary, ok, err := AcknowledgedBoundaryCommit(ctx, repo, live)
	if err != nil || !ok {
		return false, err
	}
	return boundary.Passive, nil
}

// AcknowledgedBoundary reports the nearest acknowledged first-parent ancestor
// of a live committed base-diff candidate: the reviewed boundary (issue #5638).
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
	// Passive reports whether that delta classifies as low risk (the #4739
	// passive suppression, folded into the same walk).
	Passive bool
}

// AcknowledgedBoundaryCommit walks HEAD's first-parent ancestry back to the
// candidate's base tree, recomputes each ancestor's base-diff identity against
// that same base, and stops at the nearest ancestor with terminal consumption
// evidence: the reviewed boundary. ok is false when no ancestor in the window
// was acknowledged (or the walk reaches the base first); err is non-nil for
// any Git or tombstone fault, and callers must keep the pre-existing offer.
//
// The result may only reshape or withhold a fresh review offer. Terminal
// consumption is a deduplication fact, never authority: callers must not
// derive a receipt or a delivery gate from it, must prefer live authority, and
// must keep a live lineage visible.
func AcknowledgedBoundaryCommit(ctx context.Context, repo string, live Snapshot) (AcknowledgedBoundary, bool, error) {
	if live.Kind != TargetBaseDiff || len(live.IntendedUntracked) != 0 || len(live.Paths) == 0 {
		return AcknowledgedBoundary{}, false, nil
	}
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
	for _, ancestor := range ancestors {
		if ancestor.tree == live.BaseTree {
			return AcknowledgedBoundary{}, false, nil
		}
		paths, err := builder.changedPaths(ctx, live.BaseTree, ancestor.tree)
		if err != nil {
			return AcknowledgedBoundary{}, false, err
		}
		identity := IdentityForComponents(TargetBaseDiff, live.Projection, live.BaseTree, ancestor.tree, digestPaths(paths))
		consumed, err := CompactTargetConsumed(ctx, root, identity)
		if err != nil {
			return AcknowledgedBoundary{}, false, err
		}
		if !consumed {
			continue
		}
		deltaPaths, err := builder.changedPaths(ctx, ancestor.tree, live.CandidateTree)
		if err != nil {
			return AcknowledgedBoundary{}, false, err
		}
		if len(deltaPaths) == 0 {
			// Every commit since the boundary was empty: the boundary reaches
			// the live content, so there is no delta to classify.
			return AcknowledgedBoundary{Commit: ancestor.commit, Tree: ancestor.tree}, true, nil
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
		// guard:population acknowledged-passive-delta too-loose: only a committed HEAD base-diff whose nearest acknowledged first-parent ancestor (same base tree, exact recomputed identity) is reported, with the delta to the live candidate classified by the shared risk assessor; code, tests, configuration, operational markdown, and any lookup failure keep the review offer
		return AcknowledgedBoundary{
			Commit: ancestor.commit, Tree: ancestor.tree,
			DeltaPaths: deltaPaths, Passive: assessment.Level == RiskLow,
		}, true, nil
	}
	return AcknowledgedBoundary{}, false, nil
}
