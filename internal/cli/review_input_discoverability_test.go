package cli

import (
	"strings"
	"testing"
)

// Three independent testers could not reach the validating state because
// capture-result asks for a reviewer-result JSON payload and nothing in the
// product says how to produce a valid one. `gentle-ai review schema reviewer`
// emits the schema together with a working example, so the payload never has
// to be invented -- but a command nobody can find is a command that does not
// exist for the person who needs it.
func TestReviewCaptureResultPointsAtTheSchemaThatProducesItsInput(t *testing.T) {
	const pointer = "gentle-ai review schema reviewer"

	t.Run("the refusal names it", func(t *testing.T) {
		err := RunReviewCaptureResult([]string{"--lineage", "x"}, nil)
		if err == nil {
			t.Fatal("expected a refusal when required inputs are missing")
		}
		if !strings.Contains(err.Error(), pointer) {
			t.Fatalf("refusal asks for --input and never says where a valid one comes from.\n got: %s\nwant it to contain: %s", err.Error(), pointer)
		}
	})

	t.Run("the flag help names it", func(t *testing.T) {
		var out strings.Builder
		_ = RunReviewCaptureResult([]string{"--help"}, &out)
		if !strings.Contains(out.String(), pointer) {
			t.Fatalf("--input help never says where a valid payload comes from.\n got: %s\nwant it to contain: %s", out.String(), pointer)
		}
	})
}

// The named-continuation guard caught this while verifying a refusal that now
// points at `gentle-ai review schema reviewer`: the verb rejects --help, and
// its unknown-value refusal names no accepted value either, so a reader sent
// here has nowhere to go.
func TestReviewSchemaNamesItsAcceptedValues(t *testing.T) {
	for _, argument := range []string{"--help", "-h", "nonsense"} {
		t.Run(argument, func(t *testing.T) {
			var out strings.Builder
			err := RunReviewSchema([]string{argument}, &out)
			surface := out.String()
			if err != nil {
				surface += err.Error()
			}
			if !strings.Contains(surface, "reviewer") || !strings.Contains(surface, "validator") {
				t.Fatalf("review schema %q names no accepted value: %s", argument, surface)
			}
		})
	}
}
