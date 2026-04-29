package testutil

import (
	"encoding/json"
	"os"
	"testing"
)

func AssertJSONGolden(t *testing.T, actual []byte, goldenPath string) {
	t.Helper()

	var actualObj any
	if err := json.Unmarshal(actual, &actualObj); err != nil {
		t.Fatalf("actual JSON unmarshal error: %v\n%s", err, string(actual))
	}
	normalizedActual, err := json.MarshalIndent(actualObj, "", "  ")
	if err != nil {
		t.Fatalf("actual JSON marshal error: %v", err)
	}

	goldenRaw, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", goldenPath, err)
	}
	var goldenObj any
	if err := json.Unmarshal(goldenRaw, &goldenObj); err != nil {
		t.Fatalf("golden JSON unmarshal error: %v\n%s", err, string(goldenRaw))
	}
	normalizedGolden, err := json.MarshalIndent(goldenObj, "", "  ")
	if err != nil {
		t.Fatalf("golden JSON marshal error: %v", err)
	}

	if string(normalizedActual) != string(normalizedGolden) {
		t.Fatalf("JSON mismatch with golden %s\nactual:\n%s\n\nexpected:\n%s", goldenPath, string(normalizedActual), string(normalizedGolden))
	}
}

func AssertTextGolden(t *testing.T, actual string, goldenPath string) {
	t.Helper()

	goldenRaw, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", goldenPath, err)
	}

	if actual != string(goldenRaw) {
		t.Fatalf("text mismatch with golden %s\nactual:\n%s\n\nexpected:\n%s", goldenPath, actual, string(goldenRaw))
	}
}
