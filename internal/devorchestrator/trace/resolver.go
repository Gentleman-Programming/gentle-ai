package trace

import (
	"bufio"
	"bytes"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Node represents the traceability metadata of a requirement, spec, or task.
type Node struct {
	ID             string   `yaml:"id"`
	Implements     []string `yaml:"implements"`
	OriginatesFrom []string `yaml:"originates-from"`
}

// ParseTraceability reads a markdown file and extracts its frontmatter into a Node.
func ParseTraceability(filePath string) (*Node, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	frontmatter, err := extractFrontmatter(data)
	if err != nil {
		return nil, err
	}
	if frontmatter == nil {
		// Return empty node if no frontmatter is found, to prevent panics and allow defaults
		return &Node{}, nil
	}

	var node Node
	err = yaml.Unmarshal(frontmatter, &node)
	if err != nil {
		return nil, err
	}

	return &node, nil
}

// extractFrontmatter extracts the YAML frontmatter enclosed by "---" lines.
func extractFrontmatter(data []byte) ([]byte, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	var frontmatter bytes.Buffer
	inFrontmatter := false
	started := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if trimmed == "---" {
			if !started {
				started = true
				inFrontmatter = true
				continue
			}
			if inFrontmatter {
				break
			}
		}

		if inFrontmatter {
			frontmatter.WriteString(line)
			frontmatter.WriteString("\n")
		} else if trimmed != "" {
			// If we hit non-empty text before "---", there is no frontmatter
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if frontmatter.Len() == 0 {
		return nil, nil
	}

	return frontmatter.Bytes(), nil
}
