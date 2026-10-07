package opencode

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

func TestResolveCapabilityVersionTable(t *testing.T) {
	tests := []struct {
		name   string
		output string
		status CapabilityStatus
		ready  bool
	}{
		{name: "baseline", output: "1.15.11\n", status: CapabilityReady, ready: true},
		{name: "newer", output: "opencode 1.18.18", status: CapabilityReady, ready: true},
		{name: "older patch", output: "1.15.10", status: CapabilityUnsupported},
		{name: "older minor", output: "1.14.99", status: CapabilityUnsupported},
		{name: "pre release baseline", output: "1.15.11-beta.1", status: CapabilityUnsupported},
		{name: "build metadata baseline", output: "opencode v1.15.11+build.1\n", status: CapabilityReady, ready: true},
		{name: "valid adjacent punctuation", output: "opencode 1.15.11, installed", status: CapabilityReady, ready: true},
		{name: "malformed prerelease empty identifier", output: "1.15.11-alpha..1", status: CapabilityUnknown},
		{name: "malformed prerelease numeric leading zero", output: "1.15.11-01", status: CapabilityUnknown},
		{name: "malformed build metadata empty identifier", output: "1.15.11+build..1", status: CapabilityUnknown},
		{name: "malformed build metadata invalid identifier character", output: "1.15.11+build_1", status: CapabilityUnknown},
		{name: "malformed prerelease path continuation", output: "opencode 1.15.11-rc.1/evil", status: CapabilityUnknown},
		{name: "malformed build path continuation", output: "opencode 1.15.11+build.1/evil", status: CapabilityUnknown},
		{name: "malformed identifier continuation", output: "opencode 1.15.11-rc.1@bad", status: CapabilityUnknown},
		{name: "valid closing parenthesis", output: "opencode (1.15.11)", status: CapabilityReady, ready: true},
		{name: "unknown output", output: "development build", status: CapabilityUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveCapability("/real/opencode", func(string) (string, error) {
				return tt.output, nil
			})
			if got.Status != tt.status || got.Ready() != tt.ready {
				t.Fatalf("resolution = %#v, want status=%q ready=%t", got, tt.status, tt.ready)
			}
		})
	}

	unknown := ResolveCapability("/real/opencode", func(string) (string, error) {
		return "", errors.New("not runnable")
	})
	if unknown.Status != CapabilityUnknown || unknown.Ready() {
		t.Fatalf("command failure resolution = %#v, want unknown foreground", unknown)
	}
}

func TestVersionPreReleasePrecedenceIgnoresBuildMetadata(t *testing.T) {
	parse := func(t *testing.T, raw string) Version {
		t.Helper()
		version, err := ParseVersion(raw)
		if err != nil {
			t.Fatalf("ParseVersion(%q): %v", raw, err)
		}
		return version
	}

	if !parse(t, "1.15.11-rc.1").AtLeast(parse(t, "1.15.11-beta.2")) {
		t.Fatal("release candidate ranked below beta")
	}
	if parse(t, "1.15.11-1").AtLeast(parse(t, "1.15.11-alpha")) {
		t.Fatal("numeric prerelease ranked above non-numeric prerelease")
	}
	if !parse(t, "1.15.11+build.1").AtLeast(MinimumBackgroundVersion) || !MinimumBackgroundVersion.AtLeast(parse(t, "1.15.11+build.1")) {
		t.Fatal("build metadata changed stable version precedence")
	}
	if !parse(t, "1.15.11-rc.1+build.1").AtLeast(parse(t, "1.15.11-beta.2+build.2")) {
		t.Fatal("build metadata changed prerelease precedence")
	}
}

// resolveTargetCandidateName is the host-native candidate filename: Windows
// has no execute bit, so the fixture must be PATHEXT-shaped there (#3209).
func resolveTargetCandidateName() string {
	if runtime.GOOS == "windows" {
		return "opencode.cmd"
	}
	return "opencode"
}

