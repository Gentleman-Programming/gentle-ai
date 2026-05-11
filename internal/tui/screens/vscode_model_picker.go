package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	vscodeagent "github.com/gentleman-programming/gentle-ai/internal/agents/vscode"
	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
)

// VSCodeModelPickerState holds navigation state for the VS Code static model picker.
// It is embedded in the profile-create flow when ActiveProfileAdapter == VS Code.
type VSCodeModelPickerState struct {
	// Mode mirrors ModelPickerMode: ModePhaseList shows phase rows,
	// ModeModelSelect shows the flat VS Code model list for a chosen phase.
	Mode ModelPickerMode

	SelectedPhaseIdx int // which phase row was selected
	ModelCursor      int
	ModelScroll      int

	// AllPhasesModel tracks the last "Set all phases" assignment (VS Code display name).
	AllPhasesModel string // display name of the model, e.g. "Claude Sonnet 4 (copilot)"
}

// VSCodeModelRows returns the row labels for the VS Code model picker phase list.
// VS Code profiles have no orchestrator row (phases only).
// Row 0 is "Set all phases", rows 1-10 are the 10 SDD phases.
func VSCodeModelRows() []string {
	rows := make([]string, 0, 11)
	rows = append(rows, "Set all phases")
	rows = append(rows, vscodeagent.SDDPhases()...)
	return rows
}

// VSCodeStaticModelNames returns the display names of all VS Code Copilot models
// in the canonical order from vscModelEntries.
func VSCodeStaticModelNames() []string {
	entries := vscodeagent.VSCodeStaticModels()
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.DisplayName
	}
	return names
}

// RenderVSCodeModelPicker renders the VS Code model picker for profile create step 1.
// It shows a phase list in ModePhaseList, or a flat model list in ModeModelSelect.
func RenderVSCodeModelPicker(
	assignments map[string]model.ModelAssignment,
	state VSCodeModelPickerState,
	cursor int,
	editMode bool,
	profileName string,
) string {
	switch state.Mode {
	case ModeModelSelect:
		return renderVSCodeModelSelect(state)
	default:
		return renderVSCodePhaseList(assignments, state, cursor, editMode, profileName)
	}
}

func renderVSCodePhaseList(
	assignments map[string]model.ModelAssignment,
	state VSCodeModelPickerState,
	cursor int,
	editMode bool,
	profileName string,
) string {
	var b strings.Builder

	header := "Create VS Code SDD Profile"
	if editMode {
		header = "Edit VS Code SDD Profile"
	}
	b.WriteString(styles.TitleStyle.Render(header))
	b.WriteString("\n\n")
	b.WriteString(styles.HeadingStyle.Render("Assign Models"))
	b.WriteString("\n")
	b.WriteString(styles.SubtextStyle.Render("Assign Copilot models for profile: " + profileName))
	b.WriteString("\n\n")

	rows := VSCodeModelRows()
	phases := vscodeagent.SDDPhases()

	for idx, row := range rows {
		focused := idx == cursor

		var label string
		if idx == 0 {
			// "Set all phases" row
			if state.AllPhasesModel != "" {
				label = fmt.Sprintf("%-22s (%s)", row, state.AllPhasesModel)
			} else {
				label = fmt.Sprintf("%-22s (not set)", row)
			}
		} else {
			// Phase row — idx 1 maps to phases[0]
			phaseIdx := idx - 1
			if phaseIdx < len(phases) {
				phase := phases[phaseIdx]
				if assignment, ok := assignments[phase]; ok && assignment.ModelID != "" {
					label = fmt.Sprintf("%-22s %s", row, assignment.ModelID)
				} else {
					label = fmt.Sprintf("%-22s (default)", row)
				}
			}
		}

		if focused {
			b.WriteString(styles.SelectedStyle.Render(styles.Cursor+label) + "\n")
		} else {
			b.WriteString(styles.UnselectedStyle.Render("  "+label) + "\n")
		}
	}

	b.WriteString("\n")
	actionIdx := cursor - len(rows)
	b.WriteString(renderOptions([]string{"Continue", "← Back"}, actionIdx))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: change model / confirm • esc: back"))

	return styles.FrameStyle.Render(b.String())
}

func renderVSCodeModelSelect(state VSCodeModelPickerState) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Select Copilot model:"))
	b.WriteString("\n\n")

	models := VSCodeStaticModelNames()

	end := state.ModelScroll + maxVisibleItems
	if end > len(models) {
		end = len(models)
	}

	if state.ModelScroll > 0 {
		b.WriteString(styles.SubtextStyle.Render("  ↑ more"))
		b.WriteString("\n")
	}

	for i := state.ModelScroll; i < end; i++ {
		label := models[i]
		focused := i == state.ModelCursor
		if focused {
			b.WriteString(styles.SelectedStyle.Render(styles.Cursor+label) + "\n")
		} else {
			b.WriteString(styles.UnselectedStyle.Render("  "+label) + "\n")
		}
	}

	if end < len(models) {
		b.WriteString(styles.SubtextStyle.Render("  ↓ more"))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select • esc: back"))

	return b.String()
}

