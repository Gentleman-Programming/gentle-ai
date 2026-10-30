package cli

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/gga"
	"github.com/gentleman-programming/gentle-ai/v4/internal/installcmd"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

// stubGGAHomebrewEnv isolates the GGA install step from the real machine: gga
// is never available, PATH holds only the binaries listed in onPath, and the
// standard Homebrew prefixes hold only the binaries listed in atPrefix. Every
// command goes to run instead of a real process, and the resolver's
// `brew help trust` probe reports an older Homebrew without `trust`. The fake
// PATH and prefixes apply to both the cli and installcmd lookups.
func stubGGAHomebrewEnv(t *testing.T, onPath, atPrefix []string, run func(name string, args ...string) error) string {
	t.Helper()
	home := t.TempDir()
	t.Cleanup(installcmd.OverrideBrewHelpTrust(func(string) error {
		return errors.New("brew trust unsupported")
	}))

	origHome := osUserHomeDir
	origCmdLookPath := cmdLookPath
	origOsStat := osStat
	origRunCmd := runCommand
	origGGAAvailableCheck := ggaAvailableCheck
	origCleanup := cleanupGGAInstallDir
	t.Cleanup(func() {
		osUserHomeDir = origHome
		cmdLookPath = origCmdLookPath
		osStat = origOsStat
		runCommand = origRunCmd
		ggaAvailableCheck = origGGAAvailableCheck
		cleanupGGAInstallDir = origCleanup
	})

	executable := filepath.Join(t.TempDir(), "executable")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	osUserHomeDir = func() (string, error) { return home, nil }
	lookPath := func(file string) (string, error) {
		if slices.Contains(onPath, file) {
			return "/usr/bin/" + file, nil
		}
		return "", &os.PathError{Op: "lookpath", Path: file, Err: errors.New("executable file not found in $PATH")}
	}
	stat := func(name string) (os.FileInfo, error) {
		for _, prefix := range []string{"/opt/homebrew/bin/", "/usr/local/bin/", "/home/linuxbrew/.linuxbrew/bin/"} {
			if strings.HasPrefix(name, prefix) {
				if slices.Contains(atPrefix, strings.TrimPrefix(name, prefix)) {
					return os.Stat(executable)
				}
				return nil, fs.ErrNotExist
			}
		}
		return os.Stat(name)
	}
	cmdLookPath = lookPath
	osStat = stat
	t.Cleanup(installcmd.OverrideLookPath(lookPath))
	t.Cleanup(installcmd.OverrideStat(stat))
	ggaAvailableCheck = func(system.PlatformProfile) bool { return false }
	cleanupGGAInstallDir = func() error { return nil }
	runCommand = run
	return home
}

func darwinGGAStep(home string, state *runtimeState) componentApplyStep {
	return componentApplyStep{
		id:           "component:gga",
		component:    model.ComponentGGA,
		homeDir:      home,
		workspaceDir: home,
		agents:       []model.AgentID{model.AgentClaudeCode},
		selection:    model.Selection{},
		profile:      system.PlatformProfile{OS: "darwin", PackageManager: "brew", Supported: true},
		state:        state,
	}
}

// TestRunInstallMacOSWithoutHomebrewDefersGGA reproduces #3018: on macOS the
// platform profile always reports PackageManager "brew", so a machine without
// Homebrew used to fail the whole install with `exec: "brew": executable file
// not found in $PATH`. Missing Homebrew is a missing optional prerequisite:
// the install succeeds, GGA configuration is still written, and the user gets
// a manual action instead of a false "GGA is now installed globally" note.
func TestRunInstallMacOSWithoutHomebrewDefersGGA(t *testing.T) {
	var brewCalls []string
	home := stubGGAHomebrewEnv(t, nil, nil, func(name string, args ...string) error {
		if filepath.Base(name) == "brew" {
			brewCalls = append(brewCalls, strings.Join(append([]string{name}, args...), " "))
			return errors.New(`exec: "brew": executable file not found in $PATH`)
		}
		return nil
	})

	result, err := RunInstall([]string{"--agent", "claude-code", "--component", "gga"}, macOSDetectionResult())
	if err != nil {
		t.Fatalf("RunInstall() error = %v, want nil: missing Homebrew must not fail the install", err)
	}
	if len(brewCalls) != 0 {
		t.Fatalf("brew was invoked without Homebrew installed: %q", brewCalls)
	}
	if !result.Verify.Ready {
		t.Fatalf("Verify.Ready = false, want true: GGA configuration must still be applied")
	}
	if _, err := os.Stat(gga.ConfigPath(home)); err != nil {
		t.Fatalf("GGA config was not written: %v", err)
	}
	if !slices.Contains(result.ManualActions, GGAHomebrewDeferredAction) {
		t.Fatalf("ManualActions = %q, want the Homebrew prerequisite action", result.ManualActions)
	}
	if rendered := RenderInstallManualActions(result); !strings.Contains(rendered, "gentle-ai install --component gga") {
		t.Fatalf("RenderInstallManualActions() = %q, want the rerun command", rendered)
	}
	if strings.Contains(result.Verify.FinalNote, "GGA is now installed globally") {
		t.Fatalf("FinalNote claims GGA was installed: %q", result.Verify.FinalNote)
	}
}

