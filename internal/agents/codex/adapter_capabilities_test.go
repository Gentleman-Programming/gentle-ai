package codex

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

// stubLookPath returns a function that always resolves "codex" to a
// caller-supplied path (or always errors). Mirrors the style used in
// TestDetect so the new tests read the same way.
func stubLookPath(path string, err error) func(string) (string, error) {
	return func(string) (string, error) {
		return path, err
	}
}

// capabilitiesProbeFingerprint is the JSON shape a fake `codex debug
// models` returns. Tests substitute or mutate this string per scenario.
const capabilitiesProbeFingerprint = `{
	"reasoning": ["low","medium","high","xhigh","max","ultra"],
	"speed_tiers": ["fast"],
	"service_tiers": ["priority","standard"],
	"multi_agent_version": "0.42.0"
}`

// withCapabilitiesProbe swaps the runCapabilitiesCommand hook for the
// duration of the test. It returns a teardown that callers defer.
func withCapabilitiesProbe(t *testing.T, probe func(ctx context.Context, _ string) ([]byte, error)) {
	t.Helper()
	original := runCapabilitiesCommand
	runCapabilitiesCommand = probe
	t.Cleanup(func() {
		runCapabilitiesCommand = original
	})
}

// TestAdapterCapabilitiesDiscovery is the table-driven proof that
// Capabilities sources its record from the runtime when the live catalog
// answers, and from the curated fallback whenever the runtime cannot —
// lookup failure, timeout, non-zero exit, or parse error.
//
// Each table row maps to a scenario in
// openspec/changes/2218-.../specs/model-capability-discovery/spec.md:
//   - S1-R2: Live catalog → record comes from runtime.
//   - S1-R2: Catalog unavailable → curated fallback takes over.
//   - S1-R2: Bounded by the timeout, never blocks the UI.
func TestAdapterCapabilitiesDiscovery(t *testing.T) {
	tests := []struct {
		name              string
		lookPath          func(string) (string, error)
		probe             func(ctx context.Context, _ string) ([]byte, error)
		wantSource        string
		wantReasoning     []string
		wantVersion       string
		wantContainsError string
	}{
		{
			name:     "live catalog — runtime payload is parsed",
			lookPath: stubLookPath("/usr/local/bin/codex", nil),
			probe: func(_ context.Context, _ string) ([]byte, error) {
				return []byte(capabilitiesProbeFingerprint), nil
			},
			wantSource:    model.SourceRuntime,
			wantReasoning: []string{"low", "medium", "high", "xhigh", "max", "ultra"},
			wantVersion:   "0.42.0",
		},
		{
			name:     "lookup fails — curated fallback",
			lookPath: stubLookPath("", errors.New("codex not on PATH")),
			probe: func(_ context.Context, _ string) ([]byte, error) {
				return []byte(capabilitiesProbeFingerprint), nil
			},
			wantSource:        model.SourceCurated,
			wantReasoning:     []string{"low", "medium", "high", "xhigh", "max", "ultra"},
			wantVersion:       "",
			wantContainsError: "",
		},
		{
			name:     "lookup returns empty path — curated fallback",
			lookPath: stubLookPath("", nil),
			probe: func(_ context.Context, _ string) ([]byte, error) {
				return []byte(capabilitiesProbeFingerprint), nil
			},
			wantSource:    model.SourceCurated,
			wantReasoning: []string{"low", "medium", "high", "xhigh", "max", "ultra"},
			wantVersion:   "",
		},
		{
			name:     "process timeout — curated fallback",
			lookPath: stubLookPath("/usr/local/bin/codex", nil),
			probe: func(ctx context.Context, _ string) ([]byte, error) {
				// Block until the inherited WithTimeout fires. The picker's
				// 2s budget is exercised end-to-end: the command returns
				// context.DeadlineExceeded just like a hung CLI would.
				<-ctx.Done()
				return nil, ctx.Err()
			},
			wantSource:    model.SourceCurated,
			wantReasoning: []string{"low", "medium", "high", "xhigh", "max", "ultra"},
			wantVersion:   "",
		},
		{
			name:     "non-zero exit — curated fallback",
			lookPath: stubLookPath("/usr/local/bin/codex", nil),
			probe: func(_ context.Context, _ string) ([]byte, error) {
				return []byte("codex: internal error"), &exec.ExitError{}
			},
			wantSource:    model.SourceCurated,
			wantReasoning: []string{"low", "medium", "high", "xhigh", "max", "ultra"},
			wantVersion:   "",
		},
		{
			name:     "invalid JSON payload — curated fallback",
			lookPath: stubLookPath("/usr/local/bin/codex", nil),
			probe: func(_ context.Context, _ string) ([]byte, error) {
				return []byte(`{"reasoning": ["low",`), nil
			},
			wantSource:    model.SourceCurated,
			wantReasoning: []string{"low", "medium", "high", "xhigh", "max", "ultra"},
			wantVersion:   "",
		},
		{
			name:     "fast in reasoning payload — curated fallback",
			lookPath: stubLookPath("/usr/local/bin/codex", nil),
			probe: func(_ context.Context, _ string) ([]byte, error) {
				// Runtime side tried to leak "fast" into reasoning. The
				// runtime constructor rejects it, so we fall back. This
				// guards the picker's reasoning dropdown from a broken
				// runtime that happened to compile.
				return []byte(`{"reasoning":["low","medium","fast"]}`), nil
			},
			wantSource:    model.SourceCurated,
			wantReasoning: []string{"low", "medium", "high", "xhigh", "max", "ultra"},
			wantVersion:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withCapabilitiesProbe(t, tt.probe)

			a := &Adapter{
				lookPath: tt.lookPath,
				statPath: func(string) statResult { return statResult{err: errors.New("unused")} },
			}

			rec, err := a.Capabilities(context.Background(), tt.lookPath)
			if err != nil {
				t.Fatalf("Capabilities() returned error %v, want nil (picker must never block)", err)
			}
			if rec.CapabilitySource != tt.wantSource {
				t.Fatalf("CapabilitySource = %q, want %q", rec.CapabilitySource, tt.wantSource)
			}
			if !reflect.DeepEqual(rec.Reasoning, tt.wantReasoning) {
				t.Fatalf("Reasoning = %v, want %v", rec.Reasoning, tt.wantReasoning)
			}
			if rec.MultiAgentVersion != tt.wantVersion {
				t.Fatalf("MultiAgentVersion = %q, want %q", rec.MultiAgentVersion, tt.wantVersion)
			}
		})
	}
}

