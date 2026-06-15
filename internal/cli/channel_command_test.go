package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/state"
)

func unsetChannelEnv(t *testing.T) {
	t.Helper()
	previous, hadPrevious := os.LookupEnv(channelEnvVar)
	if err := os.Unsetenv(channelEnvVar); err != nil {
		t.Fatalf("Unsetenv: %v", err)
	}
	t.Cleanup(func() {
		if hadPrevious {
			_ = os.Setenv(channelEnvVar, previous)
			return
		}
		_ = os.Unsetenv(channelEnvVar)
	})
}

// isolateChannelResolution makes ResolveInstallChannel deterministic regardless of
// the host's ~/.gentle-ai/channel file and GENTLE_AI_CHANNEL value. It points HOME
// at a fresh empty temp dir (so the saved-channel lookup finds nothing and degrades
// to stable) and unsets the channel env var. Tests that exercise the default install
// channel must call this so they pass on machines with a persisted channel.
func isolateChannelResolution(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	unsetChannelEnv(t)
}

func TestRunChannelStablePersistsWithoutInstallingTarget(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	unsetChannelEnv(t)

	origGoInstall := channelGoInstall
	origExecutable := channelExecutable
	t.Cleanup(func() {
		channelGoInstall = origGoInstall
		channelExecutable = origExecutable
	})

	channelGoInstall = func(target, binDir string) error {
		t.Fatalf("stable channel must not require go install; got target=%q binDir=%q", target, binDir)
		return nil
	}
	launcherPath := filepath.Join(home, "launcher", "gentle-ai")
	channelExecutable = func() (string, error) { return launcherPath, nil }

	var out bytes.Buffer
	if err := RunChannel([]string{"stable"}, &out); err != nil {
		t.Fatalf("RunChannel(stable): %v", err)
	}
	data, err := os.ReadFile(filepath.Join(home, ".gentle-ai", "channel"))
	if err != nil {
		t.Fatalf("read channel file: %v", err)
	}
	if string(data) != "stable\n" {
		t.Fatalf("channel file = %q, want stable", string(data))
	}
	if !bytes.Contains(out.Bytes(), []byte("Binary: "+launcherPath)) {
		t.Fatalf("output = %q, want launcher binary path %q", out.String(), launcherPath)
	}
}

func TestRunChannelInstallsTargetAndPersistsChannel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	unsetChannelEnv(t)

	origGoInstall := channelGoInstall
	origGoAvailable := goToolchainAvailable
	t.Cleanup(func() {
		channelGoInstall = origGoInstall
		goToolchainAvailable = origGoAvailable
	})
	// Pin the toolchain probe so the test never depends on the host having Go.
	goToolchainAvailable = func() bool { return true }

	var gotTarget string
	var gotBinDir string
	channelGoInstall = func(target, binDir string) error {
		gotTarget = target
		gotBinDir = binDir
		if err := os.MkdirAll(binDir, 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(binDir, "gentle-ai"), []byte("fake"), 0o755)
	}

	var out bytes.Buffer
	if err := RunChannel([]string{"beta"}, &out); err != nil {
		t.Fatalf("RunChannel(beta): %v", err)
	}
	if gotTarget != "github.com/gentleman-programming/gentle-ai/cmd/gentle-ai@main" {
		t.Fatalf("target = %q, want beta @main", gotTarget)
	}
	wantBinDir := filepath.Join(home, ".gentle-ai", "channels", "beta")
	if gotBinDir != wantBinDir {
		t.Fatalf("binDir = %q, want %q", gotBinDir, wantBinDir)
	}
	data, err := os.ReadFile(filepath.Join(home, ".gentle-ai", "channel"))
	if err != nil {
		t.Fatalf("read channel file: %v", err)
	}
	if string(data) != "beta\n" {
		t.Fatalf("channel file = %q, want beta", string(data))
	}
}

