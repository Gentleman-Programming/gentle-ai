package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/kimi"
	"github.com/gentleman-programming/gentle-ai/v2/internal/backup"
	"github.com/gentleman-programming/gentle-ai/v2/internal/installcmd"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/pipeline"
	"github.com/gentleman-programming/gentle-ai/v2/internal/planner"
	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

// missingBinaryLookPath simulates all installable binaries (engram, gga) as
// missing. Go availability is no longer required for engram installation
// (pre-built binaries are downloaded directly from GitHub Releases).
func missingBinaryLookPath(name string) (string, error) {
	return "", exec.ErrNotFound
}

func assertFileContains(t *testing.T, path string, want string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	if !strings.Contains(string(body), want) {
		t.Fatalf("file %q missing %q; got:\n%s", path, want, string(body))
	}
}

func stringSliceContains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

const engramInitCommandForTest = "npm exec --yes --package gentle-engram@latest -- pi-engram init"

func TestRunInstallAppliesFilesystemChanges(t *testing.T) {
	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	runCommand = func(string, ...string) error { return nil }
	cmdLookPath = missingBinaryLookPath

	result, err := RunInstall([]string{"--agent", "opencode", "--component", "permissions"}, system.DetectionResult{})
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	if !result.Verify.Ready {
		t.Fatalf("verification ready = false, report = %#v", result.Verify)
	}

	configPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("expected config file %q: %v", configPath, err)
	}
}

// TestRunInstallReturnsStatePersistenceFailure verifies that a failed state
// commit restores the managed asset bytes and preserves the previous state.
func TestRunInstallReturnsStatePersistenceFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	osUserHomeDir = func() (string, error) { return home, nil }
	runCommand = func(string, ...string) error { return nil }
	cmdLookPath = missingBinaryLookPath
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
	})

	if err := state.Write(home, state.InstallState{}); err != nil {
		t.Fatal(err)
	}
	originalState, err := os.ReadFile(state.Path(home))
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	if _, err := os.ReadFile(configPath); !os.IsNotExist(err) {
		t.Fatalf("pre-install config read error = %v, want absent", err)
	}
	statePath := state.Path(home)
	target := filepath.Join(home, ".gentle-ai", "persisted-state.json")
	if err := os.Rename(statePath, target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, statePath); err != nil {
		t.Skipf("state symlink unavailable: %v", err)
	}

	_, err = RunInstall([]string{"--agent", "opencode", "--component", "permissions"}, system.DetectionResult{})
	if err == nil || !strings.Contains(err.Error(), "persist install state") {
		t.Fatalf("RunInstall() error = %v, want state persistence failure", err)
	}
	if _, readErr := os.ReadFile(configPath); !os.IsNotExist(readErr) {
		t.Fatalf("config after failed install read error = %v, want absent", readErr)
	}
	finalState, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(finalState) != string(originalState) {
		t.Fatalf("state after failed install changed:\n got %s\nwant %s", finalState, originalState)
	}
}

func TestRunInstallEngramForPiAndOpenCodeProvisionsBothMCPTargets(t *testing.T) {
	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	cmdLookPath = func(name string) (string, error) {
		return filepath.Join(home, "bin", name), nil
	}
	restorePreflightLookPath := installcmd.OverrideLookPath(func(name string) (string, error) {
		return filepath.Join(home, "bin", name), nil
	})
	t.Cleanup(restorePreflightLookPath)

	var commands []string
	runCommand = func(name string, args ...string) error {
		commands = append(commands, strings.Join(append([]string{name}, args...), " "))
		// Simulate pi-engram init writing mcp.json with the new schema.
		isNpmEngramInit := name == "npm" && len(args) >= 7 && args[5] == "pi-engram" && args[6] == "init"
		if isNpmEngramInit {
			mcpPath := filepath.Join(home, ".pi", "agent", "mcp.json")
			if err := os.MkdirAll(filepath.Dir(mcpPath), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(mcpPath, []byte(`{"activeMCP":"engram","mcpServers":{"engram":{"command":"node","args":["--eval","require('child_process').spawn('engram',['mcp','--tools=agent'],{stdio:'inherit'})"]}}}`+"\n"), 0o644); err != nil {
				return err
			}
		}
		return nil
	}

	result, err := RunInstall([]string{
		"--agent", "pi",
		"--agent", "opencode",
		"--component", "engram",
	}, system.DetectionResult{})
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	if !result.Verify.Ready {
		t.Fatalf("verification ready = false, report = %#v", result.Verify)
	}

	assertFileContains(t, filepath.Join(home, ".pi", "agent", "settings.json"), "npm:pi-mcp-adapter")
	assertFileContains(t, filepath.Join(home, ".pi", "npm", "package.json"), "pi-mcp-adapter")
	assertFileContains(t, filepath.Join(home, ".config", "opencode", "opencode.json"), "engram")

	if !stringSliceContains(commands, "pi install npm:pi-mcp-adapter") {
		t.Fatalf("commands missing %q; got %v", "pi install npm:pi-mcp-adapter", commands)
	}
	if !stringSliceContains(commands, engramInitCommandForTest) {
		t.Fatalf("commands missing %q; got %v", engramInitCommandForTest, commands)
	}
}

// TestAgentInstallStepSkipsMissingNonPiRuntime proves an explicitly selected
// desktop agent does not block installation or trigger agent acquisition when
// its runtime is absent.
func TestAgentInstallStepSkipsMissingNonPiRuntime(t *testing.T) {
	restoreCommand := runCommand
	recorder := &commandRecorder{}
	runCommand = recorder.record
	t.Cleanup(func() { runCommand = restoreCommand })

	step := agentInstallStep{
		id:      "agent:vscode-copilot",
		agent:   model.AgentVSCodeCopilot,
		homeDir: t.TempDir(),
	}
	if err := step.Run(); err != nil {
		t.Fatalf("agentInstallStep.Run() error = %v, want absent non-Pi runtime to be skipped", err)
	}
	if got := recorder.get(); len(got) != 0 {
		t.Fatalf("commands executed = %v, want none for non-Pi agent", got)
	}
}

func TestPiAgentInstallProgressUsesAdapterCommandNames(t *testing.T) {
	restorePreflightLookPath := installcmd.OverrideLookPath(func(name string) (string, error) { return name, nil })
	t.Cleanup(restorePreflightLookPath)

	restoreCommand := runCommand
	t.Cleanup(func() { runCommand = restoreCommand })
	runCommand = func(string, ...string) error { return nil }

	var events []pipeline.ProgressEvent
	step := agentInstallStep{
		id:      "agent:pi",
		agent:   model.AgentPi,
		homeDir: t.TempDir(),
		progress: func(event pipeline.ProgressEvent) {
			events = append(events, event)
		},
	}
	if err := step.Run(); err != nil {
		t.Fatalf("agentInstallStep.Run() error = %v", err)
	}

	wantPackages := []string{"pi install npm:gentle-pi", "pi install npm:gentle-engram", "pi install npm:pi-mcp-adapter", engramInitCommandForTest, "pi install npm:pi-subagents-j0k3r", "pi install npm:@juicesharp/rpiv-ask-user-question", "pi install npm:pi-web-access", "pi install npm:@juicesharp/rpiv-todo", "pi install npm:pi-btw"}
	if len(events) != len(wantPackages)*2 {
		t.Fatalf("progress events = %d, want %d: %v", len(events), len(wantPackages)*2, events)
	}
	for i, commandLabel := range wantPackages {
		wantID := "agent:pi:" + commandLabel
		if events[i*2].StepID != wantID || events[i*2].Status != pipeline.StepStatusRunning {
			t.Fatalf("running event[%d] = %+v, want step %q", i*2, events[i*2], wantID)
		}
		if events[i*2+1].StepID != wantID || events[i*2+1].Status != pipeline.StepStatusSucceeded {
			t.Fatalf("succeeded event[%d] = %+v, want step %q", i*2+1, events[i*2+1], wantID)
		}
	}
}

func TestRunCommandSequenceWithProgressStopsAfterFailedCommand(t *testing.T) {
	restoreCommand := runCommand
	t.Cleanup(func() { runCommand = restoreCommand })
	var commands []string
	runCommand = func(name string, args ...string) error {
		commands = append(commands, strings.Join(append([]string{name}, args...), " "))
		return errors.New("package install failed")
	}

	var events []pipeline.ProgressEvent
	err := runCommandSequenceWithProgress(
		[][]string{{"pi", "install", "npm:first"}, {"pi", "install", "npm:second"}},
		func(event pipeline.ProgressEvent) { events = append(events, event) },
		"agent:pi",
	)
	if err == nil || !strings.Contains(err.Error(), "package install failed") {
		t.Fatalf("runCommandSequenceWithProgress() error = %v, want package failure", err)
	}
	if len(commands) != 1 || commands[0] != "pi install npm:first" {
		t.Fatalf("commands = %v, want only the failed command", commands)
	}
	if len(events) != 2 {
		t.Fatalf("progress events = %v, want running and failed", events)
	}
	if events[0].StepID != "agent:pi:pi install npm:first" || events[0].Status != pipeline.StepStatusRunning {
		t.Fatalf("running event = %+v", events[0])
	}
	if events[1].StepID != events[0].StepID || events[1].Status != pipeline.StepStatusFailed || events[1].Err == nil {
		t.Fatalf("failed event = %+v", events[1])
	}
}

func TestPiAgentInstallRunsPackageCommandsWhenPiAlreadyInstalled(t *testing.T) {
	binDir := t.TempDir()
	fakePi := filepath.Join(binDir, "pi")
	if err := os.WriteFile(fakePi, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(fake pi) error = %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	fakeNpm := filepath.Join(binDir, "npm")
	if err := os.WriteFile(fakeNpm, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(fake npm) error = %v", err)
	}

	restorePreflightLookPath := installcmd.OverrideLookPath(func(name string) (string, error) {
		switch name {
		case "pi":
			return fakePi, nil
		case "npm":
			// Pi's install runs npm exec for engram init, so npm must be present.
			return fakeNpm, nil
		default:
			return "", exec.ErrNotFound
		}
	})
	t.Cleanup(restorePreflightLookPath)

	restoreCommand := runCommand
	t.Cleanup(func() { runCommand = restoreCommand })

	var commands []string
	runCommand = func(name string, args ...string) error {
		commands = append(commands, strings.Join(append([]string{name}, args...), " "))
		return nil
	}

	step := agentInstallStep{
		id:      "agent:pi",
		agent:   model.AgentPi,
		homeDir: t.TempDir(),
	}

	if err := step.Run(); err != nil {
		t.Fatalf("agentInstallStep.Run() error = %v", err)
	}

	for _, want := range []string{
		"pi install npm:gentle-pi",
		"pi install npm:gentle-engram",
		"pi install npm:pi-mcp-adapter",
		engramInitCommandForTest,
		"pi install npm:pi-subagents-j0k3r",
		"pi install npm:@juicesharp/rpiv-ask-user-question",
		"pi install npm:pi-web-access",
		"pi install npm:@juicesharp/rpiv-todo",
		"pi install npm:pi-btw",
	} {
		if !stringSliceContains(commands, want) {
			t.Fatalf("commands missing %q; got %v", want, commands)
		}
	}
}

func TestRunInstallRollsBackOnComponentFailure(t *testing.T) {
	home := t.TempDir()
	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	before := []byte("{\n  \"existing\": true\n}\n")
	if err := os.WriteFile(settingsPath, before, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
	})
	cmdLookPath = missingBinaryLookPath

	osUserHomeDir = func() (string, error) { return home, nil }
	runCommand = func(name string, args ...string) error {
		if name == "brew" && len(args) == 2 && args[0] == "install" && args[1] == "engram" {
			return os.ErrPermission
		}
		return nil
	}

	// Use only engram (not context7) — context7 injects MCP config into
	// the settings file and does not have a rollback step, so including it
	// makes the before/after comparison fail even when the pipeline rollback
	// works correctly. Context7 rollback is tracked separately.
	_, err := RunInstall(
		[]string{"--agent", "opencode", "--component", "engram"},
		system.DetectionResult{},
	)
	if err == nil {
		t.Fatalf("RunInstall() expected error")
	}

	if !strings.Contains(err.Error(), "execute install pipeline") {
		t.Fatalf("RunInstall() error = %v", err)
	}

	after, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if string(after) != string(before) {
		t.Fatalf("settings content changed after rollback\nafter=%s\nbefore=%s", after, before)
	}
}

type failingPersonaInstallStep struct{}

func (failingPersonaInstallStep) ID() string { return "test:fail-after-persona" }
func (failingPersonaInstallStep) Run() error {
	return errors.New("forced failure after persona cleanup")
}

func TestInstallPersonaOnlyRollbackRestoresOpenCodeSettingsAfterCleanup(t *testing.T) {
	home := t.TempDir()
	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	before := []byte("// preserve exact JSONC bytes\n{\"agent\":{\"gentleman\":{\"tools\":{\"write\":true},\"description\":\"keep\"},\"user-owned\":{\"tools\":{\"custom\":true}}}}\n")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath, before, 0o644); err != nil {
		t.Fatal(err)
	}

	selection := model.Selection{
		Agents:     []model.AgentID{model.AgentOpenCode},
		Components: []model.ComponentID{model.ComponentPersona},
		Persona:    model.PersonaGentleman,
	}
	resolved := planner.ResolvedPlan{Agents: selection.Agents, OrderedComponents: selection.Components}
	runtime, err := newInstallRuntime(home, ScopeGlobal, ChannelStable, selection, resolved, system.PlatformProfile{})
	if err != nil {
		t.Fatal(err)
	}
	restoreCommand := runCommand
	runCommand = func(string, ...string) error { return nil }
	t.Cleanup(func() { runCommand = restoreCommand })
	plan := runtime.stagePlan()
	plan.Prepare = plan.Prepare[1:]
	plan.Apply = append(plan.Apply, failingPersonaInstallStep{})
	result := pipeline.NewOrchestrator(pipeline.DefaultRollbackPolicy()).Execute(plan)
	if result.Err == nil || !result.Rollback.Success {
		t.Fatalf("persona-only install rollback = %#v", result)
	}
	after, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("persona-only install rollback settings = %q, want exact before-image %q", after, before)
	}
}

