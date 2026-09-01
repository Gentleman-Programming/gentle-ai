package reviewerprovider

import (
	"fmt"
	"slices"
)

// Role is a compiled provider-contract role. Hosts receive only an opaque
// Invocation; they cannot select a role, schema, capability, or storage slot.
type Role string

const (
	RoleLens              Role = "lens"
	RoleRefuter           Role = "refuter"
	RoleTargetedValidator Role = "targeted-validator"
)

const TransportCapability = "gentle-ai.provider-transport/v1"

const (
	LensResultSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://gentle-ai.dev/schema/review/reviewer/v1",
  "title": "Gentle AI reviewer result",
  "type": "object",
  "additionalProperties": false,
  "required": ["subject_hash", "inspection", "findings", "evidence"],
  "properties": {
    "subject_hash": {"type": "string", "pattern": "^sha256:[0-9a-f]{64}$"},
    "inspection": {"type": "object", "additionalProperties": false, "required": ["status", "paths"], "properties": {"status": {"const": "completed"}, "paths": {"type": "array", "description": "Complete unique unordered set of every changed_path_manifest.path.", "uniqueItems": true, "items": {"type": "string", "minLength": 1}}}},
    "lens": {"type": "string", "description": "Optional selected lens binding. Omission canonicalizes to the selected subject lens.", "enum": ["risk", "resilience", "readability", "reliability", "review-risk", "review-resilience", "review-readability", "review-reliability"]},
    "findings": {"type": "array", "items": {"type": "object", "additionalProperties": false, "required": ["location", "severity", "claim", "proof_refs"], "allOf": [{"if": {"properties": {"severity": {"enum": ["BLOCKER", "CRITICAL"]}}, "required": ["severity"]}, "then": {"required": ["evidence_class", "causal_disposition"]}}], "properties": {"id": {"type": "string", "pattern": "^R[1-4]-[A-Za-z0-9][A-Za-z0-9._-]*$", "description": "Optional explicit ID; omit it to receive a native-assigned ID. When present it must carry the prefix bound to the selected lens, not the selection order: review-risk=R1-, review-readability=R2-, review-reliability=R3-, review-resilience=R4-."}, "lens": {"type": "string", "enum": ["risk", "resilience", "readability", "reliability", "review-risk", "review-resilience", "review-readability", "review-reliability"]}, "location": {"type": "string", "description": "One canonical repository-relative path:line or inclusive path:start-end span.", "pattern": "^.+:[1-9][0-9]*(?:-[1-9][0-9]*)?$"}, "severity": {"type": "string", "enum": ["BLOCKER", "CRITICAL", "WARNING", "SUGGESTION"]}, "claim": {"type": "string", "minLength": 1}, "proof_refs": {"type": "array", "minItems": 1, "items": {"type": "string", "pattern": "\\S", "not": {"pattern": "^\\s*(?:[nN]/[aA]|[nN][aA]|[nN][oO][nN][eE]|[tT][oO][dD][oO]|[tT][bB][dD]|[pP][aA][sS][sS]|[pP][aA][sS][sS][eE][dD]|[sS][uU][cC][cC][eE][sS][sS]|[pP][lL][aA][cC][eE][hH][oO][lL][dD][eE][rR])\\s*$"}}}, "evidence_class": {"type": "string", "enum": ["deterministic", "inferential", "insufficient"]}, "causal_disposition": {"type": "string", "enum": ["introduced", "behavior-activated", "worsened", "pre-existing", "base-only", "unknown"]}}}},
    "evidence": {"type": "array", "minItems": 1, "items": {"type": "string", "pattern": "\\S", "not": {"pattern": "^\\s*(?:[nN]/[aA]|[nN][aA]|[nN][oO][nN][eE]|[tT][oO][dD][oO]|[tT][bB][dD]|[pP][aA][sS][sS]|[pP][aA][sS][sS][eE][dD]|[sS][uU][cC][cC][eE][sS][sS]|[pP][lL][aA][cC][eE][hH][oO][lL][dD][eE][rR])\\s*$"}}}
  },
  "examples": [{"subject_hash": "sha256:0000000000000000000000000000000000000000000000000000000000000000", "inspection": {"status": "completed", "paths": ["internal/example.go"]}, "findings": [], "evidence": ["reviewed the complete candidate scope"]}]
}`
	RefuterResultSchema           = `{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"https://gentle-ai.dev/schema/review/refuter/v1","title":"Gentle AI refuter result","type":"object","additionalProperties":false,"required":["refuter_request_hash","results"],"properties":{"refuter_request_hash":{"$ref":"#/$defs/sha256"},"results":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["finding_id","outcome","proof_refs"],"properties":{"finding_id":{"type":"string"},"outcome":{"type":"string","enum":["corroborated","refuted","inconclusive"]},"proof_refs":{"type":"array","minItems":1,"items":{"type":"string","pattern":"\\S"}}}}}},"$defs":{"sha256":{"type":"string","pattern":"^sha256:[0-9a-f]{64}$"}},"examples":[{"refuter_request_hash":"sha256:0000000000000000000000000000000000000000000000000000000000000000","results":[]}]}`
	TargetedValidatorResultSchema = `{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"https://gentle-ai.dev/schema/review/validator/v1","title":"Gentle AI targeted validator result","type":"object","additionalProperties":false,"required":["targeted_validation_request_hash","correction_target_identity","original_criteria","correction_regression","follow_ups"],"properties":{"targeted_validation_request_hash":{"$ref":"#/$defs/sha256"},"correction_target_identity":{"$ref":"#/$defs/sha256"},"original_criteria":{"$ref":"#/$defs/check"},"correction_regression":{"$ref":"#/$defs/check"},"follow_ups":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["observation","proof_refs"],"properties":{"observation":{"type":"string"},"proof_refs":{"type":"array","minItems":1,"items":{"type":"string","pattern":"\\S"}}}}}},"$defs":{"sha256":{"type":"string","pattern":"^sha256:[0-9a-f]{64}$"},"check":{"type":"object","additionalProperties":false,"required":["passed","evidence"],"properties":{"passed":{"type":"boolean"},"evidence":{"type":"array","minItems":1,"items":{"type":"string"}}}}},"examples":[{"targeted_validation_request_hash":"sha256:0000000000000000000000000000000000000000000000000000000000000000","correction_target_identity":"sha256:1111111111111111111111111111111111111111111111111111111111111111","original_criteria":{"passed":true,"evidence":["acceptance test passed"]},"correction_regression":{"passed":true,"evidence":["regression test passed"]},"follow_ups":[]}]}`
)

// targetedValidatorPromptInstruction is the targeted validator's complete
// briefing. Unlike a reviewing lens, this role may hold live tools on some
// runtimes, so the briefing must say which immutable-inspection command exists
// and where each of its arguments comes from. Withholding that recipe while
// inviting an "I could not inspect it" answer is what produced #3380: one
// runtime spent the lineage's single correction attempt on a non-observation
// and another stranded its lineage with no verdict at all.
const targetedValidatorPromptInstruction = "You are the read-only targeted fix validator. " +
	"Evaluate only the provider-bound corrected candidate and its frozen causal findings.\n\n" +
	"Inspecting the immutable candidate. The `evidence` array already carries the complete frozen tree-to-tree patch " +
	"for every path in `validation_request.correction_paths`. It is authoritative corrected-candidate content read " +
	"from the immutable trees, not a summary of them, so a verdict reached from it is a verified verdict. " +
	"When you can run commands, read those same immutable trees yourself with " +
	"`gentle-ai review inspect-candidate --purpose targeted-validation " +
	"--lineage <validation_request.lineage_id> " +
	"--expected-revision <validation_request.expected_revision> " +
	"--target <validation_request.correction_target_identity> " +
	"--request-hash <validation_request.request_hash> " +
	"--repository-context <repository_context> " +
	"--operation <name-status|numstat|stat|patch|object>`. " +
	"Every value comes from this input and nowhere else. " +
	"`stat` and `patch` also take `--path-index <n>`, the zero-based index into `validation_request.correction_paths`; " +
	"`object` takes that same `--path-index` plus `--side base|candidate`. Never pass `--lens` or `--order`. " +
	"That command is the only sanctioned route to the frozen trees: never read the live worktree, index, or HEAD, " +
	"and never reach the repository through any other command.\n\n" +
	"Reporting that the candidate could not be inspected is a last resort, never a first response. " +
	"It is admissible only after the supplied evidence, and that command where you can run it, have both failed to " +
	"answer. Never record an inconclusive inspection as a failed check: a failed check spends the correction budget " +
	"on something you did not observe.\n\n" +
	"Return exactly one JSON object with no prose. " +
	"Validate your result against the supplied output schema. " +
	"Echo targeted_validation_request_hash and correction_target_identity from the input. " +
	"Include original_criteria and correction_regression, each with a boolean passed and non-empty evidence. " +
	"Always emit follow_ups; use [] when none exist. " +
	"Native Go alone decides correction accounting, receipts, and delivery gates."