func TestMaybeExecChannelTargetDoesNotDelegateStable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	unsetChannelEnv(t)
	if err := SaveInstallChannel(ChannelStable); err != nil {
		t.Fatalf("SaveInstallChannel: %v", err)
	}
	target, err := ChannelBinaryPath(ChannelStable)
	if err != nil {
		t.Fatalf("ChannelBinaryPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(target, []byte("old stable target"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	dispatched, err := MaybeExecChannelTarget([]string{"version"})
	if err != nil {
		t.Fatalf("MaybeExecChannelTarget: %v", err)
	}
	if dispatched {
		t.Fatal("stable channel should stay on the launcher instead of dispatching to a managed target")
	}
}

func TestMaybeExecChannelTargetSkipsLauncherOwnedCommands(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	unsetChannelEnv(t)
	if err := SaveInstallChannel(ChannelBeta); err != nil {
		t.Fatalf("SaveInstallChannel: %v", err)
	}
	target, err := ChannelBinaryPath(ChannelBeta)
	if err != nil {
		t.Fatalf("ChannelBinaryPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(target, []byte("fake"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	launcherOwned := [][]string{
		{"channel", "stable"},
		{"update"},
		{"upgrade"},
		// Identity and help commands are launcher-owned so the launcher's own
		// handlers stay reachable on a non-stable channel.
		{"version"},
		{"--version"},
		{"-v"},
		{"help"},
		{"--help"},
		{"-h"},
	}
	for _, args := range launcherOwned {
		dispatched, err := MaybeExecChannelTarget(args)
		if err != nil {
			t.Fatalf("MaybeExecChannelTarget(%v): %v", args, err)
		}
		if dispatched {
			t.Fatalf("%v must stay in launcher and not dispatch to target", args)
		}
	}
}

func TestMaybeExecChannelTargetDelegatesWhenTargetExists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	unsetChannelEnv(t)
	if err := SaveInstallChannel(ChannelBeta); err != nil {
		t.Fatalf("SaveInstallChannel: %v", err)
	}
	target, err := ChannelBinaryPath(ChannelBeta)
	if err != nil {
		t.Fatalf("ChannelBinaryPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(target, []byte("fake"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	origExecutable := channelExecutable
	origRun := channelRun
	t.Cleanup(func() {
		channelExecutable = origExecutable
		channelRun = origRun
	})
	channelExecutable = func() (string, error) { return filepath.Join(home, "launcher", "gentle-ai"), nil }

	var gotTarget string
	var gotArgs []string
	var gotEnv []string
	channelRun = func(target string, args []string, env []string) error {
		gotTarget = target
		gotArgs = append([]string(nil), args...)
		gotEnv = append([]string(nil), env...)
		return nil
	}

	// Use a delegated command (not launcher-owned): version/help/channel/update/
	// upgrade stay on the launcher, so they would not exercise the dispatch path.
	dispatched, err := MaybeExecChannelTarget([]string{"sync"})
	if err != nil {
		t.Fatalf("MaybeExecChannelTarget: %v", err)
	}
	if !dispatched {
		t.Fatal("expected dispatch to beta target")
	}
	if gotTarget != target {
		t.Fatalf("target = %q, want %q", gotTarget, target)
	}
	if len(gotArgs) != 1 || gotArgs[0] != "sync" {
		t.Fatalf("args = %v, want [sync]", gotArgs)
	}
	if !containsEnv(gotEnv, channelExecEnv+"=1") {
		t.Fatalf("env missing %s=1", channelExecEnv)
	}
	if !containsEnv(gotEnv, "GENTLE_AI_NO_SELF_UPDATE=1") {
		t.Fatal("delegated channel targets must not run self-update; env missing GENTLE_AI_NO_SELF_UPDATE=1")
	}
}

func containsEnv(env []string, want string) bool {
	for _, entry := range env {
		if entry == want {
			return true
		}
	}
	return false
}

// fakeExitError implements the ExitCode() int interface that *exec.ExitError
// satisfies, so exit-code propagation can be tested hermetically without
// spawning a real process.
type fakeExitError struct {
	code int
}

func (e fakeExitError) Error() string { return fmt.Sprintf("exit status %d", e.code) }
func (e fakeExitError) ExitCode() int { return e.code }

// TestMaybeExecChannelTargetPropagatesChildExitCode verifies that when the
// delegated channel binary exits non-zero, the launcher exits transparently with
// the child's exit code instead of collapsing every failure to 1 with a spurious
// "Error:" wrapper line. Non-exit errors (e.g. binary not found) must still bubble
// up as real errors so the top-level error handler reports them.
func TestMaybeExecChannelTargetPropagatesChildExitCode(t *testing.T) {
	tests := []struct {
		name         string
		runErr       error
		wantExit     bool
		wantExitCode int
		wantErr      bool
	}{
		{name: "child exits non-zero propagates code", runErr: fakeExitError{code: 42}, wantExit: true, wantExitCode: 42},
		{name: "child exits with 2 propagates 2", runErr: fakeExitError{code: 2}, wantExit: true, wantExitCode: 2},
		// A signal-terminated child reports ExitCode() == -1; os.Exit would coerce that
		// to 255, so the launcher clamps any negative code to a conventional 1.
		{name: "signal-terminated child clamps -1 to 1", runErr: fakeExitError{code: -1}, wantExit: true, wantExitCode: 1},
		{name: "non-exit error bubbles up", runErr: errors.New("exec: \"gentle-ai\": executable file not found"), wantErr: true},
		{name: "success neither exits nor errors", runErr: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			unsetChannelEnv(t)
			if err := SaveInstallChannel(ChannelBeta); err != nil {
				t.Fatalf("SaveInstallChannel: %v", err)
			}
			target, err := ChannelBinaryPath(ChannelBeta)
			if err != nil {
				t.Fatalf("ChannelBinaryPath: %v", err)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}
			if err := os.WriteFile(target, []byte("fake"), 0o755); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}

			origExecutable := channelExecutable
			origRun := channelRun
			origExit := channelExit
			t.Cleanup(func() {
				channelExecutable = origExecutable
				channelRun = origRun
				channelExit = origExit
			})
			channelExecutable = func() (string, error) { return filepath.Join(home, "launcher", "gentle-ai"), nil }
			channelRun = func(string, []string, []string) error { return tt.runErr }

			var gotExit bool
			var gotCode int
			channelExit = func(code int) {
				gotExit = true
				gotCode = code
			}

			dispatched, err := MaybeExecChannelTarget([]string{"sync"})
			if !dispatched {
				t.Fatalf("expected dispatch to beta target")
			}
			if gotExit != tt.wantExit {
				t.Fatalf("channelExit called = %v, want %v", gotExit, tt.wantExit)
			}
			if tt.wantExit && gotCode != tt.wantExitCode {
				t.Fatalf("exit code = %d, want %d", gotCode, tt.wantExitCode)
			}
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected a non-exit error to bubble up, got nil")
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestChannelBinaryStale(t *testing.T) {
	dir := t.TempDir()
	launcher := filepath.Join(dir, "launcher")
	channelBin := filepath.Join(dir, "channel")
	for _, p := range []string{launcher, channelBin} {
		if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
			t.Fatalf("WriteFile %s: %v", p, err)
		}
	}
	base := time.Unix(1_700_000_000, 0)
	older := base
	newer := base.Add(time.Hour)

	tests := []struct {
		name          string
		launcherMtime time.Time
		channelMtime  time.Time
		want          bool
	}{
		{name: "launcher newer than channel binary is stale", launcherMtime: newer, channelMtime: older, want: true},
		{name: "channel binary newer is fresh", launcherMtime: older, channelMtime: newer, want: false},
		{name: "equal mtimes are not stale", launcherMtime: base, channelMtime: base, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := os.Chtimes(launcher, tt.launcherMtime, tt.launcherMtime); err != nil {
				t.Fatalf("Chtimes launcher: %v", err)
			}
			if err := os.Chtimes(channelBin, tt.channelMtime, tt.channelMtime); err != nil {
				t.Fatalf("Chtimes channel: %v", err)
			}
			if got := channelBinaryStale(launcher, channelBin); got != tt.want {
				t.Fatalf("channelBinaryStale = %v, want %v", got, tt.want)
			}
		})
	}

	// A missing path degrades to "not stale" so it never produces a spurious warning.
	if channelBinaryStale(filepath.Join(dir, "missing"), channelBin) {
		t.Fatal("missing launcher must not be reported stale")
	}
	if channelBinaryStale(launcher, filepath.Join(dir, "missing")) {
		t.Fatal("missing channel binary must not be reported stale")
	}
}

func TestMaybeExecChannelTargetWarnsWhenChannelBinaryStale(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	unsetChannelEnv(t)
	if err := SaveInstallChannel(ChannelBeta); err != nil {
		t.Fatalf("SaveInstallChannel: %v", err)
	}
	target, err := ChannelBinaryPath(ChannelBeta)
	if err != nil {
		t.Fatalf("ChannelBinaryPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(target, []byte("stale beta"), 0o755); err != nil {
		t.Fatalf("WriteFile target: %v", err)
	}
	launcher := filepath.Join(home, "launcher", "gentle-ai")
	if err := os.MkdirAll(filepath.Dir(launcher), 0o755); err != nil {
		t.Fatalf("MkdirAll launcher: %v", err)
	}
	if err := os.WriteFile(launcher, []byte("new launcher"), 0o755); err != nil {
		t.Fatalf("WriteFile launcher: %v", err)
	}
	// Launcher rebuilt after the channel binary (the brew-upgrade signature).
	base := time.Unix(1_700_000_000, 0)
	if err := os.Chtimes(target, base, base); err != nil {
		t.Fatalf("Chtimes target: %v", err)
	}
	if err := os.Chtimes(launcher, base.Add(time.Hour), base.Add(time.Hour)); err != nil {
		t.Fatalf("Chtimes launcher: %v", err)
	}

	origExecutable := channelExecutable
	origRun := channelRun
	origStderr := channelStderr
	var warn bytes.Buffer
	t.Cleanup(func() {
		channelExecutable = origExecutable
		channelRun = origRun
		channelStderr = origStderr
	})
	channelExecutable = func() (string, error) { return launcher, nil }
	channelRun = func(string, []string, []string) error { return nil }
	channelStderr = &warn

	dispatched, err := MaybeExecChannelTarget([]string{"sync"})
	if err != nil {
		t.Fatalf("MaybeExecChannelTarget: %v", err)
	}
	if !dispatched {
		t.Fatal("expected delegation to the beta target")
	}
	if !strings.Contains(warn.String(), "older than the launcher") {
		t.Fatalf("expected staleness warning, got %q", warn.String())
	}

	// When the channel binary is newer than the launcher, no warning is emitted.
	warn.Reset()
	if err := os.Chtimes(target, base.Add(2*time.Hour), base.Add(2*time.Hour)); err != nil {
		t.Fatalf("Chtimes target fresh: %v", err)
	}
	if _, err := MaybeExecChannelTarget([]string{"sync"}); err != nil {
		t.Fatalf("MaybeExecChannelTarget fresh: %v", err)
	}
	if warn.Len() != 0 {
		t.Fatalf("fresh channel binary must not warn, got %q", warn.String())
	}
}

func TestMaybeExecChannelTargetStaleWarningCooldown(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	unsetChannelEnv(t)
	if err := SaveInstallChannel(ChannelBeta); err != nil {
		t.Fatalf("SaveInstallChannel: %v", err)
	}
	target, err := ChannelBinaryPath(ChannelBeta)
	if err != nil {
		t.Fatalf("ChannelBinaryPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(target, []byte("stale beta"), 0o755); err != nil {
		t.Fatalf("WriteFile target: %v", err)
	}
	launcher := filepath.Join(home, "launcher", "gentle-ai")
	if err := os.MkdirAll(filepath.Dir(launcher), 0o755); err != nil {
		t.Fatalf("MkdirAll launcher: %v", err)
	}
	if err := os.WriteFile(launcher, []byte("new launcher"), 0o755); err != nil {
		t.Fatalf("WriteFile launcher: %v", err)
	}
	base := time.Unix(1_700_000_000, 0)
	if err := os.Chtimes(target, base, base); err != nil {
		t.Fatalf("Chtimes target: %v", err)
	}
	if err := os.Chtimes(launcher, base.Add(time.Hour), base.Add(time.Hour)); err != nil {
		t.Fatalf("Chtimes launcher: %v", err)
	}

	origExecutable := channelExecutable
	origRun := channelRun
	origStderr := channelStderr
	origNow := channelStaleNow
	var warn bytes.Buffer
	now := base.Add(48 * time.Hour) // well past any prior warning
	t.Cleanup(func() {
		channelExecutable = origExecutable
		channelRun = origRun
		channelStderr = origStderr
		channelStaleNow = origNow
	})
	channelExecutable = func() (string, error) { return launcher, nil }
	channelRun = func(string, []string, []string) error { return nil }
	channelStderr = &warn
	channelStaleNow = func() time.Time { return now }

	warned := func() bool {
		warn.Reset()
		if _, err := MaybeExecChannelTarget([]string{"sync"}); err != nil {
			t.Fatalf("MaybeExecChannelTarget: %v", err)
		}
		return strings.Contains(warn.String(), "older than the launcher")
	}

	// First stale launch: warns and records the timestamp.
	if !warned() {
		t.Fatal("first stale launch must warn")
	}
	// Second launch within the 24h window: suppressed by cooldown.
	if warned() {
		t.Fatal("second launch within cooldown must not warn")
	}
	// Advance past the TTL: warns again.
	now = now.Add(channelStaleWarnTTL + time.Minute)
	if !warned() {
		t.Fatal("launch after the cooldown window must warn again")
	}
}

// TestRecordChannelStaleWarningRecoversFromCorruptState verifies that a corrupt
// (non-NotExist) state.json does not defeat the once-per-day cooldown: recording must
// overwrite the unreadable file with a fresh state carrying the new timestamp so a
// subsequent shouldWarnChannelStale is cooled down within the TTL. This exercises the
// REAL state.Read path (no seam) across every corruption shape state.Read classifies
// as ErrCorruptState — structurally-broken JSON AND a corrupt timestamp in the
// *time.Time field. A corrupt timestamp surfaces as *time.ParseError (not a json
// type error), so classifying via the ErrCorruptState sentinel — rather than
// enumerating concrete json error types — is what keeps the cooldown working.
func TestRecordChannelStaleWarningRecoversFromCorruptState(t *testing.T) {
	corruptBlobs := map[string]string{
		"structurally broken json":    "{ this is not valid json",
		"malformed RFC3339 timestamp": `{"last_channel_stale_warning":"not-a-timestamp"}`,
		"wrong-typed timestamp":       `{"last_channel_stale_warning":12345}`,
	}
	for name, blob := range corruptBlobs {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			unsetChannelEnv(t)

			// Seed a corrupt state.json so the REAL state.Read returns a non-NotExist
			// decode error (wrapped with ErrCorruptState), exercising the recovery branch.
			stateDir := filepath.Join(home, ".gentle-ai")
			if err := os.MkdirAll(stateDir, 0o755); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}
			if err := os.WriteFile(filepath.Join(stateDir, "state.json"), []byte(blob), 0o644); err != nil {
				t.Fatalf("WriteFile corrupt state: %v", err)
			}

			origNow := channelStaleNow
			t.Cleanup(func() { channelStaleNow = origNow })
			now := time.Unix(1_700_000_000, 0)
			channelStaleNow = func() time.Time { return now }

			recordChannelStaleWarning(home, now)

			// The timestamp must have been persisted despite the prior corrupt file.
			persisted, err := state.Read(home)
			if err != nil {
				t.Fatalf("state.Read after record: %v (corrupt state must be overwritten)", err)
			}
			if persisted.LastChannelStaleWarning == nil {
				t.Fatal("LastChannelStaleWarning was not persisted; cooldown will never record on corrupt state")
			}
			if !persisted.LastChannelStaleWarning.Equal(now) {
				t.Fatalf("persisted timestamp = %v, want %v", persisted.LastChannelStaleWarning, now)
			}

			// Within the TTL the warning must now be suppressed (cooldown took effect).
			if shouldWarnChannelStale() {
				t.Fatal("warning should be cooled down within TTL after recording over corrupt state")
			}
		})
	}
}

// TestIsResettableStateError verifies the precise classification used by
// recordChannelStaleWarning: only a genuinely absent file (os.ErrNotExist) or a
// genuinely undecodable file (state.ErrCorruptState) may be reset to a fresh state.
// The classifier keys off the state.ErrCorruptState sentinel — state.Read is the
// single source of truth that wraps it around EVERY decode failure, including ones
// that surface as *time.ParseError or a plain errors.errorString (not just the
// concrete json error types). Every other read error (permission/IO) must be
// preserved so a valid-but-unreadable state.json is never clobbered.
func TestIsResettableStateError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil error is not resettable", err: nil, want: false},
		{name: "absent file is resettable", err: os.ErrNotExist, want: true},
		{name: "wrapped absent file is resettable", err: fmt.Errorf("open state: %w", os.ErrNotExist), want: true},
		{name: "ErrCorruptState sentinel is resettable", err: state.ErrCorruptState, want: true},
		{name: "wrapped ErrCorruptState is resettable", err: fmt.Errorf("read state: %w", state.ErrCorruptState), want: true},
		{name: "corrupt read from malformed JSON is resettable", err: corruptReadError(t, "{ not valid"), want: true},
		{name: "corrupt read from type mismatch is resettable", err: corruptReadError(t, `{"installed_agents": 5}`), want: true},
		{name: "corrupt read from malformed timestamp (time.ParseError) is resettable", err: corruptReadError(t, `{"last_channel_stale_warning":"not-a-timestamp"}`), want: true},
		{name: "corrupt read from wrong-typed timestamp is resettable", err: corruptReadError(t, `{"last_channel_stale_warning":12345}`), want: true},
		{name: "permission error is preserved", err: fmt.Errorf("open state: %w", os.ErrPermission), want: false},
		{name: "EISDIR-style IO error is preserved", err: &os.PathError{Op: "read", Path: "state.json", Err: syscall.EISDIR}, want: false},
		{name: "generic IO error is preserved", err: errors.New("input/output error"), want: false},
		{name: "bare json syntax error (not via state.Read) is preserved", err: jsonSyntaxError(t), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isResettableStateError(tt.err); got != tt.want {
				t.Fatalf("isResettableStateError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// corruptReadError writes the given blob to a real state.json under a temp home and
// returns the error state.Read produces for it. This exercises the actual sentinel
// wrapping (including nested *time.Time decode failures) rather than a hand-built
// error, so the classifier is tested against what production code really returns.
func corruptReadError(t *testing.T, blob string) error {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".gentle-ai"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(state.Path(home), []byte(blob), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := state.Read(home)
	if err == nil {
		t.Fatalf("state.Read(%q) expected a corrupt-decode error, got nil", blob)
	}
	return err
}

// jsonSyntaxError returns a bare *json.SyntaxError NOT routed through state.Read.
// Under the sentinel contract this is NOT resettable on its own: only errors that
// state.Read wraps with ErrCorruptState qualify, so a raw json error from some other
// source must be treated conservatively (preserve).
func jsonSyntaxError(t *testing.T) error {
	t.Helper()
	var v map[string]any
	err := json.Unmarshal([]byte("{ not valid"), &v)
	if err == nil {
		t.Fatal("expected a JSON syntax error")
	}
	return err
}

// TestRecordChannelStaleWarningPreservesStateOnNonDecodeReadError verifies the
// regression fix: when state.Read fails with a NON-decode error (a transient
// IO/permission failure) against a state.json that actually holds valid persisted
// data, recordChannelStaleWarning must NOT write — so InstalledAgents, Persona,
// PendingSync, etc. are never clobbered by an unreadable-this-once read. The read
// failure is injected via the channelStateRead seam because such errors are not
// portably reproducible against the real filesystem.
func TestRecordChannelStaleWarningPreservesStateOnNonDecodeReadError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	unsetChannelEnv(t)

	// Seed a genuinely valid state.json with populated fields that must survive.
	valid := state.InstallState{
		InstalledAgents: []string{"claude-code", "opencode"},
		Persona:         "gentleman",
		PendingSync:     true,
	}
	if err := state.Write(home, valid); err != nil {
		t.Fatalf("seed valid state: %v", err)
	}

	origRead := channelStateRead
	t.Cleanup(func() { channelStateRead = origRead })
	// Simulate a transient IO/permission read failure (not a decode error, not NotExist).
	channelStateRead = func(string) (state.InstallState, error) {
		return state.InstallState{}, &os.PathError{Op: "open", Path: state.Path(home), Err: os.ErrPermission}
	}

	now := time.Unix(1_700_000_000, 0)
	recordChannelStaleWarning(home, now)

	// The conservative branch must NOT have written: the on-disk state is untouched.
	// Read with the real reader (restore happens at cleanup, so read directly).
	persisted, err := origRead(home)
	if err != nil {
		t.Fatalf("state.Read after record: %v", err)
	}
	if len(persisted.InstalledAgents) != 2 || persisted.InstalledAgents[0] != "claude-code" || persisted.InstalledAgents[1] != "opencode" {
		t.Fatalf("InstalledAgents clobbered: got %v, want [claude-code opencode]", persisted.InstalledAgents)
	}
	if persisted.Persona != "gentleman" {
		t.Fatalf("Persona clobbered: got %q, want gentleman", persisted.Persona)
	}
	if !persisted.PendingSync {
		t.Fatal("PendingSync clobbered: got false, want true")
	}
	if persisted.LastChannelStaleWarning != nil {
		t.Fatal("timestamp was written on a non-decode read error; the conservative branch must not write")
	}
}

// writeSavedChannelFile writes raw bytes to ~/.gentle-ai/channel under the test's
// HOME so tests can simulate persisted (or corrupt) channel preferences.
func writeSavedChannelFile(t *testing.T, contents string) {
	t.Helper()
	path, err := channelFilePath()
	if err != nil {
		t.Fatalf("channelFilePath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// TestResolveInstallChannelSavedFileDegradesGracefully verifies that the saved
// channel file is a best-effort preference cache: a corrupt/garbage value must
// degrade to stable without ever surfacing an error (FIX 1), while a valid saved
// value is honored.
func TestResolveInstallChannelSavedFileDegradesGracefully(t *testing.T) {
	tests := []struct {
		name  string
		saved string
		want  InstallChannel
	}{
		{name: "garbage value degrades to stable", saved: "garbage", want: ChannelStable},
		{name: "empty file degrades to stable", saved: "", want: ChannelStable},
		{name: "whitespace file degrades to stable", saved: "   \n", want: ChannelStable},
		{name: "valid beta is honored", saved: "beta\n", want: ChannelBeta},
		{name: "valid stable is honored", saved: "stable\n", want: ChannelStable},
		{name: "nightly alias resolves to beta", saved: "nightly\n", want: ChannelBeta},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			unsetChannelEnv(t)
			writeSavedChannelFile(t, tt.saved)

			got, err := ResolveInstallChannel("")
			if err != nil {
				t.Fatalf("ResolveInstallChannel(\"\") error = %v, want nil (saved file must never error)", err)
			}
			if got != tt.want {
				t.Fatalf("ResolveInstallChannel(\"\") = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestMaybeExecChannelTargetCorruptSavedFileStaysOnLauncher verifies that a garbage
// saved channel file does not brick the binary: dispatch resolves to stable and the
// launcher keeps control with no error (FIX 1).
func TestMaybeExecChannelTargetCorruptSavedFileStaysOnLauncher(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	unsetChannelEnv(t)
	writeSavedChannelFile(t, "garbage")

	dispatched, err := MaybeExecChannelTarget(nil)
	if err != nil {
		t.Fatalf("MaybeExecChannelTarget(nil) error = %v, want nil", err)
	}
	if dispatched {
		t.Fatal("corrupt saved channel must degrade to stable and stay on the launcher")
	}
}

// TestResolveInstallChannelEmptyEnvFallsThroughToSavedFile verifies that an
// empty/whitespace GENTLE_AI_CHANNEL is treated as unset and falls through to the
// saved-file preference instead of short-circuiting to stable (FIX 3).
func TestResolveInstallChannelEmptyEnvFallsThroughToSavedFile(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
	}{
		{name: "empty env falls through", envValue: ""},
		{name: "whitespace env falls through", envValue: "   "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv(channelEnvVar, tt.envValue)
			writeSavedChannelFile(t, "beta\n")

			got, err := ResolveInstallChannel("")
			if err != nil {
				t.Fatalf("ResolveInstallChannel(\"\") error = %v", err)
			}
			if got != ChannelBeta {
				t.Fatalf("ResolveInstallChannel(\"\") = %q, want beta (env must fall through to saved file)", got)
			}
		})
	}
}

// TestRunChannelBetaRequiresGoToolchain verifies that activating beta without a Go
// toolchain fails with an actionable message (mirroring the install script) instead
// of a raw exec error, never runs go install, and never persists the channel.
func TestRunChannelBetaRequiresGoToolchain(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	unsetChannelEnv(t)

	origGoInstall := channelGoInstall
	origGoAvailable := goToolchainAvailable
	t.Cleanup(func() {
		channelGoInstall = origGoInstall
		goToolchainAvailable = origGoAvailable
	})
	goToolchainAvailable = func() bool { return false }
	channelGoInstall = func(target, binDir string) error {
		t.Fatalf("must not attempt go install when Go is unavailable; got target=%q", target)
		return nil
	}

	var out bytes.Buffer
	err := RunChannel([]string{"beta"}, &out)
	if err == nil {
		t.Fatal("RunChannel(beta) without Go must return an error")
	}
	if !strings.Contains(err.Error(), "requires Go 1.24+") || !strings.Contains(err.Error(), "gentle-ai channel beta") {
		t.Fatalf("error = %q, want actionable Go-toolchain guidance", err.Error())
	}
	// A failed activation must not persist the channel preference.
	if _, statErr := os.Stat(filepath.Join(home, ".gentle-ai", "channel")); !os.IsNotExist(statErr) {
		t.Fatalf("channel must not be persisted on failed activation, stat err = %v", statErr)
	}
}

// TestRunChannelNoArgShowsCurrentChannel verifies the no-arg show path reports the
// resolved channel without mutating the saved preference.
func TestRunChannelNoArgShowsCurrentChannel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	unsetChannelEnv(t)
	writeSavedChannelFile(t, "stable\n")

	origGoInstall := channelGoInstall
	origExecutable := channelExecutable
	t.Cleanup(func() {
		channelGoInstall = origGoInstall
		channelExecutable = origExecutable
	})
	channelGoInstall = func(target, binDir string) error {
		t.Fatalf("show path must not install anything; got target=%q binDir=%q", target, binDir)
		return nil
	}
	launcherPath := filepath.Join(home, "launcher", "gentle-ai")
	channelExecutable = func() (string, error) { return launcherPath, nil }

	var out bytes.Buffer
	if err := RunChannel(nil, &out); err != nil {
		t.Fatalf("RunChannel(nil): %v", err)
	}
	if !bytes.Contains(out.Bytes(), []byte("Gentle AI channel: stable")) {
		t.Fatalf("output = %q, want current channel stable", out.String())
	}
	// The show path must not have written or changed the saved file.
	data, err := os.ReadFile(filepath.Join(home, ".gentle-ai", "channel"))
	if err != nil {
		t.Fatalf("read channel file: %v", err)
	}
	if string(data) != "stable\n" {
		t.Fatalf("channel file = %q, want unchanged stable", string(data))
	}
}

// TestRunChannelRejectsInvalidInput verifies that an explicit invalid channel
// argument is rejected with an error (FIX 4 messaging) and never persisted.
func TestRunChannelRejectsInvalidInput(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	unsetChannelEnv(t)

	var out bytes.Buffer
	err := RunChannel([]string{"engram-beta"}, &out)
	if err == nil {
		t.Fatal("RunChannel(engram-beta) error = nil, want rejection of invalid channel")
	}
	if !strings.Contains(err.Error(), "unsupported Gentle AI channel") {
		t.Fatalf("error = %q, want unsupported-channel message", err.Error())
	}
	// Invalid input must not create the saved channel file.
	if _, statErr := os.Stat(filepath.Join(home, ".gentle-ai", "channel")); !os.IsNotExist(statErr) {
		t.Fatalf("channel file should not exist after invalid input; stat err = %v", statErr)
	}
}
