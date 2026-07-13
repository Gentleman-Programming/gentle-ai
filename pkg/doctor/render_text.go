package doctor

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
)

// TextRenderer outputs the report as a colored table.
type TextRenderer struct {
	writer  io.Writer
	options RenderOptions
	styles  textStyles
	verbose bool
}

// textStyles holds lipgloss styles for text output.
type textStyles struct {
	passed   lipgloss.Style
	warn     lipgloss.Style
	fail     lipgloss.Style
	info     lipgloss.Style
	skip     lipgloss.Style
	header   lipgloss.Style
	category lipgloss.Style
	summary  lipgloss.Style
	help     lipgloss.Style
}

// NewTextRenderer creates a new text renderer.
func NewTextRenderer(w io.Writer, opts RenderOptions) *TextRenderer {
	t := &TextRenderer{
		writer:  w,
		options: opts,
		verbose: opts.Verbose,
	}
	t.initStyles()
	return t
}

func (t *TextRenderer) initStyles() {
	if t.options.Color {
		t.styles.passed = lipgloss.NewStyle().Foreground(styles.ColorGreen).Bold(true)
		t.styles.warn = lipgloss.NewStyle().Foreground(styles.ColorYellow).Bold(true)
		t.styles.fail = lipgloss.NewStyle().Foreground(styles.ColorRed).Bold(true)
		t.styles.info = lipgloss.NewStyle().Foreground(styles.ColorBlue).Bold(true)
		t.styles.skip = lipgloss.NewStyle().Foreground(styles.ColorOverlay)
		t.styles.header = lipgloss.NewStyle().Foreground(styles.ColorLavender).Bold(true)
		t.styles.category = lipgloss.NewStyle().Foreground(styles.ColorMauve).Bold(true)
		t.styles.summary = lipgloss.NewStyle().Foreground(styles.ColorText)
		t.styles.help = lipgloss.NewStyle().Foreground(styles.ColorSubtext)
	} else {
		t.styles.passed = lipgloss.NewStyle()
		t.styles.warn = lipgloss.NewStyle()
		t.styles.fail = lipgloss.NewStyle()
		t.styles.info = lipgloss.NewStyle()
		t.styles.skip = lipgloss.NewStyle()
		t.styles.header = lipgloss.NewStyle()
		t.styles.category = lipgloss.NewStyle()
		t.styles.summary = lipgloss.NewStyle()
		t.styles.help = lipgloss.NewStyle()
	}
}

// Render outputs the report as a colored table.
func (t *TextRenderer) Render(report DoctorReport) error {
	var b strings.Builder

	// Header
	b.WriteString(t.styles.header.Render("gentle-ai doctor — system health check"))
	b.WriteString("\n\n")

	// Group by category
	categories := []Category{
		CategoryHardware,
		CategorySoftware,
		CategoryConfig,
	}

	for _, cat := range categories {
		catResults := filterByCategory(report.Results, cat)
		if len(catResults) == 0 {
			continue
		}

		// Category header
		catName := strings.Title(string(cat))
		b.WriteString(t.styles.category.Render(fmt.Sprintf("  %s:", catName)))
		b.WriteString("\n")

		for _, result := range catResults {
			if !t.shouldShow(result) {
				continue
			}

			statusIcon := t.statusIcon(result.Status)
			checkName := result.Name
			summary := result.Summary

			b.WriteString(fmt.Sprintf("  %s  %-38s %s\n", statusIcon, checkName, summary))

		if t.verbose && result.Detail != "" {
			b.WriteString(fmt.Sprintf("  %s\n", indentLines(result.Detail, "  ")))
		}

			if result.Remediation != nil && result.Remediation.Description != "" {
				b.WriteString(fmt.Sprintf("       %s %s\n",
					t.styles.help.Render("→"),
					result.Remediation.Description))
				if len(result.Remediation.ManualSteps) > 0 {
					for _, step := range result.Remediation.ManualSteps {
						b.WriteString(fmt.Sprintf("         - %s\n", step))
					}
				}
				if len(result.Remediation.Links) > 0 {
					for _, link := range result.Remediation.Links {
						b.WriteString(fmt.Sprintf("         %s %s\n",
							t.styles.help.Render("🔗"), link))
					}
				}
			}

			// Show fix command when FixMode is enabled and check failed
			if t.options.FixMode && result.Status == StatusFail && report.FixRegistry != nil {
				fix, ok := report.FixRegistry.GetFixes(report.GOOS, result.Name)
				if ok {
					b.WriteString(fmt.Sprintf("       %s %s\n",
						t.styles.help.Render("fix:"),
						fix.Command))
					if fix.RequiresSudo {
						b.WriteString(fmt.Sprintf("         %s\n",
							t.styles.warn.Render("(requires sudo)")))
					}
					if len(fix.Alternatives) > 0 {
						b.WriteString(fmt.Sprintf("         %s\n",
							t.styles.help.Render("alternatives: "+strings.Join(fix.Alternatives, " | "))))
					}
				}
			}
		}
		b.WriteString("\n")
	}

	// Summary bar
	b.WriteString(t.renderSummaryBar(report.Summary))
	b.WriteString("\n")

	_, err := t.writer.Write([]byte(b.String()))
	return err
}

func (t *TextRenderer) statusIcon(status Status) string {
	switch status {
	case StatusPass:
		return t.styles.passed.Render("✓")
	case StatusWarn:
		return t.styles.warn.Render("⚠")
	case StatusFail:
		return t.styles.fail.Render("✗")
	case StatusInfo:
		return t.styles.info.Render("ℹ")
	case StatusSkip:
		return t.styles.skip.Render("○")
	default:
		return "?"
	}
}

func (t *TextRenderer) shouldShow(result CheckResult) bool {
	if t.verbose {
		return true
	}
	switch result.Status {
	case StatusPass:
		return t.options.ShowPassed
	case StatusSkip:
		return t.options.ShowSkipped
	default:
		return true
	}
}

func (t *TextRenderer) renderSummaryBar(summary Summary) string {
	var b strings.Builder

	passStyle := t.styles.passed
	warnStyle := t.styles.warn
	failStyle := t.styles.fail
	infoStyle := t.styles.info
	skipStyle := t.styles.skip

	b.WriteString("Summary: ")
	b.WriteString(passStyle.Render(fmt.Sprintf("%d passed", summary.Pass)))
	b.WriteString(", ")
	b.WriteString(failStyle.Render(fmt.Sprintf("%d failed", summary.Fail)))
	b.WriteString(", ")
	b.WriteString(warnStyle.Render(fmt.Sprintf("%d warnings", summary.Warn)))
	b.WriteString(", ")
	b.WriteString(infoStyle.Render(fmt.Sprintf("%d info", summary.Info)))
	b.WriteString(", ")
	b.WriteString(skipStyle.Render(fmt.Sprintf("%d skipped", summary.Skip)))

	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("Duration: %v  ", summary.Duration.Round(time.Millisecond)))

	status := "healthy"
	statusStyle := passStyle
	if summary.Fail > 0 {
		status = "unhealthy"
		statusStyle = failStyle
	} else if summary.Warn > 0 {
		status = "degraded"
		statusStyle = warnStyle
	}
	b.WriteString("Status: ")
	b.WriteString(statusStyle.Render(status))

	return b.String()
}

func filterByCategory(results []CheckResult, cat Category) []CheckResult {
	var filtered []CheckResult
	for _, r := range results {
		if r.Category == cat {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

func indentLines(s, indent string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = indent + line
	}
	return strings.Join(lines, "\n")
}