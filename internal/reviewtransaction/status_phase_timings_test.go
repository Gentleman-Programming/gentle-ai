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

	wantFresh := "status_phase_timings: repository_identity_resolution=[not measured] snapshot_construction=[not measured] implicit_base_range_derivation=[not measured] predecessor_consumption_ancestry=[not measured]"
	if s := NewStatusPhaseTimings().Snapshot().FormatSummary(); s != wantFresh {
		t.Fatalf("fresh summary = %q, want %q", s, wantFresh)
	}

	zeroDur := NewStatusPhaseTimings()
	zeroDur.Start(StatusPhaseRepositoryIdentityResolution)()
	if !strings.Contains(zeroDur.Snapshot().FormatSummary(), "repository_identity_resolution=0ms") {
		t.Fatalf("measured 0ms phase must render =0ms: %s", zeroDur.Snapshot().FormatSummary())
	}

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

	if sum := timings.Snapshot().FormatSummary(); !strings.Contains(sum, "snapshot_construction=[not measured]") || !strings.Contains(sum, "predecessor_consumption_ancestry=30ms") {
		t.Fatalf("expected unmeasured nested and measured outer phases in summary: %s", sum)
	}

	for _, p := range timings.Snapshot().Phases {
		if (p.Phase == StatusPhaseSnapshotConstruction && p.Count != 0) || (p.Phase == StatusPhasePredecessorConsumptionAncestry && p.DurationMs != 30) {
			t.Fatalf("phase timing mismatch: %+v", p)
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
