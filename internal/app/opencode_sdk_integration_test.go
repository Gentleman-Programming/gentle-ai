package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/telemetryruntime"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/opencode"
	"github.com/gentleman-programming/gentle-ai/v4/internal/pipeline"
	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
	"github.com/gentleman-programming/gentle-ai/v4/internal/state"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
	"github.com/gentleman-programming/gentle-ai/v4/internal/tui"
)

// This exercises the real app/CLI bridge, but deliberately fails Prepare after
// SDK provisioning. It does not claim a complete install or plugin activation.
func TestTUIOpenCodeSDKConsentProvisioningIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test executes an isolated fake npm subprocess")
	}
	if runtime.GOOS == "windows" {
		t.Skip("fake npm fixture requires a POSIX shell")
	}
	for _, tc := range []struct {
		name       string
		version    string
		decline    bool
		stale      bool
		wantSDKerr string
	}{
		{name: "default decline", decline: true},
		{name: "affirmative materializes pinned SDK", version: "2.0.4"},
		{name: "manager succeeds without SDK", wantSDKerr: "completed without materializing @opencode/plugin@2.0.4"},
		{name: "manager succeeds with wrong SDK", version: "2.0.3", wantSDKerr: "completed without materializing @opencode/plugin@2.0.4"},
		{name: "dependency state changes after proposal", stale: true, wantSDKerr: "package.json is present"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			home, config, log := isolateSDKBridgeTest(t, root, tc.version)
			oldHome, oldVersion := appUserHomeDir, opencode.VersionRunnerOverride
			t.Cleanup(func() {
				appUserHomeDir = oldHome
				opencode.VersionRunnerOverride = oldVersion
			})
			appUserHomeDir = func() (string, error) { return home, nil }
			opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
				return opencode.CommandOutput{Stdout: []byte("2.0.18")}, nil
			}

			m := tui.NewModel(system.DetectionResult{}, "test")
			m.Selection = model.Selection{Agents: []model.AgentID{model.AgentOpenCode}}
			m.DependencyPlan.Agents = m.Selection.Agents
			m.BackgroundIntent = model.OpenCodeBackgroundOff
			m.PiBackgroundIntent = model.PiBackgroundOff
			m.ExecuteSDKFn = tuiExecuteWithSDK
			m.Screen, m.Cursor = tui.ScreenReview, 0
			updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m = updated.(tui.Model)
			if cmd != nil || m.Screen != tui.ScreenOpenCodeSDKConfirm || m.Cursor != 1 {
				t.Fatalf("Review Enter: screen=%v cursor=%d command=%v err=%v", m.Screen, m.Cursor, cmd != nil, m.Err)
			}
			for _, text := range []string{"@opencode/plugin@2.0.4", "npm", config, "No / Back", "rollback"} {
				if !strings.Contains(m.View(), text) {
					t.Fatalf("SDK confirmation missing %q: %s", text, m.View())
				}
			}
			assertSDKBridgeMissing(t, log)
			assertSDKBridgeMissing(t, filepath.Join(home, ".gentle-ai", "backups"))
			if tc.decline {
				updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
				m = updated.(tui.Model)
				if m.Screen != tui.ScreenReview || cmd != nil {
					t.Fatalf("default No did not return to Review: screen=%v command=%v", m.Screen, cmd != nil)
				}
				assertSDKBridgeMissing(t, log)
				assertSDKBridgeMissing(t, filepath.Join(config, "node_modules"))
				assertSDKBridgeMissing(t, filepath.Join(home, ".gentle-ai", "backups"))
				return
			}
			if tc.stale {
				writeSDKBridgeFile(t, filepath.Join(config, "package.json"), "{\"private\":true}\n", 0600)
			}
			updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyUp})
			m = updated.(tui.Model)
			if cmd != nil || m.Cursor != 0 {
				t.Fatal("Up did not select affirmative SDK confirmation")
			}
			updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m = updated.(tui.Model)
			if m.Screen != tui.ScreenInstalling || cmd == nil {
				t.Fatalf("affirmative did not schedule installation: screen=%v", m.Screen)
			}
			m = drainSDKBridgeCommands(t, m, cmd)
			if m.Execution.Err == nil || m.Execution.Prepare.Success || len(m.Execution.Apply.Steps) != 0 {
				t.Fatalf("expected terminal Prepare failure without Apply: %+v", m.Execution)
			}
			if !strings.Contains(strings.Join(m.Progress.Logs, "\n"), "pipeline completed with errors") {
				t.Fatalf("terminal failure not surfaced to TUI: %v", m.Progress.Logs)
			}
			// Prepare gates stop at the first refusal under every failure policy
			// (#5035): a failed SDK step ends the stage; otherwise the deliberate
			// telemetry ownership conflict does.
			wantIDs := []string{"prepare:opencode-settings-validation", "prepare:opencode-plugin-dependency"}
			if tc.wantSDKerr == "" {
				wantIDs = append(wantIDs, "prepare:opencode-telemetry")
			}
			if len(m.Execution.Prepare.Steps) != len(wantIDs) {
				t.Fatalf("unexpected Prepare steps: %+v", m.Execution.Prepare.Steps)
			}
			for i, step := range m.Execution.Prepare.Steps {
				if step.StepID != wantIDs[i] {
					t.Fatalf("Prepare step %d = %s, want %s", i, step.StepID, wantIDs[i])
				}
				wantErr := ""
				if i == 1 {
					wantErr = tc.wantSDKerr
				} else if i == 2 {
					wantErr = "telemetry runtime ownership conflict"
				}
				if wantErr != "" {
					if step.Status != pipeline.StepStatusFailed || step.Err == nil || !strings.Contains(step.Err.Error(), wantErr) {
						t.Fatalf("%s: want failure %q, got %+v", step.StepID, wantErr, step)
					}
				} else if step.Status != pipeline.StepStatusSucceeded || step.Err != nil {
					t.Fatalf("%s unexpectedly failed: %+v", step.StepID, step)
				}
			}
			if tc.stale {
				assertSDKBridgeMissing(t, log)
				data, err := os.ReadFile(filepath.Join(config, "package.json"))
				if err != nil || string(data) != "{\"private\":true}\n" {
					t.Fatalf("changed dependency state not preserved: %q, %v", data, err)
				}
			} else {
				data, err := os.ReadFile(log)
				if err != nil {
					t.Fatal(err)
				}
				physicalConfig, err := filepath.EvalSymlinks(config)
				if err != nil {
					t.Fatal(err)
				}
				lines := strings.Split(strings.TrimSpace(string(data)), "\n")
				if lines[0] != physicalConfig || strings.Count(string(data), "@opencode/plugin@2.0.4\n") != 1 || strings.Count(string(data), "install\n") != 1 {
					t.Fatalf("fake npm did not receive exactly one pinned install in config: %s", data)
				}
				for _, arg := range []string{"--ignore-scripts", "--registry=https://registry.npmjs.org", "--prefix=" + physicalConfig} {
					if !strings.Contains(string(data), "\n"+arg+"\n") {
						t.Fatalf("missing isolated npm argument %q: %s", arg, data)
					}
				}
			}
			manifest := filepath.Join(config, "node_modules", "@opencode", "plugin", "package.json")
			if tc.version == "" {
				assertSDKBridgeMissing(t, manifest)
			} else if data, err := os.ReadFile(manifest); err != nil || string(data) != fmt.Sprintf("{\"version\":\"%s\"}\n", tc.version) {
				t.Fatalf("materialized SDK = %q, %v", data, err)
			}
			data, err := os.ReadFile(filepath.Join(config, "plugins", "telemetry-runtime.ts"))
			if err != nil || string(data) != "// custom telemetry blocker\n" {
				t.Fatalf("custom telemetry changed: %q, %v", data, err)
			}
			assertSDKBridgeMissing(t, filepath.Join(config, "opencode.json"))
			assertSDKBridgeMissing(t, filepath.Join(config, ".gentle-ai-telemetry-runtime.json"))
		})
	}
}