// TestGGAStepWithoutHomebrewRecordsDeferredAction covers the TUI path, which
// reads the GGA manual actions from the runtime state.
func TestGGAStepWithoutHomebrewRecordsDeferredAction(t *testing.T) {
	home := stubGGAHomebrewEnv(t, nil, nil, func(string, ...string) error { return nil })
	state := &runtimeState{}

	if err := darwinGGAStep(home, state).Run(); err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if !slices.Contains(state.ggaManualActions(), GGAHomebrewDeferredAction) {
		t.Fatalf("ggaManualActions() = %q, want the Homebrew prerequisite action", state.ggaManualActions())
	}
	if got := state.deferredComponents(); !slices.Equal(got, []model.ComponentID{model.ComponentGGA}) {
		t.Fatalf("deferredComponents() = %v, want [gga]", got)
	}
}

// TestGGAStepUsesHomebrewOutsidePath pins that a Homebrew found only at its
// standard prefix is used by absolute path, the same way Engram resolves it.
func TestGGAStepUsesHomebrewOutsidePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Homebrew prefixes are POSIX paths; Windows never resolves Homebrew outside PATH")
	}
	var commands []string
	home := stubGGAHomebrewEnv(t, nil, []string{"brew"}, func(name string, args ...string) error {
		commands = append(commands, strings.Join(append([]string{name}, args...), " "))
		return nil
	})
	var probed []string
	t.Cleanup(installcmd.OverrideBrewHelpTrust(func(brew string) error {
		probed = append(probed, brew)
		return nil
	}))
	state := &runtimeState{}

	if err := darwinGGAStep(home, state).Run(); err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if !slices.Equal(probed, []string{"/opt/homebrew/bin/brew"}) {
		t.Fatalf("brew help trust probed %q, want the prefix Homebrew only", probed)
	}
	want := []string{
		"/opt/homebrew/bin/brew tap Gentleman-Programming/homebrew-tap",
		"/opt/homebrew/bin/brew trust --formula gentleman-programming/tap/gga",
	}
	if len(commands) < len(want) || !slices.Equal(commands[:len(want)], want) {
		t.Fatalf("commands = %q, want the Homebrew sequence to start with %q", commands, want)
	}
	if state.ggaInstallDeferred || state.deferredComponents() != nil {
		t.Fatal("GGA install was deferred although Homebrew is available")
	}
}

// TestGGAStepHomebrewFailureStillFails pins that a present Homebrew failing for
// its own reasons is still a real install failure, not a deferred prerequisite.
func TestGGAStepHomebrewFailureStillFails(t *testing.T) {
	home := stubGGAHomebrewEnv(t, []string{"brew"}, nil, func(name string, args ...string) error {
		if filepath.Base(name) == "brew" {
			return errors.New("simulated brew tap failure: exit status 1")
		}
		return nil
	})
	state := &runtimeState{}

	err := darwinGGAStep(home, state).Run()
	if err == nil || !strings.Contains(err.Error(), "simulated brew tap failure") {
		t.Fatalf("Run() error = %v, want the brew failure", err)
	}
	if state.ggaInstallDeferred {
		t.Fatal("a real Homebrew failure was reported as a deferred install")
	}
}
