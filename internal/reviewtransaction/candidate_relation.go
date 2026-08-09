// Package reviewtransaction — candidate relation algebra (Wave 1 Slice 3;
// promoted out of the shadow gate in Wave 3 Slice 1, design decision 2).
// This file used to be shadow_relation.go, part of the read-only shadow of
// the target RDD relation model
// (docs/architecture/rdd-root-simplification-design.md). Promotion means it
// now serves the live ReviewCore (Wave 3 Slice 3+) directly — the Wave 1
// shadow observer that also called it (shadow_observer.go,
// GENTLE_AI_RDD_SHADOW) retired in Wave 7 S2a; this file's algebra is
// unaffected. It compares a frozen CandidateIdentity with a live one and
// returns exactly one of seven target-architecture relations, in a fixed
// fail-closed order (design decision 5). It must still never mutate
// authority state, a Store, or a CompactState — see
// candidate_readonly_guard_test.go for the AST guard that enforces this.
//
// CandidateRelation is the second symbol this slice exports (design
// decision 1, after Slice 2's CandidateIdentity); everything else here
// stays unexported until ReviewCore needs it.
package reviewtransaction

// CandidateRelation is the exact seven-value relation vocabulary
// (Requirement: Seven-Value Relation Output,
// openspec/changes/rdd-root-simplification-wave1/specs/rdd-candidate-relation-algebra/spec.md:9-11).
// No eighth value is ever produced.
//
// ShadowRelation is a type alias (Wave 3 Slice 1, design decision 2) kept
// so Wave 1's remaining tests keep compiling unchanged after the promotion
// rename — it is the exact same type, not a distinct one. Its own retiring
// is deferred past Wave 7 S2a/S2b/S2c pending a rename pass across the
// tests that still spell it out (see Wave 7 apply-progress).
type CandidateRelation string

type ShadowRelation = CandidateRelation

const (
	ShadowRelationExact                 CandidateRelation = "exact"
	ShadowRelationCompatibleBaseAdvance CandidateRelation = "compatible_base_advance"
	ShadowRelationProvableContraction   CandidateRelation = "provable_contraction"
	ShadowRelationChanged               CandidateRelation = "changed"
	ShadowRelationUnrelated             CandidateRelation = "unrelated"
	ShadowRelationAmbiguous             CandidateRelation = "ambiguous"
	ShadowRelationUnknown               CandidateRelation = "unknown"
)

// shadowRelationInput is the pure evaluation input (design.md "Interfaces /
// Contracts"). LiveUnresolvable is a design elaboration beyond the design's
// literal field sketch: it is the single boolean signal a caller (a future
// PR5 gate call site, or this slice's own threat-matrix tests) sets when
// commit-state or push-state resolution could not produce a stable live
// candidate at all — unborn HEAD with nothing staged, or an unresolvable
// push/pre-pr boundary (Threat Matrix rows "Commit state" / "Push state").
// Handoff note carried in tasks.md 3.5/3.6: "unknown is a relation-function
// outcome, not an identity-resolver outcome" — this field is how that
// outcome reaches the pure ordered function without relateCandidates itself
// needing to know *why* resolution failed.
type shadowRelationInput struct {
	Frozen, Live                 CandidateIdentity
	FrozenSnapshot, LiveSnapshot Snapshot
	GenesisPaths                 []string
	BaseAdvance                  *BaseAdvanceCompatibility // nil when not derivable
	AdmittedFindingPaths         []string
	AdmittedPathsKnown           bool // false => contraction degrades to changed
	ApplicableAuthorities        int  // >1 => ambiguous
	LiveUnresolvable             bool
}

