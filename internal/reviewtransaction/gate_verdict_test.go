package reviewtransaction

// Wave 5 (Gate Cutover), Slice 3: gateVerdict totality (task 4.1) and the
// absorbed N2 per-gate precondition contract (task 4.7, PR0's "Absorbed
// from Wave 3/4 Verification" section). Both are pure, unwired unit tests
// against gateVerdict directly -- the function's own totality is a property
// of the function, independent of which live call sites route through it
// yet (design decision 1: legacy-through-algebra projection is a Slice 4
// deliverable, so gateVerdict cannot be the sole source of truth for every
// outcome until then).

import "testing"

var gateVerdictAllGates = []GateKind{GatePostApply, GatePreCommit, GatePrePush, GatePrePR, GateRelease}

var gateVerdictAllRelations = []CandidateRelation{
	ShadowRelationExact, ShadowRelationCompatibleBaseAdvance, ShadowRelationProvableContraction,
	ShadowRelationChanged, ShadowRelationAmbiguous, ShadowRelationUnknown, ShadowRelationUnrelated,
}

// gateVerdictWantResultByRelation and gateVerdictWantReasonCodeByRelation are
// the EXACT expected (GateResult, ReasonCode) per relation under a healthy
// precondition context (CRITICAL-D, Wave 5 fix cycle 1, verify-report
// #10186): before this fix, TestGateVerdict_TotalFunction_35Cells asserted
// only that the result was one of the four known GateResult values and that
// a non-allow carried SOME next step -- flipping provable_contraction,
// unrelated, ambiguous, or unknown to GateAllow left this test (and every
// other suite) green. These two tables pin gateVerdict's relation switch
// (gate.go:1751-1774) by VALUE, not merely by shape, so a regression to
// fail-open on any of the seven relations fails this test by name.
var gateVerdictWantResultByRelation = map[CandidateRelation]GateResult{
	ShadowRelationExact:                 GateAllow,
	ShadowRelationCompatibleBaseAdvance: GateAllow,
	ShadowRelationProvableContraction:   GateInvalidated,
	ShadowRelationChanged:               GateInvalidated,
	ShadowRelationUnrelated:             GateInvalidated,
	ShadowRelationAmbiguous:             GateInvalidated,
	ShadowRelationUnknown:               GateInvalidated,
}

var gateVerdictWantReasonCodeByRelation = map[CandidateRelation]string{
	ShadowRelationExact:                 "allow",
	ShadowRelationCompatibleBaseAdvance: "allow",
	ShadowRelationProvableContraction:   "scope_contracted",
	ShadowRelationChanged:               "candidate_changed",
	ShadowRelationUnrelated:             "no_receipt",
	ShadowRelationAmbiguous:             "authority_ambiguous",
	ShadowRelationUnknown:               "target_unresolvable",
}

// TestGateVerdict_TotalFunction_35Cells is task 4.1: every one of the
// 5 gates x 7 relations = 35 pairings must resolve to the EXACT expected
// (GateResult, ReasonCode) pinned above -- not merely a defined shape -- and
// every denial (non-allow) must name either an executable Transition or a
// ReasonCode -- never a bare denial with neither.
func TestGateVerdict_TotalFunction_35Cells(t *testing.T) {
	// A "healthy" context: BaseRelationshipValid true and Release populated,
	// so the totality sweep exercises the relation table itself, not the
	// absorbed N2 preconditions (those get their own dedicated test below).
	healthy := GateContext{BaseRelationshipValid: true, Release: &ReleaseEvidence{
		ReleaseTree: "tree", ConfigurationHash: hash("1"), GeneratedArtifactHash: hash("2"),
		ProvenanceHash: hash("3"), PublicationBoundaryHash: hash("4"), PublicationState: PublicationStateSealed,
		EvidenceFreshnessHash: hash("5"), EvidenceFreshnessState: EvidenceFreshnessCurrent,
	}}
	cells := 0
	for _, gate := range gateVerdictAllGates {
		for _, relation := range gateVerdictAllRelations {
			cells++
			context := healthy
			context.Gate = gate
			result, next := gateVerdict(gate, relation, context)
			wantResult, ok := gateVerdictWantResultByRelation[relation]
			if !ok {
				t.Fatalf("relation %q has no pinned expected result; the 7-value CandidateRelation vocabulary and this table have drifted apart", relation)
			}
			if result != wantResult {
				t.Fatalf("gate %q relation %q = %q, want the pinned %q (CRITICAL-D: this is the exact regression a fail-open flip must be caught by)", gate, relation, result, wantResult)
			}
			wantReasonCode := gateVerdictWantReasonCodeByRelation[relation]
			if next.ReasonCode != wantReasonCode {
				t.Fatalf("gate %q relation %q reason_code = %q, want the pinned %q", gate, relation, next.ReasonCode, wantReasonCode)
			}
			if result != GateAllow && next.Transition == "" && next.ReasonCode == "" {
				t.Fatalf("gate %q relation %q denial carries neither a Transition nor a ReasonCode (bare denial)", gate, relation)
			}
		}
	}
	if cells != 35 {
		t.Fatalf("swept %d cells, want 35 (5 gates x 7 relations)", cells)
	}
}

