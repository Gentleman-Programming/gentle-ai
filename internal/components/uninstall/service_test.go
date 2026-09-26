package uninstall

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v3/internal/agents/claude"
	"github.com/gentleman-programming/gentle-ai/v3/internal/agents/codex"
	"github.com/gentleman-programming/gentle-ai/v3/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/v3/internal/agents/pi"
	"github.com/gentleman-programming/gentle-ai/v3/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v3/internal/backup"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/communitytool"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/engram"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/gga"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/opencodedefault"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/skills"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/telemetryruntime"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
	opencodeactivation "github.com/gentleman-programming/gentle-ai/v3/internal/opencode"
	"github.com/gentleman-programming/gentle-ai/v3/internal/reviewtransaction"
	"github.com/gentleman-programming/gentle-ai/v3/internal/state"
	"github.com/gentleman-programming/gentle-ai/v3/internal/statecoord"
)

func TestUninstallOpenCodeFamilyManagedAgents(t *testing.T) {
	for _, agent := range []model.AgentID{model.AgentOpenCode, model.AgentKilocode} {
		t.Run(string(agent), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			svc, err := NewService(home, t.TempDir(), "dev")
			if err != nil {
				t.Fatal(err)
			}
			svc.snapshotter = stubSnapshotter{}
			adapter, _ := svc.registry.Get(agent)
			path := adapter.SettingsPath(home)
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			prompt, err := assets.Read("opencode/agents/jd-judge-b.md")
			if err != nil {
				t.Fatal(err)
			}
			agents := map[string]any{
				"jd-judge-b":          map[string]any{"mode": "subagent", "hidden": true, "description": "Judgment Day blind adversarial reviewer B. Read-only; independently reports findings and does not fix code.", "prompt": prompt, "permission": map[string]any{"write": "deny", "edit": "deny", "task": "deny"}, "model": "custom"},
				"jd-judge-a":          map[string]any{"prompt": "user modified"},
				"my-agent":            map[string]any{"prompt": "mine"},
				"gentle-orchestrator": map[string]any{"prompt": "<!-- gentle-ai:orchestrator -->\nmanaged\n<!-- /gentle-ai:orchestrator -->\n", "permission": map[string]any{"task": map[string]any{"jd-judge-b": "allow", "jd-judge-a": "allow", "my-agent": "allow"}}},
			}
			if agent == model.AgentKilocode {
				agents["gentleman"] = map[string]any{"mode": "primary", "description": "Senior Architect mentor - helpful first, challenging when it matters", "prompt": "{file:./AGENTS.md}", "tools": map[string]any{"write": true, "edit": true}}
			}
			raw, err := json.Marshal(map[string]any{"agent": agents, "other": true})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				if _, err := svc.PartialUninstall([]model.AgentID{agent}, allManagedComponents); err != nil {
					t.Fatal(err)
				}
				body, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var root map[string]any
				if err := json.Unmarshal(body, &root); err != nil {
					t.Fatal(err)
				}
				remaining := root["agent"].(map[string]any)
				if _, ok := remaining["jd-judge-b"]; ok {
					t.Fatalf("managed agent retained: %s", body)
				}
				if agent == model.AgentKilocode && remaining["gentleman"] != nil {
					t.Fatalf("Kilo gentleman retained: %s", body)
				}
				if remaining["jd-judge-a"] == nil || remaining["my-agent"] == nil {
					t.Fatalf("user agents lost: %s", body)
				}
				if orchestrator, ok := remaining["gentle-orchestrator"].(map[string]any); ok {
					task := orchestrator["permission"].(map[string]any)["task"].(map[string]any)
					if _, ok := task["jd-judge-b"]; ok {
						t.Fatalf("managed task retained: %s", body)
					}
					if task["jd-judge-a"] != "allow" || task["my-agent"] != "allow" {
						t.Fatalf("user tasks lost: %s", body)
					}
				}
				info, err := os.Stat(path)
				if err != nil || info.Mode().Perm() != 0600 {
					t.Fatalf("mode: %v, %v", info, err)
				}
			}
		})
	}
}

func TestCompleteUninstallLegacyOpenCodeDefaultOwnership(t *testing.T) {
	for _, tt := range []struct {
		name, current, want string
		removeRecord        bool
	}{
		{name: "owned default rolls back", current: opencodedefault.ManagedAgent, want: "build", removeRecord: true},
		{name: "user modified default is preserved", current: "user-agent", want: "user-agent"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			settings := opencode.NewAdapter().SettingsPath(home)
			if err := os.MkdirAll(filepath.Dir(settings), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(settings, []byte(`{"default_agent":"`+tt.current+`","unrelated":true}`), 0600); err != nil {
				t.Fatal(err)
			}
			record := opencodedefault.OwnershipPath(settings)
			metadata := `{"schema":"gentle-ai.opencode-default-agent","version":1,"state":"managed","previous_state":"value","previous_default":"build"}`
			if err := os.WriteFile(record, []byte(metadata), 0600); err != nil {
				t.Fatal(err)
			}
			svc, err := NewService(home, t.TempDir(), "dev")
			if err != nil {
				t.Fatal(err)
			}
			svc.snapshotter = stubSnapshotter{}
			if _, err := svc.CompleteUninstall(); err != nil {
				t.Fatal(err)
			}
			body, err := os.ReadFile(settings)
			if err != nil {
				t.Fatal(err)
			}
			var root map[string]any
			if err := json.Unmarshal(body, &root); err != nil {
				t.Fatal(err)
			}
			if root["default_agent"] != tt.want || root["unrelated"] != true {
				t.Fatalf("settings = %s, want default_agent %q and unrelated data", body, tt.want)
			}
			_, err = os.Stat(record)
			if tt.removeRecord && !os.IsNotExist(err) || !tt.removeRecord && err != nil {
				t.Fatalf("ownership record existence mismatch: %v", err)
			}
		})
	}
}

func TestRetiredSDDDefaultUninstallSkipsLegacyCleanup(t *testing.T) {
	for _, kind := range []string{"complete", "partial"} {
		t.Run(kind, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			legacy := filepath.Join(home, ".claude", "commands", "sdd-custom.md")
			if err := os.MkdirAll(filepath.Dir(legacy), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(legacy, []byte("user customized"), 0600); err != nil {
				t.Fatal(err)
			}
			svc, err := NewService(home, t.TempDir(), "dev")
			if err != nil {
				t.Fatal(err)
			}
			svc.snapshotter = stubSnapshotter{}
			if kind == "complete" {
				_, err = svc.CompleteUninstall()
			} else {
				_, err = svc.PartialUninstall([]model.AgentID{model.AgentClaudeCode}, nil)
			}
			if err != nil {
				t.Fatalf("default uninstall dispatched retired SDD: %v", err)
			}
			if body, err := os.ReadFile(legacy); err != nil || string(body) != "user customized" {
				t.Fatalf("custom legacy file changed: %q, %v", body, err)
			}
		})
	}
}

