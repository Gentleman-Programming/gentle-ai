package agentbuilder

import (
	"errors"
	"regexp"
	"strings"
)

var (
	// reCodeFenceOpen matches opening code fences (```markdown, ```md, ```, etc.)
	reCodeFenceOpen = regexp.MustCompile("(?m)^```[a-zA-Z]*\\s*$")
	// reCodeFenceClose matches closing code fences
	reCodeFenceClose = regexp.MustCompile("(?m)^```\\s*$")
	// reH1 matches a Markdown H1 line
	reH1 = regexp.MustCompile(`(?m)^#\s+(.+)$`)
	// reH2Section extracts content from a ## section until the next ## or EOF
	reH2Section = regexp.MustCompile(`(?ms)^##\s+%s\s*\n(.*?)(?:^##\s|\z)`)
)

// Parse converts raw AI output into a GeneratedAgent.
// It strips code fences, extracts the required sections, and validates completeness.
func Parse(raw string) (*GeneratedAgent, error) {
	cleaned := stripCodeFences(raw)
	cleaned = strings.TrimSpace(cleaned)

	if cleaned == "" {
		return nil, errors.New("parse: empty content after stripping code fences")
	}

	title, err := extractTitle(cleaned)
	if err != nil {
		return nil, err
	}

	description, err := extractSection(cleaned, "Description")
	if err != nil {
		return nil, err
	}

	trigger, err := extractSection(cleaned, "Trigger")
	if err != nil {
		return nil, err
	}

	// Instructions is required for validation but stored only in Content.
	if _, err := extractSection(cleaned, "Instructions"); err != nil {
		return nil, err
	}

	name := titleToName(title)
	if name == "" {
		return nil, errors.New("parse: generated agent title produced no valid name characters")
	}

	return &GeneratedAgent{
		Name:        name,
		Title:       title,
		Description: strings.TrimSpace(description),
		Trigger:     strings.TrimSpace(trigger),
		Content:     cleaned,
	}, nil
}

// stripCodeFences removes leading/trailing code fence markers.
// It handles ```markdown, ```md, ``` and similar variants.
func stripCodeFences(raw string) string {
	// Remove leading fence.
	loc := reCodeFenceOpen.FindStringIndex(raw)
	if loc != nil && loc[0] == 0 {
		raw = raw[loc[1]:]
	} else if loc != nil {
		// Check if only whitespace precedes the fence.
		prefix := strings.TrimSpace(raw[:loc[0]])
		if prefix == "" {
			raw = raw[loc[1]:]
		}
	}

	// Remove trailing fence.
	closeLoc := reCodeFenceClose.FindAllStringIndex(raw, -1)
	if closeLoc != nil {
		last := closeLoc[len(closeLoc)-1]
		suffix := strings.TrimSpace(raw[last[1]:])
		if suffix == "" {
			raw = raw[:last[0]]
		}
	}

	return raw
}

// extractTitle finds the first H1 line and returns its text.
func extractTitle(content string) (string, error) {
	m := reH1.FindStringSubmatch(content)
	if m == nil {
		return "", errors.New("parse: missing '# Title' section")
	}
	return strings.TrimSpace(m[1]), nil
}

// extractSection returns the body of the named ## section.
// It returns an error when the section is absent or empty.
func extractSection(content, name string) (string, error) {
	pattern := regexp.MustCompile(`(?ms)^##\s+` + regexp.QuoteMeta(name) + `\s*\n(.*?)(?:^##\s|\z)`)
	m := pattern.FindStringSubmatch(content)
	if m == nil {
		return "", errors.New("parse: missing '## " + name + "' section")
	}
	body := strings.TrimSpace(m[1])
	if body == "" {
		return "", errors.New("parse: '## " + name + "' section is empty")
	}
	return body, nil
}

// titleToName converts a title string to a kebab-case name safe for directory use.
// Example: "My Custom Agent" → "my-custom-agent"
func titleToName(title string) string {
	// Lowercase.
	s := strings.ToLower(title)
	// Replace non-alphanumeric characters with hyphens.
	var sb strings.Builder
	prevHyphen := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
			prevHyphen = false
		} else {
			if !prevHyphen && sb.Len() > 0 {
				sb.WriteRune('-')
				prevHyphen = true
			}
		}
	}
	return strings.TrimRight(sb.String(), "-")
}