// The fake manager emits only a version manifest. Successful application and
// persistence here do not prove that a real SDK package is usable by OpenCode.
func TestTUIOpenCodeSDKConsentFullApplyIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test executes an isolated fake npm subprocess")
	}
	if runtime.GOOS == "windows" {
		t.Skip("fake npm fixture requires a POSIX shell")
	}
	home, config, log := isolateSDKBridgeTest(t, t.TempDir(), "2.0.4")
	runSDKFullApplyIntegration(t, home, config, log, false)
}

func runSDKFullApplyIntegration(t *testing.T, home, config, log string, realSDK bool) {
	t.Helper()
	// Remove only the deliberate negative-test ownership conflict, before consent.
	if err := os.Remove(filepath.Join(config, "plugins", "telemetry-runtime.ts")); err != nil {
		t.Fatal(err)
	}
	oldHome, oldVersion := appUserHomeDir, opencode.VersionRunnerOverride
	t.Cleanup(func() {
		appUserHomeDir = oldHome
		opencode.VersionRunnerOverride = oldVersion
	})
	appUserHomeDir = func() (string, error) { return home, nil }
	opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
		return opencode.CommandOutput{Stdout: []byte("2.0.18")}, nil
	}

	m := tui.NewModel(system.DetectionResult{}, "test")
	m.Selection = model.Selection{Agents: []model.AgentID{model.AgentOpenCode}}
	m.DependencyPlan.Agents = m.Selection.Agents
	m.BackgroundIntent = model.OpenCodeBackgroundOff
	m.PiBackgroundIntent = model.PiBackgroundOff
	m.InstallReviewModeChoiceSet = false
	m.ExecuteSDKFn = tuiExecuteWithSDK
	// Fail closed if this flow unexpectedly schedules native review-mode work.
	reviewCalls := 0
	m.ReviewModeStatusFn = func(context.Context, string) (reviewtransaction.RDDModeStatus, error) {
		reviewCalls++
		return reviewtransaction.RDDModeStatus{}, fmt.Errorf("unexpected native review status call")
	}
	m.ReviewModeSetGlobalFn = func(context.Context, string, bool) (reviewtransaction.RDDModeStatus, error) {
		reviewCalls++
		return reviewtransaction.RDDModeStatus{}, fmt.Errorf("unexpected native review mode mutation")
	}
	m.Screen, m.Cursor = tui.ScreenReview, 0
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(tui.Model)
	if cmd != nil || m.Screen != tui.ScreenOpenCodeSDKConfirm || m.Cursor != 1 {
		t.Fatalf("Review Enter: screen=%v cursor=%d command=%v err=%v", m.Screen, m.Cursor, cmd != nil, m.Err)
	}
	for _, text := range []string{"@opencode/plugin@2.0.4", "npm", config, "No / Back", "rollback"} {
		if !strings.Contains(m.View(), text) {
			t.Fatalf("SDK confirmation missing %q: %s", text, m.View())
		}
	}
	assertSDKBridgeMissing(t, log)
	assertSDKBridgeMissing(t, filepath.Join(config, "node_modules"))
	assertSDKBridgeMissing(t, state.Path(home))
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(tui.Model)
	if cmd != nil || m.Cursor != 0 {
		t.Fatal("Up did not select affirmative SDK confirmation")
	}
	assertSDKBridgeMissing(t, log)
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(tui.Model)
	if m.Screen != tui.ScreenInstalling || cmd == nil {
		t.Fatalf("affirmative did not schedule installation: screen=%v", m.Screen)
	}
	if realSDK {
		m = drainSDKBridgeCommands(t, m, cmd, 60*time.Second)
	} else {
		m = drainSDKBridgeCommands(t, m, cmd)
	}
	if m.Execution.Err != nil || !m.Execution.Prepare.Success || !m.Execution.Apply.Success {
		if realSDK {
			output, _ := os.ReadFile(filepath.Join(filepath.Dir(log), "npm-helper.log"))
			t.Logf("offline npm helper: %s", output)
		}
		t.Fatalf("full installation failed: %+v", m.Execution)
	}
	for name, stage := range map[string]pipeline.StageResult{"Prepare": m.Execution.Prepare, "Apply": m.Execution.Apply} {
		if len(stage.Steps) == 0 {
			t.Fatalf("%s did not execute any steps", name)
		}
		for _, step := range stage.Steps {
			if step.Status != pipeline.StepStatusSucceeded || step.Err != nil {
				t.Fatalf("%s step did not succeed: %+v", name, step)
			}
		}
	}
	if len(m.Execution.Prepare.Steps) != 5 {
		t.Fatalf("unexpected Prepare steps: %+v", m.Execution.Prepare.Steps)
	}
	read := func(path string) []byte {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	physicalConfig, err := filepath.EvalSymlinks(config)
	if err != nil {
		t.Fatal(err)
	}
	wantNPM := strings.Join([]string{physicalConfig, "install", "--save-exact", "--no-audit", "--no-fund", "--ignore-scripts", "--workspaces=false", "--prefix=" + physicalConfig, "--registry=https://registry.npmjs.org", "@opencode/plugin@2.0.4", ""}, "\n")
	if got := string(read(log)); got != wantNPM {
		t.Fatalf("expected exactly one pinned SDK install with isolated arguments: got %q, want %q", got, wantNPM)
	}
	if got := string(read(filepath.Join(config, "node_modules", "@opencode", "plugin", "package.json"))); !realSDK && got != "{\"version\":\"2.0.4\"}\n" {
		t.Fatalf("unexpected fake SDK manifest: %s", got)
	}
	for _, name := range []string{"model-variants.ts", "skill-registry.ts", "opencode-review-transport.ts", "telemetry-runtime.ts"} {
		if got := string(read(filepath.Join(config, "plugins", name))); got != assets.MustRead("opencode/plugins-v2/"+name) {
			t.Errorf("managed asset %s differs from embedded V2 bytes", name)
		}
	}
	read(filepath.Join(config, ".gentle-ai-telemetry-runtime.json"))
	if err := telemetryruntime.CheckManaged(config); err != nil {
		t.Fatalf("telemetry ownership is invalid: %v", err)
	}
	var settings struct {
		DefaultAgent string `json:"default_agent"`
		Agent        map[string]struct {
			Prompt string `json:"prompt"`
		} `json:"agent"`
	}
	if err := json.Unmarshal(read(filepath.Join(config, "opencode.json")), &settings); err != nil {
		t.Fatal(err)
	}
	if settings.DefaultAgent != "gentle-orchestrator" || !strings.Contains(settings.Agent["gentle-orchestrator"].Prompt, "gentle-ai:agent-routing") {
		t.Fatalf("missing generated default agent or routing: %+v", settings)
	}
	persisted, err := state.Read(home)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(persisted.InstalledAgents, ",") != "opencode" || !persisted.SelectionConfigured || !persisted.CommunityToolsConfigured || len(persisted.Components) != 0 || len(persisted.CommunityTools) != 0 || len(persisted.Skills) != 0 || persisted.Preset != m.Selection.Preset || persisted.SDDMode != m.Selection.SDDMode {
		t.Fatalf("installed selection was not persisted: %+v", persisted)
	}
	if reviewCalls != 0 || persisted.RDDMode != "" || persisted.RDDModeRecordedAt != nil || m.InstallReviewModePersisting || m.InstallReviewModePersistErr != nil {
		t.Fatalf("unexpected native review-mode work: calls=%d state=%+v", reviewCalls, persisted)
	}
	if !strings.Contains(strings.Join(m.Progress.Logs, "\n"), "pipeline completed successfully") {
		t.Fatalf("terminal success not surfaced to TUI: %v", m.Progress.Logs)
	}
}

