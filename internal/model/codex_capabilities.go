package model

import (
	"encoding/json"
	"fmt"
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
// that this package parses for the legacy flat shape (top-level slices).
// The current real envelope wraps every per-model entry inside a `models`
// array; that shape is handled by recordFromRuntimeEnvelope below. This
// flat struct is kept only so existing legacy callers of RecordFromRuntime
// (which feed a top-level payload) still compile.
type runtimeDebugPayload struct {
	Reasoning         []string `json:"reasoning"`
	SpeedTiers        []string `json:"speed_tiers"`
	ServiceTiers      []string `json:"service_tiers"`
	MultiAgentVersion string   `json:"multi_agent_version"`
}

// runtimeModelEnvelope is one per-model entry inside the `models` array of
// the real `codex debug models` output. It mirrors the same fields as
// runtimeDebugPayload but adds `slug`, which is the model identifier the
// picker exposes.
//
// The field names below are best-effort: the codebase already used
// reasoning/speed_tiers/service_tiers for the legacy flat parser, and the
// same names appear as the natural names inside a per-model entry. The real
// Codex CLI JSON shape is not committed anywhere in the repository and has
// not been re-probed against a live Codex binary in this slice. If the live
// CLI uses different field names, this struct is the ONE place to update;
// RunCodexCapabilityForModel re-derives everything from it.
type runtimeModelEnvelope struct {
	Slug             string   `json:"slug"`
	Reasoning        []string `json:"reasoning"`
	SpeedTiers       []string `json:"speed_tiers"`
	ServiceTiers     []string `json:"service_tiers"`
	MultiAgentVer    string   `json:"multi_agent_version"`
}

// runtimeEnvelope is the wrapping shape returned by `codex debug models`:
// a single `models` array carrying one entry per Codex model the CLI knows.
// The filtering selector (`Visibility`/`SupportedInAPI`) lives in
// internal/model/codex_model.go's codexDiscoveredModelCatalog; this struct
// keeps the capability-relevant subset only.
type runtimeEnvelope struct {
	Models []runtimeModelEnvelope `json:"models"`
}

// RecordFromRuntime parses a `codex debug models` JSON payload and returns
// a CapabilityRecord stamped with CapabilitySource = "runtime". The payload
// must (1) decode as JSON, (2) keep Reasoning disjoint from SpeedTiers and
// ServiceTiers, and (3) not carry "fast" or "priority" inside Reasoning —
// those are speed/service selectors, never reasoning effort values.
//
// This helper still treats the payload as a flat object with top-level
// reasoning/speed_tiers/service_tiers. The real envelope wraps per-model
// entries inside `models: [...]`; that case is handled by
// RecordFromRuntimeForModel. This flat parser survives for any caller that
// feeds a hand-curated JSON literal at the flat level.
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

// RecordFromRuntimeForModel parses the real `codex debug models` envelope
// shape `{"models":[{"slug":"...","reasoning":[...],...},...]}` and
// returns the CapabilityRecord for the entry whose slug matches modelID.
// It returns (zero, err) on malformed JSON, missing `models` key, or no
// matching slug so the caller can distinguish "missing model" from "empty
// capabilities for present model" and fall back to the curated matrix.
//
// The matched record is stamped CapabilitySource = "runtime"; missing or
// empty slice fields inside the entry are permitted (the CLI may omit
// service_tiers on a model that only offers reasoning tiers, for example).
func RecordFromRuntimeForModel(payload []byte, modelID string) (CapabilityRecord, error) {
	var env runtimeEnvelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return CapabilityRecord{}, fmt.Errorf("codex capabilities: parse runtime envelope: %w", err)
	}

	for _, entry := range env.Models {
		if entry.Slug != modelID {
			continue
		}
		rec := CapabilityRecord{
			Reasoning:         append([]string(nil), entry.Reasoning...),
			SpeedTiers:        append([]string(nil), entry.SpeedTiers...),
			ServiceTiers:      append([]string(nil), entry.ServiceTiers...),
			MultiAgentVersion: entry.MultiAgentVer,
			CapabilitySource:  SourceRuntime,
		}
		if err := Validate(rec); err != nil {
			return CapabilityRecord{}, fmt.Errorf("model %q in runtime envelope: %w", modelID, err)
		}
		return rec, nil
	}
	return CapabilityRecord{}, fmt.Errorf("codex capabilities: model %q not present in runtime envelope", modelID)
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
// both slices, with case-insensitive comparison so "Fast" in SpeedTiers
// and "fast" in Reasoning are caught the same way containsValue catches
// "Fast" alone in Reasoning. Returns the empty string when the slices
// are disjoint. The curated matrix and the runtime payload both use
// lowercase identifiers, so case-insensitivity makes the two
// partition checks consistent rather than letting one slide past
// the other.
func firstOverlap(a, b []string) (string, string) {
	if len(a) == 0 || len(b) == 0 {
		return "", ""
	}
	seen := make(map[string]struct{}, len(b))
	for _, v := range b {
		seen[strings.ToLower(v)] = struct{}{}
	}
	for _, v := range a {
		if _, ok := seen[strings.ToLower(v)]; ok {
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
