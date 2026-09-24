package pi

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func operationEnvelopeFixture(operation ModelRoutingOperation, ok bool, exitClass, result string) []byte {
	okValue := "false"
	if ok {
		okValue = "true"
	}
	return []byte(`{"version":1,"contract":"` + modelRoutingContract + `","operation":"` + string(operation) + `","ok":` + okValue + `,"exitClass":"` + exitClass + `","result":` + result + `}`)
}

func operationEnvelopeWithoutField(t *testing.T, payload []byte, field string) []byte {
	t.Helper()
	var object map[string]json.RawMessage
	if err := json.Unmarshal(payload, &object); err != nil {
		t.Fatal(err)
	}
	delete(object, field)
	result, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func operationEnvelopeWithNullField(t *testing.T, payload []byte, field string) []byte {
	t.Helper()
	var object map[string]json.RawMessage
	if err := json.Unmarshal(payload, &object); err != nil {
		t.Fatal(err)
	}
	object[field] = json.RawMessage("null")
	result, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func operationError(t *testing.T, payload []byte, expected ModelRoutingOperation, want ModelRoutingResponseErrorKind) (*ModelRoutingResponseError, error) {
	t.Helper()
	_, err := decodeModelRoutingOperationEnvelope(payload, expected)
	if err == nil {
		t.Fatal("decodeModelRoutingOperationEnvelope accepted invalid response")
	}
	var typed *ModelRoutingResponseError
	if !errors.As(err, &typed) || typed.Kind != want {
		t.Fatalf("error = %T %v; want kind %q", err, err, want)
	}
	return typed, err
}

func TestDecodeModelRoutingOperationEnvelopeSuccessAndRawResult(t *testing.T) {
	result := `{"arbitrary":true,"nested":[1,"two",{"futureField":false}]}`
	for _, tt := range []struct {
		name      string
		operation ModelRoutingOperation
		ok        bool
		exitClass string
	}{
		{name: "validate success", operation: ModelRoutingOperationValidate, ok: true, exitClass: "success"},
		{name: "validate false", operation: ModelRoutingOperationValidate, ok: false, exitClass: "invalid-input"},
		{name: "apply success", operation: ModelRoutingOperationApply, ok: true, exitClass: "success"},
		{name: "apply false without exit interpretation", operation: ModelRoutingOperationApply, ok: false, exitClass: "success"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodeModelRoutingOperationEnvelope(operationEnvelopeFixture(tt.operation, tt.ok, tt.exitClass, result), tt.operation)
			if err != nil {
				t.Fatal(err)
			}
			if got.Version != 1 || got.Contract != modelRoutingContract || got.Operation != tt.operation || got.OK != tt.ok || got.ExitClass != tt.exitClass {
				t.Fatalf("envelope = %#v", got)
			}
			if string(got.Result) != result {
				t.Fatalf("result = %s; want %s", got.Result, result)
			}
		})
	}
}

func TestDecodeModelRoutingOperationEnvelopeRejectsNonObjectsAndTrailing(t *testing.T) {
	valid := operationEnvelopeFixture(ModelRoutingOperationValidate, true, "success", `{}`)
	for _, tt := range []struct {
		name    string
		payload []byte
	}{
		{name: "empty", payload: nil},
		{name: "whitespace", payload: []byte(" \n\t")},
		{name: "null", payload: []byte("null")},
		{name: "array", payload: []byte("[]")},
		{name: "string", payload: []byte(`"result"`)},
		{name: "number", payload: []byte("1")},
		{name: "boolean", payload: []byte("false")},
		{name: "syntax", payload: []byte(`{"version":`)},
		{name: "trailing value", payload: append(append([]byte(nil), valid...), []byte(" {}")...)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			operationError(t, tt.payload, ModelRoutingOperationValidate, ModelRoutingResponseErrorMalformed)
		})
	}
}

func TestDecodeModelRoutingOperationEnvelopeRejectsRecursiveDuplicates(t *testing.T) {
	valid := operationEnvelopeFixture(ModelRoutingOperationValidate, true, "success", `{"nested":[{"key":1,"key":2}]}`)
	_, err := decodeModelRoutingOperationEnvelope(valid, ModelRoutingOperationValidate)
	if err == nil {
		t.Fatal("decoder accepted duplicate key nested in result")
	}
	var duplicate *duplicateJSONKeyError
	if !errors.As(err, &duplicate) || duplicate.Key != "key" || !errors.Is(err, ErrModelRoutingResponseMalformed) {
		t.Fatalf("duplicate error = %T %v", err, err)
	}

	outer := []byte(strings.Replace(string(operationEnvelopeFixture(ModelRoutingOperationValidate, true, "success", `{}`)), `"version":1,`, `"version":1,"version":1,`, 1))
	operationError(t, outer, ModelRoutingOperationValidate, ModelRoutingResponseErrorMalformed)
}

func TestDecodeModelRoutingOperationEnvelopeDisallowsOnlyOuterUnknownFields(t *testing.T) {
	result := `{"operationSpecificUnknown":true,"future":{"value":1}}`
	valid := operationEnvelopeFixture(ModelRoutingOperationValidate, true, "success", result)
	got, err := decodeModelRoutingOperationEnvelope(valid, ModelRoutingOperationValidate)
	if err != nil || string(got.Result) != result {
		t.Fatalf("raw result = %s, err = %v", got.Result, err)
	}
	unknownOuter := []byte(strings.Replace(string(valid), `,"result":`, `,"extra":true,"result":`, 1))
	operationError(t, unknownOuter, ModelRoutingOperationValidate, ModelRoutingResponseErrorMalformed)
}

