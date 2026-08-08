package model

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// CapabilityRecord is the canonical capability surface consumed by the TUI
// pickers and the adapter layer. It describes what reasoning, speed, and
// service tiers a Codex model advertises, plus the provenance of the data
// (live runtime catalog or curated fallback).
//
// The slices are disjoint by construction: a value such as "fast" lives
// only in SpeedTiers and a value such as "priority" lives only in
// ServiceTiers — never in Reasoning. The Validate helper enforces the
// invariant; constructors (RecordFromRuntime, RecordFromCurated) refuse to
// produce a record that violates it.
//
// CapabilitySource is "runtime" when the data came from `codex debug
// models` and "curated" when it came from the embedded matrix. It is
// never empty and never both.
type CapabilityRecord struct {
	Reasoning         []string `json:"reasoning"`
	SpeedTiers        []string `json:"speed_tiers"`
	ServiceTiers      []string `json:"service_tiers"`
	MultiAgentVersion string   `json:"multi_agent_version,omitempty"`
	CapabilitySource  string   `json:"capability_source"`
}

// SourceRuntime is the CapabilitySource value stamped onto a record that
// was parsed from a live `codex debug models` payload.
const SourceRuntime = "runtime"

// SourceCurated is the CapabilitySource value stamped onto a record that
// was served by the embedded fallback matrix.
const SourceCurated = "curated"

// runtimeDebugPayload is the subset of the `codex debug models` JSON output
// that this package parses. The full catalog is much larger; we only need
// the slice fields and the multi_agent_version stamp.
type runtimeDebugPayload struct {
	Reasoning         []string `json:"reasoning"`
	SpeedTiers        []string `json:"speed_tiers"`
	ServiceTiers      []string `json:"service_tiers"`
	MultiAgentVersion string   `json:"multi_agent_version"`
}

// RecordFromRuntime parses a `codex debug models` JSON payload and returns
// a CapabilityRecord stamped with CapabilitySource = "runtime". The payload
// must (1) decode as JSON, (2) keep Reasoning disjoint from SpeedTiers and
// ServiceTiers, and (3) not carry "fast" or "priority" inside Reasoning —
// those are speed/service selectors, never reasoning effort values.
func RecordFromRuntime(payload []byte) (CapabilityRecord, error) {
	var raw runtimeDebugPayload
	if err := json.Unmarshal(payload, &raw); err != nil {
		return CapabilityRecord{}, fmt.Errorf("codex capabilities: parse runtime payload: %w", err)
	}

	rec := CapabilityRecord{
		Reasoning:         append([]string(nil), raw.Reasoning...),
		SpeedTiers:        append([]string(nil), raw.SpeedTiers...),
		ServiceTiers:      append([]string(nil), raw.ServiceTiers...),
		MultiAgentVersion: raw.MultiAgentVersion,
		CapabilitySource:  SourceRuntime,
	}

	if err := Validate(rec); err != nil {
		return CapabilityRecord{}, err
	}
	return rec, nil
}

// curatedCapabilityRow is the internal shape of the embedded fallback
// matrix. It is keyed by the Codex model identifier the picker exposes.
type curatedCapabilityRow struct {
	ModelID      string
	Reasoning    []string
	SpeedTiers   []string
	ServiceTiers []string
}

// codexCuratedCapabilityMatrix is the single source of truth for the
// fallback when `codex debug models` is unavailable. Each row corresponds
// to a Codex model advertised in the picker.
//
// Per the runtime evidence in issue #2218, sol and terra advertise the
// full reasoning ladder through `ultra`; luna advertises `max` but
// deliberately stops there — Codex never offers `ultra` for luna. The
// test file pins both ends of this contract so a future change cannot
// silently widen luna to `ultra` or strip `max` from sol/terra.
var codexCuratedCapabilityMatrix = []curatedCapabilityRow{
	{
		ModelID:      "gpt-5.6-sol",
		Reasoning:    []string{"low", "medium", "high", "xhigh", "max", "ultra"},
		SpeedTiers:   []string{"fast"},
		ServiceTiers: []string{"priority", "standard"},
	},
	{
		ModelID:      "gpt-5.6-terra",
		Reasoning:    []string{"low", "medium", "high", "xhigh", "max", "ultra"},
		SpeedTiers:   []string{"fast"},
		ServiceTiers: []string{"priority", "standard"},
	},
	{
		ModelID:      "gpt-5.6-luna",
		Reasoning:    []string{"low", "medium", "high", "xhigh", "max"},
		SpeedTiers:   []string{"fast"},
		ServiceTiers: []string{"priority", "standard"},
	},
}

