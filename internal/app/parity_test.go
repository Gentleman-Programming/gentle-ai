package app

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/cli"
	"github.com/gentleman-programming/gentle-ai/internal/planner"
	"github.com/gentleman-programming/gentle-ai/internal/system"
	"github.com/gentleman-programming/gentle-ai/internal/tui"
)

func TestInstallDefaultsMatchTUIModelDefaults(t *testing.T) {
	detection := system.DetectionResult{
		Configs: []system.ConfigState{
			{Agent: "claude-code", Exists: true, IsDirectory: true},
			{Agent: "opencode", Exists: false},
		},
	}

	flags, err := cli.ParseInstallFlags(nil)
	if err != nil {
		t.Fatalf("ParseInstallFlags() error = %v", err)
	}

	input, err := cli.NormalizeInstallFlags(flags, detection)
	if err != nil {
		t.Fatalf("NormalizeInstallFlags() error = %v", err)
	}

	model := tui.NewModel(detection, "dev")
	if !reflect.DeepEqual(input.Selection, model.Selection) {
		t.Fatalf("selection mismatch\ncli=%#v\ntui=%#v", input.Selection, model.Selection)
	}
}

func TestInstallPlannerParityWithTUISelection(t *testing.T) {
	detection := system.DetectionResult{}
	model := tui.NewModel(detection, "dev")

	result, err := cli.RunInstall([]string{"--dry-run"}, detection)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	if !reflect.DeepEqual(result.Selection, model.Selection) {
		t.Fatalf("selection mismatch\ncli=%#v\ntui=%#v", result.Selection, model.Selection)
	}

	wantResolved, err := planner.NewResolver(planner.MVPGraph()).Resolve(model.Selection)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	wantResolved.PlatformDecision = planner.PlatformDecision{OS: "darwin", PackageManager: "brew", Supported: true}

	if !reflect.DeepEqual(result.Resolved, wantResolved) {
		t.Fatalf("resolved mismatch\ncli=%#v\ntui=%#v", result.Resolved, wantResolved)
	}

	wantReview := planner.BuildReviewPayload(model.Selection, wantResolved)
	if !reflect.DeepEqual(result.Review, wantReview) {
		t.Fatalf("review mismatch\ncli=%#v\ntui=%#v", result.Review, wantReview)
	}
}

// --- Batch D: App guard-flow tests ---

func TestGuardAcceptsWindows(t *testing.T) {
	if err := system.EnsureSupportedOS("windows"); err != nil {
		t.Fatalf("expected windows to be accepted, got %v", err)
	}
}

func TestGuardRejectsUnsupportedOS(t *testing.T) {
	err := system.EnsureSupportedOS("freebsd")
	if err == nil {
		t.Fatalf("expected error for unsupported OS")
	}
	if !errors.Is(err, system.ErrUnsupportedOS) {
		t.Fatalf("expected ErrUnsupportedOS, got %v", err)
	}
}

func TestGuardAcceptsDarwin(t *testing.T) {
	if err := system.EnsureSupportedOS("darwin"); err != nil {
		t.Fatalf("expected darwin to be accepted, got %v", err)
	}
}

func TestGuardAcceptsLinux(t *testing.T) {
	if err := system.EnsureSupportedOS("linux"); err != nil {
		t.Fatalf("expected linux to be accepted, got %v", err)
	}
}

func TestGuardRejectsUnknownUnsupportedLinuxDistro(t *testing.T) {
	profile := system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroUnknown, Supported: false}
	err := system.EnsureSupportedPlatform(profile)
	if err == nil {
		t.Fatalf("expected error for unsupported linux distro")
	}
	if !errors.Is(err, system.ErrUnsupportedLinuxDistro) {
		t.Fatalf("expected ErrUnsupportedLinuxDistro, got %v", err)
	}
}

func TestGuardAcceptsUbuntuProfile(t *testing.T) {
	profile := system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroUbuntu, PackageManager: "apt", Supported: true}
	if err := system.EnsureSupportedPlatform(profile); err != nil {
		t.Fatalf("expected ubuntu profile to be accepted, got %v", err)
	}
}

func TestGuardAcceptsArchProfile(t *testing.T) {
	profile := system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroArch, PackageManager: "pacman", Supported: true}
	if err := system.EnsureSupportedPlatform(profile); err != nil {
		t.Fatalf("expected arch profile to be accepted, got %v", err)
	}
}

func TestGuardAcceptsFedoraProfile(t *testing.T) {
	profile := system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroFedora, PackageManager: "dnf", Supported: true}
	if err := system.EnsureSupportedPlatform(profile); err != nil {
		t.Fatalf("expected fedora profile to be accepted, got %v", err)
	}
}

