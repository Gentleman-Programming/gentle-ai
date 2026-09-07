package sdd

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

const (
	sddSessionPreflightMarker       = "<!-- gentle-ai:sdd-session-preflight -->"
	sddSessionPreflightEnd          = "<!-- /gentle-ai:sdd-session-preflight -->"
	sddSessionPreflightInit         = "### SDD Init Guard (MANDATORY)"
	legacySDDSessionPreflightMarker = "<!-- gentle-ai:sdd-session-preflight-migration -->"
	legacySDDSessionPreflightEnd    = "<!-- /gentle-ai:sdd-session-preflight-migration -->"
	sddSessionPreflightBody         = "### SDD Session Preflight (HARD GATE)\n\nBefore every SDD command or natural-language SDD request, run this preflight before the SDD init guard; cache choices for the session.\n\nUse the `question` tool only when available and all three groups (Pace, Artifacts, and PR strategy) are exactly representable; otherwise use the lossless blocking fallback and STOP.\nAsk Pace, Artifacts, and PR strategy in ONE `question` tool call; no sequential wizard and no three separate calls.\nMatch labels and descriptions to the conversation language and persona; do not expose canonical/internal codes.\n\n1. **Pace**: Interactive or Automatic.\n2. **Artifacts**: OpenSpec, Engram, or Both (user-facing Both maps only to internal `hybrid`).\n3. **PR strategy**: Ask me, Single PR, or Auto.\n\nReview policy is fixed at 400 changed lines per PR; above 400, split the PR or require maintainer-approved `size:exception`; NEVER ask it as a fourth group or selectable budget.\n\nCanonical mappings:\n- Interactive -> `interactive`\n- Automatic -> `auto`\n- OpenSpec -> `openspec`\n- Engram -> `engram`\n- Both -> `hybrid`\n- Ask me -> `ask-on-risk`\n- Single PR -> `single-pr`\n- Auto -> `auto-chain`"
)

// The native workflow consumes installed authority, never a second policy body.
const windsurfSessionPreflightReference = "Read `~/.codeium/windsurf/memories/global_rules.md` before any SDD-owned mutation. Follow its SDD Session Preflight, then its native dispatcher and init guards for `/sdd-new`. If that installed authority is missing, malformed, or cannot resolve the session, STOP; do not initialize, infer defaults, or recreate policy in this workflow."
const windsurfSessionPreflightPlaceholder = "{{GENTLE_AI_SDD_SESSION_PREFLIGHT_AUTHORITY}}"

func renderWindsurfSessionPreflightEntry(content string) (string, error) {
	if _, err := sddSessionPreflightNewline(content); err != nil {
		return "", err
	}
	if strings.Count(content, windsurfSessionPreflightPlaceholder) != 1 {
		return "", fmt.Errorf("Windsurf sdd-new requires exactly one installed-authority reference")
	}
	normalized, err := normalizeSDDSessionPreflightLineEndings(content)
	if err != nil {
		return "", err
	}
	const shell = "---\ndescription: Start a change through the installed SDD authority in Windsurf\n---\n\n# /sdd-new\n\n"
	if normalized != shell+windsurfSessionPreflightPlaceholder+"\n" {
		return "", fmt.Errorf("Windsurf sdd-new must remain a thin authority consumer")
	}
	return strings.Replace(content, windsurfSessionPreflightPlaceholder, windsurfSessionPreflightReference, 1), nil
}

func validateRenderedSessionPreflight(content string, agent model.AgentID) error {
	switch {
	case usesFallbackSessionPreflight(agent):
		return validateSDDSessionPreflightProjection(content, "### Native SDD Dispatcher Guard", "")
	case agent == model.AgentClaudeCode:
		return validateSDDSessionPreflightProjection(content, "### SDD Entry Routing (MANDATORY)", "AskUserQuestion")
	case agent == model.AgentOpenCode || agent == model.AgentKilocode:
		return validateSDDSessionPreflightProjection(content, "### SDD Entry Routing (MANDATORY)")
	}
	return nil
}

func sddSessionPreflightBlock() string {
	return sddSessionPreflightBlockWithTool("question")
}

