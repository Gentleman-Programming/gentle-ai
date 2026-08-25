package pi

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func applyResult(ok bool, outcome string, saved bool, suffix string) string {
	okValue, savedValue := "false", "false"
	if ok {
		okValue = "true"
	}
	if saved {
		savedValue = "true"
	}
	return `{"contract":"` + modelRoutingContract + `","ok":` + okValue + `,"outcome":"` + outcome + `","saved":` + savedValue + `,"diagnostics":[]` + suffix + `}`
}
func applyEnvelope(result string, ok bool, class string) []byte {
	return operationEnvelopeFixture(ModelRoutingOperationApply, ok, class, result)
}
func applyEdit(t *testing.T, result, field, value string) string {
	t.Helper()
	var object map[string]json.RawMessage
	if err := json.Unmarshal([]byte(result), &object); err != nil {
		t.Fatal(err)
	}
	if value == "-" {
		delete(object, field)
	} else {
		object[field] = json.RawMessage(value)
	}
	data, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
func applyMalformed(t *testing.T, payload []byte) error {
	t.Helper()
	_, err := ParseModelRoutingApplyResponse(payload)
	if err == nil {
		t.Fatal("ParseModelRoutingApplyResponse accepted invalid response")
	}
	var typed *ModelRoutingResponseError
	if !errors.As(err, &typed) || typed.Kind != ModelRoutingResponseErrorMalformed || typed.Operation != ModelRoutingOperationApply || typed.ExpectedOperation != ModelRoutingOperationApply || !errors.Is(err, ErrModelRoutingResponseMalformed) {
		t.Fatalf("error = %T %v; want generic apply malformed error", err, err)
	}
	return err
}
func TestParseModelRoutingApplyResponseOutcomesAndMaterialization(t *testing.T) {
	full := `,"target":"global","configPath":"/cfg/model-routing.json","materialization":{"affected":["global","project"],"succeeded":["global"],"failed":[{"target":"project","message":"write failed"}]}`
	cases := []struct {
		name, outcome, class                  string
		envelopeOK, resultOK, saved, complete bool
	}{
		{"success", "success", "success", true, true, true, true},
		{"validation failure", "validation-failure", "invalid-input", false, false, false, false},
		{"unavailable runtime", "unavailable-runtime", "unavailable-runtime", false, false, false, false},
		{"persistence failure", "persistence-failure", "persistence-failure", false, false, false, false},
		{"partial saved", "partial", "success", false, false, true, true},
		{"mismatch remains data", "persistence-failure", "success", true, false, true, false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			suffix := ""
			if tt.complete {
				suffix = full
			}
			got, err := ParseModelRoutingApplyResponse(applyEnvelope(applyResult(tt.resultOK, tt.outcome, tt.saved, suffix), tt.envelopeOK, tt.class))
			if err != nil {
				t.Fatal(err)
			}
			if got.Operation != ModelRoutingOperationApply || got.OK != tt.envelopeOK || got.ExitClass != tt.class || got.Result.OK != tt.resultOK || got.Result.Saved != tt.saved || string(got.Result.Outcome) != tt.outcome {
				t.Fatalf("response = %#v", got)
			}
			if tt.complete {
				if got.Result.Target == nil || *got.Result.Target != ModelRoutingTargetGlobal || got.Result.ConfigPath == nil || *got.Result.ConfigPath != "/cfg/model-routing.json" || got.Result.Materialization == nil || len(got.Result.Materialization.Affected) != 2 || len(got.Result.Materialization.Succeeded) != 1 || len(got.Result.Materialization.Failed) != 1 {
					t.Fatalf("materialization = %#v", got.Result)
				}
			} else if got.Result.Target != nil || got.Result.ConfigPath != nil || got.Result.Materialization != nil {
				t.Fatalf("omitted optionals = %#v", got.Result)
			}
		})
	}
}
func TestParseModelRoutingApplyResponseRequiresNonNullResultFields(t *testing.T) {
	base := applyResult(true, "success", true, "")
	for _, field := range []string{"contract", "ok", "outcome", "saved", "diagnostics"} {
		for _, value := range []string{"-", "null"} {
			t.Run(field+"/"+value, func(t *testing.T) {
				applyMalformed(t, applyEnvelope(applyEdit(t, base, field, value), true, "success"))
			})
		}
	}
	for _, field := range []string{"target", "configPath", "materialization"} {
		t.Run(field+"/null", func(t *testing.T) {
			applyMalformed(t, applyEnvelope(applyEdit(t, base, field, "null"), true, "success"))
		})
	}
}
func TestParseModelRoutingApplyResponseRejectsInvalidEnumsAndNestedUnknown(t *testing.T) {
	full := applyResult(true, "success", true, `,"target":"global","materialization":{"affected":[],"succeeded":[],"failed":[{"target":"project","message":"m"}]}`)
	for _, tt := range []struct{ name, field, value string }{
		{"outcome", "outcome", `"bogus"`}, {"target", "target", `"workspace"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			applyMalformed(t, applyEnvelope(applyEdit(t, full, tt.field, tt.value), true, "success"))
		})
	}
	for _, tt := range []struct{ name, old, new string }{
		{"result", `,"diagnostics":[]`, `,"extra":true,"diagnostics":[]`},
		{"materialization", `"affected":[]`, `"extra":true,"affected":[]`},
		{"failure", `"message":"m"`, `"extra":true,"message":"m"`},
	} {
		t.Run("unknown "+tt.name, func(t *testing.T) {
			applyMalformed(t, applyEnvelope(strings.Replace(full, tt.old, tt.new, 1), true, "success"))
		})
	}
}
func TestParseModelRoutingApplyResponseRejectsMalformedMaterialization(t *testing.T) {
	base := applyResult(true, "success", true, `,"materialization":{"affected":[],"succeeded":[],"failed":[]}`)
	for _, tt := range []struct{ name, old, new string }{
		{"affected null", `"affected":[]`, `"affected":null`}, {"affected object", `"affected":[]`, `"affected":{}`}, {"affected non-string", `"affected":[]`, `"affected":[1]`},
		{"succeeded non-string", `"succeeded":[]`, `"succeeded":[null]`}, {"failed object", `"failed":[]`, `"failed":{}`}, {"missing failure target", `"failed":[]`, `"failed":[{"message":"m"}]`}, {"null failure message", `"failed":[]`, `"failed":[{"target":"project","message":null}]`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			applyMalformed(t, applyEnvelope(strings.Replace(base, tt.old, tt.new, 1), true, "success"))
		})
	}
}
func TestParseModelRoutingApplyResponseUsesCanonicalDiagnostics(t *testing.T) {
	base := applyResult(true, "success", true, "")
	valid := applyEdit(t, base, "diagnostics", `[{"code":"C","message":"M","severity":"warning","path":"targets"}]`)
	got, err := ParseModelRoutingApplyResponse(applyEnvelope(valid, true, "success"))
	if err != nil || len(got.Result.Diagnostics) != 1 || got.Result.Diagnostics[0].Path == nil {
		t.Fatalf("diagnostics = %#v, err=%v", got.Result.Diagnostics, err)
	}
	for _, value := range []string{`[{"code":"C","message":"M","severity":"warning","extra":true}]`, `[{"message":"M","severity":"warning"}]`, `[{"code":"C","message":"M","severity":"bogus"}]`, `[{"code":"C","message":"M","severity":"warning","path":null}]`} {
		applyMalformed(t, applyEnvelope(applyEdit(t, base, "diagnostics", value), true, "success"))
	}
}
func TestParseModelRoutingApplyResponsePropagatesEnvelopeAndOwnsData(t *testing.T) {
	valid := applyEnvelope(applyResult(true, "success", true, ""), true, "success")
	wrong := []byte(strings.Replace(string(valid), `"operation":"apply"`, `"operation":"validate"`, 1))
	_, err := ParseModelRoutingApplyResponse(wrong)
	var typed *ModelRoutingResponseError
	if !errors.As(err, &typed) || typed.Kind != ModelRoutingResponseErrorUnsupportedOperation || !errors.Is(err, ErrModelRoutingResponseUnsupportedOperation) || typed.Operation != ModelRoutingOperationValidate {
		t.Fatalf("wrong operation error = %#v, %v", typed, err)
	}
	var syntax *json.SyntaxError
	if !errors.As(applyMalformed(t, []byte(`{"version":1,}`)), &syntax) {
		t.Fatal("malformed envelope lost syntax cause")
	}
	bad := applyEdit(t, applyResult(true, "success", true, ""), "diagnostics", `[{"code":"C","message":"M","severity":1}]`)
	var typeErr *json.UnmarshalTypeError
	if !errors.As(applyMalformed(t, applyEnvelope(bad, true, "success")), &typeErr) {
		t.Fatal("result lost type cause")
	}
	payload := applyEnvelope(applyEdit(t, applyResult(true, "success", true, `,"target":"global","configPath":"/cfg"`), "diagnostics", `[{"code":"C","message":"M","severity":"error","path":"p"}]`), true, "success")
	original := append([]byte(nil), payload...)
	got, err := ParseModelRoutingApplyResponse(payload)
	if err != nil {
		t.Fatal(err)
	}
	for i := range payload {
		payload[i] = 'x'
	}
	*got.Result.Target, *got.Result.ConfigPath = ModelRoutingTargetProject, "/mutated"
	*got.Result.Diagnostics[0].Path = "mutated"
	again, err := ParseModelRoutingApplyResponse(original)
	if err != nil || *again.Result.Target != ModelRoutingTargetGlobal || *again.Result.ConfigPath != "/cfg" || *again.Result.Diagnostics[0].Path != "p" {
		t.Fatalf("parsed data not owned: %#v, %v", again, err)
	}
}
