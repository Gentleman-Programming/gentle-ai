package sddstatus

import (
	"encoding/json"
	"testing"
)

func TestStatusV2RecordStoreAdmitsUndeclaredAndOpenRecord(t *testing.T) {
	tests := []struct {
		value RecordStore
		want  bool
	}{
		{RecordStoreUndeclared, true},
		{RecordStoreOpenRecord, true},
		{RecordStore("something-else"), false},
	}
	for _, tt := range tests {
		if got := statusV2RecordStore(tt.value); got != tt.want {
			t.Errorf("statusV2RecordStore(%q) = %v, want %v", tt.value, got, tt.want)
		}
	}
}

func TestProjectStatusV2RejectsUnsupportedRecordStore(t *testing.T) {
	status := baseStatus(ArtifactStoreOpenSpec, "/repo", nil, nil, nil, "apply", nil)
	status.RecordStore = RecordStoreDeclaration{Declared: "bogus", Resolved: RecordStore("bogus")}
	if _, err := ProjectStatusV2(status); err == nil {
		t.Fatal("ProjectStatusV2() with an unsupported record store did not error")
	}
}

func TestProjectStatusV2CarriesRecordStore(t *testing.T) {
	status := baseStatus(ArtifactStoreOpenSpec, "/repo", nil, nil, nil, "apply", nil)
	status.RecordStore = RecordStoreDeclaration{Declared: "openrecord", Resolved: RecordStoreOpenRecord}

	projected, err := ProjectStatusV2(status)
	if err != nil {
		t.Fatalf("ProjectStatusV2() error = %v", err)
	}
	if projected.RecordStore.Declared != "openrecord" || projected.RecordStore.Resolved != RecordStoreOpenRecord {
		t.Fatalf("projected RecordStore = %+v, want {Declared: openrecord, Resolved: openrecord}", projected.RecordStore)
	}

	raw, err := json.Marshal(projected)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	recordStore, ok := decoded["recordStore"].(map[string]any)
	if !ok {
		t.Fatalf("recordStore field missing or wrong shape in projected JSON: %s", raw)
	}
	if recordStore["declared"] != "openrecord" || recordStore["resolved"] != "openrecord" {
		t.Errorf("recordStore JSON = %+v, want declared/resolved = openrecord", recordStore)
	}
}

// TestProjectStatusV2PreExistingFieldsUnchanged pins that every field present
// before this change carries the same value it did before, with an undeclared
// record store (the default for every workspace that predates this change).
func TestProjectStatusV2PreExistingFieldsUnchanged(t *testing.T) {
	status := baseStatus(ArtifactStoreEngram, "/repo", nil, nil, nil, "apply", nil)

	projected, err := ProjectStatusV2(status)
	if err != nil {
		t.Fatalf("ProjectStatusV2() error = %v", err)
	}
	if projected.RecordStore.Declared != "" || projected.RecordStore.Resolved != RecordStoreUndeclared {
		t.Fatalf("an undeclared workspace projected RecordStore = %+v, want the empty declaration", projected.RecordStore)
	}

	raw, err := json.Marshal(projected)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	wantUnchanged := map[string]any{
		"schemaName":      SchemaName,
		"schemaVersion":   float64(SchemaVersion),
		"artifactStore":   "engram",
		"nextRecommended": "apply",
	}
	for key, want := range wantUnchanged {
		if got := decoded[key]; got != want {
			t.Errorf("decoded[%q] = %v, want %v — recordStore must not have altered a pre-existing field", key, got, want)
		}
	}
	if _, ok := decoded["recordStore"]; !ok {
		t.Error("recordStore is missing from the projected document")
	}
}
