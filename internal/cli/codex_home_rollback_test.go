package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/codex"
	"github.com/gentleman-programming/gentle-ai/v4/internal/backup"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/pipeline"
	"github.com/gentleman-programming/gentle-ai/v4/internal/planner"
)

// TestRollbackRestoresEngramFilesUnderExternalCodexHome pins that a failed
// install restores Codex files written under a CODEX_HOME override that lives
// outside both the home and the workspace. The override is the selected Codex
// adapter's root, so it extends the restore roots like the Pi agent directory.
func TestRollbackRestoresEngramFilesUnderExternalCodexHome(t *testing.T) {
	t.Cleanup(codex.SetRuntimeVersionCommandForTest("codex-cli 0.144.0", nil))
	root := t.TempDir()
	home := filepath.Join(root, "home")
	workspace := filepath.Join(root, "workspace")
	override := filepath.Join(root, "codex-home")
	for _, dir := range []string{home, workspace, override} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CODEX_HOME", override)

	selected := filepath.Join(override, "engram-instructions.md")
	before := []byte("user-owned instructions\n")
	if err := os.WriteFile(selected, before, 0o644); err != nil {
		t.Fatal(err)
	}
	selection := model.Selection{Agents: []model.AgentID{model.AgentCodex}, Components: []model.ComponentID{model.ComponentEngram}}
	targets, err := backupTargets(home, workspace, ScopeGlobal, selection, planner.ResolvedPlan{Agents: selection.Agents, OrderedComponents: selection.Components})
	if err != nil || !slices.Contains(targets, selected) {
		t.Fatalf("snapshot targets = %v, %v; want %q", targets, err, selected)
	}

	state := &runtimeState{}
	cause := errors.New("injected failure after the Codex write")
	plan := pipeline.StagePlan{
		Prepare: []pipeline.Step{prepareBackupStep{id: "prepare:backup-snapshot", snapshotter: backup.NewSnapshotter(), snapshotDir: filepath.Join(home, "backup"), targets: []string{selected}, state: state}},
		Apply: []pipeline.Step{
			rollbackRestoreStep{id: "apply:rollback-restore", state: state, homeDir: home, workspaceDir: workspace, codexHomeDir: codexRollbackRoot(home, selection.Agents)},
			overwriteThenFailStep{path: selected, cause: cause},
		},
	}
	result := pipeline.NewOrchestrator(pipeline.DefaultRollbackPolicy()).Execute(plan)
	if !errors.Is(result.Err, cause) || !result.Rollback.Success {
		t.Fatalf("failure = %v, rollback = %#v", result.Err, result.Rollback)
	}
	got, err := os.ReadFile(selected)
	if err != nil || !bytes.Equal(got, before) {
		t.Fatalf("restored bytes = %q, %v; want %q", got, err, before)
	}
}

// TestCodexRollbackRootOnlyForSelectedOverride pins that only a selected Codex
// with an applicable CODEX_HOME override extends the restore roots.
func TestCodexRollbackRootOnlyForSelectedOverride(t *testing.T) {
	home := t.TempDir()
	override := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	canonical, err := filepath.EvalSymlinks(override)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		override string
		home     string
		agents   []model.AgentID
		want     string
	}{
		{name: "selected codex with override", override: override, home: home, agents: []model.AgentID{model.AgentCodex}, want: canonical},
		{name: "codex not selected", override: override, home: home, agents: []model.AgentID{model.AgentOpenCode}},
		{name: "no override", home: home, agents: []model.AgentID{model.AgentCodex}},
		{name: "isolated home", override: override, home: t.TempDir(), agents: []model.AgentID{model.AgentCodex}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CODEX_HOME", tc.override)
			if got := codexRollbackRoot(tc.home, tc.agents); got != tc.want {
				t.Fatalf("codexRollbackRoot() = %q, want %q", got, tc.want)
			}
		})
	}
}

type overwriteThenFailStep struct {
	path  string
	cause error
}

func (s overwriteThenFailStep) ID() string { return "test:overwrite-then-fail" }
func (s overwriteThenFailStep) Run() error {
	if err := os.WriteFile(s.path, []byte("managed instructions\n"), 0o644); err != nil {
		return err
	}
	return s.cause
}
func (s overwriteThenFailStep) Rollback() error { return nil }
