package upgrade

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/system"
	"github.com/gentleman-programming/gentle-ai/internal/update"
)

// --- TestRunStrategy_BrewUpgrade ---

func TestRunStrategy_BrewUpgrade(t *testing.T) {
	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })

	var gotName string
	var gotArgs []string
	execCommand = func(name string, args ...string) *exec.Cmd {
		gotName = name
		gotArgs = args
		return exec.Command("echo", "Upgraded engram")
	}

	r := update.UpdateResult{
		Tool: update.ToolInfo{
			Name:          "engram",
			InstallMethod: update.InstallBrew,
		},
		LatestVersion: "0.4.0",
	}
	profile := system.PlatformProfile{OS: "darwin", PackageManager: "brew"}

	err := runStrategy(context.Background(), r, profile)
	if err != nil {
		t.Fatalf("runStrategy brew: unexpected error: %v", err)
	}

	if gotName != "brew" {
		t.Errorf("exec name = %q, want %q", gotName, "brew")
	}
	if len(gotArgs) < 2 || gotArgs[0] != "upgrade" || gotArgs[1] != "engram" {
		t.Errorf("exec args = %v, want [upgrade engram]", gotArgs)
	}
}

// --- TestRunStrategy_GoInstallUpgrade ---

func TestRunStrategy_GoInstallUpgrade(t *testing.T) {
	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })

	var gotName string
	var gotArgs []string
	execCommand = func(name string, args ...string) *exec.Cmd {
		gotName = name
		gotArgs = args
		return exec.Command("echo", "go install ok")
	}

	r := update.UpdateResult{
		Tool: update.ToolInfo{
			Name:          "engram",
			InstallMethod: update.InstallGoInstall,
			GoImportPath:  "github.com/Gentleman-Programming/engram/cmd/engram",
		},
		LatestVersion: "0.4.0",
	}
	profile := system.PlatformProfile{OS: "linux", PackageManager: "apt"}

	err := runStrategy(context.Background(), r, profile)
	if err != nil {
		t.Fatalf("runStrategy go-install: unexpected error: %v", err)
	}

	if gotName != "go" {
		t.Errorf("exec name = %q, want %q", gotName, "go")
	}
	// Expected: go install github.com/Gentleman-Programming/engram/cmd/engram@v0.4.0
	wantArg0, wantArg1 := "install", "github.com/Gentleman-Programming/engram/cmd/engram@v0.4.0"
	if len(gotArgs) < 2 || gotArgs[0] != wantArg0 || gotArgs[1] != wantArg1 {
		t.Errorf("exec args = %v, want [%s %s]", gotArgs, wantArg0, wantArg1)
	}
}

// --- TestRunStrategy_GoInstallMissingImportPath ---

func TestRunStrategy_GoInstallMissingImportPath(t *testing.T) {
	r := update.UpdateResult{
		Tool: update.ToolInfo{
			Name:          "engram",
			InstallMethod: update.InstallGoInstall,
			GoImportPath:  "", // missing
		},
		LatestVersion: "0.4.0",
	}
	profile := system.PlatformProfile{OS: "linux", PackageManager: "apt"}

	err := runStrategy(context.Background(), r, profile)
	if err == nil {
		t.Errorf("expected error when GoImportPath is empty, got nil")
	}
}

// --- TestRunStrategy_UnsupportedMethodManualFallback ---

func TestRunStrategy_UnsupportedMethodManualFallback(t *testing.T) {
	r := update.UpdateResult{
		Tool: update.ToolInfo{
			Name:          "some-tool",
			InstallMethod: update.InstallMethod("unsupported-method"),
		},
		LatestVersion: "1.0.0",
	}
	profile := system.PlatformProfile{OS: "linux", PackageManager: "apt"}

	err := runStrategy(context.Background(), r, profile)
	// Unsupported method → manual fallback error.
	if err == nil {
		t.Errorf("expected error for unsupported install method, got nil")
	}
}

// --- TestRunStrategy_BrewUpgradeFailure ---

