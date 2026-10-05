package reviewerprovider

import (
	"fmt"
	"slices"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
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
    "inspection": {"type": "object", "additionalProperties": false, "required": ["status", "paths"], "allOf": [{"if": {"properties": {"status": {"const": "unavailable"}}, "required": ["status"]}, "then": {"required": ["reason"]}}], "properties": {"status": {"type": "string", "enum": ["completed", "unavailable"], "description": "\"completed\" asserts every changed_path_manifest path was actually inspected. \"unavailable\" asserts the candidate could not be inspected at all and requires a non-empty reason; this is the typed admission-completeness signal; evidence prose is not a substitute for it."}, "paths": {"type": "array", "description": "Complete unique unordered set of every changed_path_manifest.path when status is \"completed\"; empty when status is \"unavailable\".", "uniqueItems": true, "items": {"type": "string", "minLength": 1}}, "reason": {"type": "string", "minLength": 1, "description": "Required and non-empty only when status is \"unavailable\": why the candidate could not be inspected."}}},
    "lens": {"type": "string", "description": "Optional selected lens binding. Omission canonicalizes to the selected subject lens.", "enum": ["risk", "resilience", "readability", "reliability", "review-risk", "review-resilience", "review-readability", "review-reliability"]},
    "findings": {"type": "array", "items": {"type": "object", "additionalProperties": false, "required": ["location", "severity", "claim", "proof_refs"], "allOf": [{"if": {"properties": {"severity": {"enum": ["BLOCKER", "CRITICAL"]}}, "required": ["severity"]}, "then": {"required": ["evidence_class", "causal_disposition"]}}], "properties": {"id": {"type": "string", "pattern": "^R[1-4]-[A-Za-z0-9][A-Za-z0-9._-]*$", "description": "Optional explicit ID; omit it to receive a native-assigned ID. When present it must carry the prefix bound to the selected lens, not the selection order: review-risk=R1-, review-readability=R2-, review-reliability=R3-, review-resilience=R4-."}, "lens": {"type": "string", "enum": ["risk", "resilience", "readability", "reliability", "review-risk", "review-resilience", "review-readability", "review-reliability"]}, "location": {"type": "string", "description": "One canonical repository-relative path:line or inclusive path:start-end span.", "pattern": "^.+:[1-9][0-9]*(?:-[1-9][0-9]*)?$"}, "severity": {"type": "string", "enum": ["BLOCKER", "CRITICAL", "WARNING", "SUGGESTION"]}, "claim": {"type": "string", "minLength": 1}, "proof_refs": {"type": "array", "minItems": 1, "items": {"type": "string", "pattern": "\\S", "not": {"pattern": "^\\s*(?:[nN]/[aA]|[nN][aA]|[nN][oO][nN][eE]|[tT][oO][dD][oO]|[tT][bB][dD]|[pP][aA][sS][sS]|[pP][aA][sS][sS][eE][dD]|[sS][uU][cC][cC][eE][sS][sS]|[pP][lL][aA][cC][eE][hH][oO][lL][dD][eE][rR])\\s*$"}}}, "evidence_class": {"type": "string", "enum": ["deterministic", "inferential", "insufficient"]}, "causal_disposition": {"type": "string", "enum": ["introduced", "behavior-activated", "worsened", "pre-existing", "base-only", "unknown"]}}}},
    "evidence": {"type": "array", "minItems": 1, "items": {"type": "string", "pattern": "\\S", "not": {"pattern": "^\\s*(?:[nN]/[aA]|[nN][aA]|[nN][oO][nN][eE]|[tT][oO][dD][oO]|[tT][bB][dD]|[pP][aA][sS][sS]|[pP][aA][sS][sS][eE][dD]|[sS][uU][cC][cC][eE][sS][sS]|[pP][lL][aA][cC][eE][hH][oO][lL][dD][eE][rR])\\s*$"}}}
  },
  "examples": [{"subject_hash": "sha256:0000000000000000000000000000000000000000000000000000000000000000", "inspection": {"status": "completed", "paths": ["internal/example.go"]}, "findings": [], "evidence": ["reviewed the complete candidate scope"]}, {"subject_hash": "sha256:0000000000000000000000000000000000000000000000000000000000000000", "inspection": {"status": "unavailable", "paths": [], "reason": "the immutable inspection command timed out before any path could be read"}, "findings": [], "evidence": ["inspection was unavailable; see inspection.reason"]}]
}`
	RefuterResultSchema           = `{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"https://gentle-ai.dev/schema/review/refuter/v1","title":"Gentle AI refuter result","type":"object","additionalProperties":false,"required":["refuter_request_hash","results"],"properties":{"refuter_request_hash":{"$ref":"#/$defs/sha256"},"results":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["finding_id","outcome","proof_refs"],"properties":{"finding_id":{"type":"string"},"outcome":{"type":"string","enum":["corroborated","refuted","inconclusive"]},"proof_refs":{"type":"array","minItems":1,"items":{"type":"string","pattern":"\\S"}}}}}},"$defs":{"sha256":{"type":"string","pattern":"^sha256:[0-9a-f]{64}$"}},"examples":[{"refuter_request_hash":"sha256:0000000000000000000000000000000000000000000000000000000000000000","results":[]}]}`
	TargetedValidatorResultSchema = `{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"https://gentle-ai.dev/schema/review/validator/v1","title":"Gentle AI targeted validator result","type":"object","additionalProperties":false,"required":["targeted_validation_request_hash","correction_target_identity","original_criteria","correction_regression","follow_ups"],"properties":{"targeted_validation_request_hash":{"$ref":"#/$defs/sha256"},"correction_target_identity":{"$ref":"#/$defs/sha256"},"original_criteria":{"$ref":"#/$defs/check"},"correction_regression":{"$ref":"#/$defs/check"},"follow_ups":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["observation","proof_refs"],"properties":{"observation":{"type":"string"},"proof_refs":{"type":"array","minItems":1,"items":{"type":"string","pattern":"\\S"}}}}}},"allOf":[{"if":{"properties":{"correction_regression":{"type":"object","properties":{"passed":{"const":false}},"required":["passed"]}},"required":["correction_regression"]},"then":{"properties":{"correction_regression":{"type":"object","properties":{"regressions":{"minItems":1}},"required":["regressions"]}}}}],"$defs":{"sha256":{"type":"string","pattern":"^sha256:[0-9a-f]{64}$"},"check":{"type":"object","additionalProperties":false,"required":["passed","evidence"],"properties":{"passed":{"type":"boolean","description":"true means the named check passed; false means the named check failed."},"evidence":{"type":"array","minItems":1,"items":{"type":"string"}},"regressions":{"type":"array","items":{"$ref":"#/$defs/regression"},"description":"Required with at least one entry when this is correction_regression and passed is false: one entry per observed regression, omitted or empty otherwise."},"inspection":{"$ref":"#/$defs/inspection"}},"allOf":[{"if":{"properties":{"inspection":{"type":"object","properties":{"status":{"const":"unavailable"}},"required":["status"]}},"required":["inspection"]},"then":{"properties":{"inspection":{"type":"object","required":["reason"]}}}}]},"inspection":{"type":"object","additionalProperties":false,"required":["status"],"properties":{"status":{"type":"string","enum":["completed","unavailable"],"description":"completed means this check's verdict came from actually reading the frozen candidate trees. unavailable means it did not, and this check produced no verdict."},"reason":{"type":"string","minLength":1,"description":"Required when status is unavailable: why the frozen candidate trees could not be read."}},"description":"Optional. Omit this field entirely when inspection completed normally -- every check that predates this field already assumed that default. Never infer unavailable from evidence wording; only this typed field marks a check inconclusive."},"regression":{"type":"object","additionalProperties":false,"required":["location","claim","proof_refs"],"properties":{"id":{"type":"string","description":"Optional explicit ID; omit it to receive a native-assigned ID."},"location":{"type":"string","description":"One canonical repository-relative path:line or inclusive path:start-end span.","pattern":"^.+:[1-9][0-9]*(?:-[1-9][0-9]*)?$"},"claim":{"type":"string","minLength":1},"proof_refs":{"type":"array","minItems":1,"items":{"type":"string","pattern":"\\S"}}}}},"examples":[{"targeted_validation_request_hash":"sha256:0000000000000000000000000000000000000000000000000000000000000000","correction_target_identity":"sha256:1111111111111111111111111111111111111111111111111111111111111111","original_criteria":{"passed":true,"evidence":["acceptance test passed"]},"correction_regression":{"passed":true,"evidence":["regression test passed"]},"follow_ups":[]}]}`
)

// targetedValidatorPromptInstruction is the targeted validator's complete
// briefing. Unlike a reviewing lens, this role may hold live tools on some
// runtimes, so the briefing must say which immutable-inspection command exists
// and where each of its arguments comes from. Withholding that recipe while
// inviting an "I could not inspect it" answer is what produced #3380: one
// runtime spent the lineage's single correction attempt on a non-observation
// and another stranded its lineage with no verdict at all.
//
// It also tells the validator to report an unreachable candidate through the
// typed check.inspection field, never through evidence wording (issue #4266):
// before this field existed, admission keyword-scanned each check's evidence
// for phrases like "could not be inspected", so a validator that faithfully
// quoted the candidate's own strings, or that described a feature about
// unreadable candidates, was misjudged as unable to read the candidate on
// every retry.
//
// The evidence paragraph distinguishes authored from generated paths because
// reviewProviderMaterializeEvidence hands this role the same representation a
// lens receives: a generated path arrives as an immutable metadata summary
// with its content hunks omitted. A briefing that still promised the complete
// patch for every path would have this role read a verified verdict out of a
// summary it was told was not one -- a signed verdict on bytes it never saw.
// A lens can be told that what it cannot see is not evidence; this role cannot,
// because a correction that touches a lockfile or a golden is exactly what it
// must judge. So the briefing routes that case to inspect-candidate, and to a
// typed unavailable check when no command is available.
const targetedValidatorPromptInstruction = "You are the read-only targeted fix validator. " +
	"Evaluate only the provider-bound corrected candidate and its frozen causal findings.\n\n" +
	"Inspecting the immutable candidate. The `evidence` array carries frozen tree-to-tree evidence for every path " +
	"in `validation_request.correction_paths`, in exactly the representation a reviewing lens receives. An authored " +
	"path carries its complete patch: authoritative corrected-candidate content read from the immutable trees, not " +
	"a summary of them, so a verdict reached from it is a verified verdict. A generated path instead carries an " +
	"immutable metadata summary marked `\"generated\": true` and `\"content_omitted\": true`. Its content hunks are " +
	"not in this input, so the summary alone never verifies a claim about what those hunks say. " +
	"When you can run commands, read those same immutable trees yourself with " +
	"`gentle-ai review inspect-candidate --purpose targeted-validation " +
	"--lineage <validation_request.lineage_id> " +
	"--expected-revision <validation_request.expected_revision> " +
	"--target <validation_request.correction_target_identity> " +
	"--request-hash <validation_request.request_hash> " +
	"--repository-context <repository_context> " +
	"--operation <name-status|numstat|stat|patch|object>`. " +
	"Every value comes from this input and nowhere else. " +
	"Use it whenever a check turns on a generated path's content, because that content reaches you no other way. " +
	"`stat` and `patch` also take `--path-index <n>`, the zero-based index into `validation_request.correction_paths`; " +
	"`object` takes that same `--path-index` plus `--side base|candidate`. Never pass `--lens` or `--order`. " +
	"That command is the only sanctioned route to the frozen trees: never read the live worktree, index, or HEAD, " +
	"and never reach the repository through any other command.\n\n" +
	"Reporting that the candidate could not be inspected is a last resort, never a first response. " +
	"It is admissible only after the supplied evidence, and that command where you can run it, have both failed to " +
	"answer. Never record an inconclusive inspection as a failed check: a failed check spends the correction budget " +
	"on something you did not observe. When a check turns on a generated path's omitted content and you cannot run " +
	"that command, that check carries no verdict: mark it unavailable rather than reading one out of the summary.\n\n" +
	"Reporting an unreachable candidate: use each check's inspection field, never evidence wording. When a check's " +
	"verdict came from actually reading the frozen trees, omit inspection entirely -- that is the default. When it " +
	"did not, and the check therefore carries no verdict, set that check's inspection.status to \"unavailable\" and " +
	"inspection.reason to why the frozen trees could not be read. Only this typed field marks a check inconclusive: " +
	"quoting the candidate's own text, including a fixture string or a feature description that itself talks about " +
	"unreadable candidates, is ordinary evidence and never changes the verdict on its own.\n\n" +
	"Return exactly one JSON object with no prose. " +
	"Validate your result against the supplied output schema. " +
	"Echo targeted_validation_request_hash and correction_target_identity from the input. " +
	"Include original_criteria and correction_regression, each with a boolean passed and non-empty evidence. " +
	"Set `original_criteria.passed` to true only when every original criterion is met in the corrected candidate; set it to false when any original criterion remains unmet. " +
	"Set `correction_regression.passed` to true only when the correction caused no regression; set it to false when you observe a regression. " +
	"When `correction_regression.passed` is false, populate `correction_regression.regressions` with one entry per observed regression (id, location, claim, proof_refs) before writing its evidence prose; when it is true, omit `regressions` or use []. " +
	"Always emit follow_ups; use [] when none exist. " +
	"Native Go alone decides correction accounting, receipts, and delivery gates."

