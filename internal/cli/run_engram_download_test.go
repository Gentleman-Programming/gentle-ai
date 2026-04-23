package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/components/engram"
	"github.com/gentleman-programming/gentle-ai/internal/system"
)

// TestRunInstallLinuxEngramUsesDownloadNotGoInstall verifies that after the fix,
// Linux engram installation does NOT use "go install" but instead calls
// DownloadLatestBinary (i.e. no "go install" in recorder.get()).
func TestRunInstallLinuxEngramUsesDownloadNotGoInstall(t *testing.T) {
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

	// Override the engram download function to succeed without hitting GitHub.
	origDownloadFn := engramDownloadFn
	engramDownloadFn = func(profile system.PlatformProfile) (string, error) {
		// Simulate a successful binary download to a temp path.
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

	// Must NOT have called "go install" for engram.
	for _, cmd := range recorder.get() {
		if strings.Contains(cmd, "go install") && strings.Contains(cmd, "engram") {
			t.Fatalf("Linux engram install should NOT use go install, got command: %s", cmd)
		}
	}
}

// TestRunInstallEngramDownloadAddsBinDirToPath verifies that after downloading
// the engram binary, its directory is prepended to PATH so that subsequent
// commands (engram setup, resolveEngramCommand) can find it.
func TestRunInstallEngramDownloadAddsBinDirToPath(t *testing.T) {
	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	restorePath := os.Getenv("PATH")
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
		os.Setenv("PATH", restorePath)
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	cmdLookPath = missingBinaryLookPath
	recorder := &commandRecorder{}
	runCommand = recorder.record

	fakeBinDir := filepath.Join(home, "engram-bin")
	os.MkdirAll(fakeBinDir, 0o755)
	fakeBinaryPath := filepath.Join(fakeBinDir, "engram")

	origDownloadFn := engramDownloadFn
	engramDownloadFn = func(profile system.PlatformProfile) (string, error) {
		return fakeBinaryPath, nil
	}
	t.Cleanup(func() { engramDownloadFn = origDownloadFn })

	detection := linuxDetectionResult(system.LinuxDistroUbuntu, "apt")
	_, err := RunInstall(
		[]string{"--agent", "opencode", "--component", "engram"},
		detection,
	)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	currentPath := os.Getenv("PATH")
	if !strings.Contains(currentPath, fakeBinDir) {
		t.Fatalf("PATH should contain engram bin dir %q after download, got PATH=%q", fakeBinDir, currentPath)
	}
}

// TestRunInstallWindowsEngramUsesDownloadNotGoInstall verifies Windows path.
func TestRunInstallWindowsEngramUsesDownloadNotGoInstall(t *testing.T) {
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
		return `C:\fake\engram.exe`, nil
	}
	t.Cleanup(func() { engramDownloadFn = origDownloadFn })

	detection := system.DetectionResult{
		System: system.SystemInfo{
			OS:        "windows",
			Arch:      "amd64",
			Supported: true,
			Profile: system.PlatformProfile{
				OS:             "windows",
				PackageManager: "winget",
				Supported:      true,
			},
		},
	}

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

	// Must NOT have called "go install" for engram.
	for _, cmd := range recorder.get() {
		if strings.Contains(cmd, "go install") && strings.Contains(cmd, "engram") {
			t.Fatalf("Windows engram install should NOT use go install, got command: %s", cmd)
		}
	}
}

// TestRunInstallMacOSEngramStillUsesBrew verifies macOS unchanged.
func TestRunInstallMacOSEngramStillUsesBrew(t *testing.T) {
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

	// DownloadFn should NOT be called for macOS (brew handles it).
	origDownloadFn := engramDownloadFn
	engramDownloadFn = func(profile system.PlatformProfile) (string, error) {
		t.Error("DownloadLatestBinary should NOT be called on macOS (brew handles it)")
		return "", nil
	}
	t.Cleanup(func() { engramDownloadFn = origDownloadFn })

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

	// Must use brew install engram.
	commands := recorder.get()
	foundBrew := false
	for _, cmd := range commands {
		if strings.Contains(cmd, "brew install engram") {
			foundBrew = true
		}
	}
	if !foundBrew {
		t.Fatalf("expected brew install engram on macOS, got commands: %v", commands)
	}
}

