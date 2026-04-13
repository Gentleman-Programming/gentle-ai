package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
)

// RenderProfileCreate renders the multi-step profile create/edit screen.
//
// step 0: name input (text field with validation feedback)
// step 1: assign models — orchestrator + sub-agents in one ModelPicker screen
// step 2: confirm screen with summary + Create/Save & Sync button
//
// editMode=true shows "Edit Profile" header and "Save & Sync" instead of "Create & Sync".
// In edit mode, step 0 shows the name as read-only.
func RenderProfileCreate(
	step int,
	draft model.Profile,
	nameInput string,
	namePos int,
	nameErr string,
	editMode bool,
	assignments map[string]model.ModelAssignment,
	picker ModelPickerState,
	cursor int,
) string {
	switch step {
	case 0:
		return renderProfileNameStep(draft, nameInput, namePos, nameErr, editMode)
	case 1:
		return renderProfileModelStep(assignments, picker, cursor, editMode, draft.Name)
	default:
		return renderProfileConfirmStep(draft, cursor, editMode)
	}
}

// renderProfileNameStep renders step 0: profile name text input.
func renderProfileNameStep(draft model.Profile, nameInput string, namePos int, nameErr string, editMode bool) string {
	var b strings.Builder

	header := "Create SDD Profile"
	if editMode {
		header = "Edit Profile"
	}
	b.WriteString(styles.TitleStyle.Render(header))
	b.WriteString("\n\n")

	if editMode && draft.Name != "" {
		// In edit mode, show the name as read-only (can't rename existing profile).
		b.WriteString(styles.SubtextStyle.Render("Profile: "))
		b.WriteString(styles.SelectedStyle.Render(draft.Name))
		b.WriteString("\n\n")
		b.WriteString(styles.SubtextStyle.Render("(Name cannot be changed when editing)"))
		b.WriteString("\n\n")
	} else {
		b.WriteString(styles.HeadingStyle.Render("Profile name:"))
		b.WriteString("\n")
		b.WriteString(styles.SubtextStyle.Render("Use lowercase alphanumeric characters and hyphens only (slug format)"))
		b.WriteString("\n\n")

		// Render text input with cursor indicator.
		runes := []rune(nameInput)
		var inputDisplay strings.Builder
		for i, r := range runes {
			if i == namePos {
				inputDisplay.WriteString(styles.SelectedStyle.Render("|"))
			}
			inputDisplay.WriteRune(r)
		}
		if namePos == len(runes) {
			inputDisplay.WriteString(styles.SelectedStyle.Render("|"))
		}
		b.WriteString(styles.UnselectedStyle.Render("  > "))
		b.WriteString(inputDisplay.String())
		b.WriteString("\n\n")

		if nameErr != "" {
			b.WriteString(styles.ErrorStyle.Render("✗ " + nameErr))
			b.WriteString("\n\n")
		}
	}

	b.WriteString(styles.HelpStyle.Render("enter: next • esc: back"))

	return styles.FrameStyle.Render(b.String())
}

// renderProfileModelStep renders step 1: assign models for orchestrator + sub-agents.
// Uses the existing ModelPicker with all rows (orchestrator, Set all, 9 phases).
func renderProfileModelStep(
	assignments map[string]model.ModelAssignment,
	picker ModelPickerState,
	cursor int,
	editMode bool,
	profileName string,
) string {
	var b strings.Builder

	header := "Create SDD Profile"
	if editMode {
		header = "Edit Profile"
	}
	b.WriteString(styles.TitleStyle.Render(header))
	b.WriteString("\n\n")
	b.WriteString(styles.HeadingStyle.Render("Assign Models"))
	b.WriteString("\n")
	b.WriteString(styles.SubtextStyle.Render("Assign models for profile: " + profileName))
	b.WriteString("\n\n")

	// Reuse the full ModelPicker (orchestrator + Set all + 9 phases).
	b.WriteString(RenderModelPicker(assignments, picker, cursor))

	return styles.FrameStyle.Render(b.String())
}

// renderProfileConfirmStep renders step 3: summary + confirm button.
func renderProfileConfirmStep(draft model.Profile, cursor int, editMode bool) string {
	var b strings.Builder

	header := "Create SDD Profile"
	if editMode {
		header = "Edit Profile"
	}
	b.WriteString(styles.TitleStyle.Render(header))
	b.WriteString("\n\n")
	b.WriteString(styles.HeadingStyle.Render("Profile Summary"))
	b.WriteString("\n\n")

	b.WriteString(styles.SubtextStyle.Render("Name: "))
	b.WriteString(styles.SelectedStyle.Render(draft.Name))
	b.WriteString("\n")

	b.WriteString(styles.SubtextStyle.Render("Orchestrator: "))
	if draft.OrchestratorModel.ProviderID != "" {
		b.WriteString(styles.UnselectedStyle.Render(draft.OrchestratorModel.ProviderID + "/" + draft.OrchestratorModel.ModelID))
	} else {
		b.WriteString(styles.UnselectedStyle.Render("(default)"))
	}
	b.WriteString("\n")

	phaseCount := len(draft.PhaseAssignments)
	if phaseCount > 0 {
		b.WriteString(styles.SubtextStyle.Render("Phase assignments: "))
		b.WriteString(styles.UnselectedStyle.Render(fmt.Sprintf("%d assigned", phaseCount)))
		b.WriteString("\n")
	}

	b.WriteString("\n")

	confirmLabel := "Create & Sync"
	if editMode {
		confirmLabel = "Save & Sync"
	}
	b.WriteString(renderOptions([]string{confirmLabel, "Cancel"}, cursor))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: confirm • esc: back"))

	return styles.FrameStyle.Render(b.String())
}

// ProfileCreateOptionCount returns the number of selectable options for a
// given step in the profile create/edit flow.
//
// step 0: 0 (text input — no cursor navigation)
// step 1: ModelPicker option count (rows + Continue + Back)
// step 2: 2 ("Create & Sync" / "Save & Sync" + "Cancel")
func ProfileCreateOptionCount(step int, picker ModelPickerState) int {
	switch step {
	case 0:
		return 0 // text input mode
	case 1:
		if len(picker.AvailableIDs) == 0 {
			return 1 // only "Back"
		}
		return len(ModelPickerRows()) + 2 // rows + Continue + Back
	default:
		return 2 // "Create & Sync" / "Save & Sync" + "Cancel"
	}
}
