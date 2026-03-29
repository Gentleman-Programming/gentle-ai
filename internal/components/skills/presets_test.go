package skills

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestSkillsForPresetMinimalReturnsSDDOnly(t *testing.T) {
	skills := SkillsForPreset(model.PresetMinimal)
	if len(skills) == 0 {
		t.Fatalf("SkillsForPreset(minimal) returned empty")
	}

	// Orchestration skills that are always bundled with SDD.
	orchestrationSkills := map[model.SkillID]bool{
		model.SkillJudgmentDay: true,
	}

	for _, skill := range skills {
		isSDD := len(skill) >= 4 && skill[:3] == "sdd"
		if !isSDD && !orchestrationSkills[skill] {
			t.Fatalf("minimal preset should only contain SDD/orchestration skills, got %q", skill)
		}
	}
}

func TestSkillsForPresetEcosystemIncludesFrameworks(t *testing.T) {
	skills := SkillsForPreset(model.PresetEcosystemOnly)

	hasGoTesting := false
	hasSkillCreator := false
	hasSDDInit := false
	for _, skill := range skills {
		if skill == model.SkillGoTesting {
			hasGoTesting = true
		}
		if skill == model.SkillCreator {
			hasSkillCreator = true
		}
		if skill == model.SkillSDDInit {
			hasSDDInit = true
		}
	}

	if !hasGoTesting {
		t.Fatalf("ecosystem preset should include go-testing")
	}
	if !hasSDDInit {
		t.Fatalf("ecosystem preset should include sdd-init")
	}
	if !hasSkillCreator {
		t.Fatalf("ecosystem preset should include skill-creator")
	}
}

func TestSkillsForPresetFullIncludesAll(t *testing.T) {
	skills := SkillsForPreset(model.PresetFullGentleman)
	all := AllSkillIDs()

	if len(skills) != len(all) {
		t.Fatalf("full preset skills len = %d, all skills len = %d", len(skills), len(all))
	}
}

func TestSkillsForPresetCustomReturnsNil(t *testing.T) {
	skills := SkillsForPreset(model.PresetCustom)
	if skills != nil {
		t.Fatalf("custom preset should return nil, got %v", skills)
	}
}

func TestAllSkillIDsIncludesEveryKnownSkill(t *testing.T) {
	all := AllSkillIDs()

	required := []model.SkillID{
		model.SkillSDDInit,
		model.SkillGoTesting,
		model.SkillCreator,
		model.SkillJudgmentDay,
	}

	skillSet := make(map[model.SkillID]struct{}, len(all))
	for _, skill := range all {
		skillSet[skill] = struct{}{}
	}

	for _, req := range required {
		if _, ok := skillSet[req]; !ok {
			t.Fatalf("AllSkillIDs() missing %q", req)
		}
	}
}
