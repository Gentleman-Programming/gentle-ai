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

// Windows npm stub bodies shared by the SDK provisioning tests. The manager
// runs in the credential-isolated environment, whose PATH has no System32, so
// they use cmd.exe built-ins only. Each writes relative to its working
// directory, which is the config directory the POSIX stubs reach via $PWD.
const (
	fakeNPMWindowsTouchInvoked = "@echo off\ntype nul > invoked\n"
	fakeNPMWindowsTouchRan     = "@echo off\ntype nul > ran\n"
	fakeNPMWindowsExitZero     = "@echo off\nexit 0\n"
	fakeNPMWindowsMaterialize  = "@echo off\nmkdir node_modules\\@opencode\\plugin\n>node_modules\\@opencode\\plugin\\package.json echo {\"version\":\"2.0.4\"}\n"
	// fakeNPMWindowsSecretThenHang spins until the manager deadline kills it.
	// A child such as ping would outlive cmd.exe and keep the temporary
	// working directory locked past the test's cleanup.
	fakeNPMWindowsSecretThenHang = "@echo off\necho SECRET_TOKEN_DO_NOT_LOG\n:spin\ngoto spin\n"
	fakeNPMWindowsSecretExit17   = "@echo off\necho SECRET_TOKEN_DO_NOT_LOG 1>&2\nexit 17\n"
)

// writeFakeNPM writes the npm stub that exec.LookPath resolves on this
// platform. Windows ignores the extensionless POSIX script (PATHEXT), so
// there the stub is an equivalent npm.cmd batch file with CRLF line endings.
func writeFakeNPM(t *testing.T, dir, posix, windows string) string {
	t.Helper()
	name, script := "npm", posix
	if runtime.GOOS == "windows" {
		name, script = "npm.cmd", strings.ReplaceAll(windows, "\n", "\r\n")
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}

// installFakeNPM writes the platform's npm stub into dir and puts dir first on
// PATH.
func installFakeNPM(t *testing.T, dir, posix, windows string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := writeFakeNPM(t, dir, posix, windows)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return path
}

func TestV2SDKProvisionReportsManagerFailureClass(t *testing.T) {
	for _, tc := range []struct {
		name, script, windows, want string
		timeout                     time.Duration
	}{
		{name: "non-zero exit", script: "#!/bin/sh\nprintf 'SECRET_TOKEN_DO_NOT_LOG' >&2\nexit 17\n", windows: fakeNPMWindowsSecretExit17, want: "exited with code 17"},
		{name: "timeout", script: "#!/bin/sh\nprintf 'SECRET_TOKEN_DO_NOT_LOG'; sleep 3\n", windows: fakeNPMWindowsSecretThenHang, want: "timed out after", timeout: 30 * time.Millisecond},
		{name: "verification", script: "#!/bin/sh\nexit 0\n", windows: fakeNPMWindowsExitZero, want: "verification failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, config := freshV2SDKConfig(t)
			npm := installFakeNPM(t, filepath.Join(t.TempDir(), "bin"), tc.script, tc.windows)
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
	for _, tc := range []struct {
		name, dir, script, want string
		// shebang marks cases detected from the script's #! line, which the
		// product inspects only on POSIX.
		shebang bool
	}{
		{name: "volta", dir: ".volta/bin", script: "#!/bin/sh\ntouch \"$PWD/invoked\"\n", want: "volta"},
		{name: "asdf", dir: ".asdf/shims", script: "#!/bin/sh\ntouch \"$PWD/invoked\"\n", want: "asdf"},
		{name: "mise", dir: ".local/share/mise/shims", script: "#!/bin/sh\ntouch \"$PWD/invoked\"\n", want: "mise"},
		{name: "nodenv", dir: ".nodenv/shims", script: "#!/bin/sh\ntouch \"$PWD/invoked\"\n", want: "nodenv"},
		{name: "shell script shim", dir: "tools/bin", script: "#!/usr/bin/env bash\nexec asdf exec \"npm\" \"$@\"\n", want: "asdf", shebang: true},
		{name: "missing interpreter", dir: "plain/bin", script: "#!/usr/bin/env gentle-ai-missing-interpreter\n", want: "interpreter", shebang: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.shebang && runtime.GOOS == "windows" {
				t.Skip("shebang interpreter detection is POSIX-only")
			}
			home, config := freshV2SDKConfig(t)
			root := t.TempDir()
			installFakeNPM(t, filepath.Join(root, filepath.FromSlash(tc.dir)), tc.script, fakeNPMWindowsTouchInvoked)
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
		installFakeNPM(t, filepath.Join(t.TempDir(), ".nvm", "versions", "node", "v22.0.0", "bin"), "#!/bin/sh\nexit 0\n", fakeNPMWindowsExitZero)
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
