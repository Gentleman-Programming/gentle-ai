package assets

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRDDDefectWorkflowSkillContract(t *testing.T) {
	const embeddedPath = "skills/rdd-defect-workflow/SKILL.md"

	content, err := Read(embeddedPath)
	if err != nil {
		t.Fatalf("Read(%q) error = %v", embeddedPath, err)
	}

	source, err := os.ReadFile(filepath.Join("..", "..", "skills", "rdd-defect-workflow", "SKILL.md"))
	if err != nil {
		t.Fatalf("ReadFile(source skill) error = %v", err)
	}
	if !bytes.Equal(source, []byte(content)) {
		t.Fatal("public source skill and embedded distribution asset differ")
	}

	sections := []string{"## Activation Contract", "## Hard Rules", "## Decision Gates", "## Execution Steps", "## Output Contract", "## References"}
	position := -1
	for _, section := range sections {
		next := strings.Index(content, section)
		if next <= position {
			t.Fatalf("section %q missing or out of order", section)
		}
		position = next
	}
	if words := len(strings.Fields(content)); words > 700 {
		t.Fatalf("skill has %d words, want at most 700", words)
	}

	for _, required := range []string{
		"does not grant issue approval, label, review, exception, or merge authority",
		"`disabled/unmanaged`", "do not start receipt reviews or fabricate approval",
		"current `main`", "`status:approved`", "superseded or conflicting",
		"causal authority invariant", "rollback boundary", "every operator flow",
		"one truthful black-box bench journey", "actual runtime E2E proof",
		"Synthetic proxy coverage", "CodeGraph-first", "dedicated worktree",
		"behavior-first tests", "source-mutating normalization", "400 additions plus deletions",
		"independent read-only", "maintainer-owned", "unresolved_authority_decisions",
	} {
		if !strings.Contains(content, required) {
			t.Errorf("skill missing contract %q", required)
		}
	}

	for _, forbidden := range []string{"/home/", `C:\Users\`, "local memory"} {
		if strings.Contains(content, forbidden) {
			t.Errorf("skill contains environment-specific dependency %q", forbidden)
		}
	}
}
