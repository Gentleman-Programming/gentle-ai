package cli

import (
	"context"
	"errors"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// resolveGoverningAuthority is Amendment C's single shared branch (design
// decision 4): every one of the five gates traverses it through
// runReviewFacadeValidate, immediately before discoverCompactFacadeGateReview
// runs — so legacy discovery stays byte-identical whenever nothing v3
// governs this candidate, which is every call that supplies no explicit
// --lineage marker (the ordinary hook-invoked shape) or names a lineage id
// with no v3/ directory at all.
//
// governs=true means the caller MUST NOT fall through to legacy discovery
// for this candidate, even when a legacy receipt exists (Amendment C: "a
// new-lineage candidate is never authorized by legacy"). evaluation is only
// meaningful when governs is true and discoveryErr is nil.
//
// discoveryErr is non-nil for exactly two Amendment-C denial shapes, both of
// which MUST NEVER fall through to legacy authorization: the
// discovery-integrity corruption path (a v3/<lineage> marker exists but its
// record could not be read — reviewtransaction.NewLineageMarkerCorruptedError,
// task 5.2) and the matrix's own "new present but unrelated, legacy present"
// deny cell. Task 5.3's decision: both reuse existing
// ReviewReceiptDiscoveryKind constants rather than minting a new one —
// ReviewAuthorityCorrupted for the corruption path (identical semantics to
// every other "the inventory could not be trusted" denial this file already
// emits) and ReviewReceiptUnrelated for the deny cell (identical semantics
// to the legacy "a receipt exists but does not govern this candidate"
// denial). No silent default: every reachable *ReviewReceiptDiscoveryError
// this function returns has an explicit Kind assigned at its construction
// site below.
func resolveGoverningAuthority(ctx context.Context, root, lineage string, gateInput reviewtransaction.NativeGateRequestInput) (governs bool, evaluation reviewtransaction.NativeGateEvaluation, discoveryErr *ReviewReceiptDiscoveryError) {
	if strings.TrimSpace(lineage) == "" {
		// Discovery rule (design's corrected Amendment C clause): lineage
		// kind is established SOLELY by v3/ record presence. No explicit
		// lineage marker means no v3 authority is discoverable for this
		// candidate at all — "new absent" by construction — so the
		// ordinary hook-invoked gate call costs no extra Git subprocess
		// here, matching decision 5's zero-cost-by-default guarantee.
		return false, reviewtransaction.NativeGateEvaluation{}, nil
	}
	record, found, err := reviewtransaction.DiscoverNewLineage(ctx, root, lineage)
	if err != nil {
		var corrupted *reviewtransaction.NewLineageMarkerCorruptedError
		if errors.As(err, &corrupted) {
			return true, reviewtransaction.NativeGateEvaluation{}, &ReviewReceiptDiscoveryError{Kind: ReviewAuthorityCorrupted, Detail: corrupted.Error()}
		}
		return false, reviewtransaction.NativeGateEvaluation{}, nil
	}
	if !found {
		return false, reviewtransaction.NativeGateEvaluation{}, nil
	}
	live, evidence, err := governingAuthorityLiveEvidence(ctx, root, gateInput)
	if err != nil {
		// No live candidate could be resolved at all — legacy discovery
		// below reports its own target-resolution failure; nothing new
		// governs a candidate identity that could not even be built.
		return false, reviewtransaction.NativeGateEvaluation{}, nil
	}
	observation := reviewtransaction.DeriveObservation(record.Authority, live, evidence)
	legacyPresent := false
	if observation.Relation == reviewtransaction.ShadowRelationUnrelated {
		if _, _, _, legacyErr := discoverFacadeReview(ctx, root, lineage, true); legacyErr == nil {
			legacyPresent = true
		}
	}
	switch reviewtransaction.ResolveGoverningAuthority(true, observation.Relation, legacyPresent) {
	case reviewtransaction.GoverningAuthorityKindDeny:
		return true, reviewtransaction.NativeGateEvaluation{}, &ReviewReceiptDiscoveryError{
			Kind: ReviewReceiptUnrelated, Detail: "a new-lineage candidate is never authorized by a legacy receipt",
		}
	case reviewtransaction.GoverningAuthorityKindLegacy:
		return false, reviewtransaction.NativeGateEvaluation{}, nil
	default: // reviewtransaction.GoverningAuthorityKindNew
		// C5 remediation (verify-report CRITICAL, "default-deny at gates"):
		// the prior implementation special-cased only `escalated`, so every
		// other non-approved state (reviewing, correcting, validating) fell
		// through to ReviewCore.Next(validate) below, whose relation-based
		// switch returns CoreTransitionContinue -> GateAllow for an exact
		// relation regardless of whether the authority was ever approved —
		// silently authorizing delivery from an in-flight, unfinalized
		// lineage at every one of the five gates, including release. The
		// switch below is exhaustive over the five persisted states and is
		// the ONLY place this function may return early with an ALLOW-
		// eligible outcome: `approved`, and only after LoadReceipt proves a
		// structurally valid, exactly-matching receipt actually exists —
		// the persisted state field alone is never trusted. Every other arm
		// denies unconditionally, before any relation is even evaluated,
		// with an executable next step mirroring legacy's own
		// receipt-missing denial (reviewFacadeReceiptNotAvailableReason).
		switch record.Authority.State {
		case reviewtransaction.NewLineageStateApproved:
			authorityStore, storeErr := reviewtransaction.NewLineageAuthorityStore(ctx, root, lineage)
			if storeErr != nil {
				return true, reviewtransaction.NativeGateEvaluation{}, &ReviewReceiptDiscoveryError{Kind: ReviewAuthorityCorrupted, Detail: storeErr.Error()}
			}
			receipt, receiptErr := authorityStore.LoadReceipt()
			if receiptErr != nil || receipt.LineageID != record.Authority.LineageID ||
				receipt.AuthorityRevision != record.Revision || receipt.TerminalState != record.Authority.State {
				return true, reviewtransaction.NativeGateEvaluation{
					Result: reviewtransaction.GateInvalidated, Reason: reviewFacadeReceiptNotAvailableReason(record.Authority.LineageID),
					Context: reviewtransaction.GateContext{
						Gate: gateInput.Gate, LineageID: record.Authority.LineageID, StoreRevision: record.Revision,
						BaseTree: record.Authority.CandidateIdentity.BaseTree, CandidateTree: record.Authority.CandidateIdentity.CandidateTree,
						PolicyHash: record.Authority.CandidateIdentity.PolicyHash,
						Denial:     &reviewtransaction.GateDenial{Stage: "new-lineage-validate", Code: "approved_without_receipt"},
					},
				}, nil
			}
		case reviewtransaction.NewLineageStateEscalated:
			// Adjudication (b) pinning fix: `escalated` is a terminal
			// NON-approval (design decision 7's own "ambiguous/unknown/unrelated
			// -> escalate" mapping, and ReviewCore.finalize issues a receipt for
			// escalated exactly like approved — see review_core.go's finalize).
			context := reviewtransaction.GateContext{
				Gate: gateInput.Gate, LineageID: record.Authority.LineageID, StoreRevision: record.Revision,
				BaseTree: record.Authority.CandidateIdentity.BaseTree, CandidateTree: record.Authority.CandidateIdentity.CandidateTree,
				PolicyHash: record.Authority.CandidateIdentity.PolicyHash,
				Denial:     &reviewtransaction.GateDenial{Stage: "new-lineage-validate", Code: string(reviewtransaction.NewLineageStateEscalated)},
			}
			return true, reviewtransaction.NativeGateEvaluation{Result: reviewtransaction.GateEscalated, Reason: "authority already escalated: escalated is a terminal non-approval", Context: context}, nil
		default: // reviewing, correcting, validating — and any future non-approved value
			return true, reviewtransaction.NativeGateEvaluation{
				Result: reviewtransaction.GateInvalidated, Reason: reviewFacadeReceiptNotAvailableReason(record.Authority.LineageID),
				Context: reviewtransaction.GateContext{
					Gate: gateInput.Gate, LineageID: record.Authority.LineageID, StoreRevision: record.Revision,
					BaseTree: record.Authority.CandidateIdentity.BaseTree, CandidateTree: record.Authority.CandidateIdentity.CandidateTree,
					PolicyHash: record.Authority.CandidateIdentity.PolicyHash,
					Denial:     &reviewtransaction.GateDenial{Stage: "new-lineage-validate", Code: string(record.Authority.State)},
				},
			}, nil
		}
		transition, err := (reviewtransaction.ReviewCore{}).Next(ctx, record.Authority, reviewtransaction.CoreRequest{
			Kind: reviewtransaction.CoreRequestValidate, LiveCandidateIdentity: live, Evidence: evidence,
		})
		if err != nil {
			return true, reviewtransaction.NativeGateEvaluation{}, &ReviewReceiptDiscoveryError{Kind: ReviewAuthorityCorrupted, Detail: err.Error()}
		}
		return true, newLineageGateEvaluation(gateInput.Gate, record, transition), nil
	}
}

// governingAuthorityLiveEvidence resolves the live candidate identity and
// validate evidence ReviewCore.Next(validate) needs, reusing the same
// workspace-selector resolution ObserveShadowRelation already uses at every
// one of its five gate call sites (shadow_observer.go) rather than
// rebuilding a gate-specific target a second time.
//
// Task 6.7 widens S4's own documented scope cut one step: pre-commit reads
// the STAGED (index) projection, exactly matching
// buildCompactGateRequestWithPushBase's own `if input.Gate == GatePreCommit {
// projection = ProjectionStaged }` branch (compact_gate.go) — pre-commit
// commits the index, not the live working tree, so a dirty unstaged edit made
// after start must never be read as a scope change here.
//
// The remaining three gates (post-apply, pre-push, pre-pr, release) still
// resolve the full live workspace target, not each gate's own
// committed-range/push-boundary/pre-PR-boundary composition
// (buildPushTarget, buildPrePRTarget) — those selectors additionally depend
// on recovery-chain rebinding and squashed-delivery detection that has no
// v3 analogue yet (new-lineage authority carries no recovery chain concept
// in this wave). Widening them without that supporting machinery would
// silently misclassify a legitimately-delivered push/PR candidate rather
// than correctly defer, which is worse than the current workspace-only
// approximation. This narrower scope is documented here rather than
// silently left as S4's original blanket note.
func governingAuthorityLiveEvidence(ctx context.Context, repo string, gateInput reviewtransaction.NativeGateRequestInput) (reviewtransaction.CandidateIdentity, reviewtransaction.CoreValidateEvidence, error) {
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	intendedUntracked := gateInput.IntendedUntracked
	if intendedUntracked == nil {
		intendedUntracked = []string{}
	}
	projection := reviewtransaction.ProjectionWorkspace
	if gateInput.Gate == reviewtransaction.GatePreCommit {
		projection = reviewtransaction.ProjectionStaged
	}
	snapshot, err := builder.Build(ctx, reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: projection,
		IntendedUntracked: intendedUntracked,
	})
	if err != nil {
		return reviewtransaction.CandidateIdentity{}, reviewtransaction.CoreValidateEvidence{}, err
	}
	// Task 6.7 fix: an empty policyHash here defaulted to the sentinel
	// "unknown" inside FreezeCandidateIdentity, which never equals the real
	// default-policy digest runReviewFacadeStartNewLineage froze at start
	// (facadePayloadHash(facadePolicyBytes(""))). That made every ordinary
	// gate call — including a genuinely unchanged candidate — report
	// "changed" instead of "exact", because CandidateIdentity's equality
	// check (relateCandidates, candidate_relation.go) includes PolicyHash.
	// This had never been exercised before this slice: S4's own tests never
	// drove an explicit --lineage marker through to this function at all.
	// Resolving the identical policy source ordinary START uses — the
	// caller-supplied policy artifact when present, the built-in default
	// otherwise — is what makes an unchanged candidate compare equal again.
	policy, err := facadePolicyBytes(gateInput.PolicyArtifact)
	if err != nil {
		return reviewtransaction.CandidateIdentity{}, reviewtransaction.CoreValidateEvidence{}, err
	}
	live, err := reviewtransaction.FreezeCandidateIdentity(ctx, repo, snapshot, facadePayloadHash(policy))
	if err != nil {
		return reviewtransaction.CandidateIdentity{}, reviewtransaction.CoreValidateEvidence{}, err
	}
	return live, reviewtransaction.CoreValidateEvidence{LiveSnapshot: snapshot, ApplicableAuthorities: 1}, nil
}