// TestGateVerdict_PerGatePreconditions_MatchLegacyValidateDerivedGate is the
// absorbed N2 debt (task 4.7, PR0): gateVerdict must reproduce
// validateDerivedGate's (receipt.go:279-321) per-gate precondition shape --
// BaseRelationshipValid gated to pre-pr/release only, release evidence
// gated to release only -- instead of newLineageGateEvaluation's uniform
// continue->allow for every gate (review_governing_authority.go:240-261,
// the exact gap N2 identified).
func TestGateVerdict_PerGatePreconditions_MatchLegacyValidateDerivedGate(t *testing.T) {
	completeRelease := &ReleaseEvidence{
		ReleaseTree: "tree", ConfigurationHash: hash("1"), GeneratedArtifactHash: hash("2"),
		ProvenanceHash: hash("3"), PublicationBoundaryHash: hash("4"), PublicationState: PublicationStateSealed,
		EvidenceFreshnessHash: hash("5"), EvidenceFreshnessState: EvidenceFreshnessCurrent,
	}

	t.Run("BaseRelationshipValid=false denies at pre-pr and release, not the other three gates", func(t *testing.T) {
		for _, gate := range gateVerdictAllGates {
			context := GateContext{Gate: gate, BaseRelationshipValid: false, Release: completeRelease}
			result, next := gateVerdict(gate, ShadowRelationExact, context)
			wantDeny := gate == GatePrePR || gate == GateRelease
			if wantDeny && result == GateAllow {
				t.Fatalf("gate %q allowed an exact relation despite BaseRelationshipValid=false, want deny (validateDerivedGate parity)", gate)
			}
			if !wantDeny && result != GateAllow {
				t.Fatalf("gate %q denied an exact relation solely because BaseRelationshipValid=false, want allow (only pre-pr/release gate on it) -- next=%#v", gate, next)
			}
		}
	})

	t.Run("compatible_base_advance relation exempts the BaseRelationshipValid precondition at pre-PR only", func(t *testing.T) {
		// Mirrors validateDerivedGate's own !compatibleAdvance exemption
		// (receipt.go:289, receipt.go:304): compatibleAdvance there is
		// literally scoped `context.Gate == GatePrePR && ...` -- release is
		// NOT exempted. W-3 (Wave 5 fix cycle 1, verify-report #10186):
		// gateVerdict's own exemption previously fired for BOTH GatePrePR and
		// GateRelease (the exemption's condition tested only the relation,
		// never the gate), a latent fail-open that becomes real the moment
		// release is evaluated through this function -- exactly what
		// EvaluateLegacyGate's CRITICAL-B fix now does. A compatible base
		// advance still allows at pre-PR (where the relation is derived and
		// the exemption is scoped) and must DENY at release, matching
		// validateDerivedGate exactly.
		prePR := GateContext{Gate: GatePrePR, BaseRelationshipValid: false, Release: completeRelease}
		if result, _ := gateVerdict(GatePrePR, ShadowRelationCompatibleBaseAdvance, prePR); result != GateAllow {
			t.Fatalf("pre-pr denied a compatible_base_advance relation despite the exemption: %q", result)
		}
		release := GateContext{Gate: GateRelease, BaseRelationshipValid: false, Release: completeRelease}
		if result, _ := gateVerdict(GateRelease, ShadowRelationCompatibleBaseAdvance, release); result == GateAllow {
			t.Fatal("release allowed a compatible_base_advance relation with BaseRelationshipValid=false; validateDerivedGate scopes this exemption to pre-PR only (receipt.go:289)")
		}
	})

	t.Run("release evidence absent or mismatched denies only at release", func(t *testing.T) {
		for _, gate := range gateVerdictAllGates {
			context := GateContext{Gate: gate, BaseRelationshipValid: true, Release: nil}
			result, next := gateVerdict(gate, ShadowRelationExact, context)
			wantDeny := gate == GateRelease
			if wantDeny && result == GateAllow {
				t.Fatalf("release gate allowed an exact relation with no release evidence, want deny")
			}
			if !wantDeny && result != GateAllow {
				t.Fatalf("gate %q denied an exact relation solely because Release is nil, want allow (release evidence is release-only) -- next=%#v", gate, next)
			}
		}
	})
}
