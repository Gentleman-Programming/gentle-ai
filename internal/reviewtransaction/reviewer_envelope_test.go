package reviewtransaction

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestReviewerResultEnvelopeMatchesAdmission ties the published schema to the
// code that actually admits. The envelope is what every prompt derives its
// wording from, so a schema that drifts from AdmitArtifact would teach lens
// agents a shape admission refuses.
func TestReviewerResultEnvelopeMatchesAdmission(t *testing.T) {
	envelope := NewReviewerResultEnvelope()
	if envelope.CompletedInspectionStatus != string(ArtifactInspectionCompleted) {
		t.Fatalf("CompletedInspectionStatus = %q, want %q", envelope.CompletedInspectionStatus, ArtifactInspectionCompleted)
	}
	for _, want := range []string{"subject_hash", "inspection", "findings", "evidence"} {
		if !contains(envelope.RequiredTopLevelFields, want) {
			t.Fatalf("required fields %v omit %q", envelope.RequiredTopLevelFields, want)
		}
	}
	for _, lens := range supportedLenses {
		if !contains(envelope.LensAgentNames, lens) {
			t.Fatalf("lens agent names %v omit the supported lens %q", envelope.LensAgentNames, lens)
		}
	}
	if len(envelope.LensAgentNames) != len(supportedLenses) {
		t.Fatalf("lens agent names %v do not match the supported lenses %v", envelope.LensAgentNames, supportedLenses)
	}
}

// TestSchemaExampleShapedResultIsAdmitted builds a payload the way a reader
// following only the published schema would build it — its own example, with
// the placeholder subject and paths replaced by the real binding — and proves
// the real admission path accepts it. The mirror case proves the refusal for a
// missing subject_hash still names the way out.
func TestSchemaExampleShapedResultIsAdmitted(t *testing.T) {
	subject, frozen, request := admittedArtifactFixture(t)

	var document struct {
		Examples []map[string]json.RawMessage `json:"examples"`
	}
	if err := json.Unmarshal([]byte(ReviewerResultSchema), &document); err != nil || len(document.Examples) == 0 {
		t.Fatalf("published schema carries no worked example: %v", err)
	}
	example := document.Examples[0]

	var inspection ArtifactInspection
	if err := json.Unmarshal(example["inspection"], &inspection); err != nil {
		t.Fatalf("example inspection does not decode into the admitted type: %v", err)
	}
	if inspection.Status != ArtifactInspectionCompleted {
		t.Fatalf("example inspection.status = %q, want %q", inspection.Status, ArtifactInspectionCompleted)
	}
	var evidence []string
	if err := json.Unmarshal(example["evidence"], &evidence); err != nil || len(evidence) == 0 {
		t.Fatalf("example evidence does not decode into a non-empty list: %v", err)
	}

	// The example's placeholders resolve to the frozen binding, exactly as a
	// reviewer resolves them from its issued binding and changed-path manifest.
	inspection.Paths = nil
	for _, entry := range frozen.ChangedPathManifest {
		inspection.Paths = append(inspection.Paths, entry.Path)
	}
	request.EchoedSubjectHash = subject.SubjectHash
	request.Inspection = inspection
	request.Result = LensResult{Lens: LensReliability, Findings: []Finding{}, Evidence: evidence}
	request.CandidateCausalFindingIDs = nil

	_, admission, err := AdmitArtifact(t.Context(), request)
	if err != nil || admission.Decision != ArtifactAdmissionCompleted {
		t.Fatalf("AdmitArtifact(schema-shaped result) decision = %q, error = %v; want completed", admission.Decision, err)
	}

	request.EchoedSubjectHash = ""
	_, admission, err = AdmitArtifact(t.Context(), request)
	if err == nil || admission.Decision != ArtifactAdmissionIncomplete {
		t.Fatalf("AdmitArtifact(no subject_hash) decision = %q, error = %v; want incomplete", admission.Decision, err)
	}
	for _, want := range []string{"subject_hash", "inspection", "re-run"} {
		if !strings.Contains(admission.Diagnostic, want) {
			t.Fatalf("refusal %q does not name %q", admission.Diagnostic, want)
		}
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