// TestWithEngramEnv_SetsAndRestoresEnv verifies that withEngramEnv sets
// ENGRAM_DATA_DIR for the duration of fn and restores the previous value after.
func TestWithEngramEnv_SetsAndRestoresEnv(t *testing.T) {
	orig := os.Getenv(engram.DataDirEnvVar)
	defer os.Setenv(engram.DataDirEnvVar, orig)

	if orig != "" {
		os.Unsetenv(engram.DataDirEnvVar)
	}

	called := false
	err := withEngramEnv("/custom/engram", func() error {
		called = true
		if got := os.Getenv(engram.DataDirEnvVar); got != "/custom/engram" {
			t.Fatalf("expected ENGRAM_DATA_DIR=/custom/engram during fn, got %q", got)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("fn was not called")
	}
	if got := os.Getenv(engram.DataDirEnvVar); got != "" {
		t.Fatalf("expected ENGRAM_DATA_DIR unset after fn, got %q", got)
	}
}

// TestWithEngramEnv_RestoresPreviousValue verifies that an existing env value
// is restored after fn returns.
func TestWithEngramEnv_RestoresPreviousValue(t *testing.T) {
	orig := os.Getenv(engram.DataDirEnvVar)
	defer os.Setenv(engram.DataDirEnvVar, orig)

	os.Setenv(engram.DataDirEnvVar, "/previous/dir")

	err := withEngramEnv("/new/dir", func() error {
		if got := os.Getenv(engram.DataDirEnvVar); got != "/new/dir" {
			t.Fatalf("expected ENGRAM_DATA_DIR=/new/dir during fn, got %q", got)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := os.Getenv(engram.DataDirEnvVar); got != "/previous/dir" {
		t.Fatalf("expected ENGRAM_DATA_DIR=/previous/dir after fn, got %q", got)
	}
}

// TestWithEngramEnv_EmptyDataDirIsNoOp verifies that an empty dataDir does not
// mutate the environment.
func TestWithEngramEnv_EmptyDataDirIsNoOp(t *testing.T) {
	orig := os.Getenv(engram.DataDirEnvVar)
	defer os.Setenv(engram.DataDirEnvVar, orig)

	os.Setenv(engram.DataDirEnvVar, "/existing/dir")

	err := withEngramEnv("", func() error {
		if got := os.Getenv(engram.DataDirEnvVar); got != "/existing/dir" {
			t.Fatalf("expected ENGRAM_DATA_DIR unchanged during fn, got %q", got)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := os.Getenv(engram.DataDirEnvVar); got != "/existing/dir" {
		t.Fatalf("expected ENGRAM_DATA_DIR=/existing/dir after fn, got %q", got)
	}
}

// TestWithEngramEnv_RestoresOnError verifies the env is restored even when fn
// returns an error.
func TestWithEngramEnv_RestoresOnError(t *testing.T) {
	orig := os.Getenv(engram.DataDirEnvVar)
	defer os.Setenv(engram.DataDirEnvVar, orig)

	os.Setenv(engram.DataDirEnvVar, "/original")

	err := withEngramEnv("/changed", func() error {
		return fmt.Errorf("intentional failure")
	})
	if err == nil {
		t.Fatal("expected error from fn")
	}
	if got := os.Getenv(engram.DataDirEnvVar); got != "/original" {
		t.Fatalf("expected ENGRAM_DATA_DIR=/original after error, got %q", got)
	}
}

// Make sure the engram package's DownloadLatestBinary is accessible.
var _ = engram.DownloadLatestBinary

// TestWithEngramEnv_NestedCallsRestoreCorrectly verifies that nested
// withEngramEnv calls each restore their own outer value, not the original.
func TestWithEngramEnv_NestedCallsRestoreCorrectly(t *testing.T) {
	orig := os.Getenv(engram.DataDirEnvVar)
	defer os.Setenv(engram.DataDirEnvVar, orig)

	os.Setenv(engram.DataDirEnvVar, "/original")

	err := withEngramEnv("/outer", func() error {
		if got := os.Getenv(engram.DataDirEnvVar); got != "/outer" {
			t.Fatalf("outer: expected ENGRAM_DATA_DIR=/outer, got %q", got)
		}
		innerErr := withEngramEnv("/inner", func() error {
			if got := os.Getenv(engram.DataDirEnvVar); got != "/inner" {
				t.Fatalf("inner: expected ENGRAM_DATA_DIR=/inner, got %q", got)
			}
			return nil
		})
		if innerErr != nil {
			t.Fatalf("inner call failed: %v", innerErr)
		}
		// After inner call returns, outer value should be restored.
		if got := os.Getenv(engram.DataDirEnvVar); got != "/outer" {
			t.Fatalf("after inner: expected ENGRAM_DATA_DIR=/outer, got %q", got)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("outer call failed: %v", err)
	}
	// After outer call returns, original value should be restored.
	if got := os.Getenv(engram.DataDirEnvVar); got != "/original" {
		t.Fatalf("after outer: expected ENGRAM_DATA_DIR=/original, got %q", got)
	}
}
