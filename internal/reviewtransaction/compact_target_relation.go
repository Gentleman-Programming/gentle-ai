package reviewtransaction

// compactTargetRelationKind is the provider-owned authorization vocabulary for
// comparing one frozen compact target with a newly derived live target. The
// classification is evidence only: callers still decide which transitions a
// relation may authorize at recovery, status, or a delivery gate.
type compactTargetRelationKind string

const (
	compactTargetSame                compactTargetRelationKind = "same"
	compactTargetCompatibleAdvance   compactTargetRelationKind = "compatible-advance"
	compactTargetProvableContraction compactTargetRelationKind = "provable-contraction"
	compactTargetChangedScope        compactTargetRelationKind = "changed-scope"
	compactTargetUnsafe              compactTargetRelationKind = "unsafe"
)

type compactPathSetRelation string

const (
	compactPathsSame        compactPathSetRelation = "same"
	compactPathsContraction compactPathSetRelation = "contraction"
	compactPathsOverlap     compactPathSetRelation = "overlap"
	compactPathsDisjoint    compactPathSetRelation = "disjoint"
	compactPathsInvalid     compactPathSetRelation = "invalid"
)

type compactTargetRelationEvidence struct {
	CompatibleAdvance *BaseAdvanceCompatibility
	// ExplicitScopeChange records the append-only recovery ceremony that
	// creates a fresh reviewing successor. It does not transfer approval, so a
	// disjoint but genuinely changed target may be named without making the
	// same target applicable during unqualified status or gate selection.
	ExplicitScopeChange bool
}

type compactTargetRelation struct {
	Kind  compactTargetRelationKind
	Paths compactPathSetRelation
}

// classifyCompactTargetRelation is deliberately pure. Git-derived compatible
// base evidence is supplied by the caller, while canonical path-set and target
// relationships are classified identically for recovery, negotiated status,
// gate authorization, and chain normalization.
func classifyCompactTargetRelation(frozen, live Snapshot, genesisPaths []string, evidence compactTargetRelationEvidence) compactTargetRelation {
	paths := classifyCompactPathSetRelation(genesisPaths, live.Paths)
	relation := compactTargetRelation{Kind: compactTargetUnsafe, Paths: paths}
	if paths == compactPathsInvalid || !sameRecoveryProjection(frozen.Projection, live.Projection) {
		return relation
	}
	substantiveScopeChange := frozen.CandidateTree != live.CandidateTree || !equalStrings(frozen.Paths, live.Paths)
	if !compactStartTargetKindsCompatible(frozen.Kind, live.Kind) &&
		!(evidence.ExplicitScopeChange && substantiveScopeChange) {
		return relation
	}

	if proof := evidence.CompatibleAdvance; proof != nil && proof.valid() &&
		proof.OriginalMergeBaseTree == frozen.BaseTree && proof.NewBaseTree == live.BaseTree &&
		frozen.CandidateTree == live.CandidateTree && proof.DeliveredPathsDigest == digestPaths(frozen.Paths) {
		relation.Kind = compactTargetCompatibleAdvance
		return relation
	}
	if frozen.BaseTree == live.BaseTree && frozen.CandidateTree == live.CandidateTree &&
		equalStrings(frozen.Paths, live.Paths) {
		relation.Kind = compactTargetSame
		return relation
	}
	if paths == compactPathsContraction {
		relation.Kind = compactTargetProvableContraction
		return relation
	}
	// An accepted degenerate recovery wrapper can have an empty path set while
	// retaining the predecessor candidate exactly. That is related authority,
	// not an unrelated disjoint target, and chain normalization handles it.
	if frozen.CandidateTree == live.CandidateTree || paths == compactPathsSame || paths == compactPathsOverlap {
		relation.Kind = compactTargetChangedScope
	} else if evidence.ExplicitScopeChange &&
		(frozen.BaseTree != live.BaseTree || frozen.CandidateTree != live.CandidateTree || !equalStrings(frozen.Paths, live.Paths)) {
		relation.Kind = compactTargetChangedScope
	}
	return relation
}

func classifyCompactPathSetRelation(genesisPaths, livePaths []string) compactPathSetRelation {
	genesis, genesisErr := canonicalPaths(genesisPaths)
	live, liveErr := canonicalPaths(livePaths)
	if genesisErr != nil || liveErr != nil || !equalStrings(genesis, genesisPaths) || !equalStrings(live, livePaths) {
		return compactPathsInvalid
	}
	if equalStrings(genesis, live) {
		return compactPathsSame
	}
	if len(live) > 0 && len(live) < len(genesis) && pathsAreSubset(live, genesis) == nil {
		return compactPathsContraction
	}
	known := make(map[string]struct{}, len(genesis))
	for _, path := range genesis {
		known[path] = struct{}{}
	}
	for _, path := range live {
		if _, ok := known[path]; ok {
			return compactPathsOverlap
		}
	}
	return compactPathsDisjoint
}
