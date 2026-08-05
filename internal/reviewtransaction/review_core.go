// Package reviewtransaction — ReviewCore (Wave 3 Slice 3, design decision
// 1). ReviewCore is the sole transition owner for new-lineage reviews:
// start, finalize, and validate all go through one entry point, Next. It
// never re-derives review authority — tier comes from ClassifyRisk (via the
// caller's already-computed RiskAssessment), lenses from SelectReviewLenses,
// consent from the reused authorizeReviewStart/v2 consent envelope (both
// live in package cli and are called by the caller before Next, never by
// this package — reviewtransaction cannot import cli), and correction
// budget from CorrectionBudget. AuthorityStore (authority_store.go, Wave 3
// Slice 2) is the only thing this package mutates through; ReviewCore
// decides WHAT the next NewLineageAuthority should be, AuthorityStore.Mutate
// decides WHETHER the write is accepted (CAS).
package reviewtransaction

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

// newLineageActivationEnvVar is the start-only activation switch (design
// decision 5): unset or empty means OFF (legacy `review start` stays
// byte-identical); any other value means ON. Read fresh on every start —
// no process restart is needed to toggle it between runs. It is a distinct
// switch from the user-owned RDD kill switch (delivery-gate scope); neither
// substitutes for the other. (The read-only shadow observer's own
// independent switch, GENTLE_AI_RDD_SHADOW, retired in Wave 7 S2a.)
//
// Wave 7 S7 (WU18) attempted removal and reverted it (design decision,
// coordinator amendment): v3's negotiated START never gained
// repository_context support (see runReviewFacadeStartNewLineage's own doc
// comment and openspec/changes/rdd-root-simplification-wave7/specs/
// rdd-single-lifecycle for the full rationale) — removing the switch would
// make every negotiated START take the gapped v3 path unconditionally,
// turning a narrow, rarely-reached gap into a universal one. Non-negotiable
// #3 (never remove a switch over a known capability gap) blocks removal
// until that gap closes. Kept here deliberately: v3 stays opt-in, which is
// also the safer posture for the upcoming release candidate's community
// testing. WU18a (S7a) kept the genuinely additive work this attempt
// produced -- the v1/v2 legacy-collision start guards and v3's new frozen-
// candidate-context negotiated support -- both live and switch-independent.
const newLineageActivationEnvVar = "GENTLE_AI_RDD_NEW_LINEAGE"

// NewLineageActivationEnabled reports the start-only activation switch. It
// gates only where a NEW lineage is created (review_facade.go's start
// branch, Wave 3 Slice 3); finalize, validate, and every gate route on the
// v3/ record's own discovered presence, never on this switch — gating
// finalize would strand an in-flight lineage on rollback (design decision
// 5).
func NewLineageActivationEnabled() bool {
	return strings.TrimSpace(os.Getenv(newLineageActivationEnvVar)) != ""
}

// ErrConsentRequiredBeforeFreeze is refused when a tier 1|4 start requests a
// freeze without ConsentGranted (spec rdd-review-core-transitions,
// "Consent-Gated Freeze With Immutable Tier, Lenses, and Budget"). Tier 0's
// carve-out never reaches this check — see start's own tier switch.
var ErrConsentRequiredBeforeFreeze = errors.New("new-lineage authority freeze requires the granted consent envelope for this tier") // refusal:by-design world-action: the caller must obtain consent through the reused authorizeReviewStart/v2 envelope (package cli) before calling start again; there is no operator command that grants consent on the caller's behalf

// ErrFinalizeRequiresTerminalState is refused when finalize is requested for
// an authority that has not reached approved or escalated (spec
// rdd-review-core-transitions, "Terminal Receipt Issuance Exactly Once").
var ErrFinalizeRequiresTerminalState = errors.New("new-lineage finalize requires an approved or escalated authority") // refusal:by-design world-action: reaching a terminal state is validate's job; finalize is never the place to force one, so the fix is calling validate/collect first, not an operator command

