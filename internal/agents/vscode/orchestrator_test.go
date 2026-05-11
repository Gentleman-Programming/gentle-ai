package vscode

import (
	"os"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

// TestGenerateAgentFile_Orchestrator_DefaultProfile_HasAllRequiredFields verifies
// that the orchestrator agent renders with the YAML frontmatter fields VS Code
// Copilot requires to dispatch to sub-agents: tools, agents whitelist, and
// user-invocable.
func TestGenerateAgentFile_Orchestrator_DefaultProfile_HasAllRequiredFields(t *testing.T) {
	content := GenerateAgentFile(OrchestratorPhase, model.Profile{})

	mustContain := []string{
		"name: sdd-orchestrator\n",
		"tools: ['agent']\n",
		"agents:\n",
		"user-invocable: true\n",
		"readonly: false\n",
		"background: false\n",
	}
	for _, want := range mustContain {
		if !strings.Contains(content, want) {
			t.Errorf("orchestrator frontmatter missing %q\n--- content ---\n%s", want, content)
		}
	}

	// All 10 phases must appear in the agents whitelist, in canonical order,
	// unsuffixed for the default profile.
	for _, phase := range sddPhases {
		entry := "  - " + phase + "\n"
		if !strings.Contains(content, entry) {
			t.Errorf("orchestrator agents whitelist missing %q", entry)
		}
	}
}

// TestGenerateAgentFile_Orchestrator_NamedProfile_SuffixesAgentNames verifies
// that a named profile's orchestrator references the suffixed phase agents,
// not the unsuffixed defaults. Without this, the orchestrator would dispatch
// to the wrong agents.
func TestGenerateAgentFile_Orchestrator_NamedProfile_SuffixesAgentNames(t *testing.T) {
	profile := model.Profile{Name: "cheap"}
	content := GenerateAgentFile(OrchestratorPhase, profile)

	if !strings.Contains(content, "name: sdd-orchestrator-cheap\n") {
		t.Errorf("orchestrator name should be suffixed for named profile; content:\n%s", content)
	}

	for _, phase := range sddPhases {
		suffixed := "  - " + phase + "-cheap\n"
		if !strings.Contains(content, suffixed) {
			t.Errorf("orchestrator agents whitelist missing suffixed entry %q", suffixed)
		}
		// Unsuffixed entry must NOT be present in a named profile's orchestrator.
		unsuffixed := "  - " + phase + "\n"
		if strings.Contains(content, unsuffixed) {
			t.Errorf("named profile orchestrator must not list unsuffixed agent %q", unsuffixed)
		}
	}

	// Body references must also be suffixed so the dispatch instructions match
	// the agents whitelist.
	for _, phase := range []string{"sdd-explore", "sdd-apply", "sdd-verify", "sdd-archive"} {
		suffixedRef := "`" + phase + "-cheap`"
		if !strings.Contains(content, suffixedRef) {
			t.Errorf("orchestrator body must reference suffixed phase %q", suffixedRef)
		}
	}
}

// TestGenerateAgentFile_Orchestrator_OrchestratorModelAssignment verifies that
// the orchestrator's model field comes from Profile.OrchestratorModel (not from
// PhaseAssignments, which is for the executors). Empty assignment must omit the
// model line so Copilot uses its default.
func TestGenerateAgentFile_Orchestrator_OrchestratorModelAssignment(t *testing.T) {
	tests := []struct {
		name           string
		orchModel      model.ModelAssignment
		wantModelLine  string // "" → must omit
	}{
		{
			name:          "no model → omit field",
			orchModel:     model.ModelAssignment{},
			wantModelLine: "",
		},
		{
			name:          "claude sonnet → mapped display",
			orchModel:     model.ModelAssignment{ProviderID: "anthropic", ModelID: "claude-sonnet-4-20250514"},
			wantModelLine: "model: \"Claude Sonnet 4 (copilot)\"\n",
		},
		{
			name:          "gpt-5 unknown → provider/model fallback",
			orchModel:     model.ModelAssignment{ProviderID: "openai", ModelID: "gpt-5-future"},
			wantModelLine: "model: \"openai/gpt-5-future\"\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := model.Profile{Name: "cheap", OrchestratorModel: tt.orchModel}
			content := GenerateAgentFile(OrchestratorPhase, profile)

			if tt.wantModelLine == "" {
				if strings.Contains(content, "model:") {
					t.Errorf("orchestrator should omit model line when no assignment; got:\n%s", content)
				}
				return
			}
			if !strings.Contains(content, tt.wantModelLine) {
				t.Errorf("orchestrator model line = missing %q; content:\n%s", tt.wantModelLine, content)
			}
		})
	}
}

// TestGenerateVSCodeProfileFiles_IncludesOrchestrator verifies that a named
// profile generates 11 files (orchestrator + 10 phase executors), not 10.
// Regression guard against the earlier 10-only design.
func TestGenerateVSCodeProfileFiles_IncludesOrchestrator(t *testing.T) {
	agentsDir := t.TempDir()
	profile := model.Profile{
		Name: "premium",
		OrchestratorModel: model.ModelAssignment{
			ProviderID: "anthropic", ModelID: "claude-opus-4-5",
		},
		PhaseAssignments: map[string]model.ModelAssignment{
			"sdd-apply": {ProviderID: "anthropic", ModelID: "claude-sonnet-4"},
		},
	}

	files, err := GenerateVSCodeProfileFiles(profile, agentsDir)
	if err != nil {
		t.Fatalf("GenerateVSCodeProfileFiles() error = %v", err)
	}
	if got, want := len(files), 11; got != want {
		t.Fatalf("GenerateVSCodeProfileFiles() wrote %d files, want %d (orchestrator + 10 phases)", got, want)
	}

	// Orchestrator file must exist with the suffix.
	orchPath := agentsDir + "/sdd-orchestrator-premium.agent.md"
	for _, f := range files {
		if strings.HasSuffix(f, "sdd-orchestrator-premium.agent.md") {
			orchPath = f
			break
		}
	}
	if orchPath == "" {
		t.Fatalf("orchestrator file not in returned list; files: %v", files)
	}
}

// TestRemoveVSCodeProfileAgents_AlsoRemovesOrchestrator verifies that removing
// a profile cleans up its orchestrator file alongside the 10 phase files.
// Without this, the orchestrator would linger and silently dispatch to
// nonexistent suffixed agents.
func TestRemoveVSCodeProfileAgents_AlsoRemovesOrchestrator(t *testing.T) {
	agentsDir := t.TempDir()
	profile := model.Profile{Name: "cheap"}
	if _, err := GenerateVSCodeProfileFiles(profile, agentsDir); err != nil {
		t.Fatalf("setup: GenerateVSCodeProfileFiles() error = %v", err)
	}

	if err := RemoveVSCodeProfileAgents(agentsDir, "cheap"); err != nil {
		t.Fatalf("RemoveVSCodeProfileAgents() error = %v", err)
	}

	// Orchestrator file must be gone.
	orchPath := agentsDir + "/sdd-orchestrator-cheap.agent.md"
	if _, err := os.Stat(orchPath); err == nil {
		t.Errorf("sdd-orchestrator-cheap.agent.md still exists after removal")
	}
}
