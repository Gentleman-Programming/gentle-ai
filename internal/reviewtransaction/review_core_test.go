package reviewtransaction

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

// TestReviewCoreStartFreezesOnlyAfterConsentGranted is task 4.1's RED test
// (spec rdd-review-core-transitions, "Consent-Gated Freeze With Immutable
// Tier, Lenses, and Budget"): tier 1|4 candidates must not freeze until the
// reused v2 consent envelope is granted; tier 0 is the carve-out that
// proceeds straight to freeze without a consent question.
func TestReviewCoreStartFreezesOnlyAfterConsentGranted(t *testing.T) {
	identity := fixtureCandidateIdentity(strings.Repeat("a", 40), strings.Repeat("b", 40), "c", "d")
	cases := []struct {
		name           string
		tier           RiskLevel
		consentGranted bool
		wantFreeze     bool
	}{
		{"tier0 no consent question proceeds to freeze", RiskLow, false, true},
		{"tier0 consent granted still freezes", RiskLow, true, true},
		{"tier1 blocked without consent", RiskMedium, false, false},
		{"tier1 freezes once consent granted", RiskMedium, true, true},
		{"tier4 blocked without consent", RiskHigh, false, false},
		{"tier4 freezes once consent granted", RiskHigh, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			core := ReviewCore{}
			transition, err := core.Next(context.Background(), NewLineageAuthority{}, CoreRequest{
				Kind: CoreRequestStart, CandidateIdentity: identity, Tier: tc.tier,
				SelectedLenses: []string{"risk"}, OriginalChangedLines: 41, ConsentGranted: tc.consentGranted,
			})
			if tc.wantFreeze {
				if err != nil {
					t.Fatalf("Next(tier=%q, consent=%v) error = %v, want nil", tc.tier, tc.consentGranted, err)
				}
				if transition.Kind != CoreTransitionContinue || transition.Authority == nil {
					t.Fatalf("Next(tier=%q, consent=%v) = %#v, want a continue transition with a frozen authority", tc.tier, tc.consentGranted, transition)
				}
				if transition.Authority.Tier != tc.tier || transition.Authority.State != NewLineageStateReviewing {
					t.Fatalf("frozen authority = %#v, want tier %q state reviewing", transition.Authority, tc.tier)
				}
				if transition.Authority.CorrectionBudgetLines != 21 {
					t.Fatalf("frozen authority correction budget = %d, want 21", transition.Authority.CorrectionBudgetLines)
				}
				return
			}
			if !errors.Is(err, ErrConsentRequiredBeforeFreeze) {
				t.Fatalf("Next(tier=%q, consent=false) error = %v, want ErrConsentRequiredBeforeFreeze", tc.tier, err)
			}
			if transition.Authority != nil {
				t.Fatalf("consent-blocked start must not freeze an authority, got %#v", transition.Authority)
			}
		})
	}
}

// TestReviewCoreConsentBlockedStartPersistsNothing proves the ordering
// invariant end-to-end, not just in Next's return value: a consent-blocked
// start gives its caller nothing to Mutate with (transition.Authority is
// nil), so no v3 artifact is ever created for a tier that required consent
// it never received.
func TestReviewCoreConsentBlockedStartPersistsNothing(t *testing.T) {
	repo := initSnapshotRepo(t)
	store, err := NewLineageAuthorityStore(context.Background(), repo, "consent-blocked-lineage")
	if err != nil {
		t.Fatal(err)
	}
	core := ReviewCore{}
	transition, err := core.Next(context.Background(), NewLineageAuthority{}, CoreRequest{
		Kind:                 CoreRequestStart,
		CandidateIdentity:    fixtureCandidateIdentity(strings.Repeat("a", 40), strings.Repeat("b", 40), "e", "f"),
		Tier:                 RiskHigh,
		SelectedLenses:       []string{"risk", "resilience", "readability", "reliability"},
		OriginalChangedLines: 80,
		ConsentGranted:       false,
	})
	if !errors.Is(err, ErrConsentRequiredBeforeFreeze) {
		t.Fatalf("Next(tier4, consent=false) error = %v, want ErrConsentRequiredBeforeFreeze", err)
	}
	if transition.Authority != nil {
		t.Fatal("consent-blocked start returned a frozen authority")
	}
	if _, statErr := os.Stat(store.StatePath()); !os.IsNotExist(statErr) {
		t.Fatalf("consent-blocked start left a v3 artifact behind: %v", statErr)
	}
}