// --- Batch D: Linux profile runtime wiring integration tests ---

// linuxDetectionResult builds a DetectionResult with a Linux profile for integration tests.
func linuxDetectionResult(distro, pkgMgr string) system.DetectionResult {
	return system.DetectionResult{
		System: system.SystemInfo{
			OS:        "linux",
			Arch:      "amd64",
			Shell:     "/bin/bash",
			Supported: true,
			Profile: system.PlatformProfile{
				OS:             "linux",
				LinuxDistro:    distro,
				PackageManager: pkgMgr,
				Supported:      true,
			},
		},
	}
}

// commandRecorder captures all external commands invoked during a pipeline run.
type commandRecorder struct {
	mu       sync.Mutex
	commands []string
}

func (r *commandRecorder) record(name string, args ...string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commands = append(r.commands, fmt.Sprintf("%s %s", name, strings.Join(args, " ")))
	return nil
}

func (r *commandRecorder) get() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]string, len(r.commands))
	copy(cp, r.commands)
	return cp
}

func TestRunInstallLinuxUbuntuResolvesAptCommands(t *testing.T) {
	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	cmdLookPath = missingBinaryLookPath
	recorder := &commandRecorder{}
	runCommand = recorder.record

	detection := linuxDetectionResult(system.LinuxDistroUbuntu, "apt")
	result, err := RunInstall(
		[]string{"--agent", "opencode", "--component", "permissions"},
		detection,
	)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	if !result.Verify.Ready {
		t.Fatalf("verification ready = false, report = %#v", result.Verify)
	}

	// Verify platform decision was resolved from the Linux profile.
	if result.Resolved.PlatformDecision.OS != "linux" {
		t.Fatalf("platform decision OS = %q, want linux", result.Resolved.PlatformDecision.OS)
	}
	if result.Resolved.PlatformDecision.PackageManager != "apt" {
		t.Fatalf("platform decision package manager = %q, want apt", result.Resolved.PlatformDecision.PackageManager)
	}
}

func TestRunInstallLinuxArchResolvesPacmanCommands(t *testing.T) {
	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	cmdLookPath = missingBinaryLookPath
	recorder := &commandRecorder{}
	runCommand = recorder.record

	detection := linuxDetectionResult(system.LinuxDistroArch, "pacman")
	result, err := RunInstall(
		[]string{"--agent", "opencode", "--component", "permissions"},
		detection,
	)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	if !result.Verify.Ready {
		t.Fatalf("verification ready = false, report = %#v", result.Verify)
	}

	if result.Resolved.PlatformDecision.PackageManager != "pacman" {
		t.Fatalf("platform decision package manager = %q, want pacman", result.Resolved.PlatformDecision.PackageManager)
	}
}

func TestRunInstallLinuxUbuntuWithEngramUsesDirectDownload(t *testing.T) {
	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	cmdLookPath = missingBinaryLookPath
	recorder := &commandRecorder{}
	runCommand = recorder.record

	// Override engramDownloadFn to avoid real HTTP calls.
	origDownloadFn := engramDownloadFn
	engramDownloadFn = func(profile system.PlatformProfile) (string, error) {
		return "/tmp/fake-engram", nil
	}
	t.Cleanup(func() { engramDownloadFn = origDownloadFn })

	detection := linuxDetectionResult(system.LinuxDistroUbuntu, "apt")
	result, err := RunInstall(
		[]string{"--agent", "opencode", "--component", "engram"},
		detection,
	)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	if !result.Verify.Ready {
		t.Fatalf("verification ready = false, report = %#v", result.Verify)
	}

	// Must NOT use go install for engram on Linux.
	for _, cmd := range recorder.get() {
		if strings.Contains(cmd, "go install") && strings.Contains(cmd, "engram") {
			t.Fatalf("Linux engram install should NOT use go install, got command: %s", cmd)
		}
	}
}

func TestRunInstallLinuxArchWithEngramUsesDirectDownload(t *testing.T) {
	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	cmdLookPath = missingBinaryLookPath
	recorder := &commandRecorder{}
	runCommand = recorder.record

	origDownloadFn := engramDownloadFn
	engramDownloadFn = func(profile system.PlatformProfile) (string, error) {
		return "/tmp/fake-engram", nil
	}
	t.Cleanup(func() { engramDownloadFn = origDownloadFn })

	detection := linuxDetectionResult(system.LinuxDistroArch, "pacman")
	result, err := RunInstall(
		[]string{"--agent", "opencode", "--component", "engram"},
		detection,
	)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	if !result.Verify.Ready {
		t.Fatalf("verification ready = false, report = %#v", result.Verify)
	}

	// Must NOT use go install for engram on Arch Linux.
	for _, cmd := range recorder.get() {
		if strings.Contains(cmd, "go install") && strings.Contains(cmd, "engram") {
			t.Fatalf("Arch Linux engram install should NOT use go install, got command: %s", cmd)
		}
	}
}

