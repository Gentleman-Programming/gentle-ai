package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestPowerShell51ReviewerPayloadBindsProviderArtifactSubject(t *testing.T) {
	reviewEnabledHome(t)
	repo, started, _, record := newArtifactReview(t, false)
	lens := record.State.SelectedLenses[0]
	input := filepath.Join(t.TempDir(), "reviewer.json")
	if err := os.WriteFile(input, powerShell51ReviewerPayloadForTest(t, repo, record, lens, 0), 0o600); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := RunReviewCaptureResult([]string{
		"--cwd", repo,
		"--lineage", started.LineageID,
		"--target", record.State.InitialSnapshot.Identity,
		"--lens", lens,
		"--order", "0",
		"--input", input,
	}, &output); err != nil {
		t.Fatalf("PowerShell reviewer fixture omitted its provider-owned artifact subject: %v", err)
	}
	var terminal reviewLastEventClosureResult
	decodeStrictReviewJSON(t, output.Bytes(), &terminal)
	if terminal.Operation != "review/capture-result" || terminal.State != reviewtransaction.StateApproved {
		t.Fatalf("PowerShell reviewer terminal capture = %#v", terminal)
	}
}

func powerShell51ReviewerPayloadForTest(t *testing.T, repo string, record reviewtransaction.CompactRecord, lens string, order int) []byte {
	t.Helper()
	return admittedReviewerPayloadForTest(t, repo, record, lens, order,
		"checked exact target through Windows PowerShell 5.1")
}
