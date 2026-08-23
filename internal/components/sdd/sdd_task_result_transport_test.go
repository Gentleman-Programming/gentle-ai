package sdd

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

const sddTaskResultPluginName = "sdd-task-result-artifacts.ts"

func TestSDDTaskResultPluginIsOpenCodeOnlyAndCanonicallySynced(t *testing.T) {
	for _, tc := range []struct {
		name       string
		agent      model.AgentID
		wantPlugin bool
		assetPath  string
	}{
		{name: "OpenCode", agent: model.AgentOpenCode, wantPlugin: true, assetPath: "opencode/sdd-orchestrator.md"},
		{name: "Kilo Code", agent: model.AgentKilocode, assetPath: "opencode/sdd-orchestrator.md"},
		{name: "Claude Code", agent: model.AgentClaudeCode, assetPath: "claude/sdd-orchestrator.md"},
		{name: "Pi", agent: model.AgentPi, assetPath: "generic/sdd-orchestrator.md"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotPlugin := slices.Contains(managedOpenCodePluginNames(tc.agent), sddTaskResultPluginName)
			if gotPlugin != tc.wantPlugin {
				t.Fatalf("managedOpenCodePluginNames(%q) contains %q = %t, want %t", tc.agent, sddTaskResultPluginName, gotPlugin, tc.wantPlugin)
			}

			if tc.agent == model.AgentClaudeCode || tc.agent == model.AgentPi {
				if got := sddOrchestratorAsset(tc.agent); got != tc.assetPath {
					t.Fatalf("sddOrchestratorAsset(%q) = %q, want %q", tc.agent, got, tc.assetPath)
				}
				content := assets.MustRead(tc.assetPath)
				for _, forbidden := range []string{"task_result", "sdd_task_result", sddTaskResultPluginName, "tool.execute.after"} {
					if strings.Contains(content, forbidden) {
						t.Fatalf("%s completion transport depends on the OpenCode task envelope hook via %q", tc.name, forbidden)
					}
				}
			}
		})
	}

	for _, path := range []string{
		"claude/plugins/" + sddTaskResultPluginName,
		"generic/plugins/" + sddTaskResultPluginName,
	} {
		if _, err := assets.Read(path); err == nil {
			t.Fatalf("non-OpenCode asset unexpectedly ships %q", path)
		}
	}

	const assetPath = "opencode/plugins/" + sddTaskResultPluginName
	canonical := assets.MustRead(assetPath)
	for _, required := range []string{
		"const TASK_RESULT",
		"function isBackgroundTask",
		"if (isBackgroundTask(input.args)) return",
	} {
		if !strings.Contains(canonical, required) {
			t.Fatalf("%s is missing OpenCode task transport contract %q", assetPath, required)
		}
	}
	for _, path := range []string{"opencode/sdd-orchestrator.md", "skills/_shared/sdd-phase-common.md"} {
		content := assets.MustRead(path)
		for _, required := range []string{"terminal", "background: true", "must not produce either transport failure or a session latch"} {
			if !strings.Contains(content, required) {
				t.Fatalf("%s is missing nonterminal background transport guidance %q", path, required)
			}
		}
	}

	home := t.TempDir()
	installed := filepath.Join(home, ".config", "opencode", "plugins", sddTaskResultPluginName)
	if err := os.MkdirAll(filepath.Dir(installed), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(installed, []byte("// stale task-result plugin\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := RefreshInstalledOpenCodePlugins(home, opencodeAdapter())
	if err != nil {
		t.Fatalf("RefreshInstalledOpenCodePlugins() error = %v", err)
	}
	if !result.Changed || !slices.Contains(result.Files, installed) {
		t.Fatalf("RefreshInstalledOpenCodePlugins() = %#v, want changed %q", result, installed)
	}
	got, err := os.ReadFile(installed)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != canonical {
		t.Fatalf("synced %s differs from canonical embedded asset", sddTaskResultPluginName)
	}
}
