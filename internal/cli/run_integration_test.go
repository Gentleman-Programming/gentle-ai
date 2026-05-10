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

	"github.com/gentleman-programming/gentle-ai/internal/agents/kimi"
	"github.com/gentleman-programming/gentle-ai/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/internal/backup"
	"github.com/gentleman-programming/gentle-ai/internal/installcmd"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/system"
)

// missingBinaryLookPath simulates all installable binaries (engram, gga) as
// missing. Go availability is no longer required for engram installation
// (pre-built binaries are downloaded directly from GitHub Releases).
func missingBinaryLookPath(name string) (string, error) {
	return "", exec.ErrNotFound
}

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

func TestRunInstallLinuxAgentInstallResolvesGoInstallCommand(t *testing.T) {
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

	// Set the agent adapter's lookPath to simulate missing opencode
	opencodeAdapterLookPath := opencode.LookPathOverride
	opencode.LookPathOverride = missingBinaryLookPath
	t.Cleanup(func() {
		opencode.LookPathOverride = opencodeAdapterLookPath
	})

	detection := linuxDetectionResult(system.LinuxDistroUbuntu, "apt")
	_, err := RunInstall(
		[]string{"--agent", "opencode", "--component", "permissions"},
		detection,
	)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	// OpenCode on Ubuntu should resolve via npm install (official method from opencode.ai).
	commands := recorder.get()
	foundNpmInstall := false
	for _, cmd := range commands {
		if strings.Contains(cmd, "sudo npm install -g opencode-ai") {
			foundNpmInstall = true
			break
		}
	}
	if !foundNpmInstall {
		t.Fatalf("expected npm install command for opencode agent, got commands: %v", commands)
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

func TestRunInstallAntigravityCopiesGeminiSettingsAfterEngramSetup(t *testing.T) {
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

	settingsPath := filepath.Join(home, ".gemini", "antigravity", "settings.json")
	got, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", settingsPath, err)
	}
	if string(got) != "{\"theme\":\"dark\"}\n" {
		t.Fatalf("antigravity settings = %q, want copied Gemini settings", got)
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
		if strings.Contains(cmd, "git clone https://github.com/Gentleman-Programming/gentleman-guardian-angel.git /tmp/gentleman-guardian-angel") {
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
	pluginPath := filepath.Join(home, ".config", "opencode", "plugins", "background-agents.ts")
	if !containsPath(expectedPaths, pluginPath) {
		t.Fatalf("dry-run expected paths missing multi-mode plugin %q\npaths=%v", pluginPath, expectedPaths)
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
		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("expected dry-run path %q to exist after install: %v", path, statErr)
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
	engramMCPPath := filepath.Join(home, ".claude", "mcp", "engram.json")

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
	// 11 SDD skills (from sdd dep, includes sdd-onboard) + 2 explicit skills (go-testing, branch-pr) + 1 _shared/SKILL.md = 14
	if skillCount != 14 {
		t.Fatalf("expected 14 skill files (11 SDD + 2 explicit + 1 _shared), got %d", skillCount)
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
	// Expect exactly 12 SKILL.md files: 10 SDD phases + judgment-day (from SDD dependency) + 1 _shared/SKILL.md.
	// The skills component itself adds 0 (no --skills flag, SkillsForPreset(custom) = nil).
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

func TestRunInstallKimiMissingUVFailsBeforeExecutingInstallCommands(t *testing.T) {
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

	restoreInstallcmdLookPath := installcmd.OverrideLookPath(func(name string) (string, error) {
		if name == "uv" {
			return "", exec.ErrNotFound
		}
		return "/usr/bin/" + name, nil
	})
	t.Cleanup(restoreInstallcmdLookPath)

	_, err := RunInstall(
		[]string{"--agent", "kimi", "--component", "permissions"},
		macOSDetectionResult(),
	)
	if err == nil {
		t.Fatal("RunInstall() expected error when Kimi uv preflight fails")
	}

	if !strings.Contains(err.Error(), "preflight for agent \"kimi\"") || !strings.Contains(err.Error(), "uv") {
		t.Fatalf("RunInstall() error = %q, expected Kimi uv preflight error", err.Error())
	}

	if got := recorder.get(); len(got) != 0 {
		t.Fatalf("expected no install commands to execute before Kimi preflight failure, got: %v", got)
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
