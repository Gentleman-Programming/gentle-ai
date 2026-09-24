package pi

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestDecodeModelRoutingInspectionJSONRichProviderDocument(t *testing.T) {
	got, raw, err := decodeModelRoutingInspectionJSON([]byte(richModelRoutingInspection))
	if err != nil {
		t.Fatal(err)
	}
	if got.Contract != "gentle-pi.model-routing/v1" || got.Context == nil || !got.Models[1].Authenticated.Unknown {
		t.Fatalf("typed inspection = %#v", got)
	}
	if got.Models[1].Provider != "provider" || got.Models[1].Capabilities.Input[1] != "image" || got.Diagnostics[1].Severity != ModelRoutingDiagnosticSeverityInfo {
		t.Fatalf("provider DTOs = %#v", got.Models[1])
	}
	for _, key := range []string{"contract", "context", "targets", "assignments", "agents", "providers", "models", "diagnostics"} {
		if _, ok := raw[key]; !ok {
			t.Fatalf("raw document lacks %q: %#v", key, raw)
		}
	}
	if string(raw["contract"]) != `"gentle-pi.model-routing/v1"` {
		t.Fatalf("raw contract = %s", raw["contract"])
	}
}

func TestDecodeModelRoutingInspectionJSONRejectsDocumentSyntax(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
	}{
		{"empty", ""},
		{"whitespace", " \n\t"},
		{"null", "null"},
		{"array", "[]"},
		{"scalar", "1"},
		{"malformed", `{"contract":`},
		{"trailing object", `{"contract":"v"} {}`},
		{"trailing scalar", `{"contract":"v"} false`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := decodeModelRoutingInspectionJSON([]byte(tt.body)); err == nil {
				t.Fatalf("accepted %s document", tt.name)
			}
		})
	}
}

func TestDecodeModelRoutingInspectionJSONRejectsRecursiveDuplicateKeys(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
	}{
		{"top-level", `{"contract":"a","contract":"b"}`},
		{"nested object", `{"context":{"cwd":"a","cwd":"b"}}`},
		{"nested array object", `{"models":[{"name":"a","name":"b"}]}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := decodeModelRoutingInspectionJSON([]byte(tt.body)); err == nil {
				t.Fatalf("accepted duplicate %s key", tt.name)
			}
		})
	}
}

func TestDecodeModelRoutingInspectionJSONRejectsUnknownFieldsAtEveryTypedLevel(t *testing.T) {
	for _, body := range []string{
		`{"unexpected":true}`,
		`{"context":{"unexpected":true}}`,
		`{"models":[{"capabilities":{"unexpected":true}}]}`,
	} {
		if _, _, err := decodeModelRoutingInspectionJSON([]byte(body)); err == nil {
			t.Fatalf("accepted unknown field in %s", body)
		}
	}
}

func knownInspectionContractSize(size int) []byte {
	const prefix, suffix = `{"contract":"`, `"}`
	return []byte(prefix + strings.Repeat("x", size-len(prefix)-len(suffix)) + suffix)
}

func TestDecodeModelRoutingInspectionJSONEnforcesResponseBound(t *testing.T) {
	exact := knownInspectionContractSize(MaxModelRoutingResponseBytes)
	if len(exact) != MaxModelRoutingResponseBytes {
		t.Fatalf("exact fixture size = %d", len(exact))
	}
	if _, _, err := decodeModelRoutingInspectionJSON(exact); err != nil {
		t.Fatalf("exact known-field bound rejected: %v", err)
	}
	oversized := knownInspectionContractSize(MaxModelRoutingResponseBytes + 1)
	if _, _, err := decodeModelRoutingInspectionJSON(oversized); err == nil {
		t.Fatal("accepted oversized valid known-field document")
	}
}

func TestDecodeModelRoutingInspectionJSONOwnsTypedAndRawResults(t *testing.T) {
	input := []byte(richModelRoutingInspection)
	got, raw, err := decodeModelRoutingInspectionJSON(input)
	if err != nil {
		t.Fatal(err)
	}
	contractRaw := append([]byte(nil), raw["contract"]...)
	for i := range input {
		input[i] = 'x'
	}
	if got.Contract != "gentle-pi.model-routing/v1" || !bytes.Equal(raw["contract"], contractRaw) {
		t.Fatal("decoded values retained caller input storage")
	}

	got.Contract = "changed"
	got.Targets[ModelRoutingTargetGlobal].Assignments["new"] = ModelRoutingAssignment{}
	raw["contract"][1] = 'x'
	delete(raw, "providers")

	again, rawAgain, err := decodeModelRoutingInspectionJSON([]byte(richModelRoutingInspection))
	if err != nil {
		t.Fatal(err)
	}
	if again.Contract == "changed" || len(again.Targets[ModelRoutingTargetGlobal].Assignments) != 0 || string(rawAgain["contract"]) != string(contractRaw) || rawAgain["providers"] == nil {
		t.Fatalf("decoded results shared state: typed=%#v raw=%#v", again, rawAgain)
	}
}

func TestDecodeModelRoutingInspectionJSONLeavesSemanticsToLaterLayer(t *testing.T) {
	// Syntax-only boundary: #3585 owns required fields, enum/literal validity,
	// catalog/configurable truth, null-optionals, and contract semantics.
	got, _, err := decodeModelRoutingInspectionJSON([]byte(`{"models":[{"operational":"future"}],"context":null}`))
	if err != nil {
		t.Fatal(err)
	}
	if got.Contract != "" || got.Models[0].Operational != ModelRoutingOperational("future") || got.Context != nil {
		t.Fatalf("syntax decoder imposed semantic validation: %#v", got)
	}
}

func TestDecodeModelRoutingInspectionJSONRejectsTypedFieldKinds(t *testing.T) {
	if _, _, err := decodeModelRoutingInspectionJSON([]byte(`{"models":"not-an-array"}`)); err == nil {
		t.Fatal("accepted wrong known-field type")
	}
	if _, _, err := decodeModelRoutingInspectionJSON([]byte(`{"models":[{"authenticated":null}]}`)); err == nil {
		t.Fatal("accepted invalid canonical authenticated representation")
	}
	var raw json.RawMessage = []byte(`{"contract":"v"}`)
	if _, _, err := decodeModelRoutingInspectionJSON(raw); err != nil {
		t.Fatal(err)
	}
}
