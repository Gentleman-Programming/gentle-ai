package pi

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func inspectEnvelopeFixture(result []byte, exitClass string, ok bool) []byte {
	okValue := "false"
	if ok {
		okValue = "true"
	}
	return []byte(`{"version":1,"contract":"` + modelRoutingContract + `","operation":"inspect","ok":` + okValue + `,"exitClass":"` + exitClass + `","result":` + string(result) + `}`)
}

func richInspectResultWithWarning(t *testing.T) []byte {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal(semanticInspectionFixture(t), &document); err != nil {
		t.Fatal(err)
	}
	document["providers"] = []any{"provider"}
	document["diagnostics"].([]any)[0].(map[string]any)["severity"] = "warning"
	result, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func inspectEnvelopeWithoutField(t *testing.T, body []byte, field string) []byte {
	t.Helper()
	var document map[string]json.RawMessage
	if err := json.Unmarshal(body, &document); err != nil {
		t.Fatal(err)
	}
	delete(document, field)
	result, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func inspectEnvelopeWithNullField(t *testing.T, body []byte, field string) []byte {
	t.Helper()
	var document map[string]json.RawMessage
	if err := json.Unmarshal(body, &document); err != nil {
		t.Fatal(err)
	}
	document[field] = json.RawMessage("null")
	result, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func inspectErrorKind(t *testing.T, body []byte, want ModelRoutingInspectErrorKind) error {
	t.Helper()
	_, err := ParseModelRoutingInspectResponse(body)
	if err == nil {
		t.Fatal("ParseModelRoutingInspectResponse accepted invalid response")
	}
	var typed *ModelRoutingInspectError
	if !errors.As(err, &typed) {
		t.Fatalf("error = %T %v; want *ModelRoutingInspectError", err, err)
	}
	if typed.Kind != want {
		t.Fatalf("ModelRoutingInspectError.Kind = %q, want %q", typed.Kind, want)
	}
	return err
}

func TestParseModelRoutingInspectResponseSuccessFailureAndWarnings(t *testing.T) {
	result := richInspectResultWithWarning(t)
	for _, tt := range []struct {
		name      string
		ok        bool
		exitClass string
	}{
		{name: "success", ok: true, exitClass: "success"},
		{name: "failure data", ok: false, exitClass: "invalid-input"},
		{name: "inconsistent data remains data", ok: false, exitClass: "success"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseModelRoutingInspectResponse(inspectEnvelopeFixture(result, tt.exitClass, tt.ok))
			if err != nil {
				t.Fatalf("parse error = %v", err)
			}
			if got.Version != 1 || got.Contract != modelRoutingContract || got.Operation != "inspect" || got.OK != tt.ok || got.ExitClass != tt.exitClass {
				t.Fatalf("envelope = %#v", got)
			}
			if got.Result.Context == nil || got.Result.Context.Target != ModelRoutingTargetProject || len(got.Result.Providers) != 1 || got.Result.Providers[0] != "provider" {
				t.Fatalf("result = %#v", got.Result)
			}
			if got.Result.Diagnostics[0].Severity != ModelRoutingDiagnosticSeverityWarning || got.Result.Agents[0].Assignment == nil {
				t.Fatalf("diagnostics/optional result = %#v", got.Result)
			}
		})
	}
}

func TestParseModelRoutingInspectResponseRejectsEnvelopeAndNestedViolations(t *testing.T) {
	valid := inspectEnvelopeFixture([]byte(`{"contract":"`+modelRoutingContract+`","targets":{},"assignments":{},"agents":[],"providers":[],"models":[],"diagnostics":[]}`), "success", true)
	validText := string(valid)
	cases := []struct {
		name     string
		body     []byte
		kind     ModelRoutingInspectErrorKind
		sentinel error
	}{
		{name: "empty", body: nil, kind: ModelRoutingInspectErrorMalformed},
		{name: "null", body: []byte(`null`), kind: ModelRoutingInspectErrorMalformed},
		{name: "array", body: []byte(`[]`), kind: ModelRoutingInspectErrorMalformed},
		{name: "scalar", body: []byte(`1`), kind: ModelRoutingInspectErrorMalformed},
		{name: "malformed", body: []byte(`{"version":`), kind: ModelRoutingInspectErrorMalformed},
		{name: "trailing", body: append(append([]byte(nil), valid...), []byte(` {}`)...), kind: ModelRoutingInspectErrorMalformed},
		{name: "duplicate outer", body: []byte(strings.Replace(validText, `"version":1`, `"version":1,"version":1`, 1)), kind: ModelRoutingInspectErrorMalformed},
		{name: "duplicate nested", body: []byte(strings.Replace(validText, `"contract":"`+modelRoutingContract+`","targets"`, `"contract":"`+modelRoutingContract+`","contract":"`+modelRoutingContract+`","targets"`, 1)), kind: ModelRoutingInspectErrorMalformed},
		{name: "unknown outer", body: []byte(strings.Replace(validText, `,"result"`, `,"extra":true,"result"`, 1)), kind: ModelRoutingInspectErrorMalformed},
		{name: "unknown nested", body: []byte(strings.Replace(validText, `,"diagnostics":[]}`, `,"diagnostics":[],"extra":true}`, 1)), kind: ModelRoutingInspectErrorMalformed},
		{name: "undocumented result capability", body: []byte(strings.Replace(validText, `,"diagnostics":[]}`, `,"diagnostics":[],"capabilities":{}}`, 1)), kind: ModelRoutingInspectErrorMalformed},
		{name: "wrong version", body: []byte(strings.Replace(validText, `"version":1`, `"version":2`, 1)), kind: ModelRoutingInspectErrorUnsupportedVersion, sentinel: ErrModelRoutingInspectUnsupportedVersion},
		{name: "wrong contract", body: []byte(strings.Replace(validText, `"contract":"`+modelRoutingContract+`"`, `"contract":"other/v1"`, 1)), kind: ModelRoutingInspectErrorUnsupportedContract, sentinel: ErrModelRoutingInspectUnsupportedContract},
		{name: "wrong operation", body: []byte(strings.Replace(validText, `"operation":"inspect"`, `"operation":"validate"`, 1)), kind: ModelRoutingInspectErrorUnsupportedOperation, sentinel: ErrModelRoutingInspectUnsupportedOperation},
		{name: "nested required field", body: []byte(strings.Replace(validText, `"contract":"`+modelRoutingContract+`","targets"`, `"targets"`, 1)), kind: ModelRoutingInspectErrorMalformed},
		{name: "nested literal", body: []byte(strings.Replace(validText, `,"result":{"contract":"`+modelRoutingContract+`"`, `,"result":{"contract":"other/v1"`, 1)), kind: ModelRoutingInspectErrorMalformed},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			err := inspectErrorKind(t, tt.body, tt.kind)
			if tt.sentinel != nil && !errors.Is(err, tt.sentinel) {
				t.Fatalf("error = %v; errors.Is(..., %v) = false", err, tt.sentinel)
			}
		})
	}

	for _, field := range []string{"version", "contract", "operation", "ok", "exitClass", "result"} {
		t.Run("missing "+field, func(t *testing.T) {
			inspectErrorKind(t, inspectEnvelopeWithoutField(t, valid, field), ModelRoutingInspectErrorMalformed)
		})
	}
}