// TestAdapterCapabilities_NilLookupUsesAdapterField asserts that when the
// caller passes a nil lookup, the adapter falls back to its own
// lookPath field. The picker is allowed to omit the lookup argument
// once it has wired the adapter.
func TestAdapterCapabilities_NilLookupUsesAdapterField(t *testing.T) {
	withCapabilitiesProbe(t, func(_ context.Context, _ string) ([]byte, error) {
		return []byte(capabilitiesProbeFingerprint), nil
	})

	a := &Adapter{
		lookPath: stubLookPath("/usr/local/bin/codex", nil),
		statPath: func(string) statResult { return statResult{err: errors.New("unused")} },
	}

	rec, err := a.Capabilities(context.Background(), nil)
	if err != nil {
		t.Fatalf("Capabilities() with nil lookup returned error %v", err)
	}
	if rec.CapabilitySource != model.SourceRuntime {
		t.Fatalf("CapabilitySource = %q, want %q", rec.CapabilitySource, model.SourceRuntime)
	}
}

// TestAdapterCapabilities_NilAdapterLookPathCoversDefensive asserts the
// fully-degenerate case: a caller that constructs an Adapter without
// setting lookPath (e.g. a future test helper) still receives a curated
// fallback. The picker must never see a nil-deref panic.
func TestAdapterCapabilities_NilAdapterLookPathCoversDefensive(t *testing.T) {
	withCapabilitiesProbe(t, func(_ context.Context, _ string) ([]byte, error) {
		return []byte(capabilitiesProbeFingerprint), nil
	})

	a := &Adapter{
		lookPath: nil,
		statPath: func(string) statResult { return statResult{err: errors.New("unused")} },
	}

	rec, err := a.Capabilities(context.Background(), nil)
	if err != nil {
		t.Fatalf("Capabilities() returned error %v", err)
	}
	if rec.CapabilitySource != model.SourceCurated {
		t.Fatalf("CapabilitySource = %q, want %q", rec.CapabilitySource, model.SourceCurated)
	}
}

