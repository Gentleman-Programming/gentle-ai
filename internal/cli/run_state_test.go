package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
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

	merged, err := mergeExplicitAgentInstallState(home, state.InstallState{InstalledAgents: []string{"codex"}, Persona: "gentleman"}, []string{"codex"}, InstallFlags{})
	if err != nil {
		t.Fatalf("mergeExplicitAgentInstallState() error = %v, want nil", err)
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

func TestMergeExplicitAgentInstallStateMergesOnlyExplicitSelectionField(t *testing.T) {
	original := state.InstallState{InstalledAgents: []string{"opencode"}, SelectionConfigured: true, Components: []model.ComponentID{model.ComponentEngram}, Skills: []model.SkillID{model.SkillCommentWriter}, Preset: model.PresetCustom, SDDMode: model.SDDModeSingle, StrictTDD: true, Persona: "neutral"}
	fresh := state.InstallState{InstalledAgents: []string{"codex"}, SelectionConfigured: true, Components: []model.ComponentID{model.ComponentSDD}, Skills: []model.SkillID{model.SkillSDDInit}, Preset: model.PresetFullGentleman, SDDMode: model.SDDModeMulti, Persona: "gentleman"}
	cases := []InstallFlags{{Components: []string{"sdd"}}, {Skills: []string{"sdd-init"}}, {Preset: "full-gentleman"}, {SDDMode: "multi"}, {Persona: "gentleman"}}
	wants := []string{"[sdd]|[comment-writer]|custom|single|true|neutral", "[engram]|[sdd-init]|custom|single|true|neutral", "[engram]|[comment-writer]|full-gentleman|single|true|neutral", "[engram]|[comment-writer]|custom|multi|true|neutral", "[engram]|[comment-writer]|custom|single|true|gentleman"}
	for i, flags := range cases {
		home := t.TempDir()
		if err := state.Write(home, original); err != nil {
			t.Fatal(err)
		}
		got, err := mergeExplicitAgentInstallState(home, fresh, []string{"codex"}, flags)
		if key := fmt.Sprintf("%v|%v|%s|%s|%t|%s", got.Components, got.Skills, got.Preset, got.SDDMode, got.StrictTDD, got.Persona); err != nil || key != wants[i] {
			t.Errorf("flags %#v merged selection %s, err %v", flags, key, err)
		}
	}
}

func TestRunInstallPersistsConfiguredSelection(t *testing.T) {
	home := t.TempDir()
	original := osUserHomeDir
	osUserHomeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { osUserHomeDir = original })
	if _, err := RunInstall([]string{"--agent", "cursor", "--preset", "custom", "--sdd-mode", "multi"}, system.DetectionResult{}); err != nil {
		t.Fatal(err)
	}
	got, err := state.Read(home)
	if err != nil || !got.SelectionConfigured || got.Preset != model.PresetCustom || got.SDDMode != model.SDDModeMulti || len(got.Components) != 0 {
		t.Fatalf("persisted selection = %#v, err = %v", got, err)
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

	merged, err := mergeExplicitAgentInstallState(home, fresh, []string{"codex"}, InstallFlags{})
	if err != nil {
		t.Fatalf("mergeExplicitAgentInstallState() error = %v, want nil", err)
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

// TestMergeExplicitAgentInstallStateFailsHonestlyOnCorruptState was renamed
// from TestMergeExplicitAgentInstallStateSkipsCorruptState (install/sync
// surface audit finding 2). The old assertion (ok == false, no error) let
// RunInstall silently return (result, nil) on an unreadable/corrupted
// ~/.gentle-ai/state.json — the pipeline ran to completion, but the user's
// agent selection was never persisted and the CLI reported success anyway.
// The honest contract is: an unreadable existing state during an explicit
// `--agent` install must fail loudly instead of vanishing.
func TestMergeExplicitAgentInstallStateFailsHonestlyOnCorruptState(t *testing.T) {
	home := t.TempDir()
	statePath := state.Path(home)
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(statePath, []byte("{not valid json\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := mergeExplicitAgentInstallState(home, state.InstallState{InstalledAgents: []string{"codex"}}, []string{"codex"}, InstallFlags{})
	if err == nil {
		t.Fatal("mergeExplicitAgentInstallState() error = nil for corrupt state, want a non-nil error")
	}
}

// TestRunInstallFailsHonestlyWhenExistingStateIsCorruptDuringExplicitAgentInstall
// closes install/sync surface audit finding 2: previously, `gentle-ai install
// --agent X` against a corrupted ~/.gentle-ai/state.json completed the whole
// pipeline (files written, verification passed) and RunInstall returned
// (result, nil) -- reported success -- WITHOUT ever calling state.Write. The
// user believed the install fully completed; state.json stayed corrupted
// forever, silently breaking every future `gentle-ai sync`.
func TestRunInstallFailsHonestlyWhenExistingStateIsCorruptDuringExplicitAgentInstall(t *testing.T) {
	home := t.TempDir()
	original := osUserHomeDir
	osUserHomeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { osUserHomeDir = original })

	statePath := state.Path(home)
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(statePath, []byte("{not valid json\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := RunInstall([]string{"--agent", "cursor", "--preset", "custom"}, system.DetectionResult{})
	if err == nil {
		t.Fatal("RunInstall() error = nil, want an error naming the unreadable install state instead of a silent success")
	}
}
