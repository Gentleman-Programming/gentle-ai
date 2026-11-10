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

// A curated registry is loaded verbatim, never regenerated, and still mirrored
// to Engram so agents without startup skill loading can recall it (#516).
func TestSkillRegistrySkillLoadsCuratedRegistryAndSavesItToEngram(t *testing.T) {
	data, err := Read("skills/skill-registry/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"## Load a Curated Registry",
		"gentle-ai skill-registry load <path>",
		"Do not regenerate",
		"save the loaded content with `title: skill-registry`, `topic_key: skill-registry`, `type: config`, and `capture_prompt: false`",
	} {
		if !strings.Contains(data, want) {
			t.Fatalf("skill-registry SKILL.md must describe the curated load flow; missing %q", want)
		}
	}
}