// ErrFinalizeRequiresLensResults is refused when finalize is about to
// approve (not escalate) a non-terminal authority whose frozen
// SelectedLenses is non-empty but request.AdvanceRequest.CapturedLensResults
// does not cover every selected lens (absorbed N1, W3/W4 verify). A tier
// with an empty SelectedLenses (tier-low) legitimately needs no captured
// results at all and never triggers this refusal.
var ErrFinalizeRequiresLensResults = errors.New("new-lineage finalize requires captured results for every frozen selected lens before approving") // refusal:by-design world-action: the caller must actually capture a result for each selected lens (the v3 reviewer-result ingestion pipeline, not yet wired) before calling finalize without --failed; there is no operator command that approves on the caller's behalf

// ReviewCore is the sole transition owner for new-lineage reviews (spec
// rdd-review-core-transitions, "Sole Transition Owner for New Lineages"). It
// holds no state of its own — every decision is a pure function of the
// authority and request Next receives.
type ReviewCore struct{}

// Next is ReviewCore's one entry point (design decision 1). request.Kind
// selects which of start/finalize/validate runs; authority is the current
// persisted NewLineageAuthority (its zero value for a start request, since
// no lineage exists yet).
func (core ReviewCore) Next(ctx context.Context, authority NewLineageAuthority, request CoreRequest) (CoreTransition, error) {
	if err := ctx.Err(); err != nil {
		return CoreTransition{}, err
	}
	switch request.Kind {
	case CoreRequestStart:
		return core.start(request)
	case CoreRequestFinalize:
		return core.finalize(authority, request)
	case CoreRequestValidate:
		return core.validate(authority, request)
	default:
		return CoreTransition{}, fmt.Errorf("unsupported review core request kind %q", request.Kind) // refusal:by-design world-action: CoreRequestKind is a closed three-value vocabulary constructed only by this package's own callers; an out-of-vocabulary value is a caller bug, not an operator-fixable state
	}
}

// start freezes a new NewLineageAuthority (spec rdd-review-core-transitions,
// "Consent-Gated Freeze With Immutable Tier, Lenses, and Budget"): tier,
// lenses, and correction budget are frozen together, once, and only after
// consent is granted for tier 1|4 — tier 0 proceeds without a consent
// question. Tier, lenses, and OriginalChangedLines are the caller's
// already-computed values (ClassifyRisk/SelectReviewLenses via the caller,
// design decision 6); start only enforces the ordering invariant and
// computes the budget via the reused CorrectionBudget. LineageID is
// deliberately left unset: start does not know the lineage name (the
// facade/store does), so the caller sets it and calls RecordTransition, if
// it wants one recorded, inside the Mutate apply closure.
func (core ReviewCore) start(request CoreRequest) (CoreTransition, error) {
	switch request.Tier {
	case RiskLow, RiskMedium, RiskHigh:
	default:
		return CoreTransition{}, fmt.Errorf("unsupported review core tier %q", request.Tier) // refusal:by-design world-action: Tier is frozen from ClassifyRisk's own closed vocabulary by the caller; an out-of-vocabulary value is a caller bug, not an operator-fixable state
	}
	if request.Tier != RiskLow && !request.ConsentGranted {
		return CoreTransition{}, fmt.Errorf("%w: tier %q", ErrConsentRequiredBeforeFreeze, request.Tier)
	}
	if request.CandidateIdentity == (CandidateIdentity{}) {
		return CoreTransition{}, errors.New("review core start requires a frozen candidate identity") // refusal:by-design world-action: the caller must resolve CandidateIdentity (FreezeCandidateIdentity or the promoted resolver) before calling start; a zero value is a caller ordering bug, not an operator-fixable state
	}
	budget, err := CorrectionBudget(request.OriginalChangedLines)
	if err != nil {
		return CoreTransition{}, err
	}
	authority := NewLineageAuthority{
		State: NewLineageStateReviewing, CandidateIdentity: request.CandidateIdentity, Tier: request.Tier,
		SelectedLenses: append([]string(nil), request.SelectedLenses...), CorrectionBudgetLines: budget,
	}
	return CoreTransition{Kind: CoreTransitionContinue, Authority: &authority}, nil
}

