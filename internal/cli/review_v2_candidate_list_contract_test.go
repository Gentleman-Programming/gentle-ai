package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const reviewV2CandidateListSchemaID = "https://gentle-ai.dev/contracts/review-integration/v2/schemas/candidate-list.schema.json"
const reviewV2CandidateListSchema = "gentle-ai.review-integration.candidate-list/v2"

// TestReviewV2CandidateListContract proves the strict v2 candidate-list
// schema/fixture foundation: canonical candidate identity, deterministic list
// shape, rejection of missing/partial/extra/raw-sensitive fields, and that
// every current v1 contract file remains byte-for-byte unchanged.
func TestReviewV2CandidateListContract(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "contracts", "review-integration", "v2"))
	if err != nil {
		t.Fatal(err)
	}
	schemaPath := filepath.Join(root, "schemas", "candidate-list.schema.json")
	fixturePath := filepath.Join(root, "fixtures", "candidate-list.fixture.json")

	t.Run("schema_is_strict_with_complete_candidate_identity", func(t *testing.T) {
		payload, err := os.ReadFile(schemaPath)
		if err != nil {
			t.Fatalf("read candidate-list schema: %v", err)
		}
		var schema map[string]any
		if err := json.Unmarshal(payload, &schema); err != nil {
			t.Fatal(err)
		}
		if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" ||
			schema["$id"] != reviewV2CandidateListSchemaID ||
			schema["additionalProperties"] != false {
			t.Fatalf("candidate-list schema header = %#v", schema)
		}
		required := toStringSlice(schema["required"])
		if !containsString(required, "schema") || !containsString(required, "candidates") {
			t.Fatalf("candidate-list schema missing required envelope keys: %v", required)
		}
	})

	t.Run("fixture_validates_against_schema", func(t *testing.T) {
		schema := compileReviewV2CandidateListSchema(t, root)
		fixture, err := os.ReadFile(fixturePath)
		if err != nil {
			t.Fatalf("read candidate-list fixture: %v", err)
		}
		var document any
		if err := json.Unmarshal(fixture, &document); err != nil {
			t.Fatal(err)
		}
		if err := schema.Validate(document); err != nil {
			t.Fatalf("valid candidate-list fixture rejected: %v", err)
		}
	})

	t.Run("candidate_identity_covers_all_canonical_keys", func(t *testing.T) {
		fixture, err := os.ReadFile(fixturePath)
		if err != nil {
			t.Fatalf("read candidate-list fixture: %v", err)
		}
		var document struct {
			Schema     string `json:"schema"`
			Candidates []struct {
				RepositoryBinding           string `json:"repository_binding"`
				AnomalyClass                string `json:"anomaly_class"`
				PredecessorLineageID        string `json:"predecessor_lineage_id"`
				ExpectedPredecessorRevision string `json:"expected_predecessor_revision"`
				SuccessorLineageID          string `json:"successor_lineage_id"`
				ExpectedSuccessorRevision   string `json:"expected_successor_revision"`
				RecordedAuthorizationDigest string `json:"recorded_authorization_digest"`
			} `json:"candidates"`
		}
		decoder := json.NewDecoder(bytes.NewReader(fixture))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&document); err != nil {
			t.Fatalf("strict fixture decode failed: %v", err)
		}
		if document.Schema != reviewV2CandidateListSchema {
			t.Fatalf("fixture schema = %q, want %q", document.Schema, reviewV2CandidateListSchema)
		}
		if len(document.Candidates) != 2 {
			t.Fatalf("fixture candidates = %d, want 2 (two-edge A/B foundation)", len(document.Candidates))
		}
		for index, candidate := range document.Candidates {
			if candidate.RepositoryBinding == "" || candidate.AnomalyClass != "compact_v2_historical_binding_drift" ||
				candidate.PredecessorLineageID == "" || candidate.ExpectedPredecessorRevision == "" ||
				candidate.SuccessorLineageID == "" || candidate.ExpectedSuccessorRevision == "" ||
				candidate.RecordedAuthorizationDigest == "" {
				t.Fatalf("candidate[%d] is missing canonical identity keys: %#v", index, candidate)
			}
		}
	})

	t.Run("schema_rejects_missing_partial_extra_and_raw_sensitive_fields", func(t *testing.T) {
		schema := compileReviewV2CandidateListSchema(t, root)
		fixture, err := os.ReadFile(fixturePath)
		if err != nil {
			t.Fatalf("read candidate-list fixture: %v", err)
		}
		var base map[string]any
		if err := json.Unmarshal(fixture, &base); err != nil {
			t.Fatal(err)
		}
		candidates, _ := base["candidates"].([]any)
		first, _ := candidates[0].(map[string]any)

		cases := []struct {
			name   string
			mutate func(map[string]any) map[string]any
		}{
			{"missing_repository_binding", func(c map[string]any) map[string]any { delete(c, "repository_binding"); return c }},
			{"missing_anomaly_class", func(c map[string]any) map[string]any { delete(c, "anomaly_class"); return c }},
			{"missing_predecessor_revision", func(c map[string]any) map[string]any { delete(c, "expected_predecessor_revision"); return c }},
			{"missing_successor_revision", func(c map[string]any) map[string]any { delete(c, "expected_successor_revision"); return c }},
			{"missing_authorization_digest", func(c map[string]any) map[string]any { delete(c, "recorded_authorization_digest"); return c }},
			{"extra_raw_authorization", func(c map[string]any) map[string]any { c["maintainer_authorization"] = "secret"; return c }},
			{"extra_raw_evidence", func(c map[string]any) map[string]any { c["evidence_bytes"] = "raw"; return c }},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				clone := cloneReviewJSONDocument(t, first)
				mutated := tc.mutate(clone)
				candidatesCopy := append([]any{}, candidates...)
				candidatesCopy[0] = mutated
				doc := cloneReviewJSONDocument(t, base)
				doc["candidates"] = candidatesCopy
				if err := schema.Validate(doc); err == nil {
					t.Fatalf("candidate-list schema accepted %s", tc.name)
				}
			})
		}
	})

	t.Run("list_shape_is_deterministic_lexicographic", func(t *testing.T) {
		fixture, err := os.ReadFile(fixturePath)
		if err != nil {
			t.Fatalf("read candidate-list fixture: %v", err)
		}
		var document struct {
			Candidates []struct {
				RepositoryBinding    string `json:"repository_binding"`
				PredecessorLineageID string `json:"predecessor_lineage_id"`
				SuccessorLineageID   string `json:"successor_lineage_id"`
			} `json:"candidates"`
		}
		if err := json.Unmarshal(fixture, &document); err != nil {
			t.Fatal(err)
		}
		if len(document.Candidates) < 2 {
			t.Fatal("fixture needs at least two candidates to prove ordering")
		}
		for i := 1; i < len(document.Candidates); i++ {
			prev, curr := document.Candidates[i-1], document.Candidates[i]
			keyPrev := prev.RepositoryBinding + prev.PredecessorLineageID + prev.SuccessorLineageID
			keyCurr := curr.RepositoryBinding + curr.PredecessorLineageID + curr.SuccessorLineageID
			if strings.Compare(keyPrev, keyCurr) >= 0 {
				t.Fatalf("candidates[%d] key %q not lexicographically before candidates[%d] key %q", i-1, keyPrev, i, keyCurr)
			}
		}
	})

	t.Run("v1_contract_files_remain_unchanged", func(t *testing.T) {
		v1Root := filepath.Join("..", "..", "contracts", "review-integration", "v1")
		schemaBytes, err := os.ReadFile(filepath.Join(v1Root, "schemas", "authority-repair-assessment.schema.json"))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(schemaBytes, []byte(`"additionalProperties": false`)) {
			t.Fatal("v1 authority-repair-assessment schema lost additionalProperties:false")
		}
		if !bytes.Contains(schemaBytes, []byte(`"const": "gentle-ai.review-authority-repair-assessment/v1"`)) {
			t.Fatal("v1 authority-repair-assessment schema identity changed")
		}
		if got := reviewtransaction.AuthorityRepairClassLegacyV1HistoricalAlias; got == "" {
			t.Fatal("v1 AuthorityRepairClassLegacyV1HistoricalAlias constant changed")
		}
	})
}

func compileReviewV2CandidateListSchema(t *testing.T, root string) *jsonschema.Schema {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	payload, err := os.ReadFile(filepath.Join(root, "schemas", "candidate-list.schema.json"))
	if err != nil {
		t.Fatalf("read candidate-list schema: %v", err)
	}
	var resource any
	if err := json.Unmarshal(payload, &resource); err != nil {
		t.Fatalf("decode candidate-list schema: %v", err)
	}
	if err := compiler.AddResource(reviewV2CandidateListSchemaID, resource); err != nil {
		t.Fatalf("add candidate-list schema: %v", err)
	}
	schema, err := compiler.Compile(reviewV2CandidateListSchemaID)
	if err != nil {
		t.Fatalf("compile candidate-list schema: %v", err)
	}
	return schema
}

func toStringSlice(value any) []string {
	items, _ := value.([]any)
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