// Only interaction transport varies; all runtimes share one policy body.
func sddSessionPreflightBlockWithTool(tool string) string {
	body := strings.ReplaceAll(sddSessionPreflightBody, "`question`", "`"+tool+"`")
	if tool == "" {
		start := strings.Index(sddSessionPreflightBody, "Use the `question` tool")
		end := strings.Index(sddSessionPreflightBody, "Match labels")
		body = sddSessionPreflightBody[:start] + "This runtime has no classified native question UI. Present ONE complete blocking prompt in plain chat or terminal with all three groups below, in order, and explain that their answers are required before init. Each group is single-select; its listed labels are the complete allowed-answer domain. Include every label and description, the fixed review policy, and this answer syntax: `Pace: <label>; Artifacts: <label>; PR strategy: <label>`. Then STOP; never default, infer, or launch dependent work. Validate all three answers under the Lossless Blocking Prompts contract before caching their canonical mappings.\n\n" + sddSessionPreflightBody[end:]
	}
	return sddSessionPreflightMarker + "\n" + body + "\n" + sddSessionPreflightEnd
}

// migratePreservedSDDSessionPreflight is the only migration path for a
// preserved external OpenCode or Kilo prompt. It replaces owned legacy ranges
// in place and otherwise appends the canonical block without interpreting any
// unmarked user-authored prompt text.
func validatePreservedSDDSessionPreflight(rendered string) error {
	canonicalStart, _, err := sddSessionPreflightMarkerRange(rendered)
	if err != nil {
		return err
	}
	legacyStart, _, err := sddSessionPreflightOwnedMarkerRange(rendered, legacySDDSessionPreflightMarker, legacySDDSessionPreflightEnd)
	if err != nil {
		return err
	}
	if canonicalStart >= 0 && legacyStart >= 0 {
		return fmt.Errorf("sdd session preflight cannot contain canonical and legacy marker pairs")
	}
	return nil
}

func migratePreservedSDDSessionPreflight(rendered string) (string, error) {
	if err := validatePreservedSDDSessionPreflight(rendered); err != nil {
		return "", err
	}
	newline, err := sddSessionPreflightNewline(rendered)
	if err != nil {
		return "", err
	}
	legacyStart, legacyEnd, err := sddSessionPreflightOwnedMarkerRange(rendered, legacySDDSessionPreflightMarker, legacySDDSessionPreflightEnd)
	if err != nil {
		return "", err
	}
	block := strings.ReplaceAll(sddSessionPreflightBlock(), "\n", newline)
	if legacyStart >= 0 {
		rendered = rendered[:legacyStart] + block + rendered[legacyEnd:]
	}
	open, closeEnd, err := sddSessionPreflightMarkerRange(rendered)
	if err != nil {
		return "", err
	}
	if open >= 0 {
		return rendered[:open] + block + rendered[closeEnd:], nil
	}
	if rendered == "" {
		return block, nil
	}
	if strings.HasSuffix(rendered, newline) {
		return rendered + newline + block, nil
	}
	return rendered + newline + newline + block, nil
}

func projectSDDSessionPreflight(rendered, preInitAnchor string) (string, error) {
	return projectSDDSessionPreflightWithTool(rendered, preInitAnchor, "question")
}

