package screens

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/pi"
	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/styles"
)

type PiModelInspectionStatus uint8

type PiModelInspectionMode uint8

const (
	PiModelInspectionModeAgents PiModelInspectionMode = iota
	PiModelInspectionModeProviders
	PiModelInspectionModeModels
	PiModelInspectionModeThinking
	PiModelInspectionModeValidating
	PiModelInspectionModeValidationResult
	PiModelInspectionModeReviewReady
	PiModelInspectionModeConfirmApply
	PiModelInspectionModeApplying
	PiModelInspectionModeApplyResult
)

const (
	PiModelInspectionLoading PiModelInspectionStatus = iota + 1
	PiModelInspectionError
	PiModelInspectionSuccess
)

type PiModelInspectionRow struct {
	Agent, Model, Thinking string
	Pending                bool
}
type PiModelInspectionState struct {
	Status            PiModelInspectionStatus
	Target            pi.ModelRoutingTarget
	Inspection        pi.ModelRoutingInspection
	Draft             pi.ModelRoutingDraft
	ValidationResult  pi.ModelRoutingValidationResult
	ValidationErr     error
	ApplyResult       pi.ModelRoutingApplyResult
	ApplyErr          error
	Mode              PiModelInspectionMode
	SelectedAgent     string
	SelectedProvider  string
	PendingAssignment pi.ModelRoutingDraftAssignment
	Err               error
	Cursor, Scroll    int
}

