package cli

// Wave 5 (Gate Cutover), Slice 1: characterization corpus. These tests pinned
// the CURRENT observable behavior of two facade-level surfaces Wave 5 would
// remove or downgrade — the invalidation verb's approved-invalidation branch
// (design decision 2; review_facade.go's `review invalidate --gate` branch,
// which calls reviewtransaction.InvalidateApprovedCompactAuthority) and the
// candidate-decline gate authorization branch (design decision 6;
// runReviewFacadeValidate's ResolveCandidateDeclineForGate branch) — before
// either was deleted/downgraded in later slices (S7 and S6 respectively).
//
// Slice 6 downgraded the decline branch: its own before-picture pin
// (TestCandidateDeclineCharacterization_ResolveCandidateDeclineForGate) is
// superseded by TestCandidateDeclineDowngrade_DeniesLikeAnyNeverReviewedCandidate
// below, which pins the permanent after-picture. Slice 7 deleted the
// invalidation verb's approved-invalidation branch: its own before-picture
// pin (TestInvalidationVerbCharacterization_InvalidateApprovedCompactAuthority)
// is superseded by TestInvalidationVerbDowngrade_RefusesInsteadOfDeriving
// below.
//
// Both surfaces already had coverage of their underlying reviewtransaction
// package functions (compact_approved_invalidation_test.go,
// candidate_decline_test.go — the latter deleted in Slice 6), but neither had
// a test driving the actual CLI entry point / funnel branch end to end
// (verified: no existing test calls RunReviewInvalidate with --gate, and no
// existing test drove a declined candidate through RunReviewFacadeValidate).
// These tests closed exactly that gap and were safety nets: a first-run pass
// was valid and expected — they pinned behavior that already existed, not a
// bugfix.

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestInvalidationVerbCharacterization_InvalidateApprovedCompactAuthority
// pinned `review invalidate --gate`'s approved-invalidation branch
// (review_facade.go, RunReviewInvalidate) through Wave 5 Slice 6: it took
// the compact store's writer lock (via
// reviewtransaction.InvalidateApprovedCompactAuthority), rewrote persisted
// state to invalidated with bound InvalidationEvidence, and removed the
// receipt file.
//
// TestInvalidationVerbDowngrade_RefusesInsteadOfDeriving below supersedes
// it (Wave 5 Slice 7, design decision 2): `invalidated` is now a derived
// verdict `review validate --gate <gate>` reports directly
// (TestReceiptFilePersistsAfterDerivedInvalidation_AllFiveGates,
// review_invalidation_verb_deletion_test.go, proves this for all five
// gates), never a write this verb performs — `review invalidate` refuses
// unconditionally for an approved lineage instead, touching no authority
// bytes, and the receipt is never removed
// (TestNoGateWritesAuthority_CallAbsenceGuard,
// internal/reviewtransaction/gate_write_guard_test.go, proves it by
// call-absence).
func TestInvalidationVerbDowngrade_RefusesInsteadOfDeriving(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	stubReviewConsole(t, false, "")
	lineage := "review-invalidation-verb-characterization"
	writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o644)
	finalizeApprovedFacadeReview(t, repo, lineage)

	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if before.State.State != reviewtransaction.StateApproved {
		t.Fatalf("fixture did not reach approved: %#v", before.State)
	}
	receiptBefore, err := os.ReadFile(store.ReceiptPath())
	if err != nil {
		t.Fatalf("approved fixture carries no receipt: %v", err)
	}

	// The identical out-of-scope drift the old branch used to re-derive
	// invalidated from: no longer relevant to the outcome
	// (review invalidate refuses unconditionally for any approved lineage
	// now, regardless of drift), kept only so this fixture stays a faithful
	// "before" shape.
	writeReviewStartCandidate(t, repo, "outside.txt", "outside frozen scope\n", 0o644)

	var output bytes.Buffer
	err = RunReviewInvalidate([]string{
		"--cwd", repo, "--lineage", lineage, "--expected-revision", before.Revision,
		"--gate", string(reviewtransaction.GatePostApply),
	}, &output)
	if err == nil {
		t.Fatalf("review invalidate --gate unexpectedly succeeded for an approved lineage:\n%s", output.String())
	}
	if !strings.Contains(err.Error(), "no longer performs gate-derived invalidation") || !strings.Contains(err.Error(), "review validate") {
		t.Fatalf("review invalidate --gate refusal = %v, want it to name review validate as the alternative", err)
	}

	after, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if after.State.State != reviewtransaction.StateApproved || after.Revision != before.Revision {
		t.Fatalf("review invalidate --gate mutated persisted authority: before=%#v after=%#v", before.State, after.State)
	}
	receiptAfter, err := os.ReadFile(store.ReceiptPath())
	if err != nil {
		t.Fatalf("review invalidate --gate removed the receipt: %v", err)
	}
	if !bytes.Equal(receiptBefore, receiptAfter) {
		t.Fatal("review invalidate --gate changed receipt bytes")
	}
}

