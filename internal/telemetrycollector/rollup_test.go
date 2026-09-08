package telemetrycollector

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func installEvent(t *testing.T, installID string) Event {
	t.Helper()
	body := `{
		"schema": "gentle-ai.telemetry-event/v1",
		"event": "install",
		"install_id": "` + installID + `",
		"sent_at": "2026-01-01T00:00:00Z",
		"version": "1.0.0",
		"os": "linux",
		"arch": "amd64",
		"agents": ["claude-code"],
		"components": ["sdd"],
		"rdd_enabled": true
	}`
	return mustParse(t, body)
}

func heartbeatEvent(t *testing.T, installID, version string, agents []string, rddEnabled bool) Event {
	t.Helper()
	agentsJSON := `[`
	for i, a := range agents {
		if i > 0 {
			agentsJSON += ","
		}
		agentsJSON += `"` + a + `"`
	}
	agentsJSON += `]`
	body := `{
		"schema": "gentle-ai.telemetry-event/v1",
		"event": "heartbeat",
		"install_id": "` + installID + `",
		"sent_at": "2026-01-01T00:00:00Z",
		"version": "` + version + `",
		"os": "linux",
		"arch": "amd64",
		"agents": ` + agentsJSON + `,
		"components": ["sdd"],
		"rdd_enabled": ` + boolLiteral(rddEnabled) + `,
		"counters": {"syncs":1,"sdd_phase_runs":0,"reviews_approved":0,"reviews_correction":0,"reviews_escalated":0}
	}`
	return mustParse(t, body)
}

func boolLiteral(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

const (
	installA = "11111111-1111-4111-8111-111111111111"
	installB = "22222222-2222-4222-8222-222222222222"
	installC = "33333333-3333-4333-8333-333333333333"
)

func TestComputeDayStats_UsesLatestEventPerInstallForAttributes(t *testing.T) {
	morning := time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC)
	evening := time.Date(2026, 5, 1, 20, 0, 0, 0, time.UTC)

	events := []storedEvent{
		{ReceivedAt: morning, InstallID: installA, Version: "1.0.0", Agents: []string{"claude-code"}, Components: []string{"sdd"}, RDDEnabled: false},
		{ReceivedAt: evening, InstallID: installA, Version: "1.1.0", Agents: []string{"claude-code", "opencode"}, Components: []string{"sdd", "engram"}, RDDEnabled: true},
	}

	stats := computeDayStats(events)

	if _, active := stats.activeInstalls[installA]; !active {
		t.Fatal("expected install A to be active")
	}
	if len(stats.activeInstalls) != 1 {
		t.Errorf("activeInstalls = %d, want 1 (one install, two events)", len(stats.activeInstalls))
	}
	if stats.versionCounts["1.1.0"] != 1 || stats.versionCounts["1.0.0"] != 0 {
		t.Errorf("versionCounts should reflect the latest event only: %+v", stats.versionCounts)
	}
	if stats.agentCounts["opencode"] != 1 {
		t.Errorf("agentCounts[opencode] = %d, want 1 from the latest event", stats.agentCounts["opencode"])
	}
	if stats.rddTrue != 1 || stats.rddFalse != 0 {
		t.Errorf("rdd counts should reflect the latest event: true=%d false=%d", stats.rddTrue, stats.rddFalse)
	}
}

func TestComputeDayStats_DedupesRepeatedAgentsWithinOneEvent(t *testing.T) {
	events := []storedEvent{
		{ReceivedAt: time.Now(), InstallID: installA, Agents: []string{"claude-code", "claude-code"}, Components: nil},
	}
	stats := computeDayStats(events)
	if stats.agentCounts["claude-code"] != 1 {
		t.Errorf("agentCounts[claude-code] = %d, want 1 (deduped within the event)", stats.agentCounts["claude-code"])
	}
}

func TestRunDailyRollup_IsIdempotent(t *testing.T) {
	s := openTestStorage(t)
	ctx := context.Background()
	day := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	if err := s.InsertEvent(ctx, installEvent(t, installA), day.Add(9*time.Hour)); err != nil {
		t.Fatalf("InsertEvent: %v", err)
	}
	if err := s.InsertEvent(ctx, installEvent(t, installB), day.Add(10*time.Hour)); err != nil {
		t.Fatalf("InsertEvent: %v", err)
	}

	if err := s.RunDailyRollup(ctx, day); err != nil {
		t.Fatalf("RunDailyRollup (1st run): %v", err)
	}
	first, err := s.rollupRows(ctx, metricActiveInstall, "2026-05-01", "2026-05-01")
	if err != nil {
		t.Fatalf("rollupRows: %v", err)
	}

	if err := s.RunDailyRollup(ctx, day); err != nil {
		t.Fatalf("RunDailyRollup (2nd run): %v", err)
	}
	second, err := s.rollupRows(ctx, metricActiveInstall, "2026-05-01", "2026-05-01")
	if err != nil {
		t.Fatalf("rollupRows: %v", err)
	}

	if len(first) != 2 || len(second) != 2 {
		t.Fatalf("rollup rows: first=%d second=%d, want 2 both times", len(first), len(second))
	}
}

