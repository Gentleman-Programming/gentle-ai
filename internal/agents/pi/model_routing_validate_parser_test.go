package pi

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func modelRoutingValidateResult(ok bool, diagnostics string) string {
	value := "false"
	if ok {
		value = "true"
	}
	return `{"contract":"` + modelRoutingContract + `","ok":` + value + `,"diagnostics":` + diagnostics + `}`
}

func modelRoutingValidateEnvelope(result string, ok bool, exitClass string) []byte {
	value := "false"
	if ok {
		value = "true"
	}
	return []byte(`{"version":1,"contract":"` + modelRoutingContract + `","operation":"validate","ok":` + value + `,"exitClass":"` + exitClass + `","result":` + result + `}`)
}

func modelRoutingValidateError(t *testing.T, payload []byte, want ModelRoutingResponseErrorKind) (*ModelRoutingResponseError, error) {
	t.Helper()
	_, err := ParseModelRoutingValidateResponse(payload)
	if err == nil {
		t.Fatal("ParseModelRoutingValidateResponse accepted invalid response")
	}
	var typed *ModelRoutingResponseError
	if !errors.As(err, &typed) || typed.Kind != want || typed.ExpectedOperation != ModelRoutingOperationValidate {
		t.Fatalf("error = %T %v; want validate %q", err, err, want)
	}
	return typed, err
}

func TestParseModelRoutingValidateResponseData(t *testing.T) {
	diagnostics := `[{"code":"warn","message":"check","severity":"warning"},{"code":"info","message":"context","severity":"info","path":"models"}]`
	for _, tt := range []struct {
		name       string
		envelopeOK bool
		resultOK   bool
		exitClass  string
		wantDiag   int
		wantPath   bool
	}{
		{name: "success with warning and info", envelopeOK: true, resultOK: true, exitClass: "success", wantDiag: 2, wantPath: true},
		{name: "invalid input is data", envelopeOK: false, resultOK: false, exitClass: "invalid-input", wantDiag: 2, wantPath: true},
		{name: "envelope and result mismatch remains data", envelopeOK: true, resultOK: false, exitClass: "success", wantDiag: 2, wantPath: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseModelRoutingValidateResponse(modelRoutingValidateEnvelope(modelRoutingValidateResult(tt.resultOK, diagnostics), tt.envelopeOK, tt.exitClass))
			if err != nil {
				t.Fatal(err)
			}
			if got.Version != 1 || got.Contract != modelRoutingContract || got.Operation != ModelRoutingOperationValidate || got.OK != tt.envelopeOK || got.ExitClass != tt.exitClass {
				t.Fatalf("response = %#v", got)
			}
			if got.Result.Contract != modelRoutingContract || got.Result.OK != tt.resultOK || len(got.Result.Diagnostics) != tt.wantDiag {
				t.Fatalf("result = %#v", got.Result)
			}
			if (got.Result.Diagnostics[1].Path != nil) != tt.wantPath || got.Result.Diagnostics[0].Severity != ModelRoutingDiagnosticSeverityWarning || got.Result.Diagnostics[1].Severity != ModelRoutingDiagnosticSeverityInfo {
				t.Fatalf("diagnostics = %#v", got.Result.Diagnostics)
			}
		})
	}
}

func TestParseModelRoutingValidateResponseRejectsStrictResultShape(t *testing.T) {
	base := modelRoutingValidateResult(true, `[{"code":"C","message":"M","severity":"error","path":"agents"}]`)
	cases := []struct {
		name string
		body string
	}{
		{name: "unknown result field", body: strings.Replace(base, `,"diagnostics"`, `,"extra":true,"diagnostics"`, 1)},
		{name: "unknown diagnostic field", body: strings.Replace(base, `,"path":"agents"`, `,"extra":true,"path":"agents"`, 1)},
		{name: "missing result contract", body: strings.Replace(base, `"contract":"`+modelRoutingContract+`",`, "", 1)},
		{name: "null result contract", body: strings.Replace(base, `"contract":"`+modelRoutingContract+`"`, `"contract":null`, 1)},
		{name: "wrong result contract", body: strings.Replace(base, modelRoutingContract, "other/v1", 1)},
		{name: "missing result ok", body: strings.Replace(base, `,"ok":true`, "", 1)},
		{name: "null result ok", body: strings.Replace(base, `,"ok":true`, `,"ok":null`, 1)},
		{name: "missing result diagnostics", body: strings.Replace(base, `,"diagnostics":[{"code":"C","message":"M","severity":"error","path":"agents"}]`, "", 1)},
		{name: "null result diagnostics", body: strings.Replace(base, `,"diagnostics":[{"code":"C","message":"M","severity":"error","path":"agents"}]`, `,"diagnostics":null`, 1)},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			modelRoutingValidateError(t, modelRoutingValidateEnvelope(tt.body, true, "success"), ModelRoutingResponseErrorMalformed)
		})
	}
}

