// Package antigravityrules publishes the Gentle AI guidance staged for
// Antigravity (agy 1.4+) as always-on global rule files.
//
// Every Gentle AI prompt writer appends its marker section to the adapter's
// SystemPromptFile. agy truncates each rule file at MaxRuleFileBytes, so every
// staged section is rendered, in order, into
// ~/.gemini/config/rules/gentle-ai-NN-<section-id>[-partK].md, and oversized
// sections are split at heading (fallback: line) boundaries (#5676, #1989).
// Parts past AlwaysOnBudgetBytes are published as model_decision rules.
package antigravityrules

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxRuleFileBytes is agy's per-rule-file cap; content past it is truncated.
const MaxRuleFileBytes = 24000

// AlwaysOnBudgetBytes caps the always_on rule bytes Render publishes. agy
// shares a 20,000-token budget across every always-on rule and demotes the
// overflow to bare path pointers in its own order, which dropped the
// orchestrator. On agy 1.4.12, 55,053 bytes of these rules stayed inline next
// to a 242-byte GEMINI.md; this cap keeps headroom for a few user rules.
const AlwaysOnBudgetBytes = 55000

// managedHeader makes a global rule load in every agy session.
const (
	managedHeader = "---\ntrigger: always_on\n---\n"
	frontmatter   = managedHeader + "\n"
)

// maxDescriptionBytes bounds a demoted rule's description, so every part,
// whichever frontmatter it gets, fits in MaxRuleFileBytes.
const maxDescriptionBytes = 200

// corePriority lists the sections whose first part claims the always-on
// budget first, in this order; every other part follows in rendered order.
var corePriority = []string{"persona", "engram-protocol", "orchestrator", "agent-routing"}

var (
	openMarkerPattern = regexp.MustCompile(`<!-- gentle-ai:([A-Za-z0-9_.-]+) -->`)
	// managedHeaderPattern matches both frontmatters Render writes; Publish
	// only removes owned-name files that start with one of them.
	managedHeaderPattern = regexp.MustCompile(`\A---\ntrigger: (always_on|model_decision\ndescription: "[^"\\\n]*")\n---\n`)
)

// demotedFrontmatter makes agy list the rule by description and load it on
// demand instead of inline.
func demotedFrontmatter(description string) string {
	return "---\ntrigger: model_decision\ndescription: \"" + description + "\"\n---\n\n"
}

// Rule is one rendered agy rule file.
type Rule struct {
	Name    string
	Content string
}

// Section is one complete gentle-ai marker section: content[Start:End] spans
// both markers, and Body is the text between them without the newline that
// follows the opening marker.
type Section struct {
	ID         string
	Start, End int
	Body       string
}

// Sections returns every complete gentle-ai marker section of content in
// order. An opening marker without its closing marker is skipped.
func Sections(content string) []Section {
	var sections []Section
	for search := 0; ; {
		loc := openMarkerPattern.FindStringSubmatchIndex(content[search:])
		if loc == nil {
			return sections
		}
		id := content[search+loc[2] : search+loc[3]]
		bodyStart := search + loc[1]
		closeMarker := "<!-- /gentle-ai:" + id + " -->"
		closeOffset := strings.Index(content[bodyStart:], closeMarker)
		if closeOffset < 0 {
			search = bodyStart
			continue
		}
		end := bodyStart + closeOffset + len(closeMarker)
		sections = append(sections, Section{
			ID:    id,
			Start: search + loc[0],
			End:   end,
			Body:  strings.TrimPrefix(content[bodyStart:bodyStart+closeOffset], "\n"),
		})
		search = end
	}
}

// Render turns every complete gentle-ai marker section of the staging content
// into rule files no larger than MaxRuleFileBytes. Concatenating the bodies of
// a section's parts, in order, yields the section body unchanged. Parts claim
// AlwaysOnBudgetBytes greedily, corePriority first; a part that does not fit
// becomes a model_decision rule described by its first heading.
func Render(staging string) []Rule {
	var rules []Rule
	var bodies, descriptions []string
	var ranks []int
	limit := MaxRuleFileBytes - len(demotedFrontmatter(strings.Repeat("x", maxDescriptionBytes)))
	index := 0
	for _, section := range Sections(staging) {
		if strings.TrimSpace(section.Body) == "" {
			continue
		}
		index++
		parts := splitBody(section.Body, limit)
		for i, part := range parts {
			name := fmt.Sprintf("gentle-ai-%02d-%s.md", index, section.ID)
			if len(parts) > 1 {
				name = fmt.Sprintf("gentle-ai-%02d-%s-part%d.md", index, section.ID, i+1)
			}
			rank := slices.Index(corePriority, section.ID)
			if rank < 0 || i > 0 {
				rank = len(corePriority)
			}
			rules = append(rules, Rule{Name: name, Content: frontmatter + part})
			bodies = append(bodies, part)
			descriptions = append(descriptions, describe(section.ID, i, len(parts), part))
			ranks = append(ranks, rank)
		}
	}

	order := make([]int, len(rules))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return ranks[a] - ranks[b] })
	inline := 0
	for _, i := range order {
		if inline+len(rules[i].Content) <= AlwaysOnBudgetBytes {
			inline += len(rules[i].Content)
			continue
		}
		rules[i].Content = demotedFrontmatter(descriptions[i]) + bodies[i]
	}
	return rules
}

// describe tells the model what a demoted part covers and when to read it,
// bounded by maxDescriptionBytes and free of characters that would break the
// double-quoted YAML value.
func describe(id string, part, parts int, body string) string {
	topic := id
	for _, block := range headingBlocks(body) {
		if line, _, _ := strings.Cut(block, "\n"); isHeading(line) {
			topic = strings.TrimSpace(strings.TrimLeft(line, "#"))
			break
		}
	}
	description := "Gentle AI " + strings.TrimSuffix(id, "-guidance") + " guidance"
	if parts > 1 {
		description += fmt.Sprintf(" (part %d of %d)", part+1, parts)
	}
	description += "; read this file before work that involves: " + topic
	description = strings.Map(func(r rune) rune {
		if r == '"' || r == '\\' || unicode.IsControl(r) {
			return -1
		}
		return r
	}, description)
	for len(description) > maxDescriptionBytes {
		_, size := utf8.DecodeLastRuneInString(description)
		description = description[:len(description)-size]
	}
	return description
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
		if isHeading(line) && !inFence && current.Len() > 0 {
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

// isHeading reports whether line opens a #, ## or ### markdown heading.
func isHeading(line string) bool {
	return strings.HasPrefix(line, "# ") || strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "### ")
}
