package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestReviewConsentEnvelopeSerializedBytesUnchanged is the byte-equality half
// of the #2554 consent-envelope core extraction proof. It was written BEFORE
// the extraction and must stay green through it: for every shipped consent
// fixture, decoding into the live Go type (rejecting unknown fields, so the
// struct provably covers every fixture field) and re-encoding with the
// fixtures' own two-space indentation must reproduce the exact original file
// bytes. Any extraction that changes a JSON tag, a field order, an omitempty,
// or a field type fails here before it can ship.
func TestReviewConsentEnvelopeSerializedBytesUnchanged(t *testing.T) {
	fixtures := []struct {
		name string
		path string
	}{
		{name: "v1 consent", path: filepath.Join("..", "..", "contracts", "review-integration", "v1", "fixtures", "consent.fixture.json")},
		{name: "v2 consent", path: filepath.Join("..", "..", "contracts", "review-integration", "v2", "fixtures", "consent.fixture.json")},
		{name: "v2.1 consent-v3", path: filepath.Join("..", "..", "contracts", "review-integration", "v2", "fixtures", "consent-v3.fixture.json")},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			payload, err := os.ReadFile(fixture.path)
			if err != nil {
				t.Fatal(err)
			}
			decoder := json.NewDecoder(bytes.NewReader(payload))
			decoder.DisallowUnknownFields()
			var result ReviewIntegrationConsentResult
			if err := decoder.Decode(&result); err != nil {
				t.Fatalf("consent fixture no longer decodes into the live type: %v", err)
			}
			if err := result.Validate(); err != nil {
				t.Fatalf("consent fixture no longer validates: %v", err)
			}
			remarshaled, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			remarshaled = append(remarshaled, '\n')
			if !bytes.Equal(remarshaled, payload) {
				t.Fatalf("consent envelope no longer serializes to the shipped fixture bytes\n--- fixture ---\n%s\n--- remarshaled ---\n%s", payload, remarshaled)
			}
		})
	}
}
