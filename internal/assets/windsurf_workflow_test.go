package assets

import (
	"strings"
	"testing"
)

func TestWindsurfSDDNewWorkflowOrdersPreProposalLifecycle(t *testing.T) {
	workflow := MustRead("windsurf/workflows/sdd-new.md")
	orderedSteps := []string{
		"### 1. Initialize OpenSpec and Engram lifecycle",
		"### 2. Run local exploration",
		"### 3. Run optional admitted research",
		"### 4. Confirm product decisions",
		"### 5. Create proposal and specification",
	}

	previous := -1
	for _, step := range orderedSteps {
		index := strings.Index(workflow, step)
		if index < 0 {
			t.Fatalf("Windsurf sdd-new workflow missing %q", step)
		}
		if index <= previous {
			t.Fatalf("Windsurf sdd-new workflow step %q is out of order", step)
		}
		previous = index
	}

	for _, required := range []string{
		"openspec/changes/<change>/",
		"sdd/<change>/",
		"`sdd-explore`",
		"`sdd-research`",
		"exact admitted evidence grants",
		"decisions are confirmed",
		"`sdd-propose`",
		"`sdd-spec`",
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("Windsurf sdd-new workflow missing lifecycle contract %q", required)
		}
	}
}

func TestWindsurfSDDNewWorkflowRejectsLegacySDDArtifacts(t *testing.T) {
	workflow := MustRead("windsurf/workflows/sdd-new.md")

	for _, legacy := range []string{".sdd/", ".sdd/proposal.md", ".sdd/spec.md"} {
		if strings.Contains(workflow, legacy) {
			t.Errorf("Windsurf sdd-new workflow still creates legacy artifact %q", legacy)
		}
	}
}
