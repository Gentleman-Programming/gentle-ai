package pi

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const validCapabilitiesResponse = `{"version":1,"contract":"gentle-pi.model-routing/v1","operation":"capabilities","ok":true,"exitClass":"success","result":{"contract":"gentle-pi.model-routing/v1","supported":true,"operations":["capabilities","inspect","validate","apply"]}}`

func TestParseCapabilitiesResponse(t *testing.T) {
	cases := []struct {
		name string
		body string
		kind CapabilitiesErrorKind
	}{
		{name: "malformed", body: `{"version":`, kind: CapabilitiesErrorMalformed},
		{name: "trailing value", body: validCapabilitiesResponse + ` {}`, kind: CapabilitiesErrorMalformed},
		{name: "duplicate envelope key", body: strings.Replace(validCapabilitiesResponse, `"version":1`, `"version":1,"version":1`, 1), kind: CapabilitiesErrorMalformed},
		{name: "duplicate result key", body: strings.Replace(validCapabilitiesResponse, `"contract":"gentle-pi.model-routing/v1","supported"`, `"contract":"gentle-pi.model-routing/v1","contract":"gentle-pi.model-routing/v1","supported"`, 1), kind: CapabilitiesErrorMalformed},
		{name: "unknown envelope field", body: strings.Replace(validCapabilitiesResponse, `,"result"`, `,"extra":true,"result"`, 1), kind: CapabilitiesErrorMalformed},
		{name: "wrong version", body: strings.Replace(validCapabilitiesResponse, `"version":1`, `"version":2`, 1), kind: CapabilitiesErrorUnsupportedVersion},
		{name: "wrong contract", body: strings.Replace(validCapabilitiesResponse, `"contract":"gentle-pi.model-routing/v1"`, `"contract":"other/v1"`, 1), kind: CapabilitiesErrorUnsupportedContract},
		{name: "wrong operation", body: strings.Replace(validCapabilitiesResponse, `"operation":"capabilities"`, `"operation":"inspect"`, 1), kind: CapabilitiesErrorUnsupportedOperation},
		{name: "wrong exit class", body: strings.Replace(validCapabilitiesResponse, `"exitClass":"success"`, `"exitClass":"invalid-input"`, 1), kind: CapabilitiesErrorExplicitRemoteFailure},
		{name: "omitted ok", body: strings.Replace(validCapabilitiesResponse, `"ok":true,`, "", 1), kind: CapabilitiesErrorMalformed},
		{name: "false ok", body: strings.Replace(validCapabilitiesResponse, `"ok":true`, `"ok":false`, 1), kind: CapabilitiesErrorExplicitRemoteFailure},
		{name: "missing operation", body: strings.Replace(validCapabilitiesResponse, `,"apply"`, "", 1), kind: CapabilitiesErrorInvalidOperations},
		{name: "duplicate operation", body: strings.Replace(validCapabilitiesResponse, `,"apply"]`, `,"apply","apply"]`, 1), kind: CapabilitiesErrorInvalidOperations},
		{name: "unknown top-level operations", body: strings.Replace(validCapabilitiesResponse, `,"result"`, `,"operations":["inspect"],"result"`, 1), kind: CapabilitiesErrorInvalidOperations},
		{name: "nested alternative operations", body: strings.Replace(validCapabilitiesResponse, `"result":{`, `"result":{"capabilities":{"operations":["capabilities","inspect","validate","apply"]},`, 1), kind: CapabilitiesErrorInvalidOperations},
		{name: "conflicting operation representations", body: strings.Replace(validCapabilitiesResponse, `,"result"`, `,"operations":["apply"],"result"`, 1), kind: CapabilitiesErrorInvalidOperations},
		{name: "explicit unsupported result", body: strings.Replace(validCapabilitiesResponse, `"supported":true`, `"supported":false`, 1), kind: CapabilitiesErrorUnsupportedContract},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseCapabilitiesResponse([]byte(tc.body))
			if err == nil {
				t.Fatal("ParseCapabilitiesResponse() accepted invalid response")
			}
			var typed *CapabilitiesError
			if !errors.As(err, &typed) {
				t.Fatalf("error = %T %v; want *CapabilitiesError", err, err)
			}
			if typed.Kind != tc.kind {
				t.Fatalf("CapabilitiesError.Kind = %q, want %q", typed.Kind, tc.kind)
			}
		})
	}

	got, err := ParseCapabilitiesResponse([]byte(validCapabilitiesResponse))
	if err != nil {
		t.Fatalf("ParseCapabilitiesResponse() error = %v", err)
	}
	want := Capabilities{Contract: modelRoutingContract, Supported: true, Operations: []string{"capabilities", "inspect", "validate", "apply"}}
	if got.Contract != want.Contract || got.Supported != want.Supported || !slicesEqual(got.Operations, want.Operations) {
		t.Fatalf("capabilities = %#v, want %#v", got, want)
	}
}

func TestParseCapabilitiesResponseBoundsAndUnwrap(t *testing.T) {
	exact := append([]byte(validCapabilitiesResponse), bytes.Repeat([]byte(" "), MaxCapabilitiesResponseBytes-len(validCapabilitiesResponse))...)
	if _, err := ParseCapabilitiesResponse(exact); err != nil {
		t.Fatalf("ParseCapabilitiesResponse(exact bound) error = %v", err)
	}

	_, err := ParseCapabilitiesResponse(bytes.Repeat([]byte("x"), MaxCapabilitiesResponseBytes+1))
	if err == nil {
		t.Fatal("ParseCapabilitiesResponse() accepted oversized response")
	}
	var typed *CapabilitiesError
	if !errors.As(err, &typed) || typed.Kind != CapabilitiesErrorOversized || typed.Unwrap() == nil {
		t.Fatalf("oversized error = %T %#v; want typed error with cause", err, err)
	}

	_, err = ParseCapabilitiesResponse([]byte(`{"version":1,}`))
	var syntax *json.SyntaxError
	if !errors.As(err, &syntax) {
		t.Fatalf("malformed error = %T %v; want wrapped json.SyntaxError", err, err)
	}
	if !errors.Is(err, ErrCapabilitiesMalformed) || errors.Unwrap(err) == nil {
		t.Fatal("malformed error does not expose its class and cause through Unwrap")
	}
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