func TestParseModelRoutingInspectResponseNullFieldsAndNestedCause(t *testing.T) {
	result := []byte(`{"contract":"` + modelRoutingContract + `","targets":{},"assignments":{},"agents":[],"providers":[],"models":[],"diagnostics":[]}`)
	valid := string(inspectEnvelopeFixture(result, "success", true))
	for _, field := range []string{"version", "contract", "operation", "ok", "exitClass", "result"} {
		t.Run("null "+field, func(t *testing.T) {
			inspectErrorKind(t, inspectEnvelopeWithNullField(t, []byte(valid), field), ModelRoutingInspectErrorMalformed)
		})
	}

	nested := strings.Replace(valid, `"targets":{}`, `"targets":null`, 1)
	err := inspectErrorKind(t, []byte(nested), ModelRoutingInspectErrorMalformed)
	if !strings.Contains(err.Error(), "required field") && !strings.Contains(err.Error(), "must be an object") {
		t.Fatalf("nested cause = %v", err)
	}
}

func TestParseModelRoutingInspectResponseBoundsCauseAndOwnership(t *testing.T) {
	result := richInspectResultWithWarning(t)
	valid := inspectEnvelopeFixture(result, "success", true)
	exact := append(append([]byte(nil), valid...), bytes.Repeat([]byte(" "), MaxModelRoutingResponseBytes-len(valid))...)
	if _, err := ParseModelRoutingInspectResponse(exact); err != nil {
		t.Fatalf("exact bound error = %v", err)
	}
	_, err := ParseModelRoutingInspectResponse(bytes.Repeat([]byte("x"), MaxModelRoutingResponseBytes+1))
	var oversized *ModelRoutingInspectError
	if !errors.As(err, &oversized) || oversized.Kind != ModelRoutingInspectErrorOversized || !errors.Is(err, ErrModelRoutingInspectOversized) || oversized.Unwrap() == nil {
		t.Fatalf("oversized error = %T %v", err, err)
	}
	_, err = ParseModelRoutingInspectResponse([]byte(`{"version":1,}`))
	var syntax *json.SyntaxError
	if !errors.As(err, &syntax) || !errors.Is(err, ErrModelRoutingInspectMalformed) {
		t.Fatalf("malformed cause = %T %v", err, err)
	}

	got, err := ParseModelRoutingInspectResponse(valid)
	if err != nil {
		t.Fatal(err)
	}
	for i := range valid {
		valid[i] = 'x'
	}
	if got.Result.Context.AgentDir != "/agents" || got.Result.Providers[0] != "provider" {
		t.Fatalf("parsed values alias input: %#v", got.Result)
	}
	got.Result.Providers[0] = "mutated"
	again, err := ParseModelRoutingInspectResponse(inspectEnvelopeFixture(result, "success", true))
	if err != nil || again.Result.Providers[0] == "mutated" {
		t.Fatalf("parsed values share state: %#v, %v", again, err)
	}
}
