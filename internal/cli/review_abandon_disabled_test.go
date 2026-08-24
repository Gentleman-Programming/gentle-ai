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

// pristineReviewingCLIFixture starts one reviewing compact lineage through the
// atomic START boundary, then restores the clean workspace so the lineage is
// stale relative to the live worktree. It is the reviewing sibling of
// pristineInvalidatedCLIFixture: the exact shape issue #2528 reports locked in,
// a review that was started, never captured a lens result, and went stale.
func pristineReviewingCLIFixture(t *testing.T, repo string) (revision, snapshotIdentity string) {
	t.Helper()
	stale := filepath.Join(repo, "stale.go")
	if err := os.WriteFile(stale, []byte("package stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "stale.go")
	runReviewCLIGit(t, repo, "commit", "-qm", "stale baseline")
	if err := os.WriteFile(stale, []byte("package stale\n\nvar Value = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := atomicStartV2(t, repo, "abandon-stale-reviewing")
	store := atomicCompactStartStore(t, repo, started.LineageID)
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(stale); err != nil {
		t.Fatal(err)
	}
	return record.Revision, record.State.InitialSnapshot.Identity
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
	reviewEnabledHome(t)
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
	reviewEnabledHome(t)
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
// remains refused with the switch off.
func TestReviewAbandonWhileDisabledKeepsEligibilityAndRevisionChecks(t *testing.T) {
	reviewEnabledHome(t)
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
	before := snapshotAuthorityTree(t, reviewCLIAuthorityRoot(t, repo))

	var output bytes.Buffer
	err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "review-disabled-start"}, &output)
	if err == nil || !errors.Is(err, reviewtransaction.ErrRDDDisabled) {
		t.Fatalf("review start while disabled = %v\n%s", err, output.String())
	}
	after := snapshotAuthorityTree(t, reviewCLIAuthorityRoot(t, repo))
	if before != after {
		t.Fatalf("refused start mutated review authority:\nbefore:\n%s\nafter:\n%s", before, after)
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

// breakForeignCompactSnapshotIdentity rewrites one persisted compact record's
// frozen snapshot identity so the record no longer loads. Validate() runs
// before the checksum comparison in parseCompactRecord, so the load failure is
// the same typed semantic "compact snapshot identity does not match its
// metadata" a released binary reports for a record it can no longer read --
// the exact shape issue #3124 and its occurrences describe.
func breakForeignCompactSnapshotIdentity(t *testing.T, repo, lineage string) {
	t.Helper()
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	statePath := store.StatePath()
	payload, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("fixture lineage %q does not load before it is broken: %v", lineage, err)
	}
	retired := "sha256:" + strings.Repeat("c", 64)
	broken := strings.ReplaceAll(string(payload), loaded.State.InitialSnapshot.Identity, retired)
	if broken == string(payload) {
		t.Fatalf("fixture lineage %q carries no snapshot identity to retire", lineage)
	}
	if err := os.WriteFile(statePath, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, loadErr := store.Load(); loadErr == nil {
		t.Fatalf("fixture lineage %q still loads after its snapshot identity was retired", lineage)
	} else if !strings.Contains(loadErr.Error(), "identity does not match its metadata") {
		t.Fatalf("fixture lineage %q load error = %v, want a snapshot identity mismatch", lineage, loadErr)
	}
}

// TestReviewAbandonOfAPristineLineageIgnoresAnUnrelatedUnloadableLineage is
// issue #3124 driven through the live `gentle-ai review abandon` command. An
// unreadable foreign compact entry must not refuse the sanctioned exit for a
// pristine reviewing lineage.
func TestReviewAbandonOfAPristineLineageIgnoresAnUnrelatedUnloadableLineage(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "foreign.go", "package foreign\n", 0o644)
	foreignStarted := atomicStartV2(t, repo, "abandon-foreign-active")
	runReviewCLIGit(t, repo, "commit", "-qm", "foreign active candidate")
	breakForeignCompactSnapshotIdentity(t, repo, foreignStarted.LineageID)

	revision, snapshotIdentity := pristineReviewingCLIFixture(t, repo)
	binding := staleReviewingAbandonBinding(t, repo, revision, snapshotIdentity)

	var output bytes.Buffer
	if err := RunReview(staleReviewingAbandonArgs(repo, revision, binding), &output); err != nil {
		t.Fatalf("an unrelated unloadable lineage refused the sanctioned exit of a pristine one: %v\n%s", err, output.String())
	}
	var result ReviewAbandonResult
	decodeStrictReviewJSON(t, output.Bytes(), &result)
	if result.Record.Status != reviewtransaction.CompactReclaimCommitted || result.Record.LineageID != "abandon-stale-reviewing" {
		t.Fatalf("abandonment record = %#v", result.Record)
	}

	// Scoping never widens authority: the unrelated entry is untouched and
	// still unreadable, so nothing was silently repaired to make this pass.
	foreign, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, foreignStarted.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if _, loadErr := foreign.Load(); loadErr == nil {
		t.Fatal("the unrelated lineage became readable, so this test proved nothing about scoping")
	}
}