// Contract is the sole role authority for schema serving, capability reporting,
// storage routing, prompt instruction, and raw-output limits.
type Contract struct {
	ID                   string
	Role                 Role
	RequestSchemaID      string
	ResultSchemaID       string
	ResultSchema         []byte
	StorageSlot          string
	RequiredCapabilities []string
	PromptInstruction    string
	ResultLimit          int
}

var contracts = []Contract{
	{
		ID: string(RoleLens), Role: RoleLens, RequestSchemaID: "gentle-ai.review-lens-context/v1",
		ResultSchemaID: "https://gentle-ai.dev/schema/review/reviewer/v1", ResultSchema: []byte(LensResultSchema), StorageSlot: "selected-lens",
		RequiredCapabilities: []string{TransportCapability}, ResultLimit: 4 << 20,
	},
	{
		ID: string(RoleRefuter), Role: RoleRefuter, RequestSchemaID: "gentle-ai.review-provider-refuter-request/v1",
		ResultSchemaID: "https://gentle-ai.dev/schema/review/refuter/v1", ResultSchema: []byte(RefuterResultSchema), StorageSlot: "transaction-refuter-batch",
		RequiredCapabilities: []string{TransportCapability}, ResultLimit: 4 << 20,
		PromptInstruction: "You are the detached read-only refuter for exactly ONE transaction-wide inferential batch. Return exactly one corroborated, refuted, or inconclusive outcome for every supplied claim. Add no findings, modify nothing, and return exactly one JSON object with no prose. Native Go alone applies the result to RDD authority.",
	},
	{
		ID: string(RoleTargetedValidator), Role: RoleTargetedValidator, RequestSchemaID: "gentle-ai.review-targeted-validation-request/v1",
		ResultSchemaID: "https://gentle-ai.dev/schema/review/validator/v1", ResultSchema: []byte(TargetedValidatorResultSchema), StorageSlot: "correction-targeted-validator",
		RequiredCapabilities: []string{TransportCapability}, ResultLimit: 4 << 20,
		PromptInstruction: targetedValidatorPromptInstruction,
	},
}

func Contracts() []Contract {
	result := make([]Contract, len(contracts))
	for index, contract := range contracts {
		contract.ResultSchema = append([]byte(nil), contract.ResultSchema...)
		contract.RequiredCapabilities = slices.Clone(contract.RequiredCapabilities)
		result[index] = contract
	}
	return result
}

func ContractFor(role Role) (Contract, error) {
	for _, contract := range Contracts() {
		if contract.Role == role {
			return contract, nil
		}
	}
	return Contract{}, fmt.Errorf("unsupported reviewer provider role %q", role)
}
