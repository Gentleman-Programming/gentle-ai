package trace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseTraceability(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name        string
		content     string
		expected    *Node
		expectEmpty bool
	}{
		{
			name: "Valid frontmatter",
			content: `---
id: feature-12435
implements:
  - spec-061
originates-from:
  - req-001
---
# Content here
`,
			expected: &Node{
				ID:             "feature-12435",
				Implements:     []string{"spec-061"},
				OriginatesFrom: []string{"req-001"},
			},
		},
		{
			name: "No frontmatter",
			content: `# Just some content
No frontmatter here.
`,
			expectEmpty: true,
		},
		{
			name:        "Empty file",
			content:     ``,
			expectEmpty: true,
		},
		{
			name: "Frontmatter with missing fields",
			content: `---
id: task-091
---
# Content
`,
			expected: &Node{
				ID: "task-091",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			filePath := filepath.Join(tempDir, tc.name+".md")
			err := os.WriteFile(filePath, []byte(tc.content), 0644)
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}

			node, err := ParseTraceability(filePath)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if tc.expectEmpty {
				if node.ID != "" || len(node.Implements) > 0 || len(node.OriginatesFrom) > 0 {
					t.Errorf("Expected empty node, got %+v", node)
				}
				return
			}

			if node.ID != tc.expected.ID {
				t.Errorf("Expected ID %q, got %q", tc.expected.ID, node.ID)
			}

			if len(node.Implements) != len(tc.expected.Implements) {
				t.Errorf("Expected %d Implements, got %d", len(tc.expected.Implements), len(node.Implements))
			} else {
				for i, v := range node.Implements {
					if v != tc.expected.Implements[i] {
						t.Errorf("Expected Implements[%d]=%q, got %q", i, tc.expected.Implements[i], v)
					}
				}
			}

			if len(node.OriginatesFrom) != len(tc.expected.OriginatesFrom) {
				t.Errorf("Expected %d OriginatesFrom, got %d", len(tc.expected.OriginatesFrom), len(node.OriginatesFrom))
			} else {
				for i, v := range node.OriginatesFrom {
					if v != tc.expected.OriginatesFrom[i] {
						t.Errorf("Expected OriginatesFrom[%d]=%q, got %q", i, tc.expected.OriginatesFrom[i], v)
					}
				}
			}
		})
	}
}
