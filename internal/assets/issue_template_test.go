package assets

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoryIssueTemplatesUseMarkdownContract(t *testing.T) {
	root := filepath.Join("..", "..", ".github", "ISSUE_TEMPLATE")
	for _, name := range []string{"bug_report.yml", "feature_request.yml"} {
		if _, err := os.Stat(filepath.Join(root, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("legacy Issue Form %s must not exist: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "config.yml")); err != nil {
		t.Fatalf("issue chooser config must remain present: %v", err)
	}

	tests := []struct {
		name, file, frontmatter string
		headings                []string
	}{
		{
			name: "bug", file: "bug_report.md",
			frontmatter: "---\nname: Bug Report\nabout: File a bug report for Gentle AI\ntitle: \"\"\nlabels: \"bug, status:needs-review\"\nassignees: \"\"\n---\n",
			headings:    []string{"Approval Workflow", "Pre-flight Checklist", "Bug Description", "Steps to Reproduce", "Expected Behavior", "Actual Behavior", "Gentle AI Version", "Operating System", "AI Agent / Client", "Affected Area", "Logs / Error Output", "Additional Context"},
		},
		{
			name: "feature", file: "feature_request.md",
			frontmatter: "---\nname: Feature Request\nabout: Suggest a new feature or enhancement for Gentle AI\ntitle: \"\"\nlabels: \"enhancement, status:needs-review\"\nassignees: \"\"\n---\n",
			headings:    []string{"Approval Workflow", "Pre-flight Checklist", "Affected Area", "Problem Statement", "Proposed Solution", "Alternatives Considered", "Additional Context"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join(root, tt.file))
			if err != nil {
				t.Fatalf("read template: %v", err)
			}
			text := string(content)
			if !strings.HasPrefix(text, tt.frontmatter) {
				t.Fatalf("frontmatter does not match the Markdown template contract:\n%s", text)
			}
			for _, heading := range tt.headings {
				if !strings.Contains(text, "\n## "+heading+"\n") {
					t.Errorf("missing required section %q", heading)
				}
			}
		})
	}
}