// finalize issues the terminal receipt reference for an authority that has
// already reached approved or escalated (spec rdd-review-core-transitions,
// "Terminal Receipt Issuance Exactly Once"). The "exactly once" guarantee
// itself is enforced by AuthorityStore.WriteReceipt's immutable publication
// (Wave 3 Slice 2), reused rather than re-derived here — finalize only binds
// the ReceiptRef to the exact authority revision reaching that state.
//
// C6 remediation: a non-terminal authority (reviewing, correcting,
// validating) with an explicit request.AdvanceRequest is finalize's OWN
// decision to advance, not a refusal — this is what makes ReviewCore the
// sole transition owner for the whole reviewing/correcting -> approved|
// escalated advance (design decision 1), rather than a caller mutating
// NewLineageState directly before ever calling finalize. A nil
// AdvanceRequest (every pre-existing caller, and this method's own original
// contract) still refuses with ErrFinalizeRequiresTerminalState — the
// pinned TestReviewCoreFinalizeRequiresTerminalState invariant is preserved
// exactly, since a zero-value CoreRequest leaves AdvanceRequest nil.
func (core ReviewCore) finalize(authority NewLineageAuthority, request CoreRequest) (CoreTransition, error) {
	switch authority.State {
	case NewLineageStateApproved, NewLineageStateEscalated:
		revision, err := NewLineageRevisionForState(authority)
		if err != nil {
			return CoreTransition{}, err
		}
		kind := CoreTransitionApprove
		if authority.State == NewLineageStateEscalated {
			kind = CoreTransitionEscalate
		}
		return CoreTransition{
			Kind: kind, ReasonCode: string(authority.State),
			Receipt: &ReceiptRef{LineageID: authority.LineageID, AuthorityRevision: revision},
		}, nil
	case NewLineageStateReviewing, NewLineageStateCorrecting, NewLineageStateValidating:
		if request.AdvanceRequest == nil {
			return CoreTransition{}, fmt.Errorf("%w: got %q", ErrFinalizeRequiresTerminalState, authority.State)
		}
		escalating := request.AdvanceRequest.Failed || len(request.AdvanceRequest.AdmittedFindingIDs) > 0
		if !escalating && !hasCapturedAllSelectedLenses(authority.SelectedLenses, request.AdvanceRequest.CapturedLensResults) {
			return CoreTransition{}, fmt.Errorf("%w: lineage %q", ErrFinalizeRequiresLensResults, authority.LineageID)
		}
		next := authority
		next.State = NewLineageStateApproved
		if escalating {
			next.State = NewLineageStateEscalated
		}
		if len(request.AdvanceRequest.AdmittedFindingIDs) > 0 {
			next.AdmittedFindingIDs = append(append([]string{}, next.AdmittedFindingIDs...), request.AdvanceRequest.AdmittedFindingIDs...)
		}
		revision, err := NewLineageRevisionForState(next)
		if err != nil {
			return CoreTransition{}, err
		}
		kind := CoreTransitionApprove
		if next.State == NewLineageStateEscalated {
			kind = CoreTransitionEscalate
		}
		return CoreTransition{
			Kind: kind, ReasonCode: string(next.State), Authority: &next,
			Receipt: &ReceiptRef{LineageID: next.LineageID, AuthorityRevision: revision},
		}, nil
	default:
		return CoreTransition{}, fmt.Errorf("%w: got %q", ErrFinalizeRequiresTerminalState, authority.State)
	}
}

// hasCapturedAllSelectedLenses reports whether captured covers every lens in
// selected (absorbed N1). An empty selected always reports true (tier-low
// legitimately needs no lens results); a non-empty selected requires every
// entry to be present in captured, regardless of order or duplicates.
func hasCapturedAllSelectedLenses(selected, captured []string) bool {
	if len(selected) == 0 {
		return true
	}
	have := make(map[string]bool, len(captured))
	for _, lens := range captured {
		have[lens] = true
	}
	for _, lens := range selected {
		if !have[lens] {
			return false
		}
	}
	return true
}

