package cli

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// contendedReceiptWriter wraps the real receipt writer so that its first call
// runs while the repository-wide authority lock is genuinely held, exactly as
// a concurrent FINALIZE winner holds it across its own publication
// bookkeeping. The lock is the real advisory primitive on the real path; only
// the timing is scripted, so whatever error surfaces is the production error.
func contendedReceiptWriter(t *testing.T, lockPath string, release time.Duration) {
	t.Helper()
	original := writeCompactFacadeReceipt
	t.Cleanup(func() { writeCompactFacadeReceipt = original })
	var once atomic.Bool
	writeCompactFacadeReceipt = func(ctx context.Context, store reviewtransaction.CompactStore, receipt reviewtransaction.CompactReceipt) error {
		if once.CompareAndSwap(false, true) {
			held, err := reviewtransaction.AcquireAuthorityFileLock(lockPath)
			if err != nil {
				return fmt.Errorf("hold authority lock for receipt contention: %w", err)
			}
			if release > 0 {
				go func() {
					time.Sleep(release)
					_ = held.Release()
				}()
			} else {
				t.Cleanup(func() { _ = held.Release() })
			}
		}
		return original(ctx, store, receipt)
	}
}

// TestFinalizeConvergesWhenReceiptPublicationMeetsBrieflyHeldLock is the
// deterministic reproduction of the third concurrent-FINALIZE loser shape
// (after lock contention and the revision conflict): a finalizer whose
// transitions all converged reaches receipt publication at the instant a
// competitor holds the advisory lock. The competitor holds it for
// milliseconds and publishes the identical derived receipt, so the honest
// outcome is success by convergence — not a `receipt_publication_pending`
// failure with `mutation_outcome: committed` sending the caller to replay a
// publication that is not pending in any durable sense.
func TestFinalizeConvergesWhenReceiptPublicationMeetsBrieflyHeldLock(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	// Receipt publication pending/replay is compact-v2 behavior. Construct the
	// historical compact authority directly; negotiated START now creates v3.
	lineage := startFacadeReview(t, repo).LineageID
	contendedReceiptWriter(t, compactAuthorityLockPath(t, repo, lineage), 150*time.Millisecond)

	var output bytes.Buffer
	if err := RunReview([]string{
		"finalize", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", lineage,
	}, &output); err != nil {
		t.Fatalf("FINALIZE did not converge past a briefly-held authority lock at receipt publication: %v\n%s", err, output.String())
	}
	finalized := assertApprovedBurnedCompactNegotiatedFinalize(t, output.Bytes())
	if finalized.LineageID != lineage {
		t.Fatalf("converged FINALIZE lineage = %q, want %q", finalized.LineageID, lineage)
	}
	if bytes.Contains(output.Bytes(), []byte("review.status")) || bytes.Contains(output.Bytes(), []byte("next_transition")) {
		t.Fatalf("clean FINALIZE added a terminal STATUS ceremony: %s", output.String())
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	assertApprovedCompactAuthorityBurned(t, store, lineage)
}

// TestFinalizeReceiptPublicationExhaustionRequiresStatusBeforeReplay is the
// guard on the convergence above: `receipt_publication_pending` exists for the
// genuine window where terminal authority is committed and the receipt is still
// unpublished — a crashed competitor, a disk fault, or contention that
// outlives the bounded wait. That ambiguity must first route through the exact
// lineage-bound read-only STATUS, which re-derives whether native reconciliation
// may ask for the original FINALIZE request again. Do not relax this test to
// make publication failures disappear.
func TestFinalizeReceiptPublicationExhaustionRequiresStatusBeforeReplay(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	// Receipt publication pending/replay is compact-v2 behavior. Construct the
	// historical compact authority directly; negotiated START now creates v3.
	lineage := startFacadeReview(t, repo).LineageID
	lockPath := compactAuthorityLockPath(t, repo, lineage)
	// The lock is taken only at the receipt writer and held past any bounded
	// wait, so the operation provably reaches the publication step and the
	// obstruction provably outlives it — earlier acquisitions would surface as
	// ordinary pre-native lock contention instead.
	original := writeCompactFacadeReceipt
	t.Cleanup(func() { writeCompactFacadeReceipt = original })
	var held *reviewtransaction.AuthorityFileLock
	var once atomic.Bool
	writeCompactFacadeReceipt = func(ctx context.Context, store reviewtransaction.CompactStore, receipt reviewtransaction.CompactReceipt) error {
		if once.CompareAndSwap(false, true) {
			lock, err := reviewtransaction.AcquireAuthorityFileLock(lockPath)
			if err != nil {
				return fmt.Errorf("hold authority lock for receipt exhaustion: %w", err)
			}
			held = lock
		}
		return original(ctx, store, receipt)
	}

	var output bytes.Buffer
	err := RunReview([]string{
		"finalize", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", lineage,
	}, &output)
	writeCompactFacadeReceipt = original
	if held == nil {
		t.Fatal("the receipt writer never ran; this test proved nothing")
	}
	if releaseErr := held.Release(); releaseErr != nil {
		t.Fatal(releaseErr)
	}
	if err == nil {
		t.Fatalf("FINALIZE reached a receipt through a continuously-held authority lock: %s", output.String())
	}
	failure := decodeReviewIntegrationFailure(t, output.Bytes())
	if failure.Code != "receipt_publication_pending" || failure.Phase != "native_committed" ||
		failure.MutationOutcome != ReviewMutationCommitted || failure.RetrySafe ||
		failure.Replayability != reviewtransaction.ReplayabilityStatusRequired ||
		failure.NextAction != "review.status" || failure.LineageID != lineage ||
		!strings.HasPrefix(failure.TargetIdentity, "sha256:") || !strings.HasPrefix(failure.RequestDigest, "sha256:") {
		t.Fatalf("exhausted publication failure = %#v, want the committed status-required pending shape", failure)
	}

	var statusOutput bytes.Buffer
	if err := RunReview([]string{
		"status", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", lineage, "--next-transition",
	}, &statusOutput); err != nil {
		t.Fatalf("exact lineage-bound STATUS after publication ambiguity: %v\n%s", err, statusOutput.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, statusOutput.Bytes(), &status)
	if status.TargetIdentity != failure.TargetIdentity || status.NextTransition == nil ||
		status.NextTransition.Kind != reviewNextTransitionStop ||
		status.NextTransition.ReasonCode != "original_finalize_request_required" {
		t.Fatalf("publication ambiguity STATUS = %#v, want native original-request replay only after target-bound STATUS", status)
	}

	var converged bytes.Buffer
	if err := RunReview([]string{
		"finalize", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", lineage,
	}, &converged); err != nil {
		t.Fatalf("exact lineage-only replay after native STATUS reconciliation: %v\n%s", err, converged.String())
	}
	finalized := assertApprovedBurnedCompactNegotiatedFinalize(t, converged.Bytes())
	if finalized.LineageID != lineage {
		t.Fatalf("replay after obstruction lineage = %q, want %q", finalized.LineageID, lineage)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	assertApprovedCompactAuthorityBurned(t, store, lineage)
}
