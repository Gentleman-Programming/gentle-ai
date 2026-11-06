package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/gga"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/pipeline"
	"github.com/gentleman-programming/gentle-ai/v4/internal/planner"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

func ggaNoticeSharedPlan(t *testing.T, rt *installRuntime) pipeline.StagePlan {
	t.Helper()
	for _, step := range rt.stagePlan().Apply {
		if step.ID() == "component:gga" {
			return pipeline.StagePlan{Apply: []pipeline.Step{step}}
		}
	}
	t.Fatal("real GGA apply step missing")
	return pipeline.StagePlan{}
}

// Run the real shared component, including its output behavior. Discovery and
// persistent PATH writes are stubbed; no external installer may execute.
func TestTUIInstallGGAProviderNotices(t *testing.T) {
	for _, fixture := range []struct{ name, config string }{
		{"missing provider", ""},
		{"dynamic provider", "PROVIDER=$(touch sentinel)\n"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			home := t.TempDir()
			for _, key := range []string{"HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "XDG_RUNTIME_DIR", "PI_CODING_AGENT_DIR", "TEMP", "TMP", "TMPDIR"} {
				t.Setenv(key, home)
			}
			t.Setenv("PATH", t.TempDir())
			path := gga.ConfigPath(home)
			original := strings.ReplaceAll(fixture.config, "sentinel", "'"+filepath.ToSlash(filepath.Join(home, "sentinel"))+"'")
			if original != "" {
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(original), 0600); err != nil {
					t.Fatal(err)
				}
			}
			priorPlan, priorCommand := tuiInstallStagePlan, runCommand
			priorAvailable := ggaAvailableCheck
			priorAdd, priorFirst := addUserPath, ensureUserPathFirst
			t.Cleanup(func() {
				tuiInstallStagePlan, runCommand = priorPlan, priorCommand
				ggaAvailableCheck = priorAvailable
				addUserPath, ensureUserPathFirst = priorAdd, priorFirst
			})
			runCommand = func(string, ...string) error { return fmt.Errorf("unexpected external installer") }
			ggaAvailableCheck = func(system.PlatformProfile) bool { return true }
			addUserPath = func(string) error { return nil }
			ensureUserPathFirst = func(string) error { return nil }
			tuiInstallStagePlan = func(rt *installRuntime) pipeline.StagePlan {
				return ggaNoticeSharedPlan(t, rt)
			}
			selection := model.Selection{Agents: []model.AgentID{model.AgentCursor}, Components: []model.ComponentID{model.ComponentGGA}}
			resolved := planner.ResolvedPlan{Agents: selection.Agents, OrderedComponents: selection.Components}
			for i := 0; i < 2; i++ {
				// Capture stderr without pipe capacity/deadlock risks.
				capture, err := os.Create(filepath.Join(t.TempDir(), "stderr"))
				if err != nil {
					t.Fatal(err)
				}
				priorStderr := os.Stderr
				func() {
					os.Stderr = capture
					defer func() {
						os.Stderr = priorStderr
						capture.Close()
					}()
					result, orchestrator, _ := ExecuteTUIInstallRecordingCodexServiceTier(home, selection, resolved, system.PlatformProfile{}, model.OpenCodeBackgroundAuto, model.PiBackgroundAuto, nil)
					if orchestrator != nil {
						defer orchestrator.Finish()
					}
					if result.Err != nil {
						t.Fatalf("TUI install: %v", result.Err)
					}
					if len(result.ManualActions) != 1 || !strings.Contains(result.ManualActions[0], "GGA config unchanged:") || !strings.Contains(result.ManualActions[0], path) || !strings.Contains(result.ManualActions[0], "rerun gentle-ai sync") {
						t.Errorf("TUI lost/duplicated GGA notice on run %d: %v", i, result.ManualActions)
					}
				}()
				stderr, err := os.ReadFile(capture.Name())
				if err != nil || len(stderr) != 0 {
					t.Errorf("stray TUI stderr: %q (%v)", stderr, err)
				}
				got, err := os.ReadFile(path)
				if original == "" {
					if !os.IsNotExist(err) {
						t.Fatalf("missing config created: %q (%v)", got, err)
					}
				} else if err != nil || string(got) != original {
					t.Fatalf("dynamic config modified: %q (%v)", got, err)
				}
			}
			if _, err := os.Stat(filepath.Join(home, "sentinel")); !os.IsNotExist(err) {
				t.Fatal("dynamic fixture evaluated")
			}
		})
	}
}

