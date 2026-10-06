package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// Issue #5252: Claude ODD role models are only consumed through the routing
// table, so both install and sync must carry the saved assignments into it.
func TestClaudeODDRoutingCarriesSavedAssignments(t *testing.T) {
	selection := model.Selection{
		Agents: []model.AgentID{model.AgentClaudeCode},
		ClaudePhaseAssignments: map[string]model.ClaudePhaseAssignment{
			"odd-worker": {Model: model.ClaudeModelOpus},
			"odd-verify": {Model: model.ClaudeModelHaiku},
		},
	}
	want := []string{"| `odd-explorer` | `sonnet` |", "| `odd-worker` | `opus` |", "| `odd-verify` | `haiku` |"}

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
