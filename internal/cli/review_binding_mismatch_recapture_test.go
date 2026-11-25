package cli

import (
	"bytes"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v5/internal/reviewtransaction"
)

func TestReviewCaptureResultRecapturesSameLensAfterBindingMismatch(t *testing.T) {
	reviewEnabledHome(t)
	repo, started, _, record := newArtifactReview(t, true)
	lens := record.State.SelectedLenses[0]
	result := admittedReviewerResultForTest(t, repo, record, lens, 0)
	otherSubjectHash := admittedReviewerResultForTest(t, repo, record, record.State.SelectedLenses[1], 1).SubjectHash
	input := filepath.Join(t.TempDir(), "result.json")
	writeReviewCLIJSON(t, input, result)

	before := explicitFrozenReviewingStatus(t, repo, started.LineageID)
	captureArgs := []string{
		"capture-result", "--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity,
		"--lens", lens, "--order", "0", "--input", input, "--subject-hash",
	}
	var refused bytes.Buffer
	rejected := RunReview(append(captureArgs, otherSubjectHash), &refused)
	after := explicitFrozenReviewingStatus(t, repo, started.LineageID)
	if !reflect.DeepEqual(before, after) {
		t.Errorf("cross-slot subject changed STATUS: before = %#v; after = %#v", before, after)
	}
	failure := decodeCaptureRefusalEnvelope(t, rejected, refused.Bytes())
	if failure.Code != "capture_binding_mismatch" || failure.Phase != "preflight" ||
		failure.NextAction != "review.status" || failure.MutationOutcome != ReviewMutationNotStarted ||
		!failure.RetrySafe || failure.LineageID != started.LineageID {
		t.Fatalf("cross-slot capture refusal = %#v", failure)
	}
	const wantRefusal = "review capture subject hash does not match the provider-owned authority; refresh the binding with gentle-ai review status --cwd <repo> --contract <same-contract> --next-transition"
	if !strings.Contains(rejected.Error(), wantRefusal) {
		t.Fatalf("cross-slot capture error = %q, want %q", rejected, wantRefusal)
	}

	var captured bytes.Buffer
	if err := RunReview(append(captureArgs, result.SubjectHash), &captured); err != nil {
		t.Fatalf("recapture with the proper host subject was refused: %v", err)
	}
	var artifact reviewResultArtifact
	decodeStrictReviewJSON(t, captured.Bytes(), &artifact)
	if artifact.Schema != reviewResultArtifactSchema || artifact.AdmissionDecision != reviewtransaction.ArtifactAdmissionCompleted ||
		artifact.LineageID != started.LineageID || artifact.Lens != lens || artifact.SelectedOrder != 0 || artifact.SubjectHash != result.SubjectHash {
		t.Fatalf("recapture artifact = %#v", artifact)
	}
}