func TestResolveTargetSkipsManagedBinAndPreventsRecursion(t *testing.T) {
	home := t.TempDir()
	managed := BinDir(home)
	real := filepath.Join(t.TempDir(), resolveTargetCandidateName())
	if err := os.MkdirAll(managed, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(managed, resolveTargetCandidateName()), []byte("managed"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(real, []byte("real"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveTarget(home, runtime.GOOS, managed+string(os.PathListSeparator)+filepath.Dir(real))
	if err != nil {
		t.Fatal(err)
	}
	if got != normalizedCandidatePath(t, real) {
		t.Fatalf("ResolveTarget() = %q, want %q", got, real)
	}
}

// normalizedCandidatePath resolves the fixture path exactly the way
// ResolveTarget resolves candidates (EvalSymlinks + Abs): on Windows
// t.TempDir() hands out 8.3 short names that production expands (#3209),
// and on macOS /tmp itself is a symlink.
func normalizedCandidatePath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	absolute, err := filepath.Abs(resolved)
	if err != nil {
		t.Fatal(err)
	}
	return absolute
}

func TestResolveTargetRejectsEmptyAndRelativeEntries(t *testing.T) {
	home := t.TempDir()
	if _, err := ResolveTarget(home, "linux", ""); err == nil {
		t.Fatal("ResolveTarget(empty PATH) error = nil, want unsafe PATH entry rejected")
	}
	workingDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workingDir, "opencode"), []byte("cwd"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(workingDir)
	if _, err := ResolveTarget(home, "linux", "."); err == nil {
		t.Fatal("ResolveTarget(relative PATH) error = nil, want cwd executable rejected")
	}
}

func TestResolveTargetContinuesAfterBrokenCandidate(t *testing.T) {
	home := t.TempDir()
	brokenDir := t.TempDir()
	validDir := t.TempDir()
	// A directory with the candidate name is a broken candidate on every OS:
	// stat succeeds and IsRegular fails, so resolution must continue. A broken
	// symlink would prove the same only where symlinks are creatable (#3209).
	if err := os.MkdirAll(filepath.Join(brokenDir, resolveTargetCandidateName()), 0o755); err != nil {
		t.Fatal(err)
	}
	valid := filepath.Join(validDir, resolveTargetCandidateName())
	if err := os.WriteFile(valid, []byte("valid"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveTarget(home, runtime.GOOS, strings.Join([]string{brokenDir, validDir}, string(os.PathListSeparator)))
	if err != nil {
		t.Fatal(err)
	}
	if got != normalizedCandidatePath(t, valid) {
		t.Fatalf("ResolveTarget() = %q, want %q", got, valid)
	}
}

func TestSplitPathUsesTargetOperatingSystem(t *testing.T) {
	if got := splitPath(`C:\\OpenCode;D:\\Tools`, "windows"); len(got) != 2 {
		t.Fatalf("Windows splitPath() = %#v, want two entries", got)
	}
	if got := splitPath(`/opt/opencode:/usr/local/bin`, "linux"); len(got) != 2 {
		t.Fatalf("POSIX splitPath() = %#v, want two entries", got)
	}
}

func TestLauncherContentsPreserveExplicitFalse(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX launcher execution is not supported on Windows")
	}
	home := t.TempDir()
	target := filepath.Join(t.TempDir(), "real-opencode")
	if err := os.WriteFile(target, []byte("#!/bin/sh\nprintf '%s|%s' \"${OPENCODE_EXPERIMENTAL_BACKGROUND_SUBAGENTS-unset}\" \"$1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	plan, err := PrepareActivation(home, ActivationOptions{
		OS:   "linux",
		Path: filepath.Dir(target),
		RunVersion: func(string) (string, error) {
			return "1.15.11", nil
		},
		AddToUserPath: func(string) error { return nil },
		ResolveTarget: func(string, string, string) (string, error) { return target, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(); err != nil {
		t.Fatal(err)
	}

	launcher := POSIXLauncherPath(home)
	run := func(value string) string {
		cmd := exec.Command(launcher, "arg")
		env := []string{}
		for _, entry := range os.Environ() {
			if !strings.HasPrefix(entry, BackgroundSubagentsEnv+"=") {
				env = append(env, entry)
			}
		}
		if value != "" {
			env = append(env, BackgroundSubagentsEnv+"="+value)
		}
		cmd.Env = env
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("launcher output: %v", err)
		}
		return string(output)
	}
	if got := run(""); got != "true|arg" {
		t.Fatalf("unset environment output = %q, want true|arg", got)
	}
	if got := run("false"); got != "false|arg" {
		t.Fatalf("explicit false output = %q, want false|arg", got)
	}
}

func TestWindowsLauncherContents(t *testing.T) {
	contents := launcherContent("windows", `C:\Program Files\OpenCode\opencode.exe`)
	for _, name := range []string{WindowsCMDPathPlaceholder, WindowsPS1PathPlaceholder} {
		content := contents[name]
		if !strings.Contains(content, OwnershipMarker) || !strings.Contains(content, BackgroundSubagentsEnv) {
			t.Fatalf("%s launcher = %q, missing ownership/env contract", name, content)
		}
		if !strings.Contains(content, "opencode.exe") {
			t.Fatalf("%s launcher = %q, missing real target", name, content)
		}
	}
	if strings.Contains(contents[WindowsCMDPathPlaceholder], "opencode.cmd") || strings.Contains(contents[WindowsPS1PathPlaceholder], "opencode.ps1") {
		t.Fatal("Windows launchers must execute the resolved real target, not themselves")
	}
}

func TestActivationIsIdempotentAndOffRemovesOnlyOwnedFiles(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(t.TempDir(), "opencode")
	if err := os.WriteFile(target, []byte("real"), 0o755); err != nil {
		t.Fatal(err)
	}
	options := ActivationOptions{
		OS:            runtime.GOOS,
		Path:          filepath.Dir(target),
		RunVersion:    func(string) (string, error) { return "1.18.18", nil },
		AddToUserPath: func(string) error { return nil },
		AddToUserPathWithResult: func(string) (system.UserPathAddition, error) {
			return system.UserPathAddition{}, nil
		},
		ResolveTarget: func(string, string, string) (string, error) { return target, nil },
	}
	paths := []string{POSIXLauncherPath(home)}
	if runtime.GOOS == "windows" {
		paths = []string{WindowsCMDPath(home), WindowsPS1Path(home)}
	}
	first, err := Activate(home, options)
	if err != nil {
		t.Fatal(err)
	}
	before := make(map[string]string, len(paths))
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		before[path] = string(body)
	}
	second, err := Activate(home, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.ChangedPaths()) != len(paths) || len(second.ChangedPaths()) != 0 {
		t.Fatalf("activation changed paths first=%v second=%v", first.ChangedPaths(), second.ChangedPaths())
	}
	for _, path := range paths {
		after, err := os.ReadFile(path)
		if err != nil || string(after) != before[path] {
			t.Fatalf("launcher %s changed on repeat: %v", path, err)
		}
	}
	if _, err := Deactivate(home, options); err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("owned launcher %s stat error = %v, want absent", path, err)
		}
	}
	if secondOff, err := Deactivate(home, options); err != nil || len(secondOff.ChangedPaths()) != 0 {
		t.Fatalf("repeated deactivation = %v, %v; want no changes", secondOff, err)
	}
}

func TestActivationRefusesUserOwnedCollision(t *testing.T) {
	home := t.TempDir()
	path := POSIXLauncherPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("user launcher"), 0o755); err != nil {
		t.Fatal(err)
	}
	plan, err := PrepareActivation(home, ActivationOptions{
		OS:            "linux",
		RunVersion:    func(string) (string, error) { return "1.18.18", nil },
		AddToUserPath: func(string) error { return nil },
		ResolveTarget: func(string, string, string) (string, error) { return "/real/opencode", nil },
	})
	if err == nil || plan != nil || !strings.Contains(err.Error(), "user-owned") {
		t.Fatalf("PrepareActivation() plan=%v error=%v, want collision refusal", plan, err)
	}
}

func TestActivationRollsBackLauncherWritesWhenPathUpdateFails(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(t.TempDir(), "opencode")
	if err := os.WriteFile(target, []byte("real"), 0o755); err != nil {
		t.Fatal(err)
	}
	pathErr := errors.New("path update failed")
	plan, err := PrepareActivation(home, ActivationOptions{
		OS:            "linux",
		RunVersion:    func(string) (string, error) { return "1.15.11", nil },
		AddToUserPath: func(string) error { return pathErr },
		ResolveTarget: func(string, string, string) (string, error) { return target, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(); err == nil || !strings.Contains(err.Error(), pathErr.Error()) {
		t.Fatalf("Apply() error = %v, want path update failure", err)
	}
	if _, err := os.Stat(POSIXLauncherPath(home)); !os.IsNotExist(err) {
		t.Fatalf("launcher after failed activation = %v, want absent", err)
	}
}

func TestWindowsActivationRollbackRemovesOnlyPlanOwnedPathEntry(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(t.TempDir(), "opencode.exe")
	if err := os.WriteFile(target, []byte("real"), 0o755); err != nil {
		t.Fatal(err)
	}
	managed := BinDir(home)
	entries := []string{`C:\Tools`, `"C:\Other"`}
	remove := func(dir string) error {
		for i, entry := range entries {
			if strings.EqualFold(strings.Trim(strings.TrimSpace(entry), `"`), dir) {
				entries = append(entries[:i], entries[i+1:]...)
				return nil
			}
		}
		return nil
	}
	plan, err := PrepareActivation(home, ActivationOptions{
		OS: "windows", RunVersion: func(string) (string, error) { return "1.15.11", nil },
		ResolveTarget: func(string, string, string) (string, error) { return target, nil },
		AddToUserPathWithResult: func(string) (system.UserPathAddition, error) {
			entries = append([]string{managed}, entries...)
			return system.UserPathAddition{ProcessAdded: true, PersistentAdded: true}, nil
		},
		RollbackUserPathAddition: func(dir string, addition system.UserPathAddition) error {
			if !addition.PersistentAdded {
				return nil
			}
			return remove(dir)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(); err != nil {
		t.Fatal(err)
	}
	if err := plan.Rollback(); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(entries, ";"), `C:\Tools;"C:\Other"`; got != want {
		t.Fatalf("persistent PATH = %q, want unrelated entries preserved as %q", got, want)
	}
}

func TestWindowsActivationRollbackRestoresProcessPathWhenPersistentEntryPreexists(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(t.TempDir(), "opencode.exe")
	if err := os.WriteFile(target, []byte("real"), 0o755); err != nil {
		t.Fatal(err)
	}
	persistentEntries := []string{`"` + BinDir(home) + `"`, `C:\Tools`}
	processEntries := []string{`C:\Tools`}
	rollbackCalled := false
	plan, err := PrepareActivation(home, ActivationOptions{
		OS: "windows", RunVersion: func(string) (string, error) { return "1.15.11", nil },
		ResolveTarget: func(string, string, string) (string, error) { return target, nil },
		AddToUserPathWithResult: func(dir string) (system.UserPathAddition, error) {
			processEntries = append([]string{dir}, processEntries...)
			return system.UserPathAddition{ProcessAdded: true}, nil
		},
		RollbackUserPathAddition: func(dir string, addition system.UserPathAddition) error {
			rollbackCalled = true
			if !addition.ProcessAdded || addition.PersistentAdded {
				t.Fatalf("rollback addition = %+v, want process-only ownership", addition)
			}
			if processEntries[0] != dir {
				t.Fatalf("process PATH first entry = %q, want %q", processEntries[0], dir)
			}
			processEntries = processEntries[1:]
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(); err != nil {
		t.Fatal(err)
	}
	if err := plan.Rollback(); err != nil {
		t.Fatal(err)
	}
	if !rollbackCalled || strings.Join(processEntries, ";") != `C:\Tools` {
		t.Fatalf("process PATH rollback called=%t entries=%q, want original process PATH", rollbackCalled, processEntries)
	}
	if got, want := strings.Join(persistentEntries, ";"), `"`+BinDir(home)+`";C:\Tools`; got != want {
		t.Fatalf("persistent PATH = %q, want unchanged %q", got, want)
	}
}

func TestWindowsActivationRollbackJoinsPathRemovalFailureAndRestoresLaunchers(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(t.TempDir(), "opencode.exe")
	if err := os.WriteFile(target, []byte("real"), 0o755); err != nil {
		t.Fatal(err)
	}
	pathErr := errors.New("path removal failed")
	plan, err := PrepareActivation(home, ActivationOptions{
		OS: "windows", RunVersion: func(string) (string, error) { return "1.15.11", nil },
		ResolveTarget: func(string, string, string) (string, error) { return target, nil },
		AddToUserPathWithResult: func(string) (system.UserPathAddition, error) {
			return system.UserPathAddition{ProcessAdded: true, PersistentAdded: true}, nil
		},
		RollbackUserPathAddition: func(string, system.UserPathAddition) error { return pathErr },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(); err != nil {
		t.Fatal(err)
	}
	if err := plan.Rollback(); !errors.Is(err, pathErr) {
		t.Fatalf("Rollback() error = %v, want joined %v", err, pathErr)
	}
	for _, path := range plan.Paths() {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("launcher %q after rollback = %v, want absent", path, err)
		}
	}
}

func TestWindowsActivationFailureAfterPathAdditionRollsBackPath(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(t.TempDir(), "opencode.exe")
	if err := os.WriteFile(target, []byte("real"), 0o755); err != nil {
		t.Fatal(err)
	}
	pathErr := errors.New("persistent PATH failed after process update")
	entries := []string{`C:\Tools`}
	removed := false
	plan, err := PrepareActivation(home, ActivationOptions{
		OS: "windows", RunVersion: func(string) (string, error) { return "1.15.11", nil },
		ResolveTarget: func(string, string, string) (string, error) { return target, nil },
		AddToUserPathWithResult: func(dir string) (system.UserPathAddition, error) {
			entries = append([]string{dir}, entries...)
			return system.UserPathAddition{ProcessAdded: true, PersistentAdded: true}, pathErr
		},
		RollbackUserPathAddition: func(dir string, addition system.UserPathAddition) error {
			if !addition.PersistentAdded {
				return nil
			}
			removed = true
			entries = entries[1:]
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(); !errors.Is(err, pathErr) {
		t.Fatalf("Apply() error = %v, want %v", err, pathErr)
	}
	if !removed || strings.Join(entries, ";") != `C:\Tools` {
		t.Fatalf("PATH failure compensation = removed:%t entries:%q, want original entries", removed, entries)
	}
}

func TestWindowsActivationPersistentPathFailurePreservesPreexistingProcessEntry(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(t.TempDir(), "opencode.exe")
	if err := os.WriteFile(target, []byte("real"), 0o755); err != nil {
		t.Fatal(err)
	}
	preexistingProcessPath := true
	persistentErr := errors.New("persistent PATH failed")
	rollbackCalled := false
	plan, err := PrepareActivation(home, ActivationOptions{
		OS:            "windows",
		RunVersion:    func(string) (string, error) { return "1.15.11", nil },
		ResolveTarget: func(string, string, string) (string, error) { return target, nil },
		AddToUserPathWithResult: func(string) (system.UserPathAddition, error) {
			return system.UserPathAddition{}, persistentErr
		},
		RollbackUserPathAddition: func(string, system.UserPathAddition) error {
			rollbackCalled = true
			preexistingProcessPath = false
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(); !errors.Is(err, persistentErr) {
		t.Fatalf("Apply() error = %v, want %v", err, persistentErr)
	}
	if rollbackCalled || !preexistingProcessPath {
		t.Fatalf("process PATH compensation called=%t preexisting=%t, want pre-existing entry preserved", rollbackCalled, preexistingProcessPath)
	}
}

// The transaction and parsing conveniences below are test-scoped: production
// callers drive Prepare*/Apply through the pipeline step, so keeping these in
// the shipped package would be dead code under the ratchet.
// Activate prepares and applies a managed activation transaction.
func Activate(homeDir string, options ActivationOptions) (*ActivationPlan, error) {
	plan, err := PrepareActivation(homeDir, options)
	if err != nil {
		return nil, err
	}
	if err := plan.Apply(); err != nil {
		return plan, err
	}
	return plan, nil
}

// Deactivate prepares and applies a managed deactivation transaction.
func Deactivate(homeDir string, options ActivationOptions) (*ActivationPlan, error) {
	plan, err := PrepareDeactivation(homeDir, options)
	if err != nil {
		return nil, err
	}
	if err := plan.Apply(); err != nil {
		return plan, err
	}
	return plan, nil
}

// ParseVersion parses a semantic version such as "v1.15.11".
func ParseVersion(raw string) (Version, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return Version{}, errors.New("OpenCode version is empty")
	}
	if strings.HasPrefix(value, "v") || strings.HasPrefix(value, "V") {
		value = value[1:]
	}
	match := versionPattern.FindStringSubmatch(" " + value + " ")
	if match == nil || strings.TrimSpace(match[0]) != value {
		return Version{}, fmt.Errorf("invalid OpenCode version %q", raw)
	}
	return versionFromMatch(match)
}

// TestDefaultActivationWriteFileForcesLauncherMode pins that the default
// launcher writer applies the requested mode to an existing file: a launcher
// that lost its executable bit must become executable again on rewrite, and a
// rollback must reinstate the recorded mode.
func TestDefaultActivationWriteFileForcesLauncherMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits")
	}
	path := filepath.Join(t.TempDir(), "opencode")
	if err := os.WriteFile(path, []byte("old launcher\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	options := ActivationOptions{}.normalized()
	if err := options.WriteFile(path, []byte("new launcher\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o755 {
		t.Fatalf("launcher mode = %v, want 0755", got)
	}
}

// TestActivationRestoresLauncherExecutableModeWhenContentMatches pins that
// re-activation repairs a managed launcher whose bytes are current but whose
// executable bit was lost; the content-equality skip must not hide it.
func TestActivationRestoresLauncherExecutableModeWhenContentMatches(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits")
	}
	home := t.TempDir()
	target := filepath.Join(t.TempDir(), "opencode")
	if err := os.WriteFile(target, []byte("real"), 0o755); err != nil {
		t.Fatal(err)
	}
	options := ActivationOptions{
		OS:            "linux",
		Path:          filepath.Dir(target),
		RunVersion:    func(string) (string, error) { return "1.18.18", nil },
		AddToUserPath: func(string) error { return nil },
		ResolveTarget: func(string, string, string) (string, error) { return target, nil },
	}
	if _, err := Activate(home, options); err != nil {
		t.Fatal(err)
	}
	path := POSIXLauncherPath(home)
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Activate(home, options); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o755 {
		t.Fatalf("launcher mode after re-activation = %v, want 0755", got)
	}
}

// Issue #3451: ownership is the exact generated launcher, never a file that
// merely mentions the marker.
func TestActivationRefusesIncidentalLauncherMarker(t *testing.T) {
	home := t.TempDir()
	launcher := POSIXLauncherPath(home)
	if err := os.MkdirAll(filepath.Dir(launcher), 0o755); err != nil {
		t.Fatal(err)
	}
	userBytes := "#!/bin/sh\n# " + OwnershipMarker + " belongs to user\nexec /usr/local/bin/opencode \"$@\"\n"
	if err := os.WriteFile(launcher, []byte(userBytes), 0o755); err != nil {
		t.Fatal(err)
	}
	plan, err := PrepareActivation(home, ActivationOptions{
		OS:            "linux",
		RunVersion:    func(string) (string, error) { return "1.15.11", nil },
		AddToUserPath: func(string) error { return nil },
		ResolveTarget: func(string, string, string) (string, error) { return "/real/opencode", nil },
	})
	if err == nil || plan != nil || !strings.Contains(err.Error(), "user-owned") || !strings.Contains(err.Error(), "move or delete it") {
		t.Fatalf("PrepareActivation() plan!=nil=%t error=%v, want user-owned collision refusal", plan != nil, err)
	}
	if got, err := os.ReadFile(launcher); err != nil || string(got) != userBytes {
		t.Fatalf("user launcher = %q, %v; want preserved", got, err)
	}
}

// Issue #3451: a launcher replaced between preparation and Apply is not the
// state the plan was approved for, so activation must not overwrite it.
func TestActivationApplyRevalidatesLauncherBeforeWrite(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(t.TempDir(), "opencode")
	if err := os.WriteFile(target, []byte("real"), 0o755); err != nil {
		t.Fatal(err)
	}
	plan, err := PrepareActivation(home, ActivationOptions{
		OS:            "linux",
		RunVersion:    func(string) (string, error) { return "1.15.11", nil },
		AddToUserPath: func(string) error { return nil },
		ResolveTarget: func(string, string, string) (string, error) { return target, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	launcher := POSIXLauncherPath(home)
	if err := os.MkdirAll(filepath.Dir(launcher), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(launcher, []byte("user replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(); err == nil || !strings.Contains(err.Error(), "revalidate") {
		t.Fatalf("Apply() error = %v, want revalidation failure", err)
	}
	if got, err := os.ReadFile(launcher); err != nil || string(got) != "user replacement" {
		t.Fatalf("replacement after stale activation = %q, %v; want preserved", got, err)
	}
	if got := plan.ChangedPaths(); len(got) != 0 {
		t.Fatalf("ChangedPaths() = %v, want none", got)
	}
}

// Issue #3451: deactivation removes only the exact launcher it inspected.
func TestDeactivationApplyRevalidatesLauncherBeforeRemoval(t *testing.T) {
	home := t.TempDir()
	launcher := POSIXLauncherPath(home)
	if err := os.MkdirAll(filepath.Dir(launcher), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(launcher, []byte(posixLauncher("/old/opencode")), 0o755); err != nil {
		t.Fatal(err)
	}
	plan, err := PrepareDeactivation(home, ActivationOptions{OS: "linux"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(launcher, []byte("user replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(); err == nil || !strings.Contains(err.Error(), "revalidate") {
		t.Fatalf("Apply() error = %v, want revalidation failure", err)
	}
	if got, err := os.ReadFile(launcher); err != nil || string(got) != "user replacement" {
		t.Fatalf("replacement after stale deactivation = %q, %v; want preserved", got, err)
	}
}

// Issue #3451: a later launcher replaced while an earlier one is written is
// preserved, and the earlier plan-owned write is rolled back.
func TestActivationRevalidationFailureRollsBackEarlierLauncher(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(t.TempDir(), "opencode.exe")
	if err := os.WriteFile(target, []byte("real"), 0o755); err != nil {
		t.Fatal(err)
	}
	first := WindowsCMDPath(home)
	second := WindowsPS1Path(home)
	writeCalls := 0
	plan, err := PrepareActivation(home, ActivationOptions{
		OS:            "windows",
		RunVersion:    func(string) (string, error) { return "1.15.11", nil },
		ResolveTarget: func(string, string, string) (string, error) { return target, nil },
		WriteFile: func(path string, content []byte, mode os.FileMode) error {
			writeCalls++
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(path, content, mode); err != nil {
				return err
			}
			if writeCalls == 1 {
				return os.WriteFile(second, []byte("user replacement"), 0o600)
			}
			return nil
		},
		AddToUserPathWithResult: func(string) (system.UserPathAddition, error) {
			return system.UserPathAddition{}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(); err == nil || !strings.Contains(err.Error(), "revalidate") {
		t.Fatalf("Apply() error = %v, want revalidation failure", err)
	}
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Fatalf("earlier launcher after failed activation = %v, want absent", err)
	}
	if got, err := os.ReadFile(second); err != nil || string(got) != "user replacement" {
		t.Fatalf("replaced launcher after failed activation = %q, %v; want preserved", got, err)
	}
}

// Issue #3451: rollback restores only bytes this plan still owns; a launcher
// replaced after the write is left for its new owner.
func TestActivationRollbackPreservesLauncherReplacedAfterWrite(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(t.TempDir(), "opencode")
	if err := os.WriteFile(target, []byte("real"), 0o755); err != nil {
		t.Fatal(err)
	}
	launcher := POSIXLauncherPath(home)
	pathErr := errors.New("path update failed")
	plan, err := PrepareActivation(home, ActivationOptions{
		OS:         "linux",
		RunVersion: func(string) (string, error) { return "1.15.11", nil },
		AddToUserPath: func(string) error {
			if err := os.WriteFile(launcher, []byte("user replacement"), 0o600); err != nil {
				return err
			}
			return pathErr
		},
		ResolveTarget: func(string, string, string) (string, error) { return target, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(); !errors.Is(err, pathErr) {
		t.Fatalf("Apply() error = %v, want %v", err, pathErr)
	}
	if got, err := os.ReadFile(launcher); err != nil || string(got) != "user replacement" {
		t.Fatalf("replacement after rollback = %q, %v; want preserved", got, err)
	}
}

func TestIsManagedLauncherRejectsIncidentalAndMalformedMarkers(t *testing.T) {
	for _, tt := range []struct {
		name, path, content string
		want                bool
	}{
		{"posix old target", "opencode", posixLauncher("/old/opencode"), true},
		{"cmd old target", "opencode.cmd", windowsCMDLauncher(`C:\old\opencode.exe`), true},
		{"cmd powershell target", "opencode.cmd", windowsCMDLauncher(`C:\old\opencode.ps1`), true},
		{"powershell old target", "opencode.ps1", windowsPS1Launcher(`C:\old\opencode.exe`), true},
		{"incidental marker", "opencode", "#!/bin/sh\n# user mentions " + OwnershipMarker + "\necho user\n", false},
		{"truncated generated header", "opencode", "#!/bin/sh\n# " + OwnershipMarker + "\nset -eu\n", false},
		{"cmd direct form for powershell target", "opencode.cmd", strings.Replace(windowsCMDLauncher(`C:\old\opencode.ps1`), `powershell -NoProfile -ExecutionPolicy Bypass -File `, "", 1), false},
		{"launcher for another name", "opencode.cmd", posixLauncher("/old/opencode"), false},
		{"wrong launcher path", "custom-opencode", posixLauncher("/old/opencode"), false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsManagedLauncher(tt.path, []byte(tt.content)); got != tt.want {
				t.Fatalf("IsManagedLauncher() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestIsManagedLauncherRequiresCanonicalGeneratedBytes(t *testing.T) {
	for _, tt := range []struct {
		name, path, content string
	}{
		{"posix shell characters", "opencode", posixLauncher("/old path/a'b;$HOME")},
		{"cmd spaces and quotes", "opencode.cmd", windowsCMDLauncher(`C:\old path\a"b.exe`)},
		{"powershell apostrophe", "opencode.ps1", windowsPS1Launcher(`C:\old path\a'b.exe`)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if !IsManagedLauncher(tt.path, []byte(tt.content)) {
				t.Fatal("generated launcher was rejected")
			}
			for _, forged := range []string{
				"echo injected\n" + tt.content,
				tt.content + "echo injected\n",
				strings.Replace(tt.content, "\n", "\necho injected\n", 1),
				tt.content[:len(tt.content)-1],
			} {
				if IsManagedLauncher(tt.path, []byte(forged)) {
					t.Fatalf("forged launcher accepted: %q", forged)
				}
			}
		})
	}
}

func TestActivationRefusesSymlinkedLauncher(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges not guaranteed on Windows")
	}
	home := t.TempDir()
	launcher := POSIXLauncherPath(home)
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte(posixLauncher("/old/opencode")), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(launcher), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, launcher); err != nil {
		t.Fatal(err)
	}
	options := ActivationOptions{OS: "linux"}
	if plan, err := PrepareDeactivation(home, options); err == nil || plan != nil {
		t.Fatalf("PrepareDeactivation() plan!=nil=%t error=%v, want non-regular refusal", plan != nil, err)
	}
	if result, err := RemoveManagedLauncher(launcher); err != nil || result.Status != ManagedLauncherRemovalRefused {
		t.Fatalf("RemoveManagedLauncher() = %q, %v; want refused", result.Status, err)
	}
	if got, err := os.Readlink(launcher); err != nil || got != target {
		t.Fatalf("launcher symlink = %q, %v; want preserved", got, err)
	}
}

// A launcher deleted by someone else between revalidation and removal is not
// this plan's change, so rollback of a later failure must not recreate it.
func TestDeactivationRollbackDoesNotRecreateConcurrentlyDeletedLauncher(t *testing.T) {
	home := t.TempDir()
	first := WindowsCMDPath(home)
	second := WindowsPS1Path(home)
	if err := os.MkdirAll(filepath.Dir(first), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(first, []byte(windowsCMDLauncher(`C:\old\opencode.exe`)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte(windowsPS1Launcher(`C:\old\opencode.exe`)), 0o644); err != nil {
		t.Fatal(err)
	}
	removeErr := errors.New("injected removal failure")
	plan, err := PrepareDeactivation(home, ActivationOptions{
		OS: "windows",
		RemoveFile: func(path string) error {
			if path == second {
				return removeErr
			}
			if err := os.Remove(path); err != nil {
				return err
			}
			return os.ErrNotExist
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(); !errors.Is(err, removeErr) {
		t.Fatalf("Apply() error = %v, want %v", err, removeErr)
	}
	if _, err := os.Lstat(first); !os.IsNotExist(err) {
		t.Fatalf("concurrently deleted launcher = %v, want still absent", err)
	}
	if got, err := os.ReadFile(second); err != nil || string(got) != windowsPS1Launcher(`C:\old\opencode.exe`) {
		t.Fatalf("launcher whose removal failed = %q, %v; want unchanged", got, err)
	}
}

func TestRemoveManagedLauncherRemovesOwnedLauncher(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX launcher removal is covered by the Windows handle test on Windows")
	}
	home := t.TempDir()
	path := POSIXLauncherPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(posixLauncher("/old/opencode")), 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := RemoveManagedLauncher(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := result.Status, ManagedLauncherRemovalRemoved; got != want {
		t.Fatalf("removal status = %q, want %q", got, want)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("owned launcher after removal = %v, want absent", err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("launcher directory after removal = %v, want no quarantine residue", entries)
	}
}

func TestRemoveManagedLauncherReportsAbsentAndNotOwned(t *testing.T) {
	home := t.TempDir()
	path := POSIXLauncherPath(home)
	if result, err := RemoveManagedLauncher(path); err != nil || result.Status != ManagedLauncherRemovalAbsent {
		t.Fatalf("absent removal = %q, %v; want absent", result.Status, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	userBytes := "#!/bin/sh\n# " + OwnershipMarker + "\n"
	if err := os.WriteFile(path, []byte(userBytes), 0o755); err != nil {
		t.Fatal(err)
	}
	if result, err := RemoveManagedLauncher(path); err != nil || result.Status != ManagedLauncherRemovalNotOwned {
		t.Fatalf("user launcher removal = %q, %v; want not-owned", result.Status, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != userBytes {
		t.Fatalf("user launcher = %q, %v; want preserved", got, err)
	}
}

func TestRemoveManagedLauncherRefusesReplacementBeforeCapture(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("replacement fixture relies on POSIX rename semantics")
	}
	for _, tt := range []struct {
		name    string
		symlink bool
	}{
		{name: "user file"},
		{name: "user symlink", symlink: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			path := POSIXLauncherPath(home)
			target := filepath.Join(t.TempDir(), "target")
			if err := os.WriteFile(target, []byte("target"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(posixLauncher("/old/opencode")), 0o755); err != nil {
				t.Fatal(err)
			}
			managedPath := path + ".managed"
			original := managedLauncherRemovalBeforeDelete
			t.Cleanup(func() { managedLauncherRemovalBeforeDelete = original })
			managedLauncherRemovalBeforeDelete = func(candidate string) {
				if candidate != path {
					return
				}
				if err := os.Rename(path, managedPath); err != nil {
					t.Errorf("move validated launcher aside: %v", err)
					return
				}
				if tt.symlink {
					if err := os.Symlink(target, path); err != nil {
						t.Errorf("install replacement symlink: %v", err)
					}
				} else if err := os.WriteFile(path, []byte("user replacement"), 0o600); err != nil {
					t.Errorf("install replacement file: %v", err)
				}
			}

			result, err := RemoveManagedLauncher(path)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := result.Status, ManagedLauncherRemovalRefused; got != want {
				t.Fatalf("removal status = %q, want %q", got, want)
			}
			if tt.symlink {
				if got, err := os.Readlink(path); err != nil || got != target {
					t.Fatalf("replacement symlink = %q, %v; want %q", got, err, target)
				}
			} else if got, err := os.ReadFile(path); err != nil || string(got) != "user replacement" {
				t.Fatalf("replacement after refused removal = %q, %v", got, err)
			}
			if got, err := os.ReadFile(managedPath); err != nil || !IsManagedLauncher(path, got) {
				t.Fatalf("original managed launcher = %q, %v; want preserved", got, err)
			}
		})
	}
}

func TestRemoveManagedLauncherPreservesReplacementAfterFinalValidation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("replacement fixture relies on POSIX rename semantics")
	}
	for _, tt := range []struct {
		name    string
		symlink bool
	}{
		{name: "user file"},
		{name: "user symlink", symlink: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			path := POSIXLauncherPath(home)
			target := filepath.Join(t.TempDir(), "target")
			replacementBytes := []byte("user replacement")
			if err := os.WriteFile(target, []byte("target"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(posixLauncher("/old/opencode")), 0o755); err != nil {
				t.Fatal(err)
			}
			original := managedLauncherRemovalBeforeUnlink
			t.Cleanup(func() { managedLauncherRemovalBeforeUnlink = original })
			managedLauncherRemovalBeforeUnlink = func(candidate string) {
				if candidate != path {
					return
				}
				// The validated launcher is already captured, so the public
				// name is free for a concurrent writer.
				if tt.symlink {
					if err := os.Symlink(target, path); err != nil {
						t.Errorf("create replacement symlink: %v", err)
					}
				} else if err := os.WriteFile(path, replacementBytes, 0o600); err != nil {
					t.Errorf("create replacement file: %v", err)
				}
			}

			result, err := RemoveManagedLauncher(path)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := result.Status, ManagedLauncherRemovalRemoved; got != want {
				t.Fatalf("removal status = %q, want %q", got, want)
			}
			if tt.symlink {
				if got, err := os.Readlink(path); err != nil || got != target {
					t.Fatalf("replacement symlink = %q, %v; want %q", got, err, target)
				}
				return
			}
			if got, err := os.ReadFile(path); err != nil || string(got) != string(replacementBytes) {
				t.Fatalf("replacement bytes = %q, %v; want %q", got, err, replacementBytes)
			}
		})
	}
}
