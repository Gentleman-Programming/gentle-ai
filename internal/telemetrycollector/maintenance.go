package telemetrycollector

import (
	"context"
	"fmt"
	"time"
)

// RunMaintenance performs one cycle of the collector's daily job: it rolls
// up every UTC day from the last rolled day (or the oldest raw event, if
// nothing has ever been rolled up) through yesterday — catching up in one
// run after the process was down for a while — and purges legacy raw events and
// whole runtime deliveries older than retentionDays, and runtime delivery
// identities (the --runtime-store=metrics dedup table) older than
// runtimeDedupDays, never longer than retentionDays. Runtime-only databases
// still reach purge when the legacy rollup range is empty. Runtime observations
// are not rolled up here; their age is server receipt time, not activity time.
//
// Each day's rollup runs with context.WithoutCancel(ctx), so a cancelled
// ctx (e.g. SIGTERM) never interrupts a day already in progress: that day's
// transaction always either completes and commits, or never starts. ctx
// itself is checked only between days, so cancellation stops the loop
// before the next day starts rather than mid-day. The next run resumes
// exactly where this one left off, since lastRolledDay only ever reflects
// committed days.
func RunMaintenance(ctx context.Context, s *Storage, now time.Time, retentionDays, runtimeDedupDays int) error {
	if runtimeDedupDays < 1 { // would purge every identity each run and disable replay rejection
		return fmt.Errorf("runtime dedup days must be at least 1, got %d", runtimeDedupDays)
	}
	yesterday := truncateToDay(now.UTC()).AddDate(0, 0, -1)

	start, err := s.rollupStartDay(ctx, yesterday)
	if err != nil {
		return fmt.Errorf("determine rollup start day: %w", err)
	}

	for day := start; !day.After(yesterday); day = day.AddDate(0, 0, 1) {
		if err := s.RunDailyRollup(context.WithoutCancel(ctx), day); err != nil {
			return fmt.Errorf("roll up %s: %w", day.Format(dayLayout), err)
		}
		if ctx.Err() != nil {
			// Stop between days; the next run resumes at day+1.
			return nil
		}
	}

	cutoff := truncateToDay(now.UTC()).AddDate(0, 0, -retentionDays)
	dedupCutoff := truncateToDay(now.UTC()).AddDate(0, 0, -runtimeDedupDays)
	if dedupCutoff.Before(cutoff) {
		dedupCutoff = cutoff
	}
	if _, err := s.PurgeOlderThan(ctx, cutoff, dedupCutoff); err != nil {
		// PurgeOlderThan already names which purge failed and its cutoff.
		return fmt.Errorf("purge: %w", err)
	}

	return nil
}

// rollupStartDay is the first day RunMaintenance should roll up: the day
// after the last one already rolled, or the oldest raw event's day if
// nothing has been rolled up yet, or yesterday+1 (an empty range) if there
// is no data at all.
func (s *Storage) rollupStartDay(ctx context.Context, yesterday time.Time) (time.Time, error) {
	if last, ok, err := s.lastRolledDay(ctx); err != nil {
		return time.Time{}, err
	} else if ok {
		return last.AddDate(0, 0, 1), nil
	}

	if oldest, ok, err := s.oldestEventDay(ctx); err != nil {
		return time.Time{}, err
	} else if ok {
		return oldest, nil
	}

	return yesterday.AddDate(0, 0, 1), nil
}
