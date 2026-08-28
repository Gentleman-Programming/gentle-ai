package sddstatus

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestRuntimeStatusRetiresBindingFields(t *testing.T) {
	statusType := reflect.TypeOf(RuntimeStatus{})
	for _, name := range []string{"Binding", "BindingRevision"} {
		if _, exists := statusType.FieldByName(name); exists {
			t.Fatalf("runtime status still carries retired %s field", name)
		}
	}
}

func TestRuntimeRecordRetiresBindingField(t *testing.T) {
	rejectRetiredRuntimeRecordField(t, `{"binding":{}}`, "binding")
}

func TestRuntimeRecordRetiresReceiptField(t *testing.T) {
	rejectRetiredRuntimeRecordField(t, `{"receipt":{}}`, "receipt")
}

func rejectRetiredRuntimeRecordField(t *testing.T, payload, field string) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewBufferString(payload))
	decoder.DisallowUnknownFields()
	var record runtimeRecord
	if err := decoder.Decode(&record); err == nil || !strings.Contains(err.Error(), `json: unknown field "`+field+`"`) {
		t.Fatalf("strict runtime record decode for %q = %v, want unknown field rejection", field, err)
	}
}

func TestRuntimeRecordRejectsRetiredBindingSetOperation(t *testing.T) {
	record := runtimeRecord{
		Schema: runtimeRecordSchema, Change: "retired-binding-set", Operation: "binding/set",
		RequestID: "retired-binding-set", RequestDigest: runtimeTestHash('a'),
	}
	// #3816: assert the typed condition rather than a bespoke message. The
	// message collapsed; the condition is the information it carried.
	if err := validateRuntimeRecordShape(record); !isRuntimeRecordRejection(err, "invalid_operation") {
		t.Fatalf("binding/set dispatch = %v, want the invalid_operation rejection", err)
	}
}