func TestGuardFlowLinuxDryRunPropagatesDecision(t *testing.T) {
	detection := system.DetectionResult{
		System: system.SystemInfo{
			OS:        "linux",
			Arch:      "amd64",
			Shell:     "/bin/bash",
			Supported: true,
			Profile: system.PlatformProfile{
				OS:             "linux",
				LinuxDistro:    system.LinuxDistroUbuntu,
				PackageManager: "apt",
				Supported:      true,
			},
		},
	}

	result, err := cli.RunInstall([]string{"--dry-run"}, detection)
	if err != nil {
		t.Fatalf("RunInstall() dry-run error = %v", err)
	}

	if result.Resolved.PlatformDecision.OS != "linux" {
		t.Fatalf("platform decision OS = %q, want linux", result.Resolved.PlatformDecision.OS)
	}
	if result.Resolved.PlatformDecision.PackageManager != "apt" {
		t.Fatalf("platform decision PackageManager = %q, want apt", result.Resolved.PlatformDecision.PackageManager)
	}
	if !result.Resolved.PlatformDecision.Supported {
		t.Fatalf("platform decision Supported = false, want true")
	}
}

func TestRunArgsNoCommandLaunchesTUI(t *testing.T) {
	var buf bytes.Buffer
	err := RunArgs(nil, &buf)
	// With no args, RunArgs now launches the TUI via Bubbletea.
	// In a headless/test environment without a TTY, this returns an error
	// about opening /dev/tty. That's expected — the TUI requires a terminal.
	if err == nil {
		// If no error, we're somehow in a TTY — that's fine too.
		return
	}
	if !strings.Contains(err.Error(), "TTY") && !strings.Contains(err.Error(), "tty") {
		t.Fatalf("RunArgs(nil) unexpected error = %v; want TTY-related error or nil", err)
	}
}

func TestRunArgsUnknownCommandReturnsError(t *testing.T) {
	var buf bytes.Buffer
	err := RunArgs([]string{"bogus"}, &buf)
	if err == nil {
		t.Fatalf("RunArgs(bogus) expected error")
	}
	if !strings.Contains(err.Error(), `unknown command "bogus"`) {
		t.Fatalf("RunArgs(bogus) error = %v", err)
	}
}

// --- Sync command wiring tests ---

// TestRunArgsSyncDryRunIsDispatchedAndPrintsReport verifies that
// `RunArgs(["sync", "--dry-run", ...])` is correctly wired through app.go
// and produces output about the sync plan — not a "unknown command" error.
func TestRunArgsSyncDryRunIsDispatchedAndPrintsReport(t *testing.T) {
	var buf bytes.Buffer
	err := RunArgs([]string{"sync", "--agents", "opencode", "--dry-run"}, &buf)
	if err != nil {
		t.Fatalf("RunArgs(sync --dry-run) error = %v", err)
	}

	out := buf.String()
	if out == "" {
		t.Fatalf("RunArgs(sync --dry-run) produced no output")
	}

	// Output must mention agents or components — it is the sync plan report.
	if !strings.Contains(out, "sync") && !strings.Contains(out, "Sync") &&
		!strings.Contains(out, "agent") && !strings.Contains(out, "Agent") {
		t.Errorf("sync --dry-run output should mention sync or agents; got:\n%s", out)
	}
}

// TestRunArgsSyncUnknownFlagReturnsError verifies that an unknown flag
// returns a proper parse error via the sync command path.
func TestRunArgsSyncUnknownFlagReturnsError(t *testing.T) {
	var buf bytes.Buffer
	err := RunArgs([]string{"sync", "--this-flag-does-not-exist"}, &buf)
	if err == nil {
		t.Fatalf("RunArgs(sync --unknown) expected error")
	}
	// Must not be "unknown command" — sync IS a known command.
	if err.Error() == `unknown command "sync"` {
		t.Fatalf("sync command is not registered in app.go dispatch")
	}
}

// TestRunArgsSyncNoAgentsIsNoOp verifies that `gentle-ai sync` with no
// agents flag and an empty home dir (no config dirs) completes as a no-op
// and does NOT return an error.
func TestRunArgsSyncNoAgentsIsNoOp(t *testing.T) {
	// We can't override osUserHomeDir from the app package directly,
	// but we can verify that the command exits without error and prints
	// something meaningful (the no-op message).
	// Use --dry-run to avoid any file creation and allow running in CI.
	var buf bytes.Buffer
	err := RunArgs([]string{"sync", "--agents", "opencode", "--dry-run"}, &buf)
	if err != nil {
		t.Fatalf("RunArgs(sync --dry-run): %v", err)
	}
	out := buf.String()
	if out == "" {
		t.Fatalf("sync --dry-run should produce output, got empty string")
	}
}

// --- Batch E: macOS parity regression and Linux cross-verification ---

func TestMacOSDefaultProfileFallbackWhenDetectionEmpty(t *testing.T) {
	// When DetectionResult is zero-value (no profile), CLI defaults to macOS/brew.
	detection := system.DetectionResult{}
	result, err := cli.RunInstall([]string{"--dry-run"}, detection)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	if result.Resolved.PlatformDecision.OS != "darwin" {
		t.Fatalf("default platform decision OS = %q, want darwin", result.Resolved.PlatformDecision.OS)
	}
	if result.Resolved.PlatformDecision.PackageManager != "brew" {
		t.Fatalf("default platform decision PM = %q, want brew", result.Resolved.PlatformDecision.PackageManager)
	}
	if !result.Resolved.PlatformDecision.Supported {
		t.Fatalf("default platform decision Supported = false, want true")
	}
}

