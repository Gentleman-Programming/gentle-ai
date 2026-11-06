package screens

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/gentleman-programming/gentle-ai/v4/internal/tui/styles"
)

// ShellBloomFrames is the number of rose-opening frames on success.
var ShellBloomFrames = styles.RoseFrameCount()

// ShellInstallStep is one ordered installation step. State is pending,
// running, done or failed. It carries no installation authority.
type ShellInstallStep struct {
	Label   string
	State   string
	Details []string
}

var shellInstallSpinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// RenderShellInstall keeps the operator inside the TUI while the owned worker
// runs. It never claims success without a completed worker result.
// shellRoseFrameLines exposes the embedded rose frames to the installer
// screen. The frames are already styled by the shared palette.
func shellRoseFrameLines(frame int) []string {
	return strings.Split(styles.RenderRoseFrame(frame), "\n")
}

// ShellRoseDelay returns the dwell time for a frame.
func ShellRoseDelay(frame int) time.Duration {
	if frame <= 0 {
		return 0
	}
	return styles.RoseFrameDelay(frame)
}

func shellRoseCentered(line string, width int) string {
	pad := 0 // keep the rose aligned with the completion copy below
	if pad <= 0 {
		return line
	}
	return strings.Repeat(" ", pad) + line
}

// shellFailedSteps keeps a failed run from rendering as complete: a refusal
// reported after every step finished belongs to the final publish-and-verify
// step, which is then shown as failed.
func shellFailedSteps(steps []ShellInstallStep) []ShellInstallStep {
	if len(steps) == 0 {
		return steps
	}
	for _, step := range steps {
		if step.State != "done" {
			return steps
		}
	}
	steps = append([]ShellInstallStep(nil), steps...)
	steps[len(steps)-1].State = "failed"
	return steps
}

func RenderShellInstall(steps []ShellInstallStep, failure string, done bool, frame, bloom int, destinationPublished, fallbackAvailable bool, mode, launchCommand string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	title := "Installing Gentle-Shell"
	if done && failure == "" {
		title = "Installation complete"
	}
	if done && failure != "" {
		title = "Installation failed"
		steps = shellFailedSteps(steps)
	}
	completed := 0
	for _, step := range steps {
		if step.State == "done" {
			completed++
		}
	}
	barWidth := 12
	filled := 0
	if len(steps) > 0 {
		filled = barWidth * completed / len(steps)
	}
	bar := styles.ProgressFilled.Render(strings.Repeat("█", filled)) + styles.ProgressEmpty.Render(strings.Repeat("░", barWidth-filled))
	lines := []string{styles.TitleStyle.Render(fmt.Sprintf("%s  [%s] %d/%d", title, bar, completed, len(steps))), ""}
	labelWidth := 0
	for _, step := range steps {
		labelWidth = max(labelWidth, lipgloss.Width(step.Label))
	}
	current := ""
	for _, step := range steps {
		icon, state := "[ ]", "pending"
		iconStyle, stateStyle := styles.SubtextStyle, styles.SubtextStyle
		switch step.State {
		case "running":
			icon = "[" + shellInstallSpinner[frame%len(shellInstallSpinner)] + "]"
			state, iconStyle, stateStyle = "running", styles.WarningStyle, styles.WarningStyle
		case "done":
			icon, state, iconStyle, stateStyle = "[x]", "done", styles.SuccessStyle, styles.SuccessStyle
		case "failed":
			icon, state, iconStyle, stateStyle = "[!]", "failed", styles.ErrorStyle, styles.ErrorStyle
		}
		lines = append(lines, fmt.Sprintf("  %s %-*s  %s", iconStyle.Render(icon), labelWidth, step.Label, stateStyle.Render(state)))
		if step.State == "running" {
			for _, detail := range step.Details {
				lines = append(lines, "        › "+detail)
				current = detail
			}
		}
	}
	if failure != "" {
		lines = append(lines, "")
		switch {
		case !destinationPublished && mode == "separate":
			lines = append(lines, styles.WarningStyle.Render("No installation was applied: the process did not complete and no target was published."))
		case !destinationPublished:
			lines = append(lines, styles.WarningStyle.Render("No target was published. Agent/Shared side effects may need inspection before retrying."))
		default:
			lines = append(lines, styles.WarningStyle.Render("A destination exists; the transaction may be partial or uncertain. Inspect the preserved evidence."))
		}
		if fallbackAvailable {
			lines = append(lines, styles.WarningStyle.Render("Latest Main HEAD was not compatible. The last qualified Main snapshot is available."))
		}
		lines = append(lines, styles.ErrorStyle.Render("The installation stopped before publishing a verified result:"))
		for _, line := range strings.Split(failure, "\n") {
			for len(line) > width-2 && width > 2 {
				lines = append(lines, "  "+line[:width-2])
				line = line[width-2:]
			}
			lines = append(lines, "  "+line)
		}
		lines = append(lines, "")
		if fallbackAvailable {
			lines = append(lines, styles.HelpStyle.Render("Enter: try last qualified Main  •  esc: back to menu"))
		} else {
			lines = append(lines, styles.HelpStyle.Render("Enter: back to menu"))
		}
		return finishShellLines(lines, width, height)
	}
	if done {
		bloom = min(max(0, bloom), ShellBloomFrames-1)
		lines = append(lines, "  "+styles.SubtextStyle.Render(strings.Repeat("─", max(0, width-4))))
		roseWidth, roseHeight := styles.RoseSize()
		if width >= roseWidth+14 && height >= roseHeight+13 {
			for _, line := range shellRoseFrameLines(bloom) {
				lines = append(lines, shellRoseCentered(line, width))
			}
		} else {
			lines = append(lines, "        ✿")
		}
		if launchCommand == "" {
			launchCommand = "gentle-shell"
		}
		command := lipgloss.NewStyle().Foreground(styles.ColorPeach).Bold(true).Render(launchCommand)
		if bloom >= ShellBloomFrames-1 {
			start := styles.SubtextStyle.Render("To start using our agent, execute the ") + `"` + command + `"` + styles.SubtextStyle.Render(" command")
			lines = append(lines,
				"",
				styles.TitleStyle.Render("Welcome to the gentle-tailored experience"),
				start,
				"",
				styles.HelpStyle.Render("Enter: finish"),
			)
		}
		return finishShellLines(lines, width, height)
	}
	if current != "" {
		lines = append(lines, styles.TitleStyle.Render("CURRENT TASK"), "  "+current, "")
	}
	lines = append(lines, styles.HelpStyle.Render("The worker is running; do not close the terminal"))
	return finishShellLines(lines, width, height)
}

func finishShellLines(lines []string, width, height int) string {
	if len(lines) > height {
		lines = lines[:height]
	}
	for index, line := range lines {
		if lipgloss.Width(line) > width {
			lines[index] = lipgloss.NewStyle().MaxWidth(width).Render(line)
		}
	}
	return strings.Join(lines, "\n")
}
