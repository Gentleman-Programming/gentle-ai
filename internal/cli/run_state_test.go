package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v4/internal/backup"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/state"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
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
	original := state.InstallState{InstalledAgents: []string{"opencode"}, SelectionConfigured: true, Components: []model.ComponentID{model.ComponentEngram}, Skills: []model.SkillID{model.SkillCommentWriter}, Preset: model.PresetCustom, SDDMode: model.SDDModeSingle, Persona: "neutral"}
	fresh := state.InstallState{InstalledAgents: []string{"codex"}, SelectionConfigured: true, Components: []model.ComponentID{model.ComponentSDD}, Skills: []model.SkillID{model.SkillSDDInit}, Preset: model.PresetFullGentleman, SDDMode: model.SDDModeMulti, Persona: "gentleman"}
	cases := []InstallFlags{{Components: []string{"sdd"}}, {Skills: []string{"sdd-init"}}, {Preset: "full-gentleman"}, {SDDMode: "multi"}, {Persona: "gentleman"}}
	wants := []string{"[sdd]|[comment-writer]|custom|single|neutral", "[engram]|[sdd-init]|custom|single|neutral", "[engram]|[comment-writer]|full-gentleman|single|neutral", "[engram]|[comment-writer]|custom|multi|neutral", "[engram]|[comment-writer]|custom|single|gentleman"}
	for i, flags := range cases {
		home := t.TempDir()
		if err := state.Write(home, original); err != nil {
			t.Fatal(err)
		}
		persisted, err := os.ReadFile(state.Path(home))
		if err != nil {
			t.Fatal(err)
		}
		legacy := strings.Replace(string(persisted), "{", `{"strict_tdd":true,`, 1)
		if err := os.WriteFile(state.Path(home), []byte(legacy), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := mergeExplicitAgentInstallState(home, fresh, []string{"codex"}, flags)
		if err != nil {
			t.Fatalf("flags %#v merge error: %v", flags, err)
		}
		key := fmt.Sprintf("%v|%v|%s|%s|%s", got.Components, got.Skills, got.Preset, got.SDDMode, got.Persona)
		if key != wants[i] || got.StrictTDD || !got.SelectionConfigured || len(got.InstalledAgents) != 2 || got.InstalledAgents[0] != "opencode" || got.InstalledAgents[1] != "codex" {
			t.Errorf("flags %#v merged state %#v, selection %s, want %s with legacy StrictTDD ignored and agents preserved", flags, got, key, wants[i])
		}
	}
}

func TestRunInstallPersistsConfiguredSelection(t *testing.T) {
	home := t.TempDir()
	original := osUserHomeDir
	osUserHomeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { osUserHomeDir = original })
	// This test targets state persistence, not agent install behavior, so
	// simulate Cursor as already installed (its Detect checks for ~/.cursor)
	// — otherwise gentle-ai correctly refuses to proceed for an undetected
	// desktop-app agent.
	if err := os.MkdirAll(filepath.Join(home, ".cursor"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.cursor): %v", err)
	}
	if _, err := RunInstall([]string{"--agent", "cursor", "--preset", "custom"}, system.DetectionResult{}); err != nil {
		t.Fatal(err)
	}
	got, err := state.Read(home)
	if err != nil || !got.SelectionConfigured || got.Preset != model.PresetCustom || got.SDDMode != "" || len(got.Components) != 0 || len(got.InstalledAgents) != 1 || got.InstalledAgents[0] != "cursor" {
		t.Fatalf("persisted selection = %#v, err = %v", got, err)
	}
	wantDigest, err := managedAssetDigest()
	if err != nil {
		t.Fatal(err)
	}
	if got.ManagedAssetDigest != wantDigest {
		t.Fatalf("managed asset digest = %q, want %q", got.ManagedAssetDigest, wantDigest)
	}
}

