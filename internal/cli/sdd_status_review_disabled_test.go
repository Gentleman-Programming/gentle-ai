package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

// The kill switch reaches SDD status through this layer, which owns the single
// source of truth for both of its sources. These tests hold the three
// properties that make the seam safe: the switch actually reaches the archive
// gate, an unreadable switch is not a disabled switch, and a disabled run never
// produces something that reads as an approval.

// seedArchiveGatedSDDChange stages an SDD change that has reached its archive
// decision inside a real repository, with no review authority of any kind.
func seedArchiveGatedSDDChange(t *testing.T, root string) {
	t.Helper()
	changeRoot := filepath.Join(root, "openspec", "changes", "thin")
	writeSDDStatusFile(t, filepath.Join(changeRoot, "proposal.md"), "# Proposal\n")
	writeSDDStatusFile(t, filepath.Join(changeRoot, "specs", "auth", "spec.md"), "### Requirement: Auth\n#### Scenario: Valid login\n")
	writeSDDStatusFile(t, filepath.Join(changeRoot, "design.md"), "# Design\n")
	writeSDDStatusFile(t, filepath.Join(changeRoot, "tasks.md"), "- [x] 1.1 Work\n")
	writeSDDStatusFile(t, filepath.Join(changeRoot, "verify-report.md"), strings.Join([]string{
		"```yaml",
		"schema: gentle-ai.verify-result/v1",
		"evidence_revision: sha256:" + strings.Repeat("1", 64),
		"verdict: pass",
		"blockers: 0",
		"critical_findings: 0",
		"requirements: 1/1",
		"scenarios: 1/1",
		"test_command: go test ./internal/example",
		"test_exit_code: 0",
		"test_output_hash: sha256:" + strings.Repeat("2", 64),
		"build_command: go test ./cmd/gentle-ai",
		"build_exit_code: 0",
		"build_output_hash: sha256:" + strings.Repeat("3", 64),
		"```",
	}, "\n"))
	runReviewCLIGit(t, root, "init", "-q")
	runReviewCLIGit(t, root, "config", "user.email", "test@example.com")
	runReviewCLIGit(t, root, "config", "user.name", "Test")
	runReviewCLIGit(t, root, "add", ".")
	runReviewCLIGit(t, root, "commit", "-qm", "base")
}

func runSDDCommandJSON(t *testing.T, run func([]string, io.Writer) error, args ...string) sddstatus.Status {
	t.Helper()
	var stdout bytes.Buffer
	if err := run(args, &stdout); err != nil {
		t.Fatalf("SDD command error = %v\n%s", err, stdout.String())
	}
	var status sddstatus.Status
	if err := json.Unmarshal(stdout.Bytes(), &status); err != nil {
		t.Fatalf("decode sdd status: %v\n%s", err, stdout.String())
	}
	return status
}

func resolveSDDStatusJSON(t *testing.T, root string) sddstatus.Status {
	t.Helper()
	return runSDDCommandJSON(t, RunSDDStatus, "thin", "--cwd", root, "--json")
}

func requireDisabledUnmanagedSDDStatus(t *testing.T, status sddstatus.Status) {
	t.Helper()
	if status.Dependencies.Archive == sddstatus.DependencyBlocked || status.NextRecommended == "resolve-review" {
		t.Fatalf("disabled archive=%q next=%q blocked=%v, want an unmanaged route to archive",
			status.Dependencies.Archive, status.NextRecommended, status.BlockedReasons)
	}
	if status.ReviewGate == nil || status.ReviewGate.Delivery != reviewtransaction.RDDDeliveryDisabledUnmanaged {
		t.Fatalf("disabled reviewGate = %#v, want disabled/unmanaged", status.ReviewGate)
	}
	if status.ReviewGate.Result == reviewtransaction.GateAllow || status.ReviewTransaction != nil {
		t.Fatalf("disabled status fabricated review authority: gate=%#v transaction=%#v", status.ReviewGate, status.ReviewTransaction)
	}
}

// corruptCloneLocalReviewMode damages the clone-local override head record.
// The switch becomes unreadable, which is not the same as being off.
func corruptCloneLocalReviewMode(t *testing.T, repo string) {
	t.Helper()
	root := filepath.Join(repo, ".git", "gentle-ai", "review-transactions")
	corrupted := 0
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Base(filepath.Dir(path)) != "rdd-mode" {
			return nil
		}
		if !strings.HasPrefix(entry.Name(), "gen-") {
			return nil
		}
		if writeErr := os.WriteFile(path, []byte("{\n"), 0o644); writeErr != nil {
			return writeErr
		}
		corrupted++
		return nil
	}); err != nil {
		t.Fatalf("walk clone-local review mode records: %v", err)
	}
	if corrupted == 0 {
		t.Fatal("no clone-local review mode record to corrupt; the fixture never wrote one")
	}
}

