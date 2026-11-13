package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/agentguidance"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/planner"
)

func TestCodexGlobalRoutingDefaultsToModulesWhileWorkspaceStaysMonolithic(t *testing.T) {
	home, workspace := t.TempDir(), t.TempDir()
	adapter, err := agents.NewAdapter(model.AgentCodex)
	if err != nil {
		t.Fatal(err)
	}

	global, err := scopedRoutingGuidanceOptions(home, workspace, ScopeGlobal, adapter, false)
	if err != nil || !global.CodexGlobalModules {
		t.Fatalf("global options = %+v, %v; want Codex modules", global, err)
	}
	workspaceOptions, err := scopedRoutingGuidanceOptions(home, workspace, ScopeWorkspace, adapter, false)
	if err != nil || workspaceOptions.CodexGlobalModules {
		t.Fatalf("workspace options = %+v, %v; want monolithic", workspaceOptions, err)
	}

	globalPaths := routingGuidancePaths(home, workspace, ScopeGlobal, []agents.Adapter{adapter})
	wantGlobal, err := agentguidance.RoutingPathsWithOptions(home, model.AgentCodex, global)
	if err != nil || !reflect.DeepEqual(globalPaths, wantGlobal) {
		t.Fatalf("global paths = %v, want %v (%v)", globalPaths, wantGlobal, err)
	}
	if len(globalPaths) != 9 || globalPaths[0] != filepath.Join(home, ".codex", "AGENTS.md") {
		t.Fatalf("global paths = %v; want core, seven known modules, and ledger", globalPaths)
	}
	selection := model.Selection{Agents: []model.AgentID{model.AgentCodex}}
	installPaths, err := backupTargets(home, workspace, ScopeGlobal, selection, planner.ResolvedPlan{Agents: selection.Agents})
	if err != nil {
		t.Fatal(err)
	}
	syncPaths, err := syncBackupTargetsScoped(home, workspace, ScopeGlobal, selection, []agents.Adapter{adapter})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range globalPaths {
		if !slices.Contains(installPaths, path) || !slices.Contains(syncPaths, path) {
			t.Fatalf("module path %q missing from install/sync backup plans", path)
		}
	}

	workspacePaths := routingGuidancePaths(home, workspace, ScopeWorkspace, []agents.Adapter{adapter})
	if want := []string{filepath.Join(workspace, ".codex", "AGENTS.md")}; !reflect.DeepEqual(workspacePaths, want) {
		t.Fatalf("workspace paths = %v, want %v", workspacePaths, want)
	}

	step := agentRoutingGuidanceStep{
		id: "agent-guidance:" + string(model.AgentCodex), agent: model.AgentCodex,
		homeDir: home, workspaceDir: workspace, scope: ScopeGlobal,
	}
	if err := step.Run(); err != nil {
		t.Fatal(err)
	}
	for _, path := range globalPaths {
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			t.Logf("installed %s: %d bytes", filepath.Base(path), info.Size())
		}
	}
}
