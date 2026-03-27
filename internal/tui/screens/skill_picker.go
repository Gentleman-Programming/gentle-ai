package screens

import (
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/components/skills"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
)

// sddSkillIDs are the SDD orchestrator skills shown in the first group.
var sddSkillIDs = []model.SkillID{
	model.SkillSDDInit,
	model.SkillSDDExplore,
	model.SkillSDDPropose,
	model.SkillSDDSpec,
	model.SkillSDDDesign,
	model.SkillSDDTasks,
	model.SkillSDDApply,
	model.SkillSDDVerify,
	model.SkillSDDArchive,
	model.SkillJudgmentDay,
}

// foundationSkillIDs are the baseline/learning skills shown in the second group.
var foundationSkillIDs = []model.SkillID{
	model.SkillGoTesting,
	model.SkillCreator,
	model.SkillBranchPR,
	model.SkillIssueCreation,
}

// skillLabels maps each SkillID to a human-readable display label.
var skillLabels = map[model.SkillID]string{
	model.SkillSDDInit:       "SDD Init",
	model.SkillSDDExplore:    "SDD Explore",
	model.SkillSDDPropose:    "SDD Propose",
	model.SkillSDDSpec:       "SDD Spec",
	model.SkillSDDDesign:     "SDD Design",
	model.SkillSDDTasks:      "SDD Tasks",
	model.SkillSDDApply:      "SDD Apply",
	model.SkillSDDVerify:     "SDD Verify",
	model.SkillSDDArchive:    "SDD Archive",
	model.SkillJudgmentDay:   "Judgment Day",
	model.SkillGoTesting:     "Go Testing",
	model.SkillCreator:       "Skill Creator",
	model.SkillBranchPR:      "Branch & PR",
	model.SkillIssueCreation: "Issue Creation",
}

// SkillPickerOptions returns the action buttons shown after the skill checkboxes.
func SkillPickerOptions() []string {
	return []string{"Continue", "Back"}
}

// AllSkillsOrdered returns all skills in display order: SDD group first, then Foundation.
func AllSkillsOrdered() []model.SkillID {
	return skills.AllSkillIDs()
}

// SkillPickerOptionCount returns the total number of navigable rows on the skill picker screen.
func SkillPickerOptionCount() int {
	return len(AllSkillsOrdered()) + len(SkillPickerOptions())
}

// RenderSkillPicker renders the skill selection screen for custom preset mode.
func RenderSkillPicker(selectedSkills []model.SkillID, cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Select Skills"))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Toggle skills with enter or space. All are pre-selected by default."))
	b.WriteString("\n\n")

	selectedSet := make(map[model.SkillID]struct{}, len(selectedSkills))
	for _, s := range selectedSkills {
		selectedSet[s] = struct{}{}
	}

	allSkills := AllSkillsOrdered()

	// ── SDD Skills group ──────────────────────────────────────────────────────
	b.WriteString(styles.HeadingStyle.Render("SDD Skills"))
	b.WriteString("\n")

	for idx, skillID := range sddSkillIDs {
		_, checked := selectedSet[skillID]
		focused := idx == cursor
		label := skillLabelFor(skillID)
		b.WriteString(renderCheckbox(label, checked, focused))
	}

	b.WriteString("\n")

	// ── Foundation Skills group ───────────────────────────────────────────────
	b.WriteString(styles.HeadingStyle.Render("Foundation Skills"))
	b.WriteString("\n")

	for i, skillID := range foundationSkillIDs {
		idx := len(sddSkillIDs) + i
		_, checked := selectedSet[skillID]
		focused := idx == cursor
		label := skillLabelFor(skillID)
		b.WriteString(renderCheckbox(label, checked, focused))
	}

	b.WriteString("\n")

	// ── Action buttons ────────────────────────────────────────────────────────
	actionOffset := cursor - len(allSkills)
	b.WriteString(renderOptions(SkillPickerOptions(), actionOffset))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • space/enter: toggle • esc: back"))

	return b.String()
}

// skillLabelFor returns the human-readable label for a skill ID.
func skillLabelFor(id model.SkillID) string {
	if label, ok := skillLabels[id]; ok {
		return label
	}
	return string(id)
}
