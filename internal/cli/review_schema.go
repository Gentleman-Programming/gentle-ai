package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

var reviewInputSchemas = map[string]json.RawMessage{
	"reviewer":  json.RawMessage(`{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"https://gentle-ai.dev/schema/review/reviewer/v1","title":"Gentle AI reviewer result","type":"object","additionalProperties":false,"required":["findings","evidence"],"properties":{"lens":{"type":"string","enum":["risk","resilience","readability","reliability","review-risk","review-resilience","review-readability","review-reliability"]},"findings":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["location","severity","claim","proof_refs"],"allOf":[{"if":{"properties":{"severity":{"enum":["BLOCKER","CRITICAL"]}},"required":["severity"]},"then":{"required":["evidence_class","causal_disposition"]}}],"properties":{"id":{"type":"string"},"lens":{"type":"string","enum":["risk","resilience","readability","reliability"]},"location":{"type":"string","minLength":1},"severity":{"type":"string","enum":["BLOCKER","CRITICAL","WARNING","SUGGESTION"]},"claim":{"type":"string","minLength":1},"proof_refs":{"type":"array","minItems":1,"items":{"type":"string","pattern":"\\S","not":{"pattern":"^\\s*(?:[nN]/[aA]|[nN][aA]|[nN][oO][nN][eE]|[tT][oO][dD][oO]|[tT][bB][dD]|[pP][aA][sS][sS]|[pP][aA][sS][sS][eE][dD]|[sS][uU][cC][cC][eE][sS][sS]|[pP][lL][aA][cC][eE][hH][oO][lL][dD][eE][rR])\\s*$"}}},"evidence_class":{"type":"string","enum":["deterministic","inferential","insufficient"]},"causal_disposition":{"type":"string","enum":["introduced","behavior-activated","worsened","pre-existing","base-only","unknown"]}}}},"evidence":{"type":"array","minItems":1,"items":{"type":"string","pattern":"\\S","not":{"pattern":"^\\s*(?:[nN]/[aA]|[nN][aA]|[nN][oO][nN][eE]|[tT][oO][dD][oO]|[tT][bB][dD]|[pP][aA][sS][sS]|[pP][aA][sS][sS][eE][dD]|[sS][uU][cC][cC][eE][sS][sS]|[pP][lL][aA][cC][eE][hH][oO][lL][dD][eE][rR])\\s*$"}}}},"examples":[{"findings":[],"evidence":["reviewed the complete candidate scope"]}]}`),
	"refuter":   json.RawMessage(`{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"https://gentle-ai.dev/schema/review/refuter/v1","title":"Gentle AI refuter result","type":"object","additionalProperties":false,"required":["results"],"properties":{"results":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["finding_id","outcome","proof_refs"],"properties":{"finding_id":{"type":"string"},"outcome":{"type":"string","enum":["corroborated","refuted","inconclusive"]},"proof_refs":{"type":"array","minItems":1,"items":{"type":"string","pattern":"\\S"}}}}}},"examples":[{"results":[]}]}`),
	"validator": json.RawMessage(`{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"https://gentle-ai.dev/schema/review/validator/v1","title":"Gentle AI targeted validator result","type":"object","additionalProperties":false,"required":["original_criteria","correction_regression","follow_ups"],"properties":{"original_criteria":{"$ref":"#/$defs/check"},"correction_regression":{"$ref":"#/$defs/check"},"follow_ups":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["observation","proof_refs"],"properties":{"observation":{"type":"string"},"proof_refs":{"type":"array","minItems":1,"items":{"type":"string","pattern":"\\S"}}}}}},"$defs":{"check":{"type":"object","additionalProperties":false,"required":["passed","evidence"],"properties":{"passed":{"type":"boolean"},"evidence":{"type":"array","minItems":1,"items":{"type":"string"}}}}},"examples":[{"original_criteria":{"passed":true,"evidence":["acceptance test passed"]},"correction_regression":{"passed":true,"evidence":["regression test passed"]},"follow_ups":[]}]}`),
}

func RunReviewSchema(args []string, stdout io.Writer) error {
	if len(args) != 1 {
		return errors.New("review schema requires exactly one of reviewer, refuter, or validator")
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
