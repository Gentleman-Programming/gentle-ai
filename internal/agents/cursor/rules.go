package cursor

import (
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

// WrapRule adds Cursor's activation metadata to a managed Markdown body.
func WrapRule(content string) string {
	return "---\ndescription: Gentle AI rules\nalwaysApply: true\n---\n\n" + content
}

// ValidateRule requires valid MDC frontmatter with automatic activation enabled.
func ValidateRule(content []byte) error {
	lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	if len(lines) < 2 || lines[0] != "---" {
		return fmt.Errorf("Cursor rule is missing YAML frontmatter; run `gentle-ai sync` to repair it")
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return fmt.Errorf("Cursor rule has unclosed YAML frontmatter; run `gentle-ai sync` to repair it")
	}
	decoder := yaml.NewDecoder(strings.NewReader(strings.Join(lines[1:end], "\n")))
	var metadata map[string]any
	if err := decoder.Decode(&metadata); err != nil {
		return fmt.Errorf("Cursor rule has invalid YAML frontmatter: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err != nil {
			return fmt.Errorf("Cursor rule has invalid YAML frontmatter: %w", err)
		}
		return fmt.Errorf("Cursor rule frontmatter must contain exactly one YAML document")
	}
	if enabled, ok := metadata["alwaysApply"].(bool); !ok || !enabled {
		return fmt.Errorf("Cursor rule requires boolean alwaysApply: true; run `gentle-ai sync` to repair it")
	}
	return nil
}
