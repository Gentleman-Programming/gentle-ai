package screens

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/opencode"
	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
)

// ModelPickerMode represents the current sub-mode of the model picker screen.
type ModelPickerMode int

const (
	ModePhaseList      ModelPickerMode = iota // Main screen: phase list + Continue/Back
	ModeProviderSelect                        // Sub-mode: pick a provider
	ModeModelSelect                           // Sub-mode: pick a model from chosen provider
)

// maxVisibleItems is the maximum number of items shown in scrollable sub-lists.
const maxVisibleItems = 10

// ProviderEntry holds a provider ID, display name, and model count for the provider list.
type ProviderEntry struct {
	ID         string
	Name       string
	ModelCount int
}

// ModelPickerState holds the available providers and models for the picker screen,
// plus navigation state for the two-step sub-selection modes.
type ModelPickerState struct {
	Providers    map[string]opencode.Provider
	AvailableIDs []string                    // provider IDs with tool_call-capable models
	SDDModels    map[string][]opencode.Model // provider ID -> SDD-capable models

	Mode             ModelPickerMode
	SelectedPhaseIdx int    // which phase row was selected (0 = "Set all")
	SelectedProvider string // provider ID chosen in ModeProviderSelect

	ProviderCursor int
	ProviderScroll int
	ModelCursor    int
	ModelScroll    int

	// AllPhasesModel tracks the assignment last set via the "Set all phases" row.
	// It is only updated when the user selects row idx 1 ("Set all phases"), NOT
	// when individual sub-agent phases are selected. This prevents the "Set all phases"
	// label from changing when the user picks a model for a single phase.
	// Issue #146.
	AllPhasesModel model.ModelAssignment
}

// NewModelPickerState initializes the picker state from the models cache.
func NewModelPickerState(cachePath string) ModelPickerState {
	providers, err := opencode.LoadModels(cachePath)
	if err != nil {
		return ModelPickerState{}
	}

	available := opencode.DetectAvailableProviders(providers)

	sddModels := make(map[string][]opencode.Model, len(available))
	for _, id := range available {
		sddModels[id] = opencode.FilterModelsForSDD(providers[id])
	}

	return ModelPickerState{
		Providers:    providers,
		AvailableIDs: available,
		SDDModels:    sddModels,
		Mode:         ModePhaseList,
	}
}

// SDDOrchestratorPhase is the key used for the base OpenCode SDD coordinator model assignment.
const SDDOrchestratorPhase = "gentle-orchestrator"

// ModelPickerRows returns the row labels for the model picker screen.
// Row 0 is "gentle-orchestrator" (coordinator), row 1 is "Set all phases",
// rows 2-10 are the 9 SDD sub-agent phases.
func ModelPickerRows() []string {
	rows := make([]string, 0, 11)
	rows = append(rows, SDDOrchestratorPhase)
	rows = append(rows, "Set all phases")
	rows = append(rows, opencode.SDDPhases()...)
	return rows
}

// ProviderEntries returns sorted provider entries with display names and model counts.
func ProviderEntries(state ModelPickerState) []ProviderEntry {
	entries := make([]ProviderEntry, 0, len(state.AvailableIDs))
	for _, id := range state.AvailableIDs {
		name := id
		if p, ok := state.Providers[id]; ok && p.Name != "" {
			name = p.Name
		}
		count := len(state.SDDModels[id])
		entries = append(entries, ProviderEntry{ID: id, Name: name, ModelCount: count})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})
	return entries
}

// HandleModelPickerNav handles j/k/enter/esc navigation within the sub-modes.
// Returns true if the key was handled (so the caller should NOT do default nav).
// When a model is selected, it applies the assignment to the given map and returns it.
func HandleModelPickerNav(
	key string,
	state *ModelPickerState,
	assignments map[string]model.ModelAssignment,
) (handled bool, updatedAssignments map[string]model.ModelAssignment) {
	if assignments == nil {
		assignments = make(map[string]model.ModelAssignment)
	}

	switch state.Mode {
	case ModeProviderSelect:
		return handleProviderNav(key, state), assignments
	case ModeModelSelect:
		return handleModelNav(key, state, assignments)
	}
	return false, assignments
}