// This is wrapper-assisted offline evidence, not unchanged production network
// behavior. Plugin activation requires a second explicit opt-in. Only an
// explicitly supplied cache is seeded.
func TestTUIOpenCodeSDKConsentRealNPMOfflineIntegration(t *testing.T) {
	if testing.Short() || os.Getenv("GENTLE_AI_REAL_NPM_OFFLINE") != "1" {
		t.Skip("opt in with GENTLE_AI_REAL_NPM_OFFLINE=1 and exact NPM, NODE, CACHE paths")
	}
	npm, node, cache := realSDKOfflineInputs(t)
	// Capture opt-ins and paths before isolation clears GENTLE_AI_* and PATH.
	approved := approvedSDKTempRoot(t)
	activate := os.Getenv("GENTLE_AI_INSTALLED_ACTIVATION") == "1"
	var host, python, launcher string
	if activate {
		if deadline, ok := t.Deadline(); ok && time.Until(deadline) < 150*time.Second {
			t.Fatal("installed activation requires -timeout=3m or longer to reserve cleanup after Apply")
		}
		for key, target := range map[string]*string{"GENTLE_AI_REAL_OPENCODE": &host, "GENTLE_AI_REAL_PYTHON": &python} {
			value := os.Getenv(key)
			if !filepath.IsAbs(value) {
				t.Fatalf("%s must be an explicit absolute executable path", key)
			}
			resolved, err := filepath.EvalSymlinks(value)
			if err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(resolved)
			if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
				t.Fatalf("invalid %s: %v", key, err)
			}
			*target = resolved
		}
		var err error
		launcher, err = filepath.Abs(filepath.Join("..", "..", "scripts", "test-opencode-v2-host.py"))
		if err != nil {
			t.Fatal(err)
		}
	}
	if temporary, err := filepath.EvalSymlinks(os.Getenv("TMPDIR")); err != nil || temporary != approved {
		t.Fatalf("parent TMPDIR must equal the approved temporary root GENTLE_AI_APPROVED_TMPDIR=%s", approved)
	}
	root := t.TempDir()
	home, config, log := isolateSDKBridgeTest(t, root, "")
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
	// Keep dependency probes fake; only the affirmative pinned install can reach
	// the helper. Its clean environment has no ambient tokens or npm settings.
	script := "#!/bin/sh\nset -eu\nif [ \"$1\" = --version ]; then printf '10.0.0\\n'; exit 0; fi\nexec /usr/bin/env -i "
	for _, item := range []string{"GENTLE_AI_REAL_NPM_OFFLINE=1", "GENTLE_AI_APPROVED_TMPDIR=" + approved, "GENTLE_AI_REAL_NPM=" + npm, "GENTLE_AI_REAL_NODE=" + node, "GENTLE_AI_REAL_NPM_CACHE=" + cache,
		"GOMODCACHE=" + os.Getenv("GOMODCACHE"), "GOCACHE=" + os.Getenv("GOCACHE")} {
		script += quote(item) + " "
	}
	script += "\"NPM_CONFIG_CACHE=${NPM_CONFIG_CACHE:?}\" "
	script += quote(binary) + " -test.run '^TestSDKRealNPMOfflineHelper$' -- sdk-offline " + quote(root) + " \"$@\" > " + quote(filepath.Join(root, "npm-helper.log")) + " 2>&1\n"
	writeSDKBridgeFile(t, filepath.Join(root, "bin", "npm"), script, 0700)
	runSDKFullApplyIntegration(t, home, config, log, true)
	if activate {
		activation := func(want []string, extra ...string) {
			// Python's 45s deadline and bounded group cleanup complete before this
			// fallback. SIGINT enters its finally blocks; WaitDelay allows reaping.
			ctx, cancel := context.WithTimeout(context.Background(), 65*time.Second)
			defer cancel()
			arguments := append([]string{"-I", "-B", launcher, host, filepath.Join(config, "node_modules"),
				"--host-version", "2.x", "--temp-root", root, "--installed-activation-only",
				"--installed-root", root, "--installed-config", config, "--installed-workspace", filepath.Join(root, "workspace")}, extra...)
			cmd := exec.CommandContext(ctx, python, arguments...)
			cmd.Dir = filepath.Join(root, "workspace")
			cmd.Env = []string{"HOME=" + home, "TMPDIR=" + filepath.Join(root, "tmp"), "PATH=/usr/bin:/bin"}
			cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
			cmd.WaitDelay = 15 * time.Second
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("installed activation %v failed: %v\n%s", extra, err, output)
			}
			for _, marker := range want {
				if !strings.Contains(string(output), marker) {
					t.Fatalf("installed activation %v lacks %q:\n%s", extra, marker, output)
				}
			}
			t.Logf("%s", output)
		}
		// Initial launch plus a relaunch against the same installed root.
		activation([]string{"PASS: installed activation:", "PASS: installed restart:"})
		// Negative control: hide the installed SDK outside every Node resolution
		// ancestor of the fixture configuration, then relaunch unchanged plugins.
		sdk := filepath.Join(config, "node_modules", "@opencode", "plugin")
		if err := os.Rename(sdk, filepath.Join(root, "hidden-opencode-plugin-sdk")); err != nil {
			t.Fatal(err)
		}
		activation([]string{"PASS: missing SDK refused:", "Cannot find package '@opencode/plugin'"}, "--installed-sdk-missing")
	}
}

