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

			rec, err := a.Capabilities(context.Background(), tt.lookPath, "")
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

	rec, err := a.Capabilities(context.Background(), nil, "")
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

	rec, err := a.Capabilities(context.Background(), nil, "")
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

	rec, err := a.Capabilities(context.Background(), nil, "")
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

	rec, err := a.Capabilities(context.Background(), nil, "")
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

	rec, err := a.Capabilities(ctx, nil, "")
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

// realEnvelopeFixture is the shape `codex debug models` returns when
// invoked against a live Codex CLI: a `models` array carrying one entry
// per model, each with the four capability fields the picker renders.
// Field names come from the existing flat parser (reasoning/speed_tiers/
// service_tiers/multi_agent_version) and have not been re-probed against
// a live Codex binary in this slice; if a real CLI uses different names,
// runtimeModelEnvelope in internal/model/codex_capabilities.go is the one
// place to update.
const realEnvelopeFixture = `{
	"models": [
		{
			"slug": "gpt-5.6-sol",
			"reasoning": ["low","medium","high","xhigh","max","ultra"],
			"speed_tiers": ["fast"],
			"service_tiers": ["priority","standard"],
			"multi_agent_version": "0.42.0"
		},
		{
			"slug": "gpt-5.6-luna",
			"reasoning": ["low","medium","high","xhigh","max"],
			"speed_tiers": ["fast"],
			"service_tiers": ["priority"]
		},
		{
			"slug": "gpt-5.6-terra",
			"reasoning": ["low","medium","high","xhigh","max","ultra"],
			"speed_tiers": ["fast","balanced"],
			"service_tiers": ["priority","standard"],
			"multi_agent_version": "0.42.0"
		}
	]
}`

// TestAdapterCapabilitiesPerModelEnvelope pins the per-model lookup:
// when the runtime returns the real envelope `{"models":[{slug,...}]}`
// shape, Capabilities must look up the requested modelID and return its
// per-model capability slice. decode2 (2026-08-18) PR #2761 review.
func TestAdapterCapabilitiesPerModelEnvelope(t *testing.T) {
	tests := []struct {
		name              string
		modelID           string
		envelope          string
		wantSource        string
		wantReasoning     []string
		wantSpeedTiers    []string
		wantServiceTiers  []string
		wantMultiAgentVer string
	}{
		{
			name:              "sol runtime: full ladder including ultra",
			modelID:           "gpt-5.6-sol",
			envelope:          realEnvelopeFixture,
			wantSource:        model.SourceRuntime,
			wantReasoning:     []string{"low", "medium", "high", "xhigh", "max", "ultra"},
			wantSpeedTiers:    []string{"fast"},
			wantServiceTiers:  []string{"priority", "standard"},
			wantMultiAgentVer: "0.42.0",
		},
		{
			name:              "luna runtime: no ultra (spec regression guard)",
			modelID:           "gpt-5.6-luna",
			envelope:          realEnvelopeFixture,
			wantSource:        model.SourceRuntime,
			wantReasoning:     []string{"low", "medium", "high", "xhigh", "max"},
			wantSpeedTiers:    []string{"fast"},
			wantServiceTiers:  []string{"priority"},
			wantMultiAgentVer: "",
		},
		{
			name:             "model absent from runtime catalog: curated fallback for modelID",
			modelID:          "gpt-5.6-some-future-model",
			envelope:         realEnvelopeFixture,
			wantSource:       model.SourceCurated,
			wantReasoning:    []string{"low", "medium", "high"},
		},
		{
			name:              "empty modelID with envelope: legacy flat-fallback path",
			modelID:           "",
			envelope:          `{"reasoning":["low","medium","high","xhigh","max","ultra"],"speed_tiers":["fast"],"service_tiers":["priority","standard"],"multi_agent_version":"0.42.0"}`,
			wantSource:        model.SourceRuntime,
			wantReasoning:     []string{"low", "medium", "high", "xhigh", "max", "ultra"},
			wantSpeedTiers:    []string{"fast"},
			wantServiceTiers:  []string{"priority", "standard"},
			wantMultiAgentVer: "0.42.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withCapabilitiesProbe(t, func(_ context.Context, _ string) ([]byte, error) {
				return []byte(tt.envelope), nil
			})
			a := &Adapter{
				lookPath: stubLookPath("/usr/local/bin/codex", nil),
				statPath: func(string) statResult { return statResult{err: errors.New("unused")} },
			}
			rec, err := a.Capabilities(context.Background(), a.lookPath, tt.modelID)
			if err != nil {
				t.Fatalf("Capabilities(modelID=%q) returned error %v, want nil (picker must never block)", tt.modelID, err)
			}
			if rec.CapabilitySource != tt.wantSource {
				t.Errorf("CapabilitySource = %q, want %q", rec.CapabilitySource, tt.wantSource)
			}
			if !reflect.DeepEqual(rec.Reasoning, tt.wantReasoning) {
				t.Errorf("Reasoning = %v, want %v", rec.Reasoning, tt.wantReasoning)
			}
			if len(tt.wantSpeedTiers) > 0 && !reflect.DeepEqual(rec.SpeedTiers, tt.wantSpeedTiers) {
				t.Errorf("SpeedTiers = %v, want %v", rec.SpeedTiers, tt.wantSpeedTiers)
			}
			if len(tt.wantSpeedTiers) == 0 && len(rec.SpeedTiers) != 0 {
				t.Errorf("SpeedTiers = %v, want empty", rec.SpeedTiers)
			}
			if len(tt.wantServiceTiers) > 0 && !reflect.DeepEqual(rec.ServiceTiers, tt.wantServiceTiers) {
				t.Errorf("ServiceTiers = %v, want %v", rec.ServiceTiers, tt.wantServiceTiers)
			}
			if len(tt.wantServiceTiers) == 0 && len(rec.ServiceTiers) != 0 {
				t.Errorf("ServiceTiers = %v, want empty", rec.ServiceTiers)
			}
			if rec.MultiAgentVersion != tt.wantMultiAgentVer {
				t.Errorf("MultiAgentVersion = %q, want %q", rec.MultiAgentVersion, tt.wantMultiAgentVer)
			}
		})
	}
}

// TestRecordFromRuntimeForModelLookupMissing pins the lookup error path:
// when the requested modelID is not present in the envelope the helper
// returns a typed error and the adapter maps it to a curated fallback.
func TestRecordFromRuntimeForModelLookupMissing(t *testing.T) {
	_, err := model.RecordFromRuntimeForModel([]byte(realEnvelopeFixture), "gpt-5.6-does-not-exist")
	if err == nil {
		t.Fatal("RecordFromRuntimeForModel with unknown modelID = nil error, want an error so adapter can map to curated fallback")
	}
	if !strings.Contains(err.Error(), "gpt-5.6-does-not-exist") {
		t.Errorf("error must name the requested model id, got: %v", err)
	}
	if !strings.Contains(err.Error(), "not present") {
		t.Errorf("error must indicate the catalog miss, got: %v", err)
	}
}
