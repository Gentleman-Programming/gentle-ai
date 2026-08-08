package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// pristineReviewingCLIFixture persists one pristine reviewing compact lineage
// directly into the v2 store, then restores the clean workspace so the lineage
// is also stale relative to the live worktree. It is the reviewing sibling of
// pristineInvalidatedCLIFixture: the exact shape issue #2528 reports locked in,
// a review that was started, never captured a lens result, and went stale.
func pristineReviewingCLIFixture(t *testing.T, repo string) (revision, snapshotIdentity string) {
	t.Helper()
	stale := filepath.Join(repo, "stale.go")
	if err := os.WriteFile(stale, []byte("package stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	snapshot, err := builder.Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, IntendedUntracked: []string{"stale.go"},
	})
	if err != nil {
		t.Fatal(err)
	}
	risk, lines, err := builder.ClassifySnapshotRisk(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	state, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: "abandon-stale-reviewing", Mode: reviewtransaction.ModeOrdinaryBounded, Generation: 1,
		Snapshot: snapshot, PolicyHash: "sha256:" + strings.Repeat("ab", 32), RiskLevel: risk,
		SelectedLenses: []string{"review-reliability"}, OriginalChangedLines: &lines,
	})
	if err != nil {
		t.Fatal(err)
	}
	revision = writeReconcileCLIRecord(t, repo, state)
	if err := os.Remove(stale); err != nil {
		t.Fatal(err)
	}
	return revision, state.InitialSnapshot.Identity
}

func abandonBindingFromInventory(t *testing.T, repo, lineage, revision, snapshotIdentity, actor, reason string) string {
	t.Helper()
	report, err := reviewtransaction.InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range report.Entries {
		if entry.LineageID == lineage && entry.DiscardedWork != nil {
			return reviewtransaction.RenderCompactAbandonAuthorization(lineage, revision, snapshotIdentity, actor, reason, *entry.DiscardedWork)
		}
	}
	t.Fatalf("review status publishes no abandoned-work summary for %q", lineage)
	return ""
}

func staleReviewingAbandonBinding(t *testing.T, repo, revision, snapshotIdentity string) string {
	return abandonBindingFromInventory(t, repo, "abandon-stale-reviewing", revision, snapshotIdentity,
		"maintainer@example.com", reviewtransaction.CompactAbandonReasonOperatorDisposition)
}

func staleReviewingAbandonArgs(repo, revision, authorization string) []string {
	return []string{
		"abandon", "--cwd", repo,
		"--lineage", "abandon-stale-reviewing", "--expected-revision", revision,
		"--reason", reviewtransaction.CompactAbandonReasonOperatorDisposition, "--actor", "maintainer@example.com",
		"--maintainer-authorization", authorization,
	}
}

// TestReviewAbandonSucceedsWhileKillSwitchDisabled is issue #2528: abandoning a
// non-terminal lineage is destructive cleanup, not progress on a review, so it must
// stay available while receipt-driven development is switched off. The primary
// reason an operator disables the switch is to get delivery past a stale-lineage
// denial; gating cleanup behind the switch locked in exactly the state that
// caused the denial.
func TestReviewAbandonSucceedsWhileKillSwitchDisabled(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	revision, snapshotIdentity := pristineReviewingCLIFixture(t, repo)
	disableReviewForClone(t, repo)

	var output bytes.Buffer
	if err := RunReview(staleReviewingAbandonArgs(repo, revision, staleReviewingAbandonBinding(t, repo, revision, snapshotIdentity)), &output); err != nil {
		t.Fatalf("review abandon while the kill switch is off: %v\n%s", err, output.String())
	}
	var result ReviewAbandonResult
	decodeStrictReviewJSON(t, output.Bytes(), &result)
	if result.Operation != "review/abandon" || result.Record.LineageID != "abandon-stale-reviewing" ||
		result.Record.Status != reviewtransaction.CompactReclaimCommitted ||
		result.Record.Abandonment == nil ||
		result.Record.Abandonment.Schema != reviewtransaction.CompactAbandonAuthorizationSchema {
		t.Fatalf("review abandon while disabled result = %#v", result)
	}
	if _, err := os.Stat(filepath.Join(reviewCLIAuthorityRoot(t, repo), "v2", "abandon-stale-reviewing")); !os.IsNotExist(err) {
		t.Fatalf("abandoned entry still present: %v", err)
	}
	if _, err := os.Stat(filepath.Join(result.Record.QuarantinePath, "reclaim-record.json")); err != nil {
		t.Fatalf("abandonment audit record missing: %v", err)
	}
}

// TestReviewAbandonWhileDisabledStillRequiresTheExactBinding pins that the
// relaxation removes only the kill-switch gate, never the maintainer
// authorization: a wrong binding is refused while disabled exactly as while
// enabled, and the refused abandon mutates nothing.
func TestReviewAbandonWhileDisabledStillRequiresTheExactBinding(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	revision, snapshotIdentity := pristineReviewingCLIFixture(t, repo)
	statePath := filepath.Join(reviewCLIAuthorityRoot(t, repo), "v2", "abandon-stale-reviewing", "review-state.json")
	payload, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	disableReviewForClone(t, repo)

	wrong := staleReviewingAbandonBinding(t, repo, snapshotIdentity, revision)
	if err := RunReview(staleReviewingAbandonArgs(repo, revision, wrong), &bytes.Buffer{}); err == nil ||
		!strings.Contains(err.Error(), "exact maintainer authorization binding") {
		t.Fatalf("inexact abandon binding while disabled error = %v", err)
	}
	current, err := os.ReadFile(statePath)
	if err != nil || !bytes.Equal(current, payload) {
		t.Fatalf("refused abandon mutated the entry: %v", err)
	}
}