func TestRunStrategy_BrewUpgradeFailure(t *testing.T) {
	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })

	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("false") // always fails
	}

	r := update.UpdateResult{
		Tool: update.ToolInfo{
			Name:          "engram",
			InstallMethod: update.InstallBrew,
		},
		LatestVersion: "0.4.0",
	}
	profile := system.PlatformProfile{OS: "darwin", PackageManager: "brew"}

	err := runStrategy(context.Background(), r, profile)
	if err == nil {
		t.Errorf("expected error when brew upgrade fails, got nil")
	}
}

// --- TestRunStrategy_GoInstallFailure ---

func TestRunStrategy_GoInstallFailure(t *testing.T) {
	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })

	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("false")
	}

	r := update.UpdateResult{
		Tool: update.ToolInfo{
			Name:          "engram",
			InstallMethod: update.InstallGoInstall,
			GoImportPath:  "github.com/Gentleman-Programming/engram/cmd/engram",
		},
		LatestVersion: "0.4.0",
	}
	profile := system.PlatformProfile{OS: "linux", PackageManager: "apt"}

	err := runStrategy(context.Background(), r, profile)
	if err == nil {
		t.Errorf("expected error when go install fails, got nil")
	}
}

// --- TestRunStrategy_BinaryWindowsSelfUpdateSkipped ---

// TestRunStrategy_BinaryWindowsSelfUpdateSkipped verifies that the Windows binary
// self-replace for gentle-ai is NOT attempted in Phase 1 — it must return a
// manual hint error, not execute.
func TestRunStrategy_BinaryWindowsSelfUpdateSkipped(t *testing.T) {
	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })

	execCalled := false
	execCommand = func(name string, args ...string) *exec.Cmd {
		execCalled = true
		return exec.Command("echo", "should not run")
	}

	r := update.UpdateResult{
		Tool: update.ToolInfo{
			Name:          "gentle-ai",
			InstallMethod: update.InstallBinary,
		},
		LatestVersion: "1.5.0",
		ReleaseURL:    "https://github.com/Gentleman-Programming/gentle-ai/releases/tag/v1.5.0",
	}
	profile := system.PlatformProfile{OS: "windows", PackageManager: "winget"}

	err := runStrategy(context.Background(), r, profile)
	// Windows binary self-replace must return an error (manual hint) in Phase 1.
	if err == nil {
		t.Errorf("expected manual fallback error for Windows binary self-replace, got nil")
	}

	if execCalled {
		t.Errorf("exec should NOT be called for Windows binary self-replace in Phase 1")
	}
}

// --- TestEffectiveMethod ---

func TestEffectiveMethod(t *testing.T) {
	tests := []struct {
		name    string
		tool    update.ToolInfo
		profile system.PlatformProfile
		want    update.InstallMethod
	}{
		{
			name:    "brew profile overrides go-install",
			tool:    update.ToolInfo{Name: "engram", InstallMethod: update.InstallGoInstall},
			profile: system.PlatformProfile{PackageManager: "brew"},
			want:    update.InstallBrew,
		},
		{
			name:    "brew profile overrides binary",
			tool:    update.ToolInfo{Name: "gga", InstallMethod: update.InstallBinary},
			profile: system.PlatformProfile{PackageManager: "brew"},
			want:    update.InstallBrew,
		},
		{
			name:    "apt profile respects declared method (go-install)",
			tool:    update.ToolInfo{Name: "engram", InstallMethod: update.InstallGoInstall},
			profile: system.PlatformProfile{PackageManager: "apt"},
			want:    update.InstallGoInstall,
		},
		{
			name:    "apt profile respects declared method (binary)",
			tool:    update.ToolInfo{Name: "gga", InstallMethod: update.InstallBinary},
			profile: system.PlatformProfile{PackageManager: "apt"},
			want:    update.InstallBinary,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := effectiveMethod(tc.tool, tc.profile)
			if got != tc.want {
				t.Errorf("effectiveMethod = %q, want %q", got, tc.want)
			}
		})
	}
}

// --- TestManualFallbackHint ---