// TestBuildSummary_MonthlyUniqueInstalls covers the issue's core rollup-math
// requirement: unique installs per month, computed across a fixture
// spanning multiple days, installs, and a month boundary.
func TestBuildSummary_MonthlyUniqueInstalls(t *testing.T) {
	s := openTestStorage(t)
	ctx := context.Background()

	april1 := time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC)
	april2 := time.Date(2026, 4, 2, 9, 0, 0, 0, time.UTC)
	may1 := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)

	// April: A and B active (on different days). May: B and C active.
	mustInsertAndRoll(t, s, ctx, installEvent(t, installA), april1)
	mustInsertAndRoll(t, s, ctx, installEvent(t, installB), april2)
	mustInsertAndRoll(t, s, ctx, installEvent(t, installB), may1)
	mustInsertAndRoll(t, s, ctx, installEvent(t, installC), may1)

	now := time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)
	summary, err := BuildSummary(ctx, s, now)
	if err != nil {
		t.Fatalf("BuildSummary: %v", err)
	}

	got := monthlyCountByLabel(summary.InstallsPerMonth)
	if got["2026-04"] != 2 {
		t.Errorf("2026-04 unique installs = %d, want 2 (A, B)", got["2026-04"])
	}
	if got["2026-05"] != 2 {
		t.Errorf("2026-05 unique installs = %d, want 2 (B, C)", got["2026-05"])
	}
	if len(summary.InstallsPerMonth) != 12 {
		t.Errorf("InstallsPerMonth has %d entries, want 12", len(summary.InstallsPerMonth))
	}
}

// TestBuildSummary_WeeklyActiveInstalls exercises the weekly-active
// rollup math across an ISO week boundary.
func TestBuildSummary_WeeklyActiveInstalls(t *testing.T) {
	s := openTestStorage(t)
	ctx := context.Background()

	anchor := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	week0 := startOfISOWeek(anchor)
	week1 := week0.AddDate(0, 0, 7)

	mustInsertAndRoll(t, s, ctx, installEvent(t, installA), week0.Add(9*time.Hour))
	mustInsertAndRoll(t, s, ctx, installEvent(t, installB), week0.AddDate(0, 0, 2).Add(9*time.Hour))
	mustInsertAndRoll(t, s, ctx, installEvent(t, installC), week1.AddDate(0, 0, 1).Add(9*time.Hour))

	now := week1.AddDate(0, 0, 6)
	summary, err := BuildSummary(ctx, s, now)
	if err != nil {
		t.Fatalf("BuildSummary: %v", err)
	}

	got := weeklyCountByLabel(summary.WeeklyActiveInstalls)
	year0, isoWeek0 := week0.ISOWeek()
	year1, isoWeek1 := week1.ISOWeek()
	label0 := isoLabel(year0, isoWeek0)
	label1 := isoLabel(year1, isoWeek1)

	if got[label0] != 2 {
		t.Errorf("%s active installs = %d, want 2 (A, B)", label0, got[label0])
	}
	if got[label1] != 1 {
		t.Errorf("%s active installs = %d, want 1 (C)", label1, got[label1])
	}
}

// TestBuildSummary_MergesTodaysUnrolledEvents verifies the summary reads
// today's not-yet-rolled-up events directly, per the design ("computed from
// rollups_daily plus today's raw events").
func TestBuildSummary_MergesTodaysUnrolledEvents(t *testing.T) {
	s := openTestStorage(t)
	ctx := context.Background()

	now := time.Date(2026, 6, 10, 15, 0, 0, 0, time.UTC)
	if err := s.InsertEvent(ctx, installEvent(t, installA), now); err != nil {
		t.Fatalf("InsertEvent: %v", err)
	}
	// Deliberately do not call RunDailyRollup for "now"'s day.

	summary, err := BuildSummary(ctx, s, now)
	if err != nil {
		t.Fatalf("BuildSummary: %v", err)
	}

	got := monthlyCountByLabel(summary.InstallsPerMonth)
	if got["2026-06"] != 1 {
		t.Errorf("2026-06 unique installs = %d, want 1 from today's unrolled event", got["2026-06"])
	}
	if summary.AgentDistribution["claude-code"] != 1 {
		t.Errorf("AgentDistribution[claude-code] = %d, want 1 from today's unrolled event", summary.AgentDistribution["claude-code"])
	}
}

func TestBuildSummary_RDDEnabledRatio(t *testing.T) {
	s := openTestStorage(t)
	ctx := context.Background()

	day := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	mustInsertAndRoll(t, s, ctx, heartbeatEvent(t, installA, "1.0.0", []string{"claude-code"}, true), day)
	mustInsertAndRoll(t, s, ctx, heartbeatEvent(t, installB, "1.0.0", []string{"claude-code"}, true), day)
	mustInsertAndRoll(t, s, ctx, heartbeatEvent(t, installC, "1.0.0", []string{"claude-code"}, false), day)

	now := day.AddDate(0, 0, 1)
	summary, err := BuildSummary(ctx, s, now)
	if err != nil {
		t.Fatalf("BuildSummary: %v", err)
	}

	const want = 2.0 / 3.0
	if diff := summary.RDDEnabledRatio - want; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("RDDEnabledRatio = %v, want %v", summary.RDDEnabledRatio, want)
	}
}

func mustInsertAndRoll(t *testing.T, s *Storage, ctx context.Context, ev Event, receivedAt time.Time) {
	t.Helper()
	if err := s.InsertEvent(ctx, ev, receivedAt); err != nil {
		t.Fatalf("InsertEvent: %v", err)
	}
	if err := s.RunDailyRollup(ctx, receivedAt); err != nil {
		t.Fatalf("RunDailyRollup: %v", err)
	}
}

func monthlyCountByLabel(counts []MonthlyCount) map[string]int {
	out := make(map[string]int, len(counts))
	for _, c := range counts {
		out[c.Month] = c.UniqueInstalls
	}
	return out
}

func weeklyCountByLabel(counts []WeeklyCount) map[string]int {
	out := make(map[string]int, len(counts))
	for _, c := range counts {
		out[c.Week] = c.ActiveInstalls
	}
	return out
}

func isoLabel(year, week int) string {
	return fmt.Sprintf("%04d-W%02d", year, week)
}
