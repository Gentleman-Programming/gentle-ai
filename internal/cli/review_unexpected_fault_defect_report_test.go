package cli

// Issue #1881 follow-up: an unanticipated internal error on a mutating review
// operation walled the operator with no envelope and no artifact. These tests
// pin the extended defect-report treatment at the negotiated failure choke
// point:
//
//   - the residue class only: a mutating operation whose failure no typed
//     branch claimed (operation_outcome_unknown) generates a defect report
//     artifact naming the issues URL, alongside the parseable envelope;
//   - a NAMED refusal is never a defect and never writes a report;
//   - read-only operations never write a report;
//   - a failed report save never masks the original error or the envelope.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const reviewTestZeroTarget = "sha256:0000000000000000000000000000000000000000000000000000000000000000"

func reviewDefectReportDir(t *testing.T, repo string) string {
	t.Helper()
	commonDir := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "--git-common-dir"))
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(repo, commonDir)
	}
	return filepath.Join(commonDir, "gentle-ai", reviewDefectReportDirName)
}

// injectReviewStartFault forces an unanticipated internal fault at START's
// candidate freeze: a real choke point on the mutating path, reached before
// any authority exists. It replaced the untracked-scope discovery seam that
// #2394 removed from START.
func injectReviewStartFault(t *testing.T, fault error) {
	t.Helper()
	previous := reviewFacadeBuildStartSnapshot
	reviewFacadeBuildStartSnapshot = func(context.Context, reviewtransaction.SnapshotBuilder, reviewtransaction.Target) (reviewtransaction.Snapshot, error) {
		return reviewtransaction.Snapshot{}, fault
	}
	t.Cleanup(func() { reviewFacadeBuildStartSnapshot = previous })
}

func TestNegotiatedStartUnanticipatedFaultEmitsEnvelopeAndDefectReport(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "candidate\n", 0o644)
	injectReviewStartFault(t, errors.New("injected unanticipated discovery fault"))

	var output bytes.Buffer
	err := RunReview([]string{
		"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--projection", "workspace", "--target", reviewTestZeroTarget,
	}, &output)
	if err == nil {
		t.Fatalf("negotiated start succeeded through an injected internal fault:\n%s", output.String())
	}
	// The envelope must still reach stdout, parseable, classified as residue.
	var failure ReviewIntegrationFailure
	if decodeErr := json.Unmarshal(output.Bytes(), &failure); decodeErr != nil {
		t.Fatalf("negotiated fault stdout is not a parseable envelope: %v\n%s", decodeErr, output.String())
	}
	if failure.Code != "operation_outcome_unknown" {
		t.Fatalf("injected internal fault classified as %q, want the operation_outcome_unknown residue", failure.Code)
	}
	// The defect report artifact must exist and name the issues URL.
	dir := reviewDefectReportDir(t, repo)
	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		t.Fatalf("no defect report directory after an unanticipated mutating fault: %v", readErr)
	}
	if len(entries) != 1 {
		t.Fatalf("defect report directory holds %d entries, want exactly 1", len(entries))
	}
	content, readErr := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if readErr != nil {
		t.Fatal(readErr)
	}
	report := string(content)
	if !strings.Contains(report, reviewDefectReportIssuesURL) {
		t.Fatalf("defect report does not name the issues URL:\n%s", report)
	}
	if !strings.Contains(report, "operation_outcome_unknown") {
		t.Fatalf("defect report does not carry the residue reason code:\n%s", report)
	}
	// The operator-facing error names the saved report so the wall comes with
	// the report almost written.
	if !strings.Contains(err.Error(), "defect report") {
		t.Fatalf("returned error does not mention the saved defect report: %q", err.Error())
	}
}

func TestNamedRefusalsAndReadOnlyOperationsNeverWriteDefectReports(t *testing.T) {
	repo := initReviewCLIRepo(t)
	runReviewCLIGit(t, repo, "init", "-q", filepath.Join(repo, "vendor", "embedded"))
	writeReviewStartCandidate(t, repo, "tracked.txt", "candidate\n", 0o644)

	// Named refusal on a mutating operation: the embedded-repository refusal
	// is a typed, anticipated condition, not a defect.
	var output bytes.Buffer
	if err := RunReview([]string{
		"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--projection", "workspace", "--target", reviewTestZeroTarget,
	}, &output); err == nil {
		t.Fatalf("negotiated start admitted an embedded foreign repository:\n%s", output.String())
	}
	// Read-only operation failing on the same repository shape.
	output.Reset()
	_ = RunReview([]string{"status", "--contract", ReviewIntegrationContractV1, "--cwd", repo}, &output)

	if entries, err := os.ReadDir(reviewDefectReportDir(t, repo)); err == nil && len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("named refusal or read-only failure generated defect reports: %v", names)
	}
}

func TestDefectReportSaveFailureNeverMasksTheEnvelopeOrError(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "candidate\n", 0o644)
	injectReviewStartFault(t, errors.New("injected unanticipated discovery fault"))
	// Occupy the report directory path with a regular file so persisting the
	// report fails deterministically.
	commonDir := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "--git-common-dir"))
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(repo, commonDir)
	}
	if err := os.MkdirAll(filepath.Join(commonDir, "gentle-ai"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(commonDir, "gentle-ai", reviewDefectReportDirName), []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	err := RunReview([]string{
		"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--projection", "workspace", "--target", reviewTestZeroTarget,
	}, &output)
	if err == nil {
		t.Fatalf("negotiated start succeeded through an injected internal fault:\n%s", output.String())
	}
	var failure ReviewIntegrationFailure
	if decodeErr := json.Unmarshal(output.Bytes(), &failure); decodeErr != nil {
		t.Fatalf("failed report save broke the envelope: %v\n%s", decodeErr, output.String())
	}
	if failure.Code != "operation_outcome_unknown" {
		t.Fatalf("failed report save changed the classification to %q", failure.Code)
	}
	var typed *ReviewIntegrationFailureError
	if !errors.As(err, &typed) {
		t.Fatalf("failed report save replaced the typed failure error with %T: %v", err, err)
	}
	if strings.Contains(err.Error(), "defect report") {
		t.Fatalf("error claims a defect report that was never saved: %q", err.Error())
	}
}
