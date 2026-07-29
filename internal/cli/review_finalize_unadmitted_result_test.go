package cli

import (
	"bytes"
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
	if err := RunReviewFacadeStart([]string{"--cwd", repo}, &output); err != nil {
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

// writeUnadmittedCLIReviewerResults is the fabrication the defect allowed: one
// file per selected lens carrying nothing a reviewer could only produce by
// actually inspecting the candidate. No subject echo, no inspection envelope.
func writeUnadmittedCLIReviewerResults(t *testing.T, started ReviewFacadeStartResult) []string {
	t.Helper()
	directory := t.TempDir()
	args := make([]string, 0, len(started.SelectedLenses)*2)
	for order, lens := range started.SelectedLenses {
		path := filepath.Join(directory, "fabricated-"+strconv.Itoa(order)+".json")
		writeReviewCLIJSON(t, path, facadeReviewerResult{
			Lens: lens, Findings: []facadeFinding{}, Evidence: []string{"I looked at it"},
		})
		args = append(args, "--result", path)
	}
	return args
}

// TestReviewFinalizeRefusesUnadmittedReviewerResults is the fabricated-approval
// regression. `--result` performed no admission at all: it checked only that
// findings and evidence were non-nil arrays, so four hand-written files drove a
// high-risk lineage to an approved terminal receipt that the delivery gates then
// honoured, with no lens ever run and nothing on disk to show for it.
//
// The assertion deliberately targets reachable STATE rather than wording. A
// message-only assertion would still pass if the route kept minting authority
// under a different sentence, which is the defect class, not the fix.
func TestReviewFinalizeRefusesUnadmittedReviewerResults(t *testing.T) {
	repo := initReviewCLIRepo(t)
	started := startHighRiskCLIReview(t, repo)

	if err := RunReviewFacadeFinalize(append([]string{"--cwd", repo, "--lineage", started.LineageID},
		writeUnadmittedCLIReviewerResults(t, started)...), io.Discard); err == nil {
		t.Fatal("finalize accepted hand-written reviewer results that no lens produced and no admission bound")
	}

	// Refusing the call is not the invariant; refusing to ADVANCE is. A route
	// that errors after moving the lineage to validating still leaves the
	// fabricated results governing the next transition.
	var resumed bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", started.LineageID}, &resumed); err != nil {
		t.Fatalf("resume after refusal: %v", err)
	}
	var after ReviewFacadeStartResult
	if err := json.Unmarshal(resumed.Bytes(), &after); err != nil {
		t.Fatal(err)
	}
	if after.State != reviewtransaction.StateReviewing {
		t.Fatalf("authority state after refused finalize = %q, want %q: unadmitted results advanced the lineage", after.State, reviewtransaction.StateReviewing)
	}

	// The terminal artifact is what governs delivery, so its absence is the
	// load-bearing assertion.
	receipts, err := filepath.Glob(filepath.Join(repo, ".git", "gentle-ai", "review-transactions", "v2", "*", "review-receipt.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(receipts) != 0 {
		t.Fatalf("refused finalize published terminal receipts %v", receipts)
	}
	var gate bytes.Buffer
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", "pre-commit"}, &gate); err == nil {
		t.Fatalf("pre-commit gate allowed delivery with no admitted lens result: %s", gate.String())
	}
}

// TestReviewFinalizeNamedContinuationReachesApproval proves the other half of
// the branch's governing rule: a refusal may name a command only if running that
// command resolves the block. The refusal above names capture-result followed by
// finalize --captured-results, so that exact sequence must reach an approved
// receipt the delivery gate honours.
func TestReviewFinalizeNamedContinuationReachesApproval(t *testing.T) {
	repo := initReviewCLIRepo(t)
	started := startHighRiskCLIReview(t, repo)
	for order := range started.SelectedLenses {
		captureCLIReviewerResult(t, repo, started, order)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--captured-results=true"}, io.Discard); err != nil {
		t.Fatalf("finalize with captured results: %v", err)
	}
	evidencePath := filepath.Join(t.TempDir(), "evidence.txt")
	if err := os.WriteFile(evidencePath, []byte("go build ./... ok\ngo test ./... ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var finalized bytes.Buffer
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--evidence", evidencePath}, &finalized); err != nil {
		t.Fatalf("finalize with verification evidence: %v", err)
	}
	var gate bytes.Buffer
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", "pre-commit"}, &gate); err != nil {
		t.Fatalf("pre-commit gate after the named continuation: %v\n%s", err, gate.String())
	}
	assertReviewGateResult(t, gate.Bytes(), reviewtransaction.GateAllow)
}
