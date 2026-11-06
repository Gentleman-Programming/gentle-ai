package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
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

func TestShellInstallLaunchProbeIsWired(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the inline launch probe is Linux-only")
	}
	if reflect.ValueOf(shellLaunchProbe).Pointer() != reflect.ValueOf(shellinstaller.RunUserLaunchProbe).Pointer() {
		t.Fatal("inline install does not probe the delegated unit path before claiming success")
	}
}

func TestShellPNPMLaunchHelp(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("PNPM dispatch is Linux-only")
	}
	var output bytes.Buffer
	if err := RunShell([]string{"--help"}, &output); err != nil || !strings.Contains(output.String(), "launch-pnpm ROOT pi|gentle-shell [ARGS...]") || !strings.Contains(output.String(), "shell launch ROOT [PI_ARGS...]") {
		t.Fatal("missing PNPM or legacy launch help", err, output.String())
	}
}

// shellInlineHarness drives RunShellInstallInline's Linux route with a fake
// direct worker that publishes like the real one: destination tree, the
// ~/.local/bin launchers and the registry entry.
type shellInlineHarness struct {
	home, destination string
	calls             []string
	reports           []shellinstaller.UserInstallProgress
}

func newShellInlineHarness(t *testing.T, probe error) *shellInlineHarness {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the inline direct worker and launch probe are Linux routes")
	}
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", filepath.Join(home, "emptybin"))
	t.Setenv("GENTLE_AI_SHELL_REGISTRY_HOME", home)
	// gentle-ai's own state writers leave ~/.gentle-ai at 0755.
	if err := os.Mkdir(filepath.Join(home, ".gentle-ai"), 0755); err != nil {
		t.Fatal(err)
	}
	h := &shellInlineHarness{home: home, destination: filepath.Join(home, ".local", "share", "gentle-shell", "install")}
	oldDirect, oldWorker, oldProbe, oldInspect := shellInlineDirect, shellDirectWorker, shellLaunchProbe, shellInstallInspect
	t.Cleanup(func() {
		shellInlineDirect, shellDirectWorker, shellLaunchProbe, shellInstallInspect = oldDirect, oldWorker, oldProbe, oldInspect
	})
	shellInlineDirect = true
	shellInstallInspect = func(shellinstaller.UserInstallRequest) (string, error) { return "token", nil }
	shellLaunchProbe = func(context.Context, string) error {
		h.calls = append(h.calls, "probe")
		return probe
	}
	shellDirectWorker = func(_ context.Context, _ string, args []string, stdout, _ io.Writer) error {
		h.calls = append(h.calls, "worker")
		if len(args) == 0 || args[0] != h.destination {
			t.Errorf("direct worker args = %q, want the destination first", args)
		}
		bin := filepath.Join(h.destination, "bin")
		if err := os.MkdirAll(bin, 0700); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(bin, "gentle-shell"), []byte("#!/bin/sh\n"), 0700); err != nil {
			return err
		}
		manifest := `{"Schema":"gentle-shell-user-install/v1","Destination":"` + h.destination + `","Mode":"separate"}`
		if err := os.WriteFile(filepath.Join(h.destination, "installation.json"), []byte(manifest), 0600); err != nil {
			return err
		}
		if err := shellinstaller.RecordUserShellInstall(home, shellinstaller.UserShellInstall{Target: h.destination, Mode: "separate", Channel: "stable"}); err != nil {
			return err
		}
		links := filepath.Join(home, ".local", "bin")
		if err := os.MkdirAll(links, 0755); err != nil {
			return err
		}
		for _, name := range []string{"gentle-shell", "gs"} {
			if err := os.Symlink(filepath.Join(bin, "gentle-shell"), filepath.Join(links, name)); err != nil {
				return err
			}
		}
		for index, label := range []string{"Validate the selection and target", "Prepare the private stage and preferences", "Install the runtime and pinned suppliers", "Apply optional runtimes and the SDK channel", "Publish, verify and register the installation"} {
			if _, err := io.WriteString(stdout, "✓ "+strconv.Itoa(index+1)+"/5 "+label+"\n"); err != nil {
				return err
			}
		}
		return nil
	}
	return h
}

