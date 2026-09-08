package telemetrycollector

import (
	"context"
	"testing"
	"time"
)

func TestRunMaintenance_RollsUpYesterdayAndPurgesOldEvents(t *testing.T) {
	s := openTestStorage(t)
	ctx := context.Background()

	now := time.Date(2026, 6, 10, 3, 0, 0, 0, time.UTC)
	yesterday := now.AddDate(0, 0, -1)
	old := now.AddDate(0, 0, -200)

	if err := s.InsertEvent(ctx, installEvent(t, installA), yesterday); err != nil {
		t.Fatalf("InsertEvent(yesterday): %v", err)
	}
	if err := s.InsertEvent(ctx, installEvent(t, installB), old); err != nil {
		t.Fatalf("InsertEvent(old): %v", err)
	}

	if err := RunMaintenance(ctx, s, now, 90); err != nil {
		t.Fatalf("RunMaintenance: %v", err)
	}

	rows, err := s.rollupRows(ctx, metricActiveInstall, yesterday.Format(dayLayout), yesterday.Format(dayLayout))
	if err != nil {
		t.Fatalf("rollupRows: %v", err)
	}
	if len(rows) != 1 || rows[0].Key != installA {
		t.Errorf("rollup for yesterday = %+v, want one row for installA", rows)
	}

	remainingOld, err := s.eventsOnDay(ctx, old)
	if err != nil {
		t.Fatalf("eventsOnDay(old): %v", err)
	}
	if len(remainingOld) != 0 {
		t.Errorf("old event should have been purged by the 90-day retention, got %d rows", len(remainingOld))
	}

	remainingYesterday, err := s.eventsOnDay(ctx, yesterday)
	if err != nil {
		t.Fatalf("eventsOnDay(yesterday): %v", err)
	}
	if len(remainingYesterday) != 1 {
		t.Errorf("yesterday's event should survive retention, got %d rows", len(remainingYesterday))
	}
}

func TestRunMaintenance_CatchesUpMultipleUnrolledDays(t *testing.T) {
	s := openTestStorage(t)
	ctx := context.Background()

	day1 := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 5, 2, 9, 0, 0, 0, time.UTC)
	day3 := time.Date(2026, 5, 3, 9, 0, 0, 0, time.UTC)
	now := time.Date(2026, 5, 4, 3, 0, 0, 0, time.UTC) // yesterday = day3; nothing rolled up yet

	if err := s.InsertEvent(ctx, installEvent(t, installA), day1); err != nil {
		t.Fatalf("InsertEvent(day1): %v", err)
	}
	if err := s.InsertEvent(ctx, installEvent(t, installB), day2); err != nil {
		t.Fatalf("InsertEvent(day2): %v", err)
	}
	if err := s.InsertEvent(ctx, installEvent(t, installC), day3); err != nil {
		t.Fatalf("InsertEvent(day3): %v", err)
	}

	if err := RunMaintenance(ctx, s, now, 90); err != nil {
		t.Fatalf("RunMaintenance: %v", err)
	}

	for _, day := range []time.Time{day1, day2, day3} {
		rows, err := s.rollupRows(ctx, metricActiveInstall, day.Format(dayLayout), day.Format(dayLayout))
		if err != nil {
			t.Fatalf("rollupRows(%s): %v", day.Format(dayLayout), err)
		}
		if len(rows) != 1 {
			t.Errorf("day %s: rollup rows = %d, want 1 (a single maintenance run should catch up every unrolled day)", day.Format(dayLayout), len(rows))
		}
	}
}

// cancelAfterCalls reports itself as cancelled starting from its
// (cancelAfter+1)th Err() call. It is used to simulate a shutdown signal
// arriving between two days of RunMaintenance's catch-up loop: the loop
// checks ctx.Err() only between days, so this lets the test observe that
// boundary deterministically instead of racing a real signal.
type cancelAfterCalls struct {
	context.Context
	cancelAfter int
	calls       int
}

func (c *cancelAfterCalls) Err() error {
	c.calls++
	if c.calls > c.cancelAfter {
		return context.Canceled
	}
	return nil
}

func TestRunMaintenance_CancelBetweenDaysLeavesEarlierDaysCompleteAndLaterAbsent(t *testing.T) {
	s := openTestStorage(t)
	bg := context.Background()

	day1 := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 5, 2, 9, 0, 0, 0, time.UTC)
	day3 := time.Date(2026, 5, 3, 9, 0, 0, 0, time.UTC)
	now := time.Date(2026, 5, 4, 3, 0, 0, 0, time.UTC) // yesterday = day3; nothing rolled up yet

	if err := s.InsertEvent(bg, installEvent(t, installA), day1); err != nil {
		t.Fatalf("InsertEvent(day1): %v", err)
	}
	if err := s.InsertEvent(bg, installEvent(t, installB), day2); err != nil {
		t.Fatalf("InsertEvent(day2): %v", err)
	}
	if err := s.InsertEvent(bg, installEvent(t, installC), day3); err != nil {
		t.Fatalf("InsertEvent(day3): %v", err)
	}

	// Reports cancelled right after day1's rollup commits, before the
	// between-days check for day2. context.WithoutCancel inside
	// RunDailyRollup means this override never reaches day1's own
	// transaction, so day1 completes and commits regardless.
	ctx := &cancelAfterCalls{Context: bg, cancelAfter: 0}
	if err := RunMaintenance(ctx, s, now, 90); err != nil {
		t.Fatalf("RunMaintenance: %v", err)
	}

	rolledUp := func(day time.Time) int {
		rows, err := s.rollupRows(bg, metricActiveInstall, day.Format(dayLayout), day.Format(dayLayout))
		if err != nil {
			t.Fatalf("rollupRows(%s): %v", day.Format(dayLayout), err)
		}
		return len(rows)
	}

	if rolledUp(day1) != 1 {
		t.Error("day1 should be complete: it finished before cancellation was observed")
	}
	if rolledUp(day2) != 0 || rolledUp(day3) != 0 {
		t.Error("day2 and day3 should be entirely absent, never partially rolled up")
	}
}