type ggaNoticeLaterFailureStep struct{}

func (ggaNoticeLaterFailureStep) ID() string { return "test:later-failure" }
func (ggaNoticeLaterFailureStep) Run() error { return fmt.Errorf("intentional later failure") }

func TestCLIInstallGGAProviderNoticeAtWrapper(t *testing.T) {
	for _, laterFailure := range []bool{false, true} {
		t.Run(fmt.Sprintf("later_failure=%v", laterFailure), func(t *testing.T) {
			home := t.TempDir()
			for _, key := range []string{"HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "XDG_RUNTIME_DIR", "PI_CODING_AGENT_DIR", "TEMP", "TMP", "TMPDIR"} {
				t.Setenv(key, home)
			}
			t.Setenv("PATH", t.TempDir())
			priorPlan, priorCommand, priorHome := installStagePlan, runCommand, osUserHomeDir
			priorAvailable, priorAdd, priorFirst := ggaAvailableCheck, addUserPath, ensureUserPathFirst
			t.Cleanup(func() {
				installStagePlan, runCommand, osUserHomeDir = priorPlan, priorCommand, priorHome
				ggaAvailableCheck, addUserPath, ensureUserPathFirst = priorAvailable, priorAdd, priorFirst
			})
			osUserHomeDir = func() (string, error) { return home, nil }
			ggaAvailableCheck = func(system.PlatformProfile) bool { return true }
			runCommand = func(string, ...string) error { return fmt.Errorf("unexpected external installer") }
			addUserPath = func(string) error { return nil }
			ensureUserPathFirst = func(string) error { return nil }
			installStagePlan = func(rt *installRuntime) pipeline.StagePlan {
				plan := ggaNoticeSharedPlan(t, rt)
				if laterFailure {
					plan.Apply = append(plan.Apply, ggaNoticeLaterFailureStep{})
				}
				return plan
			}
			capture, err := os.Create(filepath.Join(t.TempDir(), "stderr"))
			if err != nil {
				t.Fatal(err)
			}
			priorStderr := os.Stderr
			func() {
				os.Stderr = capture
				defer func() { os.Stderr = priorStderr; capture.Close() }()
				_, err = RunInstall([]string{"--agent", "cursor", "--component", "gga"}, system.DetectionResult{})
			}()
			if laterFailure {
				if err == nil || !strings.Contains(err.Error(), "intentional later failure") {
					t.Fatalf("later failure not exercised: %v", err)
				}
			} else if err != nil && !strings.Contains(err.Error(), "post-apply verification failed") {
				t.Fatalf("pipeline did not reach verification: %v", err)
			}
			// Only GGA ran: missing unrelated Cursor assets may fail verification.
			// Neither verification nor a later failure may hide/duplicate its notice.
			stderr, readErr := os.ReadFile(capture.Name())
			if readErr != nil {
				t.Fatal(readErr)
			}
			if strings.Count(string(stderr), "WARNING: GGA config unchanged:") != 1 || !strings.Contains(string(stderr), gga.ConfigPath(home)) || !strings.Contains(string(stderr), "rerun gentle-ai sync") {
				t.Fatalf("CLI wrapper lost/duplicated GGA notice: %q", stderr)
			}
		})
	}
}

