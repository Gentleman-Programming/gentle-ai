package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
)

func reviewCLIAuthorityRoot(t *testing.T, repo string) string {
	t.Helper()
	commonDir := filepath.Clean(strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "--path-format=absolute", "--git-common-dir")))
	return filepath.Join(commonDir, "gentle-ai", "review-transactions")
}

func writeReconcileCLIRecord(t *testing.T, repo string, state reviewtransaction.CompactState) string {
	t.Helper()
	revision, err := reviewtransaction.CompactRevisionForState(state)
	if err != nil {
		t.Fatal(err)
	}
	record := reviewtransaction.CompactRecord{Schema: "gentle-ai.review-state-record/v2", Revision: revision, State: state}
	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(reviewCLIAuthorityRoot(t, repo), "v2", state.LineageID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "review-state.json"), append(payload, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	return revision
}

// invalidRecoveryEdgeCLIFixture persists an escalated docs-only predecessor
// with its receipt and a recovery successor whose sole anomaly is an unchanged
// target, then restores the clean workspace.
func invalidRecoveryEdgeCLIFixture(t *testing.T, repo string) (predecessorRevision, successorRevision string) {
	t.Helper()
	incident := filepath.Join(repo, "docs", "incident.md")
	if err := os.MkdirAll(filepath.Dir(incident), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(incident, []byte("incident\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	snapshot, err := builder.Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, IntendedUntracked: []string{"docs/incident.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	risk, lines, err := builder.ClassifySnapshotRisk(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if risk != reviewtransaction.RiskLow {
		t.Fatalf("reconcile fixture risk = %q", risk)
	}
	policy := sha256.Sum256([]byte("reconcile-policy"))
	policyHash := "sha256:" + hex.EncodeToString(policy[:])
	predecessorState, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: "reconcile-incident", Mode: reviewtransaction.ModeOrdinaryBounded, Generation: 1,
		Snapshot: snapshot, PolicyHash: policyHash, RiskLevel: risk, SelectedLenses: []string{}, OriginalChangedLines: &lines,
	})
	if err != nil {
		t.Fatal(err)
	}
	predecessorState.State = reviewtransaction.StateEscalated
	predecessorRevision = writeReconcileCLIRecord(t, repo, predecessorState)
	receipt, err := predecessorState.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	receiptPath := filepath.Join(reviewCLIAuthorityRoot(t, repo), "v2", predecessorState.LineageID, "review-receipt.json")
	if err := reviewtransaction.WriteCompactReceiptAtomic(receiptPath, receipt); err != nil {
		t.Fatal(err)
	}
	successorState, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: "reconcile-incident-g2", Mode: reviewtransaction.ModeOrdinaryBounded, Generation: 2,
		Snapshot: snapshot, PolicyHash: policyHash, RiskLevel: risk, SelectedLenses: []string{}, OriginalChangedLines: &lines,
	})
	if err != nil {
		t.Fatal(err)
	}
	successorState.Recovery = &reviewtransaction.CompactRecoveryProvenance{
		PredecessorLineageID: predecessorState.LineageID, PredecessorRevision: predecessorRevision,
		Disposition: reviewtransaction.RecoveryEscalated, Reason: "retry after escalation", Actor: "maintainer@example.com",
		RecoveredAt: time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC),
		MaintainerAuthorization: "gentle-ai.review-recovery-authorization/v1\npredecessor_lineage=" + predecessorState.LineageID +
			"\npredecessor_revision=" + predecessorRevision + "\ntarget_identity=" + successorState.InitialSnapshot.Identity +
			"\nactor=maintainer@example.com\nreason=retry after escalation",
	}
	successorRevision = writeReconcileCLIRecord(t, repo, successorState)
	if err := os.Remove(incident); err != nil {
		t.Fatal(err)
	}
	return predecessorRevision, successorRevision
}

func reconcileCLIArgs(repo, predecessorRevision, successorRevision, authorization string) []string {
	return []string{
		"reconcile-authority", "--cwd", repo,
		"--predecessor-lineage", "reconcile-incident", "--expected-predecessor-revision", predecessorRevision,
		"--successor-lineage", "reconcile-incident-g2", "--expected-successor-revision", successorRevision,
		"--reason", "quarantine invalid unchanged-target recovery edge", "--actor", "maintainer@example.com",
		"--maintainer-authorization", authorization,
	}
}

func reconcileCLIBinding(predecessorRevision, successorRevision string) string {
	return "gentle-ai.review-reconcile-authorization/v1\npredecessor_lineage=reconcile-incident\npredecessor_revision=" + predecessorRevision +
		"\nsuccessor_lineage=reconcile-incident-g2\nsuccessor_revision=" + successorRevision +
		"\nactor=maintainer@example.com\nreason=quarantine invalid unchanged-target recovery edge"
}