func TestRunInstallLinuxRollsBackOnComponentFailure(t *testing.T) {
	home := t.TempDir()
	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	before := []byte("{\n  \"linux-original\": true\n}\n")
	if err := os.WriteFile(settingsPath, before, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
	})
	cmdLookPath = missingBinaryLookPath

	osUserHomeDir = func() (string, error) { return home, nil }
	runCommand = func(name string, args ...string) error { return nil }

	// Fail the engram download to trigger rollback.
	origDownloadFn := engramDownloadFn
	engramDownloadFn = func(profile system.PlatformProfile) (string, error) {
		return "", os.ErrPermission
	}
	t.Cleanup(func() { engramDownloadFn = origDownloadFn })

	detection := linuxDetectionResult(system.LinuxDistroUbuntu, "apt")
	// Exclude context7 — it has no rollback and taints the settings file.
	_, err := RunInstall(
		[]string{"--agent", "opencode", "--component", "engram"},
		detection,
	)
	if err == nil {
		t.Fatalf("RunInstall() expected error")
	}

	if !strings.Contains(err.Error(), "execute install pipeline") {
		t.Fatalf("RunInstall() error = %v", err)
	}

	// Verify rollback restored the original file.
	after, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if string(after) != string(before) {
		t.Fatalf("settings content changed after rollback on Linux\nafter=%s\nbefore=%s", after, before)
	}
}

// TestRunInstallWorkspaceScopeRollback_SurfacesRealError reproduces issue #2451:
// a workspace-scoped install (--scope workspace) legitimately writes files whose
// OriginalPath resolves outside the user home directory. When the backup snapshot
// captures such a path (Existed:false, since it does not exist yet) and a LATER
// apply step fails, rollback fires and must restore/remove that workspace-scoped
// entry — not refuse it and mask the real download failure with a validation
// error claiming the path must be "under the user home directory".
func TestRunInstallWorkspaceScopeRollback_SurfacesRealError(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()

	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	originalCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
		if err := os.Chdir(originalCwd); err != nil {
			t.Errorf("failed to restore working directory: %v", err)
		}
	})
	cmdLookPath = missingBinaryLookPath

	osUserHomeDir = func() (string, error) { return home, nil }
	runCommand = func(name string, args ...string) error { return nil }

	if err := os.Chdir(workspace); err != nil {
		t.Fatalf("failed to change working directory to temp workspace: %v", err)
	}

	// Fail the engram download AFTER the backup snapshot has already captured
	// the not-yet-existing workspace-scoped engram MCP config path (Existed:false),
	// so the pipeline's apply stage fails and rollback fires against that entry.
	origDownloadFn := engramDownloadFn
	engramDownloadFn = func(profile system.PlatformProfile) (string, error) {
		return "", os.ErrPermission
	}
	t.Cleanup(func() { engramDownloadFn = origDownloadFn })

	detection := linuxDetectionResult(system.LinuxDistroUbuntu, "apt")
	_, err = RunInstall(
		[]string{"--scope", "workspace", "--agent", "opencode", "--component", "engram"},
		detection,
	)
	if err == nil {
		t.Fatalf("RunInstall() expected error")
	}

	if strings.Contains(err.Error(), "user home directory") {
		t.Fatalf("rollback masked the real download error with a home-directory validation refusal: %v", err)
	}
	if !strings.Contains(err.Error(), "permission") {
		t.Fatalf("RunInstall() error = %v, want it to surface the real download failure (os.ErrPermission)", err)
	}
}

func TestRunInstallFedoraQwenEngramSkipsUnsupportedSetupAndWritesSettings(t *testing.T) {
	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	cmdLookPath = missingBinaryLookPath
	recorder := &commandRecorder{}
	runCommand = recorder.record

	origDownloadFn := engramDownloadFn
	engramDownloadFn = func(profile system.PlatformProfile) (string, error) {
		return filepath.Join(home, "bin", "engram"), nil
	}
	t.Cleanup(func() { engramDownloadFn = origDownloadFn })

	detection := linuxDetectionResult(system.LinuxDistroFedora, "dnf")
	result, err := RunInstall(
		[]string{"--agent", "qwen-code", "--component", "engram"},
		detection,
	)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	if !result.Verify.Ready {
		t.Fatalf("verification ready = false, report = %#v", result.Verify)
	}

	settingsPath := filepath.Join(home, ".qwen", "settings.json")
	if _, err := os.Stat(settingsPath); err != nil {
		t.Fatalf("expected qwen settings at %q: %v", settingsPath, err)
	}

	for _, cmd := range recorder.get() {
		if strings.Contains(cmd, "engram setup qwen-code") {
			t.Fatalf("unexpected unsupported setup command: %s", cmd)
		}
	}
}

// --- Batch E: Linux verification and macOS parity matrix ---

func TestRunInstallLinuxVerificationReportsReadyOnSuccess(t *testing.T) {
	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	runCommand = func(string, ...string) error { return nil }
	cmdLookPath = missingBinaryLookPath

	detection := linuxDetectionResult(system.LinuxDistroUbuntu, "apt")
	result, err := RunInstall(
		[]string{"--agent", "opencode", "--component", "permissions"},
		detection,
	)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	if !result.Verify.Ready {
		t.Fatalf("Verify.Ready = false, want true for successful Linux install")
	}
	if result.Verify.Failed != 0 {
		t.Fatalf("Verify.Failed = %d, want 0", result.Verify.Failed)
	}
}

func TestRunInstallLinuxArchVerificationReportsReadyOnSuccess(t *testing.T) {
	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	runCommand = func(string, ...string) error { return nil }
	cmdLookPath = missingBinaryLookPath

	detection := linuxDetectionResult(system.LinuxDistroArch, "pacman")
	result, err := RunInstall(
		[]string{"--agent", "opencode", "--component", "permissions"},
		detection,
	)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	if !result.Verify.Ready {
		t.Fatalf("Verify.Ready = false, want true for successful Arch install")
	}
}

func TestRunInstallLinuxDryRunSkipsVerification(t *testing.T) {
	detection := linuxDetectionResult(system.LinuxDistroUbuntu, "apt")
	result, err := RunInstall([]string{"--dry-run"}, detection)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	if !result.DryRun {
		t.Fatalf("DryRun = false, want true")
	}
	// Verify report should be zero-value (no checks run in dry-run)
	if result.Verify.Passed != 0 || result.Verify.Failed != 0 {
		t.Fatalf("expected zero verify counters in dry-run, got passed=%d failed=%d", result.Verify.Passed, result.Verify.Failed)
	}
}

func TestRunInstallLinuxDryRunPlatformDecisionRendersCorrectly(t *testing.T) {
	detection := linuxDetectionResult(system.LinuxDistroArch, "pacman")
	result, err := RunInstall([]string{"--dry-run"}, detection)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	output := RenderDryRun(result)
	want := "os=linux distro=arch package-manager=pacman status=supported"
	if !strings.Contains(output, want) {
		t.Fatalf("RenderDryRun() missing platform decision\noutput=%s\nwant contains=%s", output, want)
	}
}

// --- macOS parity regression checks ---

func macOSDetectionResult() system.DetectionResult {
	return system.DetectionResult{
		System: system.SystemInfo{
			OS:        "darwin",
			Arch:      "arm64",
			Shell:     "/bin/zsh",
			Supported: true,
			Profile: system.PlatformProfile{
				OS:             "darwin",
				PackageManager: "brew",
				Supported:      true,
			},
		},
	}
}

func TestRunInstallMacOSStillResolvesBrewCommands(t *testing.T) {
	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	cmdLookPath = missingBinaryLookPath
	recorder := &commandRecorder{}
	runCommand = recorder.record

	detection := macOSDetectionResult()
	result, err := RunInstall(
		[]string{"--agent", "opencode", "--component", "engram"},
		detection,
	)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	if !result.Verify.Ready {
		t.Fatalf("macOS verification ready = false")
	}

	// Verify brew install command was used, not apt or pacman.
	commands := recorder.get()
	foundBrew := false
	for _, cmd := range commands {
		if strings.Contains(cmd, "brew install engram") {
			foundBrew = true
			break
		}
	}
	if !foundBrew {
		t.Fatalf("expected brew install for macOS engram, got commands: %v", commands)
	}
}

func TestRunInstallMacOSDryRunPlatformDecision(t *testing.T) {
	detection := macOSDetectionResult()
	result, err := RunInstall([]string{"--dry-run"}, detection)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	if result.Resolved.PlatformDecision.OS != "darwin" {
		t.Fatalf("macOS platform decision OS = %q, want darwin", result.Resolved.PlatformDecision.OS)
	}
	if result.Resolved.PlatformDecision.PackageManager != "brew" {
		t.Fatalf("macOS platform decision PM = %q, want brew", result.Resolved.PlatformDecision.PackageManager)
	}
	if !result.Resolved.PlatformDecision.Supported {
		t.Fatalf("macOS platform decision Supported = false, want true")
	}
}

func TestRunInstallMacOSVerificationMatchesPreLinuxBehavior(t *testing.T) {
	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	runCommand = func(string, ...string) error { return nil }
	cmdLookPath = missingBinaryLookPath

	detection := macOSDetectionResult()
	result, err := RunInstall(
		[]string{"--agent", "opencode", "--component", "permissions"},
		detection,
	)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	if !result.Verify.Ready {
		t.Fatalf("macOS verify ready = false, want true")
	}
	if result.Verify.Failed != 0 {
		t.Fatalf("macOS verify failed = %d, want 0", result.Verify.Failed)
	}
}

