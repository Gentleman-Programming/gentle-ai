package model

import "testing"

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

func TestSelection_HasAgent(t *testing.T) {
	s := Selection{
		Agents: []AgentID{AgentClaudeCode, AgentOpenCode, AgentAntigravity},
	}

	tests := []struct {
		agent AgentID
		want  bool
	}{
		{AgentClaudeCode, true},
		{AgentOpenCode, true},
		{AgentAntigravity, true},
		{AgentCursor, false},
		{"unknown-agent", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.agent), func(t *testing.T) {
			if got := s.HasAgent(tt.agent); got != tt.want {
				t.Errorf("Selection.HasAgent(%q) = %t, want %t", tt.agent, got, tt.want)
			}
		})
	}
}

func TestSelection_HasComponent(t *testing.T) {
	s := Selection{
		Components: []ComponentID{ComponentEngram, ComponentSDD, ComponentSkills},
	}

	tests := []struct {
		component ComponentID
		want      bool
	}{
		{ComponentEngram, true},
		{ComponentSDD, true},
		{ComponentSkills, true},
		{ComponentPersona, false},
		{"unknown-component", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.component), func(t *testing.T) {
			if got := s.HasComponent(tt.component); got != tt.want {
				t.Errorf("Selection.HasComponent(%q) = %t, want %t", tt.component, got, tt.want)
			}
		})
	}
}
