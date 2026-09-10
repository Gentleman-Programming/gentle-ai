package cli_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/pi"
	"github.com/gentleman-programming/gentle-ai/v2/internal/cli"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
)

func TestPiModelConfigSyncExecution(t *testing.T) {
	home := t.TempDir()

	assignments := map[string]model.PiAgentModelEntry{
		"sdd-apply":  {Model: "anthropic/claude-sonnet-4", Thinking: "medium"},
		"jd-judge-a": {Model: "anthropic/claude-opus-4", Thinking: "high"},
	}

	selection := model.Selection{
		Agents:             []model.AgentID{model.AgentPi},
		PiModelAssignments: assignments,
		PiSubscription:     model.PiSubscriptionClaude,
	}

	result, err := cli.RunSyncWithSelection(home, selection)
	if err != nil {
		t.Fatalf("RunSyncWithSelection failed: %v", err)
	}

	// Verify models.json was written
	modelsPath := pi.ModelsConfigPath(home)
	data, err := os.ReadFile(modelsPath)
	if err != nil {
		t.Fatalf("reading models.json: %v", err)
	}

	var readBack map[string]model.PiAgentModelEntry
	if err := json.Unmarshal(data, &readBack); err != nil {
		t.Fatalf("unmarshaling models.json: %v", err)
	}
	if readBack["sdd-apply"].Model != "anthropic/claude-sonnet-4" {
		t.Errorf("expected claude-sonnet-4, got %s", readBack["sdd-apply"].Model)
	}

	// Verify subagents.json was updated
	subagentsPath := pi.SubagentsConfigPath(home)
	subData, err := os.ReadFile(subagentsPath)
	if err != nil {
		t.Fatalf("reading subagents.json: %v", err)
	}

	var parsedSubagents map[string]any
	if err := json.Unmarshal(subData, &parsedSubagents); err != nil {
		t.Fatalf("unmarshaling subagents.json: %v", err)
	}
	profiles, ok := parsedSubagents["model_profiles"].(map[string]any)
	if !ok {
		t.Fatalf("model_profiles not found in subagents.json")
	}
	applyProf := profiles["sdd-apply"].(map[string]any)
	if applyProf["model"] != "anthropic/claude-sonnet-4" || applyProf["effort"] != "medium" {
		t.Errorf("unexpected sdd-apply profile: %v", applyProf)
	}

	_ = result
}

func TestPiModelConfigStatePersistence(t *testing.T) {
	home := t.TempDir()

	assignments := map[string]model.PiAgentModelEntry{
		"sdd-apply": {Model: "openai/gpt-5.6-terra", Thinking: "medium"},
	}

	s := state.InstallState{
		InstalledAgents: []string{string(model.AgentPi)},
		PiModelAssignments: map[string]state.PiModelEntryState{
			"sdd-apply": {Model: "openai/gpt-5.6-terra", Thinking: "medium"},
		},
		PiSubscription: string(model.PiSubscriptionCodex),
	}

	if err := state.Write(home, s); err != nil {
		t.Fatalf("state.Write failed: %v", err)
	}

	readState, err := state.Read(home)
	if err != nil {
		t.Fatalf("state.Read failed: %v", err)
	}

	if readState.PiSubscription != string(model.PiSubscriptionCodex) {
		t.Errorf("expected subscription codex, got %s", readState.PiSubscription)
	}
	if readState.PiModelAssignments["sdd-apply"].Model != assignments["sdd-apply"].Model {
		t.Errorf("expected gpt-5.6-terra, got %s", readState.PiModelAssignments["sdd-apply"].Model)
	}
}
