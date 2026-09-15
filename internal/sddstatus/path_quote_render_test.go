package sddstatus

import (
	"strings"
	"testing"
)

func TestNonPhaseRoutingInstructionsRenderWindowsPathVerbatim(t *testing.T) {
	want := `--cwd "C:\Users\dev\repo"`
	tests := []struct {
		next        string
		occurrences int
	}{
		{next: "select-change", occurrences: 2},
	}
	for _, tt := range tests {
		t.Run(tt.next, func(t *testing.T) {
			status := Status{NextRecommended: tt.next}
			status.ActionContext.WorkspaceRoot = `C:\Users\dev\repo`
			instructions, ok := nonPhaseRoutingInstructions(status)
			if !ok {
				t.Fatalf("nonPhaseRoutingInstructions(%q) returned no instructions", tt.next)
			}
			got := strings.Join(instructions, "\n")
			if occurrences := strings.Count(got, want); occurrences != tt.occurrences {
				t.Fatalf("routing instructions must carry the verbatim path:\nwant %d occurrences of %s, got %d\ngot: %s", tt.occurrences, want, occurrences, got)
			}
		})
	}
}

// The stranded-successor refusal is gone with the gate it served. The Windows
// path-quoting rule it guarded stays covered by the reset and
// objective-change refusals above.