func realSDKOfflineInputs(t *testing.T) (npm, node, cache string) {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Fatal("real offline integration requires macOS sandbox-exec")
	}
	if _, err := os.Stat("/usr/bin/sandbox-exec"); err != nil {
		t.Fatal(err)
	}
	paths := []*string{&npm, &node, &cache}
	for i, key := range []string{"GENTLE_AI_REAL_NPM", "GENTLE_AI_REAL_NODE", "GENTLE_AI_REAL_NPM_CACHE"} {
		value := os.Getenv(key)
		if !filepath.IsAbs(value) {
			t.Fatalf("%s must be an explicit absolute path", key)
		}
		resolved, err := filepath.EvalSymlinks(value)
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(resolved)
		if err != nil || (i == 2 && !info.IsDir()) || (i != 2 && !info.Mode().IsRegular()) {
			t.Fatalf("invalid %s: %v", key, err)
		}
		*paths[i] = resolved
	}
	return
}

// approvedSDKTempRoot is the operator-approved temporary root for the private
// real-npm fixtures, taken from GENTLE_AI_APPROVED_TMPDIR instead of any
// hard-coded per-user path. An opted-in run without it fails clearly.
func approvedSDKTempRoot(t *testing.T) string {
	t.Helper()
	value := os.Getenv("GENTLE_AI_APPROVED_TMPDIR")
	if value == "" || !filepath.IsAbs(value) {
		t.Fatal("opt-in real npm fixtures require GENTLE_AI_APPROVED_TMPDIR set to the approved absolute temporary root (and TMPDIR equal to it)")
	}
	resolved, err := filepath.EvalSymlinks(value)
	if err != nil {
		t.Fatalf("GENTLE_AI_APPROVED_TMPDIR must name an existing directory: %v", err)
	}
	if info, err := os.Stat(resolved); err != nil || !info.IsDir() {
		t.Fatalf("GENTLE_AI_APPROVED_TMPDIR must name an existing directory: %v", err)
	}
	return resolved
}

