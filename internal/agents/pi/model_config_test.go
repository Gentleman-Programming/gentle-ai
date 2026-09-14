package pi_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/pi"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

func TestWriteModelsConfig(t *testing.T) {
	tempHome := t.TempDir()

	assignments := map[string]model.PiAgentModelEntry{
		"sdd-apply":   {Model: "anthropic/claude-sonnet-4", Thinking: "medium"},
		"jd-judge-a":  {Model: "anthropic/claude-opus-4", Thinking: "high"},
		"sdd-archive": {Model: "anthropic/claude-haiku-4", Thinking: "low"},
	}

	if err := pi.WriteModelsConfig(tempHome, assignments); err != nil {
		t.Fatalf("WriteModelsConfig failed: %v", err)
	}

	path := pi.ModelsConfigPath(tempHome)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading models.json: %v", err)
	}

	var readBack map[string]model.PiAgentModelEntry
	if err := json.Unmarshal(data, &readBack); err != nil {
		t.Fatalf("unmarshaling models.json: %v", err)
	}

	if len(readBack) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(readBack))
	}
	if readBack["sdd-apply"].Model != "anthropic/claude-sonnet-4" {
		t.Errorf("expected claude-sonnet-4, got %s", readBack["sdd-apply"].Model)
	}
	if readBack["sdd-apply"].Thinking != "medium" {
		t.Errorf("expected medium thinking, got %s", readBack["sdd-apply"].Thinking)
	}
}

func TestUpdateSubagentsModelProfilesPreservesExistingFields(t *testing.T) {
	tempHome := t.TempDir()
	subagentsPath := pi.SubagentsConfigPath(tempHome)

	if err := os.MkdirAll(filepath.Dir(subagentsPath), 0o755); err != nil {
		t.Fatalf("creating dir: %v", err)
	}

	initialJSON := `{
  "debug": true,
  "default_mode": "background",
  "max_concurrency": 5,
  "model_profiles": {
    "existing-agent": {
      "model": "existing/model",
      "effort": "low"
    }
  }
}`
	if err := os.WriteFile(subagentsPath, []byte(initialJSON), 0o644); err != nil {
		t.Fatalf("writing initial subagents.json: %v", err)
	}

	assignments := map[string]model.PiAgentModelEntry{
		"sdd-apply": {Model: "openai/gpt-5.6-terra", Thinking: "medium"},
	}

	if err := pi.UpdateSubagentsModelProfiles(tempHome, assignments); err != nil {
		t.Fatalf("UpdateSubagentsModelProfiles failed: %v", err)
	}

	data, err := os.ReadFile(subagentsPath)
	if err != nil {
		t.Fatalf("reading updated subagents.json: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshaling updated subagents.json: %v", err)
	}

	if parsed["debug"] != true {
		t.Errorf("expected debug to be preserved as true")
	}
	if parsed["default_mode"] != "background" {
		t.Errorf("expected default_mode to be background")
	}

	profiles, ok := parsed["model_profiles"].(map[string]any)
	if !ok {
		t.Fatalf("expected model_profiles object in parsed json")
	}

	if _, ok := profiles["existing-agent"]; !ok {
		t.Errorf("expected existing-agent to be preserved")
	}

	applyProfile, ok := profiles["sdd-apply"].(map[string]any)
	if !ok {
		t.Fatalf("expected sdd-apply profile to exist")
	}
	if applyProfile["model"] != "openai/gpt-5.6-terra" {
		t.Errorf("expected gpt-5.6-terra, got %v", applyProfile["model"])
	}
	if applyProfile["effort"] != "medium" {
		t.Errorf("expected effort medium, got %v", applyProfile["effort"])
	}
}
