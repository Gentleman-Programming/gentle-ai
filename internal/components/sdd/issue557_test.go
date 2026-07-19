package sdd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

// TestGenerateProfileOverlay_FallsBackToGlobalAssignments reproduces issue #557:
//
// The TUI "Configure Models" flow (ScreenModelConfig) populates
// Selection.ModelAssignments with normalized phase keys (sdd-init, sdd-onboard,
// ...). The user expects these assignments to propagate to the
// profile-specific generated agents (sdd-init-homework, sdd-onboard-homework,
// ...) — at least when the profile itself has not explicitly cleared the
// assignment. Today, GenerateProfileOverlay only consults the profile's own
// PhaseAssignments; if the user did not touch a row in the profile picker
// (e.g. because they used the global flow, or because the picker showed
// "(default)" for a phase that the disk had previously lost its model on),
// the generated *-homework agent is missing the model field and OpenCode
// shows "Unassigned" for that homework agent.
//
// Contract under test: for a profile with no explicit assignment for a phase,
// the overlay should fall back to the global OpenCodeModelAssignments entry
// for the same phase, so generated agents stay consistent with the rest of
// the gentle-ai UI.
func TestGenerateProfileOverlay_FallsBackToGlobalAssignments(t *testing.T) {
	home := t.TempDir()

	// Profile draft that only explicitly assigns orchestrator + a couple of
	// phases (mirrors a user who touched a few rows in the picker but skipped
	// the rest, or who used the global flow to set most assignments).
	profile := model.Profile{
		Name:              "homework",
		OrchestratorModel: model.ModelAssignment{ProviderID: "openai", ModelID: "gpt-5.1"},
		PhaseAssignments: map[string]model.ModelAssignment{
			"sdd-apply": {ProviderID: "openai", ModelID: "gpt-5.1"},
		},
	}

	// Global assignments (what ScreenModelConfig → Configure OpenCode Models
	// populates) — this is the user's source of truth for "what Gentle AI
	// shows as assigned" in the global view.
	globalAssignments := map[string]model.ModelAssignment{
		"gentle-orchestrator": {ProviderID: "openai", ModelID: "gpt-5.1"},
		"sdd-init":            {ProviderID: "openai", ModelID: "gpt-5.1"},
		"sdd-explore":         {ProviderID: "openai", ModelID: "gpt-5.1"},
		"sdd-onboard":         {ProviderID: "openai", ModelID: "gpt-5.1"},
		"sdd-propose":         {ProviderID: "openai", ModelID: "gpt-5.1"},
		"sdd-spec":            {ProviderID: "openai", ModelID: "gpt-5.1"},
		"sdd-design":          {ProviderID: "openai", ModelID: "gpt-5.1"},
		"sdd-tasks":           {ProviderID: "openai", ModelID: "gpt-5.1"},
		"sdd-verify":          {ProviderID: "openai", ModelID: "gpt-5.1"},
		"sdd-archive":         {ProviderID: "openai", ModelID: "gpt-5.1"},
	}

	overlay, err := GenerateProfileOverlay(profile, home, globalAssignments, "")
	if err != nil {
		t.Fatalf("GenerateProfileOverlay() error = %v", err)
	}

	var root map[string]any
	if err := json.Unmarshal(overlay, &root); err != nil {
		t.Fatalf("unmarshal overlay: %v", err)
	}
	agentMap, ok := root["agent"].(map[string]any)
	if !ok {
		t.Fatalf("overlay missing agent map: %#v", root)
	}

	// Every phase present in globalAssignments must produce a suffixed agent
	// with the assigned model. The user-visible symptom of issue #557 is that
	// sdd-onboard-homework specifically is missing the model field; we assert
	// it (and the rest) to lock the contract.
	for phase, want := range globalAssignments {
		var key string
		if phase == "gentle-orchestrator" {
			key = "sdd-orchestrator-homework"
		} else {
			key = phase + "-homework"
		}
		entry, ok := agentMap[key].(map[string]any)
		if !ok {
			t.Errorf("missing generated agent %q for phase %q (fallback expected)", key, phase)
			continue
		}
		gotModel, hasModel := entry["model"].(string)
		if !hasModel {
			t.Errorf("issue #557: agent %q missing model field despite global assignment %v", key, want)
			continue
		}
		if gotModel != want.FullID() {
			t.Errorf("agent %q model = %q, want %q", key, gotModel, want.FullID())
		}
	}

	// Explicit profile overrides take precedence — confirm sdd-apply-homework
	// gets the profile-level assignment (which happens to equal the global
	// fallback in this test, but the contract is "profile wins over global").
	applyEntry, ok := agentMap["sdd-apply-homework"].(map[string]any)
	if !ok {
		t.Fatal("missing sdd-apply-homework agent")
	}
	if m, _ := applyEntry["model"].(string); m != "openai/gpt-5.1" {
		t.Errorf("sdd-apply-homework model = %q, want openai/gpt-5.1", m)
	}
}