func TestParseModelRoutingValidateResponseRejectsDiagnosticViolations(t *testing.T) {
	base := modelRoutingValidateResult(true, `[{"code":"C","message":"M","severity":"warning","path":"agents"}]`)
	for _, tt := range []struct {
		name string
		old  string
		new  string
	}{
		{name: "missing code", old: `"code":"C",`, new: ""},
		{name: "null code", old: `"code":"C"`, new: `"code":null`},
		{name: "missing message", old: `"message":"M",`, new: ""},
		{name: "null message", old: `"message":"M"`, new: `"message":null`},
		{name: "missing severity", old: `,"severity":"warning","path"`, new: `,"path"`},
		{name: "null severity", old: `"severity":"warning"`, new: `"severity":null`},
		{name: "invalid severity", old: `"severity":"warning"`, new: `"severity":"bogus"`},
		{name: "null optional path", old: `"path":"agents"`, new: `"path":null`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			body := strings.Replace(base, tt.old, tt.new, 1)
			modelRoutingValidateError(t, modelRoutingValidateEnvelope(body, true, "success"), ModelRoutingResponseErrorMalformed)
		})
	}
}

func TestParseModelRoutingValidateResponsePropagatesEnvelopeErrors(t *testing.T) {
	valid := modelRoutingValidateEnvelope(modelRoutingValidateResult(true, `[]`), true, "success")
	cases := []struct {
		name     string
		body     []byte
		kind     ModelRoutingResponseErrorKind
		sentinel error
	}{
		{name: "malformed", body: []byte(`{"version":1,}`), kind: ModelRoutingResponseErrorMalformed, sentinel: ErrModelRoutingResponseMalformed},
		{name: "wrong version", body: []byte(strings.Replace(string(valid), `"version":1`, `"version":2`, 1)), kind: ModelRoutingResponseErrorUnsupportedVersion, sentinel: ErrModelRoutingResponseUnsupportedVersion},
		{name: "wrong contract", body: []byte(strings.Replace(string(valid), modelRoutingContract, "other/v1", 1)), kind: ModelRoutingResponseErrorUnsupportedContract, sentinel: ErrModelRoutingResponseUnsupportedContract},
		{name: "wrong identity", body: []byte(strings.Replace(string(valid), `"operation":"validate"`, `"operation":"apply"`, 1)), kind: ModelRoutingResponseErrorUnsupportedOperation, sentinel: ErrModelRoutingResponseUnsupportedOperation},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			typed, err := modelRoutingValidateError(t, tt.body, tt.kind)
			if !errors.Is(err, tt.sentinel) || typed.Operation == "" {
				t.Fatalf("error = %#v, %v", typed, err)
			}
			if tt.name == "malformed" {
				var syntax *json.SyntaxError
				if !errors.As(err, &syntax) {
					t.Fatalf("malformed cause = %T %v", err, err)
				}
			}
		})
	}
}

func TestParseModelRoutingValidateResponseResultCauseAndOwnership(t *testing.T) {
	badResult := modelRoutingValidateResult(true, `[{"code":"C","message":"M","severity":1}]`)
	typed, err := modelRoutingValidateError(t, modelRoutingValidateEnvelope(badResult, true, "success"), ModelRoutingResponseErrorMalformed)
	if !errors.Is(err, ErrModelRoutingResponseMalformed) || typed.Operation != ModelRoutingOperationValidate {
		t.Fatalf("result error = %#v, %v", typed, err)
	}
	var typeErr *json.UnmarshalTypeError
	if !errors.As(err, &typeErr) {
		t.Fatalf("result cause = %T %v", err, err)
	}

	payload := modelRoutingValidateEnvelope(modelRoutingValidateResult(true, `[{"code":"C","message":"M","severity":"error","path":"agents"}]`), true, "success")
	original := append([]byte(nil), payload...)
	got, err := ParseModelRoutingValidateResponse(payload)
	if err != nil {
		t.Fatal(err)
	}
	for i := range payload {
		payload[i] = 'x'
	}
	got.Result.Diagnostics[0].Code = "mutated"
	*got.Result.Diagnostics[0].Path = "mutated"
	again, err := ParseModelRoutingValidateResponse(original)
	if err != nil || again.Result.Diagnostics[0].Code != "C" || *again.Result.Diagnostics[0].Path != "agents" {
		t.Fatalf("parsed values share state: %#v, %v", again, err)
	}
}
