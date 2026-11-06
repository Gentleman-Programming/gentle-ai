package cli

import (
	"bytes"
	"testing"
)

func TestPriorStatusSchemaKeepsCurrentCaptureCollection(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n", 0o644)
	started := runNegotiatedReviewStart(t, repo, "prior-status-capture")

	var output bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--lineage", started.LineageID,
		"--contract", ReviewIntegrationContractV1, "--next-transition",
	}, &output); err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionCollect ||
		status.NextTransition.ReasonCode != "reviewer_results_required" {
		t.Fatalf("prior status capture collection = %#v", status.NextTransition)
	}
}