// TestInjectHomeworkProfileFallsBackToGlobalAssignmentsViaInject is the full
// end-to-end reproduction of issue #557:
//
//  1. opencode.json already contains a homework profile where most phase
//     agents carry a model but sdd-onboard-homework was generated without one.
//  2. A regular sync is run with a global OpenCodeModelAssignments entry for
//     sdd-onboard (mirrors the user using "Configure Models" in the TUI).
//  3. After sync, sdd-onboard-homework MUST carry the model from the global
//     assignment — otherwise OpenCode reports it as "Unassigned" while
//     gentle-ai's UI shows sdd-onboard as assigned.
func TestInjectHomeworkProfileFallsBackToGlobalAssignmentsViaInject(t *testing.T) {
	home := t.TempDir()
	mockNoPackageManager(t)

	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Seed: homework profile exists with most phases assigned, but
	// sdd-onboard-homework is missing the model — the user-reported state.
	seed := `{
  "agent": {
    "gentle-orchestrator": { "mode": "primary" },
    "sdd-init": { "mode": "subagent", "model": "openai/gpt-5.1" },
    "sdd-orchestrator-homework": { "mode": "primary", "model": "openai/gpt-5.1" },
    "sdd-init-homework": { "mode": "subagent", "model": "openai/gpt-5.1" },
    "sdd-explore-homework": { "mode": "subagent", "model": "openai/gpt-5.1" },
    "sdd-propose-homework": { "mode": "subagent", "model": "openai/gpt-5.1" },
    "sdd-spec-homework": { "mode": "subagent", "model": "openai/gpt-5.1" },
    "sdd-design-homework": { "mode": "subagent", "model": "openai/gpt-5.1" },
    "sdd-tasks-homework": { "mode": "subagent", "model": "openai/gpt-5.1" },
    "sdd-apply-homework": { "mode": "subagent", "model": "openai/gpt-5.1" },
    "sdd-verify-homework": { "mode": "subagent", "model": "openai/gpt-5.1" },
    "sdd-archive-homework": { "mode": "subagent", "model": "openai/gpt-5.1" },
    "sdd-onboard-homework": {
      "description": "Guide user through a complete SDD cycle using their real codebase",
      "hidden": true,
      "mode": "subagent",
      "prompt": "{file:/tmp/prompts/sdd-onboard.md}",
      "tools": { "bash": true, "edit": true, "read": true, "write": true }
    }
  }
}`
	if err := os.WriteFile(settingsPath, []byte(seed), 0o644); err != nil {
		t.Fatalf("WriteFile seed: %v", err)
	}

	// Sanity check: the homework profile is detected with 9 phase assignments
	// (sdd-onboard is absent because sdd-onboard-homework has no model on disk).
	profiles, err := DetectProfiles(settingsPath)
	if err != nil {
		t.Fatalf("DetectProfiles() error = %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("DetectProfiles() returned %d profiles, want 1", len(profiles))
	}
	if got := len(profiles[0].PhaseAssignments); got != 9 {
		t.Errorf("seed profile has %d PhaseAssignments, want 9 (sdd-onboard missing); got %v", got, profiles[0].PhaseAssignments)
	}

	// Run a regular sync with the global assignments populated (mirrors the
	// user assigning models via the TUI's "Configure Models" flow and then
	// pressing Save & Sync). The detected homework profile is rebuilt from
	// disk by GenerateProfileOverlay, which now consults the global fallback.
	//
	// sdd.Inject does not auto-detect profiles from disk (that happens one
	// layer up in componentSyncStep.Run), so we pass the detected profile in
	// explicitly via opts.Profiles — same shape the orchestration layer hands
	// to sdd.Inject after DetectProfiles returns.
	global := map[string]model.ModelAssignment{
		"sdd-onboard": {ProviderID: "openai", ModelID: "gpt-5.1"},
	}
	if _, err := Inject(home, opencodeAdapter(), model.SDDModeMulti, InjectOptions{
		OpenCodeModelAssignments: global,
		Profiles:                 profiles,
	}); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var root map[string]any
	if err := json.Unmarshal(content, &root); err != nil {
		t.Fatalf("Unmarshal opencode.json: %v", err)
	}
	agentMap, ok := root["agent"].(map[string]any)
	if !ok {
		t.Fatalf("opencode.json missing agent map: %#v", root)
	}

	// Issue #557: sdd-onboard-homework MUST now carry the global model.
	onboardAgent, ok := agentMap["sdd-onboard-homework"].(map[string]any)
	if !ok {
		t.Fatalf("sdd-onboard-homework missing after Inject; agent keys: %v", keysOf(agentMap))
	}
	gotModel, hasModel := onboardAgent["model"].(string)
	if !hasModel {
		t.Fatalf("issue #557: sdd-onboard-homework still missing model field after Inject; full entry: %#v", onboardAgent)
	}
	if gotModel != "openai/gpt-5.1" {
		t.Errorf("sdd-onboard-homework model = %q, want %q", gotModel, "openai/gpt-5.1")
	}

	// Sanity: the other homework agents retained their previous assignments.
	for _, phase := range []string{"sdd-init", "sdd-explore", "sdd-propose", "sdd-spec", "sdd-design", "sdd-tasks", "sdd-apply", "sdd-verify", "sdd-archive"} {
		key := phase + "-homework"
		entry, ok := agentMap[key].(map[string]any)
		if !ok {
			t.Errorf("missing %s after Inject", key)
			continue
		}
		if m, _ := entry["model"].(string); m != "openai/gpt-5.1" {
			t.Errorf("%s model = %q, want openai/gpt-5.1 (preserved from previous sync)", key, m)
		}
	}
}