// TestReviewCoreValidateExactMatchContinues proves DeriveObservation's full
// context (Wave 3 Slice 2's deferral, resolved by this slice) reaches
// relateCandidates correctly: a live candidate identical to the frozen one
// continues rather than escalating or asking for a correction.
func TestReviewCoreValidateExactMatchContinues(t *testing.T) {
	identity := fixtureCandidateIdentity(strings.Repeat("a", 40), strings.Repeat("b", 40), "c", "d")
	authority := NewLineageAuthority{LineageID: "validate-exact-lineage", State: NewLineageStateReviewing, CandidateIdentity: identity, Tier: RiskLow}
	core := ReviewCore{}
	transition, err := core.Next(context.Background(), authority, CoreRequest{
		Kind: CoreRequestValidate, LiveCandidateIdentity: identity,
		Evidence: CoreValidateEvidence{LiveSnapshot: Snapshot{Paths: []string{"a.go"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if transition.Kind != CoreTransitionContinue || transition.ReasonCode != string(ShadowRelationExact) {
		t.Fatalf("validate(exact) = %#v, want continue/exact", transition)
	}
}

// TestReviewCoreValidateChangedAsksForOneCorrection proves the "One Bounded
// Correction" requirement's request-side: a live candidate that has drifted
// from the frozen one is a `collect` transition, not an immediate refusal —
// unless the authority is already correcting, matching
// AuthorityStore.ResolveReplay's own ErrOneCorrectionOnly boundary
// (Wave 3 Slice 2).
func TestReviewCoreValidateChangedAsksForOneCorrection(t *testing.T) {
	frozen := fixtureCandidateIdentity(strings.Repeat("a", 40), strings.Repeat("b", 40), "c", "d")
	live := fixtureCandidateIdentity(strings.Repeat("a", 40), strings.Repeat("1", 40), "c", "d")
	authority := NewLineageAuthority{LineageID: "validate-changed-lineage", State: NewLineageStateReviewing, CandidateIdentity: frozen, Tier: RiskMedium}
	core := ReviewCore{}
	transition, err := core.Next(context.Background(), authority, CoreRequest{
		Kind: CoreRequestValidate, LiveCandidateIdentity: live,
		Evidence: CoreValidateEvidence{LiveSnapshot: Snapshot{Paths: []string{"a.go"}}, ApplicableAuthorities: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if transition.Kind != CoreTransitionCollect || len(transition.Collect) != 1 {
		t.Fatalf("validate(changed, reviewing) = %#v, want one collect input", transition)
	}

	authority.State = NewLineageStateCorrecting
	_, err = core.Next(context.Background(), authority, CoreRequest{
		Kind: CoreRequestValidate, LiveCandidateIdentity: live,
		Evidence: CoreValidateEvidence{LiveSnapshot: Snapshot{Paths: []string{"a.go"}}, ApplicableAuthorities: 1},
	})
	if !errors.Is(err, ErrOneCorrectionOnly) {
		t.Fatalf("validate(changed, correcting) error = %v, want ErrOneCorrectionOnly", err)
	}
}

// TestReviewCoreFinalizeRequiresTerminalState proves finalize refuses an
// authority that has not reached approved or escalated (spec
// rdd-review-core-transitions, "Terminal Receipt Issuance Exactly Once").
func TestReviewCoreFinalizeRequiresTerminalState(t *testing.T) {
	core := ReviewCore{}
	_, err := core.Next(context.Background(), NewLineageAuthority{LineageID: "finalize-reviewing-lineage", State: NewLineageStateReviewing}, CoreRequest{Kind: CoreRequestFinalize})
	if !errors.Is(err, ErrFinalizeRequiresTerminalState) {
		t.Fatalf("finalize(reviewing) error = %v, want ErrFinalizeRequiresTerminalState", err)
	}
}

// TestReviewCoreFinalizeApprovedIssuesReceiptRef proves finalize binds the
// terminal transition to the exact authority revision it closes.
func TestReviewCoreFinalizeApprovedIssuesReceiptRef(t *testing.T) {
	authority := fixtureNewLineageAuthority("finalize-approved-lineage", NewLineageStateApproved)
	core := ReviewCore{}
	transition, err := core.Next(context.Background(), authority, CoreRequest{Kind: CoreRequestFinalize})
	if err != nil {
		t.Fatal(err)
	}
	if transition.Kind != CoreTransitionApprove || transition.Receipt == nil || transition.Receipt.LineageID != authority.LineageID {
		t.Fatalf("finalize(approved) = %#v", transition)
	}
	wantRevision, err := NewLineageRevisionForState(authority)
	if err != nil {
		t.Fatal(err)
	}
	if transition.Receipt.AuthorityRevision != wantRevision {
		t.Fatalf("finalize(approved) receipt revision = %q, want %q", transition.Receipt.AuthorityRevision, wantRevision)
	}
}

// TestReviewCoreFinalizeAdvancesNonTerminalWithAdvanceRequest is C6's own
// RED/GREEN evidence (verify-report CRITICAL, "ReviewCore-owned transitions
// in finalize"): given an explicit AdvanceRequest, finalize itself decides
// the terminal state for a non-terminal authority and hands back the
// resulting NewLineageAuthority as CoreTransition.Authority — the caller's
// only remaining job is to Mutate it verbatim, mirroring start's own
// contract. This also doubles as W14's evidence: `correcting` now finalizes
// through the identical path as `reviewing`, closing the one bounded
// correction it already spent rather than being refused outright.
func TestReviewCoreFinalizeAdvancesNonTerminalWithAdvanceRequest(t *testing.T) {
	core := ReviewCore{}
	for _, state := range []NewLineageState{NewLineageStateReviewing, NewLineageStateCorrecting, NewLineageStateValidating} {
		t.Run(string(state), func(t *testing.T) {
			authority := fixtureNewLineageAuthority("finalize-advance-"+string(state)+"-lineage", state)
			transition, err := core.Next(context.Background(), authority, CoreRequest{
				Kind: CoreRequestFinalize, AdvanceRequest: &FinalizeAdvanceRequest{},
			})
			if err != nil {
				t.Fatal(err)
			}
			if transition.Kind != CoreTransitionApprove || transition.Authority == nil || transition.Authority.State != NewLineageStateApproved {
				t.Fatalf("finalize(%s, advance, no blocker) = %#v, want approve with an advanced authority", state, transition)
			}
			if transition.Receipt == nil || transition.Receipt.LineageID != authority.LineageID {
				t.Fatalf("finalize(%s, advance) receipt = %#v", state, transition.Receipt)
			}
			wantRevision, err := NewLineageRevisionForState(*transition.Authority)
			if err != nil {
				t.Fatal(err)
			}
			if transition.Receipt.AuthorityRevision != wantRevision {
				t.Fatalf("finalize(%s, advance) receipt revision = %q, want %q", state, transition.Receipt.AuthorityRevision, wantRevision)
			}
		})
	}
}

// TestReviewCoreFinalizeAdvanceEscalatesOnFailedOrAdmittedFindings proves
// finalize's own decision, not the caller's: a failed evidence flag or any
// admitted candidate-causal finding ID routes the advance to `escalated`
// instead of `approved`, and admitted finding IDs are appended to the
// resulting authority.
func TestReviewCoreFinalizeAdvanceEscalatesOnFailedOrAdmittedFindings(t *testing.T) {
	core := ReviewCore{}
	cases := []struct {
		name    string
		request FinalizeAdvanceRequest
	}{
		{"failed evidence", FinalizeAdvanceRequest{Failed: true}},
		{"admitted candidate-causal finding", FinalizeAdvanceRequest{AdmittedFindingIDs: []string{"F-CAUSAL"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			authority := fixtureNewLineageAuthority("finalize-advance-escalate-lineage", NewLineageStateReviewing)
			transition, err := core.Next(context.Background(), authority, CoreRequest{Kind: CoreRequestFinalize, AdvanceRequest: &tc.request})
			if err != nil {
				t.Fatal(err)
			}
			if transition.Kind != CoreTransitionEscalate || transition.Authority == nil || transition.Authority.State != NewLineageStateEscalated {
				t.Fatalf("finalize(reviewing, %s) = %#v, want escalate with an escalated authority", tc.name, transition)
			}
			if len(tc.request.AdmittedFindingIDs) > 0 && !reflect.DeepEqual(transition.Authority.AdmittedFindingIDs, tc.request.AdmittedFindingIDs) {
				t.Fatalf("finalize(reviewing, %s) admitted finding ids = %v, want %v", tc.name, transition.Authority.AdmittedFindingIDs, tc.request.AdmittedFindingIDs)
			}
		})
	}
}

// TestReviewCoreFinalizeRefusesNonTerminalWithoutAdvanceRequest proves the
// opt-in boundary the pinned TestReviewCoreFinalizeRequiresTerminalState
// depends on: a bare CoreRequestFinalize (nil AdvanceRequest) against ANY
// non-terminal state — including `correcting`, which previously had no
// finalize path at all — still refuses. Finalize never silently closes out
// an in-flight lineage on a caller's behalf.
func TestReviewCoreFinalizeRefusesNonTerminalWithoutAdvanceRequest(t *testing.T) {
	core := ReviewCore{}
	for _, state := range []NewLineageState{NewLineageStateReviewing, NewLineageStateCorrecting, NewLineageStateValidating} {
		t.Run(string(state), func(t *testing.T) {
			authority := fixtureNewLineageAuthority("finalize-refuse-"+string(state)+"-lineage", state)
			_, err := core.Next(context.Background(), authority, CoreRequest{Kind: CoreRequestFinalize})
			if !errors.Is(err, ErrFinalizeRequiresTerminalState) {
				t.Fatalf("finalize(%s, no advance) error = %v, want ErrFinalizeRequiresTerminalState", state, err)
			}
		})
	}
}
