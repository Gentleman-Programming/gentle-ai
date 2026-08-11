package reviewtransaction

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// ReviewerResultSchema is the published input schema for one reviewer result.
// It lives beside AdmitArtifact on purpose: admission is the authority for the
// shape, and every consumer that describes that shape to a model -- the
// `gentle-ai review schema reviewer` command and the generated lens agent
// prompts -- derives its wording from this document instead of restating it.
// Three independent prose copies of this envelope are what let a lens agent
// emit findings/evidence with no subject_hash and no inspection (community
// report, PR #1801).
const ReviewerResultSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://gentle-ai.dev/schema/review/reviewer/v1",
  "title": "Gentle AI reviewer result",
  "type": "object",
  "additionalProperties": false,
  "required": ["subject_hash", "inspection", "findings", "evidence"],
  "properties": {
    "subject_hash": {"type": "string", "pattern": "^sha256:[0-9a-f]{64}$"},
    "inspection": {
      "type": "object",
      "additionalProperties": false,
      "required": ["status", "paths"],
      "properties": {
        "status": {"const": "completed"},
        "paths": {
          "type": "array",
          "description": "Complete unique unordered set of every changed_path_manifest.path.",
          "uniqueItems": true,
          "items": {"type": "string", "minLength": 1}
        }
      }
    },
    "lens": {
      "type": "string",
      "description": "Optional selected lens binding. Omission canonicalizes to the selected subject lens.",
      "enum": ["risk", "resilience", "readability", "reliability", "review-risk", "review-resilience", "review-readability", "review-reliability"]
    },
    "findings": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["location", "severity", "claim", "proof_refs"],
        "allOf": [{
          "if": {"properties": {"severity": {"enum": ["BLOCKER", "CRITICAL"]}}, "required": ["severity"]},
          "then": {"required": ["evidence_class", "causal_disposition"]}
        }],
        "properties": {
          "id": {"type": "string", "pattern": "^R[1-4]-[A-Za-z0-9][A-Za-z0-9._-]*$", "description": "Optional explicit ID; omit it to receive a native-assigned ID. When present it must carry the prefix bound to the selected lens, not the selection order: review-risk=R1-, review-readability=R2-, review-reliability=R3-, review-resilience=R4-."},
          "lens": {"type": "string", "enum": ["risk", "resilience", "readability", "reliability", "review-risk", "review-resilience", "review-readability", "review-reliability"]},
          "location": {"type": "string", "description": "One canonical repository-relative path:line or inclusive path:start-end span.", "pattern": "^.+:[1-9][0-9]*(?:-[1-9][0-9]*)?$"},
          "severity": {"type": "string", "enum": ["BLOCKER", "CRITICAL", "WARNING", "SUGGESTION"]},
          "claim": {"type": "string", "minLength": 1},
          "proof_refs": {
            "type": "array",
            "minItems": 1,
            "items": {
              "type": "string",
              "pattern": "\\S",
              "not": {"pattern": "^\\s*(?:[nN]/[aA]|[nN][aA]|[nN][oO][nN][eE]|[tT][oO][dD][oO]|[tT][bB][dD]|[pP][aA][sS][sS]|[pP][aA][sS][sS][eE][dD]|[sS][uU][cC][cC][eE][sS][sS]|[pP][lL][aA][cC][eE][hH][oO][lL][dD][eE][rR])\\s*$"}
            }
          },
          "evidence_class": {"type": "string", "enum": ["deterministic", "inferential", "insufficient"]},
          "causal_disposition": {"type": "string", "enum": ["introduced", "behavior-activated", "worsened", "pre-existing", "base-only", "unknown"]}
        }
      }
    },
    "evidence": {
      "type": "array",
      "minItems": 1,
      "items": {
        "type": "string",
        "pattern": "\\S",
        "not": {"pattern": "^\\s*(?:[nN]/[aA]|[nN][aA]|[nN][oO][nN][eE]|[tT][oO][dD][oO]|[tT][bB][dD]|[pP][aA][sS][sS]|[pP][aA][sS][sS][eE][dD]|[sS][uU][cC][cC][eE][sS][sS]|[pP][lL][aA][cC][eE][hH][oO][lL][dD][eE][rR])\\s*$"}
      }
    }
  },
  "examples": [{
    "subject_hash": "sha256:0000000000000000000000000000000000000000000000000000000000000000",
    "inspection": {"status": "completed", "paths": ["internal/example.go"]},
    "findings": [],
    "evidence": ["reviewed the complete candidate scope"]
  }]
}`

// ReviewerResultEnvelope is the machine-readable summary of what admission
// demands of a reviewer result, parsed out of ReviewerResultSchema. Callers
// that must describe the envelope in natural language build their wording from
// these values so a field added to admission cannot leave a prompt behind.
type ReviewerResultEnvelope struct {
	// RequiredTopLevelFields is the schema's own `required` list, in schema
	// order. A reviewer result that omits any of them is never admitted.
	RequiredTopLevelFields []string
	// CompletedInspectionStatus is the only inspection status admission
	// accepts as proof that the bound candidate was actually read.
	CompletedInspectionStatus string
	// LensAgentNames are the `review-*` lens identities the schema recognizes,
	// sorted. Each one names the lens agent whose prompt must state this
	// envelope.
	LensAgentNames []string
}

// ReviewerResult is the JSON result shape a reviewer submits before native
// admission binds it to a frozen candidate.
type ReviewerResult struct {
	SubjectHash string             `json:"subject_hash"`
	Inspection  ArtifactInspection `json:"inspection"`
	// Lens may be omitted or name the selected lens in either published form.
	Lens     string    `json:"lens,omitempty"`
	Findings []Finding `json:"findings"`
	Evidence []string  `json:"evidence"`
}

type reviewerResultShapeError struct {
	decision ArtifactAdmissionDecision
	message  string
}

func (err *reviewerResultShapeError) Error() string {
	return err.message
}

// ValidateReviewerResultBinding verifies the provider-owned binding that a
// reviewer result must echo. It does not inspect repository contents.
func ValidateReviewerResultBinding(subject ArtifactSubject, manifest []ChangedPathManifestEntry) error {
	if err := ValidateArtifactSubject(subject); err != nil {
		return err
	}
	manifestDigest, err := ChangedPathManifestDigest(manifest)
	if err != nil || manifestDigest != subject.ChangedPathManifestSHA256 {
		// refusal:by-design world-action: the caller must rebuild the subject and manifest from current frozen authority, not run a command
		return fmt.Errorf("frozen changed-path manifest does not match the artifact subject")
	}
	return nil
}

// ValidateReviewerResult strictly decodes one reviewer result and verifies the
// native schema, signed subject, selected lens, and complete frozen manifest.
// Repository-derived causality remains the responsibility of AdmitArtifact.
func ValidateReviewerResult(payload []byte, subject ArtifactSubject, manifest []ChangedPathManifestEntry) (ReviewerResult, error) {
	if err := ValidateReviewerResultBinding(subject, manifest); err != nil {
		return ReviewerResult{}, err
	}

	result, fields, err := decodeReviewerResult(payload)
	if err != nil {
		return ReviewerResult{}, err
	}
	if !requiredReviewerResultFields(fields) {
		// refusal:by-design world-action: the reviewer must resubmit a schema-conformant result, not run a command
		return ReviewerResult{}, fmt.Errorf("reviewer result does not match the required schema fields")
	}
	if result.SubjectHash != subject.SubjectHash {
		// refusal:by-design world-action: the reviewer must bind to the exact requested subject, not run a command
		return ReviewerResult{}, fmt.Errorf("reviewer result does not match the requested subject")
	}
	if result.Inspection.Status != ArtifactInspectionCompleted {
		// refusal:by-design world-action: the reviewer must complete inspection of the full frozen manifest, not run a command
		return ReviewerResult{}, fmt.Errorf("reviewer result does not report completed inspection of the frozen candidate")
	}
	if coverage, err := validateCompleteInspectionCoverage(result.Inspection.Paths, manifest); err != nil {
		if coverage != nil {
			return ReviewerResult{}, coverage
		}
		return ReviewerResult{}, fmt.Errorf("reviewer inspection paths are not canonical candidate paths") // refusal:by-design operator-knowledge: resubmit the exact complete unique manifest set from the binding
	}
	return canonicalizeReviewerResult(result, fields, subject.Lens)
}

// CanonicalizeReviewerResult shares lens-form canonicalization; authority admission remains caller-owned.
func CanonicalizeReviewerResult(payload []byte, expectedLens string) (ReviewerResult, error) {
	result, fields, err := decodeReviewerResult(payload)
	if err != nil {
		return ReviewerResult{}, err
	}
	return canonicalizeReviewerResult(result, fields, expectedLens)
}

func decodeReviewerResult(payload []byte) (ReviewerResult, map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		// refusal:by-design operator-knowledge: a reviewer must resubmit one schema-conformant JSON object
		return ReviewerResult{}, nil, fmt.Errorf("reviewer result does not match the required schema fields")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var result ReviewerResult
	if err := decoder.Decode(&result); err != nil {
		return ReviewerResult{}, nil, fmt.Errorf("decode reviewer result: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		// refusal:by-design world-action: the reviewer must return exactly one JSON object, not run a command
		return ReviewerResult{}, nil, fmt.Errorf("reviewer result must contain exactly one JSON object")
	}
	if result.Findings == nil || result.Evidence == nil {
		// refusal:by-design world-action: the reviewer must resubmit explicit findings and evidence arrays, not run a command
		return ReviewerResult{}, nil, fmt.Errorf("reviewer result requires explicit findings and evidence arrays")
	}
	return result, fields, nil
}

func canonicalizeReviewerResult(result ReviewerResult, fields map[string]json.RawMessage, expectedLens string) (ReviewerResult, error) {
	canonicalLens, err := canonicalReviewerLens(fields, expectedLens)
	if err != nil {
		return ReviewerResult{}, err
	}
	if err := validateExplicitReviewerFindingFields(fields["findings"]); err != nil {
		return ReviewerResult{}, err
	}
	canonical, err := canonicalReviewerResult(LensResult{
		Lens: canonicalLens, Findings: result.Findings, Evidence: result.Evidence,
	}, expectedLens)
	if err != nil {
		return ReviewerResult{}, err
	}
	result.Lens = canonical.Lens
	result.Findings = canonical.Findings
	result.Evidence = canonical.Evidence
	return result, nil
}

// canonicalReviewerLens is the shared omission rule for direct capture and
// advisory validation. It deliberately checks presence before canonicalizing:
// an explicit empty or malformed value is never equivalent to an omission.
func canonicalReviewerLens(fields map[string]json.RawMessage, expectedLens string) (string, error) {
	raw, present := fields["lens"]
	if !present {
		return expectedLens, nil
	}
	var lens string
	if err := json.Unmarshal(raw, &lens); err != nil || (lens != expectedLens && lens != strings.TrimPrefix(expectedLens, "review-")) {
		// refusal:by-design world-action: an explicit reviewer binding must match the selected subject exactly
		return "", fmt.Errorf("reviewer result does not report the required selected-lens binding")
	}
	return expectedLens, nil
}

func validateExplicitReviewerFindingFields(raw json.RawMessage) error {
	var findings []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &findings); err != nil {
		// refusal:by-design operator-knowledge: a reviewer must resubmit a schema-conformant findings array
		return fmt.Errorf("reviewer result findings do not match the required schema")
	}
	for index, fields := range findings {
		if rawLens, present := fields["lens"]; present {
			var lens string
			if err := json.Unmarshal(rawLens, &lens); err != nil || !isPublishedReviewerFindingLens(lens) {
				// refusal:by-design operator-knowledge: an explicit finding lens must use one published form or be omitted for canonical assignment
				return fmt.Errorf("reviewer result finding[%d] has an invalid explicit lens", index)
			}
		}
	}
	return nil
}

func isPublishedReviewerFindingLens(lens string) bool {
	return isSupportedLens(lens) || isSupportedLens("review-"+lens)
}

func requiredReviewerResultFields(fields map[string]json.RawMessage) bool {
	for _, name := range []string{"subject_hash", "inspection", "findings", "evidence"} {
		if _, found := fields[name]; !found {
			return false
		}
	}
	return true
}

// findingIDPrefixByLens is the single authoritative lens-to-finding-ID-prefix
// mapping: admission enforces it and START publishes it, so the two cannot
// drift apart.
var findingIDPrefixByLens = map[string]string{
	LensRisk: "R1-", LensReadability: "R2-", LensReliability: "R3-", LensResilience: "R4-",
}

// FindingIDPrefixForLens returns the prefix an explicit finding ID must carry
// to be admitted for the given lens, or "" for an unsupported lens. The
// published reviewer schema regex admits any R[1-4]- prefix, so this mapping
// is the machine-readable source of the per-lens namespace.
func FindingIDPrefixForLens(lens string) string {
	return findingIDPrefixByLens[lens]
}

// canonicalReviewerResult contains the result-shape checks shared by native
// admission and read-only advisory transport validation.
func canonicalReviewerResult(result LensResult, expectedLens string) (LensResult, error) {
	canonical, err := CanonicalCompactLensResult(result)
	if err != nil {
		return LensResult{}, err
	}
	if canonical.Lens != expectedLens {
		return LensResult{}, &reviewerResultShapeError{
			decision: ArtifactAdmissionBindingMismatch,
			message:  "reviewer result is not bound to the selected lens",
		}
	}
	wantPrefix := FindingIDPrefixForLens(canonical.Lens)
	for _, finding := range canonical.Findings {
		if !artifactFindingID.MatchString(finding.ID) {
			return LensResult{}, &reviewerResultShapeError{
				decision: ArtifactAdmissionBindingMismatch,
				message:  "reviewer finding ID does not match the native ASCII schema",
			}
		}
		if !strings.HasPrefix(finding.ID, wantPrefix) {
			return LensResult{}, &reviewerResultShapeError{
				decision: ArtifactAdmissionBindingMismatch,
				message:  fmt.Sprintf("reviewer finding ID is not bound to the selected lens: expected_prefix=%s received_id=%s", wantPrefix, finding.ID),
			}
		}
		if isSevereSeverity(finding.Severity) && (!isSupportedEvidenceClass(finding.EvidenceClass) || !isSupportedCausalDisposition(finding.CausalDisposition)) {
			return LensResult{}, &reviewerResultShapeError{
				decision: ArtifactAdmissionIncomplete,
				message:  "severe reviewer finding requires supported evidence_class and causal_disposition",
			}
		}
	}
	return canonical, nil
}

// NewReviewerResultEnvelope derives the envelope from the published schema.
// It never fails: the schema is a compile-time constant in this package and is
// covered by the package's own tests.
func NewReviewerResultEnvelope() ReviewerResultEnvelope {
	var document struct {
		Required   []string `json:"required"`
		Properties struct {
			Lens struct {
				Enum []string `json:"enum"`
			} `json:"lens"`
		} `json:"properties"`
	}
	_ = json.Unmarshal([]byte(ReviewerResultSchema), &document)
	lenses := make([]string, 0, len(document.Properties.Lens.Enum))
	for _, value := range document.Properties.Lens.Enum {
		if strings.HasPrefix(value, "review-") {
			lenses = append(lenses, value)
		}
	}
	sort.Strings(lenses)
	return ReviewerResultEnvelope{
		RequiredTopLevelFields:    append([]string(nil), document.Required...),
		CompletedInspectionStatus: string(ArtifactInspectionCompleted),
		LensAgentNames:            lenses,
	}
}
