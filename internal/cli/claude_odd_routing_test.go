package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v5/internal/model"
	"github.com/gentleman-programming/gentle-ai/v5/internal/pipeline"
	"github.com/gentleman-programming/gentle-ai/v5/internal/planner"
	"github.com/gentleman-programming/gentle-ai/v5/internal/system"
)

// Issue #5252: Claude ODD role models and efforts are only consumed through the routing
// table, so both install and sync must carry the saved assignments into it.
func TestClaudeODDRoutingCarriesSavedAssignments(t *testing.T) {
	selection := model.Selection{
		Agents:                 []model.AgentID{model.AgentClaudeCode},
		ClaudeModelAssignments: map[string]model.ClaudeModelAlias{"odd-verify": model.ClaudeModelHaiku},
		ClaudePhaseAssignments: map[string]model.ClaudePhaseAssignment{
			"odd-worker": {Model: model.ClaudeModelOpus, Effort: model.ClaudeEffortMax},
		},
	}
	for _, operation := range []string{"install", "sync"} {
		t.Run(operation, func(t *testing.T) {
			home := t.TempDir()
			if operation == "install" {
				runInstallInjectionSteps(t, newTestInstallRuntime(t, home, selection))
			} else {
				runSyncInjectionSteps(t, home, selection)
			}
			body := readTextFile(t, filepath.Join(home, ".claude", "CLAUDE.md"))
			for _, row := range []string{"| `odd-explorer` | `sonnet` | `default` |", "| `odd-worker` | `opus` | `max` |", "| `odd-verify` | `haiku` | `default` |"} {
				if !strings.Contains(body, row) {
					t.Errorf("routing missing %q", row)
				}
			}
		})
	}
}

// A workspace sync must refresh the routing table a workspace install wrote,
// without touching the home directory or the hooks in the project settings.
func TestClaudeODDWorkspaceSyncRefreshesProjectRoutingTable(t *testing.T) {
	home := t.TempDir()
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(workspace)
	selection := model.Selection{Agents: []model.AgentID{model.AgentClaudeCode}}
	install, err := newInstallRuntime(home, ScopeWorkspace, ChannelStable, selection, planner.ResolvedPlan{Agents: selection.Agents}, system.PlatformProfile{PackageManager: "brew"})
	if err != nil {
		t.Fatal(err)
	}
	runInstallInjectionSteps(t, install)
	project := filepath.Join(workspace, ".claude", "CLAUDE.md")
	if body := readTextFile(t, project); !strings.Contains(body, "| `odd-worker` | `sonnet` | `default` |") {
		t.Fatalf("workspace install wrote no default ODD row:\n%s", body)
	}
	settings := filepath.Join(workspace, ".claude", "settings.json")
	// Empty settings: a sync that installed hooks would write them back here.
	const settingsBefore = "{}\n"
	mustWriteFile(t, settings, []byte(settingsBefore))

	selection.ClaudePhaseAssignments = map[string]model.ClaudePhaseAssignment{"odd-worker": {Model: model.ClaudeModelOpus, Effort: model.ClaudeEffortHigh}}
	sync, err := newSyncRuntimeWithScope(home, selection, ScopeWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sync.state.cleanupRollbackSnapshot)
	plan := sync.stagePlan()
	if !slices.Contains(sync.managedPaths, project) || slices.Contains(sync.managedPaths, filepath.Join(home, ".claude", "settings.json")) {
		t.Errorf("workspace snapshot must include the project prompt but not global hook settings: %v", sync.managedPaths)
	}
	if result := pipeline.NewOrchestrator(pipeline.DefaultRollbackPolicy()).Execute(plan); result.Err != nil {
		t.Fatalf("workspace sync: %v", result.Err)
	}
	if body := readTextFile(t, project); !strings.Contains(body, "| `odd-worker` | `opus` | `high` |") {
		t.Error("workspace sync left the project routing table stale")
	}
	if got := readTextFile(t, settings); got != settingsBefore {
		t.Errorf("workspace sync changed project hook settings: %q", got)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude")); !os.IsNotExist(err) {
		t.Errorf("workspace install and sync wrote the global Claude config: %v", err)
	}
}