// SeverityRules is the one severity discipline every reviewer surface renders:
// the provider lens-context instruction, the installed reviewer bodies, and the
// refuter prompt. It lives here because all three surfaces already import this
// package. The rules are concrete on purpose: abstract severity guidance drifts
// between models and runs, while these conditions can be checked per finding.
const SeverityRules = "Severity rules. A BLOCKER or CRITICAL finding must be caused by this change -- the behavior does not already happen at the baseline -- and must be reachable with realistic input. " +
	"Behavior that already existed at the baseline and was not asked to change, and failures that need out-of-domain values, are at most WARNING. " +
	"Silently ignoring an explicit option or argument while reporting success, and unrequested changes to existing command output or messages, are severe: report them as BLOCKER or CRITICAL."

// RefuterProbeIsolated reports whether the runtime's compiled adapter can run
// the S11 refuter probe in isolation: a fresh copy of the candidate tree under
// the system temp dir, writes confined to it, and no network. Only Codex's
// workspace-write sandbox gives all three. Claude has no network isolation once
// tools are enabled, Pi reviews in process without tools, and OpenCode's bash
// default is unknown, so they stay no-probe.
func RefuterProbeIsolated(agent model.AgentID) bool {
	return agent == model.AgentCodex
}

// RefuterProbeUnavailableNote is the proof_refs entry Go adds to every refuter
// result admitted from a runtime that could not probe, so the receipt states
// that no command ran instead of leaving the reader to guess.
func RefuterProbeUnavailableNote(agent model.AgentID) string {
	name := string(agent)
	if name == "" {
		name = "an unspecified runtime"
	}
	return "probe unavailable on " + name
}

