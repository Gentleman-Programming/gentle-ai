package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestV1CorruptStatusPreservesAuthority(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeAtomicStartDocumentationCandidate(t, repo)
	const lineage = "v1-corrupt-status"
	writeAtomicStartCorruptSibling(t, repo, "v2", lineage)
	path := filepath.Join(reviewCLIAuthorityRoot(t, repo), "v2", lineage, "review-state.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, selected := range []bool{false, true} {
		var output bytes.Buffer
		args := []string{"status", "--contract", ReviewIntegrationContractV1, "--next-transition", "--cwd", repo}
		if selected {
			args = append(args, "--lineage", lineage)
		}
		if err := RunReview(args, &output); err != nil {
			t.Fatal(err)
		}
		validatePublishedReviewSchema(t, compileWholePublishedReviewSchema(t, "v1", "status-v2.schema.json"), output.Bytes())
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, after) {
			t.Fatal("STATUS changed corrupt authority bytes")
		}
	}
}

// The frozen contract is preserved by serialization, never by inventing
// a legacy receipt for today's receipt-free review lifecycle.
func TestV1StatusPublishedSchemaCompatibility(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n", 0o644)
	schema := compileWholePublishedReviewSchema(t, "v1", "status-v2.schema.json")
	for _, projection := range []string{"workspace", "staged"} {
		for _, next := range []bool{false, true} {
			args := []string{"status", "--cwd", repo, "--contract", ReviewIntegrationContractV1, "--projection", projection}
			if next {
				args = append(args, "--next-transition")
			}
			var output bytes.Buffer
			if err := RunReview(args, &output); err != nil {
				t.Fatal(err)
			}
			validatePublishedReviewSchema(t, schema, output.Bytes())
			var doc map[string]any
			if err := json.Unmarshal(output.Bytes(), &doc); err != nil {
				t.Fatal(err)
			}
			receipt, ok := doc["receipt"].(map[string]any)
			if !ok || len(receipt) != 1 || receipt["status"] != "not_applicable" {
				t.Fatalf("unexpected receipt assertion: %#v", doc["receipt"])
			}
			for _, field := range []string{"eligible_untracked_inventory", "disposition", "repository_context", "forecast"} {
				if _, exists := doc[field]; exists {
					t.Fatalf("v2 field %s leaked into v1", field)
				}
			}
		}
	}
	var modern bytes.Buffer
	if err := RunReview([]string{"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2, "--next-transition"}, &modern); err != nil {
		t.Fatal(err)
	}
	validatePublishedReviewSchema(t, compileWholeNativeStatusSchema(t, "status-v9.schema.json"), modern.Bytes())
	var doc map[string]any
	if err := json.Unmarshal(modern.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if _, exists := doc["receipt"]; exists {
		t.Fatal("legacy receipt field leaked into v2")
	}
	if doc["eligible_untracked_inventory"] == nil {
		t.Fatal("v2 inventory disappeared")
	}
}
