package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/backup"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/opencoderuntimeplugins"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/telemetryruntime"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	opencodeactivation "github.com/gentleman-programming/gentle-ai/v4/internal/opencode"
	"github.com/gentleman-programming/gentle-ai/v4/internal/pipeline"
	"github.com/gentleman-programming/gentle-ai/v4/internal/planner"
	"github.com/gentleman-programming/gentle-ai/v4/internal/telemetry"
)

// themeSettingsFixture puts the effective JSONC in the project and a distinct
// user-owned JSON at the adapter's default global path.
func themeSettingsFixture(t *testing.T) (home, workspace, selected, decoy string, original []byte) {
	t.Helper()
	home, workspace = t.TempDir(), t.TempDir()
	setOpenCodeTestHome(t, home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("OPENCODE_CONFIG_DIR", "")
	if err := os.Mkdir(filepath.Join(workspace, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	selected = filepath.Join(workspace, "opencode.jsonc")
	decoy = opencode.NewAdapter().SettingsPath(home)
	original = []byte("// project settings\n{\"theme\":\"original\",\"user\":true}\n")
	if err := os.WriteFile(selected, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(decoy), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(decoy, []byte(`{"theme":"user-owned"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if got := effectiveOpenCodeSettingsPath(home, workspace, ScopeGlobal, opencode.NewAdapter()); got != selected {
		t.Fatalf("effective path = %q, want %q", got, selected)
	}
	return
}

func TestInstallOpenCodeSettingsWritersUseSelectedJSONC(t *testing.T) {
	for _, component := range []model.ComponentID{model.ComponentPersona, model.ComponentPermission, model.ComponentContext7} {
		t.Run(string(component), func(t *testing.T) {
			home, workspace, selected, decoy, before := themeSettingsFixture(t)
			selection := model.Selection{Agents: []model.AgentID{model.AgentOpenCode}, Components: []model.ComponentID{component}, Persona: model.PersonaGentleman}
			resolved := planner.ResolvedPlan{Agents: selection.Agents, OrderedComponents: selection.Components}
			paths, err := backupTargets(home, workspace, ScopeGlobal, selection, resolved)
			if err != nil || !slices.Contains(paths, selected) {
				t.Fatalf("backup targets = %v, %v; selected missing", paths, err)
			}
			step := componentApplyStep{component: component, homeDir: home, workspaceDir: workspace, scope: ScopeGlobal, agents: selection.Agents, selection: selection}
			if err := step.Run(); err != nil {
				t.Fatal(err)
			}
			assertOpenCodeComponentSelectedOnly(t, selected, decoy, before, component)
		})
	}
}

func assertOpenCodeComponentSelectedOnly(t *testing.T, selected, decoy string, before []byte, component model.ComponentID) {
	t.Helper()
	data, err := os.ReadFile(selected)
	if err != nil || bytes.Equal(data, before) || !bytes.Contains(data, []byte("// project settings")) || !bytes.Contains(data, []byte(`"user":true`)) {
		t.Fatalf("%s selected settings = %s, %v", component, data, err)
	}
	data, err = os.ReadFile(decoy)
	if err != nil || string(data) != `{"theme":"user-owned"}` {
		t.Fatalf("%s changed decoy settings: %s, %v", component, data, err)
	}
}

func TestInstallEngramUsesSelectedOpenCodeJSONC(t *testing.T) {
	home, workspace, selected, decoy, before := themeSettingsFixture(t)
	stubEngramLookPath(t, home)
	selection := model.Selection{Agents: []model.AgentID{model.AgentOpenCode}, Components: []model.ComponentID{model.ComponentEngram}}
	targets, err := backupTargets(home, workspace, ScopeGlobal, selection, planner.ResolvedPlan{Agents: selection.Agents, OrderedComponents: selection.Components})
	if err != nil || !slices.Contains(targets, selected) {
		t.Fatalf("engram backup targets = %v, %v", targets, err)
	}
	step := componentApplyStep{component: model.ComponentEngram, homeDir: home, workspaceDir: workspace, scope: ScopeGlobal, agents: selection.Agents, selection: selection}
	if err := step.Run(); err != nil {
		t.Fatal(err)
	}
	assertOpenCodeComponentSelectedOnly(t, selected, decoy, before, model.ComponentEngram)
}

// TestInstallOpenCodeSettingsWritersWorkspaceScope proves a workspace-scoped
// install still writes the one settings document OpenCode loads (the effective
// global authority) and never strands <workspace>/.config/opencode/opencode.json,
// which OpenCode never reads (issue #1825).
func TestInstallOpenCodeSettingsWritersWorkspaceScope(t *testing.T) {
	components := []model.ComponentID{model.ComponentPersona, model.ComponentPermission, model.ComponentContext7, model.ComponentTheme, model.ComponentEngram}
	for _, component := range components {
		for _, projectFile := range []bool{true, false} {
			name := string(component) + "/home-settings"
			if projectFile {
				name = string(component) + "/project-jsonc"
			}
			t.Run(name, func(t *testing.T) {
				home, workspace, selected, decoy, before := themeSettingsFixture(t)
				loaded := selected
				if !projectFile {
					if err := os.Remove(selected); err != nil {
						t.Fatal(err)
					}
					loaded = decoy
					var err error
					if before, err = os.ReadFile(loaded); err != nil {
						t.Fatal(err)
					}
					if got := effectiveOpenCodeSettingsPath(home, workspace, ScopeGlobal, opencode.NewAdapter()); got != loaded {
						t.Fatalf("effective path = %q, want home settings %q", got, loaded)
					}
				}
				if component == model.ComponentEngram {
					stubEngramLookPath(t, home)
				}
				stranded := opencode.NewAdapter().SettingsPath(workspace)
				selection := model.Selection{Agents: []model.AgentID{model.AgentOpenCode}, Components: []model.ComponentID{component}, Persona: model.PersonaGentleman}
				paths, err := backupTargets(home, workspace, ScopeWorkspace, selection, planner.ResolvedPlan{Agents: selection.Agents, OrderedComponents: selection.Components})
				if err != nil || !slices.Contains(paths, loaded) || slices.Contains(paths, stranded) {
					t.Fatalf("workspace backup targets = %v, %v; want %q and not %q", paths, err, loaded, stranded)
				}
				if declared := componentPathsWithWorkspaceScoped(home, workspace, ScopeWorkspace, selection, resolveAdapters(selection.Agents), component); slices.Contains(declared, stranded) {
					t.Fatalf("%s declares stranded workspace settings: %v", component, declared)
				}
				step := componentApplyStep{component: component, homeDir: home, workspaceDir: workspace, scope: ScopeWorkspace, agents: selection.Agents, selection: selection}
				if err := step.Run(); err != nil {
					t.Fatal(err)
				}
				if projectFile {
					assertOpenCodeComponentSelectedOnly(t, loaded, decoy, before, component)
				} else if got, err := os.ReadFile(loaded); err != nil || bytes.Equal(got, before) {
					t.Fatalf("%s home settings = %s, %v; want a write", component, got, err)
				}
				if _, err := os.Stat(stranded); !os.IsNotExist(err) {
					t.Fatalf("%s stranded a settings document OpenCode never loads at %s (stat err = %v)", component, stranded, err)
				}
			})
		}
	}
}

// stubEngramLookPath resolves engram to a path that is never executed, with
// engram setup disabled, so the Engram component only merges its settings.
func stubEngramLookPath(t *testing.T, home string) {
	t.Helper()
	t.Setenv("GENTLE_AI_ENGRAM_SETUP_MODE", "off")
	originalLookPath := cmdLookPath
	cmdLookPath = func(name string) (string, error) {
		if name == "engram" {
			return filepath.Join(home, "test-engram-not-executed"), nil
		}
		return originalLookPath(name)
	}
	t.Cleanup(func() { cmdLookPath = originalLookPath })
}

func TestInstallThemeUsesSelectedOpenCodeJSONC(t *testing.T) {
	home, workspace, selected, decoy, _ := themeSettingsFixture(t)
	selection := model.Selection{Agents: []model.AgentID{model.AgentOpenCode}, Components: []model.ComponentID{model.ComponentTheme}}
	resolved := planner.ResolvedPlan{Agents: selection.Agents, OrderedComponents: selection.Components}
	paths, err := backupTargets(home, workspace, ScopeGlobal, selection, resolved)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(paths, selected) {
		t.Fatalf("install backup omits selected JSONC: %v", paths)
	}
	if slices.Contains(componentPathsWithWorkspaceScoped(home, workspace, ScopeGlobal, selection, resolveAdapters(selection.Agents), model.ComponentTheme), decoy) {
		t.Fatal("theme declares decoy JSON as its write target")
	}
	step := componentApplyStep{component: model.ComponentTheme, homeDir: home, workspaceDir: workspace, scope: ScopeGlobal, agents: selection.Agents}
	if err := step.Run(); err != nil {
		t.Fatal(err)
	}
	assertThemeSelectedOnly(t, selected, decoy)
}

type failAfterOpenCodeSettingsStep struct {
	selected string
	before   []byte
	cause    error
}

func (s failAfterOpenCodeSettingsStep) ID() string { return "test:fail-after-settings" }
func (s failAfterOpenCodeSettingsStep) Run() error {
	data, err := os.ReadFile(s.selected)
	if err != nil || bytes.Equal(data, s.before) {
		return errors.New("selected settings were not written before failure")
	}
	return s.cause
}

func TestInstallOpenCodeSettingsWritersRollbackSelectedJSONC(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode assertions do not apply on Windows")
	}
	for _, component := range []model.ComponentID{model.ComponentPersona, model.ComponentPermission, model.ComponentContext7} {
		t.Run(string(component), func(t *testing.T) {
			home, workspace, selected, decoy, before := themeSettingsFixture(t)
			selection := model.Selection{Agents: []model.AgentID{model.AgentOpenCode}, Components: []model.ComponentID{component}, Persona: model.PersonaGentleman}
			targets, err := backupTargets(home, workspace, ScopeGlobal, selection, planner.ResolvedPlan{Agents: selection.Agents, OrderedComponents: selection.Components})
			if err != nil || !slices.Contains(targets, selected) {
				t.Fatalf("snapshot targets = %v, %v", targets, err)
			}
			state := &runtimeState{}
			cause := errors.New("injected failure after settings write")
			plan := pipeline.StagePlan{
				Prepare: []pipeline.Step{prepareBackupStep{id: "prepare:backup-snapshot", snapshotter: backup.NewSnapshotter(), snapshotDir: filepath.Join(home, "backup"), targets: []string{selected}, state: state}},
				Apply: []pipeline.Step{
					rollbackRestoreStep{id: "apply:rollback-restore", state: state, homeDir: home, workspaceDir: workspace},
					componentApplyStep{component: component, homeDir: home, workspaceDir: workspace, scope: ScopeGlobal, agents: selection.Agents, selection: selection},
					failAfterOpenCodeSettingsStep{selected: selected, before: before, cause: cause},
				},
			}
			result := pipeline.NewOrchestrator(pipeline.DefaultRollbackPolicy()).Execute(plan)
			if !errors.Is(result.Err, cause) || !result.Rollback.Success {
				t.Fatalf("failure = %v, rollback = %#v", result.Err, result.Rollback)
			}
			got, err := os.ReadFile(selected)
			if err != nil || !bytes.Equal(got, before) {
				t.Fatalf("restored bytes = %s, %v", got, err)
			}
			info, err := os.Stat(selected)
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatalf("restored mode = %v, %v", info, err)
			}
			got, err = os.ReadFile(decoy)
			if err != nil || string(got) != `{"theme":"user-owned"}` {
				t.Fatalf("decoy = %s, %v", got, err)
			}
		})
	}
}

type failAfterThemeStep struct {
	selected string
	observed *bool
	cause    error
}

func (s failAfterThemeStep) ID() string { return "test:fail-after-theme" }
func (s failAfterThemeStep) Run() error {
	data, err := os.ReadFile(s.selected)
	if err != nil || !bytes.Contains(data, []byte(`"theme":"gentleman"`)) {
		return errors.New("theme write was not observed before failure")
	}
	*s.observed = true
	return s.cause
}

func TestInstallThemeRollbackRestoresSelectedJSONCBytesAndMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not preserve POSIX file modes")
	}
	home, workspace, selected, decoy, before := themeSettingsFixture(t)
	selection := model.Selection{Agents: []model.AgentID{model.AgentOpenCode}, Components: []model.ComponentID{model.ComponentTheme}}
	resolved := planner.ResolvedPlan{Agents: selection.Agents, OrderedComponents: selection.Components}
	targets, err := backupTargets(home, workspace, ScopeGlobal, selection, resolved)
	if err != nil || !slices.Contains(targets, selected) {
		t.Fatalf("theme backup targets = %v, %v", targets, err)
	}
	decoyBefore, err := os.ReadFile(decoy)
	if err != nil {
		t.Fatal(err)
	}
	state := &runtimeState{}
	cause := errors.New("forced failure after theme write")
	observed := false
	plan := pipeline.StagePlan{
		Prepare: []pipeline.Step{prepareBackupStep{
			id: "prepare:backup-snapshot", snapshotter: backup.NewSnapshotter(),
			snapshotDir: filepath.Join(home, "theme-backup"), targets: []string{selected}, state: state,
		}},
		Apply: []pipeline.Step{
			rollbackRestoreStep{id: "apply:rollback-restore", state: state, homeDir: home, workspaceDir: workspace},
			componentApplyStep{component: model.ComponentTheme, homeDir: home, workspaceDir: workspace, scope: ScopeGlobal, agents: selection.Agents},
			failAfterThemeStep{selected: selected, observed: &observed, cause: cause},
		},
	}
	result := pipeline.NewOrchestrator(pipeline.DefaultRollbackPolicy()).Execute(plan)
	if !errors.Is(result.Err, cause) || !observed || !result.Rollback.Success {
		t.Fatalf("post-theme failure = %v, observed=%t, rollback=%#v", result.Err, observed, result.Rollback)
	}
	if len(result.Apply.Steps) != 3 || result.Apply.Steps[1].Status != pipeline.StepStatusSucceeded || result.Apply.Steps[2].Status != pipeline.StepStatusFailed {
		t.Fatalf("theme must succeed before injected failure: %#v", result.Apply.Steps)
	}
	got, err := os.ReadFile(selected)
	if err != nil || !bytes.Equal(got, before) {
		t.Fatalf("rollback JSONC bytes = %q, %v; want %q", got, err, before)
	}
	info, err := os.Stat(selected)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("rollback JSONC mode = %v, %v; want 0600", info, err)
	}
	got, err = os.ReadFile(decoy)
	if err != nil || !bytes.Equal(got, decoyBefore) {
		t.Fatalf("decoy JSON changed: %q, %v", got, err)
	}
}

func assertThemeSelectedOnly(t *testing.T, selected, decoy string) {
	t.Helper()
	data, err := os.ReadFile(selected)
	if err != nil || !bytes.Contains(data, []byte(`"theme":"gentleman"`)) || !bytes.Contains(data, []byte(`"user":true`)) || !bytes.Contains(data, []byte("// project settings")) {
		t.Fatalf("selected JSONC theme not installed: %s, %v", data, err)
	}
	data, err = os.ReadFile(decoy)
	if err != nil || string(data) != `{"theme":"user-owned"}` {
		t.Fatalf("decoy JSON changed: %s, %v", data, err)
	}
}

func TestOpenCodeTelemetryRollbackPreservesLateEdits(t *testing.T) {
	for _, flow := range []string{"install", "sync"} {
		for _, existing := range []bool{false, true} {
			for _, edit := range []string{"plugin", "manifest", "both", "mode", "symlink", "parent-symlink"} {
				t.Run(fmtRollbackCase(flow, existing, edit), func(t *testing.T) {
					home := t.TempDir()
					setOpenCodeTestHome(t, home)
					t.Setenv("XDG_CONFIG_HOME", t.TempDir())
					if edit == "mode" && runtime.GOOS == "windows" {
						t.Skip("Windows does not preserve POSIX chmod mode mutations")
					}
					adapter := opencode.NewAdapter()
					config := adapter.GlobalConfigDir(home)
					paths := telemetryruntime.ManagedPaths(config)
					before := make([][]byte, 2)
					if existing {
						if _, err := telemetryruntime.Reconcile(config); err != nil {
							t.Fatal(err)
						}
						// A prior valid compact manifest requires a real metadata refresh.
						raw, _ := os.ReadFile(paths[1])
						var compact bytes.Buffer
						if err := json.Compact(&compact, raw); err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(paths[1], compact.Bytes(), 0600); err != nil {
							t.Fatal(err)
						}
						for i, path := range paths {
							before[i], _ = os.ReadFile(path)
						}
					}
					unrelated := adapter.SystemPromptFile(home)
					if err := os.MkdirAll(filepath.Dir(unrelated), 0700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(unrelated, []byte("before"), 0600); err != nil {
						t.Fatal(err)
					}
					selection := model.Selection{Agents: []model.AgentID{model.AgentOpenCode}}
					var plan pipeline.StagePlan
					if flow == "install" {
						rt := &installRuntime{homeDir: home, workspaceDir: t.TempDir(), backupRoot: filepath.Join(home, "backups"), scope: ScopeGlobal, selection: selection, resolved: planner.ResolvedPlan{Agents: selection.Agents}, state: &runtimeState{}}
						plan = rt.stagePlan()
					} else {
						rt, err := newSyncRuntimeWithScope(home, selection, ScopeGlobal)
						if err != nil {
							t.Fatal(err)
						}
						plan = rt.stagePlan()
					}
					for _, step := range plan.Prepare {
						if snapshot, ok := step.(prepareBackupStep); ok {
							snapshot.targets = append(snapshot.targets, unrelated)
							if err := snapshot.Run(); err != nil {
								t.Fatal(err)
							}
						}
					}
					for _, step := range plan.Apply {
						if strings.HasSuffix(step.ID(), "opencode:telemetry-runtime") {
							if err := step.Run(); err != nil {
								t.Fatal(err)
							}
						}
					}
					if err := os.WriteFile(unrelated, []byte("pipeline write"), 0600); err != nil {
						t.Fatal(err)
					}
					edited := map[int]bool{}
					if edit == "plugin" || edit == "both" {
						edited[0] = true
					}
					if edit == "manifest" || edit == "both" {
						edited[1] = true
					}
					for i := range edited {
						if err := os.WriteFile(paths[i], []byte("custom late edit"), 0600); err != nil {
							t.Fatal(err)
						}
					}
					if edit == "mode" {
						edited[0] = true
						if err := os.Chmod(paths[0], 0600); err != nil {
							t.Fatal(err)
						}
					}
					if edit == "symlink" || edit == "parent-symlink" {
						edited[0] = true
						source := paths[0]
						if edit == "parent-symlink" {
							source = filepath.Dir(source)
						}
						target := filepath.Join(config, "moved-plugin")
						if err := os.Rename(source, target); err != nil {
							t.Fatal(err)
						}
						if err := os.Symlink(target, source); err != nil {
							t.Skip(err)
						}
					}
					var rollbackErr error
					for _, step := range plan.Apply {
						if restore, ok := step.(rollbackRestoreStep); ok {
							rollbackErr = restore.Rollback()
						}
					}
					if rollbackErr == nil {
						t.Error("late edit was not a rollback conflict")
					}
					for i, path := range paths {
						data, err := os.ReadFile(path)
						if edited[i] {
							if err != nil {
								t.Errorf("edited file removed: %s", path)
								continue
							}
							if edit == "mode" {
								info, _ := os.Lstat(path)
								if info.Mode().Perm() != 0600 {
									t.Error("edited mode overwritten")
								}
							} else if edit == "symlink" || edit == "parent-symlink" {
								link := path
								if edit == "parent-symlink" {
									link = filepath.Dir(path)
								}
								info, _ := os.Lstat(link)
								if info.Mode()&os.ModeSymlink == 0 {
									t.Error("symlink replaced")
								}
							} else if string(data) != "custom late edit" {
								t.Error("edited bytes overwritten")
							}
						} else if existing {
							if err != nil || !bytes.Equal(data, before[i]) {
								t.Error("unaffected pair member not restored")
							}
						} else if !os.IsNotExist(err) {
							t.Error("unaffected new pair member remains")
						}
					}
					if data, _ := os.ReadFile(unrelated); string(data) != "before" {
						t.Error("safe unrelated rollback did not complete")
					}
				})
			}
		}
	}
}
func fmtRollbackCase(flow string, existing bool, edit string) string {
	if existing {
		return flow + "/existing/" + edit
	}
	return flow + "/fresh/" + edit
}

// setOpenCodeTestHome keeps XDG resolution bound to the test home on Windows,
// where os.UserHomeDir reads USERPROFILE rather than HOME.
func setOpenCodeTestHome(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	}
}

func TestInstallV2SDKPreflightBeforeManagedRuntimeWrites(t *testing.T) {
	for _, tc := range []struct {
		name, version string
		installed     bool
		wantError     bool
	}{
		{name: "V2 missing SDK", version: "2.0.18", wantError: true},
		{name: "V2 installed SDK", version: "2.0.18", installed: true},
		{name: "V1 without V2 SDK", version: "1.18.30"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			setOpenCodeTestHome(t, home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
			old := opencodeactivation.VersionRunnerOverride
			t.Cleanup(func() { opencodeactivation.VersionRunnerOverride = old })
			opencodeactivation.VersionRunnerOverride = func(context.Context, opencodeactivation.Command) (opencodeactivation.CommandOutput, error) {
				return opencodeactivation.CommandOutput{Stdout: []byte(tc.version)}, nil
			}
			config := opencode.NewAdapter().GlobalConfigDir(home)
			if tc.installed {
				path := filepath.Join(config, "node_modules", "@opencode", "plugin", "package.json")
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(`{"version":"2.0.4"}`), 0644); err != nil {
					t.Fatal(err)
				}
			}
			custom := filepath.Join(config, "plugins", "custom.ts")
			if err := os.MkdirAll(filepath.Dir(custom), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(custom, []byte("custom plugin"), 0644); err != nil {
				t.Fatal(err)
			}
			agents := []model.AgentID{model.AgentOpenCode}
			rt := &installRuntime{homeDir: home, workspaceDir: t.TempDir(), scope: ScopeGlobal, resolved: planner.ResolvedPlan{Agents: agents}, selection: model.Selection{Agents: agents}, state: &runtimeState{}}
			plan := rt.stagePlan()
			// The read-only settings refusal (#5035) runs first; the SDK preflight
			// still precedes telemetry and every managed runtime write.
			if plan.Prepare[0].ID() != "prepare:opencode-settings-validation" || plan.Prepare[1].ID() != "prepare:opencode-plugin-dependency" || plan.Prepare[2].ID() != "prepare:opencode-telemetry" {
				t.Fatalf("prepare order = %s, %s, %s; want settings validation, SDK preflight, telemetry", plan.Prepare[0].ID(), plan.Prepare[1].ID(), plan.Prepare[2].ID())
			}
			err := plan.Prepare[1].Run()
			if tc.wantError {
				if err == nil || !strings.Contains(err.Error(), "@opencode/plugin@2.0.4") {
					t.Fatalf("missing SDK preflight error = %v", err)
				}
				for _, name := range append([]string{"telemetry-runtime.ts"}, opencoderuntimeplugins.ManagedOpenCodePluginNames()...) {
					if _, err := os.Lstat(filepath.Join(config, "plugins", name)); !os.IsNotExist(err) {
						t.Fatalf("preflight wrote %s: %v", name, err)
					}
				}
				if data, _ := os.ReadFile(custom); string(data) != "custom plugin" {
					t.Fatal("custom plugin modified")
				}
				return
			}
			if err != nil {
				t.Fatalf("preflight refused valid runtime: %v", err)
			}
			for _, step := range plan.Apply {
				if step.ID() == "opencode:telemetry-runtime" || step.ID() == "agent:managed-opencode-plugins:opencode" {
					if err := step.Run(); err != nil {
						t.Fatalf("%s: %v", step.ID(), err)
					}
				}
			}
			assetDir := "opencode/plugins/"
			if tc.installed {
				assetDir = "opencode/plugins-v2/"
			}
			for _, name := range opencoderuntimeplugins.ManagedOpenCodePluginNames() {
				data, err := os.ReadFile(filepath.Join(config, "plugins", name))
				if err != nil || string(data) != assets.MustRead(assetDir+name) {
					t.Fatalf("%s did not use selected asset: %v", name, err)
				}
			}
			if data, _ := os.ReadFile(custom); string(data) != "custom plugin" {
				t.Fatal("custom plugin modified")
			}
		})
	}
}

func TestV2SDKPreflightNamesInstalledPackageManagerContinuation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell execution of the displayed continuation")
	}
	home := filepath.Join(t.TempDir(), "home with ' quote")
	if err := os.MkdirAll(home, 0755); err != nil {
		t.Fatal(err)
	}
	setOpenCodeTestHome(t, home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	old := opencodeactivation.VersionRunnerOverride
	t.Cleanup(func() { opencodeactivation.VersionRunnerOverride = old })
	opencodeactivation.VersionRunnerOverride = func(context.Context, opencodeactivation.Command) (opencodeactivation.CommandOutput, error) {
		return opencodeactivation.CommandOutput{Stdout: []byte("2.0.18")}, nil
	}
	config := opencode.NewAdapter().GlobalConfigDir(home)
	if err := os.MkdirAll(config, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config, "package.json"), []byte(`{"packageManager":"npm@10.8.0"}`), 0644); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	marker := filepath.Join(config, "invoked")
	command := filepath.Join(bin, "npm")
	if err := os.WriteFile(command, []byte("#!/bin/sh\nprintf '%s\\n' \"$PWD|$*\" > \"$SDK_PREFLIGHT_MARKER\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SDK_PREFLIGHT_MARKER", marker)
	err := (openCodePluginDependencyPreflightStep{homeDir: home}).Run()
	if err == nil {
		t.Fatal("missing SDK unexpectedly passed")
	}
	start := strings.Index(err.Error(), "`cd ")
	if start < 0 {
		t.Fatalf("missing runnable continuation: %v", err)
	}
	remainder := err.Error()[start+1:]
	end := strings.IndexByte(remainder, '`')
	if end < 0 {
		t.Fatalf("unterminated command: %v", err)
	}
	output, runErr := exec.Command("sh", "-c", remainder[:end]).CombinedOutput()
	if runErr != nil {
		t.Fatalf("continuation failed: %v: %s", runErr, output)
	}
	got, readErr := os.ReadFile(marker)
	if readErr != nil || string(got) != config+"|install --save --no-audit --no-fund @opencode/plugin@2.0.4\n" {
		t.Fatalf("wrong package manager or directory: %q, %v", got, readErr)
	}
}

func TestV2SDKPreflightPreservesPackageManagerOwnership(t *testing.T) {
	for _, tc := range []struct {
		name, manifest, lockfile, available, want string
	}{
		{"bun metadata", `{"packageManager":"bun@1.2.0"}`, "", "bun", "bun"},
		{"npm lockfile", `{}`, "package-lock.json", "npm", "npm"},
		{"unavailable owner", `{"packageManager":"bun@1.2.0"}`, "", "npm", ""},
		{"unsupported owner", `{"packageManager":"pnpm@9.0.0"}`, "", "npm", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := t.TempDir()
			if err := os.WriteFile(filepath.Join(config, "package.json"), []byte(tc.manifest), 0644); err != nil {
				t.Fatal(err)
			}
			if tc.lockfile != "" {
				if err := os.WriteFile(filepath.Join(config, tc.lockfile), nil, 0644); err != nil {
					t.Fatal(err)
				}
			}
			old := cmdLookPath
			t.Cleanup(func() { cmdLookPath = old })
			cmdLookPath = func(name string) (string, error) {
				if name == tc.available {
					return name, nil
				}
				return "", exec.ErrNotFound
			}
			if got := openCodePluginPackageManager(config); got != tc.want {
				t.Fatalf("package manager = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestOpenCodeV2SDKRefusesAmbiguousLockfiles(t *testing.T) {
	home := t.TempDir()
	setOpenCodeTestHome(t, home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	oldVersion := opencodeactivation.VersionRunnerOverride
	t.Cleanup(func() { opencodeactivation.VersionRunnerOverride = oldVersion })
	opencodeactivation.VersionRunnerOverride = func(context.Context, opencodeactivation.Command) (opencodeactivation.CommandOutput, error) {
		return opencodeactivation.CommandOutput{Stdout: []byte("2.0.18")}, nil
	}
	config := opencode.NewAdapter().GlobalConfigDir(home)
	if err := os.MkdirAll(config, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"bun.lock", "package-lock.json"} {
		if err := os.WriteFile(filepath.Join(config, name), nil, 0644); err != nil {
			t.Fatal(err)
		}
	}
	old := cmdLookPath
	t.Cleanup(func() { cmdLookPath = old })
	cmdLookPath = func(name string) (string, error) { return name, nil }
	if manager := openCodePluginPackageManager(config); manager != "" {
		t.Fatalf("ambiguous ownership chose %q", manager)
	}
	if err := (openCodePluginDependencyPreflightStep{homeDir: home}).Run(); err == nil || !strings.Contains(err.Error(), "bun and npm lockfiles conflict") {
		t.Fatalf("ambiguous ownership refusal = %v", err)
	}
	if err := os.WriteFile(filepath.Join(config, "package.json"), []byte(`{"packageManager":"npm@10.8.0"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if manager := openCodePluginPackageManager(config); manager != "npm" {
		t.Fatalf("explicit package owner was ignored: %q", manager)
	}
}

func TestV2SDKProvisionRequiresMatchingInvocationConsent(t *testing.T) {
	home := t.TempDir()
	setOpenCodeTestHome(t, home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	oldVersion := opencodeactivation.VersionRunnerOverride
	t.Cleanup(func() { opencodeactivation.VersionRunnerOverride = oldVersion })
	opencodeactivation.VersionRunnerOverride = func(context.Context, opencodeactivation.Command) (opencodeactivation.CommandOutput, error) {
		return opencodeactivation.CommandOutput{Stdout: []byte("2.0.18")}, nil
	}
	config := opencode.NewAdapter().GlobalConfigDir(home)
	if err := os.MkdirAll(config, 0755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(config, "invoked")
	stub := "#!/bin/sh\nprintf '%s' \"$PWD|$*\" > \"$PWD/invoked\"\nmkdir -p node_modules/@opencode/plugin\nprintf '{\"version\":\"2.0.4\"}' > node_modules/@opencode/plugin/package.json\n"
	// %* is the raw argument tail; delayed expansion prints it and the
	// separator without cmd.exe re-parsing any metacharacter they contain.
	windowsStub := "@echo off\nset \"ARGS=%*\"\nset \"SEP=|\"\nsetlocal EnableDelayedExpansion\n>invoked echo !CD!!SEP!!ARGS!\nendlocal\n" + strings.TrimPrefix(fakeNPMWindowsMaterialize, "@echo off\n")
	installFakeNPM(t, t.TempDir(), stub, windowsStub)
	proposal, err := OpenCodeSDKInstallProposal(home)
	if err != nil || proposal == nil || proposal.Manager != "npm" || proposal.Dependency != "@opencode/plugin@2.0.4" || proposal.ConfigDir != config {
		t.Fatalf("proposal = %+v, %v", proposal, err)
	}
	for _, tc := range []struct {
		name    string
		consent *OpenCodeSDKConsent
	}{
		{"absent", nil},
		{"different config", &OpenCodeSDKConsent{ConfigDir: t.TempDir(), Manager: "npm", Dependency: proposal.Dependency}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := (openCodePluginDependencyPreflightStep{homeDir: home, consent: tc.consent}).Run(); err == nil {
				t.Fatal("missing or mismatched consent passed")
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("package manager ran without consent: %v", err)
			}
		})
	}
	consent := OpenCodeSDKConsent(*proposal)
	if err := (openCodePluginDependencyPreflightStep{homeDir: home, consent: &consent}).Run(); err != nil {
		t.Fatalf("provision: %v", err)
	}
	got, err := os.ReadFile(marker)
	physicalConfig, pathErr := filepath.EvalSymlinks(config)
	if pathErr != nil {
		t.Fatal(pathErr)
	}
	// echo terminates the Windows record with CRLF, and %CD% keeps the 8.3
	// short form of the temporary directory that $PWD resolves on POSIX.
	cwd, args, _ := strings.Cut(strings.TrimRight(string(got), "\r\n"), "|")
	if physicalCwd, evalErr := filepath.EvalSymlinks(cwd); evalErr == nil {
		cwd = physicalCwd
	}
	if err != nil || cwd != physicalConfig || args != "install --save --no-audit --no-fund --ignore-scripts --workspaces=false --prefix="+physicalConfig+" --registry=https://registry.npmjs.org @opencode/plugin@2.0.4" {
		t.Fatalf("manager operation = %q, %v", got, err)
	}
}

func TestV2SDKFreshOnlyProposalAndRecheck(t *testing.T) {
	for _, name := range []string{"package.json", "bun.lock", "bun.lockb", "package-lock.json", "npm-shrinkwrap.json", ".npmrc", "bunfig.toml"} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			setOpenCodeTestHome(t, home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
			oldVersion := opencodeactivation.VersionRunnerOverride
			t.Cleanup(func() { opencodeactivation.VersionRunnerOverride = oldVersion })
			opencodeactivation.VersionRunnerOverride = func(context.Context, opencodeactivation.Command) (opencodeactivation.CommandOutput, error) {
				return opencodeactivation.CommandOutput{Stdout: []byte("2.0.18")}, nil
			}
			config := opencode.NewAdapter().GlobalConfigDir(home)
			if err := os.MkdirAll(config, 0755); err != nil {
				t.Fatal(err)
			}
			bin := t.TempDir()
			if err := os.WriteFile(filepath.Join(bin, "npm"), []byte("#!/bin/sh\ntouch \"$PWD/invoked\"\n"), 0755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			proposal, err := OpenCodeSDKInstallProposal(home)
			if err != nil || proposal == nil {
				t.Fatalf("fresh proposal = %+v, %v", proposal, err)
			}
			if err := os.WriteFile(filepath.Join(config, name), []byte(`{"dependencies":{"private":"github:owner/repo"},"overrides":{"private":"git+ssh://private/repo"}}`), 0644); err != nil {
				t.Fatal(err)
			}
			if next, err := OpenCodeSDKInstallProposal(home); next != nil || err == nil {
				t.Fatalf("existing state proposal = %+v, %v", next, err)
			}
			if err := (openCodePluginDependencyPreflightStep{homeDir: home, consent: proposal}).Run(); err == nil {
				t.Fatal("stale consent accepted")
			}
			if _, err := os.Lstat(filepath.Join(config, "invoked")); !os.IsNotExist(err) {
				t.Fatalf("manager ran: %v", err)
			}
		})
	}
}

func TestV2SDKFreshMissingConfigCreatedOnlyAfterConsent(t *testing.T) {
	home := t.TempDir()
	setOpenCodeTestHome(t, home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	oldVersion := opencodeactivation.VersionRunnerOverride
	t.Cleanup(func() { opencodeactivation.VersionRunnerOverride = oldVersion })
	opencodeactivation.VersionRunnerOverride = func(context.Context, opencodeactivation.Command) (opencodeactivation.CommandOutput, error) {
		return opencodeactivation.CommandOutput{Stdout: []byte("2.0.18")}, nil
	}
	config := opencode.NewAdapter().GlobalConfigDir(home)
	stub := "#!/bin/sh\nmkdir -p node_modules/@opencode/plugin\nprintf '{\"version\":\"2.0.4\"}' > node_modules/@opencode/plugin/package.json\n"
	installFakeNPM(t, t.TempDir(), stub, fakeNPMWindowsMaterialize)
	proposal, err := OpenCodeSDKInstallProposal(home)
	if err != nil || proposal == nil {
		t.Fatalf("proposal = %+v, %v", proposal, err)
	}
	if _, err := os.Lstat(config); !os.IsNotExist(err) {
		t.Fatalf("proposal created config: %v", err)
	}
	if err := (openCodePluginDependencyPreflightStep{homeDir: home}).Run(); err == nil {
		t.Fatal("missing consent passed")
	}
	if _, err := os.Lstat(config); !os.IsNotExist(err) {
		t.Fatalf("refusal created config: %v", err)
	}
	if err := (openCodePluginDependencyPreflightStep{homeDir: home, consent: proposal}).Run(); err != nil {
		t.Fatalf("fresh provision: %v", err)
	}
	if !openCodeSDKInstalled(config, proposal.Dependency) {
		t.Fatal("matching SDK not installed")
	}
}

func TestV2SDKProvisionRejectsChangedOwnershipAndUnmaterializedPackage(t *testing.T) {
	home := t.TempDir()
	setOpenCodeTestHome(t, home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	oldVersion := opencodeactivation.VersionRunnerOverride
	t.Cleanup(func() { opencodeactivation.VersionRunnerOverride = oldVersion })
	opencodeactivation.VersionRunnerOverride = func(context.Context, opencodeactivation.Command) (opencodeactivation.CommandOutput, error) {
		return opencodeactivation.CommandOutput{Stdout: []byte("2.0.18")}, nil
	}
	config := opencode.NewAdapter().GlobalConfigDir(home)
	if err := os.MkdirAll(config, 0755); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(config, "package.json")
	marker := filepath.Join(config, "invoked")
	installFakeNPM(t, t.TempDir(), "#!/bin/sh\ntouch \"$PWD/invoked\"\n", fakeNPMWindowsTouchInvoked)
	proposal, err := OpenCodeSDKInstallProposal(home)
	if err != nil || proposal == nil {
		t.Fatalf("proposal = %+v, %v", proposal, err)
	}
	if err := os.WriteFile(manifest, []byte(`{"packageManager":"npm@11"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := (openCodePluginDependencyPreflightStep{homeDir: home, consent: proposal}).Run(); err == nil || !strings.Contains(err.Error(), "package.json is present") {
		t.Fatalf("stale consent = %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("manager ran with stale consent: %v", err)
	}
	if err := os.Remove(manifest); err != nil {
		t.Fatal(err)
	}
	proposal, err = OpenCodeSDKInstallProposal(home)
	if err != nil || proposal == nil {
		t.Fatalf("refreshed proposal = %+v, %v", proposal, err)
	}
	if err := (openCodePluginDependencyPreflightStep{homeDir: home, consent: proposal}).Run(); err == nil || !strings.Contains(err.Error(), "without materializing") || !strings.Contains(err.Error(), "not covered by Gentle AI rollback") {
		t.Fatalf("successful manager without SDK passed: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("fake manager did not run: %v", err)
	}
}

func TestV2SDKProposalRefusesUnownedExistingManifest(t *testing.T) {
	home := t.TempDir()
	setOpenCodeTestHome(t, home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	oldVersion := opencodeactivation.VersionRunnerOverride
	t.Cleanup(func() { opencodeactivation.VersionRunnerOverride = oldVersion })
	opencodeactivation.VersionRunnerOverride = func(context.Context, opencodeactivation.Command) (opencodeactivation.CommandOutput, error) {
		return opencodeactivation.CommandOutput{Stdout: []byte("2.0.18")}, nil
	}
	config := opencode.NewAdapter().GlobalConfigDir(home)
	if err := os.MkdirAll(config, 0755); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, content string }{{"unowned", `{}`}, {"invalid", `{`}} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(config, "package.json"), []byte(tc.content), 0644); err != nil {
				t.Fatal(err)
			}
			proposal, err := OpenCodeSDKInstallProposal(home)
			if proposal != nil || err == nil {
				t.Fatalf("unowned manifest proposal = %+v, %v", proposal, err)
			}
		})
	}
}

func TestV2SDKProvisionRefusesProjectManagerConfig(t *testing.T) {
	for _, name := range []string{".npmrc", "bunfig.toml"} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			setOpenCodeTestHome(t, home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
			oldVersion := opencodeactivation.VersionRunnerOverride
			t.Cleanup(func() { opencodeactivation.VersionRunnerOverride = oldVersion })
			opencodeactivation.VersionRunnerOverride = func(context.Context, opencodeactivation.Command) (opencodeactivation.CommandOutput, error) {
				return opencodeactivation.CommandOutput{Stdout: []byte("2.0.18")}, nil
			}
			config := opencode.NewAdapter().GlobalConfigDir(home)
			if err := os.MkdirAll(config, 0755); err != nil {
				t.Fatal(err)
			}
			bin := t.TempDir()
			if err := os.WriteFile(filepath.Join(bin, "npm"), []byte("#!/bin/sh\ntouch \"$PWD/ran\"\n"), 0755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			proposal, err := OpenCodeSDKInstallProposal(home)
			if err != nil || proposal == nil {
				t.Fatalf("proposal = %+v, %v", proposal, err)
			}
			if err := os.Symlink(filepath.Join(t.TempDir(), "secret"), filepath.Join(config, name)); err != nil {
				t.Fatal(err)
			}
			if err := (openCodePluginDependencyPreflightStep{homeDir: home, consent: proposal}).Run(); err == nil || !strings.Contains(err.Error(), "manual") {
				t.Fatalf("project config refusal = %v", err)
			}
			if _, err := os.Lstat(filepath.Join(config, "ran")); !os.IsNotExist(err) {
				t.Fatalf("manager ran with project config: %v", err)
			}
			if err := os.Remove(filepath.Join(config, name)); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(config, name), []byte("private registry configuration"), 0644); err != nil {
				t.Fatal(err)
			}
			if next, err := OpenCodeSDKInstallProposal(home); next != nil || err == nil || !strings.Contains(err.Error(), "manually") {
				t.Fatalf("existing project config proposal = %+v, %v", next, err)
			}
		})
	}
}

func TestV2SDKProvisionTimesOutWithoutReflectingManagerOutput(t *testing.T) {
	home := t.TempDir()
	setOpenCodeTestHome(t, home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	oldVersion := opencodeactivation.VersionRunnerOverride
	t.Cleanup(func() { opencodeactivation.VersionRunnerOverride = oldVersion })
	opencodeactivation.VersionRunnerOverride = func(context.Context, opencodeactivation.Command) (opencodeactivation.CommandOutput, error) {
		return opencodeactivation.CommandOutput{Stdout: []byte("2.0.18")}, nil
	}
	config := opencode.NewAdapter().GlobalConfigDir(home)
	if err := os.MkdirAll(config, 0755); err != nil {
		t.Fatal(err)
	}
	installFakeNPM(t, t.TempDir(), "#!/bin/sh\nprintf 'SECRET_TOKEN_DO_NOT_LOG'; sleep 3\n", fakeNPMWindowsSecretThenHang)
	oldTimeout := openCodeSDKInstallTimeout
	openCodeSDKInstallTimeout = 30 * time.Millisecond
	t.Cleanup(func() { openCodeSDKInstallTimeout = oldTimeout })
	proposal, err := OpenCodeSDKInstallProposal(home)
	if err != nil || proposal == nil {
		t.Fatalf("proposal = %+v, %v", proposal, err)
	}
	start := time.Now()
	err = (openCodePluginDependencyPreflightStep{homeDir: home, consent: proposal}).Run()
	if err == nil || time.Since(start) > 2*time.Second || strings.Contains(err.Error(), "SECRET_TOKEN_DO_NOT_LOG") || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("bounded sanitized timeout = %v, duration=%s", err, time.Since(start))
	}
}

func TestV2SDKProvisionBoundsFailedManagerOutput(t *testing.T) {
	home := t.TempDir()
	setOpenCodeTestHome(t, home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	oldVersion := opencodeactivation.VersionRunnerOverride
	t.Cleanup(func() { opencodeactivation.VersionRunnerOverride = oldVersion })
	opencodeactivation.VersionRunnerOverride = func(context.Context, opencodeactivation.Command) (opencodeactivation.CommandOutput, error) {
		return opencodeactivation.CommandOutput{Stdout: []byte("2.0.18")}, nil
	}
	config := opencode.NewAdapter().GlobalConfigDir(home)
	if err := os.MkdirAll(config, 0755); err != nil {
		t.Fatal(err)
	}
	// 1024 lines of 1023 characters plus CRLF stand in for the 1 MiB flood.
	windowsStub := "@echo off\necho SECRET_TOKEN_DO_NOT_LOG 1>&2\nfor /L %%i in (1,1,1024) do echo " + strings.Repeat("0", 1023) + "\nexit 17\n"
	installFakeNPM(t, t.TempDir(), "#!/bin/sh\nprintf 'SECRET_TOKEN_DO_NOT_LOG' >&2\nhead -c 1048576 /dev/zero\nexit 17\n", windowsStub)
	proposal, err := OpenCodeSDKInstallProposal(home)
	if err != nil || proposal == nil {
		t.Fatalf("proposal = %+v, %v", proposal, err)
	}
	err = (openCodePluginDependencyPreflightStep{homeDir: home, consent: proposal}).Run()
	if err == nil || len(err.Error()) > 1024 || strings.Contains(err.Error(), "SECRET_TOKEN_DO_NOT_LOG") || !strings.Contains(err.Error(), "manually") {
		t.Fatalf("unsafe manager failure = %v", err)
	}
}

func TestV2SDKProvisionRejectsChangedExecutableAndPhysicalConfig(t *testing.T) {
	for _, change := range []string{"executable", "executable in place", "config"} {
		t.Run(change, func(t *testing.T) {
			home := t.TempDir()
			setOpenCodeTestHome(t, home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
			oldVersion := opencodeactivation.VersionRunnerOverride
			t.Cleanup(func() { opencodeactivation.VersionRunnerOverride = oldVersion })
			opencodeactivation.VersionRunnerOverride = func(context.Context, opencodeactivation.Command) (opencodeactivation.CommandOutput, error) {
				return opencodeactivation.CommandOutput{Stdout: []byte("2.0.18")}, nil
			}
			config := opencode.NewAdapter().GlobalConfigDir(home)
			if err := os.MkdirAll(config, 0755); err != nil {
				t.Fatal(err)
			}
			bin := t.TempDir()
			binary := installFakeNPM(t, bin, "#!/bin/sh\ntouch \"$PWD/ran\"\n", fakeNPMWindowsTouchRan)
			proposal, err := OpenCodeSDKInstallProposal(home)
			if err != nil || proposal == nil {
				t.Fatalf("proposal = %+v, %v", proposal, err)
			}
			switch change {
			case "executable":
				// Keep the approved file alive so the byte-identical replacement
				// gets a new inode; Linux filesystems reuse a freed inode at once.
				if err := os.Rename(binary, filepath.Join(t.TempDir(), "npm-approved")); err != nil {
					t.Fatal(err)
				}
				writeFakeNPM(t, bin, "#!/bin/sh\ntouch \"$PWD/ran\"\n", fakeNPMWindowsTouchRan)
			case "executable in place":
				writeFakeNPM(t, bin, "#!/bin/sh\ntouch \"$PWD/ran\"\n# changed\n", fakeNPMWindowsTouchRan+"rem changed\n")
			case "config":
				if err := os.Rename(config, config+"-old"); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(config, 0755); err != nil {
					t.Fatal(err)
				}
			}
			if err := (openCodePluginDependencyPreflightStep{homeDir: home, consent: proposal}).Run(); err == nil {
				t.Fatal("replaced executable or config directory passed consent check")
			}
			if _, err := os.Lstat(filepath.Join(config, "ran")); !os.IsNotExist(err) {
				t.Fatalf("manager ran against changed target: %v", err)
			}
		})
	}
}

func TestV2SDKProvisionUsesAnonymousIsolatedManagerEnvironment(t *testing.T) {
	home := t.TempDir()
	setOpenCodeTestHome(t, home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	t.Setenv("NPM_TOKEN", "LEAKED_TOKEN")
	t.Setenv("NPM_CONFIG_REGISTRY", "https://private.example")
	t.Setenv("BUN_CONFIG_TOKEN", "LEAKED_TOKEN")
	oldVersion := opencodeactivation.VersionRunnerOverride
	t.Cleanup(func() { opencodeactivation.VersionRunnerOverride = oldVersion })
	opencodeactivation.VersionRunnerOverride = func(context.Context, opencodeactivation.Command) (opencodeactivation.CommandOutput, error) {
		return opencodeactivation.CommandOutput{Stdout: []byte("2.0.18")}, nil
	}
	config := opencode.NewAdapter().GlobalConfigDir(home)
	if err := os.MkdirAll(config, 0755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\nprintenv > \"$PWD/probe-env\"\nprintf '%s' \"$*\" > \"$PWD/probe-args\"\nmkdir -p node_modules/@opencode/plugin\nprintf '{\"version\":\"2.0.4\"}' > node_modules/@opencode/plugin/package.json\n"
	windowsScript := "@echo off\nset > probe-env\nset \"ARGS=%*\"\nsetlocal EnableDelayedExpansion\n>probe-args echo !ARGS!\nendlocal\n" + strings.TrimPrefix(fakeNPMWindowsMaterialize, "@echo off\n")
	installFakeNPM(t, t.TempDir(), script, windowsScript)
	proposal, err := OpenCodeSDKInstallProposal(home)
	if err != nil || proposal == nil {
		t.Fatalf("proposal = %+v, %v", proposal, err)
	}
	if err := (openCodePluginDependencyPreflightStep{homeDir: home, consent: proposal}).Run(); err != nil {
		t.Fatalf("anonymous provision: %v", err)
	}
	env, err := os.ReadFile(filepath.Join(config, "probe-env"))
	if err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(filepath.Join(config, "probe-args"))
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"LEAKED_TOKEN", "private.example", "HOME=" + home, "XDG_CONFIG_HOME=" + filepath.Join(home, "xdg"), "NPM_TOKEN="} {
		if strings.Contains(string(env), leak) {
			t.Fatalf("manager environment inherited %q", leak)
		}
	}
	for _, option := range []string{"--ignore-scripts", "--registry=https://registry.npmjs.org"} {
		if !strings.Contains(string(args), option) {
			t.Errorf("manager args missing %q: %s", option, args)
		}
	}
}

func TestV2SDKBunOwnerRefusesAutomaticProvision(t *testing.T) {
	home := t.TempDir()
	setOpenCodeTestHome(t, home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	t.Setenv("BUN_CONFIG_TOKEN", "SECRET_BUN_TOKEN")
	oldVersion := opencodeactivation.VersionRunnerOverride
	t.Cleanup(func() { opencodeactivation.VersionRunnerOverride = oldVersion })
	opencodeactivation.VersionRunnerOverride = func(context.Context, opencodeactivation.Command) (opencodeactivation.CommandOutput, error) {
		return opencodeactivation.CommandOutput{Stdout: []byte("2.0.18")}, nil
	}
	config := opencode.NewAdapter().GlobalConfigDir(home)
	if err := os.MkdirAll(config, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config, "package.json"), []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config, "bun.lock"), []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	script := "#!/bin/sh\ntouch \"$PWD/probe\"\n"
	if err := os.WriteFile(filepath.Join(bin, "bun"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	proposal, err := OpenCodeSDKInstallProposal(home)
	if proposal != nil || err == nil || !strings.Contains(err.Error(), "bun add @opencode/plugin@2.0.4") {
		t.Fatalf("Bun owner must give manual continuation, proposal = %+v, %v", proposal, err)
	}
	if err := (openCodePluginDependencyPreflightStep{homeDir: home}).Run(); err == nil || !strings.Contains(err.Error(), "bun add @opencode/plugin@2.0.4") {
		t.Fatalf("Bun preflight must give manual continuation: %v", err)
	}
	if err := (openCodePluginDependencyPreflightStep{homeDir: home, consent: &OpenCodeSDKConsent{ConfigDir: config, Manager: "bun"}}).Run(); err == nil || !strings.Contains(err.Error(), "bun add @opencode/plugin@2.0.4") {
		t.Fatalf("Bun consent must not permit execution: %v", err)
	}
	if err := openCodeSDKRunApprovedManager(&OpenCodeSDKConsent{Manager: "bun"}); err == nil {
		t.Fatal("direct manager execution accepted Bun")
	}
	if _, err := os.Stat(filepath.Join(config, "probe")); !os.IsNotExist(err) {
		t.Fatalf("Bun manager ran: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(config, "node_modules", "@opencode", "plugin"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config, "node_modules", "@opencode", "plugin", "package.json"), []byte(`{"version":"2.0.4"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if proposal, err := OpenCodeSDKInstallProposal(home); proposal != nil || err != nil {
		t.Fatalf("installed Bun SDK must bypass proposal: %+v, %v", proposal, err)
	}
	if err := (openCodePluginDependencyPreflightStep{homeDir: home}).Run(); err != nil {
		t.Fatalf("installed Bun SDK must bypass install: %v", err)
	}
	if _, err := os.Stat(filepath.Join(config, "probe")); !os.IsNotExist(err) {
		t.Fatalf("Bun manager ran despite installed SDK: %v", err)
	}
}

func TestV2SDKBunMetadataWithoutExecutableStillNamesManualContinuation(t *testing.T) {
	home := t.TempDir()
	setOpenCodeTestHome(t, home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	oldVersion := opencodeactivation.VersionRunnerOverride
	t.Cleanup(func() { opencodeactivation.VersionRunnerOverride = oldVersion })
	opencodeactivation.VersionRunnerOverride = func(context.Context, opencodeactivation.Command) (opencodeactivation.CommandOutput, error) {
		return opencodeactivation.CommandOutput{Stdout: []byte("2.0.18")}, nil
	}
	config := opencode.NewAdapter().GlobalConfigDir(home)
	if err := os.MkdirAll(config, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config, "package.json"), []byte(`{"packageManager":"bun@1.2.0"}`), 0644); err != nil {
		t.Fatal(err)
	}
	oldPath := cmdLookPath
	t.Cleanup(func() { cmdLookPath = oldPath })
	cmdLookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	if proposal, err := OpenCodeSDKInstallProposal(home); proposal != nil || err == nil || !strings.Contains(err.Error(), "bun add @opencode/plugin@2.0.4") {
		t.Fatalf("Bun-owned proposal without executable = %+v, %v", proposal, err)
	}
	if err := (openCodePluginDependencyPreflightStep{homeDir: home}).Run(); err == nil || !strings.Contains(err.Error(), "bun add @opencode/plugin@2.0.4") {
		t.Fatalf("Bun-owned preflight without executable = %v", err)
	}
}

func TestV2SDKProposalRefusesSymlinkConfigDirectory(t *testing.T) {
	home := t.TempDir()
	setOpenCodeTestHome(t, home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	oldVersion := opencodeactivation.VersionRunnerOverride
	t.Cleanup(func() { opencodeactivation.VersionRunnerOverride = oldVersion })
	opencodeactivation.VersionRunnerOverride = func(context.Context, opencodeactivation.Command) (opencodeactivation.CommandOutput, error) {
		return opencodeactivation.CommandOutput{Stdout: []byte("2.0.18")}, nil
	}
	config := opencode.NewAdapter().GlobalConfigDir(home)
	if err := os.MkdirAll(filepath.Dir(config), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), config); err != nil {
		t.Fatal(err)
	}
	if proposal, err := OpenCodeSDKInstallProposal(home); proposal != nil || err == nil {
		t.Fatalf("symlink config proposal = %+v, %v", proposal, err)
	}
}

func TestV2SDKProposalRefusesNonPublicDependencySources(t *testing.T) {
	for _, tc := range []struct{ name, manifest, lockName, lock string }{
		{"manifest URL", `{"packageManager":"npm@10","dependencies":{"other":"https://private.example/pkg.tgz"}}`, "", ""},
		{"lockfile URL", `{"packageManager":"npm@10"}`, "package-lock.json", `{"packages":{"node_modules/other":{"resolved":"https://private.example/other.tgz"}}}`},
		{"escaped lockfile URL", `{"packageManager":"npm@10"}`, "package-lock.json", `{"packages":{"node_modules/other":{"resolved":"https:\/\/private.example\/other.tgz"}}}`},
		{"embedded credentials", `{"packageManager":"npm@10"}`, "package-lock.json", `{"packages":{"node_modules/other":{"resolved":"https://user:secret@registry.npmjs.org/other.tgz"}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			setOpenCodeTestHome(t, home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
			oldVersion := opencodeactivation.VersionRunnerOverride
			t.Cleanup(func() { opencodeactivation.VersionRunnerOverride = oldVersion })
			opencodeactivation.VersionRunnerOverride = func(context.Context, opencodeactivation.Command) (opencodeactivation.CommandOutput, error) {
				return opencodeactivation.CommandOutput{Stdout: []byte("2.0.18")}, nil
			}
			config := opencode.NewAdapter().GlobalConfigDir(home)
			if err := os.MkdirAll(config, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(config, "package.json"), []byte(tc.manifest), 0644); err != nil {
				t.Fatal(err)
			}
			if tc.lockName != "" {
				if err := os.WriteFile(filepath.Join(config, tc.lockName), []byte(tc.lock), 0644); err != nil {
					t.Fatal(err)
				}
			}
			bin := t.TempDir()
			if err := os.WriteFile(filepath.Join(bin, "npm"), []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			proposal, err := OpenCodeSDKInstallProposal(home)
			if proposal != nil || err == nil || !strings.Contains(err.Error(), "manually") {
				t.Fatalf("unsafe dependency source proposal = %+v, %v", proposal, err)
			}
		})
	}
}

func TestOpenCodeV2SDKWindowsPowerShellContinuation(t *testing.T) {
	config := filepath.Join(t.TempDir(), "path with ' quote %SDK_PREFLIGHT_MARKER%")
	if err := os.MkdirAll(config, 0755); err != nil {
		t.Fatal(err)
	}
	command := openCodeSDKInstallContinuation("windows", config, "npm", "@opencode/plugin@2.0.4")
	if strings.Contains(command, "cd /d") || !strings.Contains(command, "Set-Location -LiteralPath") || !strings.Contains(command, "path with '' quote %SDK_PREFLIGHT_MARKER%'") {
		t.Fatalf("unsafe or non-PowerShell continuation: %s", command)
	}
	if _, err := exec.LookPath("pwsh"); err != nil {
		t.Skip("PowerShell is unavailable for executable continuation check")
	}
	if testing.Short() {
		t.Skip("PowerShell execution is an external integration check")
	}
	marker := filepath.Join(t.TempDir(), "invoked")
	t.Setenv("SDK_PREFLIGHT_MARKER", marker)
	definition := "function npm { Set-Content -LiteralPath $env:SDK_PREFLIGHT_MARKER -Value ((Get-Location).Path + '|' + ($args -join ' ')) }\n"
	output, err := exec.Command("pwsh", "-NoProfile", "-Command", definition+command).CombinedOutput()
	if err != nil {
		t.Fatalf("PowerShell continuation failed: %v: %s", err, output)
	}
	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("PowerShell did not write the marker: %v", err)
	}
	// PowerShell reports the long path while t.TempDir() may use an 8.3 short
	// name (C:\Users\RUNNER~1), so compare directory identity, not bytes.
	dir, args, found := strings.Cut(strings.TrimSpace(string(data)), "|")
	if !found || args != "install --save --no-audit --no-fund @opencode/plugin@2.0.4" {
		t.Fatalf("PowerShell ran the wrong command: %q", data)
	}
	want, wantErr := os.Stat(config)
	got, gotErr := os.Stat(dir)
	if wantErr != nil || gotErr != nil || !os.SameFile(want, got) {
		t.Fatalf("PowerShell did not run in target directory: %q, %v, %v", data, wantErr, gotErr)
	}
}

func TestOpenCodeTelemetryInstallRollbackOutsideHome(t *testing.T) {
	home := t.TempDir()
	setOpenCodeTestHome(t, home)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	agents := []model.AgentID{model.AgentOpenCode}
	rt := &installRuntime{homeDir: home, workspaceDir: t.TempDir(), backupRoot: filepath.Join(home, "backups"), scope: ScopeGlobal, selection: model.Selection{Agents: agents}, resolved: planner.ResolvedPlan{Agents: agents}, state: &runtimeState{}}
	plan := rt.stagePlan()
	for _, step := range plan.Prepare {
		if _, ok := step.(prepareBackupStep); ok {
			if err := step.Run(); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, step := range plan.Apply {
		if step.ID() == "opencode:telemetry-runtime" {
			if err := step.Run(); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, step := range plan.Apply {
		if restore, ok := step.(rollbackRestoreStep); ok {
			if err := restore.Rollback(); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, path := range telemetryruntime.ManagedPaths(opencode.NewAdapter().GlobalConfigDir(home)) {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("rollback left new runtime artifact", path, err)
		}
	}
}

func TestOpenCodeTelemetryInstallRefusesCustomBeforeSnapshot(t *testing.T) {
	home := t.TempDir()
	setOpenCodeTestHome(t, home)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	agents := []model.AgentID{model.AgentOpenCode}
	path := telemetryruntime.ManagedPaths(opencode.NewAdapter().GlobalConfigDir(home))[0]
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("custom"), 0600); err != nil {
		t.Fatal(err)
	}
	rt := &installRuntime{homeDir: home, workspaceDir: t.TempDir(), scope: ScopeGlobal, selection: model.Selection{Agents: agents}, resolved: planner.ResolvedPlan{Agents: agents}, state: &runtimeState{}}
	plan := rt.stagePlan()
	for _, step := range plan.Prepare {
		if step.ID() == "prepare:opencode-telemetry" {
			if err := step.Run(); err == nil {
				t.Fatal("custom plugin not refused before snapshot")
			}
			break
		}
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "custom" {
		t.Fatal("custom file changed", err)
	}
}

func TestOpenCodeTelemetryOrdinaryInstall(t *testing.T) {
	for _, selected := range []bool{true, false} {
		t.Run(map[bool]string{true: "selected", false: "absent"}[selected], func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
			t.Setenv("DO_NOT_TRACK", "1")
			if err := telemetry.Save(home, telemetry.State{InstallID: "existing", Enabled: false, NoticeShown: true}); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(telemetry.Path(home))
			agents := []model.AgentID{model.AgentClaudeCode}
			if selected {
				agents = []model.AgentID{model.AgentOpenCode}
			}
			selection := model.Selection{Agents: agents}
			resolved := planner.ResolvedPlan{Agents: agents}
			rt := &installRuntime{homeDir: home, workspaceDir: t.TempDir(), scope: ScopeGlobal, selection: selection, resolved: resolved, state: &runtimeState{}}
			plan := rt.stagePlan()
			// Execute the ordinary plan's runtime-asset step, not an installer or an SDD helper.
			found := false
			for _, step := range plan.Apply {
				if step.ID() == "opencode:telemetry-runtime" {
					found = true
					if err := step.Run(); err != nil {
						t.Fatal(err)
					}
				}
			}
			if found != selected {
				t.Fatalf("runtime step present=%v selected=%v", found, selected)
			}
			path := filepath.Join(opencode.NewAdapter().GlobalConfigDir(home), "plugins", "telemetry-runtime.ts")
			data, err := os.ReadFile(path)
			if selected {
				if err != nil || string(data) != assets.MustRead("opencode/plugins/telemetry-runtime.ts") {
					t.Fatal("missing embedded runtime asset", err)
				}
				paths, err := backupTargets(home, rt.workspaceDir, ScopeGlobal, selection, resolved)
				if err != nil || !slices.Contains(paths, path) || !slices.Contains(paths, filepath.Join(filepath.Dir(filepath.Dir(path)), ".gentle-ai-telemetry-runtime.json")) {
					t.Fatal("runtime pair absent from install backup", err)
				}
			} else if !os.IsNotExist(err) {
				t.Fatal("OpenCode absent but plugin installed")
			}
			after, _ := os.ReadFile(telemetry.Path(home))
			if string(before) != string(after) {
				t.Fatal("installation changed telemetry opt-out")
			}
		})
	}
}
