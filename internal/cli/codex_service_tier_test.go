package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/codex"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/pipeline"
	"github.com/gentleman-programming/gentle-ai/v4/internal/state"
)

// TestEngramStepsForwardCodexServiceTier covers both install and sync engram
// steps: a new selection is written, and choosing standard retires exactly
// the tier Gentle AI previously wrote.
func TestEngramStepsForwardCodexServiceTier(t *testing.T) {
	t.Cleanup(codex.SetRuntimeVersionCommandForTest("codex-cli 0.144.0", nil))
	restoreCommand, restoreLookPath := runCommand, cmdLookPath
	t.Cleanup(func() { runCommand, cmdLookPath = restoreCommand, restoreLookPath })
	runCommand = func(string, ...string) error { return nil }
	cmdLookPath = func(name string) (string, error) { return "/usr/local/bin/" + name, nil }

	steps := map[string]func(home string, selection model.Selection) pipeline.Step{
		"install": func(home string, selection model.Selection) pipeline.Step {
			return componentApplyStep{component: model.ComponentEngram, homeDir: home, workspaceDir: home, scope: ScopeGlobal, agents: selection.Agents, selection: selection, state: &runtimeState{}}
		},
		"sync": func(home string, selection model.Selection) pipeline.Step {
			return componentSyncStep{component: model.ComponentEngram, homeDir: home, workspaceDir: home, agents: selection.Agents, selection: selection}
		},
	}
	for name, newStep := range steps {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			configPath := filepath.Join(home, ".codex", "config.toml")
			run := func(selection model.Selection) string {
				t.Helper()
				selection.Agents = []model.AgentID{model.AgentCodex}
				if err := newStep(home, selection).Run(); err != nil {
					t.Fatalf("Run() error = %v", err)
				}
				content, err := os.ReadFile(configPath)
				if err != nil {
					t.Fatal(err)
				}
				return string(content)
			}

			if got := run(model.Selection{CodexServiceTier: "priority"}); !strings.Contains(got, `service_tier = "priority"`) {
				t.Fatalf("selected tier not written:\n%s", got)
			}
			if got := run(model.Selection{CodexServiceTier: "priority", CodexManagedServiceTier: "priority"}); !strings.Contains(got, `service_tier = "priority"`) {
				t.Fatalf("unchanged managed tier was dropped:\n%s", got)
			}
			if got := run(model.Selection{CodexManagedServiceTier: "priority"}); strings.Contains(got, "service_tier") {
				t.Fatalf("standard did not retire the managed tier:\n%s", got)
			}
		})
	}
}

// TestSyncRecordsOnlyTheCodexServiceTierEngramWrote pins state to engram's
// write result: a sync that never writes Codex config keeps the previous value.
func TestSyncRecordsOnlyTheCodexServiceTierEngramWrote(t *testing.T) {
	t.Cleanup(codex.SetRuntimeVersionCommandForTest("codex-cli 0.144.0", nil))
	restoreCommand, restoreLookPath := runCommand, cmdLookPath
	t.Cleanup(func() { runCommand, cmdLookPath = restoreCommand, restoreLookPath })
	runCommand = func(string, ...string) error { return nil }
	cmdLookPath = func(name string) (string, error) { return "/usr/local/bin/" + name, nil }

	home := t.TempDir()
	writer, err := managedAssetDigest()
	if err != nil {
		t.Fatal(err)
	}
	if err := state.Write(home, state.InstallState{InstalledAgents: []string{"codex"}, CodexServiceTier: "flex", ManagedAssetDigest: writer}); err != nil {
		t.Fatal(err)
	}
	recorded := func(components ...model.ComponentID) string {
		t.Helper()
		selection := model.Selection{Agents: []model.AgentID{model.AgentCodex}, Components: components, CodexServiceTier: "priority", CodexManagedServiceTier: "flex"}
		if _, err := RunSyncWithSelection(home, selection); err != nil {
			t.Fatalf("RunSyncWithSelection(%v) error = %v", components, err)
		}
		persisted, err := state.Read(home)
		if err != nil {
			t.Fatal(err)
		}
		return persisted.CodexServiceTier
	}

	if got := recorded(model.ComponentPersona); got != "flex" {
		t.Fatalf("sync without engram recorded %q, want previous flex", got)
	}
	if got := recorded(model.ComponentEngram); got != "priority" {
		t.Fatalf("sync that wrote priority recorded %q, want priority", got)
	}
}

func TestRunSyncRestoresOnlyValidPersistedCodexServiceTier(t *testing.T) {
	for _, tt := range []struct{ persisted, want string }{
		{"priority", "priority"},
		{"priority\nmodel = \"x\"", ""},
	} {
		selection := model.Selection{}
		restoreCodexServiceTier(&selection, state.InstallState{CodexServiceTier: tt.persisted})
		if selection.CodexServiceTier != tt.want || selection.CodexManagedServiceTier != tt.want {
			t.Fatalf("restored %q: tier = %q managed = %q, want %q", tt.persisted, selection.CodexServiceTier, selection.CodexManagedServiceTier, tt.want)
		}
	}
}
