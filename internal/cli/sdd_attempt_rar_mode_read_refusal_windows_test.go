//go:build windows

package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

func TestRunSDDAttemptSettleRepairsUnsafeDisabledRARModeWithoutRecursing(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	const change = "unsafe-rar-mode"
	disableReviewForClone(t, repo)

	started, _ := runCompactSDDAttempt(t, compactAcquireArgs(repo, change, "unsafe-mode-acquire", 2))
	store, err := sddstatus.OpenRuntimeStore(context.Background(), repo, change)
	if err != nil {
		t.Fatal(err)
	}
	privateRARDir := filepath.Join(repo, ".git", "gentle-ai", "review-transactions", "rar-authority", "v1")
	child := filepath.Join(privateRARDir, "must-not-change.txt")
	if err := os.WriteFile(child, []byte("private child\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	childBefore := windowsACLSDDL(t, child)
	windowsRunPowerShell(t, "$acl = Get-Acl -LiteralPath "+quotePowerShellLiteral(privateRARDir)+"; $acl.SetAccessRuleProtection($false, $true); Set-Acl -LiteralPath "+quotePowerShellLiteral(privateRARDir)+" -AclObject $acl")

	runtimeBefore := snapshotRuntimeAuthorityFiles(t, store.Dir)
	rarBefore := snapshotRuntimeAuthorityFiles(t, privateRARDir)
	settleArgs := compactSettleArgs(repo, change, started.Token, "unsafe-mode-settle", "passed")
	var output bytes.Buffer
	err = RunSDDAttempt(settleArgs, &output)
	var refusal *reviewModeUnsafePathError
	if !errors.As(err, &refusal) || refusal.Path != privateRARDir || !refusal.Directory {
		t.Fatalf("unsafe disabled mode refusal = %v", err)
	}
	if !strings.Contains(err.Error(), "NON-ELEVATED") || !strings.Contains(err.Error(), "TokenOwner") {
		t.Fatalf("unsafe disabled mode did not warn about elevation: %q", err)
	}
	if strings.Contains(err.Error(), "invalid_continuation") || output.Len() != 0 {
		t.Fatalf("unsafe disabled mode was misclassified: error=%q output=%q", err, output.String())
	}
	if runtimeAfter := snapshotRuntimeAuthorityFiles(t, store.Dir); !reflect.DeepEqual(runtimeBefore, runtimeAfter) {
		t.Fatalf("unsafe disabled mode settle mutated runtime authority\nbefore=%v\nafter=%v", runtimeBefore, runtimeAfter)
	}
	if rarAfter := snapshotRuntimeAuthorityFiles(t, privateRARDir); !reflect.DeepEqual(rarBefore, rarAfter) {
		t.Fatalf("unsafe disabled mode settle mutated RAR authority\nbefore=%v\nafter=%v", rarBefore, rarAfter)
	}

	windowsRunPowerShell(t, refusal.repairCommand())
	status, err := reviewtransaction.ResolveRDDMode(context.Background(), repo, reviewtransaction.RDDGlobalMode{})
	if err != nil || status.Enabled() {
		t.Fatalf("printed repair did not restore the real RAR predicate: status=%#v error=%v", status, err)
	}
	if childAfter := windowsACLSDDL(t, child); childAfter != childBefore {
		t.Fatalf("printed repair recursively changed child ACL\nbefore=%q\nafter=%q", childBefore, childAfter)
	}
	completed, _ := runCompactSDDAttempt(t, settleArgs)
	if completed.State != "complete" {
		t.Fatalf("replayed settle after repair = %#v, want complete", completed)
	}
}

func windowsRunPowerShell(t *testing.T, script string) {
	t.Helper()
	output, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script).CombinedOutput()
	if err != nil {
		t.Fatalf("PowerShell script failed: %v\n%s", err, output)
	}
}

func windowsACLSDDL(t *testing.T, path string) string {
	t.Helper()
	output, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "(Get-Acl -LiteralPath "+quotePowerShellLiteral(path)+").Sddl").CombinedOutput()
	if err != nil {
		t.Fatalf("read ACL for %q: %v\n%s", path, err, output)
	}
	return strings.TrimSpace(string(output))
}
