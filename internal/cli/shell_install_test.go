package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

// Platform-neutral routing and flag contracts. Review-model tests live in
// shell_install_view_test.go (Linux model) and shell_install_windows_test.go.

func TestShellInstallFlags(t *testing.T) {
	for _, args := range [][]string{{"--unknown"}, {"extra"}, {"--target"}, {"--confirm"}} {
		if _, _, err := parseShellInstall(args, io.Discard); err == nil {
			t.Fatalf("accepted invalid flags %q", args)
		}
	}
	req, inspect, err := parseShellInstall([]string{"--target", "/owned/shell", "--mode", "shared", "--prefix", "/owned/pi", "--agent", "/owned/agent", "--inspect"}, io.Discard)
	if err != nil || !inspect || req.Mode != "shared" || req.Confirmation != "" {
		t.Fatalf("selection = %+v inspect=%v error=%v", req, inspect, err)
	}
	if _, err := shellinstaller.UserInstallFromEntry(shellEntryValues(req)); err != nil {
		t.Fatal(err)
	}
}

func TestShellInstallChannelMatchesPlatform(t *testing.T) {
	for _, channel := range []string{"main", "stable"} {
		req, _, err := parseShellInstall([]string{"--target", "/owned/shell", "--channel", channel}, io.Discard)
		if runtime.GOOS == "windows" {
			entry := shellEntryValues(req)
			if err != nil || entry[len(entry)-1] != channel {
				t.Fatalf("Windows channel did not reach the worker: %q, %v", entry, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), "Windows-only") || req != (shellinstaller.UserInstallRequest{}) {
			t.Fatalf("explicit --channel %s was ignored instead of refused: %+v, %v", channel, req, err)
		}
	}
	req, _, err := parseShellInstall([]string{"--target", "/owned/shell"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if entry, err := shellinstaller.UserInstallFromEntry(shellEntryValues(req)); err != nil || entry.Destination != "/owned/shell" {
		t.Fatalf("default selection does not round-trip through the platform entry: %+v, %v", entry, err)
	}
}

func TestShellInstallRefusalHelpIsRunnable(t *testing.T) {
	_, _, positionalErr := parseShellInstall([]string{"extra"}, io.Discard)
	if positionalErr == nil || !strings.Contains(positionalErr.Error(), "gentle-ai shell install --help") {
		t.Fatalf("refusal lacks the help continuation: %v", positionalErr)
	}
	var output bytes.Buffer
	if err := RunShell([]string{"install", "--help"}, &output); err != nil || !strings.Contains(output.String(), "--inspect") || !strings.Contains(output.String(), "--confirm") {
		t.Fatalf("named help is not runnable or lacks physical consent flags: %v %q", err, output.String())
	}
}

func TestShellInstallConfirmationRefusalHasNoEffects(t *testing.T) {
	if platform := runtime.GOOS + "/" + runtime.GOARCH; platform != "linux/amd64" && platform != "darwin/arm64" {
		t.Skip("physical user selection is Linux amd64 or macOS arm64 only")
	}
	parent := t.TempDir()
	if err := os.Chmod(parent, 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(parent, "shell")
	if _, err := shellinstaller.InspectUserInstall(shellinstaller.UserInstallRequest{Destination: target, Mode: "separate"}); err != nil {
		t.Fatalf("invalid physical selection fixture: %v", err)
	}
	err := RunShell([]string{"install", "--target", target, "--confirm", "not-confirmed"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "gentle-ai shell install --help") || !strings.Contains(err.Error(), "--inspect") || !strings.Contains(err.Error(), "--confirm") {
		t.Fatalf("unconfirmed selection lacks the safe continuation: %v", err)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("unconfirmed selection created or published a target: %v", err)
	}
}

func TestShellInstallSeparateRepairHelpHasNoEffects(t *testing.T) {
	// The documented repair continuation remains usable without a user manager.
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	var output bytes.Buffer
	if err := RunShell([]string{"install", "--help"}, &output); err != nil || !strings.Contains(output.String(), "--mode separate") {
		t.Fatalf("repair help is not runnable: %v %q", err, output.String())
	}
}

func TestShellInstallTopLevelCommandsHaveNoEffects(t *testing.T) {
	// An absent bus prevents a regression from reaching the real user manager.
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	for _, verb := range []string{"help", "--help", "typo", "check", "internal-typo", "internal-internal-install"} {
		t.Run(verb, func(t *testing.T) {
			var output bytes.Buffer
			err := RunShell([]string{verb}, &output)
			if verb == "help" || verb == "--help" {
				if err != nil || output.String() != shellInstallHelp {
					t.Fatalf("help reached runtime instead of printing usage: %v %q", err, output.String())
				}
			} else if err == nil || !strings.Contains(err.Error(), "unknown shell command") || !strings.Contains(err.Error(), "gentle-ai shell --help") {
				t.Fatalf("unknown verb reached runtime instead of naming help: %v", err)
			}
		})
	}
}

func TestShellInstallInternalCommandsRetainKernelChecks(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	t.Setenv("GENTLE_SHELL_UNIT", "")
	t.Setenv("GENTLE_SHELL_UNIT_RECEIPT", "")
	for _, verb := range []string{"internal-check", "internal-install", "internal-launch", "internal-recover"} {
		t.Run(verb, func(t *testing.T) {
			// No operands: cannot install, launch or recover, even on a qualified host.
			want := shellinstaller.RunUserEntry(context.Background(), "", []string{verb}, nil, io.Discard, io.Discard)
			got := RunShell([]string{verb}, io.Discard)
			if (got == nil) != (want == nil) || (got != nil && got.Error() != want.Error()) {
				t.Fatalf("internal route bypassed or blocked kernel checks: got %v, want %v", got, want)
			}
		})
	}
}