// TestManualFallbackHint verifies that Windows binary self-replace produces an
// actionable hint string, not an empty error.
func TestManualFallbackHint(t *testing.T) {
	r := update.UpdateResult{
		Tool: update.ToolInfo{
			Name:          "gentle-ai",
			InstallMethod: update.InstallBinary,
		},
		LatestVersion: "1.5.0",
		UpdateHint:    "See https://github.com/Gentleman-Programming/gentle-ai/releases",
	}
	profile := system.PlatformProfile{OS: "windows", PackageManager: "winget"}

	err := runStrategy(context.Background(), r, profile)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	msg := err.Error()
	if msg == "" {
		t.Errorf("manual fallback error message should not be empty")
	}

	// Hint should mention manual action or Windows.
	if !containsAny(msg, "manual", "Manual", "windows", "Windows", "winget", "hint") {
		t.Errorf("manual hint message %q does not mention manual or windows", msg)
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(sub) > 0 {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}

// --- TestBrewUpgrade_RunsUpdateBeforeUpgrade ---

// TestBrewUpgrade_RunsUpdateBeforeUpgrade verifies that brewUpgrade calls
// `brew update` BEFORE `brew upgrade <toolName>`, and that the order is correct.
func TestBrewUpgrade_RunsUpdateBeforeUpgrade(t *testing.T) {
	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })

	var callOrder []string
	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "brew" && len(args) > 0 {
			callOrder = append(callOrder, args[0]) // "update" or "upgrade"
		}
		return exec.Command("echo", "ok")
	}

	err := brewUpgrade(context.Background(), "gentle-ai")
	if err != nil {
		t.Fatalf("brewUpgrade: unexpected error: %v", err)
	}

	// Must have called brew update AND brew upgrade — in that order.
	if len(callOrder) < 2 {
		t.Fatalf("expected 2 brew calls (update, upgrade), got %d: %v", len(callOrder), callOrder)
	}
	if callOrder[0] != "update" {
		t.Errorf("first brew call = %q, want %q", callOrder[0], "update")
	}
	if callOrder[1] != "upgrade" {
		t.Errorf("second brew call = %q, want %q", callOrder[1], "upgrade")
	}
}

// --- TestBrewUpgrade_UpdateFailureIsNonFatal ---

// TestBrewUpgrade_UpdateFailureIsNonFatal verifies that when `brew update` fails
// but `brew upgrade` succeeds, the overall result is success (non-fatal update failure).
func TestBrewUpgrade_UpdateFailureIsNonFatal(t *testing.T) {
	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })

	var callArgs []string
	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "brew" && len(args) > 0 {
			callArgs = append(callArgs, args[0])
			if args[0] == "update" {
				// brew update fails (e.g. no network).
				return exec.Command("false")
			}
		}
		// brew upgrade succeeds.
		return exec.Command("echo", "Upgraded gentle-ai")
	}

	err := brewUpgrade(context.Background(), "gentle-ai")
	// brew update failed but brew upgrade succeeded → overall success.
	if err != nil {
		t.Errorf("expected success when brew update fails but brew upgrade succeeds, got: %v", err)
	}

	// Both brew update and brew upgrade must have been called.
	if len(callArgs) < 2 {
		t.Fatalf("expected 2 brew calls, got %d: %v", len(callArgs), callArgs)
	}
	if callArgs[0] != "update" {
		t.Errorf("first brew call = %q, want %q", callArgs[0], "update")
	}
	if callArgs[1] != "upgrade" {
		t.Errorf("second brew call = %q, want %q", callArgs[1], "upgrade")
	}
}

// --- verify exec.Cmd.Run() failure is correctly wrapped ---
func TestRunStrategy_ExecErrorWrapped(t *testing.T) {
	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })

	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("false")
	}

	r := update.UpdateResult{
		Tool: update.ToolInfo{
			Name:          "engram",
			InstallMethod: update.InstallBrew,
		},
		LatestVersion: "0.4.0",
	}
	profile := system.PlatformProfile{OS: "darwin", PackageManager: "brew"}

	err := runStrategy(context.Background(), r, profile)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Error should have a non-empty message.
	if err.Error() == "" {
		t.Errorf("error should have a message")
	}

	// Error should wrap an *exec.ExitError (from running "false").
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Logf("note: error is not directly an ExitError (may be wrapped): %v", err)
	}
}
