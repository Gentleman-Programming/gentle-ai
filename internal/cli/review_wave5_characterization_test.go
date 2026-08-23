package cli

// Wave 5 (Gate Cutover), Slice 6: candidate-decline characterization. The
// decline branch is now intentionally transient: its only observable effect is
// the one `review start` response. Later delivery-gate evaluation follows the
// ordinary never-reviewed candidate path.

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

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
