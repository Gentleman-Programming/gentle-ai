package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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

func TestV1ReachableStatusSerializationBoundaries(t *testing.T) {
	for _, name := range []string{"generated manifest", "disposition"} {
		t.Run(name, func(t *testing.T) {
			reviewEnabledHome(t)
			repo := initReviewCLIRepo(t)
			var lineage string
			var selectors []string
			if name == "generated manifest" {
				writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int { return 1 }\n", 0o644)
				writeReviewStartCandidate(t, repo, "testdata/golden/output.golden", "after\n", 0o644)
				started := runNegotiatedReviewStart(t, repo, "v1-generated")
				lineage = started.LineageID
			} else {
				writeInspectCLIRecoveryPair(t, repo, "v1-disposition", false, dispositionForgedAuthorization)
				lineage = "v1-disposition-successor"
				selectors = []string{"--projection", "staged"}
			}
			before := inspectCLIAuthorityTree(t, reviewCLIAuthorityRoot(t, repo))
			args := []string{"status", "--cwd", repo, "--contract", ReviewIntegrationContractV1, "--next-transition", "--lineage", lineage}
			var output bytes.Buffer
			if err := RunReview(append(args, selectors...), &output); err != nil {
				t.Fatal(err)
			}
			after := inspectCLIAuthorityTree(t, reviewCLIAuthorityRoot(t, repo))
			if !reflect.DeepEqual(before, after) {
				t.Fatal("read-only STATUS changed authority")
			}
			validatePublishedReviewSchema(t, compileWholePublishedReviewSchema(t, "v1", "status-v2.schema.json"), output.Bytes())
		})
	}
}
