package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
)

func TestOrdinaryMarkdownLowRiskLifecycleNeedsNoExternalEvidence(t *testing.T) {
	repo := initReviewCLIRepo(t)
	lines := make([]string, 129)
	for index := range lines {
		lines[index] = fmt.Sprintf("ordinary documentation line %03d", index+1)
	}
	path := filepath.Join(repo, "docs", "ordinary-guide.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	startedAt := time.Now()
	var startOutput bytes.Buffer
	if err := RunReview([]string{
		"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
	}, &startOutput); err != nil {
		t.Fatal(err)
	}
	var started ReviewIntegrationStartResult
	decodeStrictReviewJSON(t, startOutput.Bytes(), &started)
	if started.RiskLevel != reviewtransaction.RiskLow || started.ChangedLines != 129 || started.CorrectionBudget != 65 ||
		started.LensesRequired || !reflect.DeepEqual(started.SelectedLenses, []string{}) {
		t.Fatalf("low-risk START = %#v", started)
	}
	if !bytes.Contains(startOutput.Bytes(), []byte(`"selected_lenses": []`)) {
		t.Fatalf("zero-lens negotiated START did not encode an array: %s", startOutput.String())
	}
	var rawStartOutput bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", started.LineageID}, &rawStartOutput); err != nil {
		t.Fatal(err)
	}
	var rawStarted ReviewFacadeStartResult
	decodeStrictReviewJSON(t, rawStartOutput.Bytes(), &rawStarted)
	if !reflect.DeepEqual(rawStarted.SelectedLenses, []string{}) || !bytes.Contains(rawStartOutput.Bytes(), []byte(`"selected_lenses": []`)) {
		t.Fatalf("zero-lens raw START did not encode an array: %s", rawStartOutput.String())
	}

	var finalizeOutput bytes.Buffer
	if err := RunReview([]string{
		"finalize", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", started.LineageID,
	}, &finalizeOutput); err != nil {
		t.Fatalf("empty low-risk FINALIZE: %v; cause: %v\n%s", err, errors.Unwrap(err), finalizeOutput.String())
	}
	var finalized ReviewIntegrationFinalizeResult
	decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, finalizeOutput.Bytes()).Result, &finalized)
	if finalized.State != reviewtransaction.StateApproved {
		t.Fatalf("empty low-risk FINALIZE = %#v", finalized)
	}

	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	assessment, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).AssessSnapshotRisk(context.Background(), record.State.InitialSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	nativeEvidence, err := reviewtransaction.NativeLowRiskVerificationEvidence(record.State, assessment)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(nativeEvidence, []byte(reviewtransaction.NativeLowRiskVerificationDomain+"\x00")) {
		t.Fatalf("native evidence lacks domain separation: %q", nativeEvidence)
	}
	sum := sha256.Sum256(nativeEvidence)
	if want := "sha256:" + hex.EncodeToString(sum[:]); record.State.EvidenceHash != want {
		t.Fatalf("native evidence hash = %q, want %q", record.State.EvidenceHash, want)
	}
	receiptPayload, err := os.ReadFile(store.ReceiptPath())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(receiptPayload, []byte(`"selected_lenses": []`)) {
		t.Fatalf("zero-lens receipt did not encode an array: %s", receiptPayload)
	}

	var gateOutput bytes.Buffer
	if err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePostApply),
	}, &gateOutput); err != nil {
		t.Fatalf("unqualified exact gate: %v\n%s", err, gateOutput.String())
	}
	var gate ReviewValidateResult
	decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, gateOutput.Bytes()).Result, &gate)
	if !gate.Allowed || gate.Result != reviewtransaction.GateAllow {
		t.Fatalf("low-risk gate = %#v", gate)
	}

	var replay bytes.Buffer
	if err := RunReview([]string{
		"finalize", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", started.LineageID,
	}, &replay); err != nil {
		t.Fatal(err)
	}
	after, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != record.Revision {
		t.Fatalf("approved replay changed revision: before=%s after=%s", record.Revision, after.Revision)
	}

	files := []string{}
	if err := filepath.WalkDir(store.Dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			files = append(files, strings.ToLower(entry.Name()))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if strings.Contains(name, "model") || strings.Contains(name, "evidence") || strings.Contains(name, "result") {
			t.Fatalf("native low-risk lifecycle created external model/evidence artifact %q", name)
		}
	}
	if elapsed := time.Since(startedAt); elapsed > 10*time.Second {
		t.Fatalf("warm low-risk lifecycle took %s", elapsed)
	}
}