// TestSDDStatusArchiveGateBlocksWhileReviewIsEnabled pins today's behaviour for
// the exact fixture the disabled test relaxes. The enabled path must not move.
func TestSDDStatusArchiveGateBlocksWhileReviewIsEnabled(t *testing.T) {
	reviewModeHome(t)
	root := t.TempDir()
	seedArchiveGatedSDDChange(t, root)

	if reviewDrivenDevelopmentDisabled(context.Background(), root) {
		t.Fatal("fixture is wrong: receipt-driven development is not enabled")
	}
	status := resolveSDDStatusJSON(t, root)
	if status.ReviewGate == nil || status.ReviewGate.Result != reviewtransaction.GateInvalidated {
		t.Fatalf("enabled reviewGate = %#v, want invalidated", status.ReviewGate)
	}
	if status.ReviewGate.Delivery != "" {
		t.Fatalf("enabled reviewGate.delivery = %q, want absent from the enabled wire shape", status.ReviewGate.Delivery)
	}
	if status.Dependencies.Archive != sddstatus.DependencyBlocked || status.NextRecommended != "resolve-review" {
		t.Fatalf("enabled archive=%q next=%q, want blocked/resolve-review", status.Dependencies.Archive, status.NextRecommended)
	}
}

// TestSDDStatusArchiveGateCarriesOnWhileReviewIsDisabled is the seam under
// test: the switch the user set actually reaches the archive gate.
func TestSDDStatusArchiveGateCarriesOnWhileReviewIsDisabled(t *testing.T) {
	reviewModeHome(t)
	root := t.TempDir()
	seedArchiveGatedSDDChange(t, root)
	disableReviewForClone(t, root)

	status := resolveSDDStatusJSON(t, root)
	requireDisabledUnmanagedSDDStatus(t, status)
}

func TestSDDCommandsWithoutCWDHonorDisabledReviewMode(t *testing.T) {
	reviewModeHome(t)
	root := t.TempDir()
	seedArchiveGatedSDDChange(t, root)
	disableReviewForClone(t, root)
	t.Chdir(root)

	commands := []struct {
		name string
		run  func([]string, io.Writer) error
	}{
		{name: "status", run: RunSDDStatus},
		{name: "continue", run: RunSDDContinue},
	}
	for _, command := range commands {
		t.Run(command.name, func(t *testing.T) {
			status := runSDDCommandJSON(t, command.run, "thin", "--json")
			requireDisabledUnmanagedSDDStatus(t, status)
			if status.ActionContext.WorkspaceRoot != root {
				t.Fatalf("workspace root = %q, want %q", status.ActionContext.WorkspaceRoot, root)
			}
		})
	}
}

func TestSDDStatusWithoutCWDUsesLinkedWorktreeCommonDirMode(t *testing.T) {
	reviewModeHome(t)
	root := t.TempDir()
	seedArchiveGatedSDDChange(t, root)
	disableReviewForClone(t, root)
	linked := filepath.Join(t.TempDir(), "linked")
	runReviewCLIGit(t, root, "worktree", "add", "-q", "-b", "linked-status", linked)
	t.Chdir(linked)

	status := runSDDCommandJSON(t, RunSDDStatus, "thin", "--json")
	requireDisabledUnmanagedSDDStatus(t, status)
	if status.ActionContext.WorkspaceRoot != linked {
		t.Fatalf("workspace root = %q, want linked worktree %q", status.ActionContext.WorkspaceRoot, linked)
	}
}

// TestSDDStatusArchiveGateEnforcesWhenTheSwitchIsUnreadable holds the last
// property: a broken or tampered mode record must never be able to relax the
// archive gate, so an unreadable switch behaves exactly like an enabled one.
func TestSDDStatusArchiveGateEnforcesWhenTheSwitchIsUnreadable(t *testing.T) {
	reviewModeHome(t)
	root := t.TempDir()
	seedArchiveGatedSDDChange(t, root)
	disableReviewForClone(t, root)
	corruptCloneLocalReviewMode(t, root)

	if reviewDrivenDevelopmentDisabled(context.Background(), root) {
		t.Fatal("an unreadable switch resolved to disabled; it must fail closed to enabled")
	}
	t.Chdir(root)
	for _, args := range [][]string{
		{"thin", "--cwd", root, "--json"},
		{"thin", "--json"},
	} {
		status := runSDDCommandJSON(t, RunSDDStatus, args...)
		if status.ReviewGate == nil || status.ReviewGate.Delivery != "" {
			t.Fatalf("unreadable-switch reviewGate = %#v, want the enforcing shape", status.ReviewGate)
		}
		if status.Dependencies.Archive != sddstatus.DependencyBlocked || status.NextRecommended != "resolve-review" {
			t.Fatalf("unreadable-switch archive=%q next=%q, want blocked/resolve-review", status.Dependencies.Archive, status.NextRecommended)
		}
	}
}
