package screens

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/opencode"
)

func writeVSCodeModelPickerCache(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "models.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write models cache: %v", err)
	}
	return path
}

func TestNewVSCodeModelPickerStateFiltersGithubCopilotModels(t *testing.T) {
	cachePath := writeVSCodeModelPickerCache(t, `{
		"github-copilot": {
			"id": "github-copilot",
			"name": "GitHub Copilot",
			"models": {
				"gpt-4.1": {"id": "gpt-4.1", "name": "GPT-4.1", "tool_call": true},
				"chat-only": {"id": "chat-only", "name": "Chat Only", "tool_call": false}
			}
		},
		"anthropic": {
			"id": "anthropic",
			"name": "Anthropic",
			"models": {
				"claude-sonnet": {"id": "claude-sonnet", "name": "Claude Sonnet", "tool_call": true}
			}
		}
	}`)

	state := NewVSCodeModelPickerState(cachePath)

	if !state.ForVSCode {
		t.Fatal("NewVSCodeModelPickerState() should mark the picker as VS Code-specific")
	}
	if got := state.AvailableIDs; len(got) != 1 || got[0] != "github-copilot" {
		t.Fatalf("AvailableIDs = %v, want [github-copilot]", got)
	}
	models := state.SDDModels["github-copilot"]
	if len(models) != 1 || models[0].ID != "gpt-4.1" {
		t.Fatalf("github-copilot models = %#v, want only gpt-4.1", models)
	}
	if _, exists := state.SDDModels["anthropic"]; exists {
		t.Fatalf("non-Copilot provider leaked into VS Code picker: %#v", state.SDDModels)
	}
}

func TestModelPickerRowsForVSCodeUseNativeAgentKeys(t *testing.T) {
	rows := ModelPickerRowsForVSCode()

	if rows[0] != VSCodeCoordinatorPhase {
		t.Fatalf("first row = %q, want %q", rows[0], VSCodeCoordinatorPhase)
	}
	if !containsRow(rows, "sdd-onboard") {
		t.Fatalf("VS Code rows should include sdd-onboard; got %v", rows)
	}
	for _, row := range rows {
		if strings.Contains(row, "-cheap") || strings.Contains(row, ".agent.md") {
			t.Fatalf("VS Code row %q looks like an out-of-scope named/suffixed profile", row)
		}
	}
}

func TestRenderVSCodeModelPickerDistinguishesCoordinatorAndExplainsPrecedence(t *testing.T) {
	state := ModelPickerState{
		ForVSCode:    true,
		AvailableIDs: []string{"github-copilot"},
		Providers: map[string]opencode.Provider{
			"github-copilot": {ID: "github-copilot", Name: "GitHub Copilot", Models: map[string]opencode.Model{
				"gpt-4.1": {ID: "gpt-4.1", Name: "GPT-4.1"},
			}},
		},
	}
	assignments := map[string]model.ModelAssignment{
		VSCodeCoordinatorPhase: {ProviderID: "github-copilot", ModelID: "gpt-4.1"},
		"sdd-apply":            {ProviderID: "github-copilot", ModelID: "gpt-4.1"},
	}

	output := RenderModelPicker(assignments, state, 0)

	for _, want := range []string{
		"VS Code Copilot SDD",
		"sdd-orchestrator (coordinator)",
		"sdd-apply",
		"runSubagent",
		"frontmatter model",
		"parent model",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("RenderModelPicker() missing %q; got:\n%s", want, output)
		}
	}

	coordinatorAssignment := "sdd-orchestrator (coordinator) GitHub Copilot / GPT-4.1"
	if !strings.Contains(output, coordinatorAssignment) {
		t.Fatalf("RenderModelPicker() should show the saved VS Code coordinator model assignment %q; got:\n%s", coordinatorAssignment, output)
	}
}

func TestRenderVSCodeModelPickerDoesNotUseOpenCodeCoordinatorAssignment(t *testing.T) {
	state := ModelPickerState{
		ForVSCode:    true,
		AvailableIDs: []string{"github-copilot"},
		Providers: map[string]opencode.Provider{
			"github-copilot": {ID: "github-copilot", Name: "GitHub Copilot", Models: map[string]opencode.Model{
				"gpt-4.1": {ID: "gpt-4.1", Name: "GPT-4.1"},
			}},
		},
	}
	assignments := map[string]model.ModelAssignment{
		SDDOrchestratorPhase: {ProviderID: "github-copilot", ModelID: "gpt-4.1"},
	}

	output := RenderModelPicker(assignments, state, 0)

	if !strings.Contains(output, "sdd-orchestrator (coordinator) (default)") {
		t.Fatalf("RenderModelPicker() should keep the VS Code coordinator row on the VS Code assignment key; got:\n%s", output)
	}
	if strings.Contains(output, "sdd-orchestrator (coordinator) GitHub Copilot / GPT-4.1") {
		t.Fatalf("RenderModelPicker() leaked the OpenCode coordinator assignment into VS Code output; got:\n%s", output)
	}
}

func TestRenderVSCodeModelPickerMissingCacheAllowsInheritedModel(t *testing.T) {
	state := NewVSCodeModelPickerState(filepath.Join(t.TempDir(), "missing-models.json"))

	output := RenderModelPicker(nil, state, 0)

	for _, want := range []string{
		"GitHub Copilot model data is unavailable",
		"Continue with inherited parent model",
		"frontmatter model",
		"parent model",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("RenderModelPicker() missing %q; got:\n%s", want, output)
		}
	}
}

func TestRenderVSCodeModelPickerEmptyCopilotProviderAllowsInheritedModel(t *testing.T) {
	cachePath := writeVSCodeModelPickerCache(t, `{
		"github-copilot": {
			"id": "github-copilot",
			"name": "GitHub Copilot",
			"models": {
				"chat-only": {"id": "chat-only", "name": "Chat Only", "tool_call": false}
			}
		}
	}`)

	state := NewVSCodeModelPickerState(cachePath)
	output := RenderModelPicker(nil, state, 0)

	if len(state.AvailableIDs) != 0 {
		t.Fatalf("AvailableIDs = %v, want empty when GitHub Copilot has no tool-capable models", state.AvailableIDs)
	}
	if !strings.Contains(output, "Continue with inherited parent model") {
		t.Fatalf("empty provider output should allow inherited model; got:\n%s", output)
	}
}

func TestRenderVSCodeModelPickerWarnsAboutUnvalidatedAssignments(t *testing.T) {
	state := ModelPickerState{
		ForVSCode:    true,
		AvailableIDs: []string{"github-copilot"},
		Providers: map[string]opencode.Provider{
			"github-copilot": {ID: "github-copilot", Name: "GitHub Copilot", Models: map[string]opencode.Model{
				"gpt-4.1": {ID: "gpt-4.1", Name: "GPT-4.1"},
			}},
		},
	}
	assignments := map[string]model.ModelAssignment{
		"sdd-apply": {ProviderID: "github-copilot", ModelID: "gpt-4.1"},
	}

	output := RenderModelPicker(assignments, state, 0)

	for _, want := range []string{"cannot validate Copilot cost tier", "may be omitted"} {
		if !strings.Contains(output, want) {
			t.Fatalf("RenderModelPicker() missing %q; got:\n%s", want, output)
		}
	}
}

func containsRow(rows []string, want string) bool {
	for _, row := range rows {
		if row == want {
			return true
		}
	}
	return false
}