func TestStaticMDXRequiresReviewerEvidence(t *testing.T) {
	repo := initReviewCLIRepo(t)
	path := filepath.Join(repo, "docs", "guide.mdx")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# Static guide\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := RunReview([]string{"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo}, &output); err != nil {
		t.Fatal(err)
	}
	var started ReviewIntegrationStartResult
	decodeStrictReviewJSON(t, output.Bytes(), &started)
	if started.RiskLevel != reviewtransaction.RiskMedium || len(started.SelectedLenses) != 1 {
		t.Fatalf("static MDX START = %#v", started)
	}
	output.Reset()
	if err := RunReview([]string{"finalize", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", started.LineageID}, &output); err == nil {
		t.Fatal("empty static MDX FINALIZE succeeded")
	}
	store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	record, _ := store.Load()
	if record.State.State != reviewtransaction.StateReviewing {
		t.Fatalf("empty static MDX FINALIZE persisted %q", record.State.State)
	}
}

func TestLowRiskExternalEvidenceRemainsBackwardCompatible(t *testing.T) {
	repo := initReviewCLIRepo(t)
	path := filepath.Join(repo, "docs", "guide.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("ordinary documentation\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startReviewOperationFixture(t, repo, "review-low-external-evidence")
	evidence := []byte("external focused tests pass\n")
	evidencePath := filepath.Join(t.TempDir(), "evidence.txt")
	if err := os.WriteFile(evidencePath, evidence, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewFacadeFinalize([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--evidence", evidencePath,
	}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(evidence)
	if want := "sha256:" + hex.EncodeToString(sum[:]); record.State.EvidenceHash != want {
		t.Fatalf("external evidence hash = %q, want %q", record.State.EvidenceHash, want)
	}
}

func TestLowRiskNativeVerificationSupportsStagedProjection(t *testing.T) {
	repo := initReviewCLIRepo(t)
	path := filepath.Join(repo, "docs", "staged-guide.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("staged documentation\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "--", "docs/staged-guide.md")

	var output bytes.Buffer
	if err := RunReview([]string{
		"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--projection", "staged",
	}, &output); err != nil {
		t.Fatal(err)
	}
	var started ReviewIntegrationStartResult
	decodeStrictReviewJSON(t, output.Bytes(), &started)
	if started.RiskLevel != reviewtransaction.RiskLow || started.Projection != reviewtransaction.ProjectionStaged ||
		!reflect.DeepEqual(started.SelectedLenses, []string{}) {
		t.Fatalf("staged low-risk START = %#v", started)
	}
	output.Reset()
	if err := RunReview([]string{
		"finalize", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", started.LineageID,
	}, &output); err != nil {
		t.Fatalf("staged empty FINALIZE: %v\n%s", err, output.String())
	}
	output.Reset()
	if err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--lineage", started.LineageID, "--gate", string(reviewtransaction.GatePreCommit),
	}, &output); err != nil {
		t.Fatalf("staged pre-commit gate: %v\n%s", err, output.String())
	}
}

func TestLowRiskNativeVerificationSupportsBaseWorkspaceOverlay(t *testing.T) {
	repo := initReviewCLIRepo(t)
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	if err := os.MkdirAll(filepath.Join(repo, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "docs", "committed.md"), []byte("committed documentation\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "docs/committed.md")
	runReviewCLIGit(t, repo, "commit", "-qm", "branch documentation")
	if err := os.WriteFile(filepath.Join(repo, "docs", "overlay.md"), []byte("overlay documentation\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{
		"--cwd", repo, "--base-ref", base, "--workspace-overlay", "--lineage", "low-risk-overlay",
	}, &output); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := RunReviewFacadeFinalize([]string{
		"--cwd", repo, "--lineage", "low-risk-overlay",
	}, &output); err != nil {
		t.Fatalf("overlay empty FINALIZE: %v\n%s", err, output.String())
	}
	var finalized ReviewFacadeFinalizeResult
	decodeStrictReviewJSON(t, output.Bytes(), &finalized)
	if finalized.State != reviewtransaction.StateApproved {
		t.Fatalf("overlay empty FINALIZE = %#v", finalized)
	}
}

func TestMediumReviewCannotApproveWithoutExternalEvidence(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startReviewOperationFixture(t, repo, "review-medium-needs-evidence")
	result := filepath.Join(t.TempDir(), "reviewer.json")
	if err := os.WriteFile(result, []byte(`{"lens":"reliability","findings":[],"evidence":["reviewed the exact candidate tree"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := RunReviewFacadeFinalize([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--result", result,
	}, &output); err != nil {
		t.Fatal(err)
	}
	var finalized ReviewFacadeFinalizeResult
	decodeStrictReviewJSON(t, output.Bytes(), &finalized)
	if finalized.State != reviewtransaction.StateValidating || finalized.ReceiptPath != "" {
		t.Fatalf("medium empty-evidence FINALIZE = %#v", finalized)
	}
}