func TestRunInstallExplicitSelectionThenSync(t *testing.T) {
	lastCheck := time.Date(2026, 9, 30, 9, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name     string
		existing *state.InstallState
	}{
		{name: "no prior state"},
		{name: "update-check stub", existing: &state.InstallState{LastUpdateCheck: &lastCheck}},
		{name: "legacy install without selection", existing: &state.InstallState{
			InstalledAgents: []string{"claude-code"}, LastUpdateCheck: &lastCheck,
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			home := convergenceTestHome(t)
			oldBackupHome := backup.UserHomeDirFn
			backup.UserHomeDirFn = func() (string, error) { return home, nil }
			t.Cleanup(func() { backup.UserHomeDirFn = oldBackupHome })
			if tt.existing != nil {
				if err := state.Write(home, *tt.existing); err != nil {
					t.Fatal(err)
				}
			}

			flags := InstallFlags{Agents: []string{"claude-code"}, Components: []string{"persona", "skills", "context7"}, Preset: "minimal"}
			input, err := NormalizeInstallFlags(flags, system.DetectionResult{})
			if err != nil {
				t.Fatal(err)
			}
			installed, err := RunInstall([]string{"--agent", "claude-code", "--component", "persona,skills,context7", "--preset", "minimal"}, system.DetectionResult{})
			if err != nil || !installed.Verify.Ready {
				t.Fatalf("RunInstall() error = %v, ready = %v", err, installed.Verify.Ready)
			}
			got, err := state.Read(home)
			if err != nil {
				t.Fatal(err)
			}
			wantComponents := []model.ComponentID{model.ComponentPersona, model.ComponentSkills, model.ComponentContext7}
			if !got.SelectionConfigured || !slices.Equal(got.Components, wantComponents) || !slices.Equal(got.Skills, input.Selection.Skills) || got.Preset != model.PresetMinimal || !slices.Equal(got.InstalledAgents, []string{"claude-code"}) {
				t.Fatalf("persisted selection = %#v, want explicit components, skills and minimal preset", got)
			}
			if tt.existing != nil && (got.LastUpdateCheck == nil || !got.LastUpdateCheck.Equal(lastCheck)) {
				t.Fatalf("LastUpdateCheck = %v, want %v", got.LastUpdateCheck, lastCheck)
			}
			before := snapshotManagedTree(t, home)
			synced, err := RunSync(nil)
			if err != nil {
				t.Fatalf("RunSync() error = %v", err)
			}
			if !slices.Equal(synced.Selection.Components, wantComponents) || !slices.Equal(synced.Selection.Skills, input.Selection.Skills) || synced.Selection.Preset != model.PresetMinimal {
				t.Fatalf("sync selection = %#v, want recorded explicit selection", synced.Selection)
			}
			assertSameManagedTree(t, "sync after explicit install", before, snapshotManagedTree(t, home))
			if synced.FilesChanged != 0 {
				t.Errorf("sync changed %d files: %v", synced.FilesChanged, synced.ChangedFiles)
			}
		})
	}
}

func TestRunInstallIncrementalSelectionPreservesUnspecifiedFields(t *testing.T) {
	t.Chdir(t.TempDir())
	home := convergenceTestHome(t)
	if err := state.Write(home, state.InstallState{
		InstalledAgents: []string{"cursor"}, SelectionConfigured: true,
		Components: []model.ComponentID{model.ComponentSkills, model.ComponentContext7},
		Skills:     []model.SkillID{model.SkillCommentWriter}, Preset: model.PresetCustom, Persona: "neutral",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := RunInstall([]string{"--agent", "claude-code", "--component", "persona"}, system.DetectionResult{}); err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	got, err := state.Read(home)
	if err != nil {
		t.Fatal(err)
	}
	if !got.SelectionConfigured || !slices.Equal(got.Components, []model.ComponentID{model.ComponentPersona}) || !slices.Equal(got.Skills, []model.SkillID{model.SkillCommentWriter}) || got.Preset != model.PresetCustom || got.Persona != "neutral" || !slices.Equal(got.InstalledAgents, []string{"cursor", "claude-code"}) {
		t.Fatalf("incremental selection = %#v, want explicit components with existing skills, preset, persona and agents preserved", got)
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
