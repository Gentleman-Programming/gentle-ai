package reviewtransaction

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestEnsureNoPreparedCompactBatchReconciliation covers the retained marker
// guard directly. It used to be exercised only incidentally, as a side
// effect of ReconcileInvalidRecoveryEdges's own journal lifecycle tests
// (compact_batch_reconcile_journal_test.go, Wave 7 S4b -- that whole file,
// including its marker-creation/removal coverage, retired with the
// provider it tested). This is the direct, focused replacement: the guard
// itself is retained, live production code (status.go, authority_repair.go,
// store_lock.go and others still call it), so it keeps its own coverage
// independent of the provider that used to be its only caller's test
// subject.
func TestEnsureNoPreparedCompactBatchReconciliation(t *testing.T) {
	t.Run("no marker present is a clean pass", func(t *testing.T) {
		base := t.TempDir()
		if err := ensureNoPreparedCompactBatchReconciliation(base); err != nil {
			t.Fatalf("ensureNoPreparedCompactBatchReconciliation() = %v, want nil", err)
		}
	})

	t.Run("a leftover marker refuses with ErrCompactBatchReconcilePrepared", func(t *testing.T) {
		base := t.TempDir()
		if err := os.WriteFile(compactBatchReconcileMarkerPath(base), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		err := ensureNoPreparedCompactBatchReconciliation(base)
		if !errors.Is(err, ErrCompactBatchReconcilePrepared) {
			t.Fatalf("ensureNoPreparedCompactBatchReconciliation() = %v, want ErrCompactBatchReconcilePrepared", err)
		}
	})

	t.Run("marker path is exactly base/batch-reconcile-journal.json", func(t *testing.T) {
		base := t.TempDir()
		want := filepath.Join(base, "batch-reconcile-journal.json")
		if got := compactBatchReconcileMarkerPath(base); got != want {
			t.Fatalf("compactBatchReconcileMarkerPath(%q) = %q, want %q", base, got, want)
		}
	})
}
