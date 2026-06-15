package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/state"
)

// channelStaleWarnTTL is the minimum time between stale-channel-binary warnings.
// The warning nudges once per day instead of on every delegated command.
const channelStaleWarnTTL = 24 * time.Hour

const channelExecEnv = "GENTLE_AI_CHANNEL_TARGET"

var (
	channelExecutable              = os.Executable
	channelRun                     = runChannelTarget
	channelGoInstall               = goInstallChannelBinary
	goToolchainAvailable           = defaultGoToolchainAvailable
	channelStderr        io.Writer = os.Stderr
	channelStaleNow                = time.Now
	channelExit                    = os.Exit
	// channelStateRead reads the persisted state. It is a seam so tests can inject a
	// non-decode read error (e.g. a transient IO/permission failure) deterministically,
	// which is not portably reproducible against the real filesystem.
	channelStateRead = state.Read
)

// defaultGoToolchainAvailable reports whether a `go` toolchain is on PATH. Non-stable
// channels are built from source via `go install ...@main`, so this is checked before
// attempting the build to surface an actionable message instead of a raw exec error.
func defaultGoToolchainAvailable() bool {
	_, err := exec.LookPath("go")
	return err == nil
}

// RunChannel shows or changes the active Gentle AI runtime channel.
func RunChannel(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		channel, err := ResolveInstallChannel("")
		if err != nil {
			return err
		}
		path, _ := ActiveChannelBinaryPath(channel)
		_, _ = fmt.Fprintf(stdout, "Gentle AI channel: %s\n", channel)
		if path != "" {
			_, _ = fmt.Fprintf(stdout, "Binary: %s\n", path)
		}
		return nil
	}
	if len(args) != 1 {
		return fmt.Errorf("usage: gentle-ai channel [stable|beta] (nightly = beta)")
	}

	channel, err := ResolveInstallChannel(args[0])
	if err != nil {
		return err
	}
	path, err := ActivateChannel(channel)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "Gentle AI channel set to %s\n", channel)
	_, _ = fmt.Fprintf(stdout, "Binary: %s\n", path)
	return nil
}

// MaybeExecChannelTarget delegates to the configured channel binary. The launcher
// keeps `gentle-ai channel ...` available even when the selected target binary does
// not yet include the channel command.
func MaybeExecChannelTarget(args []string) (bool, error) {
	if os.Getenv(channelExecEnv) == "1" {
		return false, nil
	}
	if len(args) > 0 && isLauncherOwnedCommand(args[0]) {
		return false, nil
	}
	channel, err := ResolveInstallChannel("")
	if err != nil {
		return false, err
	}
	if channel == ChannelStable {
		return false, nil
	}
	target, err := ChannelBinaryPath(channel)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(target); err != nil {
		return false, nil
	}
	current, err := channelExecutable()
	if err == nil {
		if sameExecutable(current, target) {
			return false, nil
		}
		// A launcher that is newer than the channel binary signals an out-of-band
		// launcher update (e.g. `brew upgrade gentle-ai`) that left the channel
		// binary stale. Warn — the launcher still delegates so the user keeps
		// working, but their channel build is no longer fresh. Gated to once per
		// day so it nudges without nagging on every delegated command.
		if channelBinaryStale(current, target) && shouldWarnChannelStale() {
			fmt.Fprintf(channelStderr, "warning: the %s channel binary is older than the launcher (e.g. after `brew upgrade`); run `gentle-ai channel %s` to rebuild it.\n", channel, channel)
		}
	}
	env := append(os.Environ(), channelExecEnv+"=1", "GENTLE_AI_NO_SELF_UPDATE=1")
	if err := channelRun(target, args, env); err != nil {
		// The delegated binary exited non-zero: propagate its exit code transparently
		// so the launcher mirrors the child instead of collapsing every failure to 1
		// (and printing a spurious "Error: exit status N" wrapper at the top level).
		var exitErr interface{ ExitCode() int }
		if errors.As(err, &exitErr) {
			// A signal-terminated child reports ExitCode() == -1, which os.Exit coerces
			// to 255 (a misleading code). Clamp any negative code to 1 so the launcher
			// reports a conventional non-zero failure instead.
			code := exitErr.ExitCode()
			if code < 0 {
				code = 1
			}
			channelExit(code)
			return true, nil
		}
		// Non-exit failures (e.g. the binary could not be started) are real errors —
		// surface them so the top-level handler reports them.
		return true, err
	}
	return true, nil
}

// shouldWarnChannelStale gates the stale-binary warning behind a once-per-day TTL
// using LastChannelStaleWarning in state.json. It returns true (and records the new
// timestamp) when a warning is due. Best-effort: if home resolution fails it warns
// without recording (fail-open); a future timestamp from clock skew never suppresses.
func shouldWarnChannelStale() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return true
	}
	now := channelStaleNow()
	if s, err := state.Read(home); err == nil && s.LastChannelStaleWarning != nil {
		if elapsed := now.Sub(*s.LastChannelStaleWarning); elapsed >= 0 && elapsed < channelStaleWarnTTL {
			return false
		}
	}
	recordChannelStaleWarning(home, now)
	return true
}

