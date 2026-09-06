package sdd

import (
	"fmt"
	"strings"
)

// Marker vocabulary and three-choice semantics adapt decode2's PR #3676
// (60ee7efa); #3499 keeps this fresh-prompt authority separate from #3500.
const (
	sddSessionPreflightSection = "SDD Session Preflight (HARD GATE)"
	sddSessionPreflightOpen    = "<!-- gentle-ai:sdd-session-preflight -->"
	sddSessionPreflightClose   = "<!-- /gentle-ai:sdd-session-preflight -->"
	sddSessionPreflightEntry   = "### SDD Entry Routing (MANDATORY)"
	sddSessionPreflightInit    = "### SDD Init Guard (MANDATORY)"
	sddSessionPreflightBody    = `Before every SDD command or natural-language SDD request, run this preflight before the SDD init guard and cache the choices for the session.

Use the ` + "`question`" + ` tool only when it is available and all three groups (Pace, Artifacts, and PR strategy) are exactly representable. While that native route is usable, do not render a duplicate plain-chat menu. If the tool is unavailable, denied, noninteractive, or cannot represent the complete prompt, follow the Lossless Blocking Prompts fallback above and STOP.

Ask Pace, Artifacts, and PR strategy in ONE ` + "`question`" + ` tool call. Use no sequential wizard and no three separate calls. Match labels and descriptions to the conversation language and persona; do not expose canonical/internal codes.

1. **Pace**: Interactive or Automatic.
2. **Artifacts**: OpenSpec, Engram, or Both. Offer Engram and Both only when Engram is callable; user-facing Both maps only to internal ` + "`hybrid`" + `.
3. **PR strategy**: Ask me, Single PR, or Auto.

Review policy is fixed at 400 changed lines per PR. Above 400, split the PR or require maintainer-approved ` + "`size:exception`" + `; NEVER ask it as a fourth group or selectable budget.

Canonical mappings:
- Interactive -> ` + "`interactive`" + `
- Automatic -> ` + "`auto`" + `
- OpenSpec -> ` + "`openspec`" + `
- Engram -> ` + "`engram`" + `
- Both -> ` + "`hybrid`" + `
- Ask me -> ` + "`ask-on-risk`" + `
- Single PR -> ` + "`single-pr`" + `
- Auto -> ` + "`auto-chain`" + `

The PR values are exactly the ` + "`delivery_strategy`" + ` domain accepted by planning and implementation phases. The preflight cannot select ` + "`exception-ok`" + `; that value is reachable only when the maintainer explicitly accepts ` + "`size:exception`" + `.

Hard gate rules:
- ` + "`openspec/config.yaml`" + `, existing SDD artifacts, previous initialization results, or installed SDD assets do NOT satisfy session preflight.
- If the session has no preflight block, ask the single grouped preflight above. Do not run init, delegate phases, edit files, or apply tasks until all three choices are collected.
- Cache the choices for this session and include them in later phase prompts.
- If the user explicitly provided all three choices in the current conversation, summarize them as the ` + "`SDD Session Preflight`" + ` decision block and continue.`
)

func sddSessionPreflightBlock() string {
	return sddSessionPreflightOpen + "\n" + sddSessionPreflightBody + "\n" + sddSessionPreflightClose
}

func validateSDDSessionPreflight(rendered string) error {
	normalized := strings.ReplaceAll(rendered, "\r\n", "\n")
	if strings.Contains(normalized, "\r") {
		return fmt.Errorf("sdd: session preflight contains unsupported line endings")
	}
	block := sddSessionPreflightBlock()
	if strings.Count(normalized, block) != 1 ||
		strings.Count(normalized, sddSessionPreflightOpen) != 1 ||
		strings.Count(normalized, sddSessionPreflightClose) != 1 {
		return fmt.Errorf("sdd: session preflight must contain one exact canonical marker-bounded block")
	}
	entry := strings.Index(normalized, sddSessionPreflightEntry)
	init := strings.Index(normalized, sddSessionPreflightInit)
	if strings.Count(normalized, sddSessionPreflightEntry) != 1 || strings.Count(normalized, sddSessionPreflightInit) != 1 ||
		!strings.Contains(normalized, "### "+sddSessionPreflightSection+"\n\n"+block+"\n\n"+sddSessionPreflightEntry) || entry >= init {
		return fmt.Errorf("sdd: session preflight must occupy the canonical placement before init")
	}
	for _, legacy := range []string{"Both -> `both`", "Review: 400 lines, 800 lines, Other", "review_budget_lines: 800", "If Other is selected for review budget", "4. **Review"} {
		if strings.Contains(normalized, legacy) {
			return fmt.Errorf("sdd: session preflight retains legacy policy %q", legacy)
		}
	}
	return nil
}
