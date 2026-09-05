package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
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

// requireDisabledUnmanagedSDDStatus asserts the corrective verify cycle's
// CRITICAL-1 fix (rdd-post-verify-review-offer's "Kill-Switch-Off Is
// Structural Absence" requirement): while the switch is off, archive is
// never review-blocked AND the reviewGate field itself is structurally
// absent — not populated with a disabled/unmanaged disposition, which is
// the ceremony the ratified requirement forbids ("no disabled/unmanaged
// ceremony capable of failing or blocking").
func requireDisabledUnmanagedSDDStatus(t *testing.T, status sddstatus.Status) {
	t.Helper()
	if status.Dependencies.Archive == sddstatus.DependencyBlocked || status.NextRecommended == "resolve-review" {
		t.Fatalf("disabled archive=%q next=%q blocked=%v, want an unmanaged route to archive",
			status.Dependencies.Archive, status.NextRecommended, status.BlockedReasons)
	}
	if status.ReviewOffer != nil {
		t.Fatalf("disabled reviewOffer = %#v, want structural absence", status.ReviewOffer)
	}
}

// corruptCloneLocalReviewMode damages the authoritative clone-local head.
// The fixture creates exactly generation 1, so corrupting that exact path
// proves status does not confuse a mode-resolution failure with a gate result.
func corruptCloneLocalReviewMode(t *testing.T, repo string) {
	t.Helper()
	path, err := reviewtransaction.CloneLocalRDDModeRecordPath(context.Background(), repo)
	if err != nil {
		t.Fatalf("locate clone-local review mode head: %v", err)
	}
	if path == "" {
		t.Fatal("no clone-local review mode head to corrupt; the fixture never wrote one")
	}
	if err := os.WriteFile(path, []byte("{\n"), 0o600); err != nil {
		t.Fatalf("corrupt clone-local review mode head: %v", err)
	}
}

// seedScopeChangedApprovedSDDChange leaves a historical scope change behind
// after terminal closure burns its baseline authority. The historical review is not a
// governing receipt and must not become an SDD archive blocker.
func seedScopeChangedApprovedSDDChange(t *testing.T, root string) {
	t.Helper()
	seedArchiveGatedSDDChange(t, root)
	writeSDDStatusFile(t, root+"/docs/baseline.md", "# baseline\n\nplain prose, no executable content.\n")
	runReviewCLIGit(t, root, "add", "-A")
	started := startFacadeReviewResult(t, root, "scope-changed-baseline")
	if started.State != "approved" || started.Action != "closed" {
		t.Fatalf("baseline zero-lens START = %#v", started)
	}
	commitAllSDDStatus(t, root, "baseline reviewed delivery")
	writeSDDStatusFile(t, root+"/docs/scope-changed.md", "# scope changed\n\nplain prose, delivered after approval.\n")
	commitAllSDDStatus(t, root, "scope changed after approval")
}

// TestSDDStatusArchiveGateBlocksWhileReviewIsEnabled retains its stable fixture
// name while pinning #3417's replacement behavior: terminal closure burned the old
// approval, so a historical scope change has no deciding gate. Enabled status
// offers a new review but archive proceeds under ordinary repository policy.
func TestSDDStatusArchiveGateBlocksWhileReviewIsEnabled(t *testing.T) {
	reviewEnabledHome(t)
	root := t.TempDir()
	seedScopeChangedApprovedSDDChange(t, root)

	if disabled, err := reviewDrivenDevelopmentDisabled(context.Background(), root); err != nil || disabled {
		t.Fatalf("fixture must enable receipt-driven development: disabled=%t err=%v", disabled, err)
	}
	status := resolveSDDStatusJSON(t, root)
	requireEnabledOrdinaryArchive(t, status, "enabled archive after burned scope-changed review")
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

// TestSDDStatusArchiveGateEnforcesWhenTheSwitchIsUnreadable keeps the two
// decisions separate. The mode reader still reports malformed persisted state
// as a real error, but SDD status fails closed to enabled and has no governing
// receipt to enforce. It must therefore proceed under ordinary policy without
// fabricating a reviewGate from the mode error.
func TestSDDStatusArchiveGateEnforcesWhenTheSwitchIsUnreadable(t *testing.T) {
	reviewEnabledHome(t)
	root := t.TempDir()
	seedArchiveGatedSDDChange(t, root)
	disableReviewForClone(t, root)
	corruptCloneLocalReviewMode(t, root)

	mode, modeErr := reviewModeStatus(context.Background(), root)
	if modeErr == nil || mode.Enabled() {
		t.Fatalf("unreadable switch must remain a failed-closed mode resolution: mode=%#v err=%v", mode, modeErr)
	}
	var unreadable *ReviewModeUnreadableError
	if !errors.As(modeErr, &unreadable) {
		t.Fatalf("mode-resolution error = %T %v, want ReviewModeUnreadableError", modeErr, modeErr)
	}
	if disabled, err := reviewDrivenDevelopmentDisabled(context.Background(), root); err != nil || disabled {
		t.Fatalf("SDD mode projection = disabled=%t err=%v, want enabled fallback for the non-deciding archive path", disabled, err)
	}

	t.Chdir(root)
	for _, args := range [][]string{
		{"thin", "--cwd", root, "--json"},
		{"thin", "--json"},
	} {
		status := runSDDCommandJSON(t, RunSDDStatus, args...)
		if status.ReviewOffer != nil {
			t.Fatalf("unreadable mode fabricated review offer: %#v", status.ReviewOffer)
		}
		if status.Dependencies.Archive != sddstatus.DependencyReady || status.NextRecommended != "archive" {
			t.Fatalf("unreadable mode archive=%q next=%q, want ordinary ready/archive", status.Dependencies.Archive, status.NextRecommended)
		}
	}
}
