package sdd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

// TestProfileLifecycle_FullCRUD exercises the complete profile lifecycle:
// write shared prompts → create profile overlay → merge → detect → edit → remove → detect.
func TestProfileLifecycle_FullCRUD(t *testing.T) {
	home := t.TempDir()
	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")

	// Create directory for the settings file.
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Step 1: Write a minimal opencode.json with just a model field.
	initialJSON := `{
  "model": "anthropic:claude-sonnet-4-5"
}`
	if err := os.WriteFile(settingsPath, []byte(initialJSON), 0o644); err != nil {
		t.Fatalf("write initial opencode.json: %v", err)
	}

	// Step 2: WriteSharedPromptFiles — expect 10 files created.
	changed, err := WriteSharedPromptFiles(home)
	if err != nil {
		t.Fatalf("WriteSharedPromptFiles(): %v", err)
	}
	if !changed {
		t.Error("WriteSharedPromptFiles() changed = false on first call, want true")
	}

	promptDir := SharedPromptDir(home)
	for _, phase := range subAgentPhaseOrder {
		path := filepath.Join(promptDir, phase+".md")
		info, statErr := os.Stat(path)
		if statErr != nil {
			t.Errorf("shared prompt file %q not found: %v", path, statErr)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("shared prompt file %q is empty", path)
		}
	}

	// Step 3: Create Profile{Name:"cheap", OrchestratorModel: haiku}.
	haikuModel := model.ModelAssignment{ProviderID: "anthropic", ModelID: "claude-haiku-3-5"}
	cheapProfile := model.Profile{
		Name:              "cheap",
		OrchestratorModel: haikuModel,
	}

	overlayBytes, err := GenerateProfileOverlay(cheapProfile, home)
	if err != nil {
		t.Fatalf("GenerateProfileOverlay(): %v", err)
	}

	// Step 4: Merge overlay into opencode.json → verify 11 agent keys for "cheap".
	baseJSON, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(opencode.json): %v", err)
	}
	merged, err := filemerge.MergeJSONObjects(baseJSON, overlayBytes)
	if err != nil {
		t.Fatalf("MergeJSONObjects(): %v", err)
	}
	if _, writeErr := filemerge.WriteFileAtomic(settingsPath, merged, 0o644); writeErr != nil {
		t.Fatalf("WriteFileAtomic(): %v", writeErr)
	}

	// Verify 11 agent keys for "cheap" profile.
	var root map[string]any
	if err := json.Unmarshal(merged, &root); err != nil {
		t.Fatalf("unmarshal merged: %v", err)
	}
	agentMap, ok := root["agent"].(map[string]any)
	if !ok {
		t.Fatal("merged JSON missing 'agent' map")
	}
	expectedCheapKeys := ProfileAgentKeys("cheap")
	for _, key := range expectedCheapKeys {
		if _, exists := agentMap[key]; !exists {
			t.Errorf("merged agent map missing key %q", key)
		}
	}
	if len(expectedCheapKeys) != 11 {
		t.Errorf("expected 11 cheap profile keys, got %d", len(expectedCheapKeys))
	}

	// Step 5: DetectProfiles → verify 1 profile detected with correct model.
	profiles, err := DetectProfiles(settingsPath)
	if err != nil {
		t.Fatalf("DetectProfiles() after create: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("DetectProfiles() returned %d profiles, want 1", len(profiles))
	}
	if profiles[0].Name != "cheap" {
		t.Errorf("profile Name = %q, want %q", profiles[0].Name, "cheap")
	}
	if profiles[0].OrchestratorModel.ProviderID != "anthropic" {
		t.Errorf("OrchestratorModel.ProviderID = %q, want %q", profiles[0].OrchestratorModel.ProviderID, "anthropic")
	}
	if profiles[0].OrchestratorModel.ModelID != "claude-haiku-3-5" {
		t.Errorf("OrchestratorModel.ModelID = %q, want %q", profiles[0].OrchestratorModel.ModelID, "claude-haiku-3-5")
	}

	// Step 6: Edit — create Profile{Name:"cheap", OrchestratorModel: sonnet} → generate new overlay → merge.
	sonnetModel := model.ModelAssignment{ProviderID: "anthropic", ModelID: "claude-sonnet-4-5"}
	editedProfile := model.Profile{
		Name:              "cheap",
		OrchestratorModel: sonnetModel,
	}
	editOverlayBytes, err := GenerateProfileOverlay(editedProfile, home)
	if err != nil {
		t.Fatalf("GenerateProfileOverlay() for edit: %v", err)
	}

	baseJSON2, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(opencode.json) before edit merge: %v", err)
	}
	merged2, err := filemerge.MergeJSONObjects(baseJSON2, editOverlayBytes)
	if err != nil {
		t.Fatalf("MergeJSONObjects() for edit: %v", err)
	}
	if _, writeErr := filemerge.WriteFileAtomic(settingsPath, merged2, 0o644); writeErr != nil {
		t.Fatalf("WriteFileAtomic() after edit: %v", writeErr)
	}

	// Verify model changed.
	profilesAfterEdit, err := DetectProfiles(settingsPath)
	if err != nil {
		t.Fatalf("DetectProfiles() after edit: %v", err)
	}
	if len(profilesAfterEdit) != 1 {
		t.Fatalf("DetectProfiles() after edit returned %d profiles, want 1", len(profilesAfterEdit))
	}
	if profilesAfterEdit[0].OrchestratorModel.ModelID != "claude-sonnet-4-5" {
		t.Errorf("after edit: OrchestratorModel.ModelID = %q, want %q",
			profilesAfterEdit[0].OrchestratorModel.ModelID, "claude-sonnet-4-5")
	}

	// Step 7: RemoveProfileAgents → verify 11 keys removed.
	if err := RemoveProfileAgents(settingsPath, "cheap"); err != nil {
		t.Fatalf("RemoveProfileAgents(): %v", err)
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile() after remove: %v", err)
	}
	var rootAfterRemove map[string]any
	if err := json.Unmarshal(data, &rootAfterRemove); err != nil {
		t.Fatalf("unmarshal after remove: %v", err)
	}
	agentMapAfterRemove, hasAgent := rootAfterRemove["agent"].(map[string]any)
	if hasAgent {
		for _, key := range expectedCheapKeys {
			if _, exists := agentMapAfterRemove[key]; exists {
				t.Errorf("key %q still present after RemoveProfileAgents", key)
			}
		}
	}
	// (agent map may be empty or missing, both are valid after full removal of the only profile)

	// Step 8: DetectProfiles → verify 0 profiles detected.
	profilesAfterRemove, err := DetectProfiles(settingsPath)
	if err != nil {
		t.Fatalf("DetectProfiles() after remove: %v", err)
	}
	if len(profilesAfterRemove) != 0 {
		t.Errorf("DetectProfiles() after remove returned %d profiles, want 0", len(profilesAfterRemove))
	}

	// Step 9: Verify shared prompt files still exist (not deleted by remove).
	for _, phase := range subAgentPhaseOrder {
		path := filepath.Join(promptDir, phase+".md")
		if _, statErr := os.Stat(path); statErr != nil {
			t.Errorf("shared prompt file %q was deleted by RemoveProfileAgents — should NOT be: %v", path, statErr)
		}
	}
}

