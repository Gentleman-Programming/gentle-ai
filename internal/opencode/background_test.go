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

	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
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
		OS:            "linux",
		Path:          filepath.Dir(target),
		RunVersion:    func(string) (string, error) { return "1.18.18", nil },
		AddToUserPath: func(string) error { return nil },
		ResolveTarget: func(string, string, string) (string, error) { return target, nil },
	}
	first, err := Activate(home, options)
	if err != nil {
		t.Fatal(err)
	}
	path := POSIXLauncherPath(home)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Activate(home, options)
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) || len(first.ChangedPaths()) == 0 || len(second.ChangedPaths()) != 0 {
		t.Fatalf("activation changed paths first=%v second=%v", first.ChangedPaths(), second.ChangedPaths())
	}
	if _, err := Deactivate(home, options); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("owned launcher stat error = %v, want absent", err)
	}
	if _, err := Deactivate(home, options); err != nil {
		t.Fatal(err)
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
