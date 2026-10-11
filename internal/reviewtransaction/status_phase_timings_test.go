package reviewtransaction

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestStatusPhaseTimings(t *testing.T) {
	timings := NewStatusPhaseTimings()
	fakeNow := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	timings.now = func() time.Time { return fakeNow }

	for _, step := range []struct {
		phase StatusPhase
		dur   time.Duration
	}{
		{StatusPhaseRepositoryIdentityResolution, 15 * time.Millisecond},
		{StatusPhaseSnapshotConstruction, 40 * time.Millisecond},
		{StatusPhaseImplicitBaseRangeDerivation, 20 * time.Millisecond},
		{StatusPhasePredecessorConsumptionAncestry, 50 * time.Millisecond},
	} {
		stop := timings.Start(step.phase)
		fakeNow = fakeNow.Add(step.dur)
		stop()
	}

	snap := timings.Snapshot()
	wantSummary := "status_phase_timings: repository_identity_resolution=15ms snapshot_construction=40ms implicit_base_range_derivation=20ms predecessor_consumption_ancestry=50ms"
	if s := snap.FormatSummary(); s != wantSummary {
		t.Fatalf("summary = %q, want %q", s, wantSummary)
	}

	_ = timings.Start(StatusPhasePredecessorConsumptionAncestry)
	fakeNow = fakeNow.Add(500 * time.Millisecond)
	if !strings.Contains(timings.Snapshot().FormatSummary(), "predecessor_consumption_ancestry=550ms[in_flight]") {
		t.Fatalf("expected in_flight in summary: %s", timings.Snapshot().FormatSummary())
	}
}

func TestStatusPhaseTimingsOuterAndContext(t *testing.T) {
	timings := NewStatusPhaseTimings()
	fakeNow := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	timings.now = func() time.Time { return fakeNow }
	stopOuter := timings.Start(StatusPhasePredecessorConsumptionAncestry)
	fakeNow = fakeNow.Add(10 * time.Millisecond)
	stopInner := timings.Start(StatusPhaseSnapshotConstruction)
	fakeNow = fakeNow.Add(20 * time.Millisecond)
	stopInner()
	stopOuter()

	for _, p := range timings.Snapshot().Phases {
		if p.Phase == StatusPhaseSnapshotConstruction && p.Count != 0 {
			t.Fatalf("nested phase should not count: %+v", p)
		}
		if p.Phase == StatusPhasePredecessorConsumptionAncestry && p.DurationMs != 30 {
			t.Fatalf("outer phase duration want 30ms, got %+v", p)
		}
	}

	StartStatusPhase(context.Background(), StatusPhaseSnapshotConstruction)()
	ctx := WithStatusPhaseTimings(context.Background(), timings)
	stopCtx := StartStatusPhase(ctx, StatusPhaseRepositoryIdentityResolution)
	fakeNow = fakeNow.Add(5 * time.Millisecond)
	stopCtx()
	if snap := timings.Snapshot(); snap.Phases[0].Count != 1 || snap.Phases[0].DurationMs != 5 {
		t.Fatalf("context start timing mismatch: %+v", snap.Phases[0])
	}
}
