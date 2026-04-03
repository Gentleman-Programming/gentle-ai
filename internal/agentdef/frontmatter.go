package agentdef

import (
	"fmt"
	"strings"
)

const frontmatterDelimiter = "---"

// ParseFrontmatter parses a Markdown file with YAML frontmatter.
// The format is:
//
//	---
//	name: "architect"
//	model: "opus"
//	allowedTools:
//	  - Read
//	  - Grep
//	---
//
//	Agent body text here...
func ParseFrontmatter(data []byte) (AgentDef, error) {
	content := string(data)

	front, body, err := splitFrontmatter(content)
	if err != nil {
		return AgentDef{}, err
	}

	def, err := parseFrontmatterBlock(front)
	if err != nil {
		return AgentDef{}, err
	}

	def.Body = strings.TrimSpace(body)
	return def, nil
}

// splitFrontmatter splits content into frontmatter and body sections.
func splitFrontmatter(content string) (front, body string, err error) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, frontmatterDelimiter) {
		return "", content, nil
	}

	// Strip leading ---
	rest := strings.TrimPrefix(content, frontmatterDelimiter)
	rest = strings.TrimLeft(rest, "\r\n")

	// Find closing ---
	idx := strings.Index(rest, "\n"+frontmatterDelimiter)
	if idx < 0 {
		return "", "", fmt.Errorf("unclosed frontmatter block (missing closing ---)")
	}

	front = strings.TrimSpace(rest[:idx])
	body = strings.TrimSpace(rest[idx+len("\n"+frontmatterDelimiter):])

	return front, body, nil
}

// parseFrontmatterBlock parses the YAML frontmatter into an AgentDef.
func parseFrontmatterBlock(front string) (AgentDef, error) {
	var def AgentDef

	lines := strings.Split(front, "\n")
	i := 0

	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			i++
			continue
		}

		colonIdx := strings.Index(trimmed, ":")
		if colonIdx < 0 {
			i++
			continue
		}

		key := strings.TrimSpace(trimmed[:colonIdx])
		value := strings.TrimSpace(trimmed[colonIdx+1:])

		switch key {
		case "name":
			def.Name = unquoteStr(value)
		case "description":
			def.Description = unquoteStr(value)
		case "model":
			def.Model = unquoteStr(value)
		case "context":
			def.Context = unquoteStr(value)
		case "extends":
			def.Extends = unquoteStr(value)
		case "allowedTools":
			// Value is empty — parse list on following lines
			tools, newI := parseStringList(lines, i+1)
			def.AllowedTools = tools
			i = newI
			continue
		case "hooks":
			// Value is empty — parse map on following lines
			hooksMap, newI := parseStringMap(lines, i+1)
			def.Hooks = hooksMap
			i = newI
			continue
		case "rules":
			// Value is empty — parse map on following lines
			rulesMap, newI := parseAnyMap(lines, i+1)
			def.Rules = rulesMap
			i = newI
			continue
		}

		i++
	}

	if def.Name == "" {
		return AgentDef{}, fmt.Errorf("agent frontmatter missing required field: name")
	}

	return def, nil
}

// parseStringList reads a YAML list block starting after the given line index.
// Returns the list values and the next unprocessed line index.
func parseStringList(lines []string, start int) ([]string, int) {
	var result []string
	i := start

	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			break
		}
		indent := countIndent(line)
		if indent == 0 && !strings.HasPrefix(trimmed, "-") {
			break
		}
		if strings.HasPrefix(trimmed, "-") {
			val := strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
			result = append(result, unquoteStr(val))
		}
		i++
	}

	return result, i
}

// parseStringMap reads a YAML mapping block (string values only).
func parseStringMap(lines []string, start int) (map[string]string, int) {
	result := make(map[string]string)
	i := start

	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || countIndent(line) == 0 {
			break
		}
		if colonIdx := strings.Index(trimmed, ":"); colonIdx >= 0 {
			k := strings.TrimSpace(trimmed[:colonIdx])
			v := strings.TrimSpace(trimmed[colonIdx+1:])
			result[k] = unquoteStr(v)
		}
		i++
	}

	return result, i
}

// parseAnyMap reads a YAML mapping block with mixed value types.
func parseAnyMap(lines []string, start int) (map[string]any, int) {
	result := make(map[string]any)
	i := start

	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || countIndent(line) == 0 {
			break
		}
		if colonIdx := strings.Index(trimmed, ":"); colonIdx >= 0 {
			k := strings.TrimSpace(trimmed[:colonIdx])
			v := strings.TrimSpace(trimmed[colonIdx+1:])
			result[k] = parseScalar(v)
		}
		i++
	}

	return result, i
}

// parseScalar returns the Go value for a YAML scalar: bool, int, or string.
func parseScalar(s string) any {
	s = unquoteStr(s)
	switch s {
	case "true":
		return true
	case "false":
		return false
	}
	if isIntStr(s) {
		n := 0
		for _, ch := range s {
			n = n*10 + int(ch-'0')
		}
		return n
	}
	return s
}

func unquoteStr(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && ((s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'')) {
		return s[1 : len(s)-1]
	}
	return s
}

func isIntStr(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func countIndent(line string) int {
	count := 0
	for _, ch := range line {
		if ch == ' ' {
			count++
		} else {
			break
		}
	}
	return count
}
