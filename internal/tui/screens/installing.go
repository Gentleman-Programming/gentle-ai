package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/styles"
)

type ProgressItem struct {
	Label  string
	Status string
}

type InstallProgress struct {
	Percent     int
	CurrentStep string
	Items       []ProgressItem
	Logs        []string
	Done        bool
	Failed      bool
}

func RenderInstalling(progress InstallProgress, spinner string) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Installing"))
	b.WriteString("\n\n")
	b.WriteString(renderBar(progress.Percent))
	b.WriteString(" ")
	b.WriteString(styles.PercentStyle.Render(fmt.Sprintf("%d%%", progress.Percent)))
	b.WriteString("\n")

	if progress.CurrentStep != "" {
		b.WriteString(styles.SubtextStyle.Render("Current: " + progress.CurrentStep))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	for _, item := range progress.Items {
		var icon string
		switch item.Status {
		case "succeeded":
			icon = styles.SuccessStyle.Render("✓")
		case "failed":
			icon = styles.ErrorStyle.Render("✗")
		case "running":
			icon = styles.WarningStyle.Render(spinner)
		default:
			icon = styles.SubtextStyle.Render("·")
		}
		b.WriteString(fmt.Sprintf("  %s %s\n", icon, styles.UnselectedStyle.Render(item.Label)))
	}

	if len(progress.Logs) > 0 {
		b.WriteString("\n")
		b.WriteString(styles.HeadingStyle.Render("Logs"))
		b.WriteString("\n")
		start := 0
		if len(progress.Logs) > 5 {
			start = len(progress.Logs) - 5
		}
		for _, entry := range progress.Logs[start:] {
			for _, line := range strings.Split(entry, "\n") {
				b.WriteString(styles.SubtextStyle.Render("  " + line))
				b.WriteString("\n")
			}
		}
	}

	if progress.Done {
		b.WriteString("\n")
		succeeded, failed := countResults(progress.Items)
		if progress.Failed {
			b.WriteString(styles.WarningStyle.Render(fmt.Sprintf("Completed with errors: %d succeeded, %d failed", succeeded, failed)))
			b.WriteString("\n")
		} else {
			b.WriteString(styles.SuccessStyle.Render(fmt.Sprintf("All %d steps completed successfully.", succeeded)))
			b.WriteString("\n")
		}
		b.WriteString(styles.HelpStyle.Render("Press Enter to continue."))
		b.WriteString("\n")
	}

	return b.String()
}

func countResults(items []ProgressItem) (succeeded, failed int) {
	for _, item := range items {
		switch item.Status {
		case "succeeded":
			succeeded++
		case "failed":
			failed++
		}
	}
	return
}

func renderBar(percent int) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	const width = 30
	filled := (percent * width) / 100
	empty := width - filled

	return styles.ProgressFilled.Render(strings.Repeat("█", filled)) +
		styles.ProgressEmpty.Render(strings.Repeat("░", empty))
}
