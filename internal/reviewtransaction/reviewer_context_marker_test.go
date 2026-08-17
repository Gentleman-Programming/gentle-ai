package reviewtransaction

import "testing"

// TestReviewerContextMarkerForResolvesTheClosedThreeKeyTable pins the exact
// marker every mapped agent identity resolves to, and proves the omitted-agent
// default stays the generic marker every relay-agnostic reviewer already
// expects.
func TestReviewerContextMarkerForResolvesTheClosedThreeKeyTable(t *testing.T) {
	tests := []struct {
		name           string
		agent          string
		wantHeader     string
		wantTerminator string
	}{
		{name: "omitted agent defaults to the generic marker", agent: "", wantHeader: "GENTLE_AI_REVIEW_CONTEXT", wantTerminator: "GENTLE_AI_REVIEW_CONTEXT_END"},
		{name: "claude-code resolves its own marker", agent: "claude-code", wantHeader: "GENTLE_AI_CLAUDE_REVIEW_CONTEXT", wantTerminator: "GENTLE_AI_CLAUDE_REVIEW_CONTEXT_END"},
		{name: "opencode resolves the generic marker verbatim", agent: "opencode", wantHeader: "GENTLE_AI_REVIEW_CONTEXT", wantTerminator: "GENTLE_AI_REVIEW_CONTEXT_END"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			marker, ok := ReviewerContextMarkerFor(test.agent)
			if !ok {
				t.Fatalf("ReviewerContextMarkerFor(%q) ok = false, want true", test.agent)
			}
			if marker.Header != test.wantHeader || marker.Terminator != test.wantTerminator {
				t.Fatalf("ReviewerContextMarkerFor(%q) = %+v, want Header=%q Terminator=%q",
					test.agent, marker, test.wantHeader, test.wantTerminator)
			}
		})
	}
}

// TestReviewerContextMarkerForRejectsEveryValueOutsideTheClosedTable proves the
// lookup fails closed: a recognized-but-unmapped model.AgentID, case drift, and
// incidental whitespace all refuse rather than silently falling back to the
// generic marker.
func TestReviewerContextMarkerForRejectsEveryValueOutsideTheClosedTable(t *testing.T) {
	for _, agent := range []string{"codex", "kilo", "Claude-Code", " claude-code "} {
		t.Run(agent, func(t *testing.T) {
			marker, ok := ReviewerContextMarkerFor(agent)
			if ok {
				t.Fatalf("ReviewerContextMarkerFor(%q) ok = true, want false (marker=%+v)", agent, marker)
			}
			if marker != (ReviewerContextMarker{}) {
				t.Fatalf("ReviewerContextMarkerFor(%q) returned a non-zero marker on refusal: %+v", agent, marker)
			}
		})
	}
}

// TestReviewerContextMarkersInvariantTerminatorDerivesFromHeader proves every
// table entry's terminator is mechanically derived from its header, so the two
// can never drift apart by hand-editing one and not the other.
func TestReviewerContextMarkersInvariantTerminatorDerivesFromHeader(t *testing.T) {
	for agent, marker := range reviewerContextMarkers {
		if marker.Header == "" {
			t.Fatalf("agent %q has an empty header", agent)
		}
		if marker.Terminator != marker.Header+"_END" {
			t.Fatalf("agent %q terminator = %q, want %q", agent, marker.Terminator, marker.Header+"_END")
		}
	}
}
