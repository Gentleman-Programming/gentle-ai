package telemetrycollector

import (
	"context"
	"fmt"
	"time"
)

// distributionWindowDays bounds the "current usage" window for
// agent/component/version/rdd_enabled distributions: the last 30 days of
// daily install snapshots. This is a design choice, not part of the wire
// contract: unlike the monthly/weekly install counts (which the issue asks
// for explicitly, over 12 months / 12 weeks), the contract leaves the
// distribution window unspecified. Thirty days keeps the answer close to
// "who's using it now" without needing a second rollups table.
//
// These distributions count install-days, not distinct installs: an
// install active on 30 different days within the window contributes to the
// count 30 times, once per day it reported that attribute. This matches
// how the values are produced (a per-day rollup keyed by install) and is
// documented in docs/telemetry-collector.md.
const distributionWindowDays = 30

// MonthlyCount is one month's worth of unique active installs.
type MonthlyCount struct {
	Month          string `json:"month"`
	UniqueInstalls int    `json:"unique_installs"`
}

// WeeklyCount is one ISO week's worth of active installs.
type WeeklyCount struct {
	Week           string `json:"week"`
	ActiveInstalls int    `json:"active_installs"`
}

// NpmDownloads is one npm package's download counts.
type NpmDownloads struct {
	LastDay    int64 `json:"last_day"`
	Last30Days int64 `json:"last_30_days"`
}

// Downloads holds external, public download counts (npm, GitHub release
// assets) — not telemetry from any gentle-ai install.
type Downloads struct {
	Npm    map[string]NpmDownloads `json:"npm"`
	Github map[string]int64        `json:"github"`
}

// Summary is the JSON body returned by GET /v1/summary.
type Summary struct {
	GeneratedAt           time.Time        `json:"generated_at"`
	InstallsPerMonth      []MonthlyCount   `json:"installs_per_month"`
	WeeklyActiveInstalls  []WeeklyCount    `json:"weekly_active_installs"`
	AgentDistribution     map[string]int64 `json:"agent_distribution"`
	ComponentDistribution map[string]int64 `json:"component_distribution"`
	VersionDistribution   map[string]int64 `json:"version_distribution"`
	RDDEnabledRatio       float64          `json:"rdd_enabled_ratio"`
	Downloads             Downloads        `json:"downloads"`
}

// BuildSummary computes the summary as of now, reading historical days from
// rollups_daily and today's not-yet-rolled-up day directly from the events
// table.
func BuildSummary(ctx context.Context, s *Storage, now time.Time) (Summary, error) {
	now = now.UTC()
	today := truncateToDay(now)

	todayEvents, err := s.eventsOnDay(ctx, today)
	if err != nil {
		return Summary{}, fmt.Errorf("load today's events: %w", err)
	}
	todayStats := computeDayStats(todayEvents)

	monthly, err := s.monthlyInstalls(ctx, today, todayStats, 12)
	if err != nil {
		return Summary{}, err
	}
	weekly, err := s.weeklyActiveInstalls(ctx, today, todayStats, 12)
	if err != nil {
		return Summary{}, err
	}

	agents, err := s.distribution(ctx, metricAgent, today, todayStats.agentCounts)
	if err != nil {
		return Summary{}, err
	}
	components, err := s.distribution(ctx, metricComponent, today, todayStats.componentCount)
	if err != nil {
		return Summary{}, err
	}
	versions, err := s.distribution(ctx, metricVersion, today, todayStats.versionCounts)
	if err != nil {
		return Summary{}, err
	}
	rdd, err := s.distribution(ctx, metricRDDEnabled, today, map[string]int64{
		rddTrueKey:  todayStats.rddTrue,
		rddFalseKey: todayStats.rddFalse,
	})
	if err != nil {
		return Summary{}, err
	}
	ratio := 0.0
	if total := rdd[rddTrueKey] + rdd[rddFalseKey]; total > 0 {
		ratio = float64(rdd[rddTrueKey]) / float64(total)
	}

	downloads, err := s.downloadsSummary(ctx, today)
	if err != nil {
		return Summary{}, err
	}

	return Summary{
		GeneratedAt:           now,
		InstallsPerMonth:      monthly,
		WeeklyActiveInstalls:  weekly,
		AgentDistribution:     agents,
		ComponentDistribution: components,
		VersionDistribution:   versions,
		RDDEnabledRatio:       ratio,
		Downloads:             downloads,
	}, nil
}

func truncateToDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// monthlyInstalls counts distinct install ids active in each of the last n
// calendar months (UTC), including the current, partial month.
func (s *Storage) monthlyInstalls(ctx context.Context, today time.Time, todayStats dayStats, n int) ([]MonthlyCount, error) {
	firstOfCurrentMonth := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)

	out := make([]MonthlyCount, 0, n)
	for i := n - 1; i >= 0; i-- {
		monthStart := addMonths(firstOfCurrentMonth, -i)
		monthEnd := addMonths(monthStart, 1) // exclusive

		installs := map[string]struct{}{}
		fromDay := monthStart.Format(dayLayout)
		toDay := monthEnd.Add(-24 * time.Hour).Format(dayLayout)
		rows, err := s.rollupRows(ctx, metricActiveInstall, fromDay, toDay)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			installs[r.Key] = struct{}{}
		}
		if !today.Before(monthStart) && today.Before(monthEnd) {
			for id := range todayStats.activeInstalls {
				installs[id] = struct{}{}
			}
		}

		out = append(out, MonthlyCount{
			Month:          monthStart.Format("2006-01"),
			UniqueInstalls: len(installs),
		})
	}
	return out, nil
}

// weeklyActiveInstalls counts distinct install ids active in each of the
// last n ISO (Monday-start) weeks, including the current, partial week.
func (s *Storage) weeklyActiveInstalls(ctx context.Context, today time.Time, todayStats dayStats, n int) ([]WeeklyCount, error) {
	currentWeekStart := startOfISOWeek(today)

	out := make([]WeeklyCount, 0, n)
	for i := n - 1; i >= 0; i-- {
		weekStart := currentWeekStart.AddDate(0, 0, -7*i)
		weekEnd := weekStart.AddDate(0, 0, 7) // exclusive

		installs := map[string]struct{}{}
		fromDay := weekStart.Format(dayLayout)
		toDay := weekEnd.Add(-24 * time.Hour).Format(dayLayout)
		rows, err := s.rollupRows(ctx, metricActiveInstall, fromDay, toDay)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			installs[r.Key] = struct{}{}
		}
		if !today.Before(weekStart) && today.Before(weekEnd) {
			for id := range todayStats.activeInstalls {
				installs[id] = struct{}{}
			}
		}

		year, week := weekStart.ISOWeek()
		out = append(out, WeeklyCount{
			Week:           fmt.Sprintf("%04d-W%02d", year, week),
			ActiveInstalls: len(installs),
		})
	}
	return out, nil
}

// distribution sums a metric's counts over the trailing
// distributionWindowDays (excluding today, which is merged in from live
// stats) and returns it keyed by rollup key.
func (s *Storage) distribution(ctx context.Context, metric string, today time.Time, todayCounts map[string]int64) (map[string]int64, error) {
	windowStart := today.AddDate(0, 0, -(distributionWindowDays - 1))
	fromDay := windowStart.Format(dayLayout)
	toDay := today.Add(-24 * time.Hour).Format(dayLayout)

	out := map[string]int64{}
	if !rangeIsEmpty(fromDay, toDay) {
		rows, err := s.rollupRows(ctx, metric, fromDay, toDay)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out[r.Key] += r.Value
		}
	}
	for key, count := range todayCounts {
		out[key] += count
	}
	return out, nil
}

// rangeIsEmpty reports whether fromDay is after toDay (i.e. the range is
// empty, which happens on the very first day of the collector's life when
// "yesterday" predates any data).
func rangeIsEmpty(fromDay, toDay string) bool {
	return fromDay > toDay
}

// downloadsSummary reads the external npm/GitHub download counts written
// by FetchAndStoreDownloads. npm's last_day is the most recent day fetched
// for that package (unbounded lookback: the fetch runs daily, so this is
// normally very recent); last_30_days sums the trailing 30 days actually
// present. GitHub's totals are the single most recent cumulative snapshot
// per tag — summing across days would double-count, since each snapshot
// already includes everything before it.
func (s *Storage) downloadsSummary(ctx context.Context, today time.Time) (Downloads, error) {
	todayKey := today.Format(dayLayout)

	npmLatest, err := s.latestRollupPerKey(ctx, metricNpmDownloadsDay, todayKey)
	if err != nil {
		return Downloads{}, err
	}
	windowStart := today.AddDate(0, 0, -(distributionWindowDays - 1)).Format(dayLayout)
	npm := make(map[string]NpmDownloads, len(npmLatest))
	if !rangeIsEmpty(windowStart, todayKey) {
		rows, err := s.rollupRows(ctx, metricNpmDownloadsDay, windowStart, todayKey)
		if err != nil {
			return Downloads{}, err
		}
		sums := map[string]int64{}
		for _, r := range rows {
			sums[r.Key] += r.Value
		}
		for pkg, lastDay := range npmLatest {
			npm[pkg] = NpmDownloads{LastDay: lastDay, Last30Days: sums[pkg]}
		}
	}

	github, err := s.latestRollupPerKey(ctx, metricGithubReleaseDownloads, todayKey)
	if err != nil {
		return Downloads{}, err
	}

	return Downloads{Npm: npm, Github: github}, nil
}

func addMonths(t time.Time, delta int) time.Time {
	return time.Date(t.Year(), t.Month()+time.Month(delta), 1, 0, 0, 0, 0, time.UTC)
}

func startOfISOWeek(t time.Time) time.Time {
	weekday := int(t.Weekday())
	offset := weekday - 1
	if weekday == 0 { // Sunday
		offset = 6
	}
	return t.AddDate(0, 0, -offset)
}