// TestAdapterCapabilities_NeverPropagatesError is the picker contract
// gate: no matter what the runtime does, Capabilities returns a non-nil
// record and a nil error. The TUI discards the error and renders the
// record — if Capabilities ever bubbles an error up, the picker's first
// Update() would render a blank screen.
func TestAdapterCapabilities_NeverPropagatesError(t *testing.T) {
	withCapabilitiesProbe(t, func(_ context.Context, _ string) ([]byte, error) {
		return nil, errors.New("synthetic boom")
	})

	a := &Adapter{
		lookPath: stubLookPath("/usr/local/bin/codex", nil),
		statPath: func(string) statResult { return statResult{err: errors.New("unused")} },
	}

	rec, err := a.Capabilities(context.Background(), nil)
	if err != nil {
		t.Fatalf("Capabilities() returned error %v — picker contract requires nil error", err)
	}
	if rec.CapabilitySource == "" {
		t.Fatal("CapabilitySource is empty — picker cannot render footer")
	}
	if rec.CapabilitySource != model.SourceCurated {
		t.Fatalf("CapabilitySource = %q, want %q on synthetic probe failure", rec.CapabilitySource, model.SourceCurated)
	}
}

// TestAdapterCapabilities_DoesNotMutateTier checks that adding the
// Capabilities method does not change the existing Tier() contract.
// Tier() reports agent support (Full/Partial) and is unrelated to
// service tier; this regression guard prevents a future refactor from
// collapsing the two methods.
func TestAdapterCapabilities_DoesNotMutateTier(t *testing.T) {
	a := NewAdapter()
	if got := a.Tier(); got != model.TierFull {
		t.Fatalf("Tier() = %v, want TierFull (Capabilities must not change Tier)", got)
	}
}

// TestAdapterCapabilities_CuratedFallbackMentionsSol pins the
// curatedFallbackModelID constant. The picker footer surfaces this
// fallback; if a future refactor renames sol to terra by mistake, the
// reasoning dropdown would gain "ultra" for luna and break the spec.
func TestAdapterCapabilities_CuratedFallbackMentionsSol(t *testing.T) {
	withCapabilitiesProbe(t, func(_ context.Context, _ string) ([]byte, error) {
		return nil, errors.New("probe must not be called")
	})

	a := &Adapter{
		lookPath: stubLookPath("/usr/local/bin/codex", nil),
		statPath: func(string) statResult { return statResult{err: errors.New("unused")} },
	}

	rec, err := a.Capabilities(context.Background(), nil)
	if err != nil {
		t.Fatalf("Capabilities() returned error %v", err)
	}
	if rec.CapabilitySource != model.SourceCurated {
		t.Fatalf("CapabilitySource = %q, want %q", rec.CapabilitySource, model.SourceCurated)
	}
	// The curated sol row carries the full ladder including ultra.
	hasUltra := false
	for _, v := range rec.Reasoning {
		if v == "ultra" {
			hasUltra = true
			break
		}
	}
	if !hasUltra {
		t.Fatalf("curated fallback reasoning = %v — must contain ultra (this is the sol row)", rec.Reasoning)
	}
}

// TestAdapterCapabilities_EmptyParentContextDoesNotPanic guards the
// degnerate case where the supplied context is already cancelled. The
// probe returns DeadlineExceeded immediately and the adapter falls back.
func TestAdapterCapabilities_EmptyParentContextDoesNotPanic(t *testing.T) {
	withCapabilitiesProbe(t, func(ctx context.Context, _ string) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})

	a := &Adapter{
		lookPath: stubLookPath("/usr/local/bin/codex", nil),
		statPath: func(string) statResult { return statResult{err: errors.New("unused")} },
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	rec, err := a.Capabilities(ctx, nil)
	if err != nil {
		t.Fatalf("Capabilities() returned error %v", err)
	}
	if rec.CapabilitySource != model.SourceCurated {
		t.Fatalf("CapabilitySource = %q, want %q", rec.CapabilitySource, model.SourceCurated)
	}
	if strings.TrimSpace(rec.CapabilitySource) == "" {
		t.Fatal("CapabilitySource is empty after pre-cancelled context")
	}
}
