package sdd

import (
	"fmt"
	"strings"
)

const (
	sddSessionPreflightMarker = "<!-- gentle-ai:sdd-session-preflight -->"
	sddSessionPreflightEnd    = "<!-- /gentle-ai:sdd-session-preflight -->"
	sddSessionPreflightInit   = "### SDD Init Guard (MANDATORY)"
	sddSessionPreflightBody   = "### SDD Session Preflight (HARD GATE)\n\nBefore every SDD command or natural-language SDD request, run this preflight before the SDD init guard; cache choices for the session.\n\nUse the `question` tool only when available and all three groups (Pace, Artifacts, and PR strategy) are exactly representable; otherwise use the lossless blocking fallback and STOP.\nAsk Pace, Artifacts, and PR strategy in ONE `question` tool call; no sequential wizard and no three separate calls.\nMatch labels and descriptions to the conversation language and persona; do not expose canonical/internal codes.\n\n1. **Pace**: Interactive or Automatic.\n2. **Artifacts**: OpenSpec, Engram, or Both (user-facing Both maps only to internal `hybrid`).\n3. **PR strategy**: Ask me, Single PR, or Auto.\n\nReview policy is fixed at 400 changed lines per PR; above 400, split the PR or require maintainer-approved `size:exception`; NEVER ask it as a fourth group or selectable budget.\n\nCanonical mappings:\n- Interactive -> `interactive`\n- Automatic -> `auto`\n- OpenSpec -> `openspec`\n- Engram -> `engram`\n- Both -> `hybrid`\n- Ask me -> `ask-on-risk`\n- Single PR -> `single-pr`\n- Auto -> `auto-chain`"
)

func sddSessionPreflightBlock() string {
	return sddSessionPreflightMarker + "\n" + sddSessionPreflightBody + "\n" + sddSessionPreflightEnd
}
func projectSDDSessionPreflight(rendered, preInitAnchor string) (string, error) {
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
	newline := "\n"
	if strings.Contains(rendered, "\r\n") {
		newline = "\r\n"
	}
	block := strings.ReplaceAll(sddSessionPreflightBlock(), "\n", newline)
	if open >= 0 {
		rendered = rendered[:open] + block + rendered[closeEnd:]
	} else {
		rendered = rendered[:anchor] + block + newline + rendered[anchor:]
	}
	if err := validateSDDSessionPreflightProjection(rendered, preInitAnchor); err != nil {
		return "", fmt.Errorf("validate projected SDD session preflight: %w", err)
	}
	return rendered, nil
}
func validateSDDSessionPreflightProjection(rendered, preInitAnchor string) error {
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
	if actual != sddSessionPreflightBlock() {
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
	openCount := strings.Count(rendered, sddSessionPreflightMarker)
	closeCount := strings.Count(rendered, sddSessionPreflightEnd)
	if openCount == 0 && closeCount == 0 {
		return -1, -1, nil
	}
	if openCount != 1 || closeCount != 1 {
		return 0, 0, fmt.Errorf("sdd session preflight markers must contain exactly one pair")
	}
	open := strings.Index(rendered, sddSessionPreflightMarker)
	close := strings.Index(rendered, sddSessionPreflightEnd)
	if close <= open {
		return 0, 0, fmt.Errorf("sdd session preflight markers are orphaned or reversed")
	}
	closeEnd := close + len(sddSessionPreflightEnd)
	if !sddSessionPreflightLineStart(rendered, open) || !sddSessionPreflightLineEnd(rendered, closeEnd) {
		return 0, 0, fmt.Errorf("sdd session preflight markers must occupy complete lines")
	}
	return open, closeEnd, nil
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