// TestProfileLifecycle_TwoProfiles verifies create + detect + remove for two concurrent profiles.
func TestProfileLifecycle_TwoProfiles(t *testing.T) {
	home := t.TempDir()
	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")

	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte(`{"model": "anthropic:claude-sonnet-4-5"}`), 0o644); err != nil {
		t.Fatalf("write initial settings: %v", err)
	}

	if _, err := WriteSharedPromptFiles(home); err != nil {
		t.Fatalf("WriteSharedPromptFiles(): %v", err)
	}

	// Create cheap profile.
	cheapOverlay, err := GenerateProfileOverlay(model.Profile{
		Name:              "cheap",
		OrchestratorModel: model.ModelAssignment{ProviderID: "anthropic", ModelID: "claude-haiku-3-5"},
	}, home)
	if err != nil {
		t.Fatalf("GenerateProfileOverlay(cheap): %v", err)
	}

	// Create premium profile.
	premiumOverlay, err := GenerateProfileOverlay(model.Profile{
		Name:              "premium",
		OrchestratorModel: model.ModelAssignment{ProviderID: "anthropic", ModelID: "claude-opus-4-5"},
	}, home)
	if err != nil {
		t.Fatalf("GenerateProfileOverlay(premium): %v", err)
	}

	// Merge both overlays sequentially.
	base, _ := os.ReadFile(settingsPath)
	merged1, _ := filemerge.MergeJSONObjects(base, cheapOverlay)
	merged2, _ := filemerge.MergeJSONObjects(merged1, premiumOverlay)
	filemerge.WriteFileAtomic(settingsPath, merged2, 0o644)

	// Detect: 2 profiles.
	profiles, err := DetectProfiles(settingsPath)
	if err != nil {
		t.Fatalf("DetectProfiles() with 2 profiles: %v", err)
	}
	if len(profiles) != 2 {
		t.Fatalf("DetectProfiles() = %d profiles, want 2", len(profiles))
	}

	// Remove one.
	if err := RemoveProfileAgents(settingsPath, "cheap"); err != nil {
		t.Fatalf("RemoveProfileAgents(cheap): %v", err)
	}

	// Detect: 1 profile (premium).
	remaining, err := DetectProfiles(settingsPath)
	if err != nil {
		t.Fatalf("DetectProfiles() after removing cheap: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("DetectProfiles() after removing cheap = %d profiles, want 1", len(remaining))
	}
	if remaining[0].Name != "premium" {
		t.Errorf("remaining profile Name = %q, want %q", remaining[0].Name, "premium")
	}
}
