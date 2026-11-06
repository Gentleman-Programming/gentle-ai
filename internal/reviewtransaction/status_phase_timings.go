package reviewtransaction

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// StatusPhase identifies a measured native review status evaluation phase.
type StatusPhase string

const (
	StatusPhaseRepositoryIdentityResolution   StatusPhase = "repository_identity_resolution"
	StatusPhaseSnapshotConstruction           StatusPhase = "snapshot_construction"
	StatusPhaseImplicitBaseRangeDerivation    StatusPhase = "implicit_base_range_derivation"
	StatusPhasePredecessorConsumptionAncestry StatusPhase = "predecessor_consumption_ancestry"
)

var statusPhasesInOrder = []StatusPhase{
	StatusPhaseRepositoryIdentityResolution,
	StatusPhaseSnapshotConstruction,
	StatusPhaseImplicitBaseRangeDerivation,
	StatusPhasePredecessorConsumptionAncestry,
}

// StatusPhaseTiming records the timing measurement for a single status phase.
type StatusPhaseTiming struct {
	Phase      StatusPhase   `json:"phase"`
	Duration   time.Duration `json:"-"`
	DurationMs int64         `json:"duration_ms"`
	Count      int           `json:"count"`
	InFlight   bool          `json:"in_flight,omitempty"`
}

// StatusPhaseTimingSnapshot captures an immutable view of measured status phases.
type StatusPhaseTimingSnapshot struct {
	Phases      []StatusPhaseTiming `json:"phases"`
	ActivePhase StatusPhase         `json:"active_phase,omitempty"`
}

// FormatSummary formats the phase timing snapshot as a concise diagnostic string.
func (s StatusPhaseTimingSnapshot) FormatSummary() string {
	parts := make([]string, len(s.Phases))
	for i, entry := range s.Phases {
		if entry.Count == 0 {
			parts[i] = fmt.Sprintf("%s=[not measured]", entry.Phase)
		} else if entry.InFlight {
			parts[i] = fmt.Sprintf("%s=%dms[in_flight]", entry.Phase, entry.DurationMs)
		} else {
			parts[i] = fmt.Sprintf("%s=%dms", entry.Phase, entry.DurationMs)
		}
	}
	return "status_phase_timings: " + strings.Join(parts, " ")
}

// StatusPhaseTimings collects execution timings across native review status phases.
type StatusPhaseTimings struct {
	mu          sync.Mutex
	active      StatusPhase
	activeStart time.Time
	durations   map[StatusPhase]time.Duration
	counts      map[StatusPhase]int
	now         func() time.Time
}

// NewStatusPhaseTimings constructs an initialized phase timing collector.
func NewStatusPhaseTimings() *StatusPhaseTimings {
	return &StatusPhaseTimings{
		durations: make(map[StatusPhase]time.Duration),
		counts:    make(map[StatusPhase]int),
		now:       time.Now,
	}
}

// Start begins a phase measurement unless an outer phase is already active.
func (t *StatusPhaseTimings) Start(phase StatusPhase) func() {
	if t == nil {
		return func() {}
	}
	t.mu.Lock()
	if t.active != "" {
		t.mu.Unlock()
		return func() {}
	}
	t.active = phase
	t.activeStart = t.now()
	t.counts[phase]++
	t.mu.Unlock()

	return func() {
		t.mu.Lock()
		defer t.mu.Unlock()
		if t.active == phase {
			t.durations[phase] += t.now().Sub(t.activeStart)
			t.active = ""
		}
	}
}

// Snapshot returns an immutable point-in-time snapshot of accumulated timings.
func (t *StatusPhaseTimings) Snapshot() StatusPhaseTimingSnapshot {
	if t == nil {
		return StatusPhaseTimingSnapshot{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.now()
	phases := make([]StatusPhaseTiming, len(statusPhasesInOrder))
	for i, phase := range statusPhasesInOrder {
		dur, inFlight := t.durations[phase], false
		if t.active == phase {
			dur += now.Sub(t.activeStart)
			inFlight = true
		}
		phases[i] = StatusPhaseTiming{Phase: phase, Duration: dur, DurationMs: dur.Milliseconds(), Count: t.counts[phase], InFlight: inFlight}
	}
	return StatusPhaseTimingSnapshot{Phases: phases, ActivePhase: t.active}
}

type statusPhaseTimingsContextKey struct{}

func WithStatusPhaseTimings(ctx context.Context, timings *StatusPhaseTimings) context.Context {
	return context.WithValue(ctx, statusPhaseTimingsContextKey{}, timings)
}

func StatusPhaseTimingsFromContext(ctx context.Context) *StatusPhaseTimings {
	if ctx == nil {
		return nil
	}
	timings, _ := ctx.Value(statusPhaseTimingsContextKey{}).(*StatusPhaseTimings)
	return timings
}

func StartStatusPhase(ctx context.Context, phase StatusPhase) func() {
	if timings := StatusPhaseTimingsFromContext(ctx); timings != nil {
		return timings.Start(phase)
	}
	return func() {}
}
