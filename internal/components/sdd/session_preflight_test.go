package sdd

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/catalog"
)

const testSDDSessionPreflightInitAnchor = "### SDD Init Guard (MANDATORY)"

const testExpectedSDDSessionPreflightBlock = "<!-- gentle-ai:sdd-session-preflight -->\n" +
	"### SDD Session Preflight (HARD GATE)\n\n" +
	"Complete this preflight before the SDD init guard, in this order:\n\n" +
	"1. **Pace**: Interactive or Automatic.\n" +
	"2. **Artifacts**: OpenSpec, Engram, or Both (user-facing Both maps only to internal `hybrid`).\n" +
	"3. **PR strategy**: Ask me, Single PR, or Auto.\n" +
	"4. **Review policy**: fixed at 400 changed lines per PR; above 400, split the PR or require maintainer-approved `size:exception`.\n\n" +
	"Preflight MUST be complete before init. Review policy is fixed, not a choice.\n\n" +
	"Canonical mappings:\n" +
	"- Interactive -> `interactive`\n" +
	"- Automatic -> `auto`\n" +
	"- OpenSpec -> `openspec`\n" +
	"- Engram -> `engram`\n" +
	"- Both -> `hybrid`\n" +
	"- Ask me -> `ask-on-risk`\n" +
	"- Single PR -> `single-pr`\n" +
	"- Auto -> `auto-chain`\n" +
	"<!-- /gentle-ai:sdd-session-preflight -->"

func TestSDDSessionPreflightBlockIsCanonical(t *testing.T) {
	block := sddSessionPreflightBlock()
	if block != testExpectedSDDSessionPreflightBlock {
		t.Fatalf("canonical block differs from independent expected content:\n got %q\nwant %q", block, testExpectedSDDSessionPreflightBlock)
	}
	if strings.Count(block, "<!-- gentle-ai:sdd-session-preflight -->") != 1 || strings.Count(block, "<!-- /gentle-ai:sdd-session-preflight -->") != 1 {
		t.Fatalf("canonical block markers are not exactly once: %q", block)
	}
	previous := -1
	for _, decision := range []string{
		"1. **Pace**",
		"2. **Artifacts**",
		"3. **PR strategy**",
		"4. **Review policy**",
	} {
		position := strings.Index(block, decision)
		if position <= previous {
			t.Fatalf("canonical decision %q is out of order at %d (previous %d)", decision, position, previous)
		}
		previous = position
	}
	for _, want := range []string{
		"1. **Pace**",
		"2. **Artifacts**",
		"3. **PR strategy**",
		"4. **Review policy**",
		"Both -> `hybrid`",
		"fixed at 400 changed lines per PR",
		"before the SDD init guard",
	} {
		if !strings.Contains(block, want) {
			t.Fatalf("canonical block missing %q", want)
		}
	}
	if strings.Contains(block, "Both -> `both`") {
		t.Fatal("canonical block retains the legacy Both -> both mapping")
	}
	if err := validateSDDSessionPreflightProjection("prefix\n"+block+"\n"+testSDDSessionPreflightInitAnchor, testSDDSessionPreflightInitAnchor); err != nil {
		t.Fatalf("canonical block failed validation: %v", err)
	}
}

func TestSDDSessionPreflightProjectionCoversOrchestratorAssetClasses(t *testing.T) {
	seen := make(map[string]bool)
	for _, agent := range catalog.AllAgents() {
		path := sddOrchestratorAsset(agent.ID)
		if seen[path] {
			continue
		}
		seen[path] = true
		t.Run(path, func(t *testing.T) {
			existing := "projection class: " + path + "\n" + testSDDSessionPreflightInitAnchor + "\nexisting bytes"
			projected, err := projectSDDSessionPreflight(existing, testSDDSessionPreflightInitAnchor)
			if err != nil {
				t.Fatalf("projectSDDSessionPreflight(%q) error = %v", path, err)
			}
			if err := validateSDDSessionPreflightProjection(projected, testSDDSessionPreflightInitAnchor); err != nil {
				t.Fatalf("projected %q failed validation: %v", path, err)
			}
		})
	}
	if len(seen) != 12 {
		t.Fatalf("projection matrix covered %d asset classes, want 12", len(seen))
	}
}