func TestReviewReconcileAuthorityQuarantinesInvalidEdgeAndRestoresDiscovery(t *testing.T) {
	repo := initReviewCLIRepo(t)
	approveDiscoveryMarkdown(t, repo, "review-reconcile-valid", "docs/valid.md", "valid\n")
	predecessorRevision, successorRevision := invalidRecoveryEdgeCLIFixture(t, repo)

	var statusOutput bytes.Buffer
	if err := RunReview([]string{"status", "--cwd", repo}, &statusOutput); err != nil {
		t.Fatalf("review status over poisoned graph: %v", err)
	}
	var before reviewtransaction.AuthorityStatusReport
	decodeStrictReviewJSON(t, statusOutput.Bytes(), &before)
	if before.Complete || before.Authoritative {
		t.Fatalf("poisoned status = complete %v authoritative %v", before.Complete, before.Authoritative)
	}

	var blockedOutput bytes.Buffer
	err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePostApply),
	}, &blockedOutput)
	if err == nil {
		t.Fatal("invalid recovery edge did not deny lineage-less gate validation")
	}
	failure := decodeReviewIntegrationFailure(t, blockedOutput.Bytes())
	if failure.Code != "authority_corrupted" {
		t.Fatalf("poisoned gate failure = %#v", failure)
	}

	var reconcileOutput bytes.Buffer
	if err := RunReview(reconcileCLIArgs(repo, predecessorRevision, successorRevision, reconcileCLIBinding(predecessorRevision, successorRevision)), &reconcileOutput); err != nil {
		t.Fatalf("review reconcile-authority: %v\n%s", err, reconcileOutput.String())
	}
	var result ReviewReconcileAuthorityResult
	decodeStrictReviewJSON(t, reconcileOutput.Bytes(), &result)
	if result.Operation != "review/reconcile-authority" || result.Record.LineageID != "reconcile-incident-g2" ||
		result.Record.Status != reviewtransaction.CompactReclaimCommitted || result.Record.InvalidRecoveryEdge == nil ||
		!strings.Contains(result.Record.InvalidRecoveryEdge.ValidationError, "target has not changed") {
		t.Fatalf("review reconcile-authority result = %#v", result)
	}
	if _, err := os.Stat(filepath.Join(result.Record.QuarantinePath, "residue", "review-state.json")); err != nil {
		t.Fatalf("quarantined successor state missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(reviewCLIAuthorityRoot(t, repo), "v2", "reconcile-incident-g2")); !os.IsNotExist(err) {
		t.Fatalf("reconciled successor entry still present: %v", err)
	}

	statusOutput.Reset()
	if err := RunReview([]string{"status", "--cwd", repo}, &statusOutput); err != nil {
		t.Fatalf("review status after reconcile: %v", err)
	}
	var after reviewtransaction.AuthorityStatusReport
	decodeStrictReviewJSON(t, statusOutput.Bytes(), &after)
	if !after.Complete || !after.Authoritative {
		t.Fatalf("post-reconcile status = complete %v authoritative %v", after.Complete, after.Authoritative)
	}

	var validateOutput bytes.Buffer
	if err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePostApply),
	}, &validateOutput); err != nil {
		t.Fatalf("post-reconcile lineage-less validation: %v\n%s", err, validateOutput.String())
	}
	var validated ReviewValidateResult
	decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, validateOutput.Bytes()).Result, &validated)
	if !validated.Allowed || validated.Context.LineageID != "review-reconcile-valid" {
		t.Fatalf("post-reconcile validation = %#v", validated)
	}

	if err := RunReview(reconcileCLIArgs(repo, predecessorRevision, successorRevision, reconcileCLIBinding(predecessorRevision, successorRevision)), &bytes.Buffer{}); err == nil {
		t.Fatal("review reconcile-authority replayed after quarantine")
	}
}

func TestReviewReconcileAuthorityRequiresFlagsAndExactBinding(t *testing.T) {
	repo := initReviewCLIRepo(t)
	predecessorRevision, successorRevision := invalidRecoveryEdgeCLIFixture(t, repo)
	statePath := filepath.Join(reviewCLIAuthorityRoot(t, repo), "v2", "reconcile-incident-g2", "review-state.json")
	payload, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}

	if err := RunReview([]string{
		"reconcile-authority", "--cwd", repo, "--successor-lineage", "reconcile-incident-g2",
	}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "requires") {
		t.Fatalf("incomplete reconcile-authority flags error = %v", err)
	}

	wrong := reconcileCLIBinding(successorRevision, predecessorRevision)
	if err := RunReview(reconcileCLIArgs(repo, predecessorRevision, successorRevision, wrong), &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "exact maintainer authorization binding") {
		t.Fatalf("inexact reconcile-authority binding error = %v", err)
	}
	current, err := os.ReadFile(statePath)
	if err != nil || !bytes.Equal(current, payload) {
		t.Fatalf("refused reconcile-authority mutated the successor entry: %v", err)
	}
}
