package screens

import (
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
)

// StrictTDDOptionEnable is the index of the "Enable" option.
const StrictTDDOptionEnable = 0

// StrictTDDOptionDisable is the index of the "Disable" option.
const StrictTDDOptionDisable = 1

// StrictTDDOptions returns the list of option labels for the Strict TDD screen.
func StrictTDDOptions() []string {
	return []string{"Enable", "Disable"}
}

// RenderStrictTDD renders the Strict TDD Mode selection screen.
// enabled indicates whether Strict TDD Mode is currently active.
// cursor is the current cursor position.
func RenderStrictTDD(enabled bool, cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("STRICT TDD MODE"))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Should agents follow Strict TDD (RED → GREEN → REFACTOR) for every task?"))
	b.WriteString("\n")
	b.WriteString(styles.SubtextStyle.Render("When enabled, the sdd-apply agent writes tests first, confirms failure,"))
	b.WriteString("\n")
	b.WriteString(styles.SubtextStyle.Render("then implements the minimum code to pass before refactoring."))
	b.WriteString("\n\n")

	options := StrictTDDOptions()
	for idx, opt := range options {
		isSelected := (idx == StrictTDDOptionEnable && enabled) || (idx == StrictTDDOptionDisable && !enabled)
		focused := idx == cursor
		b.WriteString(renderRadio(opt, isSelected, focused))
	}

	b.WriteString("\n")
	b.WriteString(renderOptions([]string{"Back"}, cursor-len(options)))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select • esc: back"))

	return b.String()
}
