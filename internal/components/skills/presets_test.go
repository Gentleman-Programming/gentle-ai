package skills

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
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
	preset := SkillsForPreset(model.PresetFullGentleman)
	all := AllSkillIDs()

	presetSet := make(map[model.SkillID]bool, len(preset))
	for _, id := range preset {
		presetSet[id] = true
	}

	var selectableOnly []model.SkillID
	for _, id := range all {
		if !presetSet[id] {
			selectableOnly = append(selectableOnly, id)
		}
	}

	contributorOnly := map[model.SkillID]bool{
		model.SkillGentleAIBench:       true,
		model.SkillBranchPR:            true,
		model.SkillIssueCreation:       true,
		model.SkillCommentWriter:       true,
		model.SkillRDDDefectWorkflow:   true,
		model.SkillSystemicIssueTriage: true,
	}
	if len(selectableOnly) != len(contributorOnly) {
		t.Fatalf("selectable inventory has %d skills outside the full preset (%v), want exactly the %d contributor skills", len(selectableOnly), selectableOnly, len(contributorOnly))
	}
	for _, id := range selectableOnly {
		if !contributorOnly[id] {
			t.Fatalf("selectable inventory contains unexpected non-preset skill %q; only contributor skills may live outside the presets", id)
		}
	}
}

func TestSkillsForPresetExcludesContributorSkills(t *testing.T) {
	excluded := []model.SkillID{
		model.SkillBranchPR,
		model.SkillIssueCreation,
		model.SkillSystemicIssueTriage,
		model.SkillRDDDefectWorkflow,
		model.SkillGentleAIBench,
		model.SkillCommentWriter,
	}
	required := []model.SkillID{
		model.SkillChainedPR,
		model.SkillSkillRegistry,
		model.SkillWorkUnitCommits,
		model.SkillCognitiveDoc,
	}
	for _, preset := range []model.PresetID{model.PresetFullGentleman, model.PresetEcosystemOnly, "", "unknown"} {
		t.Run(string(preset), func(t *testing.T) {
			present := make(map[model.SkillID]bool)
			for _, skill := range SkillsForPreset(preset) {
				present[skill] = true
			}
			for _, skill := range excluded {
				if present[skill] {
					t.Errorf("SkillsForPreset(%q) includes contributor skill %q", preset, skill)
				}
			}
			for _, skill := range required {
				if !present[skill] {
					t.Errorf("SkillsForPreset(%q) missing product skill %q", preset, skill)
				}
			}
		})
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
		model.SkillCreator,
		model.SkillSkillRegistry,
		model.SkillCognitiveDoc,
		model.SkillCommentWriter,
		model.SkillJudgmentDay,
		model.SkillImprover,
		model.SkillGoTesting,
		model.SkillSystemicIssueTriage,
		model.SkillGentleAIBench,
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

func TestRequestedBundledSkillsAreInPresetSkillSets(t *testing.T) {
	required := []model.SkillID{
		model.SkillCreator,
		model.SkillSkillRegistry,
		model.SkillCognitiveDoc,
		model.SkillJudgmentDay,
		model.SkillSDDInit,
		model.SkillImprover,
	}

	for _, preset := range []model.PresetID{model.PresetEcosystemOnly, model.PresetFullGentleman} {
		t.Run(string(preset), func(t *testing.T) {
			skillSet := make(map[model.SkillID]struct{})
			for _, skill := range SkillsForPreset(preset) {
				skillSet[skill] = struct{}{}
			}

			for _, req := range required {
				if _, ok := skillSet[req]; !ok {
					t.Fatalf("SkillsForPreset(%q) missing requested bundled skill %q", preset, req)
				}
			}
		})
	}
}