// RefuterProbeInstruction is the runtime-conditional paragraph appended to the
// refuter prompt. It offers the probe only where RefuterProbeIsolated holds.
func RefuterProbeInstruction(agent model.AgentID) string {
	if RefuterProbeIsolated(agent) {
		return "Probe. You run in a scratch copy of the frozen candidate tree, created under the system temp dir and removed after you return; it is not the user's workspace. " +
			"You may run one reproducing command per claim in that scratch copy to confirm or drop the claim. " +
			"No network and no installs: use only tools and dependencies already present, and write only inside the scratch copy. " +
			"For every claim you probe, cite the exact command and its observed output in proof_refs; if the command cannot run, say so there and decide from the supplied evidence."
	}
	return "Probe unavailable. This runtime cannot isolate a reproducing command, so run no command and decide every claim from the supplied evidence. " +
		"Go records \"" + RefuterProbeUnavailableNote(agent) + "\" in the proof_refs of every result."
}

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
		PromptInstruction: "You are the detached read-only refuter for exactly ONE transaction-wide inferential batch. Return exactly one corroborated, refuted, or inconclusive outcome for every supplied claim. Add no findings, modify nothing, and return exactly one JSON object with no prose. Native Go alone applies the result to RDD authority.\n\n" +
			SeverityRules + " Refute a BLOCKER or CRITICAL claim that fails these conditions: it is not caused by this change, it is not reachable with realistic input, or these rules cap it at WARNING.",
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
