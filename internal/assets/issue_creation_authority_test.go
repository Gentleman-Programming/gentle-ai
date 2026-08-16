package assets

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIssueCreationAuthorityBoundary(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..")
	duplicatePath := filepath.Join(repositoryRoot, "skills", "issue-creation", "SKILL.md")
	if _, err := os.Stat(duplicatePath); !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			t.Fatalf("duplicate project issue-creation authority still exists at %s", duplicatePath)
		}
		t.Fatalf("stat duplicate project issue-creation authority: %v", err)
	}

	agents, err := os.ReadFile(filepath.Join(repositoryRoot, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	const canonicalRegistryRow = "| `issue-creation` | When creating a GitHub issue, reporting a bug, or requesting a feature. | [`internal/assets/skills/issue-creation/SKILL.md`](internal/assets/skills/issue-creation/SKILL.md) |"
	if !strings.Contains(string(agents), canonicalRegistryRow) {
		t.Fatalf("AGENTS.md must route the canonical issue-creation identity directly to the embedded authority; missing row %q", canonicalRegistryRow)
	}
	for _, stale := range []string{"gentle-ai-issue-creation", "[`skills/issue-creation/SKILL.md`](skills/issue-creation/SKILL.md)"} {
		if strings.Contains(string(agents), stale) {
			t.Fatalf("AGENTS.md still references stale issue-creation authority %q", stale)
		}
	}

	canonical := MustRead("skills/issue-creation/SKILL.md")
	if !strings.Contains(canonical, "\nname: issue-creation\n") {
		t.Fatal("embedded issue-creation authority must retain canonical frontmatter identity name: issue-creation")
	}

	collaborationPath := filepath.Join(repositoryRoot, "skills", "gentle-ai-collab-perfect", "SKILL.md")
	collaboration, err := os.ReadFile(collaborationPath)
	if err != nil {
		t.Fatalf("read collaboration skill: %v", err)
	}
	collaborationText := string(collaboration)
	for _, required := range []string{
		"internal/assets/skills/issue-creation/SKILL.md",
		"CONTRIBUTING.md",
		".github/ISSUE_TEMPLATE",
		"discovered GitHub labels",
	} {
		if !strings.Contains(collaborationText, required) {
			t.Fatalf("collaboration skill must reference canonical issue policy source %q", required)
		}
	}
	if strings.Contains(collaborationText, "gh issue create") {
		t.Fatal("collaboration skill must delegate issue publication to the canonical authority, not carry direct gh issue create mechanics")
	}
}