func TestRetiredSDDExplicitUninstallFailsWithoutWrites(t *testing.T) {
	for _, kind := range []string{"partial", "profile selection"} {
		t.Run(kind, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			legacy := filepath.Join(home, ".claude", "commands", "sdd-custom.md")
			if err := os.MkdirAll(filepath.Dir(legacy), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(legacy, []byte("user customized"), 0600); err != nil {
				t.Fatal(err)
			}
			svc, err := NewService(home, t.TempDir(), "dev")
			if err != nil {
				t.Fatal(err)
			}
			before := map[string]string{}
			err = filepath.WalkDir(home, func(path string, entry fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if !entry.IsDir() {
					data, readErr := os.ReadFile(path)
					if readErr != nil {
						return readErr
					}
					before[path] = string(data)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if kind == "partial" {
				_, err = svc.PartialUninstall([]model.AgentID{model.AgentClaudeCode}, []model.ComponentID{model.ComponentSDD})
			} else {
				_, err = svc.PartialUninstallWithProfiles([]model.AgentID{model.AgentClaudeCode}, []model.ComponentID{model.ComponentSDD}, []string{"custom"}, model.EngramUninstallScopeGlobal)
			}
			if err == nil || !strings.Contains(err.Error(), "retired") {
				t.Fatalf("expected retired component rejection, got %v", err)
			}
			after := map[string]string{}
			err = filepath.WalkDir(home, func(path string, entry fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if !entry.IsDir() {
					data, readErr := os.ReadFile(path)
					if readErr != nil {
						return readErr
					}
					after[path] = string(data)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if !maps.Equal(before, after) {
				t.Fatalf("filesystem files changed: before=%v after=%v", before, after)
			}
		})
	}
}

// TestCompleteUninstallPreservesNativeReviewAndJudgmentDayAgents proves native
// review and Judgment Day agent files survive uninstall without a dedicated
// retention allowlist: nothing in the uninstall plan enumerates or removes
// the sub-agents directory at all, so these permanently-installed native
// agents (installed unconditionally by reviewassets.InstallNativeAgents,
// independent of any removable component) are never touched.
func TestCompleteUninstallPreservesNativeReviewAndJudgmentDayAgents(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	adapter := claude.NewAdapter()
	agentsDir := adapter.SubAgentsDir(home)
	if err := os.MkdirAll(agentsDir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"review-risk.md", "jd-judge-a.md"} {
		if err := os.WriteFile(filepath.Join(agentsDir, name), []byte("native agent"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	svc, err := NewService(home, t.TempDir(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	svc.snapshotter = stubSnapshotter{}
	if _, err := svc.CompleteUninstall(); err != nil {
		t.Fatalf("CompleteUninstall: %v", err)
	}
	for _, name := range []string{"review-risk.md", "jd-judge-a.md"} {
		if _, err := os.Stat(filepath.Join(agentsDir, name)); err != nil {
			t.Fatalf("native agent %s removed by uninstall: %v", name, err)
		}
	}
}

func TestUninstallOpenCodeTelemetryOwnershipAndScope(t *testing.T) {
	for _, kind := range []string{"owned", "modified", "modified-after-plan", "unowned", "other-agent", "component-only"} {
		t.Run(kind, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
			config := opencode.NewAdapter().GlobalConfigDir(home)
			paths := telemetryruntime.ManagedPaths(config)
			if kind != "unowned" {
				if _, err := telemetryruntime.Reconcile(config); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "modified" || kind == "unowned" {
				if err := os.MkdirAll(filepath.Dir(paths[0]), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(paths[0], []byte("user plugin"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			sibling := filepath.Join(filepath.Dir(paths[0]), "community.ts")
			if err := os.WriteFile(sibling, []byte("community"), 0600); err != nil {
				t.Fatal(err)
			}
			svc, err := NewService(home, t.TempDir(), "dev")
			if err != nil {
				t.Fatal(err)
			}
			agent := model.AgentOpenCode
			components := allManagedComponents
			if kind == "other-agent" {
				agent = model.AgentClaudeCode
			}
			if kind == "component-only" {
				components = []model.ComponentID{model.ComponentPersona}
			}
			plan, err := svc.buildPlan([]model.AgentID{agent}, components)
			var result Result
			if kind == "modified-after-plan" {
				if writeErr := os.WriteFile(paths[0], []byte("user plugin"), 0600); writeErr != nil {
					t.Fatal(writeErr)
				}
			}
			if err == nil {
				result, err = svc.executePlan(plan, nil)
			}
			if strings.HasPrefix(kind, "modified") || kind == "unowned" {
				if err == nil {
					t.Fatal("ownership conflict not reported")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if kind == "owned" {
				for _, path := range paths {
					if _, err := os.Stat(path); !os.IsNotExist(err) {
						t.Fatal("owned runtime remains", path)
					}
					if !slices.Contains(result.RemovedFiles, path) {
						t.Fatal("removed file unreported", path)
					}
				}
			} else {
				if _, err := os.Stat(paths[0]); err != nil {
					t.Fatal("unrelated/custom runtime removed", err)
				}
			}
			if data, err := os.ReadFile(sibling); err != nil || string(data) != "community" {
				t.Fatal("community plugin affected")
			}
		})
	}
}

type stubSnapshotter struct{}

func TestBuildPlanRemovesOnlyOwnedOpenCodeLaunchers(t *testing.T) {
	homeDir := t.TempDir()
	svc, err := NewService(homeDir, t.TempDir(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	paths := opencodeactivation.LauncherPaths(homeDir, runtime.GOOS)
	ownedPath := paths[0]
	userPath := filepath.Join(opencodeactivation.BinDir(homeDir), "user-opencode-launcher")
	for index, path := range []string{ownedPath, userPath} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		content := []byte("user launcher")
		if index == 0 {
			content = []byte("#!/bin/sh\n# " + opencodeactivation.OwnershipMarker + "\n")
		}
		if err := os.WriteFile(path, content, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	plan, err := svc.buildPlan([]model.AgentID{model.AgentOpenCode}, allManagedComponents)
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.executePlan(plan, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ownedPath); !os.IsNotExist(err) {
		t.Fatalf("owned launcher stat error = %v, want absent", err)
	}
	if data, err := os.ReadFile(userPath); err != nil || string(data) != "user launcher" {
		t.Fatalf("user launcher = %q, error = %v; want preserved", data, err)
	}
	if !slices.Contains(result.RemovedFiles, ownedPath) {
		t.Fatalf("removed files = %v, want %q", result.RemovedFiles, ownedPath)
	}
}

func TestUninstallOpenCodeClearsBackgroundIntent(t *testing.T) {
	homeDir := t.TempDir()
	if err := state.Write(homeDir, state.InstallState{
		InstalledAgents:  []string{"opencode"},
		BackgroundIntent: model.OpenCodeBackgroundOn,
	}); err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(homeDir, t.TempDir(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	svc.snapshotter = stubSnapshotter{}
	if _, err := svc.PartialUninstall([]model.AgentID{model.AgentOpenCode}, allManagedComponents); err != nil {
		t.Fatal(err)
	}
	got, err := state.Read(homeDir)
	if err != nil {
		t.Fatal(err)
	}
	if got.BackgroundIntent != "" || len(got.InstalledAgents) != 0 {
		t.Fatalf("state after uninstall = %#v, want no OpenCode intent or installed agent", got)
	}
}

// TestPartialUninstallKeepsSharedGGAWhileOtherAgentInstalled covers #3534:
// uninstalling one agent must not wipe the shared GGA config that another
// still-installed agent depends on, while agent-scoped assets (here,
// OpenCode's gentle-logo TUI plugin) are still removed as before.
func TestPartialUninstallKeepsSharedGGAWhileOtherAgentInstalled(t *testing.T) {
	homeDir := t.TempDir()
	if err := state.Write(homeDir, state.InstallState{
		InstalledAgents: []string{"claude-code", "opencode"},
	}); err != nil {
		t.Fatal(err)
	}

	configPath := gga.ConfigPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("shared gga config"), 0o644); err != nil {
		t.Fatal(err)
	}

	logoPath := filepath.Join(homeDir, ".config", "opencode", "tui-plugins", "gentle-logo.tsx")
	if err := os.MkdirAll(filepath.Dir(logoPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logoPath, []byte("logo"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc, err := NewService(homeDir, t.TempDir(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	svc.snapshotter = stubSnapshotter{}

	result, err := svc.PartialUninstall([]model.AgentID{model.AgentOpenCode}, allManagedComponents)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("shared GGA config = %v, want it to survive because claude-code is still installed", err)
	}
	if _, err := os.Stat(logoPath); !os.IsNotExist(err) {
		t.Fatalf("opencode gentle-logo stat error = %v, want removed", err)
	}
	if !slices.Contains(result.RemovedFiles, logoPath) {
		t.Fatalf("removed files = %v, want %q", result.RemovedFiles, logoPath)
	}

	foundNote := false
	for _, action := range result.ManualActions {
		if strings.Contains(action, "GGA") && strings.Contains(action, "claude-code") {
			foundNote = true
		}
	}
	if !foundNote {
		t.Fatalf("manual actions = %v, want a note explaining the kept shared GGA config", result.ManualActions)
	}

	got, err := state.Read(homeDir)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.InstalledAgents, []string{"claude-code"}) {
		t.Fatalf("installed agents after uninstall = %v, want [claude-code]", got.InstalledAgents)
	}
}

// TestPartialUninstallRemovesSharedGGAOnLastAgent covers the other half of
// #3534: once the last agent that depends on GGA is uninstalled, the shared
// config is removed like any other managed component.
func TestPartialUninstallRemovesSharedGGAOnLastAgent(t *testing.T) {
	homeDir := t.TempDir()
	if err := state.Write(homeDir, state.InstallState{
		InstalledAgents: []string{"opencode"},
	}); err != nil {
		t.Fatal(err)
	}

	configPath := gga.ConfigPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("shared gga config"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc, err := NewService(homeDir, t.TempDir(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	svc.snapshotter = stubSnapshotter{}

	result, err := svc.PartialUninstall([]model.AgentID{model.AgentOpenCode}, allManagedComponents)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("shared GGA config stat error = %v, want removed once no agent needs it", err)
	}
	if !slices.Contains(result.RemovedFiles, configPath) {
		t.Fatalf("removed files = %v, want %q", result.RemovedFiles, configPath)
	}
	for _, action := range result.ManualActions {
		if strings.Contains(action, "GGA") {
			t.Fatalf("manual actions = %v, want no kept-GGA note on last-agent removal", result.ManualActions)
		}
	}
}

// TestPartialUninstallExplicitGGARequestSkippedWhileSharedWithAnotherAgent
// covers requesting `--components gga` explicitly for one agent while
// another installed agent still relies on it: the safer behavior is to skip
// the shared removal (rather than fail the whole command) and say why,
// instead of silently deleting a config another agent still needs.
func TestPartialUninstallExplicitGGARequestSkippedWhileSharedWithAnotherAgent(t *testing.T) {
	homeDir := t.TempDir()
	if err := state.Write(homeDir, state.InstallState{
		InstalledAgents: []string{"claude-code", "opencode"},
	}); err != nil {
		t.Fatal(err)
	}

	configPath := gga.ConfigPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("shared gga config"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc, err := NewService(homeDir, t.TempDir(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	svc.snapshotter = stubSnapshotter{}

	result, err := svc.PartialUninstall([]model.AgentID{model.AgentOpenCode}, []model.ComponentID{model.ComponentGGA})
	if err != nil {
		t.Fatalf("explicit --components gga while shared should be skipped, not fail: %v", err)
	}

	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("shared GGA config = %v, want it to survive an explicit but unsafe removal request", err)
	}
	foundNote := false
	for _, action := range result.ManualActions {
		if strings.Contains(action, "GGA") && strings.Contains(action, "claude-code") {
			foundNote = true
		}
	}
	if !foundNote {
		t.Fatalf("manual actions = %v, want a note explaining why the explicit gga removal was skipped", result.ManualActions)
	}
}

// TestIsStateNotFoundUnwrapsWrappedError guards the classification used by
// otherInstalledAgents: it must follow the error-wrap chain (via errors.Is)
// rather than only recognizing a bare *fs.PathError the way os.IsNotExist
// effectively does for a caller that wraps state.Read's error further.
func TestIsStateNotFoundUnwrapsWrappedError(t *testing.T) {
	wrapped := fmt.Errorf("read install state: %w", fs.ErrNotExist)
	if !isStateNotFound(wrapped) {
		t.Fatalf("isStateNotFound(%v) = false, want true for a wrapped not-exist error", wrapped)
	}

	other := fmt.Errorf("read install state: %w", os.ErrPermission)
	if isStateNotFound(other) {
		t.Fatalf("isStateNotFound(%v) = true, want false for a non-not-exist error", other)
	}
}

// TestReconcileSharedComponentsWithUnreadableState covers a corrupt
// state.json: reconcileSharedComponents cannot tell whether another agent
// still needs GGA, so it fails closed toward preservation — GGA is dropped
// from the returned component set and a note explains why — instead of
// aborting the caller's partial uninstall, unless the caller explicitly
// requested "--components gga", where there is nothing safe left to do for
// that explicit request and the error is returned instead.
func TestReconcileSharedComponentsWithUnreadableState(t *testing.T) {
	homeDir := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(state.Path(homeDir)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(state.Path(homeDir), []byte("{not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc, err := NewService(homeDir, t.TempDir(), "dev")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("default components keep GGA and continue", func(t *testing.T) {
		components, note, err := svc.reconcileSharedComponents(
			[]model.AgentID{model.AgentOpenCode}, slices.Clone(allManagedComponents), false)
		if err != nil {
			t.Fatalf("reconcileSharedComponents() error = %v, want nil (fail closed, don't abort)", err)
		}
		if slices.Contains(components, model.ComponentGGA) {
			t.Fatalf("components = %v, want GGA dropped when state cannot be read", components)
		}
		if !slices.Contains(components, model.ComponentTheme) {
			t.Fatalf("components = %v, want unrelated components preserved so the rest of the uninstall proceeds", components)
		}
		if !strings.Contains(note, "GGA") || !strings.Contains(note, "could not be read") {
			t.Fatalf("note = %q, want it to explain the state could not be read", note)
		}
	})

	t.Run("explicit gga request fails instead of silently skipping", func(t *testing.T) {
		_, _, err := svc.reconcileSharedComponents(
			[]model.AgentID{model.AgentOpenCode}, []model.ComponentID{model.ComponentGGA}, true)
		if err == nil {
			t.Fatal("reconcileSharedComponents() error = nil, want an error for an explicit --components gga request when state is unreadable")
		}
	})
}

func TestBuildPlanSnapshotsPiManifestAndOwnedOverlay(t *testing.T) {
	homeDir := t.TempDir()
	svc, err := NewService(homeDir, t.TempDir(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	packageChild := filepath.Join(homeDir, ".pi", "agent", "node_modules", "gentle-pi", "subagents", "package.md")
	if err := os.MkdirAll(filepath.Dir(packageChild), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(packageChild, []byte("---\ntools: bash\n---\npackage\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := communitytool.ReconcilePiCodeGraph(communitytool.PiCodeGraphOptions{HomeDir: homeDir, Selected: true, EffectiveMCPProbe: piCodeGraphProbeForServiceTest}); err != nil {
		t.Fatal(err)
	}
	plan, err := svc.buildPlan([]model.AgentID{model.AgentPi}, nil)
	if err != nil {
		t.Fatal(err)
	}
	paths := communitytool.PiCodeGraphPaths(homeDir, "")
	for _, path := range paths {
		if !slices.Contains(plan.backupTargets, path) {
			t.Fatalf("backup targets = %v, missing Pi artifact %q", plan.backupTargets, path)
		}
	}
	promptPath := pi.NewAdapter().SystemPromptFile(homeDir)
	if !slices.Contains(plan.backupTargets, promptPath) {
		t.Fatalf("backup targets = %v, missing Pi system prompt file %q", plan.backupTargets, promptPath)
	}
}

// TestExecutePlanRetiresStalePiSystemPromptBlocks covers issue #4057: a Pi
// install made before SupportsSystemPrompt()==false for Pi left gentle-ai
// managed blocks in ~/.pi/agent/APPEND_SYSTEM.md. Since adapter.SupportsSystemPrompt()
// is false, componentOperations() never queues a rewrite op for that file, so
// uninstall must retire the stale blocks directly.
func TestExecutePlanRetiresStalePiSystemPromptBlocks(t *testing.T) {
	home := t.TempDir()
	svc, err := NewService(home, t.TempDir(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	svc.snapshotter = stubSnapshotter{}

	promptPath := pi.NewAdapter().SystemPromptFile(home)
	if err := os.MkdirAll(filepath.Dir(promptPath), 0o755); err != nil {
		t.Fatal(err)
	}
	stale := "user text\n\n<!-- gentle-ai:sdd-orchestrator -->\nSDD body\n<!-- /gentle-ai:sdd-orchestrator -->\n"
	if err := os.WriteFile(promptPath, []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := svc.executePlan(plan{}, []model.AgentID{model.AgentPi})
	if err != nil {
		t.Fatal(err)
	}

	got := string(mustReadServiceFile(t, promptPath))
	want := "user text\n\n\n"
	if got != want {
		t.Fatalf("APPEND_SYSTEM.md = %q, want %q", got, want)
	}
	if !slices.Contains(result.ChangedFiles, promptPath) {
		t.Fatalf("ChangedFiles = %v, want to contain %q", result.ChangedFiles, promptPath)
	}
}

func TestExecutePlanCleansPiBeforeSharedMCPMutation(t *testing.T) {
	home := t.TempDir()
	svc, err := NewService(home, t.TempDir(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	svc.snapshotter = stubSnapshotter{}
	mcpPath := filepath.Join(home, ".pi", "agent", "mcp.json")
	child := filepath.Join(home, ".pi", "agent", "subagents", "worker.md")
	if err := os.MkdirAll(filepath.Dir(child), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(child, []byte("---\ntools: bash\n---\nwork\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := communitytool.ReconcilePiCodeGraph(communitytool.PiCodeGraphOptions{HomeDir: home, Selected: true, EffectiveMCPProbe: piCodeGraphProbeForServiceTest}); err != nil {
		t.Fatal(err)
	}
	result, err := svc.executePlan(plan{operations: []operation{{path: mcpPath, apply: func(path string) (bool, bool, error) {
		return true, false, os.WriteFile(path, []byte(`{"mcpServers":{"engram":{"command":"engram"}}}`), 0o600)
	}}}}, []model.AgentID{model.AgentPi})
	if err != nil {
		t.Fatal(err)
	}
	if body := string(mustReadServiceFile(t, mcpPath)); strings.Contains(body, `"codegraph"`) {
		t.Fatalf("false drift preserved CodeGraph entry: %s", body)
	}
	if slices.ContainsFunc(result.ManualActions, func(action string) bool { return strings.Contains(action, "CodeGraph MCP drifted") }) {
		t.Fatalf("manual actions = %v, want no false drift", result.ManualActions)
	}
}

func mustReadServiceFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestExecutePlanPiUninstallPreservesPreexistingMarkedUserChildAndUserMCP(t *testing.T) {
	homeDir := t.TempDir()
	svc, err := NewService(homeDir, t.TempDir(), "dev")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.snapshotter = stubSnapshotter{}
	mcpPath := filepath.Join(homeDir, ".pi", "agent", "mcp.json")
	childPath := filepath.Join(homeDir, ".pi", "agent", "subagents", "worker.md")
	preexisting := "---\ntools: bash, mcp\n---\nuser instructions\n\n<!-- gentle-ai:pi-codegraph-tool -->\npreexisting tool guidance\n<!-- /gentle-ai:pi-codegraph -->\n\n<!-- gentle-ai:pi-codegraph-guidance -->\npreexisting lazy-init guidance\n<!-- /gentle-ai:pi-codegraph -->\n"
	if err := os.MkdirAll(filepath.Dir(mcpPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mcpPath, []byte(`{"mcpServers":{"user":{"command":"user-server"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(childPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(childPath, []byte(preexisting), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := communitytool.ReconcilePiCodeGraph(communitytool.PiCodeGraphOptions{HomeDir: homeDir, Selected: true, EffectiveMCPProbe: piCodeGraphProbeForServiceTest}); err != nil {
		t.Fatalf("ReconcilePiCodeGraph() error = %v", err)
	}

	result, err := svc.executePlan(plan{}, []model.AgentID{model.AgentPi})
	if err != nil {
		t.Fatalf("executePlan() error = %v", err)
	}
	if got, err := os.ReadFile(childPath); err != nil || string(got) != preexisting {
		t.Fatalf("service uninstall child = %q, err = %v; want exact preexisting content", got, err)
	}
	if got, err := os.ReadFile(mcpPath); err != nil || !strings.Contains(string(got), "user-server") || strings.Contains(string(got), `"codegraph"`) {
		t.Fatalf("service uninstall MCP = %q, err = %v", got, err)
	}
	if len(result.ManualActions) != 0 {
		t.Fatalf("service uninstall manual actions = %v, want none", result.ManualActions)
	}
	if _, err := svc.executePlan(plan{}, []model.AgentID{model.AgentPi}); err != nil {
		t.Fatalf("repeat service uninstall error = %v", err)
	}
}

func TestExecutePlanPiUninstallPreservesDriftedChildAndGentlePiSource(t *testing.T) {
	homeDir := t.TempDir()
	svc, err := NewService(homeDir, t.TempDir(), "dev")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.snapshotter = stubSnapshotter{}
	childPath := filepath.Join(homeDir, ".pi", "agent", "subagents", "worker.md")
	packageChild := filepath.Join(homeDir, ".pi", "agent", "node_modules", "gentle-pi", "subagents", "package.md")
	packageBody := "---\ntools: bash\n---\npackage instructions\n"
	if err := os.MkdirAll(filepath.Dir(childPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(childPath, []byte("---\ntools: bash\n---\nuser instructions\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(packageChild), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(packageChild, []byte(packageBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := communitytool.ReconcilePiCodeGraph(communitytool.PiCodeGraphOptions{HomeDir: homeDir, Selected: true, EffectiveMCPProbe: piCodeGraphProbeForServiceTest}); err != nil {
		t.Fatalf("ReconcilePiCodeGraph() error = %v", err)
	}
	if err := os.WriteFile(childPath, append([]byte("user changed after provision\n"), []byte("keep this\n")...), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := svc.executePlan(plan{}, []model.AgentID{model.AgentPi})
	if err != nil {
		t.Fatalf("executePlan() error = %v", err)
	}
	if got, err := os.ReadFile(childPath); err != nil || !strings.Contains(string(got), "keep this") {
		t.Fatalf("drifted child was not preserved: %q, err = %v", got, err)
	}
	if got, err := os.ReadFile(packageChild); err != nil || string(got) != packageBody {
		t.Fatalf("gentle-pi package child changed: %q, err = %v", got, err)
	}
	if !slices.ContainsFunc(result.ManualActions, func(action string) bool { return strings.Contains(action, "child drifted") }) {
		t.Fatalf("manual actions = %v, want drift action", result.ManualActions)
	}
}

func TestPartialUninstallPiReportsRetainedResourcesAndOptionalCleanup(t *testing.T) {
	homeDir := t.TempDir()
	workspaceDir := t.TempDir()
	svc, err := NewService(homeDir, workspaceDir, "dev")
	if err != nil {
		t.Fatal(err)
	}
	svc.snapshotter = stubSnapshotter{}

	retainedPaths := []string{
		filepath.Join(homeDir, ".pi", "agent", "agents"),
		filepath.Join(homeDir, ".pi", "agent", "chains"),
		filepath.Join(homeDir, ".pi", "agent", "gentle-ai"),
		filepath.Join(homeDir, ".pi", "agent", "subagents.json"),
		filepath.Join(homeDir, ".pi", "gentle-ai"),
		filepath.Join(workspaceDir, ".pi", "gentle-ai"),
	}
	for _, path := range retainedPaths {
		if filepath.Ext(path) == ".json" {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(`{"user":"owned"}`), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	result, err := svc.PartialUninstall([]model.AgentID{model.AgentPi}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(result.RetainedPiResources, retainedPaths) {
		t.Fatalf("RetainedPiResources = %v, want %v", result.RetainedPiResources, retainedPaths)
	}
	wantCommands := []string{
		"pi remove npm:gentle-pi",
		"pi remove npm:gentle-engram",
		"pi remove npm:pi-mcp-adapter",
		"pi remove npm:pi-web-access",
		"pi remove npm:pi-btw",
	}
	if !slices.Equal(result.OptionalPiPackageCleanupCommands, wantCommands) {
		t.Fatalf("OptionalPiPackageCleanupCommands = %v, want %v", result.OptionalPiPackageCleanupCommands, wantCommands)
	}
	for _, path := range retainedPaths {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("retained Pi resource %q was deleted: %v", path, err)
		}
		if slices.Contains(result.RemovedDirectories, path) || slices.Contains(result.RemovedFiles, path) {
			t.Fatalf("retained Pi resource %q was reported as removed: %#v", path, result)
		}
	}
}

func TestPartialUninstallPiDoesNotReportAbsentRetainedResources(t *testing.T) {
	svc, err := NewService(t.TempDir(), t.TempDir(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	svc.snapshotter = stubSnapshotter{}

	result, err := svc.PartialUninstall([]model.AgentID{model.AgentPi}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.RetainedPiResources) != 0 {
		t.Fatalf("RetainedPiResources = %v, want none for absent paths", result.RetainedPiResources)
	}
	t.Run("dangling link", func(t *testing.T) {
		path := filepath.Join(svc.homeDir, ".pi", "gentle-ai")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(svc.homeDir, "missing"), path); err != nil {
			if runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(1314)) {
				t.Skipf("symlink privilege unavailable: %v", err)
			}
			t.Fatal(err)
		}
		result, err := svc.PartialUninstall([]model.AgentID{model.AgentPi}, nil)
		if target, linkErr := os.Readlink(path); err != nil || linkErr != nil || target != filepath.Join(svc.homeDir, "missing") || !slices.Contains(result.RetainedPiResources, path) {
			t.Fatalf("dangling link must remain and be reported: result=%+v err=%v linkErr=%v", result, err, linkErr)
		}
	})
}

func TestCompleteUninstallKeepsExecutableRemovalAction(t *testing.T) {
	svc, err := NewService(t.TempDir(), t.TempDir(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	svc.snapshotter = stubSnapshotter{}

	result, err := svc.CompleteUninstall()
	if err != nil {
		t.Fatal(err)
	}
	want := "To completely remove gentle-ai from your system, delete the executable (e.g., rm -f $(which gentle-ai))"
	if !slices.Contains(result.ManualActions, want) {
		t.Fatalf("ManualActions = %v, want %q", result.ManualActions, want)
	}
}

func piCodeGraphProbeForServiceTest(string) (communitytool.PiCodeGraphMCPProbeResult, error) {
	return communitytool.PiCodeGraphMCPProbeResult{
		AdapterAvailable: true,
		Initialized:      true,
		Tools: []communitytool.PiCodeGraphMCPTool{{
			Name: "codegraph_explore",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":       map[string]any{"type": "string"},
					"maxFiles":    map[string]any{"type": "number"},
					"projectPath": map[string]any{"type": "string"},
				},
				"required": []any{"query"},
			},
		}},
	}, nil
}

func TestExpandVisualPolishUninstallComponents(t *testing.T) {
	for _, trigger := range []model.ComponentID{model.ComponentTheme, model.ComponentOpenCodeGentleLogo} {
		got := expandVisualPolishUninstallComponents([]model.ComponentID{trigger})
		for _, want := range model.VisualPolishComponents() {
			if !slices.Contains(got, want) {
				t.Fatalf("%q expansion missing %q: %v", trigger, want, got)
			}
		}
	}
	for _, component := range []model.ComponentID{model.ComponentClaudeTheme, model.ComponentPersona} {
		got := expandVisualPolishUninstallComponents([]model.ComponentID{component})
		if !slices.Equal(got, []model.ComponentID{component}) {
			t.Fatalf("%q should not expand: %v", component, got)
		}
	}
}

func TestPartialUninstallClaudeThemeRemovesOnlyThemeAssets(t *testing.T) {
	homeDir := t.TempDir()
	workspaceDir := t.TempDir()

	svc, err := NewService(homeDir, workspaceDir, "dev")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.snapshotter = stubSnapshotter{}

	opencodeAdapter, ok := svc.registry.Get(model.AgentOpenCode)
	if !ok {
		t.Fatal("opencode adapter not found in registry")
	}
	settingsPath := opencodeAdapter.SettingsPath(homeDir)
	settings := `{"theme":"active","keep":true,"agent":{"sdd-apply":{"model":"keep"}}}`
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath, []byte(settings), 0o644); err != nil {
		t.Fatal(err)
	}

	logoPath := filepath.Join(homeDir, ".config", "opencode", "tui-plugins", "gentle-logo.tsx")
	if err := os.MkdirAll(filepath.Dir(logoPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logoPath, []byte("// managed logo"), 0o644); err != nil {
		t.Fatal(err)
	}

	managed := []string{
		filepath.Join(homeDir, ".claude", "themes", "gentleman.json"),
		filepath.Join(homeDir, ".claude", "themes", "gentleman-cute.json"),
		filepath.Join(homeDir, ".config", "opencode", "themes", "gentleman.json"),
		filepath.Join(homeDir, ".config", "opencode", "themes", "gentleman-cute.json"),
	}
	for _, path := range managed {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(`{"name":"managed"}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	preserved := map[string]string{
		filepath.Join(homeDir, ".claude", "settings.json"):                     `{"theme":"active","outputStyle":"gentleman"}`,
		filepath.Join(homeDir, ".claude", "CLAUDE.md"):                         "# persona\n",
		filepath.Join(homeDir, ".claude", "output-styles", "gentleman.md"):     "# output style\n",
		filepath.Join(homeDir, ".claude", "commands", "gentle-sdd-apply.md"):   "# SDD asset\n",
		filepath.Join(homeDir, ".config", "opencode", "tui.json"):              `{"plugins":["./tui-plugins/gentle-logo.tsx"]}`,
		filepath.Join(homeDir, ".config", "opencode", "themes", "custom.json"): `{"theme":"custom"}`,
	}
	for path, content := range preserved {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := svc.PartialUninstall(
		[]model.AgentID{model.AgentOpenCode, model.AgentClaudeCode},
		[]model.ComponentID{model.ComponentClaudeTheme},
	); err != nil {
		t.Fatalf("PartialUninstall() error = %v", err)
	}

	for _, path := range managed {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("managed theme %q should be removed: %v", path, err)
		}
	}
	for path, want := range preserved {
		if got, err := os.ReadFile(path); err != nil || string(got) != want {
			t.Fatalf("preserved asset %q = %q, %v; want %q", path, got, err, want)
		}
	}
	if got, err := os.ReadFile(settingsPath); err != nil || string(got) != settings {
		t.Fatalf("OpenCode settings = %q, %v; want unchanged %q", got, err, settings)
	}
	if got, err := os.ReadFile(logoPath); err != nil || string(got) != "// managed logo" {
		t.Fatalf("OpenCode logo = %q, %v", got, err)
	}
}

func TestPartialUninstallOpenCodeLogoScopedToOpenCodeAgent(t *testing.T) {
	homeDir := t.TempDir()
	workspaceDir := t.TempDir()

	svc, err := NewService(homeDir, workspaceDir, "dev")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.snapshotter = stubSnapshotter{}

	logoPath := filepath.Join(homeDir, ".config", "opencode", "tui-plugins", "gentle-logo.tsx")
	if err := os.MkdirAll(filepath.Dir(logoPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logoPath, []byte("// managed logo"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Uninstalling an unrelated agent must not touch the OpenCode-owned logo —
	// both when the logo component is named explicitly and, the issue's actual
	// repro, when the default component list (nil) includes it (#4229).
	for _, components := range [][]model.ComponentID{
		{model.ComponentOpenCodeGentleLogo},
		nil, // default: allManagedComponents
	} {
		if _, err := svc.PartialUninstall(
			[]model.AgentID{model.AgentGeminiCLI},
			components,
		); err != nil {
			t.Fatalf("PartialUninstall(gemini-cli, %v) error = %v", components, err)
		}
		if got, err := os.ReadFile(logoPath); err != nil || string(got) != "// managed logo" {
			t.Fatalf("OpenCode logo removed by gemini-cli uninstall (components %v): got %q, %v; want preserved", components, got, err)
		}
	}

	// Positive control: the OpenCode adapter itself still removes the logo.
	if _, err := svc.PartialUninstall(
		[]model.AgentID{model.AgentOpenCode},
		[]model.ComponentID{model.ComponentOpenCodeGentleLogo},
	); err != nil {
		t.Fatalf("PartialUninstall(opencode) error = %v", err)
	}
	if _, err := os.Stat(logoPath); !os.IsNotExist(err) {
		t.Fatalf("OpenCode logo should be removed by opencode uninstall: %v", err)
	}
}

func readJSONFileForTest(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", path, err)
	}
	return root
}

func (stubSnapshotter) Create(snapshotDir string, paths []string) (backup.Manifest, error) {
	if err := os.MkdirAll(snapshotDir, 0o755); err != nil {
		return backup.Manifest{}, err
	}
	return backup.Manifest{
		ID:        "snapshot-001",
		CreatedAt: time.Now().UTC(),
	}, nil
}

func TestExecutePlanReportsManualCleanupForNonEmptyDirectory(t *testing.T) {
	homeDir := t.TempDir()
	workspaceDir := t.TempDir()

	svc, err := NewService(homeDir, workspaceDir, "dev")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.snapshotter = stubSnapshotter{}
	svc.now = func() time.Time { return time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC) }

	nonEmptyDir := filepath.Join(homeDir, ".config", "opencode", "skills")
	if err := os.MkdirAll(nonEmptyDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(nonEmptyDir, "user-skill.md"), []byte("keep me"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	statePath := filepath.Join(homeDir, ".gentle-ai", "state.json")
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatalf("MkdirAll(state dir) error = %v", err)
	}
	if err := os.WriteFile(statePath, []byte(`{"installed_agents":[]}`), 0o644); err != nil {
		t.Fatalf("WriteFile(state) error = %v", err)
	}

	result, err := svc.executePlan(plan{
		backupTargets: []string{statePath},
		operations: []operation{
			removeDirIfEmpty(nonEmptyDir),
		},
	}, []model.AgentID{})
	if err != nil {
		t.Fatalf("executePlan() error = %v", err)
	}

	if len(result.ManualActions) != 1 {
		t.Fatalf("ManualActions len = %d, want 1; got %v", len(result.ManualActions), result.ManualActions)
	}
	if !strings.Contains(result.ManualActions[0], nonEmptyDir) {
		t.Fatalf("manual action should mention %q, got %q", nonEmptyDir, result.ManualActions[0])
	}
}

func TestComponentOperationsContext7ClaudeRemovesSettingsAndManagedLegacyFile(t *testing.T) {
	homeDir := t.TempDir()
	workspaceDir := t.TempDir()

	svc, err := NewService(homeDir, workspaceDir, "dev")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	adapter, ok := svc.registry.Get(model.AgentClaudeCode)
	if !ok {
		t.Fatal("Claude adapter not found in registry")
	}

	settingsPath := adapter.SettingsPath(homeDir)
	legacyPath := adapter.MCPConfigPath(homeDir, "context7")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(settings dir) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(legacy dir) error = %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte(`{"mcpServers":{"context7":{"command":"npx"},"engram":{"command":"engram"}},"theme":"dark"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(settings) error = %v", err)
	}
	legacyManaged := []byte(`{
  "command": "npx",
  "args": [
    "-y",
    "--package=@upstash/context7-mcp@1.0.0",
    "--",
    "context7-mcp"
  ]
}
`)
	if err := os.WriteFile(legacyPath, legacyManaged, 0o644); err != nil {
		t.Fatalf("WriteFile(legacy) error = %v", err)
	}

	ops, targets, err := svc.componentOperations(adapter, model.ComponentContext7)
	if err != nil {
		t.Fatalf("componentOperations() error = %v", err)
	}
	if !slices.Contains(targets, settingsPath) || !slices.Contains(targets, legacyPath) {
		t.Fatalf("targets = %#v, want settings and legacy paths", targets)
	}
	for _, op := range ops {
		if _, _, err := op.apply(op.path); err != nil {
			t.Fatalf("operation %v on %q error = %v", op.typeID, op.path, err)
		}
	}

	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy managed context7 file should be removed; stat err = %v", err)
	}
	settings := readJSONFileForTest(t, settingsPath)
	mcpServers := settings["mcpServers"].(map[string]any)
	if _, ok := mcpServers["context7"]; ok {
		t.Fatalf("settings still contains mcpServers.context7: %#v", settings)
	}
	if _, ok := mcpServers["engram"]; !ok {
		t.Fatalf("settings lost unrelated mcpServers.engram: %#v", settings)
	}
}

func TestComponentOperationsClaudeNeverDeleteUserRegistry(t *testing.T) {
	homeDir := t.TempDir()
	workspaceDir := t.TempDir()

	svc, err := NewService(homeDir, workspaceDir, "dev")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	adapter, ok := svc.registry.Get(model.AgentClaudeCode)
	if !ok {
		t.Fatal("Claude adapter not found in registry")
	}

	// The registry holds ONLY the managed server, so removing it empties the
	// file; ~/.claude.json must survive because Claude Code owns it.
	registryPath := claude.UserConfigPath(homeDir)
	seed := []byte(`{"mcpServers":{"context7":{"command":"npx"}}}`)
	if err := os.WriteFile(registryPath, seed, 0o600); err != nil {
		t.Fatalf("WriteFile(registry) error = %v", err)
	}

	ops, _, err := svc.componentOperations(adapter, model.ComponentContext7)
	if err != nil {
		t.Fatalf("componentOperations(context7) error = %v", err)
	}
	for _, op := range ops {
		if _, _, err := op.apply(op.path); err != nil {
			t.Fatalf("operation %v on %q error = %v", op.typeID, op.path, err)
		}
	}

	info, err := os.Stat(registryPath)
	if err != nil {
		t.Fatalf("~/.claude.json must survive removing the last managed server: %v", err)
	}
	registry := readJSONFileForTest(t, registryPath)
	if servers, ok := registry["mcpServers"].(map[string]any); ok {
		if _, still := servers["context7"]; still {
			t.Fatalf("registry still contains mcpServers.context7: %#v", registry)
		}
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("registry mode widened to %v, want 0600", info.Mode().Perm())
	}
}

func TestComponentOperationsEngramClaudePreservesRegistryAndRemovesManagedLegacy(t *testing.T) {
	homeDir := t.TempDir()
	svc, err := NewService(homeDir, t.TempDir(), "dev")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	adapter, ok := svc.registry.Get(model.AgentClaudeCode)
	if !ok {
		t.Fatal("Claude adapter not found in registry")
	}

	registryPath := claude.UserConfigPath(homeDir)
	registry := []byte(`{"oauthAccount":{"emailAddress":"user@example.com"},"mcpServers":{"codegraph":{"command":"codegraph"},"engram":{"command":"/usr/local/bin/engram","args":["mcp","--tools=agent"]}}}`)
	if err := os.WriteFile(registryPath, registry, 0o600); err != nil {
		t.Fatalf("WriteFile(user registry) error = %v", err)
	}
	legacyPath := adapter.MCPConfigPath(homeDir, "engram")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(legacy dir) error = %v", err)
	}
	if err := os.WriteFile(legacyPath, []byte(`{"command":"/usr/local/bin/engram","args":["mcp","--tools=agent"]}`), 0o644); err != nil {
		t.Fatalf("WriteFile(legacy config) error = %v", err)
	}

	ops, targets, err := svc.componentOperations(adapter, model.ComponentEngram)
	if err != nil {
		t.Fatalf("componentOperations(engram) error = %v", err)
	}
	for _, want := range []string{registryPath, legacyPath} {
		if !slices.Contains(targets, want) {
			t.Fatalf("uninstall targets missing %q: %v", want, targets)
		}
	}
	for _, op := range ops {
		if _, _, err := op.apply(op.path); err != nil {
			t.Fatalf("operation %v on %q error = %v", op.typeID, op.path, err)
		}
	}

	remaining := readJSONFileForTest(t, registryPath)
	servers, _ := remaining["mcpServers"].(map[string]any)
	if _, exists := servers["engram"]; exists {
		t.Fatalf("user registry still contains mcpServers.engram: %#v", remaining)
	}
	if got := remaining["oauthAccount"].(map[string]any)["emailAddress"]; got != "user@example.com" {
		t.Fatalf("OAuth data changed during uninstall: %#v", remaining)
	}
	if _, statErr := os.Stat(legacyPath); !os.IsNotExist(statErr) {
		t.Fatalf("managed legacy config must be removed; stat error = %v", statErr)
	}
	if info, statErr := os.Stat(registryPath); statErr != nil {
		t.Fatalf("Stat(user registry) error = %v", statErr)
	} else if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("user registry mode = %o; want 0600", info.Mode().Perm())
	}
}

func TestComponentOperationsContext7ClaudePreservesCustomLegacyFile(t *testing.T) {
	homeDir := t.TempDir()
	workspaceDir := t.TempDir()

	svc, err := NewService(homeDir, workspaceDir, "dev")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	adapter, ok := svc.registry.Get(model.AgentClaudeCode)
	if !ok {
		t.Fatal("Claude adapter not found in registry")
	}

	settingsPath := adapter.SettingsPath(homeDir)
	legacyPath := adapter.MCPConfigPath(homeDir, "context7")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(settings dir) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(legacy dir) error = %v", err)
	}
	custom := []byte(`{"command":"custom-context7"}`)
	if err := os.WriteFile(settingsPath, []byte(`{"mcpServers":{"context7":{"command":"npx"}}}`), 0o644); err != nil {
		t.Fatalf("WriteFile(settings) error = %v", err)
	}
	if err := os.WriteFile(legacyPath, custom, 0o644); err != nil {
		t.Fatalf("WriteFile(legacy) error = %v", err)
	}

	ops, _, err := svc.componentOperations(adapter, model.ComponentContext7)
	if err != nil {
		t.Fatalf("componentOperations() error = %v", err)
	}
	for _, op := range ops {
		if _, _, err := op.apply(op.path); err != nil {
			t.Fatalf("operation %v on %q error = %v", op.typeID, op.path, err)
		}
	}

	got, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatalf("ReadFile(legacy) error = %v", err)
	}
	if string(got) != string(custom) {
		t.Fatalf("custom legacy file changed: %s", string(got))
	}
}

func TestPartialUninstallOpenCodePluginsAndModelVariantsWithoutSDD(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(homeDir, "xdg"))
	workspaceDir := t.TempDir()

	svc, err := NewService(homeDir, workspaceDir, "dev")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	pluginDir := filepath.Join(opencode.NewAdapter().GlobalConfigDir(homeDir), "plugins")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(pluginDir) error = %v", err)
	}
	backgroundAgentsPath := filepath.Join(pluginDir, "background-agents.ts")
	modelVariantsPluginPath := filepath.Join(pluginDir, "model-variants.ts")
	skillRegistryPluginPath := filepath.Join(pluginDir, "skill-registry.ts")
	thirdPartyPluginPath := filepath.Join(pluginDir, "third-party.ts")
	modifiedPluginPath := filepath.Join(pluginDir, "opencode-review-transport.ts")
	for _, path := range []string{modelVariantsPluginPath, skillRegistryPluginPath} {
		body, err := assets.Read("opencode/plugins/" + filepath.Base(path))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", path, err)
		}
	}
	if err := os.WriteFile(modifiedPluginPath, []byte("modified managed plugin"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backgroundAgentsPath, []byte("user background plugin"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(thirdPartyPluginPath, []byte("third-party"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", thirdPartyPluginPath, err)
	}

	cacheDir := filepath.Join(homeDir, ".gentle-ai", "cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(cacheDir) error = %v", err)
	}
	modelVariantsCachePath := filepath.Join(cacheDir, "model-variants.json")
	modelVariantsTempPath := filepath.Join(cacheDir, "model-variants.json.tmp")
	modelVariantsRandomTempPath := filepath.Join(cacheDir, "model-variants.json.a1b2c3.tmp")
	unrelatedCachePath := filepath.Join(cacheDir, "keep.txt")
	unrelatedTempPaths := []string{
		filepath.Join(cacheDir, "model-variants.json.abc12.tmp"),
		filepath.Join(cacheDir, "model-variants.json.abc1234.tmp"),
		filepath.Join(cacheDir, "model-variants.json.ABC123.tmp"),
		filepath.Join(cacheDir, "model-variants.json.notes.tmp"),
	}
	for _, path := range append([]string{modelVariantsCachePath, modelVariantsTempPath, modelVariantsRandomTempPath, unrelatedCachePath}, unrelatedTempPaths...) {
		if err := os.WriteFile(path, []byte("cache"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", path, err)
		}
	}

	svc.snapshotter = stubSnapshotter{}
	components := withoutComponent(fullAgentRemovalComponents, model.ComponentSDD)
	result, err := svc.PartialUninstall([]model.AgentID{model.AgentOpenCode}, components)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{modelVariantsPluginPath, skillRegistryPluginPath} {
		if !slices.Contains(result.RemovedFiles, path) {
			t.Fatalf("removed files missing %s", path)
		}
	}

	for _, path := range []string{modelVariantsPluginPath, skillRegistryPluginPath, modelVariantsCachePath, modelVariantsTempPath, modelVariantsRandomTempPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("managed file %q should be removed; stat err = %v", path, err)
		}
	}
	if _, err := os.Stat(cacheDir); err != nil {
		t.Fatalf("cache directory should be preserved, stat err = %v", err)
	}
	if _, err := os.Stat(unrelatedCachePath); err != nil {
		t.Fatalf("unrelated cache file should be preserved, stat err = %v", err)
	}
	for _, path := range unrelatedTempPaths {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("unrelated model variants temp-like file should be preserved, stat err = %v", err)
		}
	}
	for _, path := range []string{thirdPartyPluginPath, backgroundAgentsPath, modifiedPluginPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("unverified plugin should be preserved: %s: %v", path, err)
		}
	}
}

func TestOpenCodePluginPreservationOutsideFullAgentRemoval(t *testing.T) {
	for _, components := range [][]model.ComponentID{{model.ComponentPersona}, {model.ComponentSDD}} {
		t.Run(string(components[0]), func(t *testing.T) {
			home := t.TempDir()
			plugin := filepath.Join(opencode.NewAdapter().GlobalConfigDir(home), "plugins", "model-variants.ts")
			body, err := assets.Read("opencode/plugins/model-variants.ts")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(plugin), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(plugin, []byte(body), 0644); err != nil {
				t.Fatal(err)
			}
			svc, err := NewService(home, t.TempDir(), "dev")
			if err != nil {
				t.Fatal(err)
			}
			svc.snapshotter = stubSnapshotter{}
			if components[0] == model.ComponentSDD {
				if _, err := svc.PartialUninstall([]model.AgentID{model.AgentOpenCode}, components); err == nil || !strings.Contains(err.Error(), "retired") {
					t.Fatalf("SDD-only request must be rejected: %v", err)
				}
			} else if _, err := svc.PartialUninstall([]model.AgentID{model.AgentOpenCode}, components); err != nil {
				t.Fatal(err)
			}
			if got, err := os.ReadFile(plugin); err != nil || string(got) != body {
				t.Fatalf("retained plugin changed: %v", err)
			}
		})
	}
}

func TestComponentOperationsEngram_ProjectScopeRemovesWorkspaceDataOnly(t *testing.T) {
	homeDir := t.TempDir()
	workspaceDir := t.TempDir()

	svc, err := NewService(homeDir, workspaceDir, "dev")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	adapter, ok := svc.registry.Get(model.AgentOpenCode)
	if !ok {
		t.Fatal("openCode adapter not found in registry")
	}

	settingsPath := adapter.SettingsPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(settings dir) error = %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte(`{"mcp":{"engram":{"command":["engram"]}}}`), 0o644); err != nil {
		t.Fatalf("WriteFile(settings) error = %v", err)
	}

	projectDataDir := filepath.Join(workspaceDir, ".engram")
	if err := os.MkdirAll(projectDataDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(projectDataDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDataDir, "memory.db"), []byte("db"), 0o644); err != nil {
		t.Fatalf("WriteFile(memory.db) error = %v", err)
	}

	svc.SetEngramUninstallScope(model.EngramUninstallScopeProject)

	ops, _, err := svc.componentOperations(adapter, model.ComponentEngram)
	if err != nil {
		t.Fatalf("componentOperations() error = %v", err)
	}

	for _, op := range ops {
		if _, _, err := op.apply(op.path); err != nil {
			t.Fatalf("op.apply(%q) error = %v", op.path, err)
		}
	}

	if _, err := os.Stat(projectDataDir); !os.IsNotExist(err) {
		t.Fatalf("project .engram dir should be removed; err = %v", err)
	}

	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(settings) error = %v", err)
	}
	if !strings.Contains(string(raw), `"engram"`) {
		t.Fatalf("global engram config should be preserved in project scope, got: %s", string(raw))
	}
}

func TestComponentOperationsEngram_GlobalScopeKeepsWorkspaceProjectData(t *testing.T) {
	homeDir := t.TempDir()
	workspaceDir := t.TempDir()

	svc, err := NewService(homeDir, workspaceDir, "dev")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	adapter, ok := svc.registry.Get(model.AgentOpenCode)
	if !ok {
		t.Fatal("openCode adapter not found in registry")
	}

	settingsPath := adapter.SettingsPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(settings dir) error = %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte(`{"mcp":{"engram":{"command":["engram"]}}}`), 0o644); err != nil {
		t.Fatalf("WriteFile(settings) error = %v", err)
	}

	projectDataDir := filepath.Join(workspaceDir, ".engram")
	if err := os.MkdirAll(projectDataDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(projectDataDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDataDir, "memory.db"), []byte("db"), 0o644); err != nil {
		t.Fatalf("WriteFile(memory.db) error = %v", err)
	}

	svc.SetEngramUninstallScope(model.EngramUninstallScopeGlobal)

	ops, _, err := svc.componentOperations(adapter, model.ComponentEngram)
	if err != nil {
		t.Fatalf("componentOperations() error = %v", err)
	}

	for _, op := range ops {
		if _, _, err := op.apply(op.path); err != nil {
			t.Fatalf("op.apply(%q) error = %v", op.path, err)
		}
	}

	if _, err := os.Stat(projectDataDir); err != nil {
		t.Fatalf("project .engram dir should be preserved in global scope, err = %v", err)
	}

	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		if !os.IsNotExist(err) {
			t.Fatalf("ReadFile(settings) error = %v", err)
		}
		return
	}
	if strings.Contains(string(raw), `"engram"`) {
		t.Fatalf("global engram config should be removed in global scope, got: %s", string(raw))
	}
}

// TestComponentOperationsEngram_CodexRemovesConsolidatedProtocolAssetsWithNoOrphans
// is the task 2.9 regression assertion: the canonical-asset consolidation
// (design.md Decision 3) renamed/removed the SOURCE assets
// (internal/assets/claude/engram-protocol.md, codex/engram-instructions.md,
// codex/engram-compact-prompt.md -> internal/assets/engram/protocol.md), but
// the WRITTEN on-disk paths for Codex (~/.codex/engram-instructions.md,
// ~/.codex/engram-compact-prompt.md) MUST stay byte-identical so the
// uninstaller keeps covering them with no orphaned files left behind.
func TestComponentOperationsEngram_CodexRemovesConsolidatedProtocolAssetsWithNoOrphans(t *testing.T) {
	restore := codex.SetRuntimeVersionCommandForTest("codex-cli 0.144.0", nil)
	t.Cleanup(restore)
	homeDir := t.TempDir()
	workspaceDir := t.TempDir()

	svc, err := NewService(homeDir, workspaceDir, "dev")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	adapter, ok := svc.registry.Get(model.AgentCodex)
	if !ok {
		t.Fatal("codex adapter not found in registry")
	}

	// Actually write the files via the real (post-consolidation) engram
	// injector, instead of hand-crafting fixtures, so this test fails if the
	// renderer ever drifts from the on-disk paths the uninstaller expects.
	if _, err := engram.InjectWithOptions(homeDir, adapter, engram.InjectOptions{}); err != nil {
		t.Fatalf("engram.InjectWithOptions(codex) error = %v", err)
	}

	instructionsPath := filepath.Join(homeDir, ".codex", "engram-instructions.md")
	compactPath := filepath.Join(homeDir, ".codex", "engram-compact-prompt.md")
	for _, path := range []string{instructionsPath, compactPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected engram injection to create %q: %v", path, err)
		}
	}

	svc.SetEngramUninstallScope(model.EngramUninstallScopeGlobal)

	ops, targets, err := svc.componentOperations(adapter, model.ComponentEngram)
	if err != nil {
		t.Fatalf("componentOperations() error = %v", err)
	}

	for _, want := range []string{instructionsPath, compactPath} {
		if !slices.Contains(targets, want) {
			t.Fatalf("componentOperations() targets missing %q; got: %v", want, targets)
		}
	}

	for _, op := range ops {
		if _, _, err := op.apply(op.path); err != nil {
			t.Fatalf("op.apply(%q) error = %v", op.path, err)
		}
	}

	for _, path := range []string{instructionsPath, compactPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected %q to be removed by uninstall, err = %v", path, err)
		}
	}

	// No orphaned directory left behind either.
	if entries, err := os.ReadDir(filepath.Join(homeDir, ".codex")); err == nil {
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "engram-") {
				t.Fatalf("orphaned engram asset left behind after uninstall: %s", entry.Name())
			}
		}
	}
}

func TestFullAgentClaudeRemovesSkillRegistryHook(t *testing.T) {
	homeDir := t.TempDir()
	workspaceDir := t.TempDir()

	svc, err := NewService(homeDir, workspaceDir, "dev")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	adapter, ok := svc.registry.Get(model.AgentClaudeCode)
	if !ok {
		t.Fatal("claude adapter not found in registry")
	}
	settingsPath := adapter.SettingsPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	initial := `{
  "hooks": {
    "UserPromptSubmit": [
      {
        "matcher": "",
        "hooks": [
          {"type": "command", "command": "gentle-ai skill-registry refresh --quiet --no-gitignore --cwd \"${CLAUDE_PROJECT_DIR:-$PWD}\" || true"},
          {"type": "command", "command": "echo keep"}
        ]
      }
    ],
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [{"type": "command", "command": "echo pre"}]
      }
    ],
    "SubagentStop": [
      {
        "hooks": [
          {"type": "command", "command": "gentle-ai telemetry runtime codex --json", "async": true},
          {"type": "command", "command": "echo subagent keep"}
        ]
      }
    ],
    "Stop": [
      {
        "hooks": [
          {"type": "command", "command": "gentle-ai telemetry runtime codex --json", "async": true},
          {"type": "command", "command": "echo stop keep"}
        ]
      }
    ]
  }
}`
	if err := os.WriteFile(settingsPath, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	built, err := svc.buildPlan([]model.AgentID{adapter.Agent()}, withoutComponent(fullAgentRemovalComponents, model.ComponentSDD))
	ops := built.operations
	if err != nil {
		t.Fatalf("componentOperations() error = %v", err)
	}
	for _, op := range ops {
		if op.typeID == opRewriteFile && op.path == settingsPath {
			if _, _, err := op.apply(op.path); err != nil {
				t.Fatalf("settings rewrite op.apply() error = %v", err)
			}
		}
	}
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Contains(text, "gentle-ai skill-registry refresh") || strings.Contains(text, "gentle-ai telemetry runtime codex") {
		t.Fatalf("managed hook should be removed:\n%s", text)
	}
	if !strings.Contains(text, "echo keep") || !strings.Contains(text, "echo pre") || !strings.Contains(text, "echo subagent keep") || !strings.Contains(text, "echo stop keep") {
		t.Fatalf("unrelated hooks should be preserved:\n%s", text)
	}
}

func TestFullAgentClaudeRemovesReviewAndPreflightHooks(t *testing.T) {
	homeDir := t.TempDir()
	workspaceDir := t.TempDir()

	svc, err := NewService(homeDir, workspaceDir, "dev")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	adapter, ok := svc.registry.Get(model.AgentClaudeCode)
	if !ok {
		t.Fatal("claude adapter not found in registry")
	}
	settingsPath := adapter.SettingsPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	initial := `{
  "hooks": {
    "Stop": [
      {
        "matcher": "",
        "hooks": [
          {"type": "command", "command": "gentle-ai review stop-hook --agent claude-code", "timeout": 60},
          {"type": "command", "command": "echo keep"}
        ]
      }
    ],
    "SessionStart": [
      {
        "matcher": "startup|resume|clear|compact",
        "hooks": [
          {"type": "command", "command": "gentle-ai review stop-hook --agent claude-code", "timeout": 30},
          {"type": "command", "command": "echo custom session-start"}
        ]
      }
    ],
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [{"type": "command", "command": "echo pre"}]
      },
      {
        "matcher": "Agent",
        "hooks": [{"type": "command", "command": "gentle-ai sdd-preflight-hook --agent claude-code"}]
      }
    ],
    "PostToolUse": [
      {
        "matcher": "AskUserQuestion",
        "hooks": [
          {"type": "command", "command": "gentle-ai sdd-preflight-hook --agent claude-code"},
          {"type": "command", "command": "echo post keep"}
        ]
      }
    ],
    "SessionEnd": [
      {
        "matcher": "",
        "hooks": [{"type": "command", "command": "gentle-ai sdd-preflight-hook --agent claude-code"}]
      }
    ]
  }
}`
	if err := os.WriteFile(settingsPath, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	built, err := svc.buildPlan([]model.AgentID{adapter.Agent()}, withoutComponent(fullAgentRemovalComponents, model.ComponentSDD))
	ops := built.operations
	if err != nil {
		t.Fatalf("componentOperations() error = %v", err)
	}
	for _, op := range ops {
		if op.typeID == opRewriteFile && op.path == settingsPath {
			if _, _, err := op.apply(op.path); err != nil {
				t.Fatalf("settings rewrite op.apply() error = %v", err)
			}
		}
	}
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Contains(text, "gentle-ai review stop-hook") || strings.Count(text, "gentle-ai sdd-preflight-hook") != 3 {
		t.Fatalf("review hooks should be removed; unmarked legacy hooks preserved:\n%s", text)
	}
	if !strings.Contains(text, "echo keep") || !strings.Contains(text, "echo pre") || !strings.Contains(text, "echo post keep") || !strings.Contains(text, "echo custom session-start") {
		t.Fatalf("unrelated hooks should be preserved:\n%s", text)
	}
}

func TestFullAgentClaudeRemovesTelemetryHooks(t *testing.T) {
	homeDir := t.TempDir()
	svc, err := NewService(homeDir, t.TempDir(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	adapter, _ := svc.registry.Get(model.AgentClaudeCode)
	settingsPath := adapter.SettingsPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	initial := `{"hooks":{"Stop":[{"matcher":"","hooks":[{"type":"command","command":"gentle-ai telemetry runtime claude --json","async":true},{"type":"command","command":"echo keep"}]}],"SubagentStop":[{"matcher":"","hooks":[{"type":"command","command":"gentle-ai telemetry runtime claude --json","async":true}]}]}}`
	if err := os.WriteFile(settingsPath, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}
	built, err := svc.buildPlan([]model.AgentID{adapter.Agent()}, withoutComponent(fullAgentRemovalComponents, model.ComponentSDD))
	ops := built.operations
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range ops {
		if op.typeID == opRewriteFile && op.path == settingsPath {
			if _, _, err := op.apply(op.path); err != nil {
				t.Fatal(err)
			}
		}
	}
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "telemetry runtime claude") || !strings.Contains(string(raw), "echo keep") {
		t.Fatalf("managed telemetry hook removal failed:\n%s", raw)
	}
}

func TestFullAgentCodexRemovesSkillRegistryHook(t *testing.T) {
	homeDir := t.TempDir()
	workspaceDir := t.TempDir()

	svc, err := NewService(homeDir, workspaceDir, "dev")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	adapter, ok := svc.registry.Get(model.AgentCodex)
	if !ok {
		t.Fatal("codex adapter not found in registry")
	}
	hooksPath := filepath.Join(adapter.GlobalConfigDir(homeDir), "hooks.json")
	if err := os.MkdirAll(filepath.Dir(hooksPath), 0o755); err != nil {
		t.Fatal(err)
	}
	initial := `{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup|resume|clear|compact",
        "hooks": [
          {"type": "command", "command": "gentle-ai skill-registry refresh --quiet --no-gitignore --cwd \"$PWD\" || true"},
          {"type": "command", "command": "echo keep"}
        ]
      }
    ],
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [{"type": "command", "command": "echo pre"}]
      }
    ]
  }
}`
	if err := os.WriteFile(hooksPath, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	built, err := svc.buildPlan([]model.AgentID{adapter.Agent()}, withoutComponent(fullAgentRemovalComponents, model.ComponentSDD))
	ops := built.operations
	if err != nil {
		t.Fatalf("componentOperations() error = %v", err)
	}
	for _, op := range ops {
		if op.typeID == opRewriteFile && op.path == hooksPath {
			if _, _, err := op.apply(op.path); err != nil {
				t.Fatalf("Codex hooks rewrite op.apply() error = %v", err)
			}
		}
	}
	raw, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Contains(text, "gentle-ai skill-registry refresh") {
		t.Fatalf("managed hook should be removed:\n%s", text)
	}
	if !strings.Contains(text, "echo keep") || !strings.Contains(text, "echo pre") {
		t.Fatalf("unrelated hooks should be preserved:\n%s", text)
	}
}

func TestRetainedHooksOutsideFullAgentRemoval(t *testing.T) {
	for _, agentID := range []model.AgentID{model.AgentClaudeCode, model.AgentCodex} {
		t.Run(string(agentID), func(t *testing.T) {
			svc, err := NewService(t.TempDir(), t.TempDir(), "dev")
			if err != nil {
				t.Fatal(err)
			}
			adapter, _ := svc.registry.Get(agentID)
			path := adapter.SettingsPath(svc.homeDir)
			if agentID == model.AgentCodex {
				path = filepath.Join(adapter.GlobalConfigDir(svc.homeDir), "hooks.json")
			}
			for _, components := range [][]model.ComponentID{{model.ComponentPersona}} {
				p, err := svc.buildPlan([]model.AgentID{agentID}, components)
				if err != nil {
					t.Fatal(err)
				}
				for _, op := range p.operations {
					if op.path == path && op.typeID == opRewriteFile && agentID == model.AgentCodex {
						t.Fatal("component-only removal must not rewrite retained Codex hooks")
					}
				}
			}
			p, err := svc.buildPlan([]model.AgentID{agentID}, withoutComponent(fullAgentRemovalComponents, model.ComponentSDD))
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Contains(p.backupTargets, path) {
				t.Fatalf("missing rollback backup for %s", path)
			}
			found := false
			for _, op := range p.operations {
				if op.path == path && op.typeID == opRewriteFile {
					found = true
					if !slices.Contains(op.agents, agentID) {
						t.Fatalf("missing failure owner %s", agentID)
					}
				}
			}
			if !found {
				t.Fatalf("missing retained hook cleanup for %s", agentID)
			}
		})
	}
}

// TestFullAgentOpenCodePluginsUnderXDGConfigHome pins #3219 for the retained
// plugin owner: uninstall resolves the same OpenCode config dir as installation.
func TestFullAgentOpenCodePluginsUnderXDGConfigHome(t *testing.T) {
	homeDir := t.TempDir()
	xdg := filepath.Join(homeDir, ".xdg")
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	t.Setenv("XDG_CONFIG_HOME", xdg)

	svc, err := NewService(homeDir, t.TempDir(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	adapter, ok := svc.registry.Get(model.AgentOpenCode)
	if !ok {
		t.Fatal("OpenCode adapter not found")
	}
	pluginDir := filepath.Join(adapter.GlobalConfigDir(homeDir), "plugins")
	name := "model-variants.ts"
	body, err := assets.Read("opencode/plugins/" + name)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(pluginDir, name)
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	plan, err := svc.buildPlan([]model.AgentID{model.AgentOpenCode}, allManagedComponents)
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.executePlan(plan, []model.AgentID{model.AgentOpenCode})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("managed XDG plugin remains: %v", err)
	}
	if !slices.Contains(result.RemovedFiles, path) {
		t.Fatalf("XDG plugin removal not reported: %v", result.RemovedFiles)
	}
	if _, err := os.Stat(filepath.Join(homeDir, ".config", "opencode")); !os.IsNotExist(err) {
		t.Fatalf("uninstall touched default config despite XDG_CONFIG_HOME: %v", err)
	}
}

// TestUpdateStateAfterUninstallReReadsLatestStateAfterLockContention is the
// #1809 preservation proof for updateStateAfterUninstall: the whole
// read-modify-write must run inside the canonical install-state lock, so a
// concurrent writer's RDDMode survives the agent removal. A contended lock
// must fail fast without writing anything.
func TestUpdateStateAfterUninstallReReadsLatestStateAfterLockContention(t *testing.T) {
	home := t.TempDir()
	lockPath, err := statecoord.LockPath(home)
	if err != nil {
		t.Fatalf("install state lock path: %v", err)
	}
	held, err := reviewtransaction.AcquireAuthorityFileLock(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.Write(home, state.InstallState{
		InstalledAgents: []string{string(model.AgentOpenCode), "claude-code"},
		RDDMode:         string(reviewtransaction.RDDModeOn),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := updateStateAfterUninstall(home, []model.AgentID{model.AgentOpenCode}); err == nil || !strings.Contains(err.Error(), "acquire install state lock") {
		t.Fatalf("contended uninstall state update error = %v", err)
	}
	contended, err := state.Read(home)
	if err != nil || len(contended.InstalledAgents) != 2 || contended.RDDMode != string(reviewtransaction.RDDModeOn) {
		t.Fatalf("state after contended run = %#v, err = %v", contended, err)
	}
	if err := held.Release(); err != nil {
		t.Fatal(err)
	}

	removed, err := updateStateAfterUninstall(home, []model.AgentID{model.AgentOpenCode})
	if err != nil {
		t.Fatalf("updateStateAfterUninstall() error = %v", err)
	}
	if len(removed) != 1 || removed[0] != model.AgentOpenCode {
		t.Fatalf("removed = %v, want [opencode]", removed)
	}
	got, err := state.Read(home)
	if err != nil {
		t.Fatalf("re-read state: %v", err)
	}
	if len(got.InstalledAgents) != 1 || got.InstalledAgents[0] != "claude-code" {
		t.Fatalf("InstalledAgents = %v, want [claude-code]", got.InstalledAgents)
	}
	if got.RDDMode != string(reviewtransaction.RDDModeOn) {
		t.Fatalf("concurrent RDDMode clobbered: got %q, want %q", got.RDDMode, reviewtransaction.RDDModeOn)
	}
	if got.BackgroundIntent != "" {
		t.Fatalf("BackgroundIntent = %q, want cleared after opencode removal", got.BackgroundIntent)
	}
}

func TestUninstallSkillsRemovesLegacySharedMarkerAfterUpgrade(t *testing.T) {
	for _, tt := range []struct {
		name          string
		dirMarker     bool
		wantMarker    bool
		wantSharedDir bool
	}{
		{name: "generated regular marker is removed with emptied _shared"},
		{name: "non-regular marker is preserved", dirMarker: true, wantMarker: true, wantSharedDir: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			skillDir := claude.NewAdapter().SkillsDir(home)
			shared, err := skills.SharedReferencePaths(skillDir)
			if err != nil {
				t.Fatal(err)
			}
			for _, path := range shared {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("managed reference"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			marker := skills.LegacySharedMarkerPath(skillDir)
			if tt.dirMarker {
				if err := os.MkdirAll(marker, 0o755); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(marker, []byte("legacy generated marker"), 0o644); err != nil {
				t.Fatal(err)
			}

			svc, err := NewService(home, t.TempDir(), "dev")
			if err != nil {
				t.Fatal(err)
			}
			svc.snapshotter = stubSnapshotter{}
			result, err := svc.PartialUninstall([]model.AgentID{model.AgentClaudeCode}, []model.ComponentID{model.ComponentSkills})
			if err != nil {
				t.Fatal(err)
			}

			if _, err := os.Lstat(marker); (err == nil) != tt.wantMarker {
				t.Fatalf("legacy marker present = %v, want %v (err %v)", err == nil, tt.wantMarker, err)
			}
			if got := slices.Contains(result.RemovedFiles, marker); got == tt.wantMarker {
				t.Fatalf("RemovedFiles contains marker = %v, want %v: %v", got, !tt.wantMarker, result.RemovedFiles)
			}
			if _, err := os.Stat(filepath.Dir(marker)); (err == nil) != tt.wantSharedDir {
				t.Fatalf("_shared present = %v, want %v (err %v)", err == nil, tt.wantSharedDir, err)
			}
		})
	}
}
