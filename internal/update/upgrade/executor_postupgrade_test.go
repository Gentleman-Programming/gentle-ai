package upgrade

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
	"github.com/gentleman-programming/gentle-ai/v2/internal/update"
)

// fakeGoInstallProfile routes effectiveMethod to InstallGoInstall without
// touching Homebrew or a real network — the cheapest deterministic path for
// exercising executeOne's success branch.
func fakeGoInstallProfile() system.PlatformProfile {
	return system.PlatformProfile{OS: "linux", PackageManager: "apt", GoAvailable: true, Supported: true}
}

func stubGoInstallExec(t *testing.T) {
	t.Helper()
	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })
	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "go" && len(args) == 2 && args[0] == "env" {
			return mockCmd("true")
		}
		if name == "go" && len(args) >= 1 && args[0] == "install" {
			return mockCmd("true")
		}
		t.Fatalf("unexpected command %s %v", name, args)
		return nil
	}
}

func TestExecuteOnePostUpgrade(t *testing.T) {
	t.Run("declared and upgrade succeeds: invoked once", func(t *testing.T) {
		stubGoInstallExec(t)
		var calls int
		tool := update.ToolInfo{
			Name: "fake-tool", GoImportPath: "example.com/fake-tool/cmd/fake-tool",
			PostUpgrade: func(ctx context.Context) error { calls++; return nil },
		}
		result := executeOne(context.Background(), update.UpdateResult{Tool: tool, LatestVersion: "1.0.0"}, fakeGoInstallProfile(), false)
		if result.Status != UpgradeSucceeded {
			t.Fatalf("Status = %v, want UpgradeSucceeded", result.Status)
		}
		if calls != 1 {
			t.Fatalf("PostUpgrade invoked %d times, want 1", calls)
		}
	})

	t.Run("nil field: identical to today", func(t *testing.T) {
		stubGoInstallExec(t)
		tool := update.ToolInfo{Name: "fake-tool", GoImportPath: "example.com/fake-tool/cmd/fake-tool"}
		result := executeOne(context.Background(), update.UpdateResult{Tool: tool, LatestVersion: "1.0.0"}, fakeGoInstallProfile(), false)
		if result.Status != UpgradeSucceeded {
			t.Fatalf("Status = %v, want UpgradeSucceeded", result.Status)
		}
		if result.ManualHint != "" {
			t.Fatalf("ManualHint = %q, want empty", result.ManualHint)
		}
	})

	t.Run("step fails: UpgradeSkipped with a non-empty ManualHint and nil Err", func(t *testing.T) {
		stubGoInstallExec(t)
		tool := update.ToolInfo{
			Name: "fake-tool", GoImportPath: "example.com/fake-tool/cmd/fake-tool",
			PostUpgrade: func(ctx context.Context) error { return errors.New("emit failed") },
		}
		result := executeOne(context.Background(), update.UpdateResult{Tool: tool, LatestVersion: "1.0.0"}, fakeGoInstallProfile(), false)
		if result.Status != UpgradeSkipped {
			t.Fatalf("Status = %v, want UpgradeSkipped", result.Status)
		}
		if result.ManualHint == "" {
			t.Fatal("ManualHint is empty, want a non-empty hint naming the failed step")
		}
		if result.Err != nil {
			t.Fatalf("Err = %v, want nil — the binary itself did upgrade", result.Err)
		}
		if result.NewVersion != "1.0.0" {
			t.Fatalf("NewVersion = %q, want the upgraded version to be kept", result.NewVersion)
		}
	})

	t.Run("upgrade itself fails: PostUpgrade is never invoked", func(t *testing.T) {
		origExecCommand := execCommand
		t.Cleanup(func() { execCommand = origExecCommand })
		execCommand = func(name string, args ...string) *exec.Cmd {
			if name == "go" && len(args) == 2 && args[0] == "env" {
				return mockCmd("true")
			}
			return mockCmd("false")
		}
		var calls int
		tool := update.ToolInfo{
			Name: "fake-tool", GoImportPath: "example.com/fake-tool/cmd/fake-tool",
			PostUpgrade: func(ctx context.Context) error { calls++; return nil },
		}
		result := executeOne(context.Background(), update.UpdateResult{Tool: tool, LatestVersion: "1.0.0"}, fakeGoInstallProfile(), false)
		if result.Status != UpgradeFailed {
			t.Fatalf("Status = %v, want UpgradeFailed", result.Status)
		}
		if calls != 0 {
			t.Fatalf("PostUpgrade invoked %d times after a failed upgrade, want 0", calls)
		}
	})
}
