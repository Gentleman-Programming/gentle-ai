package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
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
	lineage := startLowRiskFacadeReview(t, repo)
	contendedReceiptWriter(t, compactAuthorityLockPath(t, repo, lineage), 150*time.Millisecond)

	var output bytes.Buffer
	if err := RunReview([]string{
		"finalize", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", lineage,
	}, &output); err != nil {
		t.Fatalf("FINALIZE did not converge past a briefly-held authority lock at receipt publication: %v\n%s", err, output.String())
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.ReceiptPath()); err != nil {
		t.Fatalf("converged FINALIZE left no terminal receipt: %v", err)
	}
}

// TestFinalizeReceiptPublicationExhaustionKeepsPendingReplay is the guard on
// the convergence above: `receipt_publication_pending` exists for the genuine
// window where terminal authority is committed and the receipt is still
// unpublished — a crashed competitor, a disk fault, or contention that
// outlives the bounded wait. That case must keep its honest shape: committed,
// exactly replayable via the lineage-only FINALIZE it names, and convergent
// once the obstruction clears. Do not relax this test to make publication
// failures disappear.
func TestFinalizeReceiptPublicationExhaustionKeepsPendingReplay(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	lineage := startLowRiskFacadeReview(t, repo)
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
		failure.MutationOutcome != ReviewMutationCommitted ||
		failure.Replayability != reviewtransaction.ReplayabilityExactReplaySafe ||
		failure.NextAction != ReviewIntegrationOperationFinalize ||
		failure.LineageID != lineage || !strings.HasPrefix(failure.RequestDigest, "sha256:") {
		t.Fatalf("exhausted publication failure = %#v, want the committed exactly-replayable pending shape", failure)
	}
	// `retry_safe` and `replayability` are independent axes of the published
	// contract: retrying the request as issued is not the route out (a
	// terminal replay admits only --lineage), while the declared exact replay
	// is. The envelope keeps saying exactly that.
	if failure.RetrySafe {
		t.Fatalf("exhausted publication failure claims the request as issued is retryable: %#v", failure)
	}

	var converged bytes.Buffer
	if err := RunReview([]string{
		"finalize", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", lineage,
	}, &converged); err != nil {
		t.Fatalf("exact lineage-only replay after the obstruction cleared: %v\n%s", err, converged.String())
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.ReceiptPath()); err != nil {
		t.Fatalf("replay after obstruction left no terminal receipt: %v", err)
	}
}
