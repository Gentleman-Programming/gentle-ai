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
	for _, required := range []string{"internal/assets/skills/issue-creation/SKILL.md", "CONTRIBUTING.md", ".github/ISSUE_TEMPLATE", "discovered GitHub labels"} {
		if !strings.Contains(collaborationText, required) {
			t.Fatalf("collaboration skill must reference canonical issue policy source %q", required)
		}
	}
	if strings.Contains(collaborationText, "gh issue create") {
		t.Fatal("collaboration skill must delegate issue publication to the canonical authority, not carry direct gh issue create mechanics")
	}
	for _, stale := range []string{"status:approved` from a maintainer", "| Add `status:approved` to an issue | ❌ | ✅ |"} {
		if strings.Contains(collaborationText, stale) {
			t.Fatalf("collaboration skill retains stale approval authority %q", stale)
		}
	}
	if strings.Contains(collaborationText, "## Pending maintainer actions") || !strings.Contains(collaborationText, "## Pending repository workflow actions") {
		t.Fatal("collaboration skill must use a neutral pending repository workflow heading")
	}

	branch, err := os.ReadFile(filepath.Join(repositoryRoot, "skills", "branch-pr", "SKILL.md"))
	if err != nil {
		t.Fatalf("read branch-pr skill: %v", err)
	}
	if strings.Contains(string(branch), "Wait for maintainer to add `status:approved` to the issue") {
		t.Fatal("branch-pr skill retains stale maintainer-only approval instruction")
	}
}

func TestPRLabelMutationsUseCanonicalIssueCreationAuthority(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..")
	for _, path := range []string{
		filepath.Join(repositoryRoot, "skills", "gentle-ai-collab-perfect", "SKILL.md"),
		filepath.Join(repositoryRoot, "skills", "branch-pr", "SKILL.md"),
	} {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(content)
		for _, required := range []string{"canonical issue-creation workflow contract", "current direct human instruction binds the exact target/action", "target-host capability", "one bounded mutation and target-host readback", "Ordinary `type:*` categorization", "Protected policy labels", "verified policy authority", "target-host `viewerPermission` `MAINTAIN` or `ADMIN`", "`size:exception` additionally requires documented over-budget rationale"} {
			if !strings.Contains(text, required) {
				t.Errorf("%s must require canonical PR-label authority marker %q", path, required)
			}
		}
		for _, stale := range []string{"| Apply `type:*` label to a PR | ❌ | ✅ |", "| Apply `size:exception` label | ❌ | ✅ |", "maintainer-applied `size:exception`", "Ask a maintainer to add the correct label; remove extras", "gh pr edit"} {
			if strings.Contains(text, stale) {
				t.Errorf("%s retains stale or unguarded PR-label guidance %q", path, stale)
			}
		}
	}
}

func TestDelegatedWorkflowMutationContract(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..")
	const delegatedWorkflowReference = "skills/issue-creation/references/delegated-workflow-actions.md"
	reference := MustRead(delegatedWorkflowReference)
	if readReference, err := Read(delegatedWorkflowReference); err != nil || readReference != reference {
		t.Fatalf("Read(%q) differs from MustRead: %v", delegatedWorkflowReference, err)
	}
	contributing, err := os.ReadFile(filepath.Join(repositoryRoot, "CONTRIBUTING.md"))
	if err != nil {
		t.Fatalf("read CONTRIBUTING.md: %v", err)
	}
	workflow, err := os.ReadFile(filepath.Join(repositoryRoot, ".github", "workflows", "pr-check.yml"))
	if err != nil {
		t.Fatalf("read pr-check workflow: %v", err)
	}
	for path, content := range map[string]string{"CONTRIBUTING.md": string(contributing), ".github/workflows/pr-check.yml": string(workflow)} {
		for _, stale := range []string{"a maintainer will add the `status:approved` label", "has been approved by a maintainer", "Issues must be approved by a maintainer before work begins.", "Please comment on the issue and wait for it to be labelled status:approved."} {
			if strings.Contains(content, stale) {
				t.Errorf("%s retains stale maintainer-only approval authority %q", path, stale)
			}
		}
		if !strings.Contains(content, "canonical issue-creation workflow contract") {
			t.Errorf("%s must route approval authority to the canonical issue-creation workflow contract", path)
		}
	}
	for _, condition := range []string{"const hasException = labels.includes('size:exception');", "if (!labels.includes('status:approved')) {"} {
		if !strings.Contains(string(workflow), condition) {
			t.Errorf("pr-check workflow must retain enforcement condition %q", condition)
		}
	}
}
