package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/components/agentguidance"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
	"github.com/gentleman-programming/gentle-ai/v3/internal/planner"
)

func TestStrictTDDInstallAndSyncWithoutSDD(t *testing.T) {
	for _, agent := range []model.AgentID{model.AgentClaudeCode, model.AgentCodex, model.AgentOpenClaw, model.AgentKimi, model.AgentCursor, model.AgentGeminiCLI, model.AgentWindsurf, model.AgentKiroIDE, model.AgentVSCodeCopilot, model.AgentAntigravity, model.AgentQwenCode, model.AgentTrae, model.AgentHermes} {
		t.Run(string(agent), func(t *testing.T) {
			home := t.TempDir()
			selection := model.Selection{Agents: []model.AgentID{agent}, StrictTDD: true}
			path := agentguidance.StrictTDDPath(home, agent)
			targets, err := backupTargets(home, "", ScopeGlobal, selection, planner.ResolvedPlan{Agents: selection.Agents})
			if err != nil || !containsPath(targets, path) {
				t.Fatalf("install backup: %v %v", targets, err)
			}
			syncTargets, err := syncBackupTargets(home, "", selection, resolveAdapters(selection.Agents))
			if err != nil || !containsPath(syncTargets, path) {
				t.Fatalf("sync backup: %v %v", syncTargets, err)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatal(err)
			}
			if agent != model.AgentKimi {
				if err := os.WriteFile(path, []byte("user content\n"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			step := agentRoutingGuidanceStep{agent: agent, homeDir: home, scope: ScopeGlobal, strictTDD: true}
			if err := step.Run(); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(path)
			if err != nil || !strings.Contains(string(raw), "Strict TDD Mode: enabled") {
				t.Fatalf("install: %q %v", raw, err)
			}
			changed := []string{}
			step.changedFiles = &changed
			if err := step.Run(); err != nil {
				t.Fatal(err)
			}
			if len(changed) != 0 {
				t.Fatalf("sync not idempotent: %v", changed)
			}
			step.strictTDD = false
			if err := step.Run(); err != nil {
				t.Fatal(err)
			}
			if agent == model.AgentKimi {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("stale module: %v", err)
				}
				return
			}
			raw, err = os.ReadFile(path)
			if err != nil || strings.Contains(string(raw), "gentle-ai:strict-tdd-mode") || !strings.Contains(string(raw), "user content") {
				t.Fatalf("stale section or lost user content: %q %v", raw, err)
			}
		})
	}
}

func TestStrictTDDOpenCodeInstallAndPersistedSync(t *testing.T) {
	for _, agent := range []model.AgentID{model.AgentOpenCode, model.AgentKilocode} {
		t.Run(string(agent), func(t *testing.T) {
			home := t.TempDir()
			selection := model.Selection{Agents: []model.AgentID{agent}, StrictTDD: true}
			adapter := resolveAdapters(selection.Agents)[0]
			path := effectiveOpenCodeSettingsPath(home, "", ScopeGlobal, adapter)
			targets, err := backupTargets(home, "", ScopeGlobal, selection, planner.ResolvedPlan{Agents: selection.Agents})
			if err != nil || !containsPath(targets, path) {
				t.Fatalf("install backup: %v %v", targets, err)
			}
			targets, err = syncBackupTargets(home, "", selection, resolveAdapters(selection.Agents))
			if err != nil || !containsPath(targets, path) {
				t.Fatalf("sync backup: %v %v", targets, err)
			}
			step := agentRoutingGuidanceStep{agent: agent, homeDir: home, scope: ScopeGlobal, strictTDD: true}
			if err := step.Run(); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(path)
			if err != nil || !strings.Contains(string(raw), "gentle-ai:strict-tdd-mode") {
				t.Fatalf("on: %s %v", raw, err)
			}
			changed := []string{}
			step.changedFiles = &changed
			if err := step.Run(); err != nil || len(changed) != 0 {
				t.Fatalf("idempotent sync: %v %v", changed, err)
			}
			step.strictTDD = false
			if err := step.Run(); err != nil {
				t.Fatal(err)
			}
			raw, err = os.ReadFile(path)
			if err != nil || strings.Contains(string(raw), "gentle-ai:strict-tdd-mode") || !strings.Contains(string(raw), "gentle-ai:agent-routing") {
				t.Fatalf("off: %s %v", raw, err)
			}
		})
	}
}
