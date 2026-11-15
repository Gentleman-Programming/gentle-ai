package opencode

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// RuntimeMajor is executable evidence, never inferred from user config keys.
type RuntimeMajor int

const (
	RuntimeUnknown RuntimeMajor = iota
	RuntimeV1
	RuntimeV2
)

// RuntimeVersion is the stable version reported by the executable, with any
// accepted prefix removed. An unknown runtime has the zero value.
type RuntimeVersion struct {
	Major   RuntimeMajor
	Version string
}

var stableRuntimeVersion = regexp.MustCompile(`^(?:opencode |OpenCode )?v?(([12])\.[0-9]+\.[0-9]+)$`)

func parseRuntimeVersion(version string) RuntimeVersion {
	match := stableRuntimeVersion.FindStringSubmatch(strings.TrimSpace(version))
	if match == nil {
		return RuntimeVersion{}
	}
	major := RuntimeV1
	if match[2] == "2" {
		major = RuntimeV2
	}
	return RuntimeVersion{Major: major, Version: match[1]}
}

func ParseRuntimeMajor(version string) RuntimeMajor {
	return parseRuntimeVersion(version).Major
}

// RuntimeProvidesPluginSDK identifies stable V2 hosts with built-in plugin modules.
// OpenCode 2.0.25 introduced host-provided @opencode/plugin; a local SDK is
// optional tooling on these hosts, not a prerequisite for loading plugins.
func RuntimeProvidesPluginSDK(version string) bool {
	if ParseRuntimeMajor(version) != RuntimeV2 {
		return false
	}
	fields := strings.Fields(version)
	parts := strings.Split(strings.TrimPrefix(fields[len(fields)-1], "v"), ".")
	minor, minorErr := strconv.ParseUint(parts[1], 10, 64)
	patch, patchErr := strconv.ParseUint(parts[2], 10, 64)
	return minorErr == nil && patchErr == nil && (minor > 0 || patch >= 25)
}

type VersionCommandRunner func(context.Context, Command) (CommandOutput, error)

// VersionRunnerOverride is a process seam for isolated tests, like adapter
// LookPathOverride. Production probes only --version with bounded output/time.
var VersionRunnerOverride VersionCommandRunner = runVersionCommand

func runVersionCommand(ctx context.Context, command Command) (CommandOutput, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, command.Path, command.Args...)
	cmd.Dir = command.Dir
	if command.Env != nil {
		cmd.Env = command.Env
	}
	stdout, stderr := &limitedBuffer{limit: command.OutputLimit, cancel: cancel}, &limitedBuffer{limit: command.OutputLimit, cancel: cancel}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err := cmd.Run()
	if stdout.overflow || stderr.overflow {
		return CommandOutput{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}, &CatalogError{Kind: CatalogErrorOutputTooLarge}
	}
	return CommandOutput{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}, err
}

type limitedBuffer struct {
	buffer   bytes.Buffer
	limit    int
	overflow bool
	cancel   context.CancelFunc
}

func (b *limitedBuffer) Write(data []byte) (int, error) {
	remaining := b.limit - b.buffer.Len()
	if len(data) > remaining {
		if remaining > 0 {
			_, _ = b.buffer.Write(data[:remaining])
		}
		b.overflow = true
		b.cancel()
		return len(data), nil
	}
	return b.buffer.Write(data)
}

func (b *limitedBuffer) Bytes() []byte { return b.buffer.Bytes() }

// ErrRuntimeVersionTimeout marks a probe that ran out of time: the runtime
// may be perfectly supported, so callers must not report it as unsupported.
var ErrRuntimeVersionTimeout = errors.New("`opencode --version` timed out")

// runtimeVersionTimeout allows for slow external CLI startup while keeping
// the probe bounded; a variable only so tests can exercise the real deadline
// path without waiting for it.
var runtimeVersionTimeout = 15 * time.Second

func DetectRuntimeMajor(ctx context.Context) (RuntimeMajor, error) {
	version, err := DetectRuntimeVersion(ctx)
	return version.Major, err
}

// DetectRuntimeVersion returns the major and exact stable version from the
// same bounded executable probe.
func DetectRuntimeVersion(ctx context.Context) (RuntimeVersion, error) {
	ctx, cancel := context.WithTimeout(ctx, runtimeVersionTimeout)
	defer cancel()
	probe := Command{Path: "opencode", Args: []string{"--version"}, OutputLimit: 4096}
	output, err := VersionRunnerOverride(ctx, probe)
	var exitError *exec.ExitError
	var pathError *os.PathError
	if (errors.As(err, &exitError) || errors.As(err, &pathError)) && ctx.Err() == nil {
		// Only bypass an exactly recognized Gentle-owned launcher. A failed
		// user wrapper or unsupported version must retain its original result.
		if target, ok := brokenManagedLauncherFallback(); ok {
			probe.Path = target
			output, err = VersionRunnerOverride(ctx, probe)
		}
	}
	if err == nil && len(output.Stdout) <= 4096 && len(output.Stderr) <= 4096 {
		if version := parseRuntimeVersion(string(output.Stdout)); version.Major != RuntimeUnknown {
			return version, nil
		}
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return RuntimeVersion{}, fmt.Errorf("%w after %s; managed runtime assets were not selected", ErrRuntimeVersionTimeout, runtimeVersionTimeout)
	}
	return RuntimeVersion{}, errors.New("OpenCode runtime version unavailable or unsupported; managed runtime assets were not selected")
}

// brokenManagedLauncherFallback resolves an alternate executable only when
// PATH selects one of our canonical launchers with exactly generated bytes.
// It does not mutate launcher files or probe an inferred runtime version.
func brokenManagedLauncherFallback() (string, bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	selected, err := exec.LookPath("opencode")
	if err != nil {
		return "", false
	}
	selected, err = filepath.Abs(selected)
	if err != nil {
		return "", false
	}
	owned := false
	for _, launcher := range ManagedLauncherPaths(home, runtime.GOOS) {
		// Match activation's ownership rule: a symlink is user-owned even
		// when its destination happens to contain exactly generated bytes.
		info, err := os.Lstat(launcher)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		physical, err := filepath.EvalSymlinks(launcher)
		if err != nil {
			continue
		}
		if samePath(selected, launcher, runtime.GOOS) || samePath(selected, physical, runtime.GOOS) {
			data, err := os.ReadFile(launcher)
			owned = err == nil && IsManagedLauncher(launcher, data)
			break
		}
	}
	if !owned {
		return "", false
	}
	target, err := ResolveTarget(home, runtime.GOOS, os.Getenv("PATH"))
	return target, err == nil
}

func (major RuntimeMajor) PluginAssetDirectory() (string, error) {
	switch major {
	case RuntimeV1:
		return "opencode/plugins/", nil
	case RuntimeV2:
		return "opencode/plugins-v2/", nil
	}
	return "", errors.New("OpenCode runtime major is unknown")
}
func (major RuntimeMajor) PluginDependency() (string, error) {
	switch major {
	case RuntimeV1:
		return "@opencode-ai/plugin@latest", nil
	case RuntimeV2:
		return "@opencode/plugin@2.0.4", nil
	}
	return "", errors.New("OpenCode runtime major is unknown")
}
