package sdd

import (
	"strings"
	"testing"
)

const testSDDSessionPreflightInitAnchor = "### SDD Init Guard (MANDATORY)"

const testExpectedSDDSessionPreflightBlock = "<!-- gentle-ai:sdd-session-preflight -->\n" +
	"### SDD Session Preflight (HARD GATE)\n\n" +
	"Before every SDD command or natural-language SDD request, run this preflight before the SDD init guard; cache choices for the session.\n\n" +
	"Use the `question` tool only when available and all three groups (Pace, Artifacts, and PR strategy) are exactly representable; otherwise use the lossless blocking fallback and STOP.\n" +
	"Ask Pace, Artifacts, and PR strategy in ONE `question` tool call; no sequential wizard and no three separate calls.\n" +
	"Match labels and descriptions to the conversation language and persona; do not expose canonical/internal codes.\n\n" +
	"1. **Pace**: Interactive or Automatic.\n" +
	"2. **Artifacts**: OpenSpec, Engram, or Both (user-facing Both maps only to internal `hybrid`).\n" +
	"3. **PR strategy**: Ask me, Single PR, or Auto.\n\n" +
	"Review policy is fixed at 400 changed lines per PR; above 400, split the PR or require maintainer-approved `size:exception`; NEVER ask it as a fourth group or selectable budget.\n\n" +
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
	for _, retired := range []string{"4. **", "Both -> `both`"} {
		if strings.Contains(block, retired) {
			t.Fatalf("canonical block retains retired content %q", retired)
		}
	}

}
