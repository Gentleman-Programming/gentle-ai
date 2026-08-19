package assets

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// devAgentCanonicalNames lists the 12 dev-agent basenames whose canonical,
// maintained definition lives under internal/assets/claude/agents/*.md.
// skills/agents/*/SKILL.md must be a thin, derived pointer to each one.
var devAgentCanonicalNames = []string{
	"backend-implementer",
	"database-specialist",
	"dev-designer",
	"dev-explorer",
	"dev-orchestrator",
	"dev-proposer",
	"dev-specifier",
	"dev-task-planner",
	"dev-verifier",
	"frontend-implementer",
	"project-bootstrap",
	"solution-architect",
}

var derivedFrontmatterNameRe = regexp.MustCompile(`(?m)^name:\s*(\S+)\s*$`)

// TestDevAgentParity enforces P1-P5 from design D4: skills/agents/*/SKILL.md
// must be a thin pointer to the canonical internal/assets/claude/agents/*.md
// definition — never a second source of truth.
func TestDevAgentParity(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..")
	skillsAgentsDir := filepath.Join(repositoryRoot, "skills", "agents")

	entries, err := os.ReadDir(skillsAgentsDir)
	if err != nil {
		t.Fatalf("read %s: %v", skillsAgentsDir, err)
	}

	var actualDirs []string
	for _, e := range entries {
		if e.IsDir() {
			actualDirs = append(actualDirs, e.Name())
		}
	}
	sort.Strings(actualDirs)

	expectedDirs := append([]string(nil), devAgentCanonicalNames...)
	sort.Strings(expectedDirs)

	// P1 — set equality: skills/agents/* dir names == the 12 dev-agent
	// canonical basenames (fails on an orphan on either side).
	if strings.Join(actualDirs, ",") != strings.Join(expectedDirs, ",") {
		t.Fatalf("P1 set-equality violated: skills/agents/* dirs %v != canonical dev-agent basenames %v", actualDirs, expectedDirs)
	}

	for _, name := range devAgentCanonicalNames {
		name := name
		t.Run(name, func(t *testing.T) {
			derivedPath := filepath.Join(skillsAgentsDir, name, "SKILL.md")
			derivedBytes, err := os.ReadFile(derivedPath)
			if err != nil {
				t.Fatalf("%s: read derived SKILL.md: %v", name, err)
			}
			derived := string(derivedBytes)

			canonicalRelPath := "claude/agents/" + name + ".md"
			canonicalBytes, err := FS.ReadFile(canonicalRelPath)
			if err != nil {
				t.Fatalf("%s: P4 canonical path missing from embed FS: internal/assets/%s: %v", name, canonicalRelPath, err)
			}
			canonical := string(canonicalBytes)

			// P2 — derived frontmatter name == canonical frontmatter name ==
			// directory basename.
			derivedMatch := derivedFrontmatterNameRe.FindStringSubmatch(derived)
			if derivedMatch == nil {
				t.Fatalf("%s: P2 derived SKILL.md missing a `name:` frontmatter field", name)
			}
			if derivedMatch[1] != name {
				t.Fatalf("%s: P2 derived frontmatter name %q != directory basename %q", name, derivedMatch[1], name)
			}
			canonicalMatch := derivedFrontmatterNameRe.FindStringSubmatch(canonical)
			if canonicalMatch == nil {
				t.Fatalf("%s: P2 canonical internal/assets/%s missing a `name:` frontmatter field", name, canonicalRelPath)
			}
			if canonicalMatch[1] != name {
				t.Fatalf("%s: P2 canonical frontmatter name %q != directory basename %q", name, canonicalMatch[1], name)
			}

			// P3 — derived body contains the do-not-hand-edit marker.
			const marker = "<!-- derived: do not hand-edit -->"
			if !strings.Contains(derived, marker) {
				t.Fatalf("%s: P3 derived SKILL.md missing marker %q", name, marker)
			}

			// P4 — derived body references the canonical path, and that path
			// exists in the embed FS (checked above via FS.ReadFile).
			canonicalRef := "internal/assets/claude/agents/" + name + ".md"
			if !strings.Contains(derived, canonicalRef) {
				t.Fatalf("%s: P4 derived SKILL.md must reference canonical path %q", name, canonicalRef)
			}

			// P5 — derived file <= 25 lines (thin-pointer budget).
			lineCount := strings.Count(derived, "\n")
			if !strings.HasSuffix(derived, "\n") && len(derived) > 0 {
				lineCount++
			}
			if lineCount > 25 {
				t.Fatalf("%s: P5 derived SKILL.md is %d lines, budget is <=25", name, lineCount)
			}
		})
	}
}
