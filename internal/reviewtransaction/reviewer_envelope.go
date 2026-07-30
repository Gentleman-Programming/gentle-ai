package reviewtransaction

import (
	"encoding/json"
	"sort"
	"strings"
)

// ReviewerResultSchema is the published input schema for one reviewer result.
// It lives beside AdmitArtifact on purpose: admission is the authority for the
// shape, and every consumer that describes that shape to a model — the
// `gentle-ai review schema reviewer` command and the generated lens agent
// prompts — derives its wording from this document instead of restating it.
// Three independent prose copies of this envelope are what let a lens agent
// emit findings/evidence with no subject_hash and no inspection (community
// report, PR #1801).
const ReviewerResultSchema = `{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"https://gentle-ai.dev/schema/review/reviewer/v1","title":"Gentle AI reviewer result","type":"object","additionalProperties":false,"required":["subject_hash","inspection","findings","evidence"],"properties":{"subject_hash":{"type":"string","pattern":"^sha256:[0-9a-f]{64}$"},"inspection":{"type":"object","additionalProperties":false,"required":["status","paths"],"properties":{"status":{"const":"completed"},"paths":{"type":"array","uniqueItems":true,"items":{"type":"string","minLength":1}}}},"lens":{"type":"string","enum":["risk","resilience","readability","reliability","review-risk","review-resilience","review-readability","review-reliability"]},"findings":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["location","severity","claim","proof_refs"],"allOf":[{"if":{"properties":{"severity":{"enum":["BLOCKER","CRITICAL"]}},"required":["severity"]},"then":{"required":["evidence_class","causal_disposition"]}}],"properties":{"id":{"type":"string","pattern":"^R[1-4]-[A-Za-z0-9][A-Za-z0-9._-]*$"},"lens":{"type":"string","enum":["risk","resilience","readability","reliability"]},"location":{"type":"string","minLength":1},"severity":{"type":"string","enum":["BLOCKER","CRITICAL","WARNING","SUGGESTION"]},"claim":{"type":"string","minLength":1},"proof_refs":{"type":"array","minItems":1,"items":{"type":"string","pattern":"\\S","not":{"pattern":"^\\s*(?:[nN]/[aA]|[nN][aA]|[nN][oO][nN][eE]|[tT][oO][dD][oO]|[tT][bB][dD]|[pP][aA][sS][sS]|[pP][aA][sS][sS][eE][dD]|[sS][uU][cC][cC][eE][sS][sS]|[pP][lL][aA][cC][eE][hH][oO][lL][dD][eE][rR])\\s*$"}}},"evidence_class":{"type":"string","enum":["deterministic","inferential","insufficient"]},"causal_disposition":{"type":"string","enum":["introduced","behavior-activated","worsened","pre-existing","base-only","unknown"]}}}},"evidence":{"type":"array","minItems":1,"items":{"type":"string","pattern":"\\S","not":{"pattern":"^\\s*(?:[nN]/[aA]|[nN][aA]|[nN][oO][nN][eE]|[tT][oO][dD][oO]|[tT][bB][dD]|[pP][aA][sS][sS]|[pP][aA][sS][sS][eE][dD]|[sS][uU][cC][cC][eE][sS][sS]|[pP][lL][aA][cC][eE][hH][oO][lL][dD][eE][rR])\\s*$"}}}},"examples":[{"subject_hash":"sha256:0000000000000000000000000000000000000000000000000000000000000000","inspection":{"status":"completed","paths":["internal/example.go"]},"findings":[],"evidence":["reviewed the complete candidate scope"]}]}`

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
