package model

import (
	"encoding/json"
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

func TestSelectionHasPiCodingAgentInAgentList(t *testing.T) {
	s := Selection{Agents: []AgentID{AgentPiCodingAgent, AgentOpenCode}}

	if !s.HasAgent(AgentPiCodingAgent) {
		t.Fatal("Selection.HasAgent(AgentPiCodingAgent) = false, want true")
	}

	if s.HasAgent(AgentClaudeCode) {
		t.Fatal("Selection.HasAgent(AgentClaudeCode) = true, want false")
	}
}

func TestSelectionPiCodingAgentJSONRoundTrip(t *testing.T) {
	original := Selection{Agents: []AgentID{AgentPiCodingAgent}}

	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	decoded := Selection{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if len(decoded.Agents) != 1 {
		t.Fatalf("decoded agents length = %d, want 1", len(decoded.Agents))
	}

	if decoded.Agents[0] != AgentPiCodingAgent {
		t.Fatalf("decoded agent = %q, want %q", decoded.Agents[0], AgentPiCodingAgent)
	}
}
