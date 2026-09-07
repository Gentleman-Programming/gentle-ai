package telemetrycollector

import (
	"context"
	"fmt"
	"time"
)

// Rollup metric names stored in rollups_daily.key alongside a per-day
// value. "active_install" rows use the install id itself as the key (value
// is always 1) so distinct installs can be counted across a date range
// without re-reading raw events; the other metrics aggregate counts of
// distinct installs reporting that attribute on that day.
const (
	metricActiveInstall = "active_install"
	metricAgent         = "agent"
	metricComponent     = "component"
	metricVersion       = "version"
	metricRDDEnabled    = "rdd_enabled"
)

const (
	rddTrueKey  = "true"
	rddFalseKey = "false"
)

// dayStats is the aggregation of one UTC calendar day's events, computed
// identically whether the day is being rolled up for storage (RunDailyRollup)
// or read live for "today" in the summary endpoint (see summary.go).
type dayStats struct {
	activeInstalls map[string]struct{}
	agentCounts    map[string]int64
	componentCount map[string]int64
	versionCounts  map[string]int64
	rddTrue        int64
	rddFalse       int64
}

func newDayStats() dayStats {
	return dayStats{
		activeInstalls: map[string]struct{}{},
		agentCounts:    map[string]int64{},
		componentCount: map[string]int64{},
		versionCounts:  map[string]int64{},
	}
}

// computeDayStats groups events by install id, treats any event as making
// that install "active" for the day, and uses each install's latest event
// of the day as the snapshot for agent/component/version/rdd_enabled
// distribution — so an install that upgraded mid-day is counted once, under
// its newest reported attributes.
func computeDayStats(events []storedEvent) dayStats {
	stats := newDayStats()

	latestByInstall := make(map[string]storedEvent, len(events))
	for _, ev := range events {
		stats.activeInstalls[ev.InstallID] = struct{}{}
		current, ok := latestByInstall[ev.InstallID]
		if !ok || ev.ReceivedAt.After(current.ReceivedAt) {
			latestByInstall[ev.InstallID] = ev
		}
	}

	for _, ev := range latestByInstall {
		for _, agent := range dedupe(ev.Agents) {
			stats.agentCounts[agent]++
		}
		for _, component := range dedupe(ev.Components) {
			stats.componentCount[component]++
		}
		stats.versionCounts[ev.Version]++
		if ev.RDDEnabled {
			stats.rddTrue++
		} else {
			stats.rddFalse++
		}
	}

	return stats
}

func dedupe(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// RunDailyRollup computes and persists the rollup for one UTC calendar day,
// replacing any previously computed rollup for that day. It is idempotent:
// running it twice for the same day yields the same rows. The delete and
// every upsert run inside one transaction, committed only at the end, so a
// failure or a cancelled ctx (see RunMaintenance) leaves the day's rollup
// entirely absent rather than partially rewritten.
func (s *Storage) RunDailyRollup(ctx context.Context, day time.Time) error {
	events, err := s.eventsOnDay(ctx, day)
	if err != nil {
		return fmt.Errorf("load events for rollup: %w", err)
	}
	stats := computeDayStats(events)
	dayKey := day.UTC().Format(dayLayout)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin rollup transaction for %s: %w", dayKey, err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op once Commit has succeeded

	if err := deleteRollupsForDay(ctx, tx, dayKey); err != nil {
		return err
	}
	for installID := range stats.activeInstalls {
		if err := upsertRollup(ctx, tx, dayKey, metricActiveInstall, installID, 1); err != nil {
			return err
		}
	}
	for agent, count := range stats.agentCounts {
		if err := upsertRollup(ctx, tx, dayKey, metricAgent, agent, count); err != nil {
			return err
		}
	}
	for component, count := range stats.componentCount {
		if err := upsertRollup(ctx, tx, dayKey, metricComponent, component, count); err != nil {
			return err
		}
	}
	for version, count := range stats.versionCounts {
		if err := upsertRollup(ctx, tx, dayKey, metricVersion, version, count); err != nil {
			return err
		}
	}
	if stats.rddTrue > 0 {
		if err := upsertRollup(ctx, tx, dayKey, metricRDDEnabled, rddTrueKey, stats.rddTrue); err != nil {
			return err
		}
	}
	if stats.rddFalse > 0 {
		if err := upsertRollup(ctx, tx, dayKey, metricRDDEnabled, rddFalseKey, stats.rddFalse); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit rollup transaction for %s: %w", dayKey, err)
	}
	return nil
}