func (h *shellInlineHarness) run() error {
	selection := ShellInstallSelection{Request: shellinstaller.UserInstallRequest{Destination: h.destination, Mode: "separate", Confirmation: "token"}, Confirmed: true, Executed: true}
	return RunShellInstallInline(context.Background(), selection, func(progress shellinstaller.UserInstallProgress) {
		h.reports = append(h.reports, progress)
	}, io.Discard)
}

func (h *shellInlineHarness) launcher(name string) (string, bool) {
	target, err := os.Readlink(filepath.Join(h.home, ".local", "bin", name))
	return target, err == nil
}

func (h *shellInlineHarness) registered() []shellinstaller.UserShellInstall {
	var entries []shellinstaller.UserShellInstall
	for _, install := range shellinstaller.ReadUserShellInstalls(h.home) {
		if install.Target == h.destination {
			entries = append(entries, install)
		}
	}
	return entries
}

// A delegated unit that cannot run makes every later launch fail, so the
// refusal must come before the worker publishes anything (L7, L12).
func TestRunShellInstallInlineRefusesUnlaunchableUnitBeforeAnyEffect(t *testing.T) {
	h := newShellInlineHarness(t, errors.New("set XDG_RUNTIME_DIR to your existing /run/user/UID; delegated user manager required"))
	err := h.run()
	if err == nil || !strings.Contains(err.Error(), "delegated unit launch probe failed") || !strings.Contains(err.Error(), "XDG_RUNTIME_DIR") {
		t.Fatalf("unlaunchable unit not refused with its cause: %v", err)
	}
	if !reflect.DeepEqual(h.calls, []string{"probe"}) {
		t.Errorf("calls = %q, want the probe alone before any worker", h.calls)
	}
	if _, statErr := os.Lstat(h.destination); !os.IsNotExist(statErr) {
		t.Errorf("refused installation left its destination: %v", statErr)
	}
	for _, name := range []string{"gentle-shell", "gs"} {
		if target, ok := h.launcher(name); ok {
			t.Errorf("refused installation left launcher %s -> %s", name, target)
		}
	}
	if entries := h.registered(); len(entries) != 0 {
		t.Errorf("refused installation is registered: %+v", entries)
	}
	if installs := shellinstaller.DiscoverUserShellInstalls(h.home); len(installs) != 0 {
		t.Errorf("refused installation is discoverable as managed: %+v", installs)
	}
	for _, progress := range h.reports {
		if progress.State == "done" {
			t.Errorf("refused installation reported step progress: %+v", h.reports)
			break
		}
	}
}

func TestRunShellInstallInlinePublishedInstallRegistersAndKeepsLaunchers(t *testing.T) {
	h := newShellInlineHarness(t, nil)
	if err := h.run(); err != nil {
		t.Fatalf("qualified installation failed: %v", err)
	}
	if !reflect.DeepEqual(h.calls, []string{"probe", "worker"}) {
		t.Fatalf("calls = %q, want the probe then the worker", h.calls)
	}
	if _, err := os.Lstat(filepath.Join(h.destination, "installation.json")); err != nil {
		t.Fatalf("published destination missing: %v", err)
	}
	for _, name := range []string{"gentle-shell", "gs"} {
		if target, ok := h.launcher(name); !ok || target != filepath.Join(h.destination, "bin", "gentle-shell") {
			t.Fatalf("launcher %s = %q (present %v)", name, target, ok)
		}
	}
	entries := h.registered()
	if len(entries) != 1 || entries[0].Mode != "separate" || entries[0].Channel != "stable" {
		t.Fatalf("registry entries = %+v, want one separate/stable", entries)
	}
	if installs := shellinstaller.DiscoverUserShellInstalls(h.home); len(installs) != 1 || installs[0].Target != h.destination {
		t.Fatalf("published installation not discoverable: %+v", installs)
	}
	done := 0
	for _, progress := range h.reports {
		if progress.State == "done" {
			done++
		}
	}
	if done != 5 {
		t.Fatalf("progress = %+v, want 5 done steps", h.reports)
	}
}