func TestGGAProviderVerificationUsesActualSkip(t *testing.T) {
	for _, agent := range []model.AgentID{model.AgentCursor, model.AgentClaudeCode} {
		t.Run(string(agent), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("APPDATA", filepath.Join(home, "roaming"))
			selection := model.Selection{Agents: []model.AgentID{agent}, Components: []model.ComponentID{model.ComponentGGA}}
			res, err := gga.Inject(home, selection.Agents)
			if err != nil {
				t.Fatal(err)
			}
			wantSkip := agent == model.AgentCursor
			if res.ConfigCreationSkipped != wantSkip {
				t.Fatalf("creation skip = %v, want %v", res.ConfigCreationSkipped, wantSkip)
			}
			state := &runtimeState{}
			state.recordGGAInjection(res)
			input := postApplyVerificationInput{HomeDir: home, Scope: ScopeGlobal, Selection: selection, Resolved: planner.ResolvedPlan{Agents: selection.Agents, OrderedComponents: selection.Components}, State: state}
			report := runPostApplyVerification(input)
			if !report.Ready {
				t.Fatalf("install verification rejected actual injection: %+v", report)
			}
			configChecked := false
			for _, check := range report.Checks {
				if check.ID == "verify:file:"+gga.ConfigPath(home) {
					configChecked = true
				}
			}
			if configChecked == wantSkip {
				t.Fatalf("config verification requirement lost: skip=%v checks=%+v", wantSkip, report.Checks)
			}
			if report := runPostSyncVerificationWithState(home, "", ScopeGlobal, selection, state); !report.Ready {
				t.Fatalf("sync verification rejected actual injection: %+v", report)
			}
			// Snapshot/rollback inventory must retain even intentionally absent config.
			paths := componentPathsWithWorkspaceScoped(home, "", ScopeGlobal, selection, resolveAdapters(selection.Agents), model.ComponentGGA)
			if !containsPath(paths, gga.ConfigPath(home)) || !containsPath(paths, gga.AgentsTemplatePath(home)) {
				t.Fatalf("managed inventory weakened: %v", paths)
			}
			// Missing files without an observed skip must still fail, including
			// an unmapped selection that never actually ran injection.
			if wantSkip {
				input.State = nil
				if report := runPostApplyVerification(input); report.Ready {
					t.Fatal("absence allowed without actual skip")
				}
				if report := runPostSyncVerificationWithState(home, "", ScopeGlobal, selection, nil); report.Ready {
					t.Fatal("sync absence allowed without actual skip")
				}
			}
			// Even an actual skip cannot exempt the required starter template.
			emptyHome := t.TempDir()
			t.Setenv("APPDATA", filepath.Join(emptyHome, "roaming"))
			input.HomeDir, input.State = emptyHome, state
			if report := runPostApplyVerification(input); report.Ready {
				t.Fatal("missing required files accepted")
			}
			if report := runPostSyncVerificationWithState(emptyHome, "", ScopeGlobal, selection, state); report.Ready {
				t.Fatal("sync accepted missing required files")
			}
		})
	}
}

// No installer is invoked: exercise the shared injection result and the exact
// installation output channel without persistent Windows PATH side effects.
func TestGGAProviderInstallNotice(t *testing.T) {
	home := t.TempDir()
	for _, key := range []string{"HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "PI_CODING_AGENT_DIR", "TEMP", "TMP", "TMPDIR"} {
		t.Setenv(key, home)
	}
	for i := 0; i < 2; i++ {
		res, err := gga.Inject(home, []model.AgentID{model.AgentCursor})
		if err != nil {
			t.Fatal(err)
		}
		var stderr bytes.Buffer
		writeGGAProviderNotices(&stderr, res.Notices)
		if strings.Count(stderr.String(), "WARNING: GGA config unchanged:") != 1 || !strings.Contains(stderr.String(), "WARNING: GGA config unchanged: no provider") || !strings.Contains(stderr.String(), gga.ConfigPath(home)) {
			t.Fatalf("installation discarded notice: %q", stderr.String())
		}
	}
}