func TestRunInstallMacOSRollbackStillWorks(t *testing.T) {
	home := t.TempDir()
	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	before := []byte("{\n  \"macos-original\": true\n}\n")
	if err := os.WriteFile(settingsPath, before, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
	})
	cmdLookPath = missingBinaryLookPath

	osUserHomeDir = func() (string, error) { return home, nil }
	runCommand = func(name string, args ...string) error {
		if name == "brew" && len(args) == 2 && args[0] == "install" && args[1] == "engram" {
			return os.ErrPermission
		}
		return nil
	}

	detection := macOSDetectionResult()
	// Exclude context7 — it has no rollback and taints the settings file.
	_, err := RunInstall(
		[]string{"--agent", "opencode", "--component", "engram"},
		detection,
	)
	if err == nil {
		t.Fatalf("RunInstall() expected error")
	}

	if !strings.Contains(err.Error(), "execute install pipeline") {
		t.Fatalf("RunInstall() error = %v", err)
	}

	after, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if string(after) != string(before) {
		t.Fatalf("macOS settings changed after rollback\nafter=%s\nbefore=%s", after, before)
	}
}

// --- Skip-when-installed and Go auto-install tests ---

func TestRunInstallEngramSkipsInstallWhenAlreadyOnPath(t *testing.T) {
	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	// Simulate engram already installed on PATH.
	cmdLookPath = func(name string) (string, error) {
		return "/usr/local/bin/" + name, nil
	}
	recorder := &commandRecorder{}
	runCommand = recorder.record

	detection := macOSDetectionResult()
	result, err := RunInstall(
		[]string{"--agent", "opencode", "--component", "engram"},
		detection,
	)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	if !result.Verify.Ready {
		t.Fatalf("verification ready = false")
	}

	// No brew/go install commands should have been recorded — only agent install.
	for _, cmd := range recorder.get() {
		if strings.Contains(cmd, "brew install engram") || (strings.Contains(cmd, "go install") && strings.Contains(cmd, "engram")) {
			t.Fatalf("expected engram install to be skipped, but got command: %s", cmd)
		}
	}
}

func TestRunInstallEngramAttemptsOpenCodeSetupWhenBinaryPresent(t *testing.T) {
	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	cmdLookPath = func(name string) (string, error) {
		return "/usr/local/bin/" + name, nil
	}
	recorder := &commandRecorder{}
	runCommand = recorder.record

	result, err := RunInstall(
		[]string{"--agent", "opencode", "--component", "engram"},
		macOSDetectionResult(),
	)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	if !result.Verify.Ready {
		t.Fatalf("verification ready = false")
	}

	commands := recorder.get()
	foundSetup := false
	for _, cmd := range commands {
		if strings.Contains(cmd, "engram setup opencode") {
			foundSetup = true
			break
		}
	}
	if !foundSetup {
		t.Fatalf("expected engram setup command, got commands: %v", commands)
	}
}

func TestRunInstallEngramFallsBackToInjectWhenSetupFails(t *testing.T) {
	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	cmdLookPath = func(name string) (string, error) {
		return "/usr/local/bin/" + name, nil
	}
	runCommand = func(name string, args ...string) error {
		if name == "engram" && len(args) == 2 && args[0] == "setup" && args[1] == "opencode" {
			return errors.New("setup failed")
		}
		return nil
	}

	result, err := RunInstall(
		[]string{"--agent", "opencode", "--component", "engram"},
		macOSDetectionResult(),
	)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	if !result.Verify.Ready {
		t.Fatalf("verification ready = false")
	}

	configPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("expected fallback inject to create %q: %v", configPath, err)
	}
}

func TestRunInstallEngramSetupStrictFailsWhenSetupFails(t *testing.T) {
	t.Setenv("GENTLE_AI_ENGRAM_SETUP_STRICT", "1")

	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	origUserHomeDirFn := backup.UserHomeDirFn
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
		backup.UserHomeDirFn = origUserHomeDirFn
	})
	// Override restore path validation to accept test temp dirs.
	backup.UserHomeDirFn = func() (string, error) { return home, nil }

	osUserHomeDir = func() (string, error) { return home, nil }
	cmdLookPath = func(name string) (string, error) {
		return "/usr/local/bin/" + name, nil
	}
	runCommand = func(name string, args ...string) error {
		if name == "engram" && len(args) == 2 && args[0] == "setup" && args[1] == "opencode" {
			return errors.New("setup failed")
		}
		return nil
	}

	_, err := RunInstall(
		[]string{"--agent", "opencode", "--component", "engram"},
		macOSDetectionResult(),
	)
	if err == nil {
		t.Fatalf("RunInstall() expected error in strict setup mode")
	}
	if !strings.Contains(err.Error(), "engram setup for \"opencode\"") {
		t.Fatalf("RunInstall() error = %v, want setup error", err)
	}
}

func TestRunInstallEngramDefaultModeAttemptsClaudeSetup(t *testing.T) {
	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	cmdLookPath = func(name string) (string, error) {
		return "/usr/local/bin/" + name, nil
	}
	recorder := &commandRecorder{}
	runCommand = recorder.record

	result, err := RunInstall(
		[]string{"--agent", "claude-code", "--component", "engram"},
		macOSDetectionResult(),
	)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	if !result.Verify.Ready {
		t.Fatalf("verification ready = false")
	}

	commands := recorder.get()
	foundSetup := false
	for _, cmd := range commands {
		if strings.Contains(cmd, "engram setup claude-code") {
			foundSetup = true
			break
		}
	}
	if !foundSetup {
		t.Fatalf("expected default setup mode to attempt claude-code setup, got commands: %v", commands)
	}
}

func TestRunInstallAntigravityInitializesCLISettingsAfterEngramSetup(t *testing.T) {
	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	cmdLookPath = func(name string) (string, error) {
		return "/usr/local/bin/" + name, nil
	}
	runCommand = func(name string, args ...string) error {
		if name == "engram" && len(args) == 2 && args[0] == "setup" && args[1] == "gemini-cli" {
			settingsPath := filepath.Join(home, ".gemini", "settings.json")
			if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
				return err
			}
			return os.WriteFile(settingsPath, []byte("{\"theme\":\"dark\"}\n"), 0o644)
		}
		return nil
	}

	// This test targets antigravity settings initialization after engram
	// setup, not agent install behavior, so simulate Antigravity as already
	// installed (its Detect looks for ~/.gemini/antigravity) — otherwise
	// gentle-ai correctly refuses to proceed for an undetected agent.
	if err := os.MkdirAll(filepath.Join(home, ".gemini", "antigravity"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.gemini/antigravity): %v", err)
	}

	result, err := RunInstall(
		[]string{"--agent", "antigravity", "--component", "engram", "--component", "context7", "--component", "permissions"},
		macOSDetectionResult(),
	)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	if !result.Verify.Ready {
		t.Fatalf("verification ready = false")
	}

	settingsPath := filepath.Join(home, ".gemini", "antigravity-cli", "settings.json")
	got, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", settingsPath, err)
	}
	if string(got) != "{}\n" {
		t.Fatalf("antigravity settings = %q, want initialized empty settings", got)
	}
}

func TestRunInstallDeduplicatesSharedEngramSetupSlugs(t *testing.T) {
	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	cmdLookPath = func(name string) (string, error) {
		return "/usr/local/bin/" + name, nil
	}

	recorder := &commandRecorder{}
	runCommand = func(name string, args ...string) error {
		if err := recorder.record(name, args...); err != nil {
			return err
		}
		if name == "engram" && len(args) == 2 && args[0] == "setup" && args[1] == "gemini-cli" {
			settingsPath := filepath.Join(home, ".gemini", "settings.json")
			if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
				return err
			}
			return os.WriteFile(settingsPath, []byte("{\"theme\":\"dark\"}\n"), 0o644)
		}
		return nil
	}

	// This test targets shared-slug engram setup dedup, not agent install
	// behavior, so simulate Antigravity as already installed (its Detect
	// looks for ~/.gemini/antigravity) — otherwise gentle-ai correctly
	// refuses to proceed for an undetected agent.
	if err := os.MkdirAll(filepath.Join(home, ".gemini", "antigravity"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.gemini/antigravity): %v", err)
	}

	result, err := RunInstall(
		[]string{"--agent", "gemini-cli", "--agent", "antigravity", "--component", "engram", "--component", "context7", "--component", "permissions"},
		macOSDetectionResult(),
	)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	if !result.Verify.Ready {
		t.Fatalf("verification ready = false")
	}

	var setupCount int
	for _, cmd := range recorder.get() {
		if strings.Contains(cmd, "engram setup gemini-cli") {
			setupCount++
		}
	}
	if setupCount != 1 {
		t.Fatalf("engram setup gemini-cli count = %d, want 1", setupCount)
	}
}

