package reviewtransaction

import (
	"encoding/json"
	"reflect"
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

func TestReviewerResultSchemaPublishesBothFindingLensForms(t *testing.T) {
	var document map[string]any
	if err := json.Unmarshal([]byte(ReviewerResultSchema), &document); err != nil {
		t.Fatalf("decode reviewer schema: %v", err)
	}
	properties := document["properties"].(map[string]any)
	want := []string{
		"risk", "resilience", "readability", "reliability",
		LensRisk, LensResilience, LensReadability, LensReliability,
	}
	for name, raw := range map[string]any{
		"result":  properties["lens"].(map[string]any)["enum"],
		"finding": properties["findings"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)["lens"].(map[string]any)["enum"],
	} {
		payload, err := json.Marshal(raw)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		if err := json.Unmarshal(payload, &got); err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("%s lens enum = %v, want %v (%v)", name, got, want, err)
		}
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

// TestFindingIDPrefixForLensPublishesAdmissionMapping pins the exported
// lens-to-prefix mapping to the exact prefixes admission enforces, so START can
// publish the same namespace the published regex leaves ambiguous.
func TestFindingIDPrefixForLensPublishesAdmissionMapping(t *testing.T) {
	want := map[string]string{
		LensRisk:        "R1-",
		LensReadability: "R2-",
		LensReliability: "R3-",
		LensResilience:  "R4-",
	}
	for _, lens := range supportedLenses {
		if got := FindingIDPrefixForLens(lens); got != want[lens] {
			t.Fatalf("FindingIDPrefixForLens(%q) = %q, want %q", lens, got, want[lens])
		}
	}
	if got := FindingIDPrefixForLens("review-unknown"); got != "" {
		t.Fatalf("FindingIDPrefixForLens(unknown) = %q, want empty", got)
	}

	var document struct {
		Properties struct {
			Findings struct {
				Items struct {
					Properties struct {
						ID struct {
							Description string `json:"description"`
						} `json:"id"`
					} `json:"properties"`
				} `json:"items"`
			} `json:"findings"`
		} `json:"properties"`
	}
	if err := json.Unmarshal([]byte(ReviewerResultSchema), &document); err != nil {
		t.Fatal(err)
	}
	description := document.Properties.Findings.Items.Properties.ID.Description
	for lens, prefix := range want {
		if !strings.Contains(description, lens+"="+prefix) {
			t.Fatalf("schema finding id description %q does not publish %s=%s", description, lens, prefix)
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
