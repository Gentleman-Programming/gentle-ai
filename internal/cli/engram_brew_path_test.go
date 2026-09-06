package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func writeFakeExecutable(t *testing.T, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-executable")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), mode); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
	return path
}

func useStandardPathFixture(t *testing.T, standardPath, fixturePath string) {
	t.Helper()
	restoreStat := osStat
	osStat = func(name string) (os.FileInfo, error) {
		if name == standardPath {
			return os.Stat(fixturePath)
		}
		return nil, os.ErrNotExist
	}
	t.Cleanup(func() { osStat = restoreStat })
}

func useMissingStandardExecutablePaths(t *testing.T) {
	t.Helper()
	restoreStat := osStat
	osStat = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
	t.Cleanup(func() { osStat = restoreStat })
}

func TestIsExecutableFile(t *testing.T) {
	executable := writeFakeExecutable(t, 0o755)
	plainFile := writeFakeExecutable(t, 0o644)
	directory := t.TempDir()

	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "executable regular file", path: executable, want: true},
		{name: "non-executable regular file", path: plainFile, want: false},
		{name: "directory", path: directory, want: false},
		{name: "missing file", path: filepath.Join(t.TempDir(), "missing"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isExecutableFile(tt.path); got != tt.want {
				t.Fatalf("isExecutableFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestResolveEngramInstalledPathUsesExecutableHomebrewFallback(t *testing.T) {
	restoreLookPath := cmdLookPath
	cmdLookPath = missingBinaryLookPath
	t.Cleanup(func() { cmdLookPath = restoreLookPath })

	fixture := writeFakeExecutable(t, 0o755)
	useStandardPathFixture(t, "/opt/homebrew/bin/engram", fixture)

	path, found := resolveEngramInstalledPath(macOSDetectionResult().System.Profile)
	if !found || path != "/opt/homebrew/bin/engram" {
		t.Fatalf("resolveEngramInstalledPath() = (%q, %v), want (%q, true)", path, found, "/opt/homebrew/bin/engram")
	}
}

func TestResolveEngramInstalledPathRejectsNonExecutableFile(t *testing.T) {
	restoreLookPath := cmdLookPath
	cmdLookPath = missingBinaryLookPath
	t.Cleanup(func() { cmdLookPath = restoreLookPath })

	fixture := writeFakeExecutable(t, 0o644)
	useStandardPathFixture(t, "/opt/homebrew/bin/engram", fixture)

	if path, found := resolveEngramInstalledPath(macOSDetectionResult().System.Profile); found {
		t.Fatalf("resolveEngramInstalledPath() = (%q, true), want not found for a non-executable file", path)
	}
}

func TestRepro4020RunInstallUsesOffPathEngramWithoutInvokingBrew(t *testing.T) {
	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	restoreVersionCommand := verifyEngramVersionCommand
	restoreProbeCommand := probeEngramProtocolFlagCommand
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
		verifyEngramVersionCommand = restoreVersionCommand
		probeEngramProtocolFlagCommand = restoreProbeCommand
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	cmdLookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	fixture := writeFakeExecutable(t, 0o755)
	useStandardPathFixture(t, "/opt/homebrew/bin/engram", fixture)

	recorder := &commandRecorder{}
	runCommand = recorder.record
	var versionCommand, probeCommand string
	verifyEngramVersionCommand = func(command string) (string, error) {
		versionCommand = command
		return "engram 1.20.0", nil
	}
	probeEngramProtocolFlagCommand = func(_ context.Context, command string) (string, error) {
		probeCommand = command
		return "Usage: engram setup <slug> --protocol=full", nil
	}

	if _, err := RunInstall([]string{"--agent", "opencode", "--component", "engram"}, macOSDetectionResult()); err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	const wantEngram = "/opt/homebrew/bin/engram"
	if versionCommand != wantEngram || probeCommand != wantEngram {
		t.Fatalf("Engram probes used version=%q probe=%q, want %q", versionCommand, probeCommand, wantEngram)
	}
	foundSetup := false
	for _, command := range recorder.get() {
		fields := strings.Fields(command)
		if len(fields) > 0 && filepath.Base(fields[0]) == "brew" {
			t.Fatalf("RunInstall() invoked brew for an existing off-PATH Engram: %v", recorder.get())
		}
		if strings.HasPrefix(command, wantEngram+" setup ") {
			foundSetup = true
		}
	}
	if !foundSetup {
		t.Fatalf("RunInstall() commands = %v, want setup through %s", recorder.get(), wantEngram)
	}
}

func TestRunInstallFreshBrewInstallUsesInstalledBinaryForVersionProbeAndSetup(t *testing.T) {
	home := t.TempDir()
	brewFixture := writeFakeExecutable(t, 0o755)
	engramFixture := writeFakeExecutable(t, 0o755)
	const brewPath = "/opt/homebrew/bin/brew"
	const engramPath = "/opt/homebrew/bin/engram"

	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	restoreStat := osStat
	restoreVersionCommand := verifyEngramVersionCommand
	restoreProbeCommand := probeEngramProtocolFlagCommand
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
		osStat = restoreStat
		verifyEngramVersionCommand = restoreVersionCommand
		probeEngramProtocolFlagCommand = restoreProbeCommand
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	cmdLookPath = missingBinaryLookPath
	installed := false
	osStat = func(name string) (os.FileInfo, error) {
		switch name {
		case brewPath:
			return os.Stat(brewFixture)
		case engramPath:
			if installed {
				return os.Stat(engramFixture)
			}
		}
		return nil, os.ErrNotExist
	}

	recorder := &commandRecorder{}
	runCommand = func(name string, args ...string) error {
		if err := recorder.record(name, args...); err != nil {
			return err
		}
		if name == brewPath && len(args) == 2 && args[0] == "install" && args[1] == "engram" {
			installed = true
		}
		return nil
	}
	var versionCommand, probeCommand string
	verifyEngramVersionCommand = func(command string) (string, error) {
		versionCommand = command
		return "engram 1.20.0", nil
	}
	probeEngramProtocolFlagCommand = func(_ context.Context, command string) (string, error) {
		probeCommand = command
		return "Usage: engram setup <slug> --protocol=full", nil
	}

	if _, err := RunInstall([]string{"--agent", "opencode", "--component", "engram"}, macOSDetectionResult()); err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	if !installed {
		t.Fatal("RunInstall() did not execute the fixture Homebrew install")
	}
	if versionCommand != engramPath || probeCommand != engramPath {
		t.Fatalf("Engram probes used version=%q probe=%q, want newly installed %q", versionCommand, probeCommand, engramPath)
	}
	foundSetup := false
	for _, command := range recorder.get() {
		if strings.HasPrefix(command, engramPath+" setup ") {
			foundSetup = true
		}
		if strings.HasPrefix(command, "engram setup ") {
			t.Fatalf("RunInstall() used unresolved Engram after Brew install: %v", recorder.get())
		}
	}
	if !foundSetup {
		t.Fatalf("RunInstall() commands = %v, want setup through newly installed %s", recorder.get(), engramPath)
	}
}

func TestResolveBrewCommandRejectsNonExecutableStandardPrefix(t *testing.T) {
	restoreLookPath := cmdLookPath
	cmdLookPath = missingBinaryLookPath
	t.Cleanup(func() { cmdLookPath = restoreLookPath })

	fixture := writeFakeExecutable(t, 0o644)
	useStandardPathFixture(t, "/opt/homebrew/bin/brew", fixture)

	if got := resolveBrewCommand(); got != "brew" {
		t.Fatalf("resolveBrewCommand() = %q, want bare brew for a non-executable file", got)
	}
}