func TestDecodeModelRoutingOperationEnvelopeRequiresEveryNonNullField(t *testing.T) {
	valid := operationEnvelopeFixture(ModelRoutingOperationValidate, true, "success", `{}`)
	for _, field := range []string{"version", "contract", "operation", "ok", "exitClass", "result"} {
		t.Run("missing "+field, func(t *testing.T) {
			operationError(t, operationEnvelopeWithoutField(t, valid, field), ModelRoutingOperationValidate, ModelRoutingResponseErrorMalformed)
		})
		t.Run("null "+field, func(t *testing.T) {
			operationError(t, operationEnvelopeWithNullField(t, valid, field), ModelRoutingOperationValidate, ModelRoutingResponseErrorMalformed)
		})
	}
}

func TestDecodeModelRoutingOperationEnvelopeRejectsWrongIdentity(t *testing.T) {
	valid := string(operationEnvelopeFixture(ModelRoutingOperationValidate, true, "success", `{}`))
	for _, tt := range []struct {
		name      string
		payload   string
		kind      ModelRoutingResponseErrorKind
		sentinel  error
		operation ModelRoutingOperation
	}{
		{name: "version", payload: strings.Replace(valid, `"version":1`, `"version":2`, 1), kind: ModelRoutingResponseErrorUnsupportedVersion, sentinel: ErrModelRoutingResponseUnsupportedVersion, operation: ModelRoutingOperationValidate},
		{name: "contract", payload: strings.Replace(valid, `"contract":"`+modelRoutingContract+`"`, `"contract":"other/v1"`, 1), kind: ModelRoutingResponseErrorUnsupportedContract, sentinel: ErrModelRoutingResponseUnsupportedContract, operation: ModelRoutingOperationValidate},
		{name: "expected operation", payload: strings.Replace(valid, `"operation":"validate"`, `"operation":"apply"`, 1), kind: ModelRoutingResponseErrorUnsupportedOperation, sentinel: ErrModelRoutingResponseUnsupportedOperation, operation: ModelRoutingOperationApply},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := operationError(t, []byte(tt.payload), ModelRoutingOperationValidate, tt.kind)
			if got.Operation != tt.operation || got.ExpectedOperation != ModelRoutingOperationValidate || !errors.Is(err, tt.sentinel) {
				t.Fatalf("error = %#v, %v", got, err)
			}
		})
	}
}

func TestDecodeModelRoutingOperationEnvelopeBoundsAndOwnership(t *testing.T) {
	result := `{"value":"owned"}`
	payload := operationEnvelopeFixture(ModelRoutingOperationValidate, false, "invalid-input", result)
	exact := append(append([]byte(nil), payload...), bytes.Repeat([]byte(" "), MaxModelRoutingResponseBytes-len(payload))...)
	got, err := decodeModelRoutingOperationEnvelope(exact, ModelRoutingOperationValidate)
	if err != nil || string(got.Result) != result || got.OK {
		t.Fatalf("exact bound = %#v, err = %v", got, err)
	}

	oversizedResult := `"` + strings.Repeat("x", MaxModelRoutingResponseBytes) + `"`
	oversized, err := operationError(t, operationEnvelopeFixture(ModelRoutingOperationApply, true, "success", oversizedResult), ModelRoutingOperationApply, ModelRoutingResponseErrorOversized)
	if oversized.Operation != ModelRoutingOperationApply || !errors.Is(err, ErrModelRoutingResponseOversized) {
		t.Fatalf("oversized = %#v, %v", oversized, err)
	}

	ownedPayload := operationEnvelopeFixture(ModelRoutingOperationValidate, true, "success", result)
	owned, err := decodeModelRoutingOperationEnvelope(ownedPayload, ModelRoutingOperationValidate)
	if err != nil {
		t.Fatal(err)
	}
	for i := range ownedPayload {
		ownedPayload[i] = 'x'
	}
	if string(owned.Result) != result || owned.Contract != modelRoutingContract || owned.ExitClass != "success" {
		t.Fatalf("envelope aliases input = %#v", owned)
	}
	owned.Result[0] = 'x'
	if string(owned.Result) == result {
		t.Fatal("test did not mutate owned result")
	}
}

func TestModelRoutingResponseErrorPreservesCauseWithoutResultContent(t *testing.T) {
	_, err := operationError(t, []byte(`{"version":1,}`), ModelRoutingOperationApply, ModelRoutingResponseErrorMalformed)
	var syntax *json.SyntaxError
	if !errors.As(err, &syntax) || !errors.Is(err, ErrModelRoutingResponseMalformed) {
		t.Fatalf("cause = %T %v", err, err)
	}

	payload := operationEnvelopeFixture(ModelRoutingOperationApply, true, "success", `{"provider-secret":1,"provider-secret":2}`)
	_, err = operationError(t, payload, ModelRoutingOperationApply, ModelRoutingResponseErrorMalformed)
	if strings.Contains(err.Error(), "provider-secret") || strings.Contains(err.Error(), "result") {
		t.Fatalf("error exposed result content: %v", err)
	}
}
