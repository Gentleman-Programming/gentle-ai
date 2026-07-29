package screens

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/styles"
)

func DetectionOptions() []string {
	return []string{"Continue", "Back"}
}

func RenderDetection(result system.DetectionResult, cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("System Detection"))
	b.WriteString("\n\n")

	supportedText := styles.ErrorStyle.Render("No")
	if result.System.Supported {
		supportedText = styles.SuccessStyle.Render("Yes")
	}

	shellName := filepath.Base(result.System.Shell)

	b.WriteString(fmt.Sprintf("  %s  %s\n", styles.HeadingStyle.Render("OS"), styles.UnselectedStyle.Render(fmt.Sprintf("%s (%s)", result.System.OS, result.System.Arch))))
	b.WriteString(fmt.Sprintf("  %s  %s\n", styles.HeadingStyle.Render("Shell"), styles.UnselectedStyle.Render(shellName)))
	b.WriteString(fmt.Sprintf("  %s  %s\n", styles.HeadingStyle.Render("Supported"), supportedText))
	b.WriteString("\n")

	if len(result.Tools) > 0 {
		b.WriteString(styles.HeadingStyle.Render("Tools"))
		b.WriteString("\n")
		keys := make([]string, 0, len(result.Tools))
		for key := range result.Tools {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			status := result.Tools[key]
			indicator := styles.ErrorStyle.Render("not found")
			if status.Installed {
				indicator = styles.SuccessStyle.Render("found")
			}
			b.WriteString(fmt.Sprintf("  %s: %s\n", styles.UnselectedStyle.Render(key), indicator))
		}
		b.WriteString("\n")
	}

	if len(result.Dependencies.Dependencies) > 0 {
		b.WriteString(styles.HeadingStyle.Render("Dependencies"))
		b.WriteString("\n")
		for _, dep := range result.Dependencies.Dependencies {
			var indicator string
			if dep.Installed {
				version := dep.Version
				if version == "" {
					version = "found"
				}
				indicator = styles.SuccessStyle.Render(version)
			} else {
				label := "not found"
				if dep.Required {
					label = "NOT FOUND (required)"
				}
				indicator = styles.ErrorStyle.Render(label)
			}

			suffix := ""
			if !dep.Required {
				suffix = styles.SubtextStyle.Render(" (optional)")
			}

			b.WriteString(fmt.Sprintf("  %s: %s%s\n",
				styles.UnselectedStyle.Render(dep.Name), indicator, suffix))
		}

		if len(result.Dependencies.MissingRequired) > 0 {
			b.WriteString("\n")
			b.WriteString(styles.WarningStyle.Render(
				fmt.Sprintf("Missing required: %s",
					strings.Join(result.Dependencies.MissingRequired, ", "))))
			b.WriteString("\n")
		}

		b.WriteString("\n")
	}

	if len(result.Configs) > 0 {
		b.WriteString(styles.HeadingStyle.Render("Detected Configs"))
		b.WriteString("\n")
		for _, config := range result.Configs {
			indicator := styles.ErrorStyle.Render("missing")
			if config.Exists {
				indicator = styles.SuccessStyle.Render("present")
			}
			b.WriteString(fmt.Sprintf("  %s: %s\n", styles.UnselectedStyle.Render(config.Agent), indicator))
		}
		b.WriteString("\n")
	}

	b.WriteString(renderOptions(DetectionOptions(), cursor))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select • esc: back"))

	return b.String()
}
