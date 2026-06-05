package model

import (
	"testing"
)

// TestSelectionHasStrictTDDField verifies that the Selection struct has a
// StrictTDD bool field.
func TestSelectionHasStrictTDDField(t *testing.T) {
	s := Selection{}
	// Field must be accessible and default to false.
	if s.StrictTDD {
		t.Fatal("Selection.StrictTDD default = true, want false")
	}

	s.StrictTDD = true
	if !s.StrictTDD {
		t.Fatal("Selection.StrictTDD set to true but read back as false")
	}
}

// TestSyncOverridesHasStrictTDDPointer verifies that SyncOverrides has a
// *bool StrictTDD field (nil = no override semantics).
func TestSyncOverridesHasStrictTDDPointer(t *testing.T) {
	o := SyncOverrides{}
	// Nil means "no override".
	if o.StrictTDD != nil {
		t.Fatal("SyncOverrides.StrictTDD default = non-nil, want nil")
	}

	enabled := true
	o.StrictTDD = &enabled
	if o.StrictTDD == nil || !*o.StrictTDD {
		t.Fatal("SyncOverrides.StrictTDD pointer set to true but read back incorrectly")
	}

	disabled := false
	o.StrictTDD = &disabled
	if o.StrictTDD == nil || *o.StrictTDD {
		t.Fatal("SyncOverrides.StrictTDD pointer set to false but read back incorrectly")
	}
}

// TestSelectionHasCodexModelAssignments verifies that the Selection struct has a
// CodexModelAssignments map field.
func TestSelectionHasCodexModelAssignments(t *testing.T) {
	s := Selection{}
	// Zero value is nil.
	if s.CodexModelAssignments != nil {
		t.Fatal("Selection.CodexModelAssignments zero value should be nil")
	}

	s.CodexModelAssignments = map[string]CodexEffort{"sdd-apply": CodexEffortHigh}
	if s.CodexModelAssignments["sdd-apply"] != CodexEffortHigh {
		t.Fatal("Selection.CodexModelAssignments not accessible after assignment")
	}
}

// TestSyncOverridesCodexModelPreset verifies that SyncOverrides has a
// CodexModelAssignments map field (nil = no override semantics).
func TestSyncOverridesCodexModelPreset(t *testing.T) {
	o := SyncOverrides{}
	if o.CodexModelAssignments != nil {
		t.Fatal("SyncOverrides.CodexModelAssignments zero value should be nil")
	}

	o.CodexModelAssignments = map[string]CodexEffort{"default": CodexEffortMedium}
	if o.CodexModelAssignments == nil {
		t.Fatal("SyncOverrides.CodexModelAssignments should be non-nil after assignment")
	}
}
