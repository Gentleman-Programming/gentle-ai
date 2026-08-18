package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
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

// TestPristineInvalidatedLineageDoesNotPoisonLineagelessGateDiscovery proves
// invalidated authority remains auditable without blocking an unrelated exact
// approved receipt. Explicit selection of the invalidated lineage still fails.
func TestPristineInvalidatedLineageDoesNotPoisonLineagelessGateDiscovery(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	approveDiscoveryMarkdown(t, repo, "review-abandon-valid", "docs/valid.md", "valid\n")
	pristineInvalidatedCLIFixture(t, repo)

	var statusOutput bytes.Buffer
	if err := RunReview([]string{"status", "--cwd", repo}, &statusOutput); err != nil {
		t.Fatalf("review status over poisoned inventory: %v", err)
	}
	var report reviewtransaction.AuthorityStatusReport
	decodeStrictReviewJSON(t, statusOutput.Bytes(), &report)
	if !report.Complete || !report.Authoritative {
		t.Fatalf("invalidated status = complete %v authoritative %v", report.Complete, report.Authoritative)
	}

	var allowedOutput bytes.Buffer
	err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePostApply),
	}, &allowedOutput)
	if err != nil {
		t.Fatalf("unrelated invalidated lineage blocked exact approved receipt: %v\n%s", err, allowedOutput.String())
	}
	var validated ReviewValidateResult
	decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, allowedOutput.Bytes()).Result, &validated)
	if !validated.Allowed || validated.Context.LineageID != "review-abandon-valid" {
		t.Fatalf("lineage-less validation = %#v", validated)
	}
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

func TestReviewAbandonRefusesTerminalInvalidatedLineage(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	approveDiscoveryMarkdown(t, repo, "review-abandon-valid", "docs/valid.md", "valid\n")
	revision, snapshotIdentity := pristineInvalidatedCLIFixture(t, repo)
	statePath := filepath.Join(reviewCLIAuthorityRoot(t, repo), "v2", "abandon-accidental", "review-state.json")
	payload, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}

	err = RunReview(abandonCLIArgs(repo, revision, abandonCLIBinding(t, repo, revision, snapshotIdentity)), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), `holds terminal "invalidated" authority`) {
		t.Fatalf("review abandon terminal invalidated = %v", err)
	}
	current, err := os.ReadFile(statePath)
	if err != nil || !bytes.Equal(current, payload) {
		t.Fatalf("terminal invalidated abandon mutated authority: %v", err)
	}
}

func TestReviewAbandonRequiresFlagsAndExactBinding(t *testing.T) {
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
	if !strings.Contains(output.String(), "non-terminal") {
		t.Fatalf("review abandon help does not describe non-terminal lineages: %s", output.String())
	}
}
