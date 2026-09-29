package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/opencode"
	opencodeactivation "github.com/gentleman-programming/gentle-ai/v4/internal/opencode"
)

const v2SDKManualInstall = "npm install --save --no-audit --no-fund @opencode/plugin@2.0.4"

// freshV2SDKConfig prepares an isolated home with a fresh OpenCode config and
// a V2 runtime, returning the home and config directories.
func freshV2SDKConfig(t *testing.T) (string, string) {
	t.Helper()
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
	return home, config
}

// installFakeNPM writes an npm stub at dir/npm and puts dir first on PATH.
func installFakeNPM(t *testing.T, dir, script string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "npm")
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return path
}

func TestV2SDKProvisionReportsManagerFailureClass(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script npm stubs require a POSIX shell")
	}
	for _, tc := range []struct {
		name, script, want string
		timeout            time.Duration
	}{
		{name: "non-zero exit", script: "#!/bin/sh\nprintf 'SECRET_TOKEN_DO_NOT_LOG' >&2\nexit 17\n", want: "exited with code 17"},
		{name: "timeout", script: "#!/bin/sh\nprintf 'SECRET_TOKEN_DO_NOT_LOG'; sleep 3\n", want: "timed out after", timeout: 30 * time.Millisecond},
		{name: "verification", script: "#!/bin/sh\nexit 0\n", want: "verification failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, config := freshV2SDKConfig(t)
			npm := installFakeNPM(t, filepath.Join(t.TempDir(), "bin"), tc.script)
			if tc.timeout > 0 {
				old := openCodeSDKInstallTimeout
				openCodeSDKInstallTimeout = tc.timeout
				t.Cleanup(func() { openCodeSDKInstallTimeout = old })
			}
			proposal, err := OpenCodeSDKInstallProposal(home)
			if err != nil || proposal == nil {
				t.Fatalf("proposal = %+v, %v", proposal, err)
			}
			err = (openCodePluginDependencyPreflightStep{homeDir: home, consent: proposal}).Run()
			if err == nil {
				t.Fatal("failed manager passed")
			}
			msg := err.Error()
			if !strings.Contains(msg, tc.want) || !strings.Contains(msg, v2SDKManualInstall) || !strings.Contains(msg, config) {
				t.Fatalf("failure class or manual continuation missing, want %q: %s", tc.want, msg)
			}
			if strings.Contains(msg, "SECRET_TOKEN_DO_NOT_LOG") || strings.Contains(msg, filepath.Dir(npm)) {
				t.Fatalf("failure leaked manager output or executable path: %s", msg)
			}
		})
	}
}

func TestV2SDKProposalRefusesVersionManagerShims(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script npm stubs require a POSIX shell")
	}
	for _, tc := range []struct {
		name, dir, script, want string
	}{
		{name: "volta", dir: ".volta/bin", script: "#!/bin/sh\ntouch \"$PWD/invoked\"\n", want: "volta"},
		{name: "asdf", dir: ".asdf/shims", script: "#!/bin/sh\ntouch \"$PWD/invoked\"\n", want: "asdf"},
		{name: "mise", dir: ".local/share/mise/shims", script: "#!/bin/sh\ntouch \"$PWD/invoked\"\n", want: "mise"},
		{name: "nodenv", dir: ".nodenv/shims", script: "#!/bin/sh\ntouch \"$PWD/invoked\"\n", want: "nodenv"},
		{name: "shell script shim", dir: "tools/bin", script: "#!/usr/bin/env bash\nexec asdf exec \"npm\" \"$@\"\n", want: "asdf"},
		{name: "missing interpreter", dir: "plain/bin", script: "#!/usr/bin/env gentle-ai-missing-interpreter\n", want: "interpreter"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, config := freshV2SDKConfig(t)
			root := t.TempDir()
			installFakeNPM(t, filepath.Join(root, filepath.FromSlash(tc.dir)), tc.script)
			proposal, err := OpenCodeSDKInstallProposal(home)
			if err == nil || proposal != nil {
				t.Fatalf("version-manager shim was offered for isolated install: proposal=%+v err=%v", proposal, err)
			}
			msg := err.Error()
			for _, want := range []string{tc.want, "isolated", v2SDKManualInstall, config, "manually"} {
				if !strings.Contains(msg, want) {
					t.Errorf("shim refusal missing %q: %s", want, msg)
				}
			}
			if strings.Contains(msg, root) {
				t.Errorf("shim refusal leaked the executable path: %s", msg)
			}
			if _, err := os.Stat(filepath.Join(config, "invoked")); !os.IsNotExist(err) {
				t.Fatalf("shim ran during proposal: %v", err)
			}
		})
	}
	t.Run("real nvm installation stays eligible", func(t *testing.T) {
		home, _ := freshV2SDKConfig(t)
		installFakeNPM(t, filepath.Join(t.TempDir(), ".nvm", "versions", "node", "v22.0.0", "bin"), "#!/bin/sh\nexit 0\n")
		if proposal, err := OpenCodeSDKInstallProposal(home); err != nil || proposal == nil {
			t.Fatalf("nvm-managed npm was refused: proposal=%+v err=%v", proposal, err)
		}
	})
}

func TestOpenCodeSDKFailureClassNamesStartExitSignalAndDeadline(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX executables")
	}
	missing := exec.Command(filepath.Join(t.TempDir(), "missing-npm"))
	startErr := missing.Run()
	exitErr := exec.Command("/bin/sh", "-c", "exit 3").Run()
	signalErr := exec.Command("/bin/sh", "-c", "kill -KILL $$").Run()
	for _, tc := range []struct {
		name   string
		err    error
		ctxErr error
		want   string
	}{
		{"start", startErr, nil, "could not start"},
		{"exit", exitErr, nil, "exited with code 3"},
		{"signal", signalErr, nil, "was terminated by a signal"},
		{"deadline", exitErr, context.DeadlineExceeded, "timed out after 2m0s"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err == nil {
				t.Fatal("fixture command unexpectedly succeeded")
			}
			if got := openCodeSDKFailureClass(tc.err, tc.ctxErr, 2*time.Minute); got != tc.want {
				t.Fatalf("failure class = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestV2SDKPreflightRuntimeDetectionFailureIsActionable(t *testing.T) {
	home := t.TempDir()
	setOpenCodeTestHome(t, home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	oldVersion := opencodeactivation.VersionRunnerOverride
	t.Cleanup(func() { opencodeactivation.VersionRunnerOverride = oldVersion })
	opencodeactivation.VersionRunnerOverride = func(context.Context, opencodeactivation.Command) (opencodeactivation.CommandOutput, error) {
		return opencodeactivation.CommandOutput{}, os.ErrNotExist
	}
	for name, run := range map[string]func() error{
		"install and sync preflight": func() error { return (openCodePluginDependencyPreflightStep{homeDir: home}).Run() },
		"TUI proposal": func() error {
			_, err := OpenCodeSDKInstallProposal(home)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := run()
			if err == nil {
				t.Fatal("unknown OpenCode runtime passed the managed-asset preflight")
			}
			msg := err.Error()
			for _, want := range []string{"OpenCode runtime version unavailable or unsupported", "opencode --version", "deselect OpenCode", "retry"} {
				if !strings.Contains(msg, want) {
					t.Errorf("detection failure missing %q: %s", want, msg)
				}
			}
		})
	}
}
