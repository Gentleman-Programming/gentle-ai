package reviewtransaction

// Wave 5 (Gate Cutover) Slice 4, design decision 1: legacy v1 lineages
// traverse the same candidate-relation algebra new-lineage (v3) candidates
// use, through a read-only projection of the legacy ValidatedChain's
// terminal transaction and its immutable receipt.json into CandidateIdentity
// — never by translating or rewriting the legacy chain's own stored bytes
// (the wave-table rule; legacy deletion is Wave 7, not this slice).
//
// Amendment (S4, documented per the S2/S3 precedent): design.md's literal
// two-argument sketch is
//
//	func projectLegacyAuthority(chain ValidatedChain, artifacts facadeArtifacts) (CandidateIdentity, ReceiptRef, error)
//
// This cannot compile as written: facadeArtifacts is an unexported package
// cli type, and reviewtransaction cannot import cli (review_core.go's own
// documented constraint — the same constraint that shaped FreezeCandidateIdentity's
// signature). The shipped signature below instead takes ctx/root — needed by
// FreezeCandidateIdentity's live tree-to-tree diff, exactly as gateVerdict's
// own S3 GateContext amendment added exactly the parameter it needed to be
// correct — and the receipt.json path directly, reading and parsing the
// Receipt itself rather than accepting a cli-package artifact bundle it
// cannot reference. The function is exported (capitalized) because its only
// caller is package cli's funnel (review_facade.go), which needs to invoke
// it directly now that the legacy subprocess re-entry (runFacadeLegacyValidateNegotiated)
// is deleted from the funnel per this same decision.

import (
	"context"
	"errors"
	"os"
)

// ProjectLegacyAuthority projects a terminal (approved or escalated) legacy
// v1 chain's own immutable receipt into a CandidateIdentity, plus a
// ReceiptRef binding that projection to the exact receipt bytes it came
// from. It is read-only: it never mutates the chain, the receipt file, or
// any authority store. chain must be the ValidatedChain discoverFacadeReview
// already loaded (never re-derived here — this function trusts its caller's
// discovery, matching FreezeCandidateIdentity's own "caller already built
// the Snapshot" contract).
func ProjectLegacyAuthority(ctx context.Context, root string, chain ValidatedChain, receiptPath string) (CandidateIdentity, ReceiptRef, error) {
	if err := ctx.Err(); err != nil {
		return CandidateIdentity{}, ReceiptRef{}, err
	}
	if len(chain.Records) == 0 {
		return CandidateIdentity{}, ReceiptRef{}, errors.New("legacy authority projection requires a non-empty validated chain") // refusal:by-design world-action: an empty chain is a caller ordering bug — discoverFacadeReview never returns one — not an operator-fixable state
	}
	payload, err := os.ReadFile(receiptPath)
	if err != nil {
		return CandidateIdentity{}, ReceiptRef{}, err
	}
	receipt, err := ParseReceipt(payload)
	if err != nil {
		return CandidateIdentity{}, ReceiptRef{}, err
	}
	tx := chain.Records[len(chain.Records)-1].Transaction
	if tx.State != StateApproved && tx.State != StateEscalated {
		return CandidateIdentity{}, ReceiptRef{}, errors.New("legacy authority projection requires a terminal approved or escalated transaction") // refusal:by-design world-action: the caller (discoverFacadeReview with terminal=true) must only ever hand a terminal chain to projection; a non-terminal chain here is a caller ordering bug, not an operator-fixable state
	}
	if tx.LineageID != receipt.LineageID || tx.BaseTree != receipt.BaseTree || tx.FinalCandidateTree != receipt.FinalCandidateTree {
		return CandidateIdentity{}, ReceiptRef{}, errors.New("legacy receipt does not match its own authoritative chain's terminal transaction") // refusal:by-design world-action: a mismatch between the immutable receipt on disk and its own chain's terminal transaction is storage corruption or tampering; there is no operator command that reconciles it, only maintainer inspection
	}
	snapshot := tx.Snapshot
	snapshot.BaseTree = receipt.BaseTree
	snapshot.CandidateTree = receipt.FinalCandidateTree
	identity, err := FreezeCandidateIdentity(ctx, root, snapshot, receipt.PolicyHash)
	if err != nil {
		return CandidateIdentity{}, ReceiptRef{}, err
	}
	return identity, ReceiptRef{LineageID: receipt.LineageID, AuthorityRevision: sha256Ref(payload)}, nil
}

