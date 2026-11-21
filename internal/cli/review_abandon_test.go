package cli

import (
	"bytes"
	"context"
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

func abandonCLIArgs(repo string) []string {
	return []string{
		"abandon", "--cwd", repo,
		"--lineage", "abandon-accidental",
		"--reason", reviewtransaction.CompactAbandonReasonOperatorDisposition, "--actor", "maintainer@example.com",
	}
}

func TestReviewAbandonQuarantinesTerminalInvalidatedLineage(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	pristineInvalidatedCLIFixture(t, repo)
	statePath := filepath.Join(reviewCLIAuthorityRoot(t, repo), "v2", "abandon-accidental", "review-state.json")
	payload, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := RunReview(abandonCLIArgs(repo), &output); err != nil {
		t.Fatalf("review abandon terminal invalidated: %v\n%s", err, output.String())
	}
	var result ReviewAbandonResult
	decodeStrictReviewJSON(t, output.Bytes(), &result)
	if result.Record.Status != reviewtransaction.CompactReclaimCommitted || len(result.Cascaded) != 0 {
		t.Fatalf("terminal invalidated abandonment = %#v", result)
	}
	moved, err := os.ReadFile(filepath.Join(result.Record.QuarantinePath, "residue", "review-state.json"))
	if err != nil || !bytes.Equal(moved, payload) {
		t.Fatalf("quarantined bytes differ: %v", err)
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("abandoned entry still present: %v", err)
	}
}

// TestReviewAbandonNeedsOnlyALineageAndDefaultsTheAuditFields: no revision, no
// authorization binding, and neither --reason nor --actor are required.
func TestReviewAbandonNeedsOnlyALineageAndDefaultsTheAuditFields(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	pristineReviewingCLIFixture(t, repo)

	var output bytes.Buffer
	if err := RunReview([]string{"abandon", "--cwd", repo, "--lineage", "abandon-stale-reviewing"}, &output); err != nil {
		t.Fatalf("review abandon with only a lineage: %v\n%s", err, output.String())
	}
	var result ReviewAbandonResult
	decodeStrictReviewJSON(t, output.Bytes(), &result)
	if result.Record.Reason != reviewtransaction.CompactAbandonReasonOperatorDisposition || strings.TrimSpace(result.Record.Actor) == "" ||
		result.Record.Status != reviewtransaction.CompactReclaimCommitted {
		t.Fatalf("defaulted audit fields = %#v", result.Record)
	}
	for _, retired := range []string{"--expected-revision", "--maintainer-authorization"} {
		err := RunReview([]string{"abandon", "--cwd", repo, "--lineage", "abandon-stale-reviewing", retired, "x"}, &bytes.Buffer{})
		if err == nil {
			t.Fatalf("retired flag %s is still accepted", retired)
		}
	}
}

// TestReviewAbandonWithoutLineageNamesTheLineagesAndWritesNothing is the form
// the negotiated STATUS continuation carries.
func TestReviewAbandonWithoutLineageNamesTheLineagesAndWritesNothing(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	pristineReviewingCLIFixture(t, repo)
	statePath := filepath.Join(reviewCLIAuthorityRoot(t, repo), "v2", "abandon-stale-reviewing", "review-state.json")
	payload, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	err = RunReview([]string{"abandon", "--cwd", repo}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires --lineage") || !strings.Contains(err.Error(), "--lineage abandon-stale-reviewing") {
		t.Fatalf("flagless abandon error = %v", err)
	}
	current, err := os.ReadFile(statePath)
	if err != nil || !bytes.Equal(current, payload) {
		t.Fatalf("flagless abandon mutated the entry: %v", err)
	}
}

func TestReviewHelpListsAbandon(t *testing.T) {
	var output bytes.Buffer
	if err := RunReview([]string{"abandon", "--help"}, &output); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"terminal", "successors", "lock-busy"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("review abandon help does not describe %q: %s", want, output.String())
		}
	}
}
