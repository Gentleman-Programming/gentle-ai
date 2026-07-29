package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/skills"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/styles"
)

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
	model.SkillSDDOnboard:    "SDD Onboard",
	model.SkillJudgmentDay:   "Judgment Day",
	model.SkillGoTesting:     "Go Testing",
	model.SkillCreator:       "Skill Creator",
	model.SkillBranchPR:      "Branch & PR",
	model.SkillIssueCreation: "Issue Creation",
}

var additionalSkillLabels = map[model.SkillID]string{
	model.SkillImprover:        "Skill Improver",
	model.SkillSkillRegistry:   "Skill Registry",
	model.SkillChainedPR:       "Chained PR",
	model.SkillCognitiveDoc:    "Cognitive Doc Design",
	model.SkillCommentWriter:   "Comment Writer",
	model.SkillWorkUnitCommits: "Work Unit Commits",
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
func RenderSkillPicker(selectedSkills []model.SkillID, cursor, height int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Select Skills"))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Toggle skills with enter or space. All are pre-selected by default."))
	b.WriteString("\n\n")

	selectedSet := make(map[model.SkillID]struct{}, len(selectedSkills))
	for _, s := range selectedSkills {
		selectedSet[s] = struct{}{}
	}

	allSkills, actions := AllSkillsOrdered(), SkillPickerOptions()
	total, sddCount := len(allSkills)+len(actions), len(skills.SkillsForPreset(model.PresetMinimal))
	start, end := skillPickerWindow(cursor, height, total)
	for idx := start; idx < end; idx++ {
		if idx < len(allSkills) {
			if idx == start || idx == sddCount {
				heading := "SDD Skills"
				if idx >= sddCount {
					b.WriteString("\n")
					heading = "Foundation Skills"
				}
				b.WriteString(styles.HeadingStyle.Render(heading) + "\n")
			}
			_, checked := selectedSet[allSkills[idx]]
			b.WriteString(renderCheckbox(skillLabelFor(allSkills[idx]), checked, idx == cursor))
		} else {
			focus := -1
			if idx == len(allSkills) {
				b.WriteString("\n")
			}
			if idx == cursor {
				focus = 0
			}
			b.WriteString(renderOptions([]string{actions[idx-len(allSkills)]}, focus))
		}
	}
	if start > 0 || end < total {
		b.WriteString(styles.SubtextStyle.Render(fmt.Sprintf("Rows %d-%d of %d", start+1, end, total)) + "\n")
	}
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • space/enter: toggle • esc: back"))

	return b.String()
}

func skillPickerWindow(cursor, height, total int) (int, int) {
	if height <= 0 || total <= height-8 {
		return 0, total
	}
	visible := max(1, height-8)
	start := max(0, min(cursor-visible/2, total-visible))
	return start, start + visible
}

// skillLabelFor returns the human-readable label for a skill ID.
func skillLabelFor(id model.SkillID) string {
	if label, ok := skillLabels[id]; ok {
		return label
	}
	if label, ok := additionalSkillLabels[id]; ok {
		return label
	}
	return string(id)
}
