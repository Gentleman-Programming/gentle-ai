package cli

import (
	"os"
	"path/filepath"
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
		Agents: []model.AgentID{model.AgentClaudeCode},
		ClaudePhaseAssignments: map[string]model.ClaudePhaseAssignment{
			"odd-worker": {Model: model.ClaudeModelOpus, Effort: model.ClaudeEffortMax},
			"odd-verify": {Model: model.ClaudeModelHaiku},
		},
	}
	want := []string{"| `odd-explorer` | `sonnet` | `default` |", "| `odd-worker` | `opus` | `max` |", "| `odd-verify` | `haiku` | `default` |"}

	t.Run("install", func(t *testing.T) {
		home := t.TempDir()
		runInstallInjectionSteps(t, newTestInstallRuntime(t, home, selection))
		body := readTextFile(t, filepath.Join(home, ".claude", "CLAUDE.md"))
		for _, row := range want {
			if !strings.Contains(body, row) {
				t.Errorf("install routing missing %q", row)
			}
		}
	})

	t.Run("sync", func(t *testing.T) {
		home := t.TempDir()
		runtime, err := newSyncRuntimeWithScope(home, selection, ScopeGlobal)
		if err != nil {
			t.Fatal(err)
		}
		ran := false
		for _, step := range runtime.stagePlan().Apply {
			if routing, ok := step.(agentRoutingGuidanceStep); ok && routing.agent == model.AgentClaudeCode {
				if err := routing.Run(); err != nil {
					t.Fatal(err)
				}
				ran = true
			}
		}
		if !ran {
			t.Fatal("sync scheduled no Claude routing step")
		}
		body := readTextFile(t, filepath.Join(home, ".claude", "CLAUDE.md"))
		for _, row := range want {
			if !strings.Contains(body, row) {
				t.Errorf("sync routing missing %q", row)
			}
		}
	})
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
	settingsBefore := []byte("{}\n")
	mustWriteFile(t, settings, settingsBefore)

	selection.ClaudePhaseAssignments = map[string]model.ClaudePhaseAssignment{"odd-worker": {Model: model.ClaudeModelOpus, Effort: model.ClaudeEffortHigh}}
	sync, err := newSyncRuntimeWithScope(home, selection, ScopeWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	if result := pipeline.NewOrchestrator(pipeline.DefaultRollbackPolicy()).Execute(sync.stagePlan()); result.Err != nil {
		t.Fatalf("workspace sync: %v", result.Err)
	}
	if body := readTextFile(t, project); !strings.Contains(body, "| `odd-worker` | `opus` | `high` |") {
		t.Errorf("workspace sync left the project routing table stale:\n%s", body)
	}
	if settingsAfter, _ := os.ReadFile(settings); string(settingsAfter) != string(settingsBefore) {
		t.Errorf("workspace sync changed project hook settings:\nbefore: %s\nafter: %s", settingsBefore, settingsAfter)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude")); !os.IsNotExist(err) {
		t.Errorf("workspace install and sync wrote the global Claude config: %v", err)
	}
}