// Invoked only by the consent-pinned fixture wrapper, never by ordinary tests.
func TestSDKRealNPMOfflineHelper(t *testing.T) {
	marker := -1
	for i, arg := range os.Args {
		if arg == "sdk-offline" {
			marker = i
			break
		}
	}
	if marker < 0 || os.Getenv("GENTLE_AI_REAL_NPM_OFFLINE") != "1" {
		t.Skip("private opt-in npm wrapper helper")
	}
	npm, node, source := realSDKOfflineInputs(t)
	if marker+2 >= len(os.Args) {
		t.Fatal("missing fixture root or install arguments")
	}
	root, err := filepath.EvalSymlinks(os.Args[marker+1])
	if err != nil {
		t.Fatal(err)
	}
	if approved := approvedSDKTempRoot(t); !strings.HasPrefix(root, approved+string(filepath.Separator)) || !sdkOfflineCacheSeparate(root, source) {
		t.Fatal("fixture must be beneath the GENTLE_AI_APPROVED_TMPDIR root and separate from source cache")
	}
	config := filepath.Join(root, "config", "opencode")
	args := os.Args[marker+2:]
	want := []string{"install", "--save-exact", "--no-audit", "--no-fund", "--ignore-scripts", "--workspaces=false", "--prefix=" + config, "--registry=https://registry.npmjs.org", "@opencode/plugin@2.0.4"}
	if strings.Join(args, "\n") != strings.Join(want, "\n") {
		t.Fatalf("refusing unexpected npm arguments: %q", args)
	}
	cachePath := os.Getenv("NPM_CONFIG_CACHE")
	cacheParent, err := filepath.EvalSymlinks(filepath.Dir(cachePath))
	if err != nil || !strings.HasPrefix(cacheParent, root+"/") || filepath.Base(cachePath) != "npm-cache" {
		t.Fatalf("npm cache must be a fresh private fixture directory: %v", err)
	}
	privateCache := filepath.Join(cacheParent, "npm-cache")
	if err := os.Mkdir(privateCache, 0700); err != nil {
		t.Fatalf("refusing existing or unavailable private cache: %v", err)
	}
	entries, err := os.ReadDir(privateCache)
	if err != nil || len(entries) != 0 {
		t.Fatalf("refusing nonempty private cache: %v", err)
	}
	// sandbox-exec must succeed before any real Node/npm execution. A failed
	// sandbox is terminal, with no unsandboxed fallback. Even cache copying is
	// network-denied and cannot write outside the fixture (including the source).
	userConfig, globalConfig := filepath.Join(root, "user.npmrc"), filepath.Join(root, "global.npmrc")
	writeSDKBridgeFile(t, userConfig, "", 0600)
	writeSDKBridgeFile(t, globalConfig, "", 0600)
	env := []string{"HOME=" + filepath.Join(root, "home"), "PATH=" + filepath.Join(root, "bin"), "TMPDIR=" + filepath.Join(root, "tmp"), "NPM_CONFIG_CACHE=" + privateCache, "NPM_CONFIG_USERCONFIG=" + userConfig, "NPM_CONFIG_GLOBALCONFIG=" + globalConfig, "NPM_CONFIG_OFFLINE=true", "NPM_CONFIG_IGNORE_SCRIPTS=true", "NPM_CONFIG_AUDIT=false", "NPM_CONFIG_FUND=false"}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	run := func(command string, arguments ...string) []byte {
		t.Helper()
		cmd := sdkOfflineCommand(ctx, root, command, arguments...)
		cmd.Dir, cmd.Env = config, env
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("sandboxed %s failed (offline misses never authorize downloads): %v\n%s", command, err, output)
		}
		return output
	}
	run("/bin/cp", "-R", source+"/.", privateCache)
	writeSDKBridgeFile(t, filepath.Join(root, "npm-install.log"), config+"\n"+strings.Join(args, "\n")+"\n", 0600)
	run(node, append([]string{npm, "--offline", "--ignore-scripts"}, args...)...)
	// Import the installed SDK only, never the managed plugins or an OpenCode
	// host. Check the root export used by all four managed V2 assets and its
	// actual dist file, not merely the version or an unrelated SDK subpath.
	resolved := run(node, "--input-type=module", "-e", `
import fs from 'node:fs';
import { fileURLToPath } from 'node:url';
const manifest = JSON.parse(fs.readFileSync('node_modules/@opencode/plugin/package.json', 'utf8'));
if (manifest.name !== '@opencode/plugin' || manifest.version !== '2.0.4' || !manifest.exports) throw Error('invalid SDK manifest');
console.log(JSON.stringify(['@opencode/plugin'].map(spec =>
  fs.realpathSync(fileURLToPath(import.meta.resolve(spec))))));
`)
	var exports []string
	if err := json.Unmarshal(resolved, &exports); err != nil || len(exports) != 1 {
		t.Fatalf("invalid resolved SDK exports: %s (%v)", resolved, err)
	}
	for _, path := range exports {
		if !sdkOfflineExportContained(root, path) {
			t.Fatalf("SDK export escapes the fixture package dist: %s", path)
		}
	}
	// Import only the canonical paths admitted above, never the original
	// specifiers again. No module code executes during the resolution phase.
	run(node, "--input-type=module", "-e", `
import { pathToFileURL } from 'node:url';
for (const path of process.argv.slice(1)) {
  const sdk = await import(pathToFileURL(path).href);
  if (typeof sdk.Plugin?.define !== 'function') throw Error('missing SDK Plugin.define export');
}
`, "--", exports[0])
}

