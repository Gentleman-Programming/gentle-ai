package reviewtransaction

// Wave 5 fix cycle 1, CRITICAL-C closure (verify-report #10186, absorbed N2):
// EvaluateNewLineageGate is the real gateVerdict-consulting replacement for
// what internal/cli/review_governing_authority.go's newLineageGateEvaluation
// used to do on its own -- map CoreTransitionContinue straight to GateAllow,
// uniformly, for every gate, discarding gateVerdict's per-gate preconditions
// entirely (the exact gap task 4.7's "ABSORBED N2" claimed was closed but
// never wired). This mirrors EvaluateLegacyGate's own shape exactly: build a
// GateContext, populate BaseRelationshipValid/Release for pre-pr/release,
// then let gateVerdict decide -- "ALL five gates evaluate through
// gateVerdict, one meaning, no re-derived gate semantics" now genuinely
// covers all three lineage kinds (legacy, compact/v2, new-lineage/v3), not
// two of three.
//
// gateVerdict's preconditions apply ONLY inside the Continue branch
// (exact/compatible_base_advance/provable_contraction): Collect (changed)
// and Escalate (ambiguous/unknown/unrelated) are already non-allow outcomes
// unaffected by base-relationship or release evidence, and Approve/Repair/
// Stop are unreachable from ReviewCore.validate() and stay explicit denials
// -- deriving release evidence for those paths would be unnecessary work for
// an outcome gateVerdict cannot change.

import "context"

// EvaluateNewLineageGate translates a ReviewCore validate CoreTransition into
// gate JSON. live is the caller's already-resolved live CandidateIdentity
// (package cli's governingAuthorityLiveEvidence, reused verbatim -- the same
// value ReviewCore.Next(validate) itself consulted to produce transition, so
// this function re-derives no relation, only the gate preconditions
// gateVerdict needs that the relation alone does not carry).
func EvaluateNewLineageGate(ctx context.Context, root string, record NewLineageRecord, transition CoreTransition, live CandidateIdentity, gateInput NativeGateRequestInput) NativeGateEvaluation {
	gate := gateInput.Gate
	context := GateContext{
		Gate: gate, LineageID: record.Authority.LineageID, StoreRevision: record.Revision,
		BaseTree: record.Authority.CandidateIdentity.BaseTree, CandidateTree: record.Authority.CandidateIdentity.CandidateTree,
		PolicyHash: record.Authority.CandidateIdentity.PolicyHash,
	}
	switch transition.Kind {
	case CoreTransitionContinue:
		// Mirrors EvaluateLegacyGate's own derivation (legacy_projection.go):
		// live.BaseTree == the frozen authority's own BaseTree, the same
		// comparison EvaluateNativeGate makes at gate.go:289.
		context.BaseRelationshipValid = live.BaseTree == record.Authority.CandidateIdentity.BaseTree
		if gate == GateRelease {
			release, releaseErr := deriveGateReleaseEvidenceFromInput(ctx, root, gateInput)
			if releaseErr != nil {
				return NativeGateEvaluation{
					Result: GateInvalidated, Reason: "release boundary cannot be derived: " + releaseErr.Error(),
					Context: context, Cause: releaseErr,
				}
			}
			context.Release = &release
		}
		relation := CandidateRelation(transition.ReasonCode)
		result, next := gateVerdict(gate, relation, context)
		if result != GateAllow {
			context.Denial = &GateDenial{Stage: "new-lineage-validate", Code: transition.ReasonCode}
		}
		return NativeGateEvaluation{Result: result, Reason: transition.ReasonCode, Context: context, Relation: relation, Next: &next}
	case CoreTransitionCollect:
		context.Denial = &GateDenial{Stage: "new-lineage-validate", Code: transition.ReasonCode}
		return NativeGateEvaluation{Result: GateScopeChanged, Reason: transition.ReasonCode, Context: context}
	case CoreTransitionEscalate:
		context.Denial = &GateDenial{Stage: "new-lineage-validate", Code: transition.ReasonCode}
		return NativeGateEvaluation{Result: GateEscalated, Reason: transition.ReasonCode, Context: context}
	case CoreTransitionApprove, CoreTransitionRepair, CoreTransitionStop:
		context.Denial = &GateDenial{Stage: "new-lineage-validate", Code: string(transition.Kind)}
		return NativeGateEvaluation{Result: GateInvalidated, Reason: transition.ReasonCode, Context: context}
	default:
		context.Denial = &GateDenial{Stage: "new-lineage-validate", Code: string(transition.Kind)}
		return NativeGateEvaluation{Result: GateInvalidated, Reason: transition.ReasonCode, Context: context}
	}
}
