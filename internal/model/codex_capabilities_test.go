package model

import (
	"reflect"
	"strings"
	"testing"
)

// TestRecordFromRuntime pins the contract of `codex debug models` payload
// decoding. Every row maps to a scenario from
// openspec/changes/2218-.../specs/model-capability-discovery/spec.md
// (S1-R1: Record fields populated from a runtime payload,
// S1-R1: Fast rejected from the reasoning slice,
// S1-R1: Capability source is exactly one of two allowed values).
func TestRecordFromRuntime(t *testing.T) {
	t.Run("valid payload", func(t *testing.T) {
		payload := []byte(`{
			"reasoning": ["low","medium","high","xhigh","max","ultra"],
			"speed_tiers": ["fast"],
			"service_tiers": ["priority","standard"],
			"multi_agent_version": "0.42.0"
		}`)

		rec, err := RecordFromRuntime(payload)
		if err != nil {
			t.Fatalf("RecordFromRuntime() unexpected error: %v", err)
		}

		wantReasoning := []string{"low", "medium", "high", "xhigh", "max", "ultra"}
		if !reflect.DeepEqual(rec.Reasoning, wantReasoning) {
			t.Fatalf("Reasoning = %v, want %v", rec.Reasoning, wantReasoning)
		}
		wantSpeed := []string{"fast"}
		if !reflect.DeepEqual(rec.SpeedTiers, wantSpeed) {
			t.Fatalf("SpeedTiers = %v, want %v", rec.SpeedTiers, wantSpeed)
		}
		wantService := []string{"priority", "standard"}
		if !reflect.DeepEqual(rec.ServiceTiers, wantService) {
			t.Fatalf("ServiceTiers = %v, want %v", rec.ServiceTiers, wantService)
		}
		if rec.MultiAgentVersion != "0.42.0" {
			t.Fatalf("MultiAgentVersion = %q, want %q", rec.MultiAgentVersion, "0.42.0")
		}
		if rec.CapabilitySource != SourceRuntime {
			t.Fatalf("CapabilitySource = %q, want %q", rec.CapabilitySource, SourceRuntime)
		}
	})

	t.Run("fast in reasoning is rejected", func(t *testing.T) {
		payload := []byte(`{"reasoning":["low","medium","fast"]}`)

		_, err := RecordFromRuntime(payload)
		if err == nil {
			t.Fatal("RecordFromRuntime() expected error for fast-in-reasoning payload, got nil")
		}
		if !strings.Contains(err.Error(), "fast") {
			t.Fatalf("RecordFromRuntime() error %q must mention %q", err.Error(), "fast")
		}
	})

	t.Run("malformed JSON is rejected", func(t *testing.T) {
		_, err := RecordFromRuntime([]byte(`{"reasoning": ["low",`))
		if err == nil {
			t.Fatal("RecordFromRuntime() expected error for malformed JSON, got nil")
		}
	})

	t.Run("empty payload decodes to zero record stamped runtime", func(t *testing.T) {
		// {} is valid JSON; the empty record has no overlaps so Validate passes.
		// This is the legitimate edge case where the runtime payload omits every
		// slice — the picker renders an empty reasoning dropdown rather than
		// falling back to curated, because the runtime call succeeded.
		rec, err := RecordFromRuntime([]byte(`{}`))
		if err != nil {
			t.Fatalf("RecordFromRuntime() unexpected error: %v", err)
		}
		if rec.CapabilitySource != SourceRuntime {
			t.Fatalf("CapabilitySource = %q, want %q", rec.CapabilitySource, SourceRuntime)
		}
		if len(rec.Reasoning) != 0 || len(rec.SpeedTiers) != 0 || len(rec.ServiceTiers) != 0 {
			t.Fatalf("expected empty slices, got Reasoning=%v Speed=%v Service=%v",
				rec.Reasoning, rec.SpeedTiers, rec.ServiceTiers)
		}
	})
}