// TestReviewAbandonWhileDisabledKeepsEligibilityAndRevisionChecks proves the
// cleanup classification does not admit arbitrary destruction: a stale revision
// and terminal authority both remain refused with the switch off.
func TestReviewAbandonWhileDisabledKeepsEligibilityAndRevisionChecks(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	revision, snapshotIdentity := pristineReviewingCLIFixture(t, repo)
	statePath := filepath.Join(reviewCLIAuthorityRoot(t, repo), "v2", "abandon-stale-reviewing", "review-state.json")
	payload, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	disableReviewForClone(t, repo)
	wrongRevision := "sha256:" + strings.Repeat("0", 64)
	if err := RunReview(staleReviewingAbandonArgs(repo, wrongRevision, staleReviewingAbandonBinding(t, repo, revision, snapshotIdentity)), &bytes.Buffer{}); !errors.Is(err, reviewtransaction.ErrConcurrentUpdate) {
		t.Fatalf("stale abandon revision while disabled = %v", err)
	}
	current, err := os.ReadFile(statePath)
	if err != nil || !bytes.Equal(current, payload) {
		t.Fatalf("stale revision mutated pristine authority: %v", err)
	}

	terminalRepo := initReviewCLIRepo(t)
	approveDiscoveryMarkdown(t, terminalRepo, "abandon-disabled-terminal", "docs/terminal.md", "terminal\n")
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), terminalRepo, "abandon-disabled-terminal")
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	payload, err = os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	disableReviewForClone(t, terminalRepo)

	const actor = "maintainer@example.com"
	const reason = reviewtransaction.CompactAbandonReasonOperatorDisposition
	authorization := abandonBindingFromInventory(t, terminalRepo, record.State.LineageID, record.Revision, record.State.InitialSnapshot.Identity, actor, reason)
	if err := RunReview([]string{
		"abandon", "--cwd", terminalRepo, "--lineage", record.State.LineageID,
		"--expected-revision", record.Revision, "--reason", reason, "--actor", actor,
		"--maintainer-authorization", authorization,
	}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), `holds terminal "approved" authority`) {
		t.Fatalf("terminal abandon while disabled = %v", err)
	}
	current, err = os.ReadFile(store.StatePath())
	if err != nil || !bytes.Equal(current, payload) {
		t.Fatalf("refused abandon mutated terminal authority: %v", err)
	}
}

// TestReviewStartStaysRefusedWhileKillSwitchDisabled is the companion pin: the
// cleanup relaxation must not leak into progress-shaped verbs. Starting a new
// review while the switch is off stays refused with the typed disabled error.
func TestReviewStartStaysRefusedWhileKillSwitchDisabled(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "candidate.go"), []byte("package candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "candidate.go")
	disableReviewForClone(t, repo)

	var output bytes.Buffer
	err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "review-disabled-start"}, &output)
	if err == nil || !errors.Is(err, reviewtransaction.ErrRDDDisabled) {
		t.Fatalf("review start while disabled = %v\n%s", err, output.String())
	}
	if _, statErr := os.Stat(filepath.Join(reviewCLIAuthorityRoot(t, repo), "v2", "review-disabled-start")); !os.IsNotExist(statErr) {
		t.Fatalf("refused start created review authority: %v", statErr)
	}
}

// TestReviewStartBooleanFlagHelpDoesNotSuggestSeparateValue is the help-text half of
// issue #2528: rendering a boolean flag as `--committed-only <value>` reads as
// though `--committed-only true` were a valid invocation, which the parser then
// refuses. Boolean flags render bare; value-taking flags keep the placeholder.
func TestReviewStartBooleanFlagHelpDoesNotSuggestSeparateValue(t *testing.T) {
	for _, testCase := range []struct{ verb, boolean string }{{verb: "start", boolean: "committed-only"}} {
		var output bytes.Buffer
		if err := RunReview([]string{testCase.verb, "--help"}, &output); err != nil {
			t.Fatalf("review %s --help: %v", testCase.verb, err)
		}
		help := output.String()
		if strings.Contains(help, "--"+testCase.boolean+" <value>") {
			t.Errorf("review %s --help renders boolean --%s with a <value> placeholder:\n%s", testCase.verb, testCase.boolean, help)
		}
		if !strings.Contains(help, "--"+testCase.boolean) {
			t.Errorf("review %s --help no longer documents --%s:\n%s", testCase.verb, testCase.boolean, help)
		}
		if !strings.Contains(help, "--cwd <value>") {
			t.Errorf("review %s --help lost the placeholder on value-taking --cwd:\n%s", testCase.verb, help)
		}
	}
	if err := RunReview([]string{"start", "--committed-only", "true"}, &bytes.Buffer{}); err == nil ||
		!strings.Contains(err.Error(), "not a separate value") {
		t.Fatalf("space-separated boolean value error = %v", err)
	}
}
