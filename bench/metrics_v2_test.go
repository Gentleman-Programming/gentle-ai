package main

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestResultsHasV2IdentityFields asserts the Results struct carries the v2
// identity envelope fields and ResultsSchema equals the v2 schema name.
func TestResultsHasV2IdentityFields(t *testing.T) {
	// Assert ResultsSchema is v2.
	if ResultsSchema != "gentle-ai-bench.results/v2" {
		t.Fatalf("ResultsSchema = %q, want %q", ResultsSchema, "gentle-ai-bench.results/v2")
	}

	// Assert Identity field exists in Results.
	resultsType := reflect.TypeOf(Results{})
	_, hasIdentity := resultsType.FieldByName("Identity")
	if !hasIdentity {
		t.Fatalf("Results has no Identity field")
	}

	// Assert Identity type has the required fields.
	identityType := reflect.TypeOf(Identity{})
	for _, f := range []struct {
		name    string
		jsonTag string
	}{
		{"TargetBinarySHA256", "target_binary_sha256"},
		{"EmbeddedVCSRevision", "embedded_vcs_revision"},
		{"VCSModified", "vcs_modified"},
		{"SourceRevision", "source_revision"},
		{"RuntimeGOOS", "runtime_goos"},
		{"RuntimeGOARCH", "runtime_goarch"},
		{"ClassifierVersion", "classifier_version"},
		{"Mode", "mode"},
	} {
		field, found := identityType.FieldByName(f.name)
		if !found {
			t.Errorf("Identity has no field %q", f.name)
			continue
		}
		if jsonTag := field.Tag.Get("json"); jsonTag != f.jsonTag {
			t.Errorf("Identity.%s json tag = %q, want %q", f.name, jsonTag, f.jsonTag)
		}
	}

	// Assert a zero Identity round-trips cleanly through JSON.
	results := Results{
		Schema:   ResultsSchema,
		Identity: Identity{},
	}
	data, err := json.Marshal(results)
	if err != nil {
		t.Fatalf("marshal results with zero identity: %v", err)
	}
	var unmarshaled Results
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("unmarshal results with identity: %v", err)
	}
	if unmarshaled.Schema != ResultsSchema {
		t.Errorf("unmarshaled schema = %q, want %q", unmarshaled.Schema, ResultsSchema)
	}
}
