package reviewtransaction

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// This file used to also hold ReconcileInvalidRecoveryEdges -- the provider
// behind the `review reconcile-authority-batch` CLI verb -- and its whole
// durable-journal apply/replay machinery. Wave 7 S4a retired the verb
// (review_reconcile_batch.go, WU9); S4b retires the provider itself, since
// nothing else called it once the verb was gone.
//
// What remains is a small, LIVE, RETAINED safety guard: an on-disk
// "batch-reconcile-journal.json" marker at the authority root means a batch
// reconciliation was prepared (or partially applied) and never finished --
// possibly from BEFORE this wave, on a repository that ran the now-retired
// verb. Six other files still call ensureNoPreparedCompactBatchReconciliation
// to check for that marker before mutating or reporting on authority
// (status.go, authority_disposition_execute.go, compact_inspect.go,
// store_lock.go, final_verification_retry.go, compact_store.go -- confirmed
// by grep before this file was cut down): deleting the guard would let those
// live mutation paths silently proceed past an unfinished historical batch
// reconciliation instead of refusing, which is exactly the forensic-safety
// regression D5 (legacy read retention) forbids for ANY historical, on-disk
// artifact this wave's deletions must never blind the product to.
// (authority_repair.go also Lstats the marker path directly for its own
// conflict-count probe, but it is not a caller of this guard function.)
const compactBatchReconcileMarker = "batch-reconcile-journal.json"

// ErrCompactBatchReconcilePrepared is returned when an on-disk marker from a
// prior (possibly historical, pre-retirement) batch reconciliation is still
// present. Nothing can create a NEW one anymore -- ReconcileInvalidRecoveryEdges
// retired with the verb -- but an existing one must still be detected and
// refused around, never silently ignored.
var ErrCompactBatchReconcilePrepared = errors.New("compact batch reconciliation is prepared; exact replay is required")

func compactBatchReconcileMarkerPath(base string) string {
	return filepath.Join(base, compactBatchReconcileMarker)
}

// ensureNoPreparedCompactBatchReconciliation is the read-only guard every
// live mutation and status-reporting path in this package still calls.
func ensureNoPreparedCompactBatchReconciliation(base string) error {
	if _, err := os.Lstat(compactBatchReconcileMarkerPath(base)); err == nil {
		return ErrCompactBatchReconcilePrepared
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("%w: inspect global marker", ErrCompactBatchReconcilePrepared)
	}
	return nil
}
