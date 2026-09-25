package skills

import "github.com/gentleman-programming/gentle-ai/v3/internal/model"

// Retained orchestration skill installed by every non-custom preset.
var orchestrationSkills = []model.SkillID{model.SkillJudgmentDay}

// contributorSkills are this repository's own workflow skills. They stay
// selectable through the TUI skill picker and explicit `--skills` resolution,
// but no default preset installs them.
var contributorSkills = []model.SkillID{
	model.SkillGentleAIBench,
	model.SkillBranchPR,
	model.SkillIssueCreation,
	model.SkillCommentWriter,
	model.SkillRDDDefectWorkflow,
	model.SkillSystemicIssueTriage,
}

// selectableFoundationSkills is the canonical display order of the retained
// general skills. Contributor skills keep their historical positions.
var selectableFoundationSkills = []model.SkillID{
	model.SkillGoTesting,
	model.SkillGentleAIBench,
	model.SkillCreator,
	model.SkillImprover,
	model.SkillBranchPR,
	model.SkillIssueCreation,
	model.SkillSkillRegistry,
	model.SkillChainedPR,
	model.SkillCognitiveDoc,
	model.SkillCommentWriter,
	model.SkillWorkUnitCommits,
	model.SkillRDDDefectWorkflow,
	model.SkillSystemicIssueTriage,
}

var foundationSkills = excludeSkills(selectableFoundationSkills, contributorSkills)

func excludeSkills(src, exclude []model.SkillID) []model.SkillID {
	excluded := make(map[model.SkillID]struct{}, len(exclude))
	for _, id := range exclude {
		excluded[id] = struct{}{}
	}
	out := make([]model.SkillID, 0, len(src))
	for _, id := range src {
		if _, ok := excluded[id]; !ok {
			out = append(out, id)
		}
	}
	return out
}

// SkillsForPreset returns retained skills for a preset. Custom has no defaults;
// unknown presets use the full product inventory.
func SkillsForPreset(preset model.PresetID) []model.SkillID {
	switch preset {
	case model.PresetMinimal:
		return copySkills(orchestrationSkills)
	case model.PresetCustom:
		return nil
	default:
		all := copySkills(orchestrationSkills)
		return append(all, foundationSkills...)
	}
}

// AllSkillIDs returns every selectable retained skill, including contributors.
func AllSkillIDs() []model.SkillID {
	all := copySkills(orchestrationSkills)
	return append(all, selectableFoundationSkills...)
}

func copySkills(src []model.SkillID) []model.SkillID {
	dst := make([]model.SkillID, len(src))
	copy(dst, src)
	return dst
}
