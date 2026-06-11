package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/state"
)

func TestMergeExplicitAgentInstallStatePreservesExistingAssignmentsWhenFreshStateIsEmpty(t *testing.T) {
	home := t.TempDir()
	if err := state.Write(home, state.InstallState{
		InstalledAgents: []string{"opencode"},
		ModelAssignments: map[string]state.ModelAssignmentState{
			"sdd-init": {ProviderID: "anthropic", ModelID: "claude-sonnet-4"},
		},
		ClaudeModelAssignments: map[string]string{
			"sdd-apply": "opus",
		},
		KiroModelAssignments: map[string]string{
			"sdd-design": "auto",
		},
		CodexModelAssignments: map[string]string{
			"sdd-apply": "low",
		},
		CodexCarrilModelAssignments: map[string]string{
			"sdd-strong": "gpt-5.5",
		},
		CodexPhaseModelAssignments: map[string]string{
			"sdd-verify": "gpt-5.4",
		},
		Persona: "neutral",
	}); err != nil {
		t.Fatalf("state.Write: %v", err)
	}

	merged, ok := mergeExplicitAgentInstallState(home, state.InstallState{InstalledAgents: []string{"codex"}, Persona: "gentleman"}, []string{"codex"})
	if !ok {
		t.Fatal("mergeExplicitAgentInstallState ok = false, want true")
	}
	if got := merged.InstalledAgents; len(got) != 2 || got[0] != "opencode" || got[1] != "codex" {
		t.Fatalf("InstalledAgents = %#v, want [opencode codex]", got)
	}
	if merged.ModelAssignments["sdd-init"].ModelID != "claude-sonnet-4" {
		t.Fatalf("ModelAssignments not preserved: %#v", merged.ModelAssignments)
	}
	if merged.ClaudeModelAssignments["sdd-apply"] != "opus" {
		t.Fatalf("ClaudeModelAssignments not preserved: %#v", merged.ClaudeModelAssignments)
	}
	if merged.KiroModelAssignments["sdd-design"] != "auto" {
		t.Fatalf("KiroModelAssignments not preserved: %#v", merged.KiroModelAssignments)
	}
	if merged.CodexModelAssignments["sdd-apply"] != "low" {
		t.Fatalf("CodexModelAssignments not preserved: %#v", merged.CodexModelAssignments)
	}
	if merged.CodexCarrilModelAssignments["sdd-strong"] != "gpt-5.5" {
		t.Fatalf("CodexCarrilModelAssignments not preserved: %#v", merged.CodexCarrilModelAssignments)
	}
	if merged.CodexPhaseModelAssignments["sdd-verify"] != "gpt-5.4" {
		t.Fatalf("CodexPhaseModelAssignments not preserved: %#v", merged.CodexPhaseModelAssignments)
	}
	if merged.Persona != "neutral" {
		t.Fatalf("Persona = %q, want existing neutral", merged.Persona)
	}
}

func TestMergeExplicitAgentInstallStatePreservesFreshAssignments(t *testing.T) {
	home := t.TempDir()
	if err := state.Write(home, state.InstallState{
		InstalledAgents: []string{"opencode"},
		CodexModelAssignments: map[string]string{
			"sdd-apply": "low",
		},
	}); err != nil {
		t.Fatalf("state.Write: %v", err)
	}

	fresh := state.InstallState{
		InstalledAgents: []string{"codex"},
		CodexModelAssignments: codexEffortsToStrings(map[string]model.CodexEffort{
			"sdd-apply": model.CodexEffortHigh,
		}),
		CodexCarrilModelAssignments: map[string]string{
			"sdd-strong": "gpt-5.5",
		},
		CodexPhaseModelAssignments: map[string]string{
			"sdd-apply": "gpt-5.4",
		},
		Persona: "gentleman",
	}

	merged, ok := mergeExplicitAgentInstallState(home, fresh, []string{"codex"})
	if !ok {
		t.Fatal("mergeExplicitAgentInstallState ok = false, want true")
	}
	if got := merged.InstalledAgents; len(got) != 2 || got[0] != "opencode" || got[1] != "codex" {
		t.Fatalf("InstalledAgents = %#v, want [opencode codex]", got)
	}
	if merged.CodexModelAssignments["sdd-apply"] != "high" {
		t.Fatalf("CodexModelAssignments[sdd-apply] = %q, want high", merged.CodexModelAssignments["sdd-apply"])
	}
	if merged.CodexCarrilModelAssignments["sdd-strong"] != "gpt-5.5" {
		t.Fatalf("CodexCarrilModelAssignments not preserved: %#v", merged.CodexCarrilModelAssignments)
	}
	if merged.CodexPhaseModelAssignments["sdd-apply"] != "gpt-5.4" {
		t.Fatalf("CodexPhaseModelAssignments not preserved: %#v", merged.CodexPhaseModelAssignments)
	}
	if merged.Persona != "gentleman" {
		t.Fatalf("Persona = %q, want gentleman", merged.Persona)
	}
}

func TestMergeExplicitAgentInstallStateSkipsCorruptState(t *testing.T) {
	home := t.TempDir()
	statePath := state.Path(home)
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(statePath, []byte("{not valid json\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, ok := mergeExplicitAgentInstallState(home, state.InstallState{InstalledAgents: []string{"codex"}}, []string{"codex"})
	if ok {
		t.Fatal("mergeExplicitAgentInstallState ok = true for corrupt state, want false")
	}
}
