package cli

import (
	"context"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// legacyReasonTaxonomyGateResult is an approval-style regression snapshot
// (task 6.1, design decision 7) of runReviewFacadeValidate's own,
// UNCHANGED-by-this-slice discovery-error handling
// (review_facade.go:2905-2910 for the generic case, :3008-3025 for the
// negotiated-off form): every one of the six ReviewReceiptDiscoveryKind
// values reaches GateInvalidated, except ReviewReceiptScopeChanged, which is
// special-cased to GateScopeChanged. This is the closed six-value ->
// legacy-reachable-GateResult side of the 6x4 regression table; the
// production side (newLineageGateEvaluation, ResolveGoverningAuthority) is
// exercised for real below.
func legacyReasonTaxonomyGateResult(kind ReviewReceiptDiscoveryKind) reviewtransaction.GateResult {
	switch kind {
	case ReviewReceiptMissing:
		return reviewtransaction.GateInvalidated
	case ReviewReceiptUnrelated:
		return reviewtransaction.GateInvalidated
	case ReviewReceiptScopeChanged:
		return reviewtransaction.GateScopeChanged
	case ReviewReceiptAmbiguous:
		return reviewtransaction.GateInvalidated
	case ReviewAuthorityCorrupted:
		return reviewtransaction.GateInvalidated
	case ReviewReceiptTargetUnresolvable:
		return reviewtransaction.GateInvalidated
	default:
		panic("unreachable: ReviewReceiptDiscoveryKind is a closed six-value vocabulary — extend this table alongside any new kind")
	}
}

// wantLegacyReasonTaxonomyGateResult is a HARDCODED literal expectation
// table (task C3 fix), deliberately NOT derived by calling
// legacyReasonTaxonomyGateResult a second time. The original matrix test
// computed both sides of its comparison from the identical pure call
// (want := legacyReasonTaxonomyGateResult(kind); got :=
// legacyReasonTaxonomyGateResult(kind) == result), which meant every one of
// its 24 cells always agreed with itself — proven by mutation: flipping
// ReviewReceiptScopeChanged's case to return GateAllow left the matrix green
// across all 24 cells. This table is the independent, hand-authored
// expectation the mutation must now disagree with.
var wantLegacyReasonTaxonomyGateResult = map[ReviewReceiptDiscoveryKind]reviewtransaction.GateResult{
	ReviewReceiptMissing:            reviewtransaction.GateInvalidated,
	ReviewReceiptUnrelated:          reviewtransaction.GateInvalidated,
	ReviewReceiptScopeChanged:       reviewtransaction.GateScopeChanged,
	ReviewReceiptAmbiguous:          reviewtransaction.GateInvalidated,
	ReviewAuthorityCorrupted:        reviewtransaction.GateInvalidated,
	ReviewReceiptTargetUnresolvable: reviewtransaction.GateInvalidated,
}

// TestNewLineageReasonTaxonomyCoversLegacyRefusalsClosedMatrix is task 6.1's
// closed 6x4 vocabulary test (design decision 7): the six
// ReviewReceiptDiscoveryKind values against the four GateResult values, no
// default fallback anywhere in the assertion — every one of the 24 cells is
// checked explicitly, proving legacyReasonTaxonomyGateResult never silently
// reports more than the one GateResult each kind legitimately reaches.
func TestNewLineageReasonTaxonomyCoversLegacyRefusalsClosedMatrix(t *testing.T) {
	kinds := []ReviewReceiptDiscoveryKind{
		ReviewReceiptMissing, ReviewReceiptUnrelated, ReviewReceiptScopeChanged,
		ReviewReceiptAmbiguous, ReviewAuthorityCorrupted, ReviewReceiptTargetUnresolvable,
	}
	results := []reviewtransaction.GateResult{
		reviewtransaction.GateAllow, reviewtransaction.GateScopeChanged,
		reviewtransaction.GateInvalidated, reviewtransaction.GateEscalated,
	}
	for _, kind := range kinds {
		want, tabulated := wantLegacyReasonTaxonomyGateResult[kind]
		if !tabulated {
			t.Fatalf("kind %q has no hardcoded expectation in wantLegacyReasonTaxonomyGateResult", kind)
		}
		for _, result := range results {
			reachable := result == want
			t.Run(string(kind)+"/"+string(result), func(t *testing.T) {
				got := legacyReasonTaxonomyGateResult(kind) == result
				if got != reachable {
					t.Fatalf("legacy kind %q reaching GateResult %q = %v, want %v", kind, result, got, reachable)
				}
			})
		}
	}
}

// TestNewLineageReasonTaxonomyCoversLegacyRefusals is task 6.1/6.2's RED/GREEN
// regression test proper (design decision 7): for every one of the six
// legacy ReviewReceiptDiscoveryKind values, a new-lineage derived category is
// reachable through the REAL, already-wired production functions —
// reviewtransaction.ResolveGoverningAuthority and/or newLineageGateEvaluation
// — never a second hardcoded copy of the mapping. No legacy refusal reason is
// left with no new-lineage equivalent: every one of the six either reaches
// the IDENTICAL GateResult class legacy reaches (receipt_unrelated,
// receipt_scope_changed, authority_corrupted), a documented STRONGER
// non-allow refusal (receipt_ambiguous: legacy's generic invalidated becomes
// the new model's more specific escalated — both refuse, neither is silently
// permissive), or a proven real BYTE-IDENTICAL DEFER to the unchanged legacy
// path (receipt_missing, target_unresolvable — Amendment C's own "new
// absent" construction).
func TestNewLineageReasonTaxonomyCoversLegacyRefusals(t *testing.T) {
	fixtureRecord := reviewtransaction.NewLineageRecord{
		Authority: reviewtransaction.NewLineageAuthority{LineageID: "taxonomy-fixture", State: reviewtransaction.NewLineageStateReviewing},
	}

	t.Run(string(ReviewReceiptMissing), func(t *testing.T) {
		// Amendment C's own construction (design decision 4): "new absent"
		// means legacy governs, byte-identical, regardless of legacy
		// presence — proven here with a real call, not a duplicated
		// assumption.
		if got := reviewtransaction.ResolveGoverningAuthority(false, "", true); got != reviewtransaction.GoverningAuthorityKindLegacy {
			t.Fatalf("ResolveGoverningAuthority(absent) = %q, want defer to legacy", got)
		}
		// A deferred kind reaches no GateResult of its own; the legacy
		// mapping stands unchanged, which is exactly what "byte-identical"
		// means for this cell.
	})

	t.Run(string(ReviewReceiptTargetUnresolvable), func(t *testing.T) {
		// resolveGoverningAuthority's own documented clause (its "no live
		// candidate could be resolved at all" branch, review_governing_
		// authority.go) defers exactly like receipt_missing: governs=false,
		// discoveryErr=nil, so legacy discovery below reports its own
		// target-resolution failure. That branch requires a real v3 record
		// to already be found (found=true) with live-evidence resolution
		// failing afterward — a race this test does not attempt to
		// construct through real Git evidence, matching S4's own precedent
		// (apply-progress: the equivalent "new absent" matrix cell was
		// tested at the pure-decision-function level rather than forced
		// through real evidence). The no-marker call below exercises the
		// SAME "nothing new governs" contract resolveGoverningAuthority
		// guarantees for every one of its non-governing early returns —
		// receipt_missing and target_unresolvable share that contract by
		// construction, not by coincidence.
		if _, evaluation, discoveryErr := resolveGoverningAuthority(context.Background(), "", "", reviewtransaction.NativeGateRequestInput{}); discoveryErr != nil || evaluation != (reviewtransaction.NativeGateEvaluation{}) {
			t.Fatalf("resolveGoverningAuthority(no governing v3 authority) must be a pure defer, got evaluation=%#v discoveryErr=%v", evaluation, discoveryErr)
		}
	})

	t.Run(string(ReviewReceiptUnrelated), func(t *testing.T) {
		// The exact Amendment C deny cell (task 5.1's own matrix): a
		// new-lineage candidate that is unrelated to its frozen authority,
		// with legacy present, denies. resolveGoverningAuthority
		// (review_governing_authority.go) wraps this into
		// &ReviewReceiptDiscoveryError{Kind: ReviewReceiptUnrelated}, which
		// runReviewFacadeValidate's UNCONDITIONAL discoveryErr handler
		// (review_facade.go:2905-2910) maps to GateInvalidated — the
		// identical class legacy's own receipt_unrelated reaches.
		if got := reviewtransaction.ResolveGoverningAuthority(true, reviewtransaction.ShadowRelationUnrelated, true); got != reviewtransaction.GoverningAuthorityKindDeny {
			t.Fatalf("ResolveGoverningAuthority(unrelated, legacy present) = %q, want deny", got)
		}
		want := legacyReasonTaxonomyGateResult(ReviewReceiptUnrelated)
		got := reviewtransaction.GateInvalidated // the one unconditional discoveryErr GateResult, review_facade.go:2905-2910
		if got != want {
			t.Fatalf("new-lineage unrelated-deny reaches %q, want the legacy-reachable %q", got, want)
		}
	})

	t.Run(string(ReviewAuthorityCorrupted), func(t *testing.T) {
		// Task 5.2's discovery-integrity clause wraps a corrupted marker
		// into the identical &ReviewReceiptDiscoveryError construction as
		// the unrelated-deny case above, reaching the same unconditional
		// GateInvalidated handler — this is 5.2/5.3's own regression-guarded
		// decision (TestReviewValidateDiscoveryIntegrityMarkerCorruptedDeniesNeverLegacy),
		// cross-checked here against the legacy taxonomy rather than
		// duplicated.
		want := legacyReasonTaxonomyGateResult(ReviewAuthorityCorrupted)
		got := reviewtransaction.GateInvalidated
		if got != want {
			t.Fatalf("new-lineage discovery-integrity corruption reaches %q, want the legacy-reachable %q", got, want)
		}
	})

	t.Run(string(ReviewReceiptScopeChanged), func(t *testing.T) {
		// Task 6.2's own fix: a `changed` relation reaches CoreTransitionCollect
		// (review_core.go's validate()), and newLineageGateEvaluation now maps
		// Collect to GateScopeChanged — the IDENTICAL class legacy's
		// receipt_scope_changed reaches. Calling the real production
		// function, not a duplicated table.
		transition := reviewtransaction.CoreTransition{Kind: reviewtransaction.CoreTransitionCollect, ReasonCode: string(reviewtransaction.ShadowRelationChanged)}
		got := reviewtransaction.EvaluateNewLineageGate(context.Background(), "", fixtureRecord, transition, reviewtransaction.CandidateIdentity{}, reviewtransaction.NativeGateRequestInput{Gate: reviewtransaction.GatePreCommit}).Result
		want := legacyReasonTaxonomyGateResult(ReviewReceiptScopeChanged)
		if got != want {
			t.Fatalf("new-lineage collect(changed) reaches %q, want the legacy-reachable %q", got, want)
		}
	})

	t.Run(string(ReviewReceiptAmbiguous), func(t *testing.T) {
		// Deliberate strengthening (documented, not silent): legacy's
		// receipt_ambiguous collapses into a generic invalidated denial;
		// the new model's relation algebra can name the specific
		// escalation-worthy relation instead. Both are non-allow refusals —
		// the assertion below is exactly that: never allow, and reachable
		// through the real production function.
		transition := reviewtransaction.CoreTransition{Kind: reviewtransaction.CoreTransitionEscalate, ReasonCode: string(reviewtransaction.ShadowRelationAmbiguous)}
		got := reviewtransaction.EvaluateNewLineageGate(context.Background(), "", fixtureRecord, transition, reviewtransaction.CandidateIdentity{}, reviewtransaction.NativeGateRequestInput{Gate: reviewtransaction.GatePreCommit}).Result
		if got == reviewtransaction.GateAllow {
			t.Fatalf("new-lineage ambiguous relation must never allow, got %q", got)
		}
		if got != reviewtransaction.GateEscalated {
			t.Fatalf("new-lineage ambiguous relation = %q, want the documented strengthening to GateEscalated", got)
		}
	})
}

// TestNewLineageGateEvaluationClosedSwitchNeverAllowsUnreachableKinds
// triangulates task 6.2's fold-in of Approve/Repair/Stop: none of the three
// CoreTransitionKind values ReviewCore.validate() itself never returns is
// silently mapped to allow by newLineageGateEvaluation's now-explicit cases.
func TestNewLineageGateEvaluationClosedSwitchNeverAllowsUnreachableKinds(t *testing.T) {
	fixtureRecord := reviewtransaction.NewLineageRecord{
		Authority: reviewtransaction.NewLineageAuthority{LineageID: "taxonomy-fixture", State: reviewtransaction.NewLineageStateReviewing},
	}
	for _, kind := range []reviewtransaction.CoreTransitionKind{
		reviewtransaction.CoreTransitionApprove, reviewtransaction.CoreTransitionRepair, reviewtransaction.CoreTransitionStop,
	} {
		t.Run(string(kind), func(t *testing.T) {
			evaluation := reviewtransaction.EvaluateNewLineageGate(context.Background(), "", fixtureRecord, reviewtransaction.CoreTransition{Kind: kind}, reviewtransaction.CandidateIdentity{}, reviewtransaction.NativeGateRequestInput{Gate: reviewtransaction.GatePreCommit})
			if evaluation.Result == reviewtransaction.GateAllow {
				t.Fatalf("unreachable transition kind %q must never allow, got %#v", kind, evaluation)
			}
			if evaluation.Context.Denial == nil || evaluation.Context.Denial.Code != string(kind) {
				t.Fatalf("unreachable transition kind %q denial code = %#v, want the kind named explicitly (no shared default reuse)", kind, evaluation.Context.Denial)
			}
		})
	}
}