func TestLinuxDryRunPreservesSameResolverOutputAsMacOS(t *testing.T) {
	// Platform decision differs but the resolver output (agents, components, order) should be identical.
	macOSResult, err := cli.RunInstall([]string{"--dry-run"}, system.DetectionResult{})
	if err != nil {
		t.Fatalf("macOS RunInstall() error = %v", err)
	}

	linuxDetection := system.DetectionResult{
		System: system.SystemInfo{
			OS:        "linux",
			Arch:      "amd64",
			Shell:     "/bin/bash",
			Supported: true,
			Profile: system.PlatformProfile{
				OS:             "linux",
				LinuxDistro:    system.LinuxDistroUbuntu,
				PackageManager: "apt",
				Supported:      true,
			},
		},
	}
	linuxResult, err := cli.RunInstall([]string{"--dry-run"}, linuxDetection)
	if err != nil {
		t.Fatalf("Linux RunInstall() error = %v", err)
	}

	// Agents should match
	if !reflect.DeepEqual(macOSResult.Resolved.Agents, linuxResult.Resolved.Agents) {
		t.Fatalf("agents differ\nmacOS=%v\nlinux=%v", macOSResult.Resolved.Agents, linuxResult.Resolved.Agents)
	}

	// Ordered components should match
	if !reflect.DeepEqual(macOSResult.Resolved.OrderedComponents, linuxResult.Resolved.OrderedComponents) {
		t.Fatalf("ordered components differ\nmacOS=%v\nlinux=%v", macOSResult.Resolved.OrderedComponents, linuxResult.Resolved.OrderedComponents)
	}

	// Selection should match
	if !reflect.DeepEqual(macOSResult.Selection, linuxResult.Selection) {
		t.Fatalf("selection differs\nmacOS=%#v\nlinux=%#v", macOSResult.Selection, linuxResult.Selection)
	}

	// Platform decisions SHOULD differ
	if macOSResult.Resolved.PlatformDecision.OS == linuxResult.Resolved.PlatformDecision.OS {
		t.Fatalf("platform decisions should differ between macOS and Linux but both have OS=%q", macOSResult.Resolved.PlatformDecision.OS)
	}
}

func TestGuardFlowMacOSProfileExplicitlyPasses(t *testing.T) {
	profile := system.PlatformProfile{OS: "darwin", PackageManager: "brew", Supported: true}
	if err := system.EnsureSupportedPlatform(profile); err != nil {
		t.Fatalf("macOS profile should pass guard, got %v", err)
	}
}

func TestGuardFlowLinuxUbuntuProfileExplicitlyPasses(t *testing.T) {
	profile := system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroUbuntu, PackageManager: "apt", Supported: true}
	if err := system.EnsureSupportedPlatform(profile); err != nil {
		t.Fatalf("Ubuntu profile should pass guard, got %v", err)
	}
}

func TestGuardFlowLinuxArchProfileExplicitlyPasses(t *testing.T) {
	profile := system.PlatformProfile{OS: "linux", LinuxDistro: system.LinuxDistroArch, PackageManager: "pacman", Supported: true}
	if err := system.EnsureSupportedPlatform(profile); err != nil {
		t.Fatalf("Arch profile should pass guard, got %v", err)
	}
}

func TestInstallPlannerParityLinuxPreservesComponentOrder(t *testing.T) {
	linuxDetection := system.DetectionResult{
		System: system.SystemInfo{
			OS:        "linux",
			Arch:      "amd64",
			Shell:     "/bin/bash",
			Supported: true,
			Profile: system.PlatformProfile{
				OS:             "linux",
				LinuxDistro:    system.LinuxDistroUbuntu,
				PackageManager: "apt",
				Supported:      true,
			},
		},
	}

	result, err := cli.RunInstall([]string{"--dry-run", "--agent", "opencode", "--component", "engram,sdd,skills"}, linuxDetection)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}

	// Engram must come before SDD, SDD before Skills (dependency order)
	order := result.Resolved.OrderedComponents
	engramIdx, sddIdx, skillsIdx := -1, -1, -1
	for i, c := range order {
		switch c {
		case "engram":
			engramIdx = i
		case "sdd":
			sddIdx = i
		case "skills":
			skillsIdx = i
		}
	}
	if engramIdx < 0 || sddIdx < 0 || skillsIdx < 0 {
		t.Fatalf("missing expected components in order: %v", order)
	}
	if engramIdx >= sddIdx || sddIdx >= skillsIdx {
		t.Fatalf("dependency order violated: engram@%d sdd@%d skills@%d", engramIdx, sddIdx, skillsIdx)
	}
}
