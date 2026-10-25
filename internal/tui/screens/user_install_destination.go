package screens

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
	"github.com/gentleman-programming/gentle-ai/v4/internal/tui/styles"
)

// RenderUserInstallDestination presents editing, not installation authority.
// The containing model owns input and fresh physical inspection.
func RenderUserInstallDestination(req shellinstaller.UserInstallRequest, field int, problem error, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	labels := []string{"Target", "Mode"}
	values := []string{req.Destination, req.Mode}
	if req.Mode == "shared" {
		labels = append(labels, "Shared prefix", "Shared agent")
		values = append(values, req.SharedPrefix, req.SharedAgent)
	}
	field = min(max(0, field), len(labels)-1)
	leftWidth := width
	wide := width >= 100 && height >= 10
	if wide {
		leftWidth = (width - 1) / 2
	}
	var left []string
	for i, label := range labels {
		style, prefix := styles.UnselectedStyle, "  "
		if i == field {
			style, prefix = styles.SelectedStyle, "> "
		}
		left = append(left, renderShellExperienceConfigLine(style, prefix+label+": "+values[i], leftWidth))
	}
	detail := "Choose a new target under a private owned parent. Separate installs Gentle-Shell in the target; Shared reuses your existing Pi prefix and agent and writes Context7 into its mcp.json after the recovery preimages exist. Main preview works with Separate or On my existing Pi; the retained Shared prefix keeps Stable. Editing applies no preferences. " + "Personal Pi and retained project overrides remain unchanged."
	if req.Experience != nil {
		detail += " Persona: " + req.Experience.Persona + ". Background subagents: " + req.Experience.BackgroundSubagents + "."
	}
	if problem != nil {
		detail = "Inspection refused: " + problem.Error() + "\n" + detail
	}
	lines := []string{}
	if height >= 5 {
		lines = append(lines, renderShellExperienceConfigLine(styles.TitleStyle, "Choose installation destination", width))
	}
	if wide {
		right := append([]string{"About: " + labels[field]}, strings.Split(ansi.Hardwrap(detail, width-leftWidth-1, true), "\n")...)
		for i := 0; i < min(height-2, max(len(left), len(right))); i++ {
			l, r := "", ""
			if i < len(left) {
				l = left[i]
			}
			if i < len(right) {
				r = right[i]
			}
			lines = append(lines, l+strings.Repeat(" ", max(0, leftWidth-ansi.StringWidth(l)))+"│"+r)
		}
	} else {
		lines = append(lines, left[field])
		if height >= 6 {
			wrapped := strings.Split(ansi.Hardwrap(detail, width, true), "\n")
			lines = append(lines, wrapped[:min(height-4, len(wrapped))]...)
		}
	}
	lines = append(lines, shellFlowFit(width, "Tab: field • Enter: review • Esc: cancel", "Tab field; Enter review; Esc cancel", "enter / esc", "esc"))
	return joinShellExperienceConfigLines(lines, height)
}

// shellFlowFit keeps the original responsive help ladder without inventing controls.
func shellFlowFit(width int, choices ...string) string {
	for _, text := range choices {
		if ansi.StringWidth(text) <= width {
			return text
		}
	}
	return ansi.Truncate(choices[len(choices)-1], width, "")
}
