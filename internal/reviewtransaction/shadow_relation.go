// Package reviewtransaction — shadow relation algebra (Wave 1, Slice 3). This
// file is part of the read-only shadow of the target RDD relation model
// (docs/architecture/rdd-root-simplification-design.md). It compares a
// frozen CandidateIdentity with a live one and returns exactly one of seven
// target-architecture relations, in a fixed fail-closed order (design
// decision 5). See shadow_readonly_guard_test.go for the AST guard that
// enforces this file never mutates authority state.
//
// ShadowRelation is the second symbol this slice exports (design decision
// 1, after Slice 2's CandidateIdentity); everything else here stays
// unexported until the observer (Slice 5) needs it.
package reviewtransaction

import (
	"context"
	"sync"
)

// ShadowRelation is the exact seven-value relation vocabulary
// (Requirement: Seven-Value Relation Output,
// openspec/changes/rdd-root-simplification-wave1/specs/rdd-candidate-relation-algebra/spec.md:9-11).
// No eighth value is ever produced.
type ShadowRelation string

const (
	ShadowRelationExact                 ShadowRelation = "exact"
	ShadowRelationCompatibleBaseAdvance ShadowRelation = "compatible_base_advance"
	ShadowRelationProvableContraction   ShadowRelation = "provable_contraction"
	ShadowRelationChanged               ShadowRelation = "changed"
	ShadowRelationUnrelated             ShadowRelation = "unrelated"
	ShadowRelationAmbiguous             ShadowRelation = "ambiguous"
	ShadowRelationUnknown               ShadowRelation = "unknown"
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
// outcome reaches the pure ordered function without shadowRelate itself
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

// shadowRelate is the ordered, fail-closed relation function (design
// decision 5): ambiguity -> unknown -> exact -> compatible_base_advance
// (delegated) -> provable_contraction -> changed -> unrelated. It is pure:
// no Git call, no mutation, no I/O. Every input needed to derive an earlier
// relation in the order has already been resolved by the caller (identity
// resolution in shadow_identity.go, base-advance delegation via
// shadowDeriveBaseAdvance below).
func shadowRelate(input shadowRelationInput) ShadowRelation {
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
		proof.NewBaseTree == input.Live.BaseTree &&
		input.Frozen.CandidateTree == input.Live.CandidateTree
}

// shadowDeriveBaseAdvance is Amendment A's delegation seam
// (spec.md:25-41): it calls deriveBaseAdvanceCompatibility (prepr.go:73)
// directly — constructing that function's unexported *resolvedPrePRRefs and
// gateArtifactPreimages argument types in-package — and never reimplements
// any of its seven conditions. A derivation error means "not compatible"
// and is reported as nil, so the caller falls through the ordered
// evaluation instead of the shadow package asserting its own opinion about
// why: no shadow-local override of the delegated result ever exists.
func shadowDeriveBaseAdvance(
	ctx context.Context,
	repo string,
	receipt Receipt,
	request GateRequest,
	snapshot Snapshot,
	refs *resolvedPrePRRefs,
	preimages gateArtifactPreimages,
) *BaseAdvanceCompatibility {
	shadowDeriveBaseAdvanceMu.Lock()
	shadowDeriveBaseAdvanceCallCount++
	shadowDeriveBaseAdvanceMu.Unlock()

	proof, err := deriveBaseAdvanceCompatibility(ctx, repo, receipt, request, snapshot, refs, preimages)
	if err != nil {
		return nil
	}
	return &proof
}

var (
	shadowDeriveBaseAdvanceMu        sync.Mutex
	shadowDeriveBaseAdvanceCallCount int
)

// shadowDeriveBaseAdvanceCallCountForTest and
// shadowResetDeriveBaseAdvanceCallCountForTest exist only for this package's
// own tests. They mirror shadow_observer.go's shadowObserverCallCountForTest
// idiom: the cheapest possible proof that the live gate path (gate.go) pays
// zero shadow-side merge-base/changedPaths/patchIdentity/merge-tree cost
// when GENTLE_AI_RDD_SHADOW is unset — "Off by Default in Live Paths"
// (spec.md).
func shadowDeriveBaseAdvanceCallCountForTest() int {
	shadowDeriveBaseAdvanceMu.Lock()
	defer shadowDeriveBaseAdvanceMu.Unlock()
	return shadowDeriveBaseAdvanceCallCount
}

func shadowResetDeriveBaseAdvanceCallCountForTest() {
	shadowDeriveBaseAdvanceMu.Lock()
	defer shadowDeriveBaseAdvanceMu.Unlock()
	shadowDeriveBaseAdvanceCallCount = 0
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