// TestCandidateDeclineCharacterization_ResolveCandidateDeclineForGate pinned
// the full facade round trip through Wave 5 Slice 5 (before Slice 6 deleted
// it): a relayed consent decline persisted a CandidateDeclineAuthorization
// (RecordCandidateDecline, review_facade.go's `errReviewDeclinedForCandidate`
// branch), and a subsequent `review validate --gate pre-commit` for the
// identical staged candidate resolved it via ResolveCandidateDeclineForGate
// and reached ordinary unmanaged delivery
// (emitCandidateDeclinedUnmanagedDelivery, Delivery:
// RDDDeliveryCandidateDeclinedUnmanaged) — never review authority, never a
// receipt-like record.
//
// TestCandidateDeclineDowngrade_DeniesLikeAnyNeverReviewedCandidate below
// supersedes it (Wave 5 Slice 6, design decision 6): decline is no longer
// durably recorded at all (RecordCandidateDecline is deleted;
// TestCandidateDecline_ZeroCallers, review_candidate_decline_downgrade_test.go,
// proves it by call-absence), so the identical fixture now reaches the SAME
// generic receipt_missing denial ANY never-reviewed candidate reaches — the
// gate has no decline-specific detail left anywhere to read. Consistent
// with Wave 4 decision 4 ("decline = unmanaged proceed: nothing recorded").
// The decline's only remaining observable effect is the ONE `review start`
// call itself reporting `Action: "declined"` — it creates no lasting
// delivery-gate authorization for any later gate call.
func TestCandidateDeclineDowngrade_DeniesLikeAnyNeverReviewedCandidate(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	stubReviewConsole(t, false, "")
	writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o644)

	relayArgs := boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--lineage", "review-decline-characterization", "--consent", "relay",
	})
	question := decodeConsentQuestion(t, runConsentRelayStart(t, relayArgs).Bytes())
	declineArgs := invocationArgs(t, question.Choices[1].Invocation)
	declined := runConsentRelayStart(t, declineArgs)
	var declinedResult ReviewFacadeStartResult
	decodeStrictReviewJSON(t, declined.Bytes(), &declinedResult)
	if declinedResult.Action != "declined" {
		t.Fatalf("decline did not report: %#v", declinedResult)
	}

	// The identical declined candidate is now a supported pre-commit
	// target: the (now-deleted) decline-gate matcher only ever covered
	// pre-commit, pre-push, and pre-pr, never post-apply/release.
	runReviewCLIGit(t, repo, "add", "scripts/deploy.sh")

	var output bytes.Buffer
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePreCommit)}, &output); err != nil {
		t.Fatalf("declined candidate delivery: %v\n%s", err, output.String())
	}
	assertEnabledUnmanagedGatePayload(t, output.Bytes(), reviewtransaction.GatePreCommit)

	// The decline never created review authority for this lineage.
	if store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, "review-decline-characterization"); err == nil {
		if _, loadErr := store.Load(); !errors.Is(loadErr, os.ErrNotExist) {
			t.Fatalf("declined candidate gate evaluation persisted review authority: %v", loadErr)
		}
	}
}
