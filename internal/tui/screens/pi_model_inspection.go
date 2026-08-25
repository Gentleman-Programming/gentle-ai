package screens

import (
	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/pi"
	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/styles"
	"strings"
)

type PiModelInspectionStatus uint8

const (
	PiModelInspectionLoading PiModelInspectionStatus = iota + 1
	PiModelInspectionError
	PiModelInspectionSuccess
)

type PiModelInspectionRow struct{ Agent, Model, Thinking string }
type PiModelInspectionState struct {
	Status         PiModelInspectionStatus
	Target         pi.ModelRoutingTarget
	Inspection     pi.ModelRoutingInspection
	Err            error
	Cursor, Scroll int
}

func NewPiModelInspectionState() PiModelInspectionState {
	return PiModelInspectionState{Status: PiModelInspectionLoading, Target: pi.ModelRoutingTargetProject}
}
func (s *PiModelInspectionState) SetResult(i pi.ModelRoutingInspection, err error) {
	s.Inspection, s.Err, s.Cursor, s.Scroll = i, err, 0, 0
	s.Status = PiModelInspectionSuccess
	if err != nil {
		s.Status = PiModelInspectionError
	}
}
func (s *PiModelInspectionState) SelectTarget(target pi.ModelRoutingTarget) {
	if s.Target != target {
		s.Target, s.Cursor, s.Scroll = target, 0, 0
	}
}
func (s *PiModelInspectionState) Move(delta, height int) {
	rows := len(s.Rows())
	if rows == 0 {
		return
	}
	if delta < 0 && s.Cursor > 0 {
		s.Cursor--
	} else if delta > 0 && s.Cursor+1 < rows {
		s.Cursor++
	}
	visible := PiModelInspectionVisibleRows(height)
	s.Scroll = max(0, min(s.Scroll, s.Cursor))
	if s.Cursor >= s.Scroll+visible {
		s.Scroll = s.Cursor - visible + 1
	}
	s.Scroll = min(max(0, s.Scroll), max(0, rows-visible))
}
func PiModelInspectionVisibleRows(height int) int {
	if height <= 0 {
		return 8
	}
	return max(3, min(8, height-12))
}
func (s PiModelInspectionState) Rows() []PiModelInspectionRow {
	target, ok := s.Inspection.Targets[s.Target]
	if !ok {
		return nil
	}
	rows := make([]PiModelInspectionRow, 0, len(s.Inspection.Agents))
	for _, agent := range s.Inspection.Agents {
		if !agent.Configurable {
			continue
		}
		a := target.Assignments[agent.Name]
		rows = append(rows, PiModelInspectionRow{Agent: agent.Name, Model: piAssignmentValue(a.Model, a.InheritModel, "unset"), Thinking: piAssignmentValue((*string)(a.Thinking), a.InheritThinking, "unset")})
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
func RenderPiModelInspection(state PiModelInspectionState, height int) string {
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
				if i == state.Cursor {
					b.WriteString(styles.SelectedStyle.Render(styles.Cursor+line) + "\n")
				} else {
					b.WriteString(styles.UnselectedStyle.Render("  "+line) + "\n")
				}
			}
		}
	}
	b.WriteString("\n" + styles.HelpStyle.Render("j/k or up/down: scroll • g: global • p: project • tab: switch • esc: back • q: quit"))
	return styles.FrameStyle.Render(b.String())
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
