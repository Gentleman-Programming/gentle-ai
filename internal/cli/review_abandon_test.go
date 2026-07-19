package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
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

// TestPristineInvalidatedLineagePoisonsLineagelessGateDiscovery reproduces
// the #1441 discovery half on the CLI surface: one pristine invalidated
// compact entry denies lineage-less gate validation for an unrelated approved
// lineage whose receipt exactly matches the live target.
func TestPristineInvalidatedLineagePoisonsLineagelessGateDiscovery(t *testing.T) {
	repo := initReviewCLIRepo(t)
	approveDiscoveryMarkdown(t, repo, "review-abandon-valid", "docs/valid.md", "valid\n")
	pristineInvalidatedCLIFixture(t, repo)

	var statusOutput bytes.Buffer
	if err := RunReview([]string{"status", "--cwd", repo}, &statusOutput); err != nil {
		t.Fatalf("review status over poisoned inventory: %v", err)
	}
	var report reviewtransaction.AuthorityStatusReport
	decodeStrictReviewJSON(t, statusOutput.Bytes(), &report)
	if report.Complete || report.Authoritative {
		t.Fatalf("poisoned status = complete %v authoritative %v", report.Complete, report.Authoritative)
	}

	var blockedOutput bytes.Buffer
	err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePostApply),
	}, &blockedOutput)
	if err == nil {
		t.Fatal("pristine invalidated entry did not deny lineage-less gate validation")
	}
	failure := decodeReviewIntegrationFailure(t, blockedOutput.Bytes())
	if failure.Code != "authority_corrupted" {
		t.Fatalf("poisoned gate failure = %#v", failure)
	}
}

func abandonCLIArgs(repo, revision, authorization string) []string {
	return []string{
		"abandon", "--cwd", repo,
		"--lineage", "abandon-accidental", "--expected-revision", revision,
		"--reason", "retire accidental pristine lineage", "--actor", "maintainer@example.com",
		"--maintainer-authorization", authorization,
	}
}

func abandonCLIBinding(revision, snapshotIdentity string) string {
	return "gentle-ai.review-abandon-authorization/v1\nlineage=abandon-accidental\nrevision=" + revision +
		"\nsnapshot_identity=" + snapshotIdentity +
		"\nactor=maintainer@example.com\nreason=retire accidental pristine lineage"
}

func TestReviewAbandonQuarantinesPristineLineageAndRestoresLifecycle(t *testing.T) {
	repo := initReviewCLIRepo(t)
	approveDiscoveryMarkdown(t, repo, "review-abandon-valid", "docs/valid.md", "valid\n")
	revision, snapshotIdentity := pristineInvalidatedCLIFixture(t, repo)

	var blockedOutput bytes.Buffer
	err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePostApply),
	}, &blockedOutput)
	if err == nil {
		t.Fatal("pristine invalidated entry did not deny lineage-less gate validation")
	}
	if failure := decodeReviewIntegrationFailure(t, blockedOutput.Bytes()); failure.Code != "authority_corrupted" {
		t.Fatalf("pre-abandon gate failure = %#v", failure)
	}

	var abandonOutput bytes.Buffer
	if err := RunReview(abandonCLIArgs(repo, revision, abandonCLIBinding(revision, snapshotIdentity)), &abandonOutput); err != nil {
		t.Fatalf("review abandon: %v\n%s", err, abandonOutput.String())
	}
	var result ReviewAbandonResult
	decodeStrictReviewJSON(t, abandonOutput.Bytes(), &result)
	if result.Operation != "review/abandon" || result.Record.LineageID != "abandon-accidental" ||
		result.Record.Status != reviewtransaction.CompactReclaimCommitted ||
		result.Record.PristineAbandonment == nil ||
		result.Record.PristineAbandonment.State != reviewtransaction.StateInvalidated ||
		result.Record.PristineAbandonment.InvalidationReason != "accidental empty lineage" ||
		result.Record.PristineAbandonment.Revision != revision ||
		result.Record.PristineAbandonment.SnapshotIdentity != snapshotIdentity {
		t.Fatalf("review abandon result = %#v", result)
	}
	if _, err := os.Stat(filepath.Join(result.Record.QuarantinePath, "residue", "review-state.json")); err != nil {
		t.Fatalf("quarantined pristine state missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(reviewCLIAuthorityRoot(t, repo), "v2", "abandon-accidental")); !os.IsNotExist(err) {
		t.Fatalf("abandoned entry still present: %v", err)
	}

	var statusOutput bytes.Buffer
	if err := RunReview([]string{"status", "--cwd", repo}, &statusOutput); err != nil {
		t.Fatalf("review status after abandon: %v", err)
	}
	var after reviewtransaction.AuthorityStatusReport
	decodeStrictReviewJSON(t, statusOutput.Bytes(), &after)
	if !after.Complete || !after.Authoritative {
		t.Fatalf("post-abandon status = complete %v authoritative %v", after.Complete, after.Authoritative)
	}

	var validateOutput bytes.Buffer
	if err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePostApply),
	}, &validateOutput); err != nil {
		t.Fatalf("post-abandon lineage-less validation: %v\n%s", err, validateOutput.String())
	}
	var validated ReviewValidateResult
	decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, validateOutput.Bytes()).Result, &validated)
	if !validated.Allowed || validated.Context.LineageID != "review-abandon-valid" {
		t.Fatalf("post-abandon validation = %#v", validated)
	}

	var replayOutput bytes.Buffer
	if err := RunReview(abandonCLIArgs(repo, revision, abandonCLIBinding(revision, snapshotIdentity)), &replayOutput); err != nil {
		t.Fatalf("idempotent review abandon replay: %v\n%s", err, replayOutput.String())
	}
	var replayed ReviewAbandonResult
	decodeStrictReviewJSON(t, replayOutput.Bytes(), &replayed)
	if replayed.Record.Status != reviewtransaction.CompactReclaimCommitted ||
		replayed.Record.QuarantinePath != result.Record.QuarantinePath {
		t.Fatalf("replayed review abandon result = %#v", replayed)
	}
}

func TestReviewAbandonRequiresFlagsAndExactBinding(t *testing.T) {
	repo := initReviewCLIRepo(t)
	revision, snapshotIdentity := pristineInvalidatedCLIFixture(t, repo)
	statePath := filepath.Join(reviewCLIAuthorityRoot(t, repo), "v2", "abandon-accidental", "review-state.json")
	payload, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}

	if err := RunReview([]string{
		"abandon", "--cwd", repo, "--lineage", "abandon-accidental",
	}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "requires") {
		t.Fatalf("incomplete abandon flags error = %v", err)
	}

	wrong := abandonCLIBinding(snapshotIdentity, revision)
	if err := RunReview(abandonCLIArgs(repo, revision, wrong), &bytes.Buffer{}); err == nil ||
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
	if err := RunReview([]string{"help"}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "abandon") {
		t.Fatalf("review help does not list abandon: %s", output.String())
	}
}
