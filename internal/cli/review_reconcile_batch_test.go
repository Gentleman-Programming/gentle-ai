package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const batchReconcileCLIActor = "maintainer@example.com"
const batchReconcileCLIReason = "atomically quarantine every declared invalid recovery edge"

func TestReviewReconcileAuthorityBatchPrepareAndApply(t *testing.T) {
	repo, declarations := batchReconcileCLIFixture(t)
	preparation := runBatchReconcileCLIPrepare(t, repo, declarations)
	if preparation.Schema != reviewtransaction.CompactBatchReconcilePreparationSchema ||
		len(preparation.Plan.InvalidEdges) != 2 || preparation.RequiredMaintainerAuthorization == "" {
		t.Fatalf("batch preparation = %#v", preparation)
	}

	request := batchReconcileCLIRequest(preparation)
	input := writeBatchReconcileCLIJSON(t, request)
	var output bytes.Buffer
	if err := RunReview([]string{"reconcile-authority-batch", "--cwd", repo, "--input", input}, &output); err != nil {
		t.Fatalf("review reconcile-authority-batch: %v\n%s", err, output.String())
	}
	var result ReviewReconcileAuthorityBatchResult
	decodeStrictReviewJSON(t, output.Bytes(), &result)
	if result.Schema != ReviewReconcileAuthorityBatchResultSchema || result.Operation != "review/reconcile-authority-batch" ||
		result.Mode != "committed" || result.Journal == nil || result.Journal.Status != reviewtransaction.CompactBatchReconcileCommitted || result.Preparation != nil {
		t.Fatalf("batch reconcile result = %#v", result)
	}
	if _, err := reviewtransaction.CompactAuthorityLeaves(context.Background(), repo); err != nil {
		t.Fatalf("post-batch authority graph: %v", err)
	}

	output.Reset()
	if err := RunReview([]string{"reconcile-authority-batch", "--cwd", repo, "--input", input}, &output); err != nil {
		t.Fatalf("exact CLI replay: %v", err)
	}
	var replay ReviewReconcileAuthorityBatchResult
	decodeStrictReviewJSON(t, output.Bytes(), &replay)
	if replay.Journal == nil || replay.Journal.RequestSHA256 != result.Journal.RequestSHA256 || replay.Journal.ReconciledAt != result.Journal.ReconciledAt {
		t.Fatalf("exact CLI replay = %#v", replay)
	}
}

func TestReviewReconcileAuthorityBatchRefusesInvalidDeclarationAndStalePlanWithoutMutation(t *testing.T) {
	t.Run("incomplete declaration", func(t *testing.T) {
		repo, declarations := batchReconcileCLIFixture(t)
		input := writeBatchReconcileCLIJSON(t, ReviewReconcileAuthorityBatchPreparationInput{
			Schema:               ReviewReconcileAuthorityBatchPreparationInputSchema,
			DeclaredInvalidEdges: declarations[:1], Actor: batchReconcileCLIActor, Reason: batchReconcileCLIReason,
		})
		if err := RunReview([]string{"reconcile-authority-batch", "--prepare", "--cwd", repo, "--input", input}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "declaration set is incomplete") {
			t.Fatalf("incomplete declaration error = %v", err)
		}
		assertBatchReconcileCLISourcesPresent(t, repo, declarations)
	})

	t.Run("stale prepared revision", func(t *testing.T) {
		repo, declarations := batchReconcileCLIFixture(t)
		preparation := runBatchReconcileCLIPrepare(t, repo, declarations)
		lineage := preparation.Plan.QuarantineEntries[0].LineageID
		store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
		if err != nil {
			t.Fatal(err)
		}
		record, err := store.Load()
		if err != nil {
			t.Fatal(err)
		}
		record.State.Recovery.RecoveredAt = record.State.Recovery.RecoveredAt.Add(time.Minute)
		writeReconcileCLIRecord(t, repo, record.State)

		input := writeBatchReconcileCLIJSON(t, batchReconcileCLIRequest(preparation))
		if err := RunReview([]string{"reconcile-authority-batch", "--cwd", repo, "--input", input}, &bytes.Buffer{}); !errors.Is(err, reviewtransaction.ErrConcurrentUpdate) {
			t.Fatalf("stale batch plan error = %v", err)
		}
		assertBatchReconcileCLISourcesPresent(t, repo, declarations)
	})
}

func TestReviewReconcileAuthorityBatchHonorsLockCancellation(t *testing.T) {
	repo, declarations := batchReconcileCLIFixture(t)
	input := writeBatchReconcileCLIJSON(t, ReviewReconcileAuthorityBatchPreparationInput{
		Schema:               ReviewReconcileAuthorityBatchPreparationInputSchema,
		DeclaredInvalidEdges: declarations, Actor: batchReconcileCLIActor, Reason: batchReconcileCLIReason,
	})
	holdCtx, release := context.WithTimeout(context.Background(), 2*time.Second)
	defer release()
	held, err := reviewtransaction.AcquireReviewMaintenanceExclusive(holdCtx, repo)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	var output bytes.Buffer
	if err := runReviewReconcileAuthorityBatch(ctx, []string{"--prepare", "--cwd", repo, "--input", input}, &output); !errors.Is(err, reviewtransaction.ErrAuthorityLockCancelled) || output.Len() != 0 {
		t.Fatalf("batch lock cancellation = %v, output=%q", err, output.String())
	}
}

