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
	if live.Kind != TargetBaseDiff || len(live.IntendedUntracked) != 0 || len(live.Paths) == 0 {
		return false, nil
	}
	builder := SnapshotBuilder{Repo: repo}
	root, err := builder.repositoryRoot(ctx)
	if err != nil {
		return false, err
	}
	builder.Repo = root
	// The ancestry is HEAD's, so the candidate must be exactly HEAD's tree.
	headTree, err := builder.resolveTree(ctx, "HEAD")
	if err != nil || headTree != live.CandidateTree {
		return false, err
	}
	ancestors, err := selectorlessCommittedBaseDiffAncestors(ctx, root)
	if err != nil {
		return false, err
	}
	for _, ancestor := range ancestors {
		if ancestor.tree == live.BaseTree {
			return false, nil
		}
		paths, err := builder.changedPaths(ctx, live.BaseTree, ancestor.tree)
		if err != nil {
			return false, err
		}
		identity := IdentityForComponents(TargetBaseDiff, live.Projection, live.BaseTree, ancestor.tree, digestPaths(paths))
		consumed, err := CompactTargetConsumed(ctx, root, identity)
		if err != nil {
			return false, err
		}
		if !consumed {
			continue
		}
		deltaPaths, err := builder.changedPaths(ctx, ancestor.tree, live.CandidateTree)
		if err != nil || len(deltaPaths) == 0 {
			return false, err
		}
		delta := Snapshot{
			Kind: TargetBaseDiff, Projection: live.Projection,
			BaseTree: ancestor.tree, CandidateTree: live.CandidateTree,
			GeneratedPathInterpretation: live.GeneratedPathInterpretation,
			Paths:                       deltaPaths, PathsDigest: digestPaths(deltaPaths),
		}
		assessment, err := builder.AssessSnapshotRisk(ctx, delta)
		if err != nil {
			return false, err
		}
		// guard:population acknowledged-passive-delta too-loose: only a committed HEAD base-diff whose nearest acknowledged first-parent ancestor (same base tree, exact recomputed identity) is followed by a low-risk passive delta; code, tests, configuration, operational markdown, and any lookup failure keep the review offer
		return assessment.Level == RiskLow, nil
	}
	return false, nil
}