// relateCandidates is the ordered, fail-closed relation function (design
// decision 5): ambiguity -> unknown -> exact -> compatible_base_advance
// (delegated) -> provable_contraction -> changed -> unrelated. It is pure:
// no Git call, no mutation, no I/O. Every input needed to derive an earlier
// relation in the order has already been resolved by the caller (identity
// resolution in candidate_identity.go, base-advance delegation via
// deriveBaseAdvanceCompatibility, prepr.go).
//
// This was shadowRelate before Wave 3 Slice 1's promotion (design decision
// 2): renamed, not wrapped, so the live ReviewCore calls one function, not
// two names for one algorithm.
func relateCandidates(input shadowRelationInput) CandidateRelation {
	if input.ApplicableAuthorities > 1 {
		return ShadowRelationAmbiguous
	}
	// Commit-state / push-state threat matrix: an unborn HEAD, an empty
	// live change set (nothing staged/changed), or an explicitly
	// unresolvable boundary all fail closed to unknown. This check runs
	// before the exact comparison on purpose — a live candidate with
	// nothing in it must never be reported as an "accidental exact" match
	// just because its base and candidate trees happen to coincide with
	// the frozen ones.
	if input.LiveUnresolvable || input.LiveSnapshot.UnbornHead || len(input.LiveSnapshot.Paths) == 0 {
		return ShadowRelationUnknown
	}
	if input.Frozen == input.Live {
		return ShadowRelationExact
	}
	if shadowBaseAdvanceApplies(input) {
		return ShadowRelationCompatibleBaseAdvance
	}
	// classifyCompactPathSetRelation (compact_target_relation.go) is the
	// live 5-value classifier's own path-set relation logic, reused as-is
	// so shadow and live agree byte-for-byte on what "contraction" means —
	// nothing about path-set classification is reimplemented here.
	if classifyCompactPathSetRelation(input.GenesisPaths, input.LiveSnapshot.Paths) == compactPathsContraction {
		// Amendment B: an admitted finding outside the live scope, or no
		// admitted-finding evidence at all (no-input degradation), both
		// degrade to changed — never unknown, never provable_contraction.
		if !input.AdmittedPathsKnown || shadowFindingReferencesExcludedPath(input.AdmittedFindingPaths, input.LiveSnapshot.Paths) {
			return ShadowRelationChanged
		}
		return ShadowRelationProvableContraction
	}
	if input.Frozen != (CandidateIdentity{}) {
		return ShadowRelationChanged
	}
	return ShadowRelationUnrelated
}

// shadowBaseAdvanceApplies reports whether the already-delegated
// BaseAdvanceCompatibility proof both validates on its own terms and binds
// to this exact Frozen/Live pair. Binding the proof to the identity tuple
// (rather than trusting proof.valid() alone) mirrors the live classifier's
// own pattern in classifyCompactTargetRelation (compact_target_relation.go)
// — a stale proof for a different base pair must never be accepted. The
// CandidateTree preservation check below was a discovered gap (Wave 1
// exit-bar finding, TestShadowMatrixUnexplainedDivergenceOnCoreRelationBlocksWave2):
// classifyCompactTargetRelation's own compatible-advance branch additionally
// requires frozen.CandidateTree == live.CandidateTree, so a proof valid on
// its own terms and matching the base pair but paired with a changed
// CandidateTree must fall through, never be accepted here.
func shadowBaseAdvanceApplies(input shadowRelationInput) bool {
	proof := input.BaseAdvance
	return proof != nil && proof.valid() &&
		proof.OriginalMergeBaseTree == input.Frozen.BaseTree &&
		(proof.OriginalMergeBaseTree == input.Live.BaseTree || proof.NewBaseTree == input.Live.BaseTree) &&
		input.Frozen.CandidateTree == input.Live.CandidateTree
}

// shadowFindingReferencesExcludedPath reports whether any admitted finding
// path falls outside the live candidate's delivered scope — Amendment B's
// literal degradation trigger.
func shadowFindingReferencesExcludedPath(findingPaths, livePaths []string) bool {
	live := make(map[string]struct{}, len(livePaths))
	for _, path := range livePaths {
		live[path] = struct{}{}
	}
	for _, path := range findingPaths {
		if _, ok := live[path]; !ok {
			return true
		}
	}
	return false
}

// shadowPushBoundaryUnresolvable reports whether a push-boundary resolution
// (prePushTargetForRequest / buildPushTarget, gate.go — reused here rather
// than re-derived) leaves nothing comparable: either the resolution itself
// failed, or it succeeded with PrePRBoundaryEmptyRemoteBootstrap ("first
// push" — no prior governing publication exists at this destination, so
// there is no live delivery-range history to compare the frozen candidate
// against). Both fail closed to unknown at the relation layer.
func shadowPushBoundaryUnresolvable(refs *resolvedPrePRRefs, err error) bool {
	if err != nil || refs == nil {
		return true
	}
	return refs.Selection.Source == PrePRBoundaryEmptyRemoteBootstrap
}

// shadowRelationHasNoLiveCounterpart reports whether relation has no
// fabricatable live decision to compare against — exactly `ambiguous` and
// `unknown` (Requirement: "ambiguous and unknown Have No Fabricated Live
// Counterpart", spec.md:80-89). A differential-matrix row (Slice 6) for
// either value MUST be labeled "no live decision" and MUST NOT be recorded
// as agreement with any live outcome.
func shadowRelationHasNoLiveCounterpart(relation ShadowRelation) bool {
	switch relation {
	case ShadowRelationAmbiguous, ShadowRelationUnknown:
		return true
	default:
		return false
	}
}
