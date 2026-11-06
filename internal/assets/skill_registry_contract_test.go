package assets

import (
	"strings"
	"testing"
)

// The skill-registry skill must delegate generation to the Go generator, which
// owns the skills that are never indexed. A hand scan that lists its own skip
// rules diverges from `gentle-ai skill-registry refresh` and can index skills
// the generator deliberately excludes.
func TestSkillRegistrySkillDelegatesGenerationToTheCLI(t *testing.T) {
	data, err := Read("skills/skill-registry/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(data, "gentle-ai skill-registry refresh --cwd <project>") {
		t.Fatal("skill-registry SKILL.md must generate the registry with `gentle-ai skill-registry refresh --cwd <project>`")
	}
	if !strings.Contains(data, "Scan by hand only when that command is unavailable") {
		t.Fatal("skill-registry SKILL.md must limit the hand scan to when the CLI is unavailable")
	}
}