func handleProviderNav(key string, state *ModelPickerState) bool {
	entries := ProviderEntries(*state)
	if len(entries) == 0 {
		return false
	}

	switch key {
	case "up", "k":
		if state.ProviderCursor > 0 {
			state.ProviderCursor--
			if state.ProviderCursor < state.ProviderScroll {
				state.ProviderScroll = state.ProviderCursor
			}
		}
		return true
	case "down", "j":
		if state.ProviderCursor < len(entries)-1 {
			state.ProviderCursor++
			if state.ProviderCursor >= state.ProviderScroll+maxVisibleItems {
				state.ProviderScroll = state.ProviderCursor - maxVisibleItems + 1
			}
		}
		return true
	case "enter":
		state.SelectedProvider = entries[state.ProviderCursor].ID
		state.Mode = ModeModelSelect
		state.ModelCursor = 0
		state.ModelScroll = 0
		return true
	case "esc":
		state.Mode = ModePhaseList
		state.ProviderCursor = 0
		state.ProviderScroll = 0
		return true
	}
	return false
}

func handleModelNav(
	key string,
	state *ModelPickerState,
	assignments map[string]model.ModelAssignment,
) (bool, map[string]model.ModelAssignment) {
	models := state.SDDModels[state.SelectedProvider]
	if len(models) == 0 {
		return false, assignments
	}

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
		selected := models[state.ModelCursor]
		assignment := model.ModelAssignment{
			ProviderID: state.SelectedProvider,
			ModelID:    selected.ID,
		}

		phases := opencode.SDDPhases()
		switch {
		case state.SelectedPhaseIdx == 0:
			// "gentle-orchestrator" row — assign only to the base orchestrator key
			assignments[SDDOrchestratorPhase] = assignment
		case state.SelectedPhaseIdx == 1:
			// "Set all phases" — sets only the 9 sub-agents, NOT the orchestrator.
			// Also update AllPhasesModel so the label stays in sync with the last
			// "Set all" action (Issue #146: individual phase selections must NOT touch this).
			for _, phase := range phases {
				assignments[phase] = assignment
			}
			state.AllPhasesModel = assignment
		default:
			// Sub-agent rows start at idx 2; phases[idx-2] is the correct phase.
			// Individual selection intentionally does NOT update AllPhasesModel (Issue #146).
			phaseIdx := state.SelectedPhaseIdx - 2
			if phaseIdx < len(phases) {
				assignments[phases[phaseIdx]] = assignment
			}
		}

		// Return to phase list
		state.Mode = ModePhaseList
		state.ModelCursor = 0
		state.ModelScroll = 0
		state.ProviderCursor = 0
		state.ProviderScroll = 0
		return true, assignments
	case "esc":
		state.Mode = ModeProviderSelect
		state.ModelCursor = 0
		state.ModelScroll = 0
		return true, assignments
	}
	return false, assignments
}

// RenderModelPicker renders the model picker screen based on the current mode.
func RenderModelPicker(
	assignments map[string]model.ModelAssignment,
	state ModelPickerState,
	cursor int,
) string {
	switch state.Mode {
	case ModeProviderSelect:
		return renderProviderSelect(state)
	case ModeModelSelect:
		return renderModelSelect(state)
	default:
		return renderPhaseList(assignments, state, cursor)
	}
}