// TestRecordFromCurated pins the curated fallback matrix. Luna is the
// regression guard: it advertises `max` but NEVER `ultra` (per the runtime
// evidence in issue #2218). The companion TestLunaNeverCarriesUltra below
// pins the exact absence of `ultra` from luna's reasoning slice.
func TestRecordFromCurated(t *testing.T) {
	tests := []struct {
		name              string
		modelID           string
		wantReasoning     []string
		wantSpeedTiers    []string
		wantServiceTiers  []string
		wantCapabilitySrc string
	}{
		{
			name:              "sol carries the full ladder through ultra",
			modelID:           "gpt-5.6-sol",
			wantReasoning:     []string{"low", "medium", "high", "xhigh", "max", "ultra"},
			wantSpeedTiers:    []string{"fast"},
			wantServiceTiers:  []string{"priority", "standard"},
			wantCapabilitySrc: SourceCurated,
		},
		{
			name:              "terra mirrors sol",
			modelID:           "gpt-5.6-terra",
			wantReasoning:     []string{"low", "medium", "high", "xhigh", "max", "ultra"},
			wantSpeedTiers:    []string{"fast"},
			wantServiceTiers:  []string{"priority", "standard"},
			wantCapabilitySrc: SourceCurated,
		},
		{
			name:              "luna carries max but not ultra",
			modelID:           "gpt-5.6-luna",
			wantReasoning:     []string{"low", "medium", "high", "xhigh", "max"},
			wantSpeedTiers:    []string{"fast"},
			wantServiceTiers:  []string{"priority", "standard"},
			wantCapabilitySrc: SourceCurated,
		},
		{
			name:              "unknown model falls back to conservative default",
			modelID:           "gpt-9.9-future",
			wantReasoning:     []string{"low", "medium", "high"},
			wantSpeedTiers:    nil,
			wantServiceTiers:  nil,
			wantCapabilitySrc: SourceCurated,
		},
		{
			name:              "empty model id falls back to conservative default",
			modelID:           "",
			wantReasoning:     []string{"low", "medium", "high"},
			wantSpeedTiers:    nil,
			wantServiceTiers:  nil,
			wantCapabilitySrc: SourceCurated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := RecordFromCurated(tt.modelID)

			if !reflect.DeepEqual(rec.Reasoning, tt.wantReasoning) {
				t.Fatalf("Reasoning = %v, want %v", rec.Reasoning, tt.wantReasoning)
			}
			if !reflect.DeepEqual(rec.SpeedTiers, tt.wantSpeedTiers) {
				t.Fatalf("SpeedTiers = %v, want %v", rec.SpeedTiers, tt.wantSpeedTiers)
			}
			if !reflect.DeepEqual(rec.ServiceTiers, tt.wantServiceTiers) {
				t.Fatalf("ServiceTiers = %v, want %v", rec.ServiceTiers, tt.wantServiceTiers)
			}
			if rec.CapabilitySource != tt.wantCapabilitySrc {
				t.Fatalf("CapabilitySource = %q, want %q", rec.CapabilitySource, tt.wantCapabilitySrc)
			}
			if rec.MultiAgentVersion != "" {
				t.Fatalf("MultiAgentVersion = %q, want \"\" (curated has no runtime version)", rec.MultiAgentVersion)
			}
		})
	}
}

// TestLunaNeverCarriesUltra is the explicit regression guard called out in
// the design document. Per the runtime evidence in issue #2218, luna
// advertises `max` but Codex never exposes `ultra` for luna. This test
// pins the upper bound of luna's reasoning slice so a future matrix
// refactor cannot silently widen luna to `ultra`.
func TestLunaNeverCarriesUltra(t *testing.T) {
	rec := RecordFromCurated("gpt-5.6-luna")

	for _, value := range rec.Reasoning {
		if value == "ultra" {
			t.Fatalf("luna reasoning carries %q — luna MUST NOT advertise ultra (per issue #2218 runtime evidence)", value)
		}
	}

	// Belt-and-suspenders: assert the exact slice. If someone widens luna to
	// include `ultra`, this fails fast and forces them to update both the
	// matrix and the assertion.
	want := []string{"low", "medium", "high", "xhigh", "max"}
	if !reflect.DeepEqual(rec.Reasoning, want) {
		t.Fatalf("luna reasoning = %v, want exactly %v", rec.Reasoning, want)
	}
}

// TestSolTerraCarryMaxAndUltra is the companion guard: sol and terra
// advertise the full reasoning ladder through `ultra`. A future refactor
// that strips `max` or `ultra` from sol/terra must fail loudly so the
// picker surfaces the regression.
func TestSolTerraCarryMaxAndUltra(t *testing.T) {
	for _, id := range []string{"gpt-5.6-sol", "gpt-5.6-terra"} {
		t.Run(id, func(t *testing.T) {
			rec := RecordFromCurated(id)
			hasMax := false
			hasUltra := false
			for _, value := range rec.Reasoning {
				if value == "max" {
					hasMax = true
				}
				if value == "ultra" {
					hasUltra = true
				}
			}
			if !hasMax {
				t.Fatalf("%s reasoning is missing `max` — sol/terra MUST carry the full reasoning ladder (per issue #2218)", id)
			}
			if !hasUltra {
				t.Fatalf("%s reasoning is missing `ultra` — sol/terra MUST carry the full reasoning ladder (per issue #2218)", id)
			}
		})
	}
}

