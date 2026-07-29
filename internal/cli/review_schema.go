package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// reviewReviewerSchema is served verbatim from the admission package. The
// schema is defined next to AdmitArtifact so this command, the generated lens
// agent prompts, and the admission logic cannot describe different envelopes.
const reviewReviewerSchema = reviewtransaction.ReviewerResultSchema

// reviewVerificationEvidenceSchema describes the input review capture-evidence
// actually accepts and readCapturedFinalEvidence actually enforces: raw,
// non-empty final test/verification evidence content, not a structured JSON
// object, bounded by the same native artifact limit every captured artifact
// uses (reviewResultArtifactLimit).
var reviewVerificationEvidenceSchema = fmt.Sprintf(
	`{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"https://gentle-ai.dev/schema/review/verification-evidence/v1","title":"Gentle AI captured final verification evidence","description":"Raw final test or verification evidence content captured by review capture-evidence. It is not a structured JSON document: any non-empty content up to the native artifact bound is accepted.","type":"string","minLength":1,"maxLength":%d}`,
	reviewResultArtifactLimit,
)

const reviewVerificationEvidenceRecordSchema = `{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"https://gentle-ai.dev/contracts/review-integration/v1/schemas/verification-evidence.schema.json","title":"Gentle AI outcome-bearing verification evidence record","type":"object","additionalProperties":false,"required":["schema","version","lineage_id","authority_revision","target_identity","candidate_tree","paths_digest","paths","ledger_ids","raw_payload_sha256","raw_payload_bytes","outcome","record_digest"],"properties":{"schema":{"const":"gentle-ai.review-verification-evidence/v2"},"version":{"const":2},"lineage_id":{"type":"string","maxLength":128,"pattern":"^[a-z0-9]+(?:-[a-z0-9]+)*$"},"authority_revision":{"$ref":"#/$defs/sha256"},"target_identity":{"$ref":"#/$defs/sha256"},"candidate_tree":{"type":"string","pattern":"^(?:[0-9a-f]{40}|[0-9a-f]{64})$"},"paths_digest":{"$ref":"#/$defs/sha256"},"paths":{"type":"array","uniqueItems":true,"items":{"type":"string","minLength":1}},"ledger_ids":{"type":"array","uniqueItems":true,"items":{"type":"string","minLength":1}},"raw_payload_sha256":{"$ref":"#/$defs/sha256"},"raw_payload_bytes":{"type":"integer","minimum":1,"maximum":4194304},"outcome":{"enum":["passed","verification_failed","procedural_tooling_failed"]},"record_digest":{"$ref":"#/$defs/sha256"}},"$defs":{"sha256":{"type":"string","pattern":"^sha256:[0-9a-f]{64}$"}}}`

var reviewInputSchemas = map[string]json.RawMessage{
	"reviewer":                     json.RawMessage(reviewReviewerSchema),
	"verification-evidence":        json.RawMessage(reviewVerificationEvidenceSchema),
	"verification-evidence-record": json.RawMessage(reviewVerificationEvidenceRecordSchema),
	"final-verification-incident":  json.RawMessage(`{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"https://gentle-ai.dev/contracts/review-integration/v1/schemas/final-verification-incident.schema.json","title":"Gentle AI final-verification tooling incident","type":"object","additionalProperties":false,"required":["schema","class","lineage_id","terminal_revision","validating_revision","target_identity","failed_evidence_hash","finalize_request_digest"],"properties":{"schema":{"const":"gentle-ai.review-final-verification-incident/v1"},"class":{"const":"procedural_tooling_failure"},"lineage_id":{"type":"string","maxLength":128,"pattern":"^[a-z0-9]+(?:-[a-z0-9]+)*$"},"terminal_revision":{"$ref":"#/$defs/sha256"},"validating_revision":{"$ref":"#/$defs/sha256"},"target_identity":{"$ref":"#/$defs/sha256"},"failed_evidence_hash":{"$ref":"#/$defs/sha256"},"finalize_request_digest":{"$ref":"#/$defs/sha256"}},"$defs":{"sha256":{"type":"string","pattern":"^sha256:[0-9a-f]{64}$"}}}`),
	"refuter":                      json.RawMessage(`{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"https://gentle-ai.dev/schema/review/refuter/v1","title":"Gentle AI refuter result","type":"object","additionalProperties":false,"required":["results"],"properties":{"results":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["finding_id","outcome","proof_refs"],"properties":{"finding_id":{"type":"string"},"outcome":{"type":"string","enum":["corroborated","refuted","inconclusive"]},"proof_refs":{"type":"array","minItems":1,"items":{"type":"string","pattern":"\\S"}}}}}},"examples":[{"results":[]}]}`),
	"validator":                    json.RawMessage(`{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"https://gentle-ai.dev/schema/review/validator/v1","title":"Gentle AI targeted validator result","type":"object","additionalProperties":false,"required":["targeted_validation_request_hash","correction_target_identity","original_criteria","correction_regression","follow_ups"],"properties":{"targeted_validation_request_hash":{"$ref":"#/$defs/sha256"},"correction_target_identity":{"$ref":"#/$defs/sha256"},"original_criteria":{"$ref":"#/$defs/check"},"correction_regression":{"$ref":"#/$defs/check"},"follow_ups":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["observation","proof_refs"],"properties":{"observation":{"type":"string"},"proof_refs":{"type":"array","minItems":1,"items":{"type":"string","pattern":"\\S"}}}}}},"$defs":{"sha256":{"type":"string","pattern":"^sha256:[0-9a-f]{64}$"},"check":{"type":"object","additionalProperties":false,"required":["passed","evidence"],"properties":{"passed":{"type":"boolean"},"evidence":{"type":"array","minItems":1,"items":{"type":"string"}}}}},"examples":[{"targeted_validation_request_hash":"sha256:0000000000000000000000000000000000000000000000000000000000000000","correction_target_identity":"sha256:1111111111111111111111111111111111111111111111111111111111111111","original_criteria":{"passed":true,"evidence":["acceptance test passed"]},"correction_regression":{"passed":true,"evidence":["regression test passed"]},"follow_ups":[]}]}`),
}

// reviewSchemaNames lists the accepted values in a stable order, derived from
// the map that actually serves them so the two can never drift. Every refusal
// names this list: capture-result now points readers here for the payload it
// demands, and arriving at a refusal that names no accepted value would put
// them back where they started.
func reviewSchemaNames() string {
	names := make([]string, 0, len(reviewInputSchemas))
	for name := range reviewInputSchemas {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func RunReviewSchema(args []string, stdout io.Writer) error {
	if len(args) != 1 {
		return fmt.Errorf("review schema requires exactly one of: %s", reviewSchemaNames())
	}
	if args[0] == "--help" || args[0] == "-h" {
		_, err := fmt.Fprintf(stdout, "Usage: gentle-ai review schema <name>\n\nEmit one input schema, with a working example where the schema carries one.\n\nAccepted names: %s\n", reviewSchemaNames())
		return err
	}
	document, ok := reviewInputSchemas[args[0]]
	if !ok {
		return fmt.Errorf("unknown review schema %q; accepted names are: %s", args[0], reviewSchemaNames())
	}
	var value any
	if err := json.Unmarshal(document, &value); err != nil {
		return err
	}
	return encodeReviewJSON(stdout, value)
}