// EvaluateLegacyGate composes ProjectLegacyAuthority with the same
// relateCandidates + gateVerdict pipeline every relation-driven gate call
// uses (DeriveObservation, gate.go's gateVerdict) — the coordinator's "ALL
// five gates evaluate legacy through gateVerdict, one meaning, no re-derived
// gate semantics". live and evidence are the caller's already-resolved live
// candidate (package cli's governingAuthorityLiveEvidence, reused verbatim —
// legacy needs no live-evidence derivation new-lineage does not already
// have).
// livePolicyOverridden mirrors EvaluateNativeGate's own conditional policy
// re-verification (gate.go:290-293, hasArtifactSource(request.PolicyArtifact,
// request.PolicyContent)): legacy's gate evaluation trusts the receipt's own
// PolicyHash unconditionally UNLESS the caller explicitly supplies a live
// policy artifact to re-verify against. Ordinary gate calls (post-apply,
// pre-commit, pre-push -- and pre-pr/release without an explicit --policy)
// never do. Discovered via genuine RED against
// TestLegacyFunnelCharacterization_RunFacadeLegacyValidateNegotiated (S1's
// reference corpus): reusing governingAuthorityLiveEvidence's v3-shaped
// default-policy resolution unconditionally compared every legacy
// candidate's own arbitrary, receipt-bound policy content against the
// unrelated v3 built-in default, so every untouched legacy candidate
// spuriously read as "changed". When false, live.PolicyHash is overridden to
// the projected identity's own PolicyHash before relating -- policy content
// legacy never asked to be live-compared can never manufacture a false
// "changed" relation.
// EvaluateLegacyGate's gate parameter was a bare GateKind before CRITICAL-B
// (Wave 5 fix cycle 1, verify-report #10186): a bare GateKind carries no
// release-boundary artifact locations, so the pre-pr/release preconditions
// gateVerdict enforces (BaseRelationshipValid, Release) could never be
// populated -- a byte-identical legacy candidate denied FOREVER at those two
// gates. NativeGateRequestInput is the same caller-supplied artifact-location
// carrier EvaluateNativeGate/evaluateCompactGate already take (native_request.go);
// its Gate field replaces the old bare parameter one-for-one.
func EvaluateLegacyGate(ctx context.Context, root string, chain ValidatedChain, receiptPath string, live CandidateIdentity, livePolicyOverridden bool, evidence CoreValidateEvidence, gateInput NativeGateRequestInput) NativeGateEvaluation {
	gate := gateInput.Gate
	identity, receiptRef, err := ProjectLegacyAuthority(ctx, root, chain, receiptPath)
	if err != nil {
		return NativeGateEvaluation{
			Result: GateInvalidated, Reason: "legacy authority projection failed: " + err.Error(),
			Context: GateContext{Gate: gate}, Cause: err,
		}
	}
	if !livePolicyOverridden {
		live.PolicyHash = identity.PolicyHash
	}
	observation := DeriveObservation(NewLineageAuthority{CandidateIdentity: identity}, live, evidence)
	context := GateContext{
		Gate: gate, LineageID: receiptRef.LineageID, StoreRevision: receiptRef.AuthorityRevision,
		BaseTree: identity.BaseTree, CandidateTree: identity.CandidateTree, PolicyHash: identity.PolicyHash,
		// CRITICAL-B fix: derived faithfully from the legacy chain's real live
		// state, mirroring EvaluateNativeGate's own derivation
		// (gate.go:289 -- BaseRelationshipValid: snapshot.BaseTree == receipt.BaseTree)
		// instead of leaving the zero value (false) that denied every legacy
		// candidate, even an unchanged one, at pre-pr/release.
		BaseRelationshipValid: live.BaseTree == identity.BaseTree,
	}
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
	result, next := gateVerdict(gate, observation.Relation, context)
	if result != GateAllow {
		context.Denial = &GateDenial{Stage: "legacy-projection", Code: string(observation.Relation)}
	}
	return NativeGateEvaluation{
		Result: result, Reason: string(observation.Relation), Context: context,
		Relation: observation.Relation, Next: &next,
	}
}

// deriveGateReleaseEvidenceFromInput derives release evidence from the same
// caller-supplied artifact locations BuildNativeGateRequest already uses to
// build a ReleaseRequest for the native v1 path (native_request.go:94-107)
// -- reused verbatim rather than re-derived, and shared by both
// EvaluateLegacyGate (CRITICAL-B) and EvaluateNewLineageGate (CRITICAL-C, in
// new_lineage_gate.go): a release boundary is held to the identical
// PublicationStateSealed/EvidenceFreshnessCurrent contract regardless of
// which lineage kind it governs.
func deriveGateReleaseEvidenceFromInput(ctx context.Context, root string, gateInput NativeGateRequestInput) (ReleaseEvidence, error) {
	head, err := resolveCommit(ctx, root, "HEAD")
	if err != nil {
		return ReleaseEvidence{}, err
	}
	request := &ReleaseRequest{
		Revision: head, ConfigurationArtifact: gateInput.ReleaseConfiguration,
		GeneratedArtifact: gateInput.ReleaseGenerated, ProvenanceArtifact: gateInput.ReleaseProvenance,
		PublicationBoundaryArtifact: gateInput.ReleasePublicationBoundary,
		EvidenceFreshnessArtifact:   gateInput.ReleaseEvidenceFreshness,
		PublicationState:            PublicationStateSealed, EvidenceFreshnessState: EvidenceFreshnessCurrent,
	}
	preimages, err := readGateArtifactPreimages(GateRequest{Release: request})
	if err != nil {
		return ReleaseEvidence{}, err
	}
	return deriveReleaseEvidence(ctx, root, request, preimages)
}
