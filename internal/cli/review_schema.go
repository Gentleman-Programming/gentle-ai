package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const reviewReviewerSchema = `{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"https://gentle-ai.dev/schema/review/reviewer/v1","title":"Gentle AI reviewer result","type":"object","additionalProperties":false,"required":["subject_hash","inspection","findings","evidence"],"properties":{"subject_hash":{"type":"string","pattern":"^sha256:[0-9a-f]{64}$"},"inspection":{"type":"object","additionalProperties":false,"required":["status","paths"],"properties":{"status":{"const":"completed"},"paths":{"type":"array","uniqueItems":true,"items":{"type":"string","minLength":1}}}},"lens":{"type":"string","enum":["risk","resilience","readability","reliability","review-risk","review-resilience","review-readability","review-reliability"]},"findings":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["location","severity","claim","proof_refs"],"allOf":[{"if":{"properties":{"severity":{"enum":["BLOCKER","CRITICAL"]}},"required":["severity"]},"then":{"required":["evidence_class","causal_disposition"]}}],"properties":{"id":{"type":"string","pattern":"^R[1-4]-[A-Za-z0-9][A-Za-z0-9._-]*$"},"lens":{"type":"string","enum":["risk","resilience","readability","reliability"]},"location":{"type":"string","minLength":1},"severity":{"type":"string","enum":["BLOCKER","CRITICAL","WARNING","SUGGESTION"]},"claim":{"type":"string","minLength":1},"proof_refs":{"type":"array","minItems":1,"items":{"type":"string","pattern":"\\S","not":{"pattern":"^\\s*(?:[nN]/[aA]|[nN][aA]|[nN][oO][nN][eE]|[tT][oO][dD][oO]|[tT][bB][dD]|[pP][aA][sS][sS]|[pP][aA][sS][sS][eE][dD]|[sS][uU][cC][cC][eE][sS][sS]|[pP][lL][aA][cC][eE][hH][oO][lL][dD][eE][rR])\\s*$"}}},"evidence_class":{"type":"string","enum":["deterministic","inferential","insufficient"]},"causal_disposition":{"type":"string","enum":["introduced","behavior-activated","worsened","pre-existing","base-only","unknown"]}}}},"evidence":{"type":"array","minItems":1,"items":{"type":"string","pattern":"\\S","not":{"pattern":"^\\s*(?:[nN]/[aA]|[nN][aA]|[nN][oO][nN][eE]|[tT][oO][dD][oO]|[tT][bB][dD]|[pP][aA][sS][sS]|[pP][aA][sS][sS][eE][dD]|[sS][uU][cC][cC][eE][sS][sS]|[pP][lL][aA][cC][eE][hH][oO][lL][dD][eE][rR])\\s*$"}}}},"examples":[{"subject_hash":"sha256:0000000000000000000000000000000000000000000000000000000000000000","inspection":{"status":"completed","paths":["internal/example.go"]},"findings":[],"evidence":["reviewed the complete candidate scope"]}]}`

const (
	reviewVerificationEvidenceSchemaName = "gentle-ai.review-verification-evidence/v1"
	reviewVerificationEvidenceSchemaID   = "https://gentle-ai.dev/contracts/review-integration/v1/schemas/verification-evidence.schema.json"
	reviewVerificationPassed             = "passed"
	reviewVerificationFailed             = "failed"
)

const reviewVerificationEvidenceSchema = `{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"https://gentle-ai.dev/contracts/review-integration/v1/schemas/verification-evidence.schema.json","title":"Gentle AI final verification evidence","type":"object","additionalProperties":false,"required":["schema","outcome","checks"],"properties":{"schema":{"const":"gentle-ai.review-verification-evidence/v1"},"outcome":{"enum":["passed","failed"]},"checks":{"type":"array","minItems":1,"items":{"type":"object","additionalProperties":false,"required":["name","status","evidence"],"properties":{"name":{"type":"string","pattern":"\\S"},"status":{"enum":["passed","failed"]},"command":{"type":"string","pattern":"\\S"},"evidence":{"type":"array","minItems":1,"items":{"type":"string","pattern":"\\S"}}}}}},"examples":[{"schema":"gentle-ai.review-verification-evidence/v1","outcome":"passed","checks":[{"name":"focused tests","status":"passed","command":"go test ./internal/cli","evidence":["ok github.com/gentleman-programming/gentle-ai/internal/cli"]}]}]}`

type reviewVerificationEvidence struct {
	Schema  string                    `json:"schema"`
	Outcome string                    `json:"outcome"`
	Checks  []reviewVerificationCheck `json:"checks"`
}

type reviewVerificationCheck struct {
	Name     string   `json:"name"`
	Status   string   `json:"status"`
	Command  string   `json:"command,omitempty"`
	Evidence []string `json:"evidence"`
}

