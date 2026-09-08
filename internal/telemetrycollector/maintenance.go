package telemetrycollector

import (
	"context"
	"fmt"
	"time"
)

// RunMaintenance performs one cycle of the collector's daily job: it rolls
// up every UTC day from the last rolled day (or the oldest raw event, if
// nothing has ever been rolled up) through yesterday — catching up in one
// run after the process was down for a while — and purges raw events older
// than retentionDays.
//
// Each day's rollup runs with context.WithoutCancel(ctx), so a cancelled
// ctx (e.g. SIGTERM) never interrupts a day already in progress: that day's
// transaction always either completes and commits, or never starts. ctx
// itself is checked only between days, so cancellation stops the loop
// before the next day starts rather than mid-day. The next run resumes
// exactly where this one left off, since lastRolledDay only ever reflects
// committed days.
func RunMaintenance(ctx context.Context, s *Storage, now time.Time, retentionDays int) error {
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
	if _, err := s.PurgeOlderThan(ctx, cutoff); err != nil {
		return fmt.Errorf("purge events before %s: %w", cutoff.Format(dayLayout), err)
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
