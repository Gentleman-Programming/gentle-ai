package cli

import (
	"bytes"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v5/internal/reviewtransaction"
)

func TestReviewCaptureExplicitEmptySubjectRefusesWithoutConsumingSlot(t *testing.T) {
	reviewEnabledHome(t)
	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "separate empty value", args: []string{"--subject-hash", ""}},
		{name: "inline empty value", args: []string{"--subject-hash="}},
		{name: "malformed value", args: []string{"--subject-hash=not-a-hash"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo, started, _, record := newArtifactReview(t, true)
			lens := record.State.SelectedLenses[0]
			result := admittedReviewerResultForTest(t, repo, record, lens, 0)
			input := writeReviewerOutputWithOptionalEcho(t, result, result.SubjectHash)
			args := []string{"capture-result", "--cwd", repo, "--lineage", started.LineageID,
				"--target", started.TargetIdentity, "--lens", lens, "--order", "0", "--input", input}
			before := explicitFrozenReviewingStatus(t, repo, started.LineageID)
			var out bytes.Buffer
			err := RunReview(append(args, test.args...), &out)
			after := explicitFrozenReviewingStatus(t, repo, started.LineageID)
			if !reflect.DeepEqual(before, after) {
				t.Error("invalid host subject changed STATUS authority or slot offers")
			}
			if err == nil {
				t.Fatalf("invalid host subject was accepted: %s", &out)
			}
			failure := decodeCaptureRefusalEnvelope(t, err, out.Bytes())
			const cause = "review capture subject hash does not match the provider-owned authority; refresh the binding with gentle-ai review status --cwd <repo> --contract <same-contract> --next-transition"
			if failure.Code != "capture_binding_mismatch" || failure.Phase != "preflight" ||
				failure.MutationOutcome != ReviewMutationNotStarted || failure.NextAction != "review.status" ||
				!failure.RetrySafe || failure.LineageID != started.LineageID || failure.Cause != cause ||
				!strings.HasPrefix(err.Error(), cause) {
				t.Fatalf("invalid host subject refusal = %#v; error = %v", failure, err)
			}
		})
	}
}

func TestReviewCaptureMissingInspectionStatusPreservesRefusal(t *testing.T) {
	reviewEnabledHome(t)
	repo, started, _, record := newArtifactReview(t, true)
	lens := record.State.SelectedLenses[0]
	result := admittedReviewerResultForTest(t, repo, record, lens, 0)
	input := filepath.Join(t.TempDir(), "result.json")
	writeReviewCLIJSON(t, input, map[string]any{
		"inspection": map[string]any{"paths": result.Inspection.Paths}, "findings": result.Findings, "evidence": result.Evidence,
	})
	before := explicitFrozenReviewingStatus(t, repo, started.LineageID)
	var out bytes.Buffer
	err := RunReview([]string{
		"capture-result", "--cwd", repo, "--lineage", started.LineageID, "--target", started.TargetIdentity,
		"--lens", lens, "--order", "0", "--input", input, "--subject-hash", result.SubjectHash, "--preflight",
	}, &out)
	after := explicitFrozenReviewingStatus(t, repo, started.LineageID)
	if !reflect.DeepEqual(before, after) {
		t.Error("missing inspection status changed STATUS authority or slot offers")
	}
	failure := decodeCaptureRefusalEnvelope(t, err, out.Bytes())
	const cause = "reviewer artifact admission incomplete: reviewer did not report completed candidate inspection"
	if failure.Code != "invalid_request" || failure.Phase != "preflight" || failure.MutationOutcome != ReviewMutationNotStarted ||
		failure.Cause != cause || err.Error() != cause+" [invalid_request]" {
		t.Fatalf("missing inspection status refusal = %#v; error = %q; want unchanged cause %q", failure, err, cause)
	}
}

func TestReviewCaptureResultManualSubjectEchoBoundary(t *testing.T) {
	reviewEnabledHome(t)
	for _, test := range []struct {
		name         string
		wantDecision reviewtransaction.ArtifactAdmissionDecision
	}{
		{name: "foreign echo", wantDecision: reviewtransaction.ArtifactAdmissionBindingMismatch},
		{name: "present but empty echo", wantDecision: reviewtransaction.ArtifactAdmissionIncomplete},
		{name: "omitted echo", wantDecision: reviewtransaction.ArtifactAdmissionCompleted},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo, started, _, record := newArtifactReview(t, true)
			lens := record.State.SelectedLenses[0]
			result := admittedReviewerResultForTest(t, repo, record, lens, 0)
			boundHash := result.SubjectHash
			var input string
			if test.name == "omitted echo" {
				input = writeReviewerOutputWithOptionalEcho(t, result, "")
			} else {
				result.SubjectHash = ""
				if test.name == "foreign echo" {
					result.SubjectHash = admittedReviewerResultForTest(t, repo, record, record.State.SelectedLenses[1], 1).SubjectHash
				}
				input = filepath.Join(t.TempDir(), "result.json")
				writeReviewCLIJSON(t, input, result)
			}
			before := explicitFrozenReviewingStatus(t, repo, started.LineageID)
			var out bytes.Buffer
			err := RunReview([]string{
				"capture-result", "--cwd", repo, "--lineage", started.LineageID, "--target", started.TargetIdentity,
				"--lens", lens, "--order", "0", "--input", input,
			}, &out)
			if test.wantDecision != reviewtransaction.ArtifactAdmissionCompleted {
				after := explicitFrozenReviewingStatus(t, repo, started.LineageID)
				if !reflect.DeepEqual(before, after) {
					t.Error("rejected manual echo changed STATUS authority or slot offers")
				}
				failure := decodeCaptureRefusalEnvelope(t, err, out.Bytes())
				prefix := "reviewer artifact admission " + string(test.wantDecision) + ":"
				if failure.Phase != "preflight" || failure.Code != "invalid_request" ||
					failure.MutationOutcome != ReviewMutationNotStarted || !strings.HasPrefix(failure.Cause, prefix) ||
					!strings.HasPrefix(err.Error(), prefix) {
					t.Fatalf("manual echo refusal = %#v; error = %v; want %s", failure, err, prefix)
				}
				return
			}
			if err != nil {
				t.Fatalf("manual output without an echo was refused: %v", err)
			}
			var artifact reviewResultArtifact
			decodeStrictReviewJSON(t, out.Bytes(), &artifact)
			if artifact.Schema != reviewResultArtifactSchema || artifact.AdmissionDecision != test.wantDecision ||
				artifact.LineageID != started.LineageID || artifact.Lens != lens || artifact.SelectedOrder != 0 || artifact.SubjectHash != boundHash {
				t.Fatalf("manual capture = %#v; want completed provider-bound artifact", artifact)
			}
		})
	}
}