// newLineageGateEvaluation translates a ReviewCore validate CoreTransition
// into gate JSON (design decision 7, task 6.2 — folds in and replaces S4's
// generic default branch). Every one of CoreTransitionKind's six values has
// its own explicit case below — the trailing default exists only as a
// fail-closed guard against a hypothetical future seventh value, never to
// silently absorb one of the six that already exist:
//
//   - continue (exact/compatible_base_advance/provable_contraction) allows —
//     the legacy analogue of a receipt that still governs exactly.
//   - collect (changed) reports scope-changed, not invalidated: this is the
//     new-lineage equivalent of legacy's receipt_scope_changed, which
//     resolveGoverningAuthorityLiveEvidence-driven relateCandidates reaches
//     the same way legacy's own scope-changed detection does. A bounded
//     correction is what a scope-changed candidate asks for, not a bare
//     denial.
//   - escalate (ambiguous/unknown/unrelated) escalates. This is a
//     deliberate STRENGTHENING over legacy: legacy's receipt_ambiguous and
//     receipt_unrelated both collapse into a generic invalidated denial,
//     while the new model's relation algebra can tell an operator the
//     specific reason an escalation-worthy relation was reached. Both are
//     still non-allow refusals; nothing here is silently permissive.
//   - approve/repair/stop are UNREACHABLE from ReviewCore's validate() (only
//     its finalize() ever returns approve, and repair/stop are not returned
//     by any Next() path this gate reaches) — they get their own explicit
//     denial cases rather than sharing a default, so a future change to
//     validate()'s own switch that starts returning one of them fails loud
//     here instead of silently reusing a catch-all.
func newLineageGateEvaluation(gate reviewtransaction.GateKind, record reviewtransaction.NewLineageRecord, transition reviewtransaction.CoreTransition) reviewtransaction.NativeGateEvaluation {
	context := reviewtransaction.GateContext{
		Gate: gate, LineageID: record.Authority.LineageID, StoreRevision: record.Revision,
		BaseTree: record.Authority.CandidateIdentity.BaseTree, CandidateTree: record.Authority.CandidateIdentity.CandidateTree,
		PolicyHash: record.Authority.CandidateIdentity.PolicyHash,
	}
	switch transition.Kind {
	case reviewtransaction.CoreTransitionContinue:
		return reviewtransaction.NativeGateEvaluation{Result: reviewtransaction.GateAllow, Reason: transition.ReasonCode, Context: context}
	case reviewtransaction.CoreTransitionCollect:
		context.Denial = &reviewtransaction.GateDenial{Stage: "new-lineage-validate", Code: transition.ReasonCode}
		return reviewtransaction.NativeGateEvaluation{Result: reviewtransaction.GateScopeChanged, Reason: transition.ReasonCode, Context: context}
	case reviewtransaction.CoreTransitionEscalate:
		context.Denial = &reviewtransaction.GateDenial{Stage: "new-lineage-validate", Code: transition.ReasonCode}
		return reviewtransaction.NativeGateEvaluation{Result: reviewtransaction.GateEscalated, Reason: transition.ReasonCode, Context: context}
	case reviewtransaction.CoreTransitionApprove, reviewtransaction.CoreTransitionRepair, reviewtransaction.CoreTransitionStop:
		context.Denial = &reviewtransaction.GateDenial{Stage: "new-lineage-validate", Code: string(transition.Kind)}
		return reviewtransaction.NativeGateEvaluation{Result: reviewtransaction.GateInvalidated, Reason: transition.ReasonCode, Context: context}
	default:
		context.Denial = &reviewtransaction.GateDenial{Stage: "new-lineage-validate", Code: string(transition.Kind)}
		return reviewtransaction.NativeGateEvaluation{Result: reviewtransaction.GateInvalidated, Reason: transition.ReasonCode, Context: context}
	}
}