func TestRunInstallGGASkipsInstallWhenAlreadyOnPath(t *testing.T) {
	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	cmdLookPath = func(name string) (string, error) {
		return "/usr/local/bin/" + name, nil
	}
	recorder := &commandRecorder{}
	runCommand = recorder.record

	detection := macOSDetectionResult()
	result, err := RunInstall(
		[]string{"--agent", "opencode", "--component", "gga"},
		detection,
	)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	if !result.Verify.Ready {
		t.Fatalf("verification ready = false")
	}

	// No brew/git clone commands for GGA should have been recorded.
	for _, cmd := range recorder.get() {
		if strings.Contains(cmd, "gga") || strings.Contains(cmd, "gentleman-guardian-angel") {
			t.Fatalf("expected gga install to be skipped, but got command: %s", cmd)
		}
	}

	prModePath := filepath.Join(home, ".local", "share", "gga", "lib", "pr_mode.sh")
	content, err := os.ReadFile(prModePath)
	if err != nil {
		t.Fatalf("expected gga runtime asset at %q: %v", prModePath, err)
	}
	if !strings.Contains(string(content), "detect_base_branch") {
		t.Fatalf("expected pr_mode.sh to contain detect_base_branch")
	}
}

func TestRunInstallGGALinuxIncludesTempCleanupBeforeClone(t *testing.T) {
	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	cmdLookPath = func(name string) (string, error) {
		if name == "gga" {
			return "", exec.ErrNotFound
		}
		return "/usr/local/bin/" + name, nil
	}
	recorder := &commandRecorder{}
	runCommand = recorder.record

	result, err := RunInstall(
		[]string{"--agent", "opencode", "--component", "gga"},
		linuxDetectionResult(system.LinuxDistroUbuntu, "apt"),
	)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	if !result.Verify.Ready {
		t.Fatalf("verification ready = false")
	}

	commands := recorder.get()
	cleanupIdx := -1
	cloneIdx := -1
	for i, cmd := range commands {
		if strings.Contains(cmd, "rm -rf /tmp/gentleman-guardian-angel") {
			cleanupIdx = i
		}
		// Match the clone intent (URL + dest) instead of the full literal command,
		// so the test stays valid when extra flags like --depth/--branch are added.
		if strings.Contains(cmd, "git") && strings.Contains(cmd, "https://github.com/Gentleman-Programming/gentleman-guardian-angel.git") && strings.Contains(cmd, "/tmp/gentleman-guardian-angel") {
			cloneIdx = i
		}
	}

	for _, cmd := range commands {
		if strings.Contains(cmd, "gga install") || strings.Contains(cmd, "gga init") {
			t.Fatalf("expected global gga provisioning only, got repo-level command: %s", cmd)
		}
	}

	if cleanupIdx == -1 {
		t.Fatalf("expected cleanup command before clone, got commands: %v", commands)
	}
	if cloneIdx == -1 {
		t.Fatalf("expected clone command, got commands: %v", commands)
	}
	if cleanupIdx >= cloneIdx {
		t.Fatalf("cleanup should run before clone (cleanup=%d clone=%d)", cleanupIdx, cloneIdx)
	}
}

// TestRunInstallEngramLinuxUsesDirectDownloadNoGoRequired verifies that on Linux,
// engram is now installed via pre-built binary download — Go is NOT required.
func TestRunInstallEngramLinuxUsesDirectDownloadNoGoRequired(t *testing.T) {
	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	// Simulate: engram missing, Go also NOT available — should still succeed.
	cmdLookPath = func(string) (string, error) {
		return "", exec.ErrNotFound
	}
	recorder := &commandRecorder{}
	runCommand = recorder.record

	// Override download to succeed without hitting GitHub.
	origDownloadFn := engramDownloadFn
	engramDownloadFn = func(profile system.PlatformProfile) (string, error) {
		return "/tmp/fake-engram", nil
	}
	t.Cleanup(func() { engramDownloadFn = origDownloadFn })

	detection := linuxDetectionResult(system.LinuxDistroUbuntu, "apt")
	result, err := RunInstall(
		[]string{"--agent", "opencode", "--component", "engram"},
		detection,
	)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	if !result.Verify.Ready {
		t.Fatalf("verification ready = false")
	}

	// Neither "go install" nor "apt-get install golang" should appear.
	for _, cmd := range recorder.get() {
		if strings.Contains(cmd, "apt-get install -y golang") {
			t.Fatalf("Go should NOT be auto-installed (no longer needed for engram), got command: %s", cmd)
		}
		if strings.Contains(cmd, "go install") && strings.Contains(cmd, "engram") {
			t.Fatalf("engram should NOT be installed via go install, got command: %s", cmd)
		}
	}
}

// TestRunInstallEngramLinuxNeverInstallsGo verifies that even if Go is present,
// we never install Go as a prerequisite for engram (direct download path).
func TestRunInstallEngramLinuxNeverInstallsGo(t *testing.T) {
	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	cmdLookPath = missingBinaryLookPath
	recorder := &commandRecorder{}
	runCommand = recorder.record

	origDownloadFn := engramDownloadFn
	engramDownloadFn = func(profile system.PlatformProfile) (string, error) {
		return "/tmp/fake-engram", nil
	}
	t.Cleanup(func() { engramDownloadFn = origDownloadFn })

	detection := linuxDetectionResult(system.LinuxDistroUbuntu, "apt")
	result, err := RunInstall(
		[]string{"--agent", "opencode", "--component", "engram"},
		detection,
	)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	if !result.Verify.Ready {
		t.Fatalf("verification ready = false")
	}

	// No Go installation commands should appear.
	for _, cmd := range recorder.get() {
		if strings.Contains(cmd, "apt-get install -y golang") || strings.Contains(cmd, "apt-get install -y go") {
			t.Fatalf("Go should never be installed as engram dependency, got command: %s", cmd)
		}
	}
}

func TestRunInstallEngramBrewSkipsGoCheck(t *testing.T) {
	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	// Simulate: engram missing — brew platform, no Go or download needed.
	cmdLookPath = func(string) (string, error) {
		return "", exec.ErrNotFound
	}
	recorder := &commandRecorder{}
	runCommand = recorder.record

	detection := macOSDetectionResult()
	result, err := RunInstall(
		[]string{"--agent", "opencode", "--component", "engram"},
		detection,
	)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	if !result.Verify.Ready {
		t.Fatalf("verification ready = false")
	}

	// Should use brew install, NOT go install, and no Go auto-install.
	commands := recorder.get()
	for _, cmd := range commands {
		if strings.Contains(cmd, "golang") || strings.Contains(cmd, "apt-get") {
			t.Fatalf("brew platform should not install Go, got command: %s", cmd)
		}
		if strings.Contains(cmd, "go install") {
			t.Fatalf("brew platform should not use go install, got command: %s", cmd)
		}
	}

	foundBrew := false
	for _, cmd := range commands {
		if strings.Contains(cmd, "brew install engram") {
			foundBrew = true
		}
	}
	if !foundBrew {
		t.Fatalf("expected brew install engram, got commands: %v", commands)
	}
}

// TestRunInstallDryRunMatchesActualInstall verifies parity: every file path
// reported by the dry-run plan is actually created by the real install.
//
// Strategy:
//  1. Run with DryRun=true to obtain the resolved plan (agents + ordered components).
//  2. Derive the expected file paths from the plan using componentPaths() — the
//     same function the runtime uses for backup targets and post-apply verification.
//  3. Run the real install (same flags, same mocks, fresh temp dir).
//  4. Assert that every expected file exists on disk — no missing files.
func TestRunInstallDryRunMatchesActualInstall(t *testing.T) {
	// ── Phase 1: dry-run — resolve the plan ───────────────────────────────────
	// We do NOT need temp dir or mocks for dry-run; it never touches the FS.
	installArgs := []string{"--agent", "opencode", "--component", "permissions"}
	dryRunArgs := append([]string{"--dry-run"}, installArgs...)
	dryResult, err := RunInstall(dryRunArgs, system.DetectionResult{})
	if err != nil {
		t.Fatalf("dry-run RunInstall() error = %v", err)
	}
	if !dryResult.DryRun {
		t.Fatalf("expected DryRun=true in result, got false")
	}

	// Use a synthetic home dir for path computation — the paths are derived
	// from the resolved plan (agents + components) and will use this root.
	// We reuse the same dir for the real install so the paths are identical.
	home := t.TempDir()

	// Derive expected file paths from the dry-run plan.  componentPaths() is
	// the single source of truth that both backup and verification use.
	adapters := resolveAdapters(dryResult.Resolved.Agents)
	var expectedPaths []string
	for _, component := range dryResult.Resolved.OrderedComponents {
		expectedPaths = append(expectedPaths, componentPaths(home, dryResult.Selection, adapters, component)...)
	}
	if len(expectedPaths) == 0 {
		t.Fatal("dry-run resolved zero file paths — test is misconfigured")
	}

	// ── Phase 2: real install — apply the plan ────────────────────────────────
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	runCommand = func(string, ...string) error { return nil }
	cmdLookPath = missingBinaryLookPath

	realResult, err := RunInstall(installArgs, system.DetectionResult{})
	if err != nil {
		t.Fatalf("real RunInstall() error = %v", err)
	}
	if !realResult.Verify.Ready {
		t.Fatalf("post-apply verification not ready: %#v", realResult.Verify)
	}

	// ── Phase 3: parity assertion ─────────────────────────────────────────────
	// Every file the dry-run said would be touched must exist on disk.
	var missing []string
	for _, path := range expectedPaths {
		if _, statErr := os.Stat(path); statErr != nil {
			missing = append(missing, path)
		}
	}
	if len(missing) > 0 {
		t.Errorf("dry-run planned %d file(s) that were NOT created by the real install:", len(missing))
		for _, p := range missing {
			t.Errorf("  missing: %s", p)
		}
	}
}