// TestCuratedMatrixServiceTierDisjoint walks every row of the embedded
// matrix and asserts Reasoning, SpeedTiers, and ServiceTiers are pairwise
// disjoint. This is the canonical guard for spec S1-R3: "Service-tier-
// disjoint across all rows".
func TestCuratedMatrixServiceTierDisjoint(t *testing.T) {
	ids := []string{"gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna"}
	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			rec := RecordFromCurated(id)

			if err := Validate(rec); err != nil {
				t.Fatalf("Validate(%s) = %v, want nil", id, err)
			}

			if _, v := firstOverlap(rec.Reasoning, rec.SpeedTiers); v != "" {
				t.Fatalf("%s reasoning ∩ speed_tiers contains %q", id, v)
			}
			if _, v := firstOverlap(rec.Reasoning, rec.ServiceTiers); v != "" {
				t.Fatalf("%s reasoning ∩ service_tiers contains %q", id, v)
			}

			for _, v := range rec.SpeedTiers {
				if v == "fast" {
					continue
				}
				t.Fatalf("%s speed_tiers carries unexpected value %q (only %q is allowed in curated rows)",
					id, v, "fast")
			}
			for _, v := range rec.ServiceTiers {
				if v == "priority" || v == "standard" {
					continue
				}
				t.Fatalf("%s service_tiers carries unexpected value %q", id, v)
			}
		})
	}
}

// TestValidate_AcceptsCuratedMatrix is the positive branch — every curated
// row must pass Validate without error. If a future change widens a row
// in a way that breaks the partition invariant, this test fails before
// the picker reaches the user.
func TestValidate_AcceptsCuratedMatrix(t *testing.T) {
	ids := []string{"gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna"}
	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			rec := RecordFromCurated(id)
			if err := Validate(rec); err != nil {
				t.Fatalf("Validate(RecordFromCurated(%q)) = %v, want nil", id, err)
			}
		})
	}
}

// TestValidate_RejectsViolations covers the negative branches of
// Validate. Each row deliberately constructs a record that should be
// rejected so the validator's checks cannot silently regress.
func TestValidate_RejectsViolations(t *testing.T) {
	tests := []struct {
		name       string
		record     CapabilityRecord
		wantSubstr string
	}{
		{
			name: "fast in reasoning",
			record: CapabilityRecord{
				Reasoning:        []string{"low", "fast"},
				SpeedTiers:       []string{"fast"},
				ServiceTiers:     []string{"priority"},
				CapabilitySource: SourceRuntime,
			},
			wantSubstr: "fast",
		},
		{
			name: "priority in reasoning",
			record: CapabilityRecord{
				Reasoning:        []string{"low", "priority"},
				SpeedTiers:       []string{},
				ServiceTiers:     []string{"priority"},
				CapabilitySource: SourceRuntime,
			},
			wantSubstr: "disjoint",
		},
		{
			name: "reasoning overlaps speed_tiers",
			record: CapabilityRecord{
				Reasoning:        []string{"low", "high"},
				SpeedTiers:       []string{"high"},
				ServiceTiers:     []string{},
				CapabilitySource: SourceRuntime,
			},
			wantSubstr: "disjoint",
		},
		{
			name: "reasoning overlaps service_tiers",
			record: CapabilityRecord{
				Reasoning:        []string{"priority"},
				SpeedTiers:       []string{"fast"},
				ServiceTiers:     []string{"priority"},
				CapabilitySource: SourceRuntime,
			},
			wantSubstr: "disjoint",
		},
		{
			name: "empty capability source",
			record: CapabilityRecord{
				Reasoning:        []string{"low"},
				CapabilitySource: "",
			},
			wantSubstr: "capability_source",
		},
		{
			name: "bogus capability source",
			record: CapabilityRecord{
				Reasoning:        []string{"low"},
				CapabilitySource: "sync",
			},
			wantSubstr: "capability_source",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.record)
			if err == nil {
				t.Fatalf("Validate() returned nil, want error containing %q", tt.wantSubstr)
			}
			if !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Fatalf("Validate() error = %q, want substring %q", err.Error(), tt.wantSubstr)
			}
		})
	}
}

// TestCapabilityRecordSlicesAreIndependent guards against an accidental
// aliasing bug where two curated rows could share the same backing array
// and mutate each other. Callers may freely modify the returned slices.
func TestCapabilityRecordSlicesAreIndependent(t *testing.T) {
	a := RecordFromCurated("gpt-5.6-sol")
	b := RecordFromCurated("gpt-5.6-sol")

	if len(a.Reasoning) > 0 && len(b.Reasoning) > 0 &&
		&a.Reasoning[0] == &b.Reasoning[0] {
		t.Fatal("Reasoning slices share backing array — callers could mutate across records")
	}
	if len(a.SpeedTiers) > 0 && len(b.SpeedTiers) > 0 &&
		&a.SpeedTiers[0] == &b.SpeedTiers[0] {
		t.Fatal("SpeedTiers slices share backing array")
	}

	a.Reasoning[0] = "MUTATED"
	if b.Reasoning[0] == "MUTATED" {
		t.Fatal("mutating one record leaked into another — slice copy is broken")
	}
}
