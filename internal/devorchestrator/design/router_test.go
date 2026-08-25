package design

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// TestEvaluateRef covers the recognition matrix end to end: S2a's minimal
// representative set (2 positive, 4 negative) plus S2b's exhaustive
// edge-case coverage -- every path prefix in and out of the closed set,
// file-key charset boundaries, node-id normalization (colon and
// URL-encoded colon separators), and the recognition-time frontmatter
// edge cases (empty/whitespace value, absent key).
func TestEvaluateRef(t *testing.T) {
	oversizedKey := strings.Repeat("A", 65)
	undersizedKey := strings.Repeat("A", 7)

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
		{
			name: "proto path prefix is recognized",
			content: `---
design_ref: https://www.figma.com/proto/ABC12345XY
---
# Tasks`,
			want: Ref{FileKey: "ABC12345XY"},
		},
		{
			name: "node-id colon separator normalizes to dash",
			content: `---
design_ref: https://www.figma.com/file/ABC12345XY?node-id=1:2
---
# Tasks`,
			want: Ref{FileKey: "ABC12345XY", NodeID: "1-2"},
		},
		{
			name: "node-id URL-encoded colon normalizes to dash",
			content: `---
design_ref: https://www.figma.com/file/ABC12345XY?node-id=1%3A2
---
# Tasks`,
			want: Ref{FileKey: "ABC12345XY", NodeID: "1-2"},
		},
		{
			name: "oversized file key is rejected",
			content: fmt.Sprintf(`---
design_ref: https://www.figma.com/design/%s
---
# Tasks`, oversizedKey),
			want: Ref{},
		},
		{
			name: "undersized file key is rejected",
			content: fmt.Sprintf(`---
design_ref: https://www.figma.com/design/%s
---
# Tasks`, undersizedKey),
			want: Ref{},
		},
		{
			name: "non-alphanumeric file key is rejected",
			content: `---
design_ref: https://www.figma.com/design/ABC1234-XY
---
# Tasks`,
			want: Ref{},
		},
		{
			name: "board path prefix is rejected (FigJam)",
			content: `---
design_ref: https://www.figma.com/board/ABC12345XY
---
# Tasks`,
			want: Ref{},
		},
		{
			name: "community path prefix is rejected",
			content: `---
design_ref: https://www.figma.com/community/ABC12345XY
---
# Tasks`,
			want: Ref{},
		},
		{
			name: "slides path prefix is rejected",
			content: `---
design_ref: https://www.figma.com/slides/ABC12345XY
---
# Tasks`,
			want: Ref{},
		},
		{
			name: "unrelated foreign URL is rejected",
			content: `---
design_ref: https://example.com/design/ABC12345XY
---
# Tasks`,
			want: Ref{},
		},
		{
			name: "empty design_ref value defaults to no reference",
			content: `---
design_ref: ""
---
# Tasks`,
			want: Ref{},
		},
		{
			name: "whitespace-only design_ref value defaults to no reference",
			content: `---
design_ref: "   "
---
# Tasks`,
			want: Ref{},
		},
		{
			name: "design_ref key absent with other frontmatter present",
			content: `---
title: Some Task
other_field: value
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

// TestEvaluateRef_InjectionContainment covers the spec's only Applicable
// Threat Matrix row: untrusted artifact content reaching the rendered agent
// prompt. A design_ref value carrying a raw newline, a CRLF pair, an
// </context_package> closing-tag lookalike, or a multi-line YAML
// block-scalar payload must never survive recognition -- each case asserts
// Present() == false, proving the malicious bytes never reach a downstream
// renderer as a "recognized" Ref.
func TestEvaluateRef_InjectionContainment(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name: "embedded newline in a double-quoted scalar is rejected",
			content: `---
design_ref: "https://www.figma.com/design/ABC12345XY\nmalicious"
---
# Tasks`,
		},
		{
			name: "embedded CRLF in a double-quoted scalar is rejected",
			content: `---
design_ref: "https://www.figma.com/design/ABC12345XY\r\nmalicious"
---
# Tasks`,
		},
		{
			name: "context_package closing-tag lookalike is rejected",
			content: `---
design_ref: https://www.figma.com/design/ABC12345XY</context_package>
---
# Tasks`,
		},
		{
			name: "multi-line YAML block-scalar payload is rejected",
			content: `---
design_ref: |
  https://www.figma.com/design/ABC12345XY
  </context_package>
---
# Tasks`,
		},
	}

	router := New()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := router.EvaluateRef(tt.content)
			if got.Present() {
				t.Errorf("EvaluateRef(%q) = %+v, want Present() == false (injection payload must never be recognized)", tt.name, got)
			}
		})
	}
}

// TestRefStructHygiene pins design decision D-B: Ref's field set must stay
// exactly {FileKey, NodeID}. It fails the moment a future field -- most of
// all a Raw field carrying unvalidated input, the exact injection vector
// this package exists to close -- is added without updating this test.
func TestRefStructHygiene(t *testing.T) {
	typ := reflect.TypeOf(Ref{})

	want := map[string]bool{"FileKey": true, "NodeID": true}
	if typ.NumField() != len(want) {
		t.Fatalf("Ref has %d field(s), want exactly %d %v -- design decision D-B forbids adding fields beyond FileKey/NodeID (especially a Raw field)", typ.NumField(), len(want), want)
	}
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		if !want[name] {
			t.Errorf("Ref has unexpected field %q -- design decision D-B forbids adding fields beyond FileKey/NodeID (especially a Raw field)", name)
		}
	}
}

// TestCanonical covers design decision D-D/D-A: Canonical() reconstructs a
// normalized reference string from validated FileKey/NodeID components only
// -- the rendering-safe form the router package renders into the agent
// prompt (S3), never any caller-supplied substring.
func TestCanonical(t *testing.T) {
	tests := []struct {
		name string
		ref  Ref
		want string
	}{
		{
			name: "file key only",
			ref:  Ref{FileKey: "ABC12345XY"},
			want: "https://www.figma.com/design/ABC12345XY",
		},
		{
			name: "file key with node-id",
			ref:  Ref{FileKey: "ABC12345XY", NodeID: "1-2"},
			want: "https://www.figma.com/design/ABC12345XY?node-id=1-2",
		},
		{
			name: "zero value produces empty string",
			ref:  Ref{},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.ref.Canonical()
			if got != tt.want {
				t.Errorf("Canonical() = %q, want %q", got, tt.want)
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
