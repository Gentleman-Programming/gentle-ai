package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

// pristineInvalidatedCLIFixture persists one pristine invalidated docs-only
// compact lineage directly into the v2 store, then restores the clean
// workspace so the lineage is also stale relative to the live worktree.
func pristineInvalidatedCLIFixture(t *testing.T, repo string) (revision, snapshotIdentity string) {
	t.Helper()
	accidental := filepath.Join(repo, "docs", "accidental.md")
	if err := os.MkdirAll(filepath.Dir(accidental), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(accidental, []byte("accidental\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	snapshot, err := builder.Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, IntendedUntracked: []string{"docs/accidental.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	risk, lines, err := builder.ClassifySnapshotRisk(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	state, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: "abandon-accidental", Mode: reviewtransaction.ModeOrdinaryBounded, Generation: 1,
		Snapshot: snapshot, PolicyHash: "sha256:" + strings.Repeat("ab", 32), RiskLevel: risk,
		SelectedLenses: []string{}, OriginalChangedLines: &lines,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := state.Invalidate("accidental empty lineage"); err != nil {
		t.Fatal(err)
	}
	revision = writeReconcileCLIRecord(t, repo, state)
	if err := os.Remove(accidental); err != nil {
		t.Fatal(err)
	}
	return revision, state.InitialSnapshot.Identity
}

func abandonCLIArgs(repo, revision, authorization string) []string {
	return []string{
		"abandon", "--cwd", repo,
		"--lineage", "abandon-accidental", "--expected-revision", revision,
		"--reason", reviewtransaction.CompactAbandonReasonOperatorDisposition, "--actor", "maintainer@example.com",
		"--maintainer-authorization", authorization,
	}
}

func abandonCLIBinding(t *testing.T, repo, revision, snapshotIdentity string) string {
	return abandonBindingFromInventory(t, repo, "abandon-accidental", revision, snapshotIdentity,
		"maintainer@example.com", reviewtransaction.CompactAbandonReasonOperatorDisposition)
}

// TestReviewAbandonQuarantinesTerminalInvalidatedLineageOnlyWithBinding: a
// terminal lineage is abandonable, but only with the exact binding; a wrong one
// refuses and leaves the store untouched, and the success JSON keeps exactly
// the keys gentle-shell decodes.
func TestReviewAbandonQuarantinesTerminalInvalidatedLineageOnlyWithBinding(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	revision, snapshotIdentity := pristineInvalidatedCLIFixture(t, repo)
	statePath := filepath.Join(reviewCLIAuthorityRoot(t, repo), "v2", "abandon-accidental", "review-state.json")
	payload, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}

	var refused bytes.Buffer
	err = RunReview(abandonCLIArgs(repo, revision, abandonCLIBinding(t, repo, snapshotIdentity, revision)), &refused)
	if err == nil || !strings.Contains(err.Error(), "exact maintainer authorization binding") || refused.Len() != 0 {
		t.Fatalf("review abandon with a wrong binding = %v, stdout %q", err, refused.String())
	}
	current, err := os.ReadFile(statePath)
	if err != nil || !bytes.Equal(current, payload) {
		t.Fatalf("refused abandon mutated authority: %v", err)
	}

	var stdout bytes.Buffer
	if err := RunReview(abandonCLIArgs(repo, revision, abandonCLIBinding(t, repo, revision, snapshotIdentity)), &stdout); err != nil {
		t.Fatalf("review abandon terminal invalidated with the exact binding: %v", err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded) != 2 || decoded["operation"] == nil || decoded["record"] == nil {
		t.Fatalf("abandon JSON keys = %s, want exactly operation and record", stdout.String())
	}
	if _, statErr := os.Stat(statePath); !os.IsNotExist(statErr) {
		t.Fatalf("abandoned entry still present: %v", statErr)
	}
}

func TestReviewAbandonRequiresFlagsAndExactBinding(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	revision, snapshotIdentity := pristineReviewingCLIFixture(t, repo)
	statePath := filepath.Join(reviewCLIAuthorityRoot(t, repo), "v2", "abandon-stale-reviewing", "review-state.json")
	payload, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}

	if err := RunReview([]string{
		"abandon", "--cwd", repo, "--lineage", "abandon-stale-reviewing",
	}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "requires") {
		t.Fatalf("incomplete abandon flags error = %v", err)
	}

	wrong := staleReviewingAbandonBinding(t, repo, snapshotIdentity, revision)
	if err := RunReview(staleReviewingAbandonArgs(repo, revision, wrong), &bytes.Buffer{}); err == nil ||
		!strings.Contains(err.Error(), "exact maintainer authorization binding") {
		t.Fatalf("inexact abandon binding error = %v", err)
	}
	current, err := os.ReadFile(statePath)
	if err != nil || !bytes.Equal(current, payload) {
		t.Fatalf("refused abandon mutated the entry: %v", err)
	}
}

func TestReviewHelpListsAbandon(t *testing.T) {
	var output bytes.Buffer
	if err := RunReview([]string{"abandon", "--help"}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Accepts every state") || !strings.Contains(output.String(), "authorization binding") {
		t.Fatalf("review abandon help does not describe the widened coverage and the binding: %s", output.String())
	}
}
