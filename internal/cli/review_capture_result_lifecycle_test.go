package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// startHighRiskCLIReview freezes a candidate the native tier assessment rates
// high risk, so the lineage really does select the canonical four lenses. The
// fabricated-approval defect only exists where lenses were required, so a
// fixture that quietly lands on tier 0 would prove nothing.
func startHighRiskCLIReview(t *testing.T, repo string) ReviewFacadeStartResult {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(repo, "internal", "auth"), 0o755); err != nil {
		t.Fatal(err)
	}
	source := "package auth\n\n// CheckToken reports whether a session token is present.\nfunc CheckToken(token string) bool {\n\treturn token != \"\"\n}\n"
	if err := os.WriteFile(filepath.Join(repo, "internal", "auth", "session.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "-A")
	var output bytes.Buffer
	if err := runLegacyFacadeStartForTest(t, []string{"--cwd", repo}, &output); err != nil {
		t.Fatalf("review start: %v", err)
	}
	var started ReviewFacadeStartResult
	if err := json.Unmarshal(output.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	if started.RiskLevel != reviewtransaction.RiskHigh || len(started.SelectedLenses) != 4 {
		t.Fatalf("fixture risk = %q with lenses %v, want high risk selecting four lenses", started.RiskLevel, started.SelectedLenses)
	}
	return started
}

// captureCLIReviewerResult drives the native admission route for one lens: ask
// the provider for the frozen binding, echo it back, and capture. This is the
// continuation the refusal below is required to name, so the test that proves
// the refusal and the test that proves the way out share one implementation.
func captureCLIReviewerResult(t *testing.T, repo string, started ReviewFacadeStartResult, order int) {
	t.Helper()
	lens := started.SelectedLenses[order]
	binding := []string{
		"--cwd", repo, "--lineage", started.LineageID, "--target", started.TargetIdentity,
		"--lens", lens, "--order", strconv.Itoa(order),
	}
	var preflightOutput bytes.Buffer
	if err := RunReviewCaptureResult(append(binding, "--preflight"), &preflightOutput); err != nil {
		t.Fatalf("capture-result --preflight for lens %q: %v", lens, err)
	}
	var preflight reviewCapturePreflightResult
	if err := json.Unmarshal(preflightOutput.Bytes(), &preflight); err != nil {
		t.Fatal(err)
	}
	paths := make([]string, len(preflight.ChangedPathManifest))
	for index, entry := range preflight.ChangedPathManifest {
		paths[index] = entry.Path
	}
	resultPath := filepath.Join(t.TempDir(), "reviewer-"+strconv.Itoa(order)+".json")
	writeReviewCLIJSON(t, resultPath, facadeReviewerResult{
		SubjectHash: preflight.ArtifactSubject.SubjectHash,
		Inspection: reviewtransaction.ArtifactInspection{
			Status: reviewtransaction.ArtifactInspectionCompleted, Paths: paths,
		},
		Findings: []facadeFinding{},
		Evidence: []string{"inspected the complete frozen candidate scope named by the capture binding"},
	})
	if err := RunReviewCaptureResult(append(binding, "--input", resultPath), io.Discard); err != nil {
		t.Fatalf("capture-result for lens %q: %v", lens, err)
	}
}

// TestCurrentCaptureResultClosesApprovedLineage verifies that the negotiated
// STATUS route remains collect-only until every selected lens has durably
// admitted its bound result. The final capture is the terminal event: it
// approves the clean review and removes its compact authority.
func TestCurrentCaptureResultClosesApprovedLineage(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	started := startHighRiskCLIReview(t, repo)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}

	for order := range started.SelectedLenses {
		var output bytes.Buffer
		if err := RunReview([]string{
			"status", "--cwd", repo, "--lineage", started.LineageID,
			"--contract", ReviewIntegrationContractV2, "--next-transition",
		}, &output); err != nil {
			t.Fatalf("STATUS before lens %d capture: %v\n%s", order, err, output.String())
		}
		var status ReviewTargetStatusResult
		decodeStrictReviewJSON(t, output.Bytes(), &status)
		if status.Authority == nil || status.Authority.LineageID != started.LineageID ||
			status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionCollect ||
			status.NextTransition.ReasonCode != "reviewer_results_required" ||
			status.NextTransition.Collect == nil || len(status.NextTransition.Collect.Inputs) != len(started.SelectedLenses)-order ||
			status.NextTransition.Collect.Inputs[0].CaptureOperation != reviewCaptureResultCaptureOperation {
			t.Fatalf("STATUS before lens %d capture = %#v", order, status)
		}
		captureCLIReviewerResult(t, repo, started, order)
	}

	assertApprovedCompactAuthorityBurned(t, store, started.LineageID)
}
