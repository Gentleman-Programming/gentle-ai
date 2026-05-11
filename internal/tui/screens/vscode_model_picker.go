package screens

import (
	"fmt"
	"strings"

	vscodeagent "github.com/gentleman-programming/gentle-ai/internal/agents/vscode"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/opencode"
	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
)

// VSCodeModelPickerState holds navigation state for the VS Code model picker.
// The model catalog is loaded dynamically from the OpenCode models cache
// (provider "github-copilot") rather than from a hardcoded list, so the
// picker reflects whatever models GitHub Copilot currently exposes to the user.
type VSCodeModelPickerState struct {
	// Mode mirrors ModelPickerMode: ModePhaseList shows phase rows,
	// ModeModelSelect shows the flat list of Copilot models for a chosen phase.
	Mode ModelPickerMode

	// Models is the list of github-copilot models loaded from the OpenCode
	// cache (filtered to tool-call-capable models, sorted by Name).
	Models []opencode.Model

	// ConfigWarning is non-empty when the OpenCode cache could not be loaded
	// or when it does not contain a github-copilot provider entry. The picker
	// is still shown but with a banner explaining the situation.
	ConfigWarning string

	SelectedPhaseIdx int
	ModelCursor      int
	ModelScroll      int

	// AllPhasesModel tracks the last "Set all phases" assignment (display name).
	AllPhasesModel string
}

// NewVSCodeModelPickerState loads the github-copilot model catalog from the
// OpenCode models cache and returns a picker state ready for use. When the
// cache is missing or the provider entry is absent, ConfigWarning is populated
// and Models is empty — the UI surfaces this to the user.
func NewVSCodeModelPickerState(cachePath string) VSCodeModelPickerState {
	providers, err := opencode.LoadModels(cachePath)
	if err != nil {
		return VSCodeModelPickerState{
			Mode:          ModePhaseList,
			ConfigWarning: fmt.Sprintf("Could not load models cache %q: %v. Run `opencode sync` to populate it.", cachePath, err),
		}
	}
	copilot, ok := providers["github-copilot"]
	if !ok {
		return VSCodeModelPickerState{
			Mode:          ModePhaseList,
			ConfigWarning: "github-copilot provider not found in OpenCode models cache. Run `opencode sync` to fetch the Copilot model catalog.",
		}
	}
	return VSCodeModelPickerState{
		Mode:   ModePhaseList,
		Models: opencode.FilterModelsForSDD(copilot),
	}
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

// vscodeModelLabel returns the display label for a single opencode.Model entry.
// Prefers Name; falls back to ID when Name is empty.
func vscodeModelLabel(m opencode.Model) string {
	if m.Name != "" {
		return m.Name
	}
	return m.ID
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

	if state.ConfigWarning != "" {
		b.WriteString(styles.WarningStyle.Render(state.ConfigWarning))
		b.WriteString("\n\n")
	}

	rows := VSCodeModelRows()
	phases := vscodeagent.SDDPhases()

	for idx, row := range rows {
		focused := idx == cursor

		var label string
		if idx == 0 {
			if state.AllPhasesModel != "" {
				label = fmt.Sprintf("%-22s (%s)", row, state.AllPhasesModel)
			} else {
				label = fmt.Sprintf("%-22s (not set)", row)
			}
		} else {
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

	if len(state.Models) == 0 {
		if state.ConfigWarning != "" {
			b.WriteString(styles.WarningStyle.Render(state.ConfigWarning))
		} else {
			b.WriteString(styles.SubtextStyle.Render("No Copilot models available."))
		}
		b.WriteString("\n\n")
		b.WriteString(styles.HelpStyle.Render("esc: back"))
		return b.String()
	}

	end := state.ModelScroll + maxVisibleItems
	if end > len(state.Models) {
		end = len(state.Models)
	}

	if state.ModelScroll > 0 {
		b.WriteString(styles.SubtextStyle.Render("  ↑ more"))
		b.WriteString("\n")
	}

	for i := state.ModelScroll; i < end; i++ {
		label := vscodeModelLabel(state.Models[i])
		focused := i == state.ModelCursor
		if focused {
			b.WriteString(styles.SelectedStyle.Render(styles.Cursor+label) + "\n")
		} else {
			b.WriteString(styles.UnselectedStyle.Render("  "+label) + "\n")
		}
	}

	if end < len(state.Models) {
		b.WriteString(styles.SubtextStyle.Render("  ↓ more"))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select • esc: back"))

	return b.String()
}

// HandleVSCodeModelPickerNav handles key navigation for the VS Code model picker.
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
		if state.ModelCursor < len(state.Models)-1 {
			state.ModelCursor++
			if state.ModelCursor >= state.ModelScroll+maxVisibleItems {
				state.ModelScroll = state.ModelCursor - maxVisibleItems + 1
			}
		}
		return true, assignments
	case "enter":
		if len(state.Models) == 0 {
			return true, assignments
		}
		entry := state.Models[state.ModelCursor]
		assignment := model.ModelAssignment{
			ProviderID: "github-copilot",
			ModelID:    entry.ID,
		}
		label := vscodeModelLabel(entry)
		if state.SelectedPhaseIdx == 0 {
			for _, phase := range phases {
				assignments[phase] = assignment
			}
			state.AllPhasesModel = label
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
// Step 1: VS Code model picker (Copilot-only catalog from cache)
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
		return 0
	case 1:
		return VSCodeModelPickerOptionCount()
	default:
		return 2
	}
}