func TestRunInstallDryRunMatchesActualInstallOpenCodeSDDMulti(t *testing.T) {
	installArgs := []string{"--agent", "opencode", "--component", "sdd", "--sdd-mode", "multi"}
	dryRunArgs := append([]string{"--dry-run"}, installArgs...)
	dryResult, err := RunInstall(dryRunArgs, system.DetectionResult{})
	if err != nil {
		t.Fatalf("dry-run RunInstall() error = %v", err)
	}
	if !dryResult.DryRun {
		t.Fatalf("expected DryRun=true in result, got false")
	}

	home := t.TempDir()
	adapters := resolveAdapters(dryResult.Resolved.Agents)
	var expectedPaths []string
	for _, component := range dryResult.Resolved.OrderedComponents {
		expectedPaths = append(expectedPaths, componentPaths(home, dryResult.Selection, adapters, component)...)
	}
	pluginPaths := []string{
		filepath.Join(home, ".config", "opencode", "plugins", "background-agents.ts"),
		filepath.Join(home, ".config", "opencode", "plugins", "model-variants.ts"),
		filepath.Join(home, ".config", "opencode", "plugins", "skill-registry.ts"),
	}
	for _, pluginPath := range pluginPaths {
		if !containsPath(expectedPaths, pluginPath) {
			t.Fatalf("dry-run expected paths missing multi-mode plugin %q\npaths=%v", pluginPath, expectedPaths)
		}
	}

	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	runCommand = func(string, ...string) error { return nil }
	cmdLookPath = missingBinaryLookPath

	realResult, err := RunInstall(installArgs, system.DetectionResult{})
	if err != nil {
		t.Fatalf("real RunInstall() error = %v", err)
	}
	if !realResult.Verify.Ready {
		t.Fatalf("post-apply verification not ready: %#v", realResult.Verify)
	}

	for _, path := range expectedPaths {
		if isLegacyOpenCodeBackgroundAgentsPlugin(path) {
			if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
				t.Fatalf("expected legacy OpenCode SDD plugin %q to be removed after install; stat err = %v", path, statErr)
			}
			continue
		}
		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("expected dry-run path %q to exist after install: %v", path, statErr)
		}
	}
	for _, pluginPath := range pluginPaths {
		if isLegacyOpenCodeBackgroundAgentsPlugin(pluginPath) {
			if _, statErr := os.Stat(pluginPath); !os.IsNotExist(statErr) {
				t.Fatalf("expected legacy OpenCode SDD plugin %q to be removed after install; stat err = %v", pluginPath, statErr)
			}
			continue
		}
		if _, statErr := os.Stat(pluginPath); statErr != nil {
			t.Fatalf("expected OpenCode SDD plugin %q to exist after install: %v", pluginPath, statErr)
		}
	}
}

func TestEnsureGoAvailableAfterInstallWindowsRefreshesPath(t *testing.T) {
	restoreLookPath := cmdLookPath
	restoreStat := osStat
	restoreSetenv := osSetenv
	oldPath := os.Getenv("PATH")
	oldProgramFiles := os.Getenv("ProgramFiles")
	t.Cleanup(func() {
		cmdLookPath = restoreLookPath
		osStat = restoreStat
		osSetenv = restoreSetenv
		_ = os.Setenv("PATH", oldPath)
		_ = os.Setenv("ProgramFiles", oldProgramFiles)
	})

	programFiles := `C:\Program Files`
	if err := os.Setenv("ProgramFiles", programFiles); err != nil {
		t.Fatalf("Setenv(ProgramFiles) error = %v", err)
	}
	if err := os.Setenv("PATH", `C:\Windows\System32`); err != nil {
		t.Fatalf("Setenv(PATH) error = %v", err)
	}

	cmdLookPath = func(name string) (string, error) {
		if name == "go" {
			return "", exec.ErrNotFound
		}
		return name, nil
	}
	osStat = func(name string) (os.FileInfo, error) {
		want := filepath.Join(programFiles, "Go", "bin", "go.exe")
		if name == want {
			return fakeFileInfo{name: "go.exe"}, nil
		}
		return nil, os.ErrNotExist
	}
	osSetenv = os.Setenv

	if err := ensureGoAvailableAfterInstall(system.PlatformProfile{OS: "windows", PackageManager: "winget"}); err != nil {
		t.Fatalf("ensureGoAvailableAfterInstall() error = %v", err)
	}

	updatedPath := os.Getenv("PATH")
	expectedPrefix := filepath.Join(programFiles, "Go", "bin") + string(os.PathListSeparator)
	if !strings.HasPrefix(updatedPath, expectedPrefix) {
		t.Fatalf("PATH = %q, want prefix %q", updatedPath, expectedPrefix)
	}
}

type fakeFileInfo struct{ name string }

func (f fakeFileInfo) Name() string     { return f.name }
func (fakeFileInfo) Size() int64        { return 0 }
func (fakeFileInfo) Mode() os.FileMode  { return 0 }
func (fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (fakeFileInfo) IsDir() bool        { return false }
func (fakeFileInfo) Sys() any           { return nil }

// TestRunInstallUpgradeIdempotency verifies that running install twice with the
// same configuration does NOT duplicate any content.  The second run must be a
// no-op or a clean update — never an append of already-present sections or MCP
// entries.
func TestRunInstallUpgradeIdempotency(t *testing.T) {
	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	runCommand = func(string, ...string) error { return nil }
	// Simulate all binaries already on PATH so install steps are skipped and
	// the test only exercises injection idempotency.
	cmdLookPath = func(name string) (string, error) {
		return "/usr/local/bin/" + name, nil
	}

	args := []string{
		"--agent", "claude-code",
		"--component", "sdd",
		"--component", "engram",
		"--component", "persona",
	}

	// --- Run 1 ---
	result1, err := RunInstall(args, system.DetectionResult{})
	if err != nil {
		t.Fatalf("RunInstall() run 1 error = %v", err)
	}
	if !result1.Verify.Ready {
		t.Fatalf("run 1: verify.Ready = false, report = %#v", result1.Verify)
	}

	// Capture all relevant output files after the first run.
	claudeMDPath := filepath.Join(home, ".claude", "CLAUDE.md")
	engramMCPPath := filepath.Join(home, ".claude.json")

	claudeMDAfterRun1, err := os.ReadFile(claudeMDPath)
	if err != nil {
		t.Fatalf("run 1: ReadFile(%q) error = %v", claudeMDPath, err)
	}
	engramMCPAfterRun1, err := os.ReadFile(engramMCPPath)
	if err != nil {
		t.Fatalf("run 1: ReadFile(%q) error = %v", engramMCPPath, err)
	}

	// --- Run 2 (same flags) ---
	result2, err := RunInstall(args, system.DetectionResult{})
	if err != nil {
		t.Fatalf("RunInstall() run 2 error = %v", err)
	}
	if !result2.Verify.Ready {
		t.Fatalf("run 2: verify.Ready = false, report = %#v", result2.Verify)
	}

	// Capture output files after the second run.
	claudeMDAfterRun2, err := os.ReadFile(claudeMDPath)
	if err != nil {
		t.Fatalf("run 2: ReadFile(%q) error = %v", claudeMDPath, err)
	}
	engramMCPAfterRun2, err := os.ReadFile(engramMCPPath)
	if err != nil {
		t.Fatalf("run 2: ReadFile(%q) error = %v", engramMCPPath, err)
	}

	// --- Assertions ---

	// 1. File bytes must be identical between the two runs.
	if string(claudeMDAfterRun1) != string(claudeMDAfterRun2) {
		t.Errorf("CLAUDE.md changed between run 1 and run 2 (idempotency violation):\n--- run1 ---\n%s\n--- run2 ---\n%s",
			claudeMDAfterRun1, claudeMDAfterRun2)
	}
	if string(engramMCPAfterRun1) != string(engramMCPAfterRun2) {
		t.Errorf("engram MCP config changed between run 1 and run 2 (idempotency violation):\n--- run1 ---\n%s\n--- run2 ---\n%s",
			engramMCPAfterRun1, engramMCPAfterRun2)
	}

	// 2. No duplicate "## Agent Teams Orchestrator" headings in CLAUDE.md.
	content := string(claudeMDAfterRun2)
	orchestratorCount := strings.Count(content, "## Agent Teams Orchestrator")
	if orchestratorCount > 1 {
		t.Errorf("CLAUDE.md contains %d occurrences of '## Agent Teams Orchestrator', want at most 1:\n%s",
			orchestratorCount, content)
	}

	// 3. No duplicate gentle-ai marker blocks — each section's open marker
	// must appear exactly once.
	for _, sectionID := range []string{"sdd-orchestrator", "engram-protocol"} {
		openMarker := "<!-- gentle-ai:" + sectionID + " -->"
		count := strings.Count(content, openMarker)
		if count != 1 {
			t.Errorf("CLAUDE.md contains %d occurrences of marker %q, want exactly 1:\n%s",
				count, openMarker, content)
		}
	}

	// 4. Engram MCP JSON must not contain duplicate keys.
	// A simple structural check: "command" key should appear exactly once.
	engramJSON := string(engramMCPAfterRun2)
	commandCount := strings.Count(engramJSON, `"command"`)
	if commandCount != 1 {
		t.Errorf("engram MCP JSON contains %d occurrences of \"command\", want exactly 1:\n%s",
			commandCount, engramJSON)
	}
}

// --- Custom preset integration tests ---

func TestRunInstallCustomPresetNoComponentsIsNoop(t *testing.T) {
	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	runCommand = func(string, ...string) error { return nil }
	cmdLookPath = missingBinaryLookPath

	result, err := RunInstall(
		[]string{"--agent", "claude-code", "--preset", "custom"},
		system.DetectionResult{},
	)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	// Custom preset with no components should resolve to zero ordered components.
	if len(result.Resolved.OrderedComponents) != 0 {
		t.Fatalf("expected 0 ordered components for custom preset, got %d: %v",
			len(result.Resolved.OrderedComponents), result.Resolved.OrderedComponents)
	}
}

func TestRunInstallCustomPresetExplicitSkillsFlagPopulatesSelection(t *testing.T) {
	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	runCommand = func(string, ...string) error { return nil }
	cmdLookPath = func(name string) (string, error) {
		return "/usr/local/bin/" + name, nil
	}

	result, err := RunInstall(
		[]string{
			"--agent", "claude-code",
			"--preset", "custom",
			"--component", "skills",
			"--skills", "go-testing,branch-pr",
		},
		system.DetectionResult{},
	)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	if !result.Verify.Ready {
		t.Fatalf("verification ready = false, report = %#v", result.Verify)
	}

	// Verify the explicitly requested skills were installed.
	goTestingPath := filepath.Join(home, ".claude", "skills", "go-testing", "SKILL.md")
	branchPRPath := filepath.Join(home, ".claude", "skills", "branch-pr", "SKILL.md")
	if _, err := os.Stat(goTestingPath); err != nil {
		t.Fatalf("expected go-testing skill file %q: %v", goTestingPath, err)
	}
	if _, err := os.Stat(branchPRPath); err != nil {
		t.Fatalf("expected branch-pr skill file %q: %v", branchPRPath, err)
	}

	// Note: the graph defines skills → sdd → engram as a hard dependency chain.
	// Selecting --component skills auto-resolves sdd (and engram) as dependencies.
	// The SDD component installs its own 10 SDD+orchestration skills during injection,
	// regardless of the --skills flag. So sdd-init and other SDD skills ARE installed.
	sddInitPath := filepath.Join(home, ".claude", "skills", "sdd-init", "SKILL.md")
	if _, err := os.Stat(sddInitPath); err != nil {
		t.Fatalf("sdd-init skill should be installed (sdd is auto-resolved as dep of skills): %v", err)
	}

	// The --skills flag controls what the skills COMPONENT adds on top of SDD skills.
	// Total = 10 SDD skills + 2 explicit skills = 12 SKILL.md files.
	skillsDir := filepath.Join(home, ".claude", "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		t.Fatalf("ReadDir(%q) error = %v", skillsDir, err)
	}
	// Count SKILL.md files across all skill subdirectories.
	var skillCount int
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillMD := filepath.Join(skillsDir, entry.Name(), "SKILL.md")
		if _, statErr := os.Stat(skillMD); statErr == nil {
			skillCount++
		}
	}
	// 12 SDD skills + 2 explicit skills = 14. _shared is support-only.
	if skillCount != 14 {
		t.Fatalf("expected 14 skill files (12 SDD + 2 explicit), got %d", skillCount)
	}
}