func TestReviewReconcileAuthorityBatchHelpAndInputContract(t *testing.T) {
	var help bytes.Buffer
	if err := RunReview([]string{"reconcile-authority-batch", "--help"}, &help); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(help.String(), "Usage: gentle-ai review reconcile-authority-batch [flags]") || !strings.Contains(help.String(), "--prepare") {
		t.Fatalf("batch reconcile help:\n%s", help.String())
	}
	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{"missing input", []string{"reconcile-authority-batch"}, "requires --input"},
		{"positional", []string{"reconcile-authority-batch", "extra", "--input", "ignored"}, `unexpected review reconcile-authority-batch argument "extra"`},
		{"unknown flag", []string{"reconcile-authority-batch", "--unknown"}, "flag provided but not defined"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := RunReview(tt.args, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("input error = %v, want %q", err, tt.want)
			}
		})
	}
}

func runBatchReconcileCLIPrepare(t *testing.T, repo string, declarations []reviewtransaction.CompactRecoveryEdgeInspection) reviewtransaction.CompactBatchReconcilePreparation {
	t.Helper()
	input := writeBatchReconcileCLIJSON(t, ReviewReconcileAuthorityBatchPreparationInput{
		Schema:               ReviewReconcileAuthorityBatchPreparationInputSchema,
		DeclaredInvalidEdges: declarations, Actor: batchReconcileCLIActor, Reason: batchReconcileCLIReason,
	})
	var output bytes.Buffer
	if err := RunReview([]string{"reconcile-authority-batch", "--prepare", "--cwd", repo, "--input", input}, &output); err != nil {
		t.Fatalf("prepare review reconcile-authority-batch: %v\n%s", err, output.String())
	}
	var result ReviewReconcileAuthorityBatchResult
	decodeStrictReviewJSON(t, output.Bytes(), &result)
	if result.Mode != "prepared" || result.Preparation == nil || result.Journal != nil {
		t.Fatalf("batch preparation result = %#v", result)
	}
	return *result.Preparation
}

func batchReconcileCLIRequest(preparation reviewtransaction.CompactBatchReconcilePreparation) reviewtransaction.CompactBatchReconcileRequest {
	return reviewtransaction.CompactBatchReconcileRequest{
		Schema:               reviewtransaction.CompactBatchReconcileRequestSchema,
		DeclaredInvalidEdges: preparation.DeclaredInvalidEdges,
		ExpectedPlan:         preparation.Plan, ExpectedPlanSHA256: preparation.PlanSHA256,
		Actor: batchReconcileCLIActor, Reason: batchReconcileCLIReason,
		MaintainerAuthorization: preparation.RequiredMaintainerAuthorization,
	}
}

func batchReconcileCLIFixture(t *testing.T) (string, []reviewtransaction.CompactRecoveryEdgeInspection) {
	t.Helper()
	repo := initReviewCLIRepo(t)
	batchReconcileCLIInvalidPair(t, repo, "batch-a")
	batchReconcileCLIInvalidPair(t, repo, "batch-b")
	report, err := reviewtransaction.InspectCompactRecoveryEdges(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	declarations := []reviewtransaction.CompactRecoveryEdgeInspection{}
	for _, edge := range report.Edges {
		if !edge.Valid {
			declarations = append(declarations, edge)
		}
	}
	if len(declarations) != 2 {
		t.Fatalf("batch CLI invalid edges = %#v", report)
	}
	return repo, declarations
}

func batchReconcileCLIInvalidPair(t *testing.T, repo, prefix string) {
	t.Helper()
	logicalPath := "docs/" + prefix + ".md"
	path := filepath.Join(repo, filepath.FromSlash(logicalPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(prefix+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	snapshot, err := builder.Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, IntendedUntracked: []string{logicalPath},
	})
	if err != nil {
		t.Fatal(err)
	}
	risk, lines, err := builder.ClassifySnapshotRisk(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	policy := sha256.Sum256([]byte("batch-reconcile-cli-" + prefix))
	policyHash := "sha256:" + hex.EncodeToString(policy[:])
	predecessor, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: prefix + "-predecessor", Mode: reviewtransaction.ModeOrdinaryBounded, Generation: 1,
		Snapshot: snapshot, PolicyHash: policyHash, RiskLevel: risk, SelectedLenses: []string{}, OriginalChangedLines: &lines,
	})
	if err != nil {
		t.Fatal(err)
	}
	predecessor.State = reviewtransaction.StateEscalated
	predecessorRevision := writeReconcileCLIRecord(t, repo, predecessor)
	successor, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: prefix + "-successor", Mode: reviewtransaction.ModeOrdinaryBounded, Generation: 2,
		Snapshot: snapshot, PolicyHash: policyHash, RiskLevel: risk, SelectedLenses: []string{}, OriginalChangedLines: &lines,
	})
	if err != nil {
		t.Fatal(err)
	}
	successor.Recovery = &reviewtransaction.CompactRecoveryProvenance{
		PredecessorLineageID: predecessor.LineageID, PredecessorRevision: predecessorRevision,
		Disposition: reviewtransaction.RecoveryEscalated, Reason: "retry batch incident", Actor: batchReconcileCLIActor,
		RecoveredAt:             time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC),
		MaintainerAuthorization: "historical maintainer approval",
	}
	writeReconcileCLIRecord(t, repo, successor)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}

func writeBatchReconcileCLIJSON(t *testing.T, value any) string {
	t.Helper()
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "batch-reconcile.json")
	if err := os.WriteFile(path, append(payload, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertBatchReconcileCLISourcesPresent(t *testing.T, repo string, declarations []reviewtransaction.CompactRecoveryEdgeInspection) {
	t.Helper()
	for _, edge := range declarations {
		store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, edge.SuccessorLineageID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.Load(); err != nil {
			t.Fatalf("source %q mutated after refusal: %v", edge.SuccessorLineageID, err)
		}
	}
}
