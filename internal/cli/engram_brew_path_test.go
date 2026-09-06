package cli

import (
	"os"
	"strings"
	"testing"
)

// TestResolveEngramInstalledPathViaHomebrewOptPrefix reproduces gentle-ai
// issue #4020: engram is genuinely installed via Homebrew, but gentle-ai's
// own process PATH omits /opt/homebrew/bin (e.g. launched outside a login
// shell), so cmdLookPath("engram") fails. resolveEngramInstalledPath must
// still find it via the well-known Homebrew prefix, mirroring ggaAvailable's
// existing fallback for the sibling GGA component.
func TestResolveEngramInstalledPathViaHomebrewOptPrefix(t *testing.T) {
	restoreLookPath := cmdLookPath
	restoreStat := osStat
	cmdLookPath = missingBinaryLookPath
	osStat = func(name string) (os.FileInfo, error) {
		if name == "/opt/homebrew/bin/engram" {
			return os.Stat(os.DevNull)
		}
		return nil, os.ErrNotExist
	}
	t.Cleanup(func() {
		cmdLookPath = restoreLookPath
		osStat = restoreStat
	})

	path, found := resolveEngramInstalledPath(macOSDetectionResult().System.Profile)
	if !found {
		t.Fatal("resolveEngramInstalledPath() found = false, want true when engram is at /opt/homebrew/bin/engram")
	}
	if path != "/opt/homebrew/bin/engram" {
		t.Fatalf("resolveEngramInstalledPath() path = %q, want /opt/homebrew/bin/engram", path)
	}
}

// TestResolveEngramInstalledPathReturnsFalseWhenNotFound verifies the
// fallback correctly reports missing when engram is nowhere to be found,
// so a genuinely missing engram still triggers installation.
func TestResolveEngramInstalledPathReturnsFalseWhenNotFound(t *testing.T) {
	restoreLookPath := cmdLookPath
	restoreStat := osStat
	cmdLookPath = missingBinaryLookPath
	osStat = func(name string) (os.FileInfo, error) { return nil, os.ErrNotExist }
	t.Cleanup(func() {
		cmdLookPath = restoreLookPath
		osStat = restoreStat
	})

	if _, found := resolveEngramInstalledPath(macOSDetectionResult().System.Profile); found {
		t.Fatal("resolveEngramInstalledPath() found = true, want false when engram is not installed anywhere")
	}
}

// TestRepro4020EngramSkipsBrewWhenAlreadyInstalledOffPath is the end-to-end
// regression test for issue #4020: with a deficient process PATH (neither
// "engram" nor "brew" resolve via cmdLookPath) but engram genuinely present
// at the standard Homebrew prefix, RunInstall must not invoke brew at all.
func TestRepro4020EngramSkipsBrewWhenAlreadyInstalledOffPath(t *testing.T) {
	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	restoreStat := osStat
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
		osStat = restoreStat
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	cmdLookPath = missingBinaryLookPath
	osStat = func(name string) (os.FileInfo, error) {
		if name == "/opt/homebrew/bin/engram" {
			return os.Stat(os.DevNull)
		}
		return nil, os.ErrNotExist
	}

	recorder := &commandRecorder{}
	runCommand = recorder.record

	detection := macOSDetectionResult()
	if _, err := RunInstall([]string{"--agent", "opencode", "--component", "engram"}, detection); err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	for _, cmd := range recorder.get() {
		if strings.Contains(cmd, "brew") {
			t.Fatalf("brew must not be invoked when engram is already installed at the standard Homebrew prefix; commands = %v", recorder.get())
		}
	}
}

// TestEngramInstallResolvesBrewViaHomebrewPrefixWhenPathDeficient covers the
// second half of issue #4020: when engram genuinely needs installing and the
// process PATH lacks brew, the resolved absolute Homebrew path is used
// instead of the bare "brew" name that failed to resolve for the reporter.
func TestEngramInstallResolvesBrewViaHomebrewPrefixWhenPathDeficient(t *testing.T) {
	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	restoreStat := osStat
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
		osStat = restoreStat
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	// Neither engram nor brew resolve via cmdLookPath (deficient PATH), but
	// brew genuinely exists at the standard Apple Silicon Homebrew prefix.
	cmdLookPath = missingBinaryLookPath
	osStat = func(name string) (os.FileInfo, error) {
		if name == "/opt/homebrew/bin/brew" {
			return os.Stat(os.DevNull)
		}
		return nil, os.ErrNotExist
	}

	recorder := &commandRecorder{}
	runCommand = recorder.record

	detection := macOSDetectionResult()
	if _, err := RunInstall([]string{"--agent", "opencode", "--component", "engram"}, detection); err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	foundResolved := false
	for _, cmd := range recorder.get() {
		if strings.HasPrefix(cmd, "/opt/homebrew/bin/brew ") {
			foundResolved = true
		}
		if strings.HasPrefix(cmd, "brew ") {
			t.Fatalf("brew invoked via bare name despite a deficient PATH; want resolved absolute path; commands = %v", recorder.get())
		}
	}
	if !foundResolved {
		t.Fatalf("expected a command using the resolved /opt/homebrew/bin/brew path, got: %v", recorder.get())
	}
}

// TestResolveBrewCommandFallsBackToLoginShell verifies the third resolution
// tier: when brew is on neither cmdLookPath nor a well-known prefix, a login
// shell lookup (which sources ~/.zprofile-style brew shellenv) is consulted.
func TestResolveBrewCommandFallsBackToLoginShell(t *testing.T) {
	restoreLookPath := cmdLookPath
	restoreStat := osStat
	restoreLoginShell := loginShellOutput
	cmdLookPath = missingBinaryLookPath
	osStat = func(name string) (os.FileInfo, error) { return nil, os.ErrNotExist }
	loginShellOutput = func(shell, script string) ([]byte, error) {
		return []byte("/custom/brew/bin/brew\n"), nil
	}
	t.Cleanup(func() {
		cmdLookPath = restoreLookPath
		osStat = restoreStat
		loginShellOutput = restoreLoginShell
	})

	if got := resolveBrewCommand(); got != "/custom/brew/bin/brew" {
		t.Fatalf("resolveBrewCommand() = %q, want /custom/brew/bin/brew", got)
	}
}

// TestResolveBrewCommandFallsBackToBareNameWhenTrulyMissing verifies that a
// genuinely missing Homebrew still produces the bare "brew" name, so the
// standard, actionable "executable file not found in $PATH" error still
// surfaces rather than being masked.
func TestResolveBrewCommandFallsBackToBareNameWhenTrulyMissing(t *testing.T) {
	restoreLookPath := cmdLookPath
	restoreStat := osStat
	restoreLoginShell := loginShellOutput
	cmdLookPath = missingBinaryLookPath
	osStat = func(name string) (os.FileInfo, error) { return nil, os.ErrNotExist }
	loginShellOutput = func(shell, script string) ([]byte, error) {
		return nil, os.ErrNotExist
	}
	t.Cleanup(func() {
		cmdLookPath = restoreLookPath
		osStat = restoreStat
		loginShellOutput = restoreLoginShell
	})

	if got := resolveBrewCommand(); got != "brew" {
		t.Fatalf("resolveBrewCommand() = %q, want bare \"brew\" when Homebrew is genuinely absent", got)
	}
}