func TestRunInstallCustomPresetSkillsNoFlagInstallsNothing(t *testing.T) {
	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	runCommand = func(string, ...string) error { return nil }
	cmdLookPath = func(name string) (string, error) {
		return "/usr/local/bin/" + name, nil
	}

	result, err := RunInstall(
		[]string{
			"--agent", "claude-code",
			"--preset", "custom",
			"--component", "skills",
		},
		system.DetectionResult{},
	)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	if !result.Verify.Ready {
		t.Fatalf("verification ready = false, report = %#v", result.Verify)
	}

	// The graph defines skills → sdd → engram as hard dependencies.
	// Selecting --component skills auto-resolves sdd (and engram).
	// The SDD component ALWAYS installs its 10 SDD+orchestration skills during injection.
	// Without --skills flag, selectedSkillIDs() returns nil for custom preset,
	// so the skills COMPONENT is a no-op — but the sdd DEPENDENCY still runs and
	// installs its 10 skills.
	skillsDir := filepath.Join(home, ".claude", "skills")
	// Count SKILL.md files (one per skill, excluding _shared and other non-skill dirs).
	var skillCount int
	if entries, readErr := os.ReadDir(skillsDir); readErr == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			skillMD := filepath.Join(skillsDir, entry.Name(), "SKILL.md")
			if _, statErr := os.Stat(skillMD); statErr == nil {
				skillCount++
			}
		}
	}
	// Expect 12 files: 11 SDD phases + judgment-day. _shared is support-only.
	if skillCount != 12 {
		t.Fatalf("expected 12 SDD skill files installed by the sdd dependency, got %d", skillCount)
	}
}

func TestRunInstallCustomPresetDryRunShowsCustomPreset(t *testing.T) {
	result, err := RunInstall(
		[]string{"--agent", "claude-code", "--preset", "custom", "--dry-run"},
		system.DetectionResult{},
	)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	if !result.DryRun {
		t.Fatalf("expected DryRun=true")
	}

	if result.Selection.Preset != model.PresetCustom {
		t.Fatalf("preset = %q, want %q", result.Selection.Preset, model.PresetCustom)
	}

	// Zero components when no --component flags provided.
	if len(result.Resolved.OrderedComponents) != 0 {
		t.Fatalf("expected 0 ordered components, got %d", len(result.Resolved.OrderedComponents))
	}

	output := RenderDryRun(result)
	if !strings.Contains(output, "custom") {
		t.Fatalf("dry-run output missing 'custom' preset name:\n%s", output)
	}
}

func TestRunInstallCustomPresetExplicitComponentsResolveCorrectly(t *testing.T) {
	result, err := RunInstall(
		[]string{
			"--agent", "claude-code",
			"--preset", "custom",
			"--component", "engram",
			"--component", "sdd",
			"--component", "permissions",
			"--dry-run",
		},
		system.DetectionResult{},
	)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	// Should have exactly the 3 explicit components (sdd depends on engram which is already selected).
	if len(result.Resolved.OrderedComponents) != 3 {
		t.Fatalf("expected 3 ordered components, got %d: %v",
			len(result.Resolved.OrderedComponents), result.Resolved.OrderedComponents)
	}

	// Verify persona, skills, context7, gga are NOT in the plan.
	for _, c := range result.Resolved.OrderedComponents {
		switch c {
		case model.ComponentPersona, model.ComponentSkills, model.ComponentContext7, model.ComponentGGA:
			t.Fatalf("unexpected component %q in custom preset plan", c)
		}
	}
}

// TestOpenCodePersonaBeforeSDDPreservesAllSections is the regression test for
// issue #121: on StrategyFileReplace agents, if Persona ran after SDD it would
// overwrite the entire AGENTS.md, destroying the SDD orchestrator section.
//
// This test exercises the full install pipeline for OpenCode with Persona +
// Engram + SDD selected together and verifies that the final AGENTS.md
// contains all three sections with no duplicates.
func TestOpenCodePersonaBeforeSDDPreservesAllSections(t *testing.T) {
	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	runCommand = func(string, ...string) error { return nil }
	cmdLookPath = missingBinaryLookPath

	_, err := RunInstall(
		[]string{
			"--agent", "opencode",
			"--component", "persona",
			"--component", "engram",
			"--component", "sdd",
			"--persona", "gentleman",
		},
		system.DetectionResult{},
	)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	agentsMD := filepath.Join(home, ".config", "opencode", "AGENTS.md")
	content, err := os.ReadFile(agentsMD)
	if err != nil {
		t.Fatalf("ReadFile(AGENTS.md) error = %v", err)
	}
	text := string(content)

	// Persona content must be present
	if !strings.Contains(text, "Senior Architect") {
		t.Error("AGENTS.md missing Gentleman persona content (persona not written)")
	}

	// For OpenCode, the SDD orchestrator goes into opencode.json (agent overlay),
	// NOT AGENTS.md. AGENTS.md only contains persona and engram sections.
	// The issue #121 regression was that Persona would overwrite AGENTS.md
	// AFTER engram had already injected the engram-protocol marker, destroying
	// the engram section. We verify persona + engram coexist.

	// Engram protocol section must be present
	if !strings.Contains(text, "<!-- gentle-ai:engram-protocol -->") {
		t.Error("AGENTS.md missing engram-protocol open marker (issue #121 regression: persona may have overwritten engram section)")
	}
	if !strings.Contains(text, "<!-- /gentle-ai:engram-protocol -->") {
		t.Error("AGENTS.md missing engram-protocol close marker")
	}

	// Engram section must not be duplicated
	marker := "<!-- gentle-ai:engram-protocol -->"
	if count := strings.Count(text, marker); count != 1 {
		t.Errorf("AGENTS.md contains %d occurrences of %q, want exactly 1 (no duplicates)", count, marker)
	}

	// AGENTS.md must NOT have sdd-orchestrator markers — OpenCode uses opencode.json overlay
	if strings.Contains(text, "<!-- gentle-ai:sdd-orchestrator -->") {
		t.Error("AGENTS.md should NOT have sdd-orchestrator marker — OpenCode uses opencode.json agent overlay")
	}

	// SDD orchestrator for OpenCode lives in opencode.json agent overlay under
	// the canonical gentle-orchestrator key. Legacy sdd-orchestrator should be
	// migrated away during injection.
	opencodeJSON := filepath.Join(home, ".config", "opencode", "opencode.json")
	jsonContent, err := os.ReadFile(opencodeJSON)
	if err != nil {
		t.Fatalf("ReadFile(opencode.json) error = %v", err)
	}
	jsonText := string(jsonContent)
	if !strings.Contains(jsonText, "gentle-orchestrator") {
		t.Error("opencode.json missing gentle-orchestrator agent entry (SDD not injected)")
	}
	if strings.Contains(jsonText, `"sdd-orchestrator"`) {
		t.Error("opencode.json should not contain legacy sdd-orchestrator agent entry")
	}
}
func TestRunInstallKimiBootstrapsHub(t *testing.T) {
	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
	})
	osUserHomeDir = func() (string, error) { return home, nil }
	runCommand = func(string, ...string) error { return nil }
	cmdLookPath = missingBinaryLookPath
	restoreInstallcmdLookPath := installcmd.OverrideLookPath(func(name string) (string, error) {
		if name == "uv" {
			return "/usr/bin/uv", nil
		}
		return "", exec.ErrNotFound
	})
	t.Cleanup(restoreInstallcmdLookPath)

	// This test targets kimiSystemPromptHubStep bootstrap content, not agent
	// install behavior, so simulate Kimi as already installed — otherwise
	// gentle-ai correctly refuses to proceed for an undetected runtime.
	restoreKimiLookPath := kimi.LookPathOverride
	kimi.LookPathOverride = func(string) (string, error) { return "/usr/local/bin/kimi", nil }
	t.Cleanup(func() { kimi.LookPathOverride = restoreKimiLookPath })

	// Install Kimi with minimalist component (e.g., permissions only, NO persona).
	_, err := RunInstall(
		[]string{"--agent", "kimi", "--component", "permissions"},
		system.DetectionResult{},
	)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	// Verify that KIMI.md was created in the agent's config dir.
	hubPath := filepath.Join(home, ".kimi", "KIMI.md")
	if _, err := os.Stat(hubPath); err != nil {
		t.Fatalf("expected Kimi prompt hub file %q to be bootstrapped: %v", hubPath, err)
	}

	// Verify content includes sub-modules (basic check).
	content, err := os.ReadFile(hubPath)
	if err != nil {
		t.Fatalf("failed to read bootstrapped hub: %v", err)
	}
	if !strings.Contains(string(content), "{% include \"persona.md\" ignore missing %}") {
		t.Errorf("bootstrapped hub missing modular include: %s", string(content))
	}
}

