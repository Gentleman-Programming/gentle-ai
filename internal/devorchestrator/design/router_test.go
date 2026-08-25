package design

import "testing"

// TestEvaluateRef is S2a's minimal representative test set: 2 positive
// cases and 4 negative cases. The exhaustive edge-case, injection, and
// struct-hygiene matrix lands in S2b (test-only slice, zero ratchet
// exposure) immediately after this slice -- see tasks S2b.
func TestEvaluateRef(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    Ref
	}{
		{
			name: "recognized design URL without node-id",
			content: `---
design_ref: https://www.figma.com/design/ABC12345XY
---
# Tasks`,
			want: Ref{FileKey: "ABC12345XY"},
		},
		{
			name: "recognized file URL with node-id",
			content: `---
design_ref: https://www.figma.com/file/ABC12345XY?node-id=1-2
---
# Tasks`,
			want: Ref{FileKey: "ABC12345XY", NodeID: "1-2"},
		},
		{
			name: "http scheme is rejected",
			content: `---
design_ref: http://www.figma.com/design/ABC12345XY
---
# Tasks`,
			want: Ref{},
		},
		{
			name: "lookalike host is rejected (never substring-matched)",
			content: `---
design_ref: https://figma.com.evil.com/design/ABC12345XY
---
# Tasks`,
			want: Ref{},
		},
		{
			name: "no frontmatter block defaults to no reference",
			content: `# Tasks
Just doing some UI work`,
			want: Ref{},
		},
		{
			name: "malformed YAML defaults to no reference",
			content: `---
design_ref: [unterminated
---
# Tasks`,
			want: Ref{},
		},
	}

	router := New()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := router.EvaluateRef(tt.content)
			if got != tt.want {
				t.Errorf("EvaluateRef(%q) = %+v, want %+v", tt.name, got, tt.want)
			}
		})
	}
}

// TestEvaluateRef_NilReceiverDoesNotPanic proves design Risk 3's mitigation:
// EvaluateRef must never dereference its receiver, so a literal-constructed
// Orchestrator{} (leaving DesignRouter nil, bypassing New()) stays safe when
// GenerateContextForAgent calls through it. Because EvaluateRef never reads
// receiver state (Router carries none), calling through a nil receiver must
// still parse exactly like a normally constructed Router -- asserting the
// real resolved value (not merely "no panic") proves genuine execution
// rather than a dead code path.
func TestEvaluateRef_NilReceiverDoesNotPanic(t *testing.T) {
	var router *Router

	got := router.EvaluateRef(`---
design_ref: https://www.figma.com/design/ABC12345XY
---
# Tasks`)

	want := Ref{FileKey: "ABC12345XY"}
	if got != want {
		t.Errorf("EvaluateRef on nil receiver = %+v, want %+v (nil receiver must parse identically to a constructed Router)", got, want)
	}
}