// recordChannelStaleWarning persists the warning timestamp, re-reading state first.
// The success path preserves all existing fields (only the timestamp is updated).
// When the existing state is genuinely absent (os.ErrNotExist) or genuinely corrupt
// (state.ErrCorruptState — the bytes decoded into nothing usable, so the data is
// already unrecoverable) it resets to a fresh state carrying the new timestamp, so the
// once-per-day cooldown still records (otherwise the warning would fire on every
// command). Any other read failure (filesystem/permission/IO) is conservative: it does
// NOT write, to avoid clobbering a valid state.json whose bytes were merely unreadable
// this once — the warning may repeat but no data is lost. Write errors are non-fatal
// (the next due launch retries).
func recordChannelStaleWarning(home string, now time.Time) {
	current, err := channelStateRead(home)
	if err != nil {
		if !isResettableStateError(err) {
			// Transient/permission/IO read error against a possibly-valid file: do not
			// write, so persisted fields are never clobbered by an unreadable-this-once read.
			return
		}
		// Absent or corrupt: the contents cannot be preserved, but the cooldown timestamp
		// must still be written so the warning does not nag every command.
		current = state.InstallState{}
	}
	current.LastChannelStaleWarning = &now
	_ = state.Write(home, current)
}

// isResettableStateError reports whether a state.Read error means the existing state
// can be safely replaced by a fresh one. Only two cases qualify: the file is genuinely
// absent (os.ErrNotExist), or its contents are genuinely undecodable
// (state.ErrCorruptState — the data is already lost). state.Read is the single source
// of truth for the corrupt classification: it wraps ErrCorruptState around ANY decode
// failure (malformed JSON, a type mismatch, or a nested failure such as a corrupt
// timestamp in a *time.Time field), so this no longer enumerates concrete json error
// types. Every other error (e.g. EACCES, EISDIR, an I/O failure) is treated as
// "preserve": the file may hold a valid state whose bytes were just unreadable this
// once, so overwriting it would clobber persisted user data.
func isResettableStateError(err error) bool {
	return errors.Is(err, os.ErrNotExist) || errors.Is(err, state.ErrCorruptState)
}

// channelBinaryStale reports whether the channel binary is older than the launcher
// executable — the signature of an out-of-band launcher upgrade (e.g. brew) that
// did not rebuild the channel binary. Best-effort: any stat failure returns false so
// a missing/unreadable mtime never produces a spurious warning.
func channelBinaryStale(launcherPath, channelBinaryPath string) bool {
	launcherInfo, err := os.Stat(launcherPath)
	if err != nil {
		return false
	}
	channelInfo, err := os.Stat(channelBinaryPath)
	if err != nil {
		return false
	}
	return launcherInfo.ModTime().After(channelInfo.ModTime())
}

func isLauncherOwnedCommand(command string) bool {
	switch command {
	// Channel management and self-update must always run on the launcher.
	case "channel", "update", "upgrade":
		return true
	// Identity and help are launcher-owned so the launcher's own version/help
	// handlers stay reachable even when a non-stable channel target is active.
	case "version", "--version", "-v", "help", "--help", "-h":
		return true
	default:
		return false
	}
}

func ActivateChannel(channel InstallChannel) (string, error) {
	if channel == ChannelStable {
		if err := SaveInstallChannel(channel); err != nil {
			return "", err
		}
		return ActiveChannelBinaryPath(channel)
	}
	path, err := InstallChannelBinary(channel)
	if err != nil {
		return "", err
	}
	if err := SaveInstallChannel(channel); err != nil {
		return "", err
	}
	return path, nil
}

func InstallChannelBinary(channel InstallChannel) (string, error) {
	// The beta channel builds from main with `go install`, which needs the Go
	// toolchain. Validate it up front so a missing Go yields an actionable message
	// (mirroring the install script) instead of a low-level "exec: go: not found".
	if !goToolchainAvailable() {
		return "", fmt.Errorf("the %s channel builds from main and requires Go 1.24+. Install Go, then run: gentle-ai channel %s", channel, channel)
	}
	path, err := ChannelBinaryPath(channel)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create channel binary dir: %w", err)
	}
	target := "github.com/gentleman-programming/gentle-ai/cmd/gentle-ai@latest"
	if channel.IsBeta() {
		target = "github.com/gentleman-programming/gentle-ai/cmd/gentle-ai@main"
	}
	if err := channelGoInstall(target, filepath.Dir(path)); err != nil {
		return "", fmt.Errorf("install Gentle AI %s binary: %w", channel, err)
	}
	return path, nil
}

func ActiveChannelBinaryPath(channel InstallChannel) (string, error) {
	path, err := ChannelBinaryPath(channel)
	if err != nil {
		return "", err
	}
	if channel == ChannelStable {
		current, err := channelExecutable()
		if err != nil {
			return "", err
		}
		return current, nil
	}
	return path, nil
}

func ChannelBinaryPath(channel InstallChannel) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}
	name := "gentle-ai"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(home, ".gentle-ai", "channels", string(channel), name), nil
}

func runChannelTarget(target string, args []string, env []string) error {
	cmd := exec.Command(target, args...)
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func goInstallChannelBinary(target, binDir string) error {
	// Derive the module from the install target (".../cmd/gentle-ai@main") so the
	// proxy/sumdb bypass forces `@main` to resolve to the true HEAD, matching the
	// self-upgrade path. GOBIN is appended last so it overrides any inherited value.
	module := target
	if i := strings.Index(target, "/cmd/"); i > 0 {
		module = target[:i]
	}
	cmd := exec.Command("go", "install", target)
	cmd.Env = append(GoProxyBypassEnv(nil, module), "GOBIN="+binDir)
	cmd.Stdin = nil
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("go install %s: %w (output: %s)", target, err, string(out))
	}
	return nil
}

func sameExecutable(a, b string) bool {
	aEval, aErr := filepath.EvalSymlinks(a)
	bEval, bErr := filepath.EvalSymlinks(b)
	if aErr == nil {
		a = aEval
	}
	if bErr == nil {
		b = bEval
	}
	return filepath.Clean(a) == filepath.Clean(b)
}
