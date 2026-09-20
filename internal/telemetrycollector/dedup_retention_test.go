package telemetrycollector

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func countDeliveryIDs(t *testing.T, s *Storage) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM runtime_delivery_ids`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func insertDeliveryIDAt(t *testing.T, s *Storage, id int, at time.Time) {
	t.Helper()
	if decision, err := s.InsertRuntimeDeliveryID(context.Background(), fmt.Sprintf("%032x", id), at); err != nil || decision != "stored" {
		t.Fatalf("insert delivery id %d: %q %v", id, decision, err)
	}
}

// The dedup table only has to reject a replay that arrives moments after the
// original, so it gets a cutoff of its own, later than the raw-data cutoff:
// identities older than the dedup cutoff go even though the raw events and
// sqlite-mode deliveries of the same age stay.
func TestPurgeOlderThan_DedupCutoffRemovesIDsButKeepsRawData(t *testing.T) {
	s := openTestStorage(t)
	ctx := context.Background()
	rawCutoff := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	dedupCutoff := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	between := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)

	if err := s.InsertEvent(ctx, installEvent(t, installA), between); err != nil {
		t.Fatal(err)
	}
	insertRetentionDelivery(t, s, 1, between)
	insertDeliveryIDAt(t, s, 10, between)                           // older than the dedup cutoff: purged
	insertDeliveryIDAt(t, s, 11, dedupCutoff.Add(-time.Nanosecond)) // just older: purged
	insertDeliveryIDAt(t, s, 12, dedupCutoff)                       // at the cutoff: kept

	if _, err := s.PurgeOlderThan(ctx, rawCutoff, dedupCutoff); err != nil {
		t.Fatal(err)
	}

	if got := countDeliveryIDs(t, s); got != 1 {
		t.Fatalf("runtime_delivery_ids after purge = %d, want 1", got)
	}
	assertRetentionCounts(t, s, 1, 1, 2)
	// An expired identity dedupes no more: the replay counts as fresh.
	insertDeliveryIDAt(t, s, 10, dedupCutoff.Add(time.Hour))
}

// A million-row purge must not hold the single writer for seconds, so the
// identities are removed in bounded batches, each its own short transaction.
func TestPurgeOlderThan_DeliveryIDsPurgedInBatches(t *testing.T) {
	s := openTestStorage(t)
	ctx := context.Background()
	previous := runtimeDeliveryIDPurgeBatch
	runtimeDeliveryIDPurgeBatch = 10
	t.Cleanup(func() { runtimeDeliveryIDPurgeBatch = previous })

	cutoff := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 35; i++ {
		insertDeliveryIDAt(t, s, 100+i, cutoff.Add(-time.Duration(i+1)*time.Minute))
	}
	for i := 0; i < 3; i++ {
		insertDeliveryIDAt(t, s, 200+i, cutoff.Add(time.Duration(i)*time.Minute))
	}

	if _, err := s.PurgeOlderThan(ctx, cutoff, cutoff); err != nil {
		t.Fatal(err)
	}
	if got := countDeliveryIDs(t, s); got != 3 {
		t.Fatalf("runtime_delivery_ids after batched purge = %d, want 3", got)
	}
}

// RunMaintenance derives both cutoffs from now: raw data keeps retentionDays,
// identities keep runtimeDedupDays, and the dedup window can never outlive
// the raw window.
func TestRunMaintenance_DedupWindowIsShorterAndNeverLongerThanRetention(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 10, 3, 0, 0, 0, time.UTC)

	t.Run("shorter dedup window drops ids the raw window would keep", func(t *testing.T) {
		s := openTestStorage(t)
		fiveDaysAgo := now.AddDate(0, 0, -5)
		if err := s.InsertEvent(ctx, installEvent(t, installA), fiveDaysAgo); err != nil {
			t.Fatal(err)
		}
		insertDeliveryIDAt(t, s, 1, fiveDaysAgo)
		insertDeliveryIDAt(t, s, 2, now.Add(-time.Hour))

		if err := RunMaintenance(ctx, s, now, 90, 2); err != nil {
			t.Fatal(err)
		}
		if got := countDeliveryIDs(t, s); got != 1 {
			t.Fatalf("delivery ids = %d, want only the one-hour-old id", got)
		}
		if remaining, err := s.eventsOnDay(ctx, fiveDaysAgo); err != nil || len(remaining) != 1 {
			t.Fatalf("five-day-old event must survive a 90-day retention: %d %v", len(remaining), err)
		}
	})

	t.Run("dedup window longer than retention is clamped to retention", func(t *testing.T) {
		s := openTestStorage(t)
		insertDeliveryIDAt(t, s, 1, now.AddDate(0, 0, -10))
		insertDeliveryIDAt(t, s, 2, now.AddDate(0, 0, -1))

		if err := RunMaintenance(ctx, s, now, 2, 30); err != nil {
			t.Fatal(err)
		}
		if got := countDeliveryIDs(t, s); got != 1 {
			t.Fatalf("delivery ids = %d, want the ten-day-old id gone under a 2-day retention", got)
		}
	})
}

// The identity trim runs after the raw purge committed: a failure there
// keeps the committed count, names its own cutoff (not the raw one), and
// undoes nothing.
func TestPurgeOlderThan_IdentityPurgeFailureAfterCommitIsAttributed(t *testing.T) {
	s := openTestStorage(t)
	ctx := context.Background()
	rawCutoff := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	dedupCutoff := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	if err := s.InsertEvent(ctx, installEvent(t, installA), rawCutoff.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	insertDeliveryIDAt(t, s, 1, dedupCutoff.Add(-time.Hour))
	if _, err := s.db.Exec(`CREATE TRIGGER fail_ids BEFORE DELETE ON runtime_delivery_ids BEGIN SELECT RAISE(ABORT,'induced failure'); END`); err != nil {
		t.Fatal(err)
	}
	count, err := s.PurgeOlderThan(ctx, rawCutoff, dedupCutoff)
	if err == nil || count != 1 || !strings.Contains(err.Error(), "runtime delivery ids before 2026-06-14") || strings.Contains(err.Error(), "2026-06-01") {
		t.Fatalf("PurgeOlderThan = %d, %v; want count 1 and an error naming the identity purge and its cutoff", count, err)
	}
	if remaining, _ := s.eventsOnDay(ctx, rawCutoff.Add(-time.Hour)); len(remaining) != 0 || countDeliveryIDs(t, s) != 1 {
		t.Fatalf("raw purge must stay committed and the identity must survive: %d events, %d ids", len(remaining), countDeliveryIDs(t, s))
	}
}

// A dedup window under one day would purge every identity on every run and
// silently disable replay rejection, so it is refused before any purge.
func TestRunMaintenance_RejectsNonPositiveDedupDays(t *testing.T) {
	now := time.Date(2026, 6, 10, 3, 0, 0, 0, time.UTC)
	for _, days := range []int{0, -1} {
		s := openTestStorage(t)
		insertDeliveryIDAt(t, s, 1, now.Add(-time.Minute))
		if err := RunMaintenance(context.Background(), s, now, 90, days); err == nil || countDeliveryIDs(t, s) != 1 {
			t.Fatalf("runtimeDedupDays=%d: err=%v, ids=%d; want an error and the identity kept", days, err, countDeliveryIDs(t, s))
		}
	}
}
