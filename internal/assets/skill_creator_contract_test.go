package assets

import (
	"strings"
	"testing"
)

func TestSkillCreatorPostCreationChecklistContract(t *testing.T) {
	data, err := Read("skills/skill-creator/SKILL.md")
	if err != nil {
		t.Fatalf("Read(skill-creator) error = %v", err)
	}

	checklistSection := "## Post-creation checklist (mandatory)"
	if !strings.Contains(data, checklistSection) {
		t.Fatalf("skill-creator SKILL.md missing mandatory section %q", checklistSection)
	}

	requiredSteps := []struct {
		name    string
		markers []string
	}{
		{
			name: "frontmatter validation",
			markers: []string{
				"Validate frontmatter",
				"`name` matches the folder",
				"lowercase-hyphenated",
				"<=64 chars",
				"single-line quoted `description`",
				"trigger keywords",
			},
		},
		{
			name: "registry refresh",
			markers: []string{
				"gentle-ai skill-registry refresh --cwd <project>",
				"confirm the new row exists in `.atl/skill-registry.md`",
			},
		},
		{
			name: "memory pointer",
			markers: []string{
				"Persist a discovery pointer",
				"mem_save",
				"`topic_key: skills/<name>`",
				"name, path, trigger, one-line purpose",
			},
		},
		{
			name: "announcement",
			markers: []string{
				"Announce to the user",
				"skill name, trigger sentence, registry row",
			},
		},
	}

	for _, step := range requiredSteps {
		t.Run(step.name, func(t *testing.T) {
			for _, marker := range step.markers {
				if !strings.Contains(data, marker) {
					t.Errorf("skill-creator post-creation checklist missing %s marker %q", step.name, marker)
				}
			}
		})
	}
}

func TestSkillCreatorExecutionStepsAndOutputContractIncludePostCreation(t *testing.T) {
	data, err := Read("skills/skill-creator/SKILL.md")
	if err != nil {
		t.Fatalf("Read(skill-creator) error = %v", err)
	}

	executionStart := strings.Index(data, "## Execution Steps\n")
	if executionStart == -1 {
		t.Fatal("missing Execution Steps section")
	}

	for _, want := range []string{
		"Register the skill in `AGENTS.md` when it is a project skill.",
		"Execute the mandatory post-creation checklist for all skills (project and global).",
		"Post-creation announcement containing skill name, trigger sentence, and registry row.",
	} {
		if !strings.Contains(data, want) {
			t.Errorf("skill-creator missing workflow contract %q", want)
		}
	}
}

func TestSkillCreatorAndSkillImproverPrescribeRegistryRefresh(t *testing.T) {
	creator, err := Read("skills/skill-creator/SKILL.md")
	if err != nil {
		t.Fatalf("Read(skill-creator) error = %v", err)
	}
	improver, err := Read("skills/skill-improver/SKILL.md")
	if err != nil {
		t.Fatalf("Read(skill-improver) error = %v", err)
	}

	const refreshCommand = "gentle-ai skill-registry refresh"
	if !strings.Contains(creator, refreshCommand) {
		t.Errorf("skill-creator does not prescribe registry refresh %q", refreshCommand)
	}
	if !strings.Contains(improver, refreshCommand) {
		t.Errorf("skill-improver does not prescribe registry refresh %q", refreshCommand)
	}
}
