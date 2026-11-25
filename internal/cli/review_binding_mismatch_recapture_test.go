package cli

import (
	"bytes"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v5/internal/reviewtransaction"
)

func TestReviewCaptureResultRecapturesSameLensAfterForeignEcho(t *testing.T) {
	reviewEnabledHome(t)
	repo, started, _, record := newArtifactReview(t, false)
	lens := record.State.SelectedLenses[0]
	result := admittedReviewerResultForTest(t, repo, record, lens, 0)
	foreign := writeReviewerOutputWithOptionalEcho(t, result, "sha256:"+strings.Repeat("b", 64))
	args := []string{
		"capture-result", "--cwd", repo, "--lineage", started.LineageID, "--target", started.TargetIdentity,
		"--lens", lens, "--order", "0", "--input",
	}

	before := explicitFrozenReviewingStatus(t, repo, started.LineageID)
	var refused bytes.Buffer
	err := RunReview(append(args, foreign), &refused)
	after := explicitFrozenReviewingStatus(t, repo, started.LineageID)
	if !reflect.DeepEqual(before, after) {
		t.Error("foreign echo changed STATUS authority or slot offers")
	}
	failure := decodeCaptureRefusalEnvelope(t, err, refused.Bytes())
	want := "reviewer artifact admission binding_mismatch: reviewer result echoed a different artifact subject: " +
		"the rejected admission did not consume the lens slot, so re-run the lens and invoke gentle-ai review capture-result again " +
		"on the same lineage with a result that echoes the binding's top-level subject_hash, which is " + result.SubjectHash
	const preserved = "; the rejected reviewer payload was preserved at "
	cause, path, _ := strings.Cut(strings.TrimSuffix(err.Error(), " [invalid_request]"), preserved)
	if failure.Cause != want+preserved+"<redacted>" || cause != want || !filepath.IsAbs(path) ||
		!strings.HasSuffix(err.Error(), " [invalid_request]") || failure.Code != "invalid_request" ||
		failure.Phase != "preflight" || failure.MutationOutcome != ReviewMutationNotStarted {
		t.Fatalf("manual foreign echo refusal = %#v; error = %v; want cause %q", failure, err, want)
	}

	var captured bytes.Buffer
	corrected := writeReviewerOutputWithOptionalEcho(t, result, result.SubjectHash)
	if err := RunReview(append(args, corrected), &captured); err != nil {
		t.Fatalf("recapture on the same lineage and lens: %v", err)
	}
	var terminal reviewLastEventClosureResult
	decodeStrictReviewJSON(t, captured.Bytes(), &terminal)
	if terminal.Schema != reviewLastEventClosureSchema || terminal.Operation != "review/capture-result" ||
		terminal.LineageID != started.LineageID || terminal.State != reviewtransaction.StateApproved {
		t.Fatalf("manual echo recapture terminal result = %#v", terminal)
	}
}

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
