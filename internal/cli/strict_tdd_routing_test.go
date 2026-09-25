package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/components/agentguidance"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

func TestStrictTDDLegacyCarrierRetiredByInstallAndSync(t *testing.T) {
	for _, agent := range []model.AgentID{model.AgentClaudeCode, model.AgentCodex, model.AgentOpenClaw, model.AgentKimi, model.AgentCursor, model.AgentGeminiCLI, model.AgentWindsurf, model.AgentKiroIDE, model.AgentVSCodeCopilot, model.AgentAntigravity, model.AgentQwenCode, model.AgentTrae, model.AgentHermes} {
		t.Run(string(agent), func(t *testing.T) {
			home := t.TempDir()
			path := agentguidance.StrictTDDPath(home, agent)
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatal(err)
			}
			if agent != model.AgentKimi {
				if err := os.WriteFile(path, []byte("user content\n"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := agentguidance.InjectStrictTDDWithOptions(home, agent, true, agentguidance.RoutingOptions{}); err != nil {
				t.Fatal(err)
			}
			step := agentRoutingGuidanceStep{agent: agent, homeDir: home, scope: ScopeGlobal}
			if err := step.Run(); err != nil {
				t.Fatal(err)
			}
			if agent == model.AgentKimi {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("legacy module remains: %v", err)
				}
			} else {
				raw, err := os.ReadFile(path)
				if err != nil || strings.Contains(string(raw), "gentle-ai:strict-tdd-mode") || !strings.Contains(string(raw), "user content") || !strings.Contains(string(raw), "relevant runnable deterministic test") {
					t.Fatalf("legacy marker or missing routing/user content: %s %v", raw, err)
				}
			}
			changed := []string{}
			step.changedFiles = &changed
			if err := step.Run(); err != nil || len(changed) != 0 {
				t.Fatalf("sync not idempotent: %v %v", changed, err)
			}
		})
	}
}

func TestStrictTDDLegacyOpenCodePromptRetired(t *testing.T) {
	for _, agent := range []model.AgentID{model.AgentOpenCode, model.AgentKilocode} {
		t.Run(string(agent), func(t *testing.T) {
			home := t.TempDir()
			adapter := resolveAdapters([]model.AgentID{agent})[0]
			path := effectiveOpenCodeSettingsPath(home, "", ScopeGlobal, adapter)
			if _, err := agentguidance.InjectStrictTDDWithOptions(home, agent, true, agentguidance.RoutingOptions{SettingsPath: path}); err != nil {
				t.Fatal(err)
			}
			step := agentRoutingGuidanceStep{agent: agent, homeDir: home, scope: ScopeGlobal}
			if err := step.Run(); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(path)
			if err != nil || strings.Contains(string(raw), "gentle-ai:strict-tdd-mode") || !strings.Contains(string(raw), "gentle-ai:agent-routing") || !strings.Contains(string(raw), "relevant runnable deterministic test") {
				t.Fatalf("legacy marker or missing routing: %s %v", raw, err)
			}
		})
	}
}
