package sdd

import (
	"fmt"
	"strings"
)

const (
	sddSessionPreflightMarker = "<!-- gentle-ai:sdd-session-preflight -->"
	sddSessionPreflightEnd    = "<!-- /gentle-ai:sdd-session-preflight -->"
	sddSessionPreflightInit   = "### SDD Init Guard (MANDATORY)"
	sddSessionPreflightBody   = "### SDD Session Preflight (HARD GATE)\n\nComplete this preflight before the SDD init guard, in this order:\n\n1. **Pace**: Interactive or Automatic.\n2. **Artifacts**: OpenSpec, Engram, or Both (user-facing Both maps only to internal `hybrid`).\n3. **PR strategy**: Ask me, Single PR, or Auto.\n4. **Review policy**: fixed at 400 changed lines per PR; above 400, split the PR or require maintainer-approved `size:exception`.\n\nPreflight MUST be complete before init. Review policy is fixed, not a choice.\n\nCanonical mappings:\n- Interactive -> `interactive`\n- Automatic -> `auto`\n- OpenSpec -> `openspec`\n- Engram -> `engram`\n- Both -> `hybrid`\n- Ask me -> `ask-on-risk`\n- Single PR -> `single-pr`\n- Auto -> `auto-chain`"
)

// sddSessionPreflightBlock is the single package-owned canonical projection.
// Its LF form is stable; projections adopt the caller's CRLF style when needed.
func sddSessionPreflightBlock() string {
	return sddSessionPreflightMarker + "\n" + sddSessionPreflightBody + "\n" + sddSessionPreflightEnd
}

// projectSDDSessionPreflight inserts or replaces only the owned marker range
// immediately before the caller's existing pre-init anchor.
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

	block := sddSessionPreflightBlockForLineEnding(rendered)
	if open >= 0 {
		rendered = rendered[:open] + block + rendered[closeEnd:]
	} else {
		rendered = insertSDDSessionPreflight(rendered, anchor, block, sddSessionPreflightLineEnding(rendered))
	}

	if err := validateSDDSessionPreflightProjection(rendered, preInitAnchor); err != nil {
		return "", fmt.Errorf("validate projected SDD session preflight: %w", err)
	}
	return rendered, nil
}

// validateSDDSessionPreflightProjection rejects any projection that is not the
// exact canonical block, uniquely placed before init and before preInitAnchor.
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
	return strings.Index(rendered, anchor), nil
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

func insertSDDSessionPreflight(rendered string, anchor int, block, newline string) string {
	before, after := rendered[:anchor], rendered[anchor:]
	var result strings.Builder
	result.Grow(len(rendered) + len(block) + 2*len(newline))
	result.WriteString(before)
	if before != "" && !sddSessionPreflightEndsLine(before) {
		result.WriteString(newline)
	}
	result.WriteString(block)
	if after != "" && !sddSessionPreflightStartsLine(after) {
		result.WriteString(newline)
	}
	result.WriteString(after)
	return result.String()
}

func sddSessionPreflightBlockForLineEnding(rendered string) string {
	block := sddSessionPreflightBlock()
	if newline := sddSessionPreflightLineEnding(rendered); newline != "\n" {
		return strings.ReplaceAll(block, "\n", newline)
	}
	return block
}

func sddSessionPreflightLineEnding(rendered string) string {
	if strings.Contains(rendered, "\r\n") {
		return "\r\n"
	}
	return "\n"
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

func sddSessionPreflightEndsLine(value string) bool {
	return strings.HasSuffix(value, "\n") || strings.HasSuffix(value, "\r")
}

func sddSessionPreflightStartsLine(value string) bool {
	return strings.HasPrefix(value, "\n") || strings.HasPrefix(value, "\r\n")
}
