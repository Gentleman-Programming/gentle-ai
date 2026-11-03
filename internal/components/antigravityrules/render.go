// Package antigravityrules publishes the Gentle AI guidance staged for
// Antigravity (agy 1.4+) as always-on global rule files.
//
// Every Gentle AI prompt writer appends its marker section to the adapter's
// SystemPromptFile. agy truncates each rule file at MaxRuleFileBytes, so every
// staged section is rendered, in order, into
// ~/.gemini/config/rules/gentle-ai-NN-<section-id>[-partK].md, and oversized
// sections are split at heading (fallback: line) boundaries (#5676, #1989).
package antigravityrules

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// MaxRuleFileBytes is agy's per-rule-file cap; content past it is truncated.
const MaxRuleFileBytes = 24000

// managedHeader makes a global rule load in every agy session; Publish only
// removes owned-name files that start with it.
const (
	managedHeader = "---\ntrigger: always_on\n---\n"
	frontmatter   = managedHeader + "\n"
)

var openMarkerPattern = regexp.MustCompile(`<!-- gentle-ai:([A-Za-z0-9_.-]+) -->`)

// Rule is one rendered agy rule file.
type Rule struct {
	Name    string
	Content string
}

// Render turns every complete gentle-ai marker section of the staging content
// into rule files no larger than MaxRuleFileBytes. Concatenating the bodies of
// a section's parts, in order, yields the section body unchanged.
func Render(staging string) []Rule {
	var rules []Rule
	index := 0
	for search := 0; ; {
		loc := openMarkerPattern.FindStringSubmatchIndex(staging[search:])
		if loc == nil {
			break
		}
		id := staging[search+loc[2] : search+loc[3]]
		bodyStart := search + loc[1]
		closeMarker := "<!-- /gentle-ai:" + id + " -->"
		closeOffset := strings.Index(staging[bodyStart:], closeMarker)
		if closeOffset < 0 {
			search = bodyStart
			continue
		}
		body := strings.TrimPrefix(staging[bodyStart:bodyStart+closeOffset], "\n")
		search = bodyStart + closeOffset + len(closeMarker)
		if strings.TrimSpace(body) == "" {
			continue
		}

		index++
		parts := splitBody(body, MaxRuleFileBytes-len(frontmatter))
		for i, part := range parts {
			name := fmt.Sprintf("gentle-ai-%02d-%s.md", index, id)
			if len(parts) > 1 {
				name = fmt.Sprintf("gentle-ai-%02d-%s-part%d.md", index, id, i+1)
			}
			rules = append(rules, Rule{Name: name, Content: frontmatter + part})
		}
	}
	return rules
}

// splitBody packs heading-delimited blocks into chunks of at most limit bytes.
// A block larger than limit is split at line boundaries, and a single line
// larger than limit at rune boundaries.
func splitBody(body string, limit int) []string {
	if len(body) <= limit {
		return []string{body}
	}
	var chunks []string
	current := ""
	add := func(piece string) {
		if len(current)+len(piece) > limit && current != "" {
			chunks = append(chunks, current)
			current = ""
		}
		current += piece
	}
	for _, block := range headingBlocks(body) {
		if len(block) <= limit {
			add(block)
			continue
		}
		for _, line := range strings.SplitAfter(block, "\n") {
			for len(line) > limit {
				cut := limit
				for cut > 0 && !utf8.RuneStart(line[cut]) {
					cut--
				}
				add(line[:cut])
				line = line[cut:]
			}
			if line != "" {
				add(line)
			}
		}
	}
	if current != "" {
		chunks = append(chunks, current)
	}
	return chunks
}

// headingBlocks splits markdown before every #, ## or ### heading that is not
// inside a fenced code block.
func headingBlocks(body string) []string {
	var blocks []string
	var current strings.Builder
	inFence := false
	for _, line := range strings.SplitAfter(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
		}
		heading := strings.HasPrefix(line, "# ") || strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "### ")
		if heading && !inFence && current.Len() > 0 {
			blocks = append(blocks, current.String())
			current.Reset()
		}
		current.WriteString(line)
	}
	if current.Len() > 0 {
		blocks = append(blocks, current.String())
	}
	return blocks
}
