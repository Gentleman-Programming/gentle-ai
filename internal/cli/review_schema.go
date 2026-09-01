package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewerprovider"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// reviewReviewerSchema is served verbatim from the admission package. The
// schema is defined next to AdmitArtifact so this command, the generated lens
// agent prompts, and the admission logic cannot describe different envelopes.
const reviewReviewerSchema = reviewtransaction.ReviewerResultSchema

const reviewResultDryRunResponseSchema = `{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"https://gentle-ai.dev/contracts/review-integration/v2/schemas/capture-result-dry-run.schema.json","title":"Gentle AI capture-result admission dry run","type":"object","additionalProperties":false,"required":["schema","operation","validation","lineage_id","lens","selected_order","subject_hash"],"properties":{"schema":{"const":"gentle-ai.review-capture-result-dry-run/v1"},"operation":{"const":"review/capture-result"},"validation":{"const":"accepted"},"lineage_id":{"type":"string","pattern":"^[a-z0-9]+(?:-[a-z0-9]+)*$"},"lens":{"enum":["risk","resilience","readability","reliability","review-risk","review-resilience","review-readability","review-reliability"]},"selected_order":{"type":"integer","minimum":0,"maximum":3},"subject_hash":{"$ref":"#/$defs/sha256"},"admission_decision":{"const":"completed"}},"$defs":{"sha256":{"type":"string","pattern":"^sha256:[0-9a-f]{64}$"}}}`

var reviewInputSchemas = map[string]json.RawMessage{
	"reviewer":               json.RawMessage(reviewReviewerSchema),
	"capture-result-dry-run": json.RawMessage(reviewResultDryRunResponseSchema),
	"refuter":                json.RawMessage(reviewerprovider.RefuterResultSchema),
	"validator":              json.RawMessage(reviewerprovider.TargetedValidatorResultSchema),
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