// HandleVSCodeModelPickerNav handles key navigation for the VS Code static model picker.
// Returns true when handled (caller should skip default nav).
func HandleVSCodeModelPickerNav(
	key string,
	state *VSCodeModelPickerState,
	assignments map[string]model.ModelAssignment,
) (handled bool, updated map[string]model.ModelAssignment) {
	if assignments == nil {
		assignments = make(map[string]model.ModelAssignment)
	}

	if state.Mode != ModeModelSelect {
		return false, assignments
	}

	models := VSCodeStaticModelNames()
	entries := vscodeagent.VSCodeStaticModels()
	phases := vscodeagent.SDDPhases()

	switch key {
	case "up", "k":
		if state.ModelCursor > 0 {
			state.ModelCursor--
			if state.ModelCursor < state.ModelScroll {
				state.ModelScroll = state.ModelCursor
			}
		}
		return true, assignments
	case "down", "j":
		if state.ModelCursor < len(models)-1 {
			state.ModelCursor++
			if state.ModelCursor >= state.ModelScroll+maxVisibleItems {
				state.ModelScroll = state.ModelCursor - maxVisibleItems + 1
			}
		}
		return true, assignments
	case "enter":
		entry := entries[state.ModelCursor]
		assignment := model.ModelAssignment{
			ProviderID: "copilot",
			ModelID:    entry.DisplayName,
		}
		if state.SelectedPhaseIdx == 0 {
			// "Set all phases"
			for _, phase := range phases {
				assignments[phase] = assignment
			}
			state.AllPhasesModel = entry.DisplayName
		} else {
			phaseIdx := state.SelectedPhaseIdx - 1
			if phaseIdx < len(phases) {
				assignments[phases[phaseIdx]] = assignment
			}
		}
		state.Mode = ModePhaseList
		state.ModelCursor = 0
		state.ModelScroll = 0
		return true, assignments
	case "esc":
		state.Mode = ModePhaseList
		state.ModelCursor = 0
		state.ModelScroll = 0
		return true, assignments
	}

	return false, assignments
}

// VSCodeModelPickerOptionCount returns the option count for the VS Code phase list.
// Rows + Continue + Back.
func VSCodeModelPickerOptionCount() int {
	return len(VSCodeModelRows()) + 2
}

// RenderVSCodeProfileCreate renders the multi-step profile create/edit screen for VS Code.
// Step 0: name input (identical to OpenCode)
// Step 1: VS Code model picker (static, no provider hierarchy)
// Step 2: confirm
func RenderVSCodeProfileCreate(
	step int,
	draft model.Profile,
	nameInput string,
	namePos int,
	nameErr string,
	editMode bool,
	assignments map[string]model.ModelAssignment,
	picker VSCodeModelPickerState,
	cursor int,
) string {
	switch step {
	case 0:
		// Reuse the OpenCode name step renderer — it's adapter-agnostic.
		return RenderProfileCreate(step, draft, nameInput, namePos, nameErr, editMode, nil, ModelPickerState{}, cursor)
	case 1:
		return RenderVSCodeModelPicker(assignments, picker, cursor, editMode, draft.Name)
	default:
		return renderVSCodeProfileConfirmStep(draft, cursor, editMode)
	}
}

// renderVSCodeProfileConfirmStep renders the VS Code confirm step.
func renderVSCodeProfileConfirmStep(draft model.Profile, cursor int, editMode bool) string {
	var b strings.Builder

	header := "Create VS Code SDD Profile"
	if editMode {
		header = "Edit VS Code SDD Profile"
	}
	b.WriteString(styles.TitleStyle.Render(header))
	b.WriteString("\n\n")
	b.WriteString(styles.HeadingStyle.Render("Profile Summary"))
	b.WriteString("\n\n")

	b.WriteString(styles.SubtextStyle.Render("Name: "))
	b.WriteString(styles.SelectedStyle.Render(draft.Name))
	b.WriteString("\n")

	phaseCount := len(draft.PhaseAssignments)
	if phaseCount > 0 {
		b.WriteString(styles.SubtextStyle.Render("Phase assignments: "))
		b.WriteString(styles.UnselectedStyle.Render(fmt.Sprintf("%d assigned", phaseCount)))
		b.WriteString("\n")
	} else {
		b.WriteString(styles.SubtextStyle.Render("Phase assignments: "))
		b.WriteString(styles.UnselectedStyle.Render("(default Copilot model)"))
		b.WriteString("\n")
	}

	b.WriteString(styles.SubtextStyle.Render("Files to write: "))
	b.WriteString(styles.UnselectedStyle.Render("10 .agent.md files in ~/.copilot/agents/"))
	b.WriteString("\n\n")

	confirmLabel := "Create"
	if editMode {
		confirmLabel = "Save"
	}
	b.WriteString(renderOptions([]string{confirmLabel, "Cancel"}, cursor))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: confirm • esc: back"))

	return styles.FrameStyle.Render(b.String())
}

// VSCodeProfileCreateOptionCount returns the number of selectable options for a given step.
func VSCodeProfileCreateOptionCount(step int) int {
	switch step {
	case 0:
		return 0 // text input
	case 1:
		return VSCodeModelPickerOptionCount()
	default:
		return 2 // Create/Save + Cancel
	}
}
