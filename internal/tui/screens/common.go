package screens

import (
	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/styles"
)

func renderOptions(options []string, cursor int) string {
	output := ""
	for idx, option := range options {
		if idx == cursor {
			output += styles.SelectedStyle.Render(styles.Cursor+option) + "\n"
		} else {
			output += styles.UnselectedStyle.Render("  "+option) + "\n"
		}
	}

	return output
}

func RenderOperationRunning(title, detail string, spinnerFrame int) string {
	return styles.TitleStyle.Render(title) + "\n\n" +
		styles.WarningStyle.Render(SpinnerChar(spinnerFrame)+"  "+detail) + "\n\n" +
		styles.HelpStyle.Render("Please wait...")
}

func renderCheckbox(label string, checked bool, focused bool) string {
	marker := "[ ]"
	markerStyle := styles.UnselectedStyle
	if checked {
		marker = "[x]"
		markerStyle = styles.SuccessStyle
	}

	prefix := "  "
	if focused {
		prefix = styles.Cursor
		return styles.SelectedStyle.Render(prefix+markerStyle.Render(marker)+" "+label) + "\n"
	}

	return styles.UnselectedStyle.Render(prefix+markerStyle.Render(marker)+" "+label) + "\n"
}

func renderRadio(label string, selected bool, focused bool) string {
	marker := "( )"
	markerStyle := styles.UnselectedStyle
	if selected {
		marker = "(*)"
		markerStyle = styles.SelectedStyle
	}

	prefix := "  "
	if focused {
		prefix = styles.Cursor
		return styles.SelectedStyle.Render(prefix+markerStyle.Render(marker)+" "+label) + "\n"
	}

	return styles.UnselectedStyle.Render(prefix+markerStyle.Render(marker)+" "+label) + "\n"
}