func projectSDDSessionPreflightWithTool(rendered, preInitAnchor, tool string) (string, error) {
	anchor, err := sddSessionPreflightAnchorIndex(rendered, preInitAnchor)
	if err != nil {
		return "", err
	}
	open, closeEnd, err := sddSessionPreflightMarkerRange(rendered)
	if err != nil {
		return "", err
	}
	if open >= 0 && (open >= anchor || closeEnd > anchor) {
		return "", fmt.Errorf("sdd session preflight is not at the supplied pre-init anchor")
	}
	newline, err := sddSessionPreflightNewline(rendered)
	if err != nil {
		return "", err
	}
	block := strings.ReplaceAll(sddSessionPreflightBlockWithTool(tool), "\n", newline)
	if open >= 0 {
		rendered = rendered[:open] + block + rendered[closeEnd:]
	} else {
		rendered = rendered[:anchor] + block + newline + rendered[anchor:]
	}
	if err := validateSDDSessionPreflightProjection(rendered, preInitAnchor, tool); err != nil {
		return "", fmt.Errorf("validate projected SDD session preflight: %w", err)
	}
	return rendered, nil
}
func validateSDDSessionPreflightProjection(rendered, preInitAnchor string, nativeTool ...string) error {
	tool := "question"
	if len(nativeTool) > 0 {
		tool = nativeTool[0]
	}
	if _, err := sddSessionPreflightNewline(rendered); err != nil {
		return err
	}
	anchor, err := sddSessionPreflightAnchorIndex(rendered, preInitAnchor)
	if err != nil {
		return err
	}
	init := strings.Index(rendered, sddSessionPreflightInit)
	if strings.Count(rendered, sddSessionPreflightInit) != 1 {
		return fmt.Errorf("sdd session preflight requires exactly one init anchor")
	}
	if anchor > init {
		return fmt.Errorf("sdd session preflight anchor follows init")
	}
	open, closeEnd, err := sddSessionPreflightMarkerRange(rendered)
	if err != nil {
		return err
	}
	if open < 0 {
		return fmt.Errorf("sdd session preflight block is missing")
	}
	if open >= anchor || closeEnd > anchor || open >= init || closeEnd > init {
		return fmt.Errorf("sdd session preflight must precede init at the supplied anchor")
	}
	actual, err := normalizeSDDSessionPreflightLineEndings(rendered[open:closeEnd])
	if err != nil {
		return err
	}
	if strings.Contains(actual, "Both -> `both`") {
		return fmt.Errorf("legacy SDD session preflight mapping Both -> both is rejected")
	}
	if actual != sddSessionPreflightBlockWithTool(tool) {
		return fmt.Errorf("sdd session preflight block is not exact canonical content")
	}
	return nil
}
func sddSessionPreflightAnchorIndex(rendered, anchor string) (int, error) {
	if anchor == "" {
		return 0, fmt.Errorf("sdd session preflight pre-init anchor is empty")
	}
	if strings.Count(rendered, anchor) != 1 {
		return 0, fmt.Errorf("sdd session preflight pre-init anchor must occur exactly once")
	}
	index := strings.Index(rendered, anchor)
	if !sddSessionPreflightLineStart(rendered, index) || !sddSessionPreflightLineEnd(rendered, index+len(anchor)) {
		return 0, fmt.Errorf("sdd session preflight pre-init anchor must occupy a complete line")
	}
	return index, nil
}

func sddSessionPreflightMarkerRange(rendered string) (int, int, error) {
	return sddSessionPreflightOwnedMarkerRange(rendered, sddSessionPreflightMarker, sddSessionPreflightEnd)
}

func sddSessionPreflightOwnedMarkerRange(rendered, openMarker, closeMarker string) (int, int, error) {
	openCount := strings.Count(rendered, openMarker)
	closeCount := strings.Count(rendered, closeMarker)
	if openCount == 0 && closeCount == 0 {
		return -1, -1, nil
	}
	if openCount != 1 || closeCount != 1 {
		return 0, 0, fmt.Errorf("sdd session preflight markers must contain exactly one pair")
	}
	open := strings.Index(rendered, openMarker)
	close := strings.Index(rendered, closeMarker)
	if close <= open {
		return 0, 0, fmt.Errorf("sdd session preflight markers are orphaned or reversed")
	}
	closeEnd := close + len(closeMarker)
	if !sddSessionPreflightLineStart(rendered, open) || !sddSessionPreflightLineEnd(rendered, closeEnd) {
		return 0, 0, fmt.Errorf("sdd session preflight markers must occupy complete lines")
	}
	return open, closeEnd, nil
}

func sddSessionPreflightNewline(value string) (string, error) {
	withoutCRLF := strings.ReplaceAll(value, "\r\n", "")
	if strings.Contains(withoutCRLF, "\r") || (strings.Contains(value, "\r\n") && strings.Contains(withoutCRLF, "\n")) {
		return "", fmt.Errorf("sdd session preflight contains mixed or unsupported line endings")
	}
	if strings.Contains(value, "\r\n") {
		return "\r\n", nil
	}
	return "\n", nil
}

func normalizeSDDSessionPreflightPromptLineEndings(value, newline string) string {
	if newline == "\n" {
		return value
	}
	return strings.ReplaceAll(strings.ReplaceAll(value, "\r\n", "\n"), "\n", newline)
}

func normalizeSDDSessionPreflightLineEndings(value string) (string, error) {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	if strings.Contains(value, "\r") {
		return "", fmt.Errorf("sdd session preflight contains unsupported line endings")
	}
	return value, nil
}

func sddSessionPreflightLineStart(value string, index int) bool {
	return index == 0 || value[index-1] == '\n'
}

func sddSessionPreflightLineEnd(value string, index int) bool {
	return index == len(value) || value[index] == '\n' || strings.HasPrefix(value[index:], "\r\n")
}
