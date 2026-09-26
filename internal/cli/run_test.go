package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/v3/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/telemetryruntime"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
	"github.com/gentleman-programming/gentle-ai/v3/internal/pipeline"
	"github.com/gentleman-programming/gentle-ai/v3/internal/planner"
	"github.com/gentleman-programming/gentle-ai/v3/internal/system"
	"github.com/gentleman-programming/gentle-ai/v3/internal/telemetry"
)

func TestInstallRejectsInvalidOpenCodeAuthorityBeforeMutation(t *testing.T) {
	for _, sidecar := range []string{"invalid", "symlink"} {
		t.Run(sidecar, func(t *testing.T) {
			home := t.TempDir()
			workspace := t.TempDir()
			t.Chdir(workspace)
			t.Setenv("OPENCODE_CONFIG_DIR", "")
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
			setOpenCodeTestHome(t, home)
			config := filepath.Join(home, ".config", "opencode")
			if err := os.MkdirAll(config, 0700); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"opencode.json", "opencode.jsonc"} {
				if err := os.WriteFile(filepath.Join(config, name), []byte("{}"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			authority := filepath.Join(config, ".gentle-ai-opencode-write-authority.json")
			if sidecar == "symlink" {
				target := filepath.Join(home, "authority-target")
				if err := os.WriteFile(target, []byte("invalid"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, authority); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			} else if err := os.WriteFile(authority, []byte("invalid"), 0600); err != nil {
				t.Fatal(err)
			}
			selection := model.Selection{Agents: []model.AgentID{model.AgentOpenCode}}
			_, err := newInstallRuntime(home, ScopeGlobal, ChannelStable, selection, planner.ResolvedPlan{Agents: selection.Agents}, system.PlatformProfile{})
			if err == nil {
				t.Fatal("expected authority preflight failure")
			}
			for _, path := range []string{filepath.Join(config, "opencode.json"), filepath.Join(config, "opencode.jsonc")} {
				raw, readErr := os.ReadFile(path)
				if readErr != nil || string(raw) != "{}" {
					t.Fatalf("settings %s changed: %q, %v", path, raw, readErr)
				}
			}
			if _, statErr := os.Stat(filepath.Join(home, ".gentle-ai", "backups")); !os.IsNotExist(statErr) {
				t.Fatalf("backup root created: %v", statErr)
			}
		})
	}
}

func TestInstallDryRunWithInvalidOpenCodeAuthorityDoesNotWrite(t *testing.T) {
	home := t.TempDir()
	setOpenCodeTestHome(t, home)
	t.Setenv("OPENCODE_CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	config := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(config, 0700); err != nil {
		t.Fatal(err)
	}
	authority := filepath.Join(config, ".gentle-ai-opencode-write-authority.json")
	if err := os.WriteFile(authority, []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := RunInstall([]string{"--dry-run", "--agent", "opencode"}, system.DetectionResult{})
	if err == nil {
		t.Fatal("expected dry-run to fail with invalid authority")
	}
	if result.DryRun {
		t.Fatalf("dry-run should not succeed with invalid authority")
	}
	if raw, err := os.ReadFile(authority); err != nil || string(raw) != "invalid" {
		t.Fatalf("authority changed: %q, %v", raw, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".gentle-ai", "backups")); !os.IsNotExist(err) {
		t.Fatalf("dry-run created backup root: %v", err)
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
						rt, err := newSyncRuntime(home, selection)
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
	if err := plan.Prepare[0].Run(); err == nil {
		t.Fatal("custom plugin not refused before snapshot")
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