var reviewInputSchemas = map[string]json.RawMessage{
	"reviewer":                    json.RawMessage(reviewReviewerSchema),
	"verification-evidence":       json.RawMessage(reviewVerificationEvidenceSchema),
	"final-verification-incident": json.RawMessage(`{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"https://gentle-ai.dev/contracts/review-integration/v1/schemas/final-verification-incident.schema.json","title":"Gentle AI final-verification tooling incident","type":"object","additionalProperties":false,"required":["schema","class","lineage_id","terminal_revision","validating_revision","target_identity","failed_evidence_hash","finalize_request_digest"],"properties":{"schema":{"const":"gentle-ai.review-final-verification-incident/v1"},"class":{"const":"procedural_tooling_failure"},"lineage_id":{"type":"string","maxLength":128,"pattern":"^[a-z0-9]+(?:-[a-z0-9]+)*$"},"terminal_revision":{"$ref":"#/$defs/sha256"},"validating_revision":{"$ref":"#/$defs/sha256"},"target_identity":{"$ref":"#/$defs/sha256"},"failed_evidence_hash":{"$ref":"#/$defs/sha256"},"finalize_request_digest":{"$ref":"#/$defs/sha256"}},"$defs":{"sha256":{"type":"string","pattern":"^sha256:[0-9a-f]{64}$"}}}`),
	"refuter":                     json.RawMessage(`{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"https://gentle-ai.dev/schema/review/refuter/v1","title":"Gentle AI refuter result","type":"object","additionalProperties":false,"required":["results"],"properties":{"results":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["finding_id","outcome","proof_refs"],"properties":{"finding_id":{"type":"string"},"outcome":{"type":"string","enum":["corroborated","refuted","inconclusive"]},"proof_refs":{"type":"array","minItems":1,"items":{"type":"string","pattern":"\\S"}}}}}},"examples":[{"results":[]}]}`),
	"validator":                   json.RawMessage(`{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"https://gentle-ai.dev/schema/review/validator/v1","title":"Gentle AI targeted validator result","type":"object","additionalProperties":false,"required":["original_criteria","correction_regression","follow_ups"],"properties":{"original_criteria":{"$ref":"#/$defs/check"},"correction_regression":{"$ref":"#/$defs/check"},"follow_ups":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["observation","proof_refs"],"properties":{"observation":{"type":"string"},"proof_refs":{"type":"array","minItems":1,"items":{"type":"string","pattern":"\\S"}}}}}},"$defs":{"check":{"type":"object","additionalProperties":false,"required":["passed","evidence"],"properties":{"passed":{"type":"boolean"},"evidence":{"type":"array","minItems":1,"items":{"type":"string"}}}}},"examples":[{"original_criteria":{"passed":true,"evidence":["acceptance test passed"]},"correction_regression":{"passed":true,"evidence":["regression test passed"]},"follow_ups":[]}]}`),
}

func RunReviewSchema(args []string, stdout io.Writer) error {
	if len(args) != 1 {
		return errors.New("review schema requires exactly one of reviewer, refuter, validator, verification-evidence, or final-verification-incident")
	}
	document, ok := reviewInputSchemas[args[0]]
	if !ok {
		return fmt.Errorf("unknown review schema %q", args[0])
	}
	var value any
	if err := json.Unmarshal(document, &value); err != nil {
		return err
	}
	return encodeReviewJSON(stdout, value)
}

func parseReviewVerificationEvidence(payload []byte) (reviewVerificationEvidence, []byte, error) {
	var evidence reviewVerificationEvidence
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&evidence); err != nil {
		return evidence, nil, fmt.Errorf("decode final verification evidence: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return evidence, nil, errors.New("decode final verification evidence: trailing data")
	}
	if evidence.Schema != reviewVerificationEvidenceSchemaName || (evidence.Outcome != reviewVerificationPassed && evidence.Outcome != reviewVerificationFailed) || len(evidence.Checks) == 0 {
		return evidence, nil, errors.New("invalid final verification evidence identity, outcome, or checks")
	}
	failed := false
	for index, check := range evidence.Checks {
		if strings.TrimSpace(check.Name) == "" || strings.TrimSpace(check.Name) != check.Name ||
			(check.Status != reviewVerificationPassed && check.Status != reviewVerificationFailed) || len(check.Evidence) == 0 {
			return evidence, nil, fmt.Errorf("invalid final verification check %d", index+1)
		}
		if check.Command != "" && (strings.TrimSpace(check.Command) == "" || strings.TrimSpace(check.Command) != check.Command) {
			return evidence, nil, fmt.Errorf("invalid final verification check %d command", index+1)
		}
		for _, proof := range check.Evidence {
			if strings.TrimSpace(proof) == "" {
				return evidence, nil, fmt.Errorf("invalid final verification check %d evidence", index+1)
			}
		}
		failed = failed || check.Status == reviewVerificationFailed
	}
	if (evidence.Outcome == reviewVerificationFailed) != failed {
		return evidence, nil, errors.New("final verification outcome does not match check statuses")
	}
	canonical, err := json.Marshal(evidence)
	if err != nil {
		return evidence, nil, err
	}
	return evidence, append(canonical, '\n'), nil
}
