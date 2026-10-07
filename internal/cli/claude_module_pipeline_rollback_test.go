package cli

import (
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/backup"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/agentguidance"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/pipeline"
	"github.com/gentleman-programming/gentle-ai/v4/internal/planner"
)

// #5256 U2b-2: the Claude routing step installs the retained review/telemetry
// and skill-registry hooks into the home settings before it delivers guidance.
// Install and sync must snapshot that file whatever optional components were
// selected, so the normal outer rollback undoes the hook writes together with
// the pilot files. These cases run the real planners, snapshotter, routing
// step and pipeline rollback; nothing here promises safety for edits made by
// someone else during the run (S26).

// claudeModulePlanners are the install and sync backup planners for a global
// selection; no optional component in it covers the Claude settings.
var claudeModulePlanners = []struct {
	name    string
	targets func(home string, selection model.Selection) ([]string, error)
}{
	{"install", func(home string, selection model.Selection) ([]string, error) {
		return backupTargets(home, "", ScopeGlobal, selection, planner.ResolvedPlan{Agents: selection.Agents, OrderedComponents: selection.Components})
	}},
	{"sync", func(home string, selection model.Selection) ([]string, error) {
		return syncBackupTargetsScoped(home, "", ScopeGlobal, selection, resolveAdapters(selection.Agents))
	}},
}

func claudeHomeSettingsPath(home string) string {
	return resolveAdapters([]model.AgentID{model.AgentClaudeCode})[0].SettingsPath(home)
}

// executeClaudeModulePipeline composes the stage shape install and sync use:
// snapshot the planned targets, register rollback-restore first, then run the
// real Claude routing step followed by any later steps.
func executeClaudeModulePipeline(t *testing.T, home string, targets []string, changed *[]string, later ...pipeline.Step) pipeline.ExecutionResult {
	t.Helper()

	state := &runtimeState{}
	plan := pipeline.StagePlan{
		Prepare: []pipeline.Step{prepareBackupStep{
			id: "prepare:backup-snapshot", snapshotter: backup.NewSnapshotter(),
			snapshotDir: filepath.Join(t.TempDir(), "snapshot"), targets: targets, state: state,
		}},
		Apply: append([]pipeline.Step{
			rollbackRestoreStep{id: "apply:rollback-restore", state: state, homeDir: home},
			claudeRoutingStep(home, "", ScopeGlobal, changed),
		}, later...),
	}
	return pipeline.NewOrchestrator(pipeline.DefaultRollbackPolicy()).Execute(plan)
}

// pilotFileStates records each pilot path's bytes, or its absence: only the
// nonempty modules exist on disk.
func pilotFileStates(t *testing.T, paths []string) map[string]string {
	t.Helper()

	states := make(map[string]string, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			states[path] = "<absent>"
		case err != nil:
			t.Fatalf("ReadFile(%q) error = %v", path, err)
		default:
			states[path] = string(data)
		}
	}
	return states
}

func TestClaudeModulePipelineRollbackPlansHomeSettingsWhereRoutingWritesHooks(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	selection := model.Selection{Agents: []model.AgentID{model.AgentClaudeCode}}
	settings := claudeHomeSettingsPath(home)

	for _, tc := range []struct {
		name  string
		scope InstallScope
		sync  bool
		want  bool
	}{
		{name: "install global", scope: ScopeGlobal, want: true},
		// Install schedules the routing step in every scope, and its hooks
		// always target the home settings.
		{name: "install workspace", scope: ScopeWorkspace, want: true},
		{name: "sync global", scope: ScopeGlobal, sync: true, want: true},
		// A workspace sync skips Claude routing entirely and must not snapshot
		// home state.
		{name: "sync workspace", scope: ScopeWorkspace, sync: true, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var targets []string
			var err error
			if tc.sync {
				targets, err = syncBackupTargetsScoped(home, workspace, tc.scope, selection, resolveAdapters(selection.Agents))
			} else {
				targets, err = backupTargets(home, workspace, tc.scope, selection, planner.ResolvedPlan{Agents: selection.Agents})
			}
			if err != nil {
				t.Fatalf("backup targets error = %v", err)
			}
			if got := containsPath(targets, settings); got != tc.want {
				t.Fatalf("backup targets contain home Claude settings %q = %t, want %t\ntargets = %v", settings, got, tc.want, targets)
			}
		})
	}
}