func TestSDDSessionPreflightProjectionPreservesSurroundingBytes(t *testing.T) {
	block := testExpectedSDDSessionPreflightBlock
	blockCRLF := strings.ReplaceAll(block, "\n", "\r\n")
	staleBlockCRLF := strings.Replace(blockCRLF, "400 changed lines", "401 changed lines", 1)
	tests := []struct {
		name   string
		input  string
		want   string
		prefix string
	}{
		{
			name:  "utf8 insertion",
			input: "前置🙂\n" + testSDDSessionPreflightInitAnchor + "\n后置🦊",
			want:  "前置🙂\n" + block + "\n" + testSDDSessionPreflightInitAnchor + "\n后置🦊",
		},
		{
			name:  "crlf insertion",
			input: "前置🙂\r\n" + testSDDSessionPreflightInitAnchor + "\r\n后置🦊",
			want:  "前置🙂\r\n" + blockCRLF + "\r\n" + testSDDSessionPreflightInitAnchor + "\r\n后置🦊",
		},
		{
			name:  "replace stale owned range",
			input: "保留前\r\n" + staleBlockCRLF + "\r\n" + testSDDSessionPreflightInitAnchor + "\r\n保留後",
			want:  "保留前\r\n" + blockCRLF + "\r\n" + testSDDSessionPreflightInitAnchor + "\r\n保留後",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := projectSDDSessionPreflight(tt.input, testSDDSessionPreflightInitAnchor)
			if err != nil {
				t.Fatalf("projectSDDSessionPreflight() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("projectSDDSessionPreflight() changed unexpected bytes:\n got %q\nwant %q", got, tt.want)
			}
			if err := validateSDDSessionPreflightProjection(got, testSDDSessionPreflightInitAnchor); err != nil {
				t.Fatalf("projected output failed validation: %v", err)
			}
		})
	}
}

func TestValidateSDDSessionPreflightProjectionRejectsMalformed(t *testing.T) {
	block := testExpectedSDDSessionPreflightBlock
	valid := "prefix\n" + block + "\n" + testSDDSessionPreflightInitAnchor + "\npostfix"
	legacyBoth := strings.Replace(block, "Both -> `hybrid`", "Both -> `both`", 1)
	incomplete := strings.Replace(block, "3. **PR strategy**: Ask me, Single PR, or Auto.\n", "", 1)
	nonExact := strings.Replace(block, "400 changed lines", "401 changed lines", 1)
	outOfOrder := strings.Replace(
		block,
		"1. **Pace**: Interactive or Automatic.\n2. **Artifacts**: OpenSpec, Engram, or Both (user-facing Both maps only to internal `hybrid`).",
		"2. **Artifacts**: OpenSpec, Engram, or Both (user-facing Both maps only to internal `hybrid`).\n1. **Pace**: Interactive or Automatic.",
		1,
	)
	bodyWithReversedMarkers := strings.TrimPrefix(block, "<!-- gentle-ai:sdd-session-preflight -->\n")
	bodyWithReversedMarkers = strings.TrimSuffix(bodyWithReversedMarkers, "\n<!-- /gentle-ai:sdd-session-preflight -->")
	reversedMarkers := "prefix\n<!-- /gentle-ai:sdd-session-preflight -->\n" + bodyWithReversedMarkers + "\n<!-- gentle-ai:sdd-session-preflight -->\n" + testSDDSessionPreflightInitAnchor
	tests := []struct {
		name string
		text string
	}{
		{name: "missing block", text: "prefix\n" + testSDDSessionPreflightInitAnchor},
		{name: "duplicate", text: "prefix\n" + block + "\n" + block + "\n" + testSDDSessionPreflightInitAnchor},
		{name: "orphan opener", text: "prefix\n<!-- gentle-ai:sdd-session-preflight -->\n" + testSDDSessionPreflightInitAnchor},
		{name: "orphan closer", text: "prefix\n<!-- /gentle-ai:sdd-session-preflight -->\n" + testSDDSessionPreflightInitAnchor},
		{name: "incomplete decisions", text: "prefix\n" + incomplete + "\n" + testSDDSessionPreflightInitAnchor},
		{name: "post-init placement", text: "prefix\n" + testSDDSessionPreflightInitAnchor + "\n" + block},
		{name: "Both maps to both", text: "prefix\n" + legacyBoth + "\n" + testSDDSessionPreflightInitAnchor},
		{name: "non-exact body", text: "prefix\n" + nonExact + "\n" + testSDDSessionPreflightInitAnchor},
		{name: "out-of-order canonical decisions", text: "prefix\n" + outOfOrder + "\n" + testSDDSessionPreflightInitAnchor},
		{name: "reversed marker order", text: reversedMarkers},
		{name: "missing init anchor", text: valid[:strings.Index(valid, testSDDSessionPreflightInitAnchor)]},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateSDDSessionPreflightProjection(tt.text, testSDDSessionPreflightInitAnchor); err == nil {
				t.Fatal("validateSDDSessionPreflightProjection() error = nil, want rejection")
			}
		})
	}
}

func TestProjectSDDSessionPreflightRejectsAmbiguousAnchorOrMarkers(t *testing.T) {
	block := sddSessionPreflightBlock()
	tests := []struct {
		name  string
		input string
	}{
		{name: "missing anchor", input: "prefix\nno init anchor"},
		{name: "duplicate anchor", input: "prefix\n" + testSDDSessionPreflightInitAnchor + "\n" + testSDDSessionPreflightInitAnchor},
		{name: "orphan marker", input: "prefix\n<!-- gentle-ai:sdd-session-preflight -->\n" + testSDDSessionPreflightInitAnchor},
		{name: "post-init owned range", input: "prefix\n" + testSDDSessionPreflightInitAnchor + "\n" + block},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := projectSDDSessionPreflight(tt.input, testSDDSessionPreflightInitAnchor); err == nil {
				t.Fatal("projectSDDSessionPreflight() error = nil, want rejection")
			}
		})
	}
}
