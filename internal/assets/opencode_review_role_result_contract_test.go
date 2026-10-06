package assets

import (
	"regexp"
	"strings"
	"testing"
)

// The OpenCode lens roles are installed as the reviewer's system prompt, while
// the provider-issued lens context in the Task prompt carries the result
// schema that admission enforces. A second result shape in the role prompt
// wins against the schema because it carries a concrete example, and admission
// then refuses the result (#5247): a review_result.lens_results wrapper fails
// the strict decoder, prefixed proof_refs such as changed-hunk:<path>:<line>
// fail repository-path admission, and LENS-001 ids fail the R[1-4]- pattern.
// The role prompt therefore defers the result shape to the provider schema and
// never restates it.
func TestOpenCodeReviewRolesDeferResultShapeToProviderSchema(t *testing.T) {
	forbidden := []*regexp.Regexp{
		regexp.MustCompile(`review_result`),
		regexp.MustCompile(`lens_results`),
		regexp.MustCompile("```json"),
		regexp.MustCompile(`\b(?:changed-hunk|candidate-created-path|differential-test|before-after):`),
		regexp.MustCompile(`\b[A-Z]+-0\d\d\b`),
	}
	for _, lens := range []string{"review-risk", "review-readability", "review-reliability", "review-resilience"} {
		t.Run(lens, func(t *testing.T) {
			source, err := Read("opencode/agents/" + lens + ".md")
			if err != nil {
				t.Fatal(err)
			}
			for _, pattern := range forbidden {
				if match := pattern.FindString(source); match != "" {
					t.Fatalf("%s restates a result shape admission refuses: %q", lens, match)
				}
			}
			if !strings.Contains(source, "GENTLE_AI_REVIEW_RESULT_SCHEMA") {
				t.Fatalf("%s does not defer its result shape to the provider-issued GENTLE_AI_REVIEW_RESULT_SCHEMA", lens)
			}
		})
	}
}
