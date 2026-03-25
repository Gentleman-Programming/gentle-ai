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
	ModePhaseList     ModelPickerMode = iota // Main screen: phase list + Continue/Back
	ModeProviderSelect                       // Sub-mode: pick a provider
	ModeModelSelect                          // Sub-mode: multi-select models from chosen provider
)

// maxVisibleItems is the maximum number of items shown in scrollable sub-lists.
const maxVisibleItems = 10

// ProviderEntry holds a provider ID, display name, and model count for the provider list.
type ProviderEntry struct {
	ID         string
	Name       string
	ModelCount int
}

// SelectedModel represents a selected model with its selection order.
type SelectedModel struct {
	Reference model.ModelReference
	Order     int // 0 = Primary, 1+ = Fallback order
}

// ModelPickerState holds the available providers and models for the picker screen,
// plus navigation state for the two-step sub-selection modes.
// Multi-select is supported: users can select multiple models per phase.
// The first selected model becomes Primary, subsequent selections become Fallbacks.
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

	// Multi-select state: tracks selected models per phase.
	// Key is phase name (e.g., "sdd-propose"), value is ordered list of model references.
	// The first model in the list is Primary, the rest are Fallbacks.
	SelectedModels map[string][]model.ModelReference
}

// NewModelPickerState initializes the picker state from the models cache.
// SelectedModels is initialized as an empty map to support multi-select.
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
		Providers:      providers,
		AvailableIDs:   available,
		SDDModels:      sddModels,
		Mode:           ModePhaseList,
		SelectedModels: make(map[string][]model.ModelReference),
	}
}