// validate compares the frozen authority against a live candidate (spec
// rdd-review-core-transitions; design's validate data flow:
// "DiscoverNewLineage(v3) -> resolveGoverningAuthority -> ReviewCore.Validate
// (relateCandidates)"). It reuses relateCandidates verbatim through
// DeriveObservation — no re-derivation of the seven-value relation algebra.
// exact/compatible_base_advance/provable_contraction continue unchanged;
// changed asks for the one bounded correction (already correcting is
// refused, matching AuthorityStore.ResolveReplay's own
// ErrOneCorrectionOnly boundary); ambiguous/unknown/unrelated escalate —
// none of the three has a fabricated live counterpart to continue against.
func (core ReviewCore) validate(authority NewLineageAuthority, request CoreRequest) (CoreTransition, error) {
	if authority.LineageID == "" {
		return CoreTransition{}, errors.New("review core validate requires an existing new-lineage authority") // refusal:by-design world-action: validate is only ever called with an authority already loaded from AuthorityStore; an empty lineage id means the caller skipped that load, a caller ordering bug, not an operator-fixable state
	}
	observation := DeriveObservation(authority, request.LiveCandidateIdentity, request.Evidence)
	switch observation.Relation {
	case ShadowRelationExact, ShadowRelationCompatibleBaseAdvance, ShadowRelationProvableContraction:
		return CoreTransition{Kind: CoreTransitionContinue, ReasonCode: string(observation.Relation)}, nil
	case ShadowRelationChanged:
		if authority.State == NewLineageStateCorrecting {
			return CoreTransition{}, fmt.Errorf("%w: candidate changed again while a correction is already in flight", ErrOneCorrectionOnly)
		}
		return CoreTransition{
			Kind: CoreTransitionCollect, ReasonCode: string(observation.Relation),
			Collect: []CoreInput{{Name: "correction", Reason: "candidate changed since the frozen authority"}},
		}, nil
	case ShadowRelationAmbiguous, ShadowRelationUnknown, ShadowRelationUnrelated:
		return CoreTransition{Kind: CoreTransitionEscalate, ReasonCode: string(observation.Relation)}, nil
	default:
		return CoreTransition{}, fmt.Errorf("unsupported candidate relation %q", observation.Relation) // refusal:by-design world-action: CandidateRelation is relateCandidates' own closed seven-value vocabulary; an out-of-vocabulary value can only follow a change to that function, a bug in this package, not an operator-fixable state
	}
}

// FreezeCandidateIdentity resolves ReviewCore's frozen CandidateIdentity
// from an already-built Snapshot, reusing the exact primitives
// shadowResolveTarget already composes for the shadow/legacy call sites
// (OpenRepositoryIdentityLease, shadowChangedPathsModesDigest) rather than
// rebuilding the snapshot a second time through a shadowSelector. Callers
// that already have a Snapshot (every review_facade.go start path does) use
// this instead of shadowCandidateIdentity, which would resolve its own
// Snapshot from a selector redundantly.
func FreezeCandidateIdentity(ctx context.Context, repo string, snapshot Snapshot, policyHash string) (CandidateIdentity, error) {
	lease, err := OpenRepositoryIdentityLease(ctx, repo)
	if err != nil {
		return CandidateIdentity{}, err
	}
	modesDigest, err := shadowChangedPathsModesDigest(ctx, repo, snapshot.Paths, snapshot.BaseTree, snapshot.CandidateTree)
	if err != nil {
		return CandidateIdentity{}, err
	}
	hash := strings.TrimSpace(policyHash)
	if hash == "" {
		hash = "unknown"
	}
	return CandidateIdentity{
		RepositoryID: lease.Identity().RepositoryRef,
		BaseTree:     snapshot.BaseTree, CandidateTree: snapshot.CandidateTree,
		ChangedPathsModesDigest: modesDigest, PolicyHash: hash,
	}, nil
}