func sdkOfflineCacheSeparate(root, source string) bool {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	source, err = filepath.EvalSymlinks(source)
	return err == nil && !sdkOfflinePathWithin(root, source) && !sdkOfflinePathWithin(source, root)
}

func sdkOfflinePathWithin(parent, path string) bool {
	relative, err := filepath.Rel(parent, path)
	return err == nil && !filepath.IsAbs(relative) && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func sdkOfflineCommand(ctx context.Context, root, command string, arguments ...string) *exec.Cmd {
	// Refuse descendants instead of relying on nonportable process-group fields
	// or a racy process-tree walk. A package requiring a fork is unavailable in
	// this fixture; never relax the sandbox or fall back to an unguarded launch.
	profile := fmt.Sprintf("(version 1) (allow default) (deny network*) (deny process-fork) (deny file-write*) (allow file-write* (subpath %q) (literal \"/dev/null\"))", root)
	cmd := exec.CommandContext(ctx, "/usr/bin/sandbox-exec", append([]string{"-p", profile, command}, arguments...)...)
	cmd.WaitDelay = 200 * time.Millisecond
	return cmd
}

func sdkOfflineExportContained(root, path string) bool {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	dist, err := filepath.EvalSymlinks(filepath.Join(root, "config", "opencode", "node_modules", "@opencode", "plugin", "dist"))
	if err != nil || !sdkOfflinePathWithin(root, dist) {
		return false
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil || !sdkOfflinePathWithin(dist, path) {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func TestSDKOfflineHelperStartupWithoutGo(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test starts an isolated test binary")
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		optIn  string
		marker bool
	}{
		{name: "marker without opt-in", marker: true},
		{name: "opt-in without marker", optIn: "1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			args := []string{"-test.run=^TestSDKRealNPMOfflineHelper$", "-test.v"}
			if tc.marker {
				args = append(args, "--", "sdk-offline")
			}
			cmd := exec.CommandContext(ctx, binary, args...)
			cmd.Env = []string{
				"PATH=" + root, "HOME=" + root, "TMPDIR=" + root,
				"GOMODCACHE=" + filepath.Join(root, "modules"),
				"GOCACHE=" + filepath.Join(root, "build"),
				"GENTLE_AI_REAL_NPM_OFFLINE=" + tc.optIn,
			}
			output, err := cmd.CombinedOutput()
			if err != nil || !strings.Contains(string(output), "--- SKIP: TestSDKRealNPMOfflineHelper") || !strings.Contains(string(output), "private opt-in npm wrapper helper") {
				t.Fatalf("helper must start without Go and retain both invocation guards: %v\n%s", err, output)
			}
		})
	}
}

func TestSDKOfflineCacheSeparation(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "fixture")
	if err := os.MkdirAll(filepath.Join(root, "cache"), 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(base, "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, source string
		want         bool
	}{
		{"equal", root, false},
		{"descendant", filepath.Join(root, "cache"), false},
		{"ancestor", base, false},
		{"sibling", filepath.Join(base, "cache"), true},
		{"prefix sibling", root + "-cache", true},
		{"canonical equal", alias, false},
		{"canonical descendant", filepath.Join(alias, "cache"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.MkdirAll(tc.source, 0700); err != nil {
				t.Fatal(err)
			}
			if got := sdkOfflineCacheSeparate(root, tc.source); got != tc.want {
				t.Fatalf("cache separation = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSDKOfflineExportProvenance(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "fixture")
	inside := filepath.Join(root, "config", "opencode", "node_modules", "@opencode", "plugin", "dist", "promise", "index.js")
	outside := filepath.Join(base, "outside", "node_modules", "@opencode", "plugin", "dist", "promise", "index.js")
	writeSDKBridgeFile(t, inside, "", 0600)
	writeSDKBridgeFile(t, outside, "", 0600)
	link := filepath.Join(filepath.Dir(inside), "escaped-index.js")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, path string
		want       bool
	}{
		{"installed V2 root entry", inside, true},
		{"outside matching substring", outside, false},
		{"symlink escape", link, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sdkOfflineExportContained(root, tc.path); got != tc.want {
				t.Fatalf("export provenance = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSDKOfflineProcessBounds(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS sandbox guard")
	}
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// Shell must fork to run the conditional command. Success means the
	// sandbox allowed a descendant, which is unsafe for direct-child cleanup.
	cmd := sdkOfflineCommand(ctx, root, "/bin/sh", "-c", "if /bin/sleep 0; then exit 99; fi")
	cmd.Env = []string{"HOME=" + root, "PATH=/usr/bin:/bin", "TMPDIR=" + root}
	output, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "Operation not permitted") {
		t.Fatalf("descendant was not refused: %v, %s", err, output)
	}
	if cmd.WaitDelay <= 0 || cmd.WaitDelay > time.Second {
		t.Fatal("pipe drain must have a positive bound of at most one second")
	}
	ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	cmd = sdkOfflineCommand(ctx, root, "/bin/sleep", "10")
	cmd.Env = []string{"HOME=" + root}
	if _, err := cmd.CombinedOutput(); err == nil || ctx.Err() == nil || time.Since(start) > 2*time.Second {
		t.Fatalf("direct child deadline not enforced: %v", err)
	}
}

func isolateSDKBridgeTest(t *testing.T, root, version string) (home, config, log string) {
	t.Helper()
	home = filepath.Join(root, "home")
	config = filepath.Join(root, "config", "opencode")
	log = filepath.Join(root, "npm-install.log")
	// Do not inherit host-specific config roots or runtime injection settings.
	for _, env := range os.Environ() {
		name, _, _ := strings.Cut(env, "=")
		if strings.HasPrefix(name, "OPENCODE_") || strings.HasPrefix(name, "GENTLE_AI_") || strings.HasPrefix(name, "XDG_") {
			t.Setenv(name, "")
		}
	}
	for name, dir := range map[string]string{
		"HOME": home, "USERPROFILE": home, "XDG_CONFIG_HOME": filepath.Dir(config),
		"XDG_DATA_HOME": filepath.Join(root, "data"), "XDG_STATE_HOME": filepath.Join(root, "state"),
		"XDG_CACHE_HOME": filepath.Join(root, "cache"), "XDG_RUNTIME_DIR": filepath.Join(root, "run"),
		"TMPDIR": filepath.Join(root, "tmp"), "TMP": filepath.Join(root, "tmp"), "TEMP": filepath.Join(root, "tmp"),
		"APPDATA": filepath.Join(root, "appdata"), "LOCALAPPDATA": filepath.Join(root, "localappdata"),
	} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		t.Setenv(name, dir)
	}
	t.Setenv("GENTLE_AI_NO_ANIMATION", "1")
	work := filepath.Join(root, "workspace")
	if err := os.MkdirAll(work, 0700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(work)
	bin := filepath.Join(root, "bin")
	t.Setenv("PATH", bin)
	// ContinueOnError still probes dependencies. Only controlled version-only
	// scripts are discoverable, and npm rejects every other unexpected command.
	for _, name := range []string{"git", "curl", "node", "go", "brew"} {
		writeSDKBridgeFile(t, filepath.Join(bin, name), "#!/bin/sh\ncase \"$1\" in --version|version) printf '22.0.0\\n';; *) exit 97;; esac\n", 0700)
	}
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
	script := "#!/bin/sh\nset -eu\nif [ \"$1\" = --version ]; then printf '10.0.0\\n'; exit 0; fi\n[ \"$1\" = install ] || exit 98\n{ pwd -P; printf '%s\\n' \"$@\"; } >> " + quote(log) + "\n"
	if version != "" {
		script += "/bin/mkdir -p node_modules/@opencode/plugin\nprintf '%s\\n' " + quote(fmt.Sprintf(`{"version":%q}`, version)) + " > node_modules/@opencode/plugin/package.json\n"
	}
	writeSDKBridgeFile(t, filepath.Join(bin, "npm"), script, 0700)
	writeSDKBridgeFile(t, filepath.Join(config, "plugins", "telemetry-runtime.ts"), "// custom telemetry blocker\n", 0600)
	return home, config, log
}

func writeSDKBridgeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func assertSDKBridgeMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("expected absent %s, got %v", path, err)
	}
}

func drainSDKBridgeCommands(t *testing.T, m tui.Model, initial tea.Cmd, timeout ...time.Duration) tui.Model {
	t.Helper()
	messages := make(chan tea.Msg, 16)
	stop := make(chan struct{})
	defer close(stop)
	launch := func(cmd tea.Cmd) {
		if cmd != nil {
			go func() {
				msg := cmd()
				select {
				case messages <- msg:
				case <-stop:
				}
			}()
		}
	}
	launch(initial)
	wait := 10 * time.Second
	if len(timeout) != 0 {
		wait = timeout[0]
	}
	deadline := time.NewTimer(wait)
	defer deadline.Stop()
	for count := 0; count < 128; count++ {
		select {
		case msg := <-messages:
			if batch, ok := msg.(tea.BatchMsg); ok {
				for _, cmd := range batch {
					launch(cmd)
				}
				continue
			}
			updated, cmd := m.Update(msg)
			m = updated.(tui.Model)
			if _, done := msg.(tui.PipelineDoneMsg); done {
				if cmd != nil {
					t.Fatal("failed pipeline scheduled unexpected follow-up work")
				}
				return m
			}
			launch(cmd)
		case <-deadline.C:
			t.Fatalf("TUI bridge did not deliver PipelineDoneMsg within %s", wait)
		}
	}
	t.Fatal("TUI bridge exceeded 128 command/progress messages")
	return m
}
