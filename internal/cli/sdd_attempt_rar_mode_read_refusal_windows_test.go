//go:build windows

package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

func TestRunSDDAttemptSettleIgnoresUnsafeRDDModeAuthority(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	const change = "unsafe-rar-mode"
	disableReviewForClone(t, repo)

	started, _ := runCompactSDDAttempt(t, compactAcquireArgs(repo, change, "unsafe-mode-acquire", 2))
	store, err := sddstatus.OpenRuntimeStore(context.Background(), repo, change)
	if err != nil {
		t.Fatal(err)
	}
	privateRARDir := filepath.Join(repo, ".git", "gentle-ai", "review-mode", "rar-authority", "v1")
	unsafeParentSetup := "$p = " + quotePowerShellLiteral(privateRARDir) + "; $acl = [System.IO.Directory]::GetAccessControl($p); $acl.SetAccessRuleProtection($false, $true); [System.IO.Directory]::SetAccessControl($p, $acl)"
	windowsRunPowerShell(t, unsafeParentSetup)

	rarBefore := snapshotRuntimeAuthorityFiles(t, privateRARDir)
	completed, _ := runCompactSDDAttempt(t, compactSettleArgs(repo, change, started.Token, "unsafe-mode-settle", "passed"))
	if completed.State != "complete" {
		t.Fatalf("unsafe RDD metadata changed SDD settlement = %#v", completed)
	}
	if rarAfter := snapshotRuntimeAuthorityFiles(t, privateRARDir); !reflect.DeepEqual(rarBefore, rarAfter) {
		t.Fatalf("SDD settlement touched unsafe RDD authority\nbefore=%v\nafter=%v", rarBefore, rarAfter)
	}
	if status, err := store.Status(); err != nil || !status.Complete {
		t.Fatalf("settled runtime status = %#v err=%v", status, err)
	}
}

func TestReviewModeUnsafeFileRepairCommandRunsOnWindowsPowerShell(t *testing.T) {
	target := filepath.Join(t.TempDir(), "unsafe file")
	if err := os.WriteFile(target, []byte("private\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	windowsRunPowerShell(t, (&reviewModeUnsafePathError{Path: target}).repairCommand())
}

func windowsRunPowerShell(t *testing.T, script string) {
	t.Helper()
	output, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script).CombinedOutput()
	if err != nil {
		t.Fatalf("PowerShell script failed: %v\n%s", err, output)
	}
}