func renderPhaseList(
	assignments map[string]model.ModelAssignment,
	state ModelPickerState,
	cursor int,
) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Assign Models to SDD Phases"))
	b.WriteString("\n\n")

	if len(state.AvailableIDs) == 0 {
		b.WriteString(styles.WarningStyle.Render("OpenCode has not been run yet — model cache not found."))
		b.WriteString("\n")
		b.WriteString(styles.SubtextStyle.Render("Run 'opencode' once, then re-run 'gentle-ai sync' to assign models."))
		b.WriteString("\n")
		b.WriteString(styles.SubtextStyle.Render("Using default model assignments for now."))
		b.WriteString("\n\n")
		b.WriteString(renderOptions([]string{"← Back to SDD mode"}, cursor))
		b.WriteString("\n")
		b.WriteString(styles.HelpStyle.Render("enter/esc: go back"))
		return b.String()
	}

	b.WriteString(styles.SubtextStyle.Render("Current assignments:"))
	b.WriteString("\n\n")

	rows := ModelPickerRows()
	phases := opencode.SDDPhases()

	for idx, row := range rows {
		focused := idx == cursor

		var label string
		switch {
		case idx == 0:
			// "gentle-orchestrator" row — coordinator, individual assignment only
			assignment, ok := assignments[SDDOrchestratorPhase]
			if ok && assignment.ProviderID != "" {
				provName, modelName := resolveNames(assignment, state)
				label = fmt.Sprintf("%-20s %s / %s", row+" (coordinator)", provName, modelName)
			} else {
				label = fmt.Sprintf("%-20s (default)", row+" (coordinator)")
			}
		case idx == 1:
			// "Set all phases" row — show AllPhasesModel (only updated when this row is used).
			// Using AllPhasesModel instead of phases[0] prevents the label from changing
			// when the user picks a model for an individual sub-agent phase (Issue #146).
			if state.AllPhasesModel.ProviderID != "" {
				provName, modelName := resolveNames(state.AllPhasesModel, state)
				label = fmt.Sprintf("%-20s (%s / %s)", row, provName, modelName)
			} else {
				label = fmt.Sprintf("%-20s (not set)", row)
			}
		default:
			// Sub-agent rows start at idx 2; phases[idx-2] maps to the correct phase
			phase := phases[idx-2]
			assignment, ok := assignments[phase]
			if ok && assignment.ProviderID != "" {
				provName, modelName := resolveNames(assignment, state)
				label = fmt.Sprintf("%-20s %s / %s", row, provName, modelName)
			} else {
				label = fmt.Sprintf("%-20s (default)", row)
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

	return b.String()
}

func renderProviderSelect(state ModelPickerState) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Select provider:"))
	b.WriteString("\n\n")

	entries := ProviderEntries(state)

	end := state.ProviderScroll + maxVisibleItems
	if end > len(entries) {
		end = len(entries)
	}

	if state.ProviderScroll > 0 {
		b.WriteString(styles.SubtextStyle.Render("  ↑ more"))
		b.WriteString("\n")
	}

	for i := state.ProviderScroll; i < end; i++ {
		entry := entries[i]
		label := fmt.Sprintf("%s (%d models)", entry.Name, entry.ModelCount)
		focused := i == state.ProviderCursor

		if focused {
			b.WriteString(styles.SelectedStyle.Render(styles.Cursor+label) + "\n")
		} else {
			b.WriteString(styles.UnselectedStyle.Render("  "+label) + "\n")
		}
	}

	if end < len(entries) {
		b.WriteString(styles.SubtextStyle.Render("  ↓ more"))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select • esc: back"))

	return b.String()
}

func renderModelSelect(state ModelPickerState) string {
	var b strings.Builder

	provName := state.SelectedProvider
	if p, ok := state.Providers[state.SelectedProvider]; ok && p.Name != "" {
		provName = p.Name
	}

	b.WriteString(styles.TitleStyle.Render(fmt.Sprintf("Select model (%s):", provName)))
	b.WriteString("\n\n")

	models := state.SDDModels[state.SelectedProvider]

	end := state.ModelScroll + maxVisibleItems
	if end > len(models) {
		end = len(models)
	}

	if state.ModelScroll > 0 {
		b.WriteString(styles.SubtextStyle.Render("  ↑ more"))
		b.WriteString("\n")
	}

	for i := state.ModelScroll; i < end; i++ {
		m := models[i]
		label := m.Name
		if m.Cost.Input > 0 || m.Cost.Output > 0 {
			label += fmt.Sprintf("  ($%.2f/$%.2f)", m.Cost.Input, m.Cost.Output)
		}
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

// resolveNames returns the display name for a provider and model from an assignment.
func resolveNames(assignment model.ModelAssignment, state ModelPickerState) (provName, modelName string) {
	provName = assignment.ProviderID
	if p, exists := state.Providers[assignment.ProviderID]; exists && p.Name != "" {
		provName = p.Name
	}

	modelName = assignment.ModelID
	if p, exists := state.Providers[assignment.ProviderID]; exists {
		if m, ok := p.Models[assignment.ModelID]; ok && m.Name != "" {
			modelName = m.Name
		}
	}

	return provName, modelName
}