// defaultCuratedRow is the conservative row served when the picker asks
// for a model the curated matrix does not know about. Only the three base
// reasoning levels are exposed — no max/ultra, no fast, no service tier.
var defaultCuratedRow = curatedCapabilityRow{
	ModelID:      "unknown",
	Reasoning:    []string{"low", "medium", "high"},
	SpeedTiers:   []string{},
	ServiceTiers: []string{},
}

// RecordFromCurated returns the curated fallback row for a given model
// identifier. Unknown identifiers receive the conservative default (only
// low/medium/high reasoning, no speed or service tiers). The returned
// record is stamped CapabilitySource = "curated" and MultiAgentVersion = ""
// because the curated matrix has no runtime version.
func RecordFromCurated(modelID string) CapabilityRecord {
	row := defaultCuratedRow
	for _, candidate := range codexCuratedCapabilityMatrix {
		if candidate.ModelID == modelID {
			row = candidate
			break
		}
	}

	return CapabilityRecord{
		Reasoning:         append([]string(nil), row.Reasoning...),
		SpeedTiers:        append([]string(nil), row.SpeedTiers...),
		ServiceTiers:      append([]string(nil), row.ServiceTiers...),
		MultiAgentVersion: "",
		CapabilitySource:  SourceCurated,
	}
}

// Validate enforces the structural invariants of CapabilityRecord:
//   - CapabilitySource is exactly "runtime" or "curated" (never empty).
//   - Reasoning is disjoint from SpeedTiers.
//   - Reasoning is disjoint from ServiceTiers.
//   - "fast" must not appear in Reasoning — it is a speed selector, not
//     a reasoning effort value.
//
// The function is the regression guard that keeps the curated matrix and
// any runtime payload honest; the picker relies on this to render the
// reasoning dropdown without ever surfacing "fast" as a reasoning option.
func Validate(cap CapabilityRecord) error {
	if cap.CapabilitySource != SourceRuntime && cap.CapabilitySource != SourceCurated {
		return fmt.Errorf("codex capabilities: capability_source must be %q or %q, got %q",
			SourceRuntime, SourceCurated, cap.CapabilitySource)
	}

	if _, value := firstOverlap(cap.Reasoning, cap.SpeedTiers); value != "" {
		return fmt.Errorf("codex capabilities: reasoning and speed_tiers must be disjoint; %q appears in both", value)
	}
	if _, value := firstOverlap(cap.Reasoning, cap.ServiceTiers); value != "" {
		return fmt.Errorf("codex capabilities: reasoning and service_tiers must be disjoint; %q appears in both", value)
	}

	if value := containsValue(cap.Reasoning, "fast"); value != "" {
		return fmt.Errorf("codex capabilities: %q must never appear in reasoning; it is a speed selector", value)
	}

	return nil
}

// firstOverlap reports the first value (lexicographic) that appears in
// both slices. Returns the empty string when the slices are disjoint.
// Compares case-sensitively — the curated matrix and the runtime payload
// both use lowercase identifiers.
func firstOverlap(a, b []string) (string, string) {
	if len(a) == 0 || len(b) == 0 {
		return "", ""
	}
	seen := make(map[string]struct{}, len(b))
	for _, v := range b {
		seen[v] = struct{}{}
	}
	for _, v := range a {
		if _, ok := seen[v]; ok {
			return v, v
		}
	}
	return "", ""
}

// containsValue returns the matched value when want appears in slice,
// otherwise the empty string. The match is case-insensitive on a lowercase
// copy so "FAST" or "Fast" cannot sneak past the validator.
func containsValue(slice []string, want string) string {
	want = strings.ToLower(want)
	for _, v := range slice {
		if strings.ToLower(v) == want {
			return v
		}
	}
	return ""
}

// sortCopy returns a lowercased, sorted copy of the input. The exported
// surface does not need ordering, but the helper exists so future
// fingerprinting (e.g. dedup or equality checks across runs) can lean on a
// canonical form. Kept unexported for now.
func sortCopy(s []string) []string {
	out := make([]string, len(s))
	for i, v := range s {
		out[i] = strings.ToLower(v)
	}
	sort.Strings(out)
	return out
}