func NewPiModelInspectionState() PiModelInspectionState {
	return PiModelInspectionState{Status: PiModelInspectionLoading, Target: pi.ModelRoutingTargetProject}
}
func (s *PiModelInspectionState) SetResult(i pi.ModelRoutingInspection, err error) {
	s.Inspection, s.Err, s.Cursor, s.Scroll = i, err, 0, 0
	s.ValidationResult, s.ValidationErr = pi.ModelRoutingValidationResult{}, nil
	s.ApplyResult, s.ApplyErr = pi.ModelRoutingApplyResult{}, nil
	s.Mode, s.SelectedAgent, s.SelectedProvider = PiModelInspectionModeAgents, "", ""
	s.PendingAssignment = pi.ModelRoutingDraftAssignment{}
	s.Status = PiModelInspectionSuccess
	if err != nil {
		s.Status = PiModelInspectionError
	}
}
func (s *PiModelInspectionState) BeginValidation() bool {
	if s.Status != PiModelInspectionSuccess || s.Mode != PiModelInspectionModeAgents {
		return false
	}
	s.ValidationResult, s.ValidationErr = pi.ModelRoutingValidationResult{}, nil
	s.ApplyResult, s.ApplyErr = pi.ModelRoutingApplyResult{}, nil
	s.Mode = PiModelInspectionModeValidating
	return true
}
func (s *PiModelInspectionState) SetValidationResult(result pi.ModelRoutingValidationResult, err error) {
	s.ValidationResult = clonePiValidationResult(result)
	s.ValidationErr = err
	if err == nil && result.OK {
		s.Mode = PiModelInspectionModeReviewReady
	} else {
		s.Mode = PiModelInspectionModeValidationResult
	}
}
func (s *PiModelInspectionState) ClearValidation() {
	s.ValidationResult, s.ValidationErr = pi.ModelRoutingValidationResult{}, nil
	s.ApplyResult, s.ApplyErr = pi.ModelRoutingApplyResult{}, nil
	s.Mode = PiModelInspectionModeAgents
}
func (s *PiModelInspectionState) BeginApplyConfirmation() bool {
	if s.Status != PiModelInspectionSuccess || s.Mode != PiModelInspectionModeReviewReady {
		return false
	}
	s.ApplyResult, s.ApplyErr = pi.ModelRoutingApplyResult{}, nil
	s.Mode = PiModelInspectionModeConfirmApply
	return true
}
func (s *PiModelInspectionState) BeginApply() bool {
	if s.Status != PiModelInspectionSuccess || s.Mode != PiModelInspectionModeConfirmApply {
		return false
	}
	s.ApplyResult, s.ApplyErr = pi.ModelRoutingApplyResult{}, nil
	s.Mode = PiModelInspectionModeApplying
	return true
}
func (s *PiModelInspectionState) SetApplyResult(result pi.ModelRoutingApplyResult, err error) {
	s.ApplyResult, s.ApplyErr = clonePiApplyResult(result), err
	s.Mode = PiModelInspectionModeApplyResult
}
func (s *PiModelInspectionState) ClearApply() {
	s.ApplyResult, s.ApplyErr = pi.ModelRoutingApplyResult{}, nil
	s.Mode = PiModelInspectionModeReviewReady
}
func clonePiValidationResult(result pi.ModelRoutingValidationResult) pi.ModelRoutingValidationResult {
	result.Diagnostics = append([]pi.ModelRoutingDiagnostic(nil), result.Diagnostics...)
	for i := range result.Diagnostics {
		if result.Diagnostics[i].Path != nil {
			path := *result.Diagnostics[i].Path
			result.Diagnostics[i].Path = &path
		}
	}
	return result
}
func clonePiApplyResult(result pi.ModelRoutingApplyResult) pi.ModelRoutingApplyResult {
	if result.Target != nil {
		target := *result.Target
		result.Target = &target
	}
	if result.ConfigPath != nil {
		path := *result.ConfigPath
		result.ConfigPath = &path
	}
	result.Diagnostics = append([]pi.ModelRoutingDiagnostic(nil), result.Diagnostics...)
	for i := range result.Diagnostics {
		if result.Diagnostics[i].Path != nil {
			path := *result.Diagnostics[i].Path
			result.Diagnostics[i].Path = &path
		}
	}
	if result.Materialization != nil {
		materialization := *result.Materialization
		materialization.Affected = append([]string(nil), materialization.Affected...)
		materialization.Succeeded = append([]string(nil), materialization.Succeeded...)
		materialization.Failed = append([]pi.ModelRoutingApplyFailure(nil), materialization.Failed...)
		result.Materialization = &materialization
	}
	return result
}
func (s *PiModelInspectionState) SelectTarget(target pi.ModelRoutingTarget) {
	if s.Target != target {
		s.Target, s.Cursor, s.Scroll = target, 0, 0
	}
}
func (s *PiModelInspectionState) move(delta, count, height int) {
	if count == 0 {
		return
	}
	if delta < 0 && s.Cursor > 0 {
		s.Cursor--
	} else if delta > 0 && s.Cursor+1 < count {
		s.Cursor++
	}
	visible := PiModelInspectionVisibleRows(height)
	s.Scroll = max(0, min(s.Scroll, s.Cursor))
	if s.Cursor >= s.Scroll+visible {
		s.Scroll = s.Cursor - visible + 1
	}
	s.Scroll = min(max(0, s.Scroll), max(0, count-visible))
}
func (s *PiModelInspectionState) Move(delta, height int) {
	s.move(delta, len(s.Rows()), height)
}
func (s *PiModelInspectionState) MoveEditor(delta, height int) {
	s.move(delta, len(s.editorOptions()), height)
}
func PiModelInspectionVisibleRows(height int) int {
	if height <= 0 {
		return 8
	}
	return max(3, min(8, height-12))
}
func (s PiModelInspectionState) Rows() []PiModelInspectionRow {
	target, ok := s.Inspection.Targets[s.Target]
	if !ok && len(s.Draft) == 0 {
		return nil
	}
	rows := make([]PiModelInspectionRow, 0, len(s.Inspection.Agents))
	for _, agent := range s.Inspection.Agents {
		if !agent.Configurable {
			continue
		}
		a := target.Assignments[agent.Name]
		row := PiModelInspectionRow{Agent: agent.Name, Model: piAssignmentValue(a.Model, a.InheritModel, "unset"), Thinking: piAssignmentValue((*string)(a.Thinking), a.InheritThinking, "unset")}
		if d, edited := s.Draft[agent.Name]; edited {
			row.Model, row.Thinking, row.Pending = piAssignmentValue(d.Model, d.Model == nil, "inherited"), piAssignmentValue((*string)(d.Thinking), d.Thinking == nil, "inherited"), true
		}
		rows = append(rows, row)
	}
	return rows
}
func piAssignmentValue(value *string, inherited bool, unset string) string {
	if inherited {
		return "inherited"
	}
	if value != nil && *value != "" {
		return *value
	}
	return unset
}
func clonePiDraftAssignment(a pi.ModelRoutingDraftAssignment) pi.ModelRoutingDraftAssignment {
	out := pi.ModelRoutingDraftAssignment{}
	if a.Model != nil {
		v := *a.Model
		out.Model = &v
	}
	if a.Thinking != nil {
		v := *a.Thinking
		out.Thinking = &v
	}
	return out
}
func piDraftFromAssignment(a pi.ModelRoutingAssignment) pi.ModelRoutingDraftAssignment {
	out := pi.ModelRoutingDraftAssignment{}
	if !a.InheritModel && a.Model != nil && *a.Model != "" {
		v := *a.Model
		out.Model = &v
	}
	if !a.InheritThinking && a.Thinking != nil && *a.Thinking != "" {
		v := *a.Thinking
		out.Thinking = &v
	}
	return out
}
func (s PiModelInspectionState) targetDraftAssignment(agent string) pi.ModelRoutingDraftAssignment {
	if target, ok := s.Inspection.Targets[s.Target]; ok {
		return piDraftFromAssignment(target.Assignments[agent])
	}
	return pi.ModelRoutingDraftAssignment{}
}
func (s PiModelInspectionState) availableModels() []pi.ModelRoutingModel {
	out := make([]pi.ModelRoutingModel, 0)
	for _, m := range s.Inspection.Models {
		if m.Configured && m.Available && m.Provider != "" && piModelKey(m) != "" {
			out = append(out, m)
		}
	}
	return out
}
func (s PiModelInspectionState) ProviderOptions() []string {
	seen := map[string]bool{}
	for _, m := range s.availableModels() {
		seen[m.Provider] = true
	}
	out := make([]string, 0, len(seen))
	for provider := range seen {
		out = append(out, provider)
	}
	sort.Strings(out)
	return out
}
func piModelKey(model pi.ModelRoutingModel) string {
	if model.CanonicalID != "" {
		return model.CanonicalID
	}
	if model.Provider != "" && model.ModelID != "" {
		return model.Provider + "/" + model.ModelID
	}
	return model.ModelID
}
func (s PiModelInspectionState) ModelOptions() []pi.ModelRoutingModel {
	models := make([]pi.ModelRoutingModel, 0)
	for _, m := range s.availableModels() {
		if m.Provider == s.SelectedProvider {
			models = append(models, m)
		}
	}
	sort.SliceStable(models, func(i, j int) bool {
		left, right := piModelKey(models[i]), piModelKey(models[j])
		if left != right {
			return left < right
		}
		if models[i].ModelID != models[j].ModelID {
			return models[i].ModelID < models[j].ModelID
		}
		return models[i].Name < models[j].Name
	})
	return models
}
func (s PiModelInspectionState) ThinkingOptions() []string {
	options := []string{"inherit"}
	if s.PendingAssignment.Model == nil {
		return options
	}
	for _, model := range s.ModelOptions() {
		if piModelKey(model) != *s.PendingAssignment.Model {
			continue
		}
		seen := map[string]bool{}
		for _, level := range model.SupportedThinkingLevels {
			if level != "" && !seen[level] {
				options, seen[level] = append(options, level), true
			}
		}
		break
	}
	return options
}
func (s PiModelInspectionState) editorOptions() []string {
	switch s.Mode {
	case PiModelInspectionModeProviders:
		return append([]string{"inherit"}, s.ProviderOptions()...)
	case PiModelInspectionModeModels:
		models, out := s.ModelOptions(), []string{}
		for _, model := range models {
			out = append(out, piModelKey(model))
		}
		return out
	case PiModelInspectionModeThinking:
		return s.ThinkingOptions()
	}
	return nil
}
func piCursor(value string, options []string, offset int) int {
	for i, option := range options {
		if option == value {
			return i + offset
		}
	}
	return 0
}
func (s PiModelInspectionState) agentCursor() int {
	for i, row := range s.Rows() {
		if row.Agent == s.SelectedAgent {
			return i
		}
	}
	return 0
}
func (s *PiModelInspectionState) BeginEdit() bool {
	if s.Status != PiModelInspectionSuccess || s.Mode != PiModelInspectionModeAgents {
		return false
	}
	rows := s.Rows()
	if s.Cursor < 0 || s.Cursor >= len(rows) {
		return false
	}
	s.SelectedAgent = rows[s.Cursor].Agent
	if draft, ok := s.Draft[s.SelectedAgent]; ok {
		s.PendingAssignment = clonePiDraftAssignment(draft)
	} else {
		s.PendingAssignment = s.targetDraftAssignment(s.SelectedAgent)
	}
	s.SelectedProvider = ""
	if s.PendingAssignment.Model != nil {
		for _, m := range s.availableModels() {
			if piModelKey(m) == *s.PendingAssignment.Model {
				s.SelectedProvider = m.Provider
				break
			}
		}
	}
	s.Mode, s.Cursor, s.Scroll = PiModelInspectionModeProviders, piCursor(s.SelectedProvider, s.ProviderOptions(), 1), 0
	return true
}
func (s *PiModelInspectionState) SelectEditor() {
	options := s.editorOptions()
	if s.Cursor < 0 || s.Cursor >= len(options) {
		return
	}
	switch s.Mode {
	case PiModelInspectionModeProviders:
		if s.Cursor == 0 {
			s.PendingAssignment.Model, s.SelectedProvider = nil, ""
			s.Mode, s.Cursor = PiModelInspectionModeThinking, piCursor(thinkingString(s.PendingAssignment.Thinking), s.ThinkingOptions(), 0)
			return
		}
		s.SelectedProvider, s.Mode = options[s.Cursor], PiModelInspectionModeModels
		s.Cursor, s.Scroll = piCursor(stringValue(s.PendingAssignment.Model), s.editorOptions(), 0), 0
	case PiModelInspectionModeModels:
		id := piModelKey(s.ModelOptions()[s.Cursor])
		s.PendingAssignment.Model = &id
		levels := s.ThinkingOptions()
		if p := s.PendingAssignment.Thinking; p != nil && piCursor(string(*p), levels, 0) == 0 {
			s.PendingAssignment.Thinking = nil
		}
		s.Mode, s.Cursor, s.Scroll = PiModelInspectionModeThinking, piCursor(thinkingString(s.PendingAssignment.Thinking), levels, 0), 0
	case PiModelInspectionModeThinking:
		if s.Cursor == 0 {
			s.PendingAssignment.Thinking = nil
		} else {
			level := pi.ModelRoutingThinking(options[s.Cursor])
			s.PendingAssignment.Thinking = &level
		}
		s.commitEdit()
	}
}
func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
func thinkingString(value *pi.ModelRoutingThinking) string {
	if value == nil {
		return ""
	}
	return string(*value)
}
func (s *PiModelInspectionState) commitEdit() {
	if s.Draft == nil {
		s.Draft = make(pi.ModelRoutingDraft)
	}
	s.Draft[s.SelectedAgent], s.Mode, s.PendingAssignment = clonePiDraftAssignment(s.PendingAssignment), PiModelInspectionModeAgents, pi.ModelRoutingDraftAssignment{}
	s.Cursor, s.Scroll = s.agentCursor(), 0
}
func (s *PiModelInspectionState) BackEditor() {
	s.Scroll = 0
	switch s.Mode {
	case PiModelInspectionModeProviders:
		s.Mode, s.Cursor, s.PendingAssignment, s.SelectedProvider = PiModelInspectionModeAgents, s.agentCursor(), pi.ModelRoutingDraftAssignment{}, ""
	case PiModelInspectionModeModels:
		s.Mode, s.Cursor = PiModelInspectionModeProviders, piCursor(s.SelectedProvider, s.ProviderOptions(), 1)
	case PiModelInspectionModeThinking:
		if s.PendingAssignment.Model != nil {
			s.Mode = PiModelInspectionModeModels
			s.Cursor = piCursor(*s.PendingAssignment.Model, s.editorOptions(), 0)
		} else {
			s.Mode, s.Cursor = PiModelInspectionModeProviders, piCursor(s.SelectedProvider, s.ProviderOptions(), 1)
		}
	}
}
func renderPiModelEditor(state PiModelInspectionState, height int) string {
	state.MoveEditor(0, height)
	current, options := state.targetDraftAssignment(state.SelectedAgent), state.editorOptions()
	var b strings.Builder
	b.WriteString(styles.SubtextStyle.Render("Agent: " + piSafeText(state.SelectedAgent) + " • phase: " + [...]string{"agent", "model provider", "model", "thinking"}[state.Mode]))
	b.WriteString("\n" + styles.SubtextStyle.Render("Current: model="+piSafeText(piAssignmentValue(current.Model, current.Model == nil, "inherited"))+" thinking="+piSafeText(piAssignmentValue((*string)(current.Thinking), current.Thinking == nil, "inherited"))))
	b.WriteString("\n" + styles.SubtextStyle.Render("Pending: model="+piSafeText(piAssignmentValue(state.PendingAssignment.Model, state.PendingAssignment.Model == nil, "inherited"))+" thinking="+piSafeText(piAssignmentValue((*string)(state.PendingAssignment.Thinking), state.PendingAssignment.Thinking == nil, "inherited"))))
	if len(options) == 0 {
		b.WriteString("\n" + styles.SubtextStyle.Render("No configured and available models for this provider."))
		return b.String()
	}
	b.WriteString("\n")
	end := min(len(options), state.Scroll+PiModelInspectionVisibleRows(height))
	for i := state.Scroll; i < end; i++ {
		label := options[i]
		if i == 0 && state.Mode == PiModelInspectionModeProviders {
			label = "Inherit model"
		}
		if i == 0 && state.Mode == PiModelInspectionModeThinking {
			label = "Inherit thinking"
		}
		label = piSafeText(label)
		if i == state.Cursor {
			b.WriteString(styles.SelectedStyle.Render(styles.Cursor+label) + "\n")
		} else {
			b.WriteString(styles.UnselectedStyle.Render("  "+label) + "\n")
		}
	}
	return b.String()
}
func RenderPiModelInspection(state PiModelInspectionState, height int) string {
	switch state.Mode {
	case PiModelInspectionModeConfirmApply:
		return renderPiApplyConfirm(state)
	case PiModelInspectionModeApplying:
		return renderPiApplying(state)
	case PiModelInspectionModeApplyResult:
		return renderPiApplyResult(state)
	}
	if state.Mode >= PiModelInspectionModeValidating {
		return renderPiDraftValidation(state)
	}
	var b strings.Builder
	b.WriteString(styles.TitleStyle.Render("Configure Pi Models") + "\n\n")
	switch state.Status {
	case PiModelInspectionLoading:
		b.WriteString(styles.SubtextStyle.Render("Loading read-only Pi model configuration..."))
	case PiModelInspectionError:
		b.WriteString(styles.ErrorStyle.Render("Pi model inspection failed."))
		if state.Err != nil {
			if detail := piSafeText(state.Err.Error()); detail != "" {
				b.WriteString("\n" + styles.SubtextStyle.Render("Details: "+detail))
			}
		}
		b.WriteString("\n" + styles.SubtextStyle.Render("Check that Pi is installed and model routing is available, then try again."))
		renderPiDiagnostics(&b, state.Inspection.Diagnostics)
	case PiModelInspectionSuccess:
		if state.Mode != PiModelInspectionModeAgents {
			b.WriteString(renderPiModelEditor(state, height))
			break
		}
		state.Move(0, height)
		target := state.Inspection.Targets[state.Target]
		b.WriteString(styles.SubtextStyle.Render("Target: " + piSafeText(string(state.Target)) + " • source: " + piSafeText(string(target.Provenance.Source)) + " • status: " + piSafeText(string(target.Provenance.Status))))
		rows, visible := state.Rows(), PiModelInspectionVisibleRows(height)
		if len(rows) == 0 {
			b.WriteString("\n" + styles.SubtextStyle.Render("No configurable Pi agents found."))
		} else {
			b.WriteString("\n")
			end := state.Scroll + visible
			if end > len(rows) {
				end = len(rows)
			}
			for i := state.Scroll; i < end; i++ {
				line := piSafeText(rows[i].Agent) + "  model: " + piSafeText(rows[i].Model) + "  thinking: " + piSafeText(rows[i].Thinking)
				if rows[i].Pending {
					line += "  [pending]"
				}
				if i == state.Cursor {
					b.WriteString(styles.SelectedStyle.Render(styles.Cursor+line) + "\n")
				} else {
					b.WriteString(styles.UnselectedStyle.Render("  "+line) + "\n")
				}
			}
		}
	}
	if state.Status == PiModelInspectionSuccess && state.Mode != PiModelInspectionModeAgents {
		b.WriteString("\n" + styles.HelpStyle.Render("j/k or up/down: navigate • enter/space: select • esc: back"))
	} else {
		b.WriteString("\n" + styles.HelpStyle.Render("j/k or up/down: scroll • g: global • p: project • tab: switch • enter: edit • v: validate • esc: back • q: quit"))
	}
	return styles.FrameStyle.Render(b.String())
}
func renderPiDraftValidation(state PiModelInspectionState) string {
	var b strings.Builder
	b.WriteString(styles.TitleStyle.Render("Configure Pi Models") + "\n\n")
	switch state.Mode {
	case PiModelInspectionModeValidating:
		b.WriteString(styles.SubtextStyle.Render("Validating the current Pi model draft..."))
	case PiModelInspectionModeReviewReady:
		b.WriteString(styles.SuccessStyle.Render("Pi draft validated and ready for review."))
	default:
		b.WriteString(styles.ErrorStyle.Render("Pi draft validation did not pass."))
	}
	b.WriteString("\n" + renderPiDraftSummary(state.Draft))
	if state.Mode == PiModelInspectionModeReviewReady {
		b.WriteString("\n" + styles.SubtextStyle.Render("No changes were applied."))
		b.WriteString("\n" + styles.SubtextStyle.Render("Press a to review and explicitly confirm applying this draft."))
	} else if state.Mode == PiModelInspectionModeValidationResult {
		b.WriteString("\n" + styles.SubtextStyle.Render(piValidationErrorText(state.ValidationResult, state.ValidationErr)))
	}
	renderPiDiagnostics(&b, state.ValidationResult.Diagnostics)
	help := "esc: cancel • q: quit"
	if state.Mode == PiModelInspectionModeReviewReady {
		help = "a: review apply • esc: back • q: quit"
	} else if state.Mode != PiModelInspectionModeValidating {
		help = "esc: back • q: quit"
	}
	b.WriteString("\n" + styles.HelpStyle.Render(help))
	return styles.FrameStyle.Render(b.String())
}
func renderPiApplyConfirm(state PiModelInspectionState) string {
	var b strings.Builder
	b.WriteString(styles.TitleStyle.Render("Confirm Pi Model Apply") + "\n\n")
	b.WriteString(styles.WarningStyle.Render("This writes Pi routing/materialization."))
	b.WriteString("\n" + styles.SubtextStyle.Render("Target: "+piSafeText(string(state.Target))))
	b.WriteString("\n" + styles.SubtextStyle.Render("Pending agent changes:"))
	b.WriteString("\n" + renderPiDraftSummary(state.Draft))
	b.WriteString("\n" + styles.HelpStyle.Render("enter or y: apply • esc: back • q: quit"))
	return styles.FrameStyle.Render(b.String())
}
func renderPiApplying(state PiModelInspectionState) string {
	var b strings.Builder
	b.WriteString(styles.TitleStyle.Render("Applying Pi Model Routing") + "\n\n")
	b.WriteString(styles.SubtextStyle.Render("Applying the validated draft to Pi."))
	b.WriteString("\n" + styles.SubtextStyle.Render("Target: "+piSafeText(string(state.Target))))
	b.WriteString("\n" + styles.SubtextStyle.Render("Writing Pi routing/materialization..."))
	b.WriteString("\n\n" + styles.WarningStyle.Render("Bounded operation in progress; please wait."))
	b.WriteString("\n" + styles.HelpStyle.Render("ctrl+c: emergency quit"))
	return styles.FrameStyle.Render(b.String())
}
func renderPiApplyResult(state PiModelInspectionState) string {
	var b strings.Builder
	result := state.ApplyResult
	b.WriteString(styles.TitleStyle.Render("Pi Model Apply Result") + "\n\n")
	if result.Outcome == "" {
		b.WriteString(styles.ErrorStyle.Render("Outcome unknown"))
		b.WriteString("\n" + styles.SubtextStyle.Render("Pi did not return an authoritative apply result. Inspect Pi configuration before retrying."))
	} else {
		if result.Outcome == pi.ModelRoutingApplyOutcomeSuccess && result.Saved {
			b.WriteString(styles.SuccessStyle.Render("Pi model routing saved."))
		} else if result.Outcome == pi.ModelRoutingApplyOutcomePartial {
			b.WriteString(styles.WarningStyle.Render("Pi model routing partially materialized."))
		} else {
			b.WriteString(styles.ErrorStyle.Render("Pi model routing was not saved."))
		}
		b.WriteString("\nSaved: " + fmt.Sprintf("%t", result.Saved))
		b.WriteString("\nOutcome: " + piSafeText(string(result.Outcome)))
		if result.Target != nil {
			b.WriteString("\nTarget: " + piSafeText(string(*result.Target)))
		} else {
			b.WriteString("\nTarget: " + piSafeText(string(state.Target)))
		}
		if result.ConfigPath != nil {
			b.WriteString("\nConfig path: " + piSafeText(*result.ConfigPath))
		}
		if message := piApplyOutcomeText(result.Outcome); message != "" {
			b.WriteString("\n" + styles.SubtextStyle.Render(message))
		}
	}
	renderPiMaterialization(&b, result.Materialization)
	renderPiDiagnostics(&b, result.Diagnostics)
	b.WriteString("\n" + styles.HelpStyle.Render("enter or esc: return to Model Config • q: quit"))
	return styles.FrameStyle.Render(b.String())
}
func piApplyOutcomeText(outcome pi.ModelRoutingApplyOutcome) string {
	switch outcome {
	case pi.ModelRoutingApplyOutcomeValidationFailure:
		return "Pi rejected this draft. Review diagnostics and edit the draft."
	case pi.ModelRoutingApplyOutcomeUnavailableRuntime:
		return "Pi runtime was unavailable. No saved state was reported."
	case pi.ModelRoutingApplyOutcomePersistenceFailure:
		return "Pi could not persist this routing. No saved state was reported."
	case pi.ModelRoutingApplyOutcomePartial:
		return "Inspect the succeeded and failed materialization targets before retrying."
	}
	return ""
}
func renderPiMaterialization(b *strings.Builder, materialization *pi.ModelRoutingApplyMaterialization) {
	if materialization == nil {
		b.WriteString("\nMaterialization: unavailable")
		return
	}
	b.WriteString(fmt.Sprintf("\nMaterialization: affected=%d succeeded=%d failed=%d", len(materialization.Affected), len(materialization.Succeeded), len(materialization.Failed)))
	b.WriteString("\nSucceeded targets: " + joinPiSafeText(materialization.Succeeded))
	if len(materialization.Failed) == 0 {
		b.WriteString("\nFailed targets: none")
		return
	}
	b.WriteString("\nFailed targets:")
	for _, failure := range materialization.Failed {
		b.WriteString("\n  " + piSafeText(failure.Target) + ": " + piSafeText(failure.Message))
	}
}
func joinPiSafeText(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	cleaned := make([]string, len(values))
	for i, value := range values {
		cleaned[i] = piSafeText(value)
	}
	return strings.Join(cleaned, ", ")
}
func renderPiDraftSummary(draft pi.ModelRoutingDraft) string {
	if len(draft) == 0 {
		return styles.SubtextStyle.Render("Draft: empty (inherit current settings)")
	}
	names := make([]string, 0, len(draft))
	for name := range draft {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("Draft overrides:")
	for _, name := range names {
		assignment := draft[name]
		model, thinking := "inherited", "inherited"
		if assignment.Model != nil {
			model = piSafeText(*assignment.Model)
		}
		if assignment.Thinking != nil {
			thinking = piSafeText(string(*assignment.Thinking))
		}
		b.WriteString("\n  " + piSafeText(name) + ": model=" + model + " thinking=" + thinking)
	}
	return styles.SubtextStyle.Render(b.String())
}
func piValidationErrorText(result pi.ModelRoutingValidationResult, err error) string {
	if err == nil {
		if !result.OK {
			return "Pi rejected this draft. Review diagnostics and edit the draft."
		}
		return ""
	}
	var clientErr *pi.ModelRoutingClientError
	if errors.As(err, &clientErr) && clientErr != nil {
		switch clientErr.ExitClass {
		case "invalid-input":
			return "Pi rejected this draft as invalid. Review diagnostics and edit the draft."
		case "unsupported-contract":
			return "Pi does not support this validation contract. Update Pi or choose a compatible client."
		case "unavailable-runtime":
			return "Pi validation is unavailable. Check the Pi runtime and try again."
		}
		if clientErr.TransportKind == pi.TransportErrorTimeout {
			return "Pi draft validation timed out. Check the Pi runtime and try again."
		}
	}
	var transportErr *pi.TransportError
	if errors.As(err, &transportErr) && transportErr != nil && transportErr.Kind == pi.TransportErrorTimeout {
		return "Pi draft validation timed out. Check the Pi runtime and try again."
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "Pi draft validation timed out. Check the Pi runtime and try again."
	}
	if errors.Is(err, errors.ErrUnsupported) {
		return "Pi does not support this validation operation. Update Pi or choose a compatible client."
	}
	return "Pi draft validation could not be completed. Check the Pi runtime and try again."
}
func piSafeText(value string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, value)
}
func renderPiDiagnostics(b *strings.Builder, diagnostics []pi.ModelRoutingDiagnostic) {
	for _, d := range diagnostics {
		message := piSafeText(d.Message)
		if message == "" {
			message = piSafeText(d.Code)
		}
		if message != "" {
			b.WriteString("\n" + styles.WarningStyle.Render("Diagnostic: "+piSafeText(string(d.Severity))+": "+message))
		}
	}
}
