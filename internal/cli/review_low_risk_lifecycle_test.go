package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestOrdinaryMarkdownLowRiskLifecycleNeedsNoExternalEvidence(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	lines := make([]string, 129)
	for index := range lines {
		lines[index] = fmt.Sprintf("ordinary documentation line %03d", index+1)
	}
	writeReviewStartCandidate(t, repo, "docs/ordinary-guide.md", strings.Join(lines, "\n")+"\n", 0o644)

	var startOutput bytes.Buffer
	if err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
	}), &startOutput); err != nil {
		t.Fatal(err)
	}
	var started ReviewIntegrationStartResult
	decodeStrictReviewJSON(t, startOutput.Bytes(), &started)
	if started.RiskLevel != reviewtransaction.RiskLow || started.ChangedLines != 129 || started.CorrectionBudget != 65 ||
		started.State != reviewtransaction.StateApproved || started.Action != "closed" ||
		started.LensesRequired || !reflect.DeepEqual(started.SelectedLenses, []string{}) {
		t.Fatalf("low-risk START = %#v", started)
	}
	if !bytes.Contains(startOutput.Bytes(), []byte(`"selected_lenses": []`)) {
		t.Fatalf("zero-lens negotiated START did not encode an array: %s", startOutput.String())
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	assertApprovedCompactAuthorityBurned(t, store, started.LineageID)
	if _, err := os.Stat(store.StatePath()); !os.IsNotExist(err) {
		t.Fatalf("zero-lens START retained compact authority state: %v", err)
	}

	var rawStartOutput bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", started.LineageID}, &rawStartOutput); err != nil {
		t.Fatal(err)
	}
	var rawStarted ReviewFacadeStartResult
	decodeStrictReviewJSON(t, rawStartOutput.Bytes(), &rawStarted)
	if rawStarted.State != reviewtransaction.StateApproved || rawStarted.Action != "closed" ||
		!reflect.DeepEqual(rawStarted.SelectedLenses, []string{}) || !bytes.Contains(rawStartOutput.Bytes(), []byte(`"selected_lenses": []`)) {
		t.Fatalf("zero-lens raw START did not close and encode zero lenses: %s", rawStartOutput.String())
	}
	assertApprovedCompactAuthorityBurned(t, store, started.LineageID)
}

// TestActiveMDXRequiresReviewerCapture pins the content-classified boundary for
// MDX: runtime syntax withdraws the passive nomination its extension carries, so
// the candidate becomes one consolidated review that closes on its reviewer
// capture rather than structural readback alone.
func TestActiveMDXRequiresReviewerEvidence(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "docs/guide.mdx", "import Widget from './widget'\n\n# Active guide\n", 0o644)
	var output bytes.Buffer
	if err := RunReview(boundNegotiatedStartArgs(t, []string{"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo}), &output); err != nil {
		t.Fatal(err)
	}
	var started ReviewIntegrationStartResult
	decodeStrictReviewJSON(t, output.Bytes(), &started)
	if started.RiskLevel != reviewtransaction.RiskMedium || len(started.SelectedLenses) != 1 {
		t.Fatalf("active MDX START = %#v", started)
	}
	legacyStarted := ReviewFacadeStartResult{
		LineageID: started.LineageID, TargetIdentity: started.RepositoryContext.TargetIdentity,
		SelectedLenses: started.SelectedLenses,
	}
	captureCleanCLIReviewerResult(t, repo, legacyStarted, 0, &output)
	if !bytes.Contains(output.Bytes(), []byte(reviewLastEventClosureSchema)) {
		t.Fatalf("active MDX terminal capture = %s", output.String())
	}
}

func TestLowRiskNativeVerificationSupportsStagedProjection(t *testing.T) {
	reviewEnabledHome(t)
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
	if err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--projection", "staged",
	}), &output); err != nil {
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
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--lineage", started.LineageID, "--gate", string(reviewtransaction.GatePreCommit),
	}, &output); err != nil {
		t.Fatalf("staged pre-commit gate: %v\n%s", err, output.String())
	}
}

func TestLowRiskNativeVerificationSupportsBaseWorkspaceOverlay(t *testing.T) {
	reviewEnabledHome(t)
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
	_, digest, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).IntendedUntrackedInventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{
		"--cwd", repo, "--base-ref", base, "--workspace-overlay", "--lineage", "low-risk-overlay",
		"--untracked-scope=exclude", "--expected-untracked-inventory=" + digest,
	}, &output); err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, output.Bytes(), &started)
	if started.State != reviewtransaction.StateApproved || started.Action != "closed" {
		t.Fatalf("overlay zero-lens START = %#v", started)
	}
}

func TestMediumReviewClosesOnCapturedEvidence(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startReviewOperationFixture(t, repo, "review-medium-needs-evidence")
	result := filepath.Join(t.TempDir(), "reviewer.json")
	if err := os.WriteFile(result, []byte(`{"lens":"reliability","findings":[],"evidence":["reviewed the exact candidate tree"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if err := captureReviewCLIResultFiles(t, repo, started.LineageID, []string{result}); err != nil {
		t.Fatal(err)
	}
	assertApprovedCompactAuthorityBurned(t, store, started.LineageID)
	if _, err := os.Stat(store.StatePath()); !os.IsNotExist(err) {
		t.Fatalf("captured medium evidence retained compact authority state: %v", err)
	}
}
