package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestV9ReleaseSchemaMatchesGoBoundaryRejections(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "contracts", "review-integration", "v2", "fixtures", "status-v5.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	schema := compileWholeNativeStatusSchema(t, "status-v9.schema.json")
	for _, reason := range []string{"correction_request_unavailable", reviewCorrectionContextBudgetCode} {
		for _, tc := range []struct{ name, command, detail string }{
			{"escaped carriage return", "gentle-ai review abandon --cwd \"a\\\rb\"", "manual release"},
			{"Unicode blank detail", "gentle-ai review abandon --cwd /repo", "\u00a0"},
		} {
			t.Run(reason+"/"+tc.name, func(t *testing.T) {
				c := &ReviewStopContinuation{Operation: "abandon", Command: tc.command, Detail: tc.detail}
				if validateCorrectionReleaseContinuation(c) == nil {
					t.Fatal("Go validator unexpectedly accepted boundary input")
				}
				var doc map[string]any
				if err := json.Unmarshal(raw, &doc); err != nil {
					t.Fatal(err)
				}
				doc["schema"] = ReviewIntegrationStatusSchemaV9
				delete(doc, "forecast")
				doc["next_transition"] = map[string]any{
					"kind": "stop", "reason_code": reason,
					"continuation": map[string]any{"operation": "abandon", "command": tc.command, "detail": tc.detail},
				}
				if schema.Validate(doc) == nil {
					t.Fatal("published schema accepted a Go-rejected continuation")
				}
			})
		}
	}
}