// ModelPickerRows returns the row labels for the model picker screen.
// Row 0 is "Set all phases", rows 1-9 are the SDD phases.
func ModelPickerRows() []string {
	rows := make([]string, 0, 10)
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
	assignments model.ModelAssignments,
) (handled bool, updatedAssignments model.ModelAssignments) {
	if assignments == nil {
		assignments = make(model.ModelAssignments)
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
	assignments model.ModelAssignments,
) (bool, model.ModelAssignments) {
	models := state.SDDModels[state.SelectedProvider]
	if len(models) == 0 {
		return false, assignments
	}

	// Determine the phase(s) being configured
	phases := opencode.SDDPhases()
	var targetPhases []string
	if state.SelectedPhaseIdx == 0 {
		// "Set all phases"
		targetPhases = phases
	} else {
		phaseIdx := state.SelectedPhaseIdx - 1
		if phaseIdx < len(phases) {
			targetPhases = []string{phases[phaseIdx]}
		}
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
	case " ":
		// Toggle selection of the current model for the target phase(s)
		selectedModel := models[state.ModelCursor]
		modelRef := model.ModelReference(state.SelectedProvider + "/" + selectedModel.ID)

		for _, phase := range targetPhases {
			// Initialize the slice if needed
			if state.SelectedModels == nil {
				state.SelectedModels = make(map[string][]model.ModelReference)
			}
			if state.SelectedModels[phase] == nil {
				state.SelectedModels[phase] = make([]model.ModelReference, 0)
			}

			// Check if already selected
			alreadySelected := false
			for i, ref := range state.SelectedModels[phase] {
				if ref == modelRef {
					// Remove from selection
					state.SelectedModels[phase] = append(
						state.SelectedModels[phase][:i],
						state.SelectedModels[phase][i+1:]...,
					)
					alreadySelected = true
					break
				}
			}

			// If not already selected, add to the list
			if !alreadySelected {
				state.SelectedModels[phase] = append(state.SelectedModels[phase], modelRef)
			}
		}
		return true, assignments
	case "enter":
		// Confirm selection: build ModelPool from SelectedModels
		// First selected = Primary, rest = Fallbacks
		for _, phase := range targetPhases {
			selected := state.SelectedModels[phase]
			if len(selected) > 0 {
				pool := model.ModelPool{
					Primary: selected[0],
				}
				if len(selected) > 1 {
					pool.Fallbacks = selected[1:]
				}
				assignments[phase] = pool
			}
		}

		// Clear selection for these phases after confirming
		for _, phase := range targetPhases {
			delete(state.SelectedModels, phase)
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
	assignments model.ModelAssignments,
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
	assignments model.ModelAssignments,
	state ModelPickerState,
	cursor int,
) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Assign Models to SDD Phases"))
	b.WriteString("\n")
	b.WriteString(styles.SubtextStyle.Render("First selected = Primary, others = Fallbacks"))
	b.WriteString("\n\n")

	if len(state.AvailableIDs) == 0 {
		b.WriteString(styles.WarningStyle.Render("No models found."))
		b.WriteString("\n")
		b.WriteString(styles.SubtextStyle.Render("Run 'opencode models --refresh' to update the model cache, or switch to single mode."))
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
		if idx == 0 {
			// "Set all phases" row — show the assignment of the first phase as representative
			pool, ok := assignments[phases[0]]
			if ok && pool.Primary != "" {
				label = formatPoolLabel(row, pool, state)
			} else {
				label = fmt.Sprintf("%-20s (not set)", row)
			}
		} else {
			phase := phases[idx-1]
			pool, ok := assignments[phase]
			if ok && pool.Primary != "" {
				label = formatPoolLabel(row, pool, state)
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
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select models • esc: back"))

	return b.String()
}

// formatPoolLabel formats a phase row label showing Primary and Fallback count.
// Example: "sdd_propose         Claude (Primary) + 2 fallbacks"
func formatPoolLabel(phaseName string, pool model.ModelPool, state ModelPickerState) string {
	provName, modelName := resolvePoolNames(pool, state)
	if provName == "" || modelName == "" {
		return fmt.Sprintf("%-20s (default)", phaseName)
	}

	if len(pool.Fallbacks) == 0 {
		return fmt.Sprintf("%-20s %s / %s", phaseName, provName, modelName)
	}

	// Show Primary + N fallbacks
	return fmt.Sprintf("%-20s %s / %s (+%d fallbacks)", phaseName, provName, modelName, len(pool.Fallbacks))
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

	// Determine which phases we're selecting for (to check selection status)
	phases := opencode.SDDPhases()
	var targetPhases []string
	if state.SelectedPhaseIdx == 0 {
		// "Set all phases" - use first phase for display purposes
		targetPhases = phases
	} else {
		phaseIdx := state.SelectedPhaseIdx - 1
		if phaseIdx < len(phases) {
			targetPhases = []string{phases[phaseIdx]}
		}
	}

	// Get currently selected models for the first target phase (for display)
	var currentSelected []model.ModelReference
	if len(targetPhases) > 0 && state.SelectedModels != nil {
		currentSelected = state.SelectedModels[targetPhases[0]]
	}

	// Show phase context in title
	phaseContext := ""
	if state.SelectedPhaseIdx == 0 {
		phaseContext = " (all phases)"
	} else if state.SelectedPhaseIdx <= len(phases) {
		phaseContext = fmt.Sprintf(" (%s)", phases[state.SelectedPhaseIdx-1])
	}

	b.WriteString(styles.TitleStyle.Render(fmt.Sprintf("Select models for %s%s:", provName, phaseContext)))
	b.WriteString("\n")
	b.WriteString(styles.SubtextStyle.Render("First selected = Primary, others = Fallbacks"))
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
		modelRef := model.ModelReference(state.SelectedProvider + "/" + m.ID)

		// Check if this model is selected
		isSelected := false
		for _, ref := range currentSelected {
			if ref == modelRef {
				isSelected = true
				break
			}
		}

		label := m.Name
		if m.Cost.Input > 0 || m.Cost.Output > 0 {
			label += fmt.Sprintf("  ($%.2f/$%.2f)", m.Cost.Input, m.Cost.Output)
		}

		// Show selection order if selected
		if isSelected {
			order := 0
			for idx, ref := range currentSelected {
				if ref == modelRef {
					order = idx + 1
					break
				}
			}
			if order == 1 {
				label = fmt.Sprintf("[x] %s (Primary)", m.Name)
			} else {
				label = fmt.Sprintf("[x] %s (Fallback #%d)", m.Name, order-1)
			}
			if m.Cost.Input > 0 || m.Cost.Output > 0 {
				label += fmt.Sprintf("  ($%.2f/$%.2f)", m.Cost.Input, m.Cost.Output)
			}
		}

		focused := i == state.ModelCursor
		b.WriteString(renderCheckbox(label, isSelected, focused))
	}

	if end < len(models) {
		b.WriteString(styles.SubtextStyle.Render("  ↓ more"))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • space: toggle selection • enter: confirm • esc: back"))

	return b.String()
}

// resolveNames returns the display name for a provider and model from an assignment.
// Deprecated: Use resolvePoolNames for ModelPool support.
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

// resolvePoolNames returns the display name for a provider and model from a ModelPool.
// Only the Primary model is displayed (fallbacks are not shown in the picker).
func resolvePoolNames(pool model.ModelPool, state ModelPickerState) (provName, modelName string) {
	if pool.Primary == "" {
		return "", ""
	}

	// Parse provider/model from the reference
	ref := string(pool.Primary)
	for i := len(ref) - 1; i >= 0; i-- {
		if ref[i] == '/' {
			providerID := ref[:i]
			modelID := ref[i+1:]

			provName = providerID
			if p, exists := state.Providers[providerID]; exists && p.Name != "" {
				provName = p.Name
			}

			modelName = modelID
			if p, exists := state.Providers[providerID]; exists {
				if m, ok := p.Models[modelID]; ok && m.Name != "" {
					modelName = m.Name
				}
			}

			return provName, modelName
		}
	}

	// No slash found - just return the reference as-is
	return ref, ref
}
