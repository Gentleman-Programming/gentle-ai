package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
)

func incompleteCompactResidueDir(t *testing.T, repo, lineage string) string {
	t.Helper()
	commonDir := filepath.Clean(strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "--path-format=absolute", "--git-common-dir")))
	residue := filepath.Join(commonDir, "gentle-ai", "review-transactions", "v2", lineage)
	if err := os.MkdirAll(residue, 0o755); err != nil {
		t.Fatal(err)
	}
	return residue
}

func TestUnqualifiedGateDiscoveryDeniesIncompleteStoreEntryWithDistinctCause(t *testing.T) {
	repo := initReviewCLIRepo(t)
	approveDiscoveryMarkdown(t, repo, "review-reclaim-valid", "docs/valid.md", "valid\n")
	incompleteCompactResidueDir(t, repo, "reclaim-audit")

	var output bytes.Buffer
	err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePostApply),
	}, &output)
	if err == nil {
		t.Fatal("incomplete store entry did not deny lineage-less gate validation")
	}
	failure := decodeReviewIntegrationFailure(t, output.Bytes())
	if failure.Code != "authority_corrupted" || failure.CauseCategory != "incomplete_store_entry" {
		t.Fatalf("incomplete store entry failure = %#v", failure)
	}
}

func TestReviewReclaimQuarantinesResidueAndRestoresLineagelessDiscovery(t *testing.T) {
	repo := initReviewCLIRepo(t)
	approveDiscoveryMarkdown(t, repo, "review-reclaim-valid", "docs/valid.md", "valid\n")
	residue := incompleteCompactResidueDir(t, repo, "reclaim-audit")

	var reclaimOutput bytes.Buffer
	if err := RunReview([]string{
		"reclaim", "--cwd", repo, "--lineage", "reclaim-audit",
		"--reason", "interrupted store write residue", "--actor", "maintainer@example.com",
	}, &reclaimOutput); err != nil {
		t.Fatalf("review reclaim: %v\n%s", err, reclaimOutput.String())
	}
	var result ReviewReclaimResult
	decodeStrictReviewJSON(t, reclaimOutput.Bytes(), &result)
	if result.Operation != "review/reclaim" || result.Record.LineageID != "reclaim-audit" || result.Record.QuarantinePath == "" {
		t.Fatalf("review reclaim result = %#v", result)
	}
	if _, err := os.Stat(residue); !os.IsNotExist(err) {
		t.Fatalf("reclaimed residue still present: %v", err)
	}
	if _, err := os.Stat(filepath.Join(result.Record.QuarantinePath, "reclaim-record.json")); err != nil {
		t.Fatalf("reclaim audit record missing: %v", err)
	}

	var validateOutput bytes.Buffer
	if err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePostApply),
	}, &validateOutput); err != nil {
		t.Fatalf("post-reclaim lineage-less validation: %v\n%s", err, validateOutput.String())
	}
	var validated ReviewValidateResult
	decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, validateOutput.Bytes()).Result, &validated)
	if !validated.Allowed || validated.Context.LineageID != "review-reclaim-valid" {
		t.Fatalf("post-reclaim validation = %#v", validated)
	}

	var refusedOutput bytes.Buffer
	if err := RunReview([]string{
		"reclaim", "--cwd", repo, "--lineage", "review-reclaim-valid",
		"--reason", "must refuse", "--actor", "maintainer@example.com",
	}, &refusedOutput); err == nil {
		t.Fatal("review reclaim touched a lineage with authoritative state")
	}
}

func TestReviewStartSucceedsDespiteIncompleteStoreResidue(t *testing.T) {
	repo := initReviewCLIRepo(t)
	incompleteCompactResidueDir(t, repo, "reclaim-audit")
	if err := os.MkdirAll(filepath.Join(repo, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "docs", "start.md"), []byte("start\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "review-reclaim-start"}, &output); err != nil {
		t.Fatalf("review start with incomplete residue present: %v\n%s", err, output.String())
	}
}

func TestReviewReclaimUnblocksStartPoisonedByEnumeratedResidue(t *testing.T) {
	repo := initReviewCLIRepo(t)
	residue := incompleteCompactResidueDir(t, repo, "reclaim-audit")
	if err := os.WriteFile(filepath.Join(residue, "stray.tmp"), []byte("stray\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "docs", "start.md"), []byte("start\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "review-reclaim-start"}, &output)
	if err == nil {
		t.Fatal("review start ignored an enumerated incomplete store entry")
	}
	if !strings.Contains(err.Error(), "load compact start authority") {
		t.Fatalf("start-time enumeration failure = %v", err)
	}

	if err := RunReview([]string{
		"reclaim", "--cwd", repo, "--lineage", "reclaim-audit",
		"--reason", "interrupted store write residue", "--actor", "maintainer@example.com",
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("reclaim enumerated residue: %v", err)
	}
	output.Reset()
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "review-reclaim-start"}, &output); err != nil {
		t.Fatalf("review start after reclaim: %v\n%s", err, output.String())
	}
}