func TestClaudeModulePipelineRollbackRemovesNewHookSettingsAfterModuleRefusal(t *testing.T) {
	for _, planned := range claudeModulePlanners {
		t.Run(planned.name, func(t *testing.T) {
			home := t.TempDir()
			paths := seedClaudeModulePilot(t, home)
			settings := claudeHomeSettingsPath(home)
			if _, err := os.Lstat(settings); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("seeded pilot left %q (stat err = %v); the test proves nothing", settings, err)
			}
			var module string
			for _, path := range paths[1 : len(paths)-1] {
				if _, err := os.Stat(path); err == nil {
					module = path
					break
				}
			}
			if module == "" {
				t.Fatal("seeded pilot wrote no module; the test proves nothing")
			}
			// The user edit predates the snapshot, so rollback must keep it.
			mustWriteFile(t, module, []byte(readTextFile(t, module)+"user edit\n"))
			before := pilotFileStates(t, paths)

			targets, err := planned.targets(home, model.Selection{Agents: []model.AgentID{model.AgentClaudeCode}})
			if err != nil {
				t.Fatalf("backup targets error = %v", err)
			}
			var changed []string
			result := executeClaudeModulePipeline(t, home, targets, &changed)

			if result.Err == nil || !result.Rollback.Success {
				t.Fatalf("pipeline over a modified pilot module: err = %v, rollback = %#v; want refusal and a successful rollback", result.Err, result.Rollback)
			}
			// The hooks really wrote before the module refusal stopped the step.
			if !slices.Contains(changed, settings) {
				t.Fatalf("routing step reported %v; want the hook write to %q before the refusal", changed, settings)
			}
			if _, err := os.Lstat(settings); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("rollback left the hook settings it created at %q (stat err = %v)", settings, err)
			}
			if after := pilotFileStates(t, paths); !maps.Equal(after, before) {
				t.Fatal("rollback did not keep the pre-run pilot bytes, including the user-modified module")
			}
		})
	}
}

func TestClaudeModulePipelineRollbackRestoresPilotAndUserSettingsAfterLaterFailure(t *testing.T) {
	for _, planned := range claudeModulePlanners {
		t.Run(planned.name, func(t *testing.T) {
			home := t.TempDir()
			// Seed the pilot with an earlier contract revision so the routing
			// step genuinely republishes the core with the current contract.
			prior := func(agent model.AgentID) (string, error) {
				contract, err := routingReviewContract(agent)
				return contract + "\nPrior contract revision.\n", err
			}
			options := agentguidance.RoutingOptions{ClaudeGlobalModules: true, ReviewContract: prior}
			if _, err := agentguidance.InjectRoutingWithOptions(home, model.AgentClaudeCode, options); err != nil {
				t.Fatalf("seed Claude module pilot: %v", err)
			}
			paths, err := agentguidance.ClaudeGlobalModulePaths(home)
			if err != nil || len(paths) != 9 {
				t.Fatalf("ClaudeGlobalModulePaths() = %v, %v; want the nine pilot paths", paths, err)
			}
			settings := claudeHomeSettingsPath(home)
			userSettings := "{\n  \"permissions\": {\"allow\": [\"Bash(ls:*)\"]},\n  \"hooks\": {\"Stop\": [{\"matcher\": \"\", \"hooks\": [{\"type\": \"command\", \"command\": \"echo user\"}]}]}\n}\n"
			mustWriteFile(t, settings, []byte(userSettings))
			before := pilotFileStates(t, paths)

			targets, err := planned.targets(home, model.Selection{Agents: []model.AgentID{model.AgentClaudeCode}})
			if err != nil {
				t.Fatalf("backup targets error = %v", err)
			}
			var changed []string
			result := executeClaudeModulePipeline(t, home, targets, &changed, failingSyncStep{})

			if result.Err == nil || !result.Rollback.Success {
				t.Fatalf("pipeline with a later failure: err = %v, rollback = %#v; want failure and a successful rollback", result.Err, result.Rollback)
			}
			if len(result.Apply.Steps) != 3 || result.Apply.Steps[1].Status != pipeline.StepStatusSucceeded {
				t.Fatalf("routing step must succeed before the later failure: %#v", result.Apply.Steps)
			}
			// Both writers really ran: the hooks merged into the user settings
			// and the core was republished with the current contract.
			for _, path := range []string{settings, paths[0]} {
				if !slices.Contains(changed, path) {
					t.Fatalf("routing step reported %v; want a write to %q before the later failure", changed, path)
				}
			}
			if got := readTextFile(t, settings); got != userSettings {
				t.Fatalf("rollback left hook changes in the user settings:\n%s", got)
			}
			if after := pilotFileStates(t, paths); !maps.Equal(after, before) {
				t.Fatal("rollback did not restore the pre-run pilot bytes")
			}
		})
	}
}