func TestRunInstallKimiAlreadyInstalledDoesNotRequireUV(t *testing.T) {
	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	cmdLookPath = missingBinaryLookPath
	recorder := &commandRecorder{}
	runCommand = recorder.record

	originalKimiLookPath := kimi.LookPathOverride
	kimi.LookPathOverride = func(name string) (string, error) {
		if name == "kimi" {
			return "/usr/local/bin/kimi", nil
		}
		return "", exec.ErrNotFound
	}
	t.Cleanup(func() { kimi.LookPathOverride = originalKimiLookPath })

	restoreInstallcmdLookPath := installcmd.OverrideLookPath(func(name string) (string, error) {
		if name == "uv" {
			return "", exec.ErrNotFound
		}
		return "/usr/bin/" + name, nil
	})
	t.Cleanup(restoreInstallcmdLookPath)

	result, err := RunInstall(
		[]string{"--agent", "kimi", "--component", "permissions"},
		macOSDetectionResult(),
	)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	if !result.Verify.Ready {
		t.Fatalf("verification ready = false, report = %#v", result.Verify)
	}

	hubPath := filepath.Join(home, ".kimi", "KIMI.md")
	if _, err := os.Stat(hubPath); err != nil {
		t.Fatalf("expected Kimi prompt hub file %q to be bootstrapped: %v", hubPath, err)
	}

	if got := recorder.get(); len(got) != 0 {
		t.Fatalf("expected no install commands when Kimi is already installed, got: %v", got)
	}
}

// TestRunInstallWorkspaceScopeVerification verifies the user-visible 'install --scope=workspace'
// behavior from issue #785. It ensures that when installing with workspace scope:
// 1. Verification files are written to the workspace directory, NOT the home directory.
// 2. Post-apply verification succeeds because it checks the workspace skill paths.
func TestRunInstallWorkspaceScopeVerification(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()

	originalCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current wd: %v", err)
	}

	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
		if err := os.Chdir(originalCwd); err != nil {
			t.Errorf("failed to restore working directory: %v", err)
		}
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	runCommand = func(string, ...string) error { return nil }
	cmdLookPath = func(name string) (string, error) {
		return "/usr/local/bin/" + name, nil
	}

	if err := os.Chdir(workspace); err != nil {
		t.Fatalf("failed to change working directory to temp workspace: %v", err)
	}

	// Run install with workspace scope, installing Claude Code agent and skills component
	args := []string{
		"--scope", "workspace",
		"--agent", "claude-code",
		"--component", "skills",
		"--preset", "custom",
		"--skill", "go-testing,branch-pr",
	}

	result, err := RunInstall(args, system.DetectionResult{})
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	if !result.Verify.Ready {
		t.Fatalf("post-apply verification failed, report = %#v", result.Verify)
	}

	// Assert that skill files were written to the workspace directory.
	expectedWorkspaceSkillFile := filepath.Join(workspace, ".claude", "skills", "go-testing", "SKILL.md")
	if _, err := os.Stat(expectedWorkspaceSkillFile); err != nil {
		t.Errorf("expected skill file in workspace %q, but was missing: %v", expectedWorkspaceSkillFile, err)
	}

	// Assert that no skill files were written to the home directory.
	unexpectedHomeSkillFile := filepath.Join(home, ".claude", "skills", "go-testing", "SKILL.md")
	if _, err := os.Stat(unexpectedHomeSkillFile); err == nil {
		t.Errorf("unexpected skill file found in home directory: %q", unexpectedHomeSkillFile)
	}
}

// TestRunInstall_Context7WorkspaceScope_PersistsToWorkspace verifies that executing
// a real workspace operation with --scope workspace and Context7 component:
//  1. Returns a successful result with verification ready.
//  2. Persists Context7 MCP configuration into <project-root>/.mcp.json, the file
//     Claude Code loads project-scoped MCP servers from (issue #2213).
//  3. Leaves the user's home directory settings untouched.
func TestRunInstall_Context7WorkspaceScope_PersistsToWorkspace(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()

	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	originalCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}

	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
		if err := os.Chdir(originalCwd); err != nil {
			t.Errorf("failed to restore working directory: %v", err)
		}
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	runCommand = func(string, ...string) error { return nil }
	cmdLookPath = func(name string) (string, error) {
		return "/usr/local/bin/" + name, nil
	}

	if err := os.Chdir(workspace); err != nil {
		t.Fatalf("failed to change working directory to temp workspace: %v", err)
	}

	args := []string{
		"--scope", "workspace",
		"--agent", "claude-code",
		"--component", "context7",
		"--preset", "custom",
	}

	result, err := RunInstall(args, system.DetectionResult{})
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	if !result.Verify.Ready {
		t.Fatalf("post-apply verification failed for Context7 workspace scope: %#v", result.Verify)
	}

	// Context7 MCP configuration must persist to <project-root>/.mcp.json, the
	// file Claude Code loads project-scoped MCP servers from (issue #2213).
	workspaceMCPFile := filepath.Join(workspace, ".mcp.json")
	assertFileContains(t, workspaceMCPFile, "context7")

	// The legacy .claude/settings.json key is inert for MCP discovery and must
	// not carry the managed context7 entry after install.
	if settingsRaw, err := os.ReadFile(filepath.Join(workspace, ".claude", "settings.json")); err == nil {
		if strings.Contains(string(settingsRaw), `"mcpServers"`) {
			t.Errorf("workspace .claude/settings.json must not carry mcpServers; got %s", settingsRaw)
		}
	}

	// Assert that no Context7 configuration was written to home directory settings.
	homeSettingsFile := filepath.Join(home, ".claude", "settings.json")
	if _, err := os.Stat(homeSettingsFile); err == nil {
		content, _ := os.ReadFile(homeSettingsFile)
		if strings.Contains(string(content), "context7") {
			t.Errorf("unexpected Context7 MCP config found in home settings file: %q", homeSettingsFile)
		}
	}
}

// TestRunInstall_Context7WorkspaceScope_FailurePath verifies that executing
// Context7 workspace installation when workspace target is unwriteable fails gracefully,
// returning an error and reporting verification failure.
func TestRunInstall_Context7WorkspaceScope_FailurePath(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()

	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	originalCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}

	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
		if err := os.Chdir(originalCwd); err != nil {
			t.Errorf("failed to restore working directory: %v", err)
		}
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	runCommand = func(string, ...string) error { return nil }
	cmdLookPath = func(name string) (string, error) {
		return "/usr/local/bin/" + name, nil
	}

	if err := os.Chdir(workspace); err != nil {
		t.Fatalf("failed to change working directory to temp workspace: %v", err)
	}

	// Block the <project-root>/.mcp.json write by making it a directory, so the
	// atomic file write fails. The primary workspace-scope target is .mcp.json
	// (issue #2213), not .claude/settings.json.
	blockingPath := filepath.Join(workspace, ".mcp.json")
	if err := os.Mkdir(blockingPath, 0o755); err != nil {
		t.Fatalf("failed to create blocking .mcp.json directory: %v", err)
	}

	args := []string{
		"--scope", "workspace",
		"--agent", "claude-code",
		"--component", "context7",
		"--preset", "custom",
	}

	_, err = RunInstall(args, system.DetectionResult{})
	if err == nil {
		t.Fatalf("RunInstall() with unwriteable workspace target expected error, got nil")
	}
}
