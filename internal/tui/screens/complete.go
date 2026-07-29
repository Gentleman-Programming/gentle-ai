package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/styles"
)

const maxErrorLines = 15

type FailedStep struct {
	ID    string
	Error string
}

type MissingDep struct {
	Name        string
	InstallHint string
}

// UpdateInfo holds version update information for a single tool.
type UpdateInfo struct {
	Name             string
	InstalledVersion string
	LatestVersion    string
	UpdateHint       string
}

type CompletePayload struct {
	ConfiguredAgents    int
	InstalledComponents int
	GGAInstalled        bool
	FailedSteps         []FailedStep
	RollbackPerformed   bool
	RollbackComplete    bool
	MissingDeps         []MissingDep
	AvailableUpdates    []UpdateInfo
	ManualActions       []string
}

func RenderComplete(data CompletePayload) string {
	if len(data.FailedSteps) > 0 {
		return renderCompleteFailed(data)
	}
	return renderCompleteSuccess(data)
}

func renderCompleteSuccess(data CompletePayload) string {
	var b strings.Builder

	b.WriteString(styles.SuccessStyle.Render("Done! Your AI agents are ready."))
	b.WriteString("\n\n")

	b.WriteString("  " + styles.HeadingStyle.Render("Configured agents") + "  " + styles.SuccessStyle.Render(fmt.Sprintf("%d", data.ConfiguredAgents)) + "\n")
	b.WriteString("  " + styles.HeadingStyle.Render("Installed components") + "  " + styles.SuccessStyle.Render(fmt.Sprintf("%d", data.InstalledComponents)) + "\n")
	b.WriteString("\n")

	renderMissingDeps(&b, data.MissingDeps)
	renderAvailableUpdates(&b, data.AvailableUpdates)
	renderManualActions(&b, data.ManualActions)

	b.WriteString(styles.HeadingStyle.Render("Next steps"))
	b.WriteString("\n")
	b.WriteString(styles.UnselectedStyle.Render("  1. Set your API keys"))
	b.WriteString("\n")
	b.WriteString(styles.UnselectedStyle.Render("  2. Run your selected agent"))
	b.WriteString("\n")
	b.WriteString(styles.UnselectedStyle.Render("  3. Try /sdd-new my-feature"))
	b.WriteString("\n\n")

	if data.GGAInstalled {
		b.WriteString(styles.HeadingStyle.Render("GGA (per project)"))
		b.WriteString("\n")
		b.WriteString(styles.UnselectedStyle.Render("  GGA was installed globally."))
		b.WriteString("\n")
		b.WriteString(styles.UnselectedStyle.Render("  In each repo run: gga init"))
		b.WriteString("\n")
		b.WriteString(styles.UnselectedStyle.Render("  Then run: gga install"))
		b.WriteString("\n\n")
	}

	b.WriteString(styles.HelpStyle.Render("Press Enter to exit."))

	return b.String()
}

func renderManualActions(b *strings.Builder, actions []string) {
	if len(actions) == 0 {
		return
	}
	b.WriteString(styles.WarningStyle.Render("Manual actions required"))
	b.WriteString("\n")
	for _, action := range actions {
		b.WriteString("  " + styles.WarningStyle.Render(action) + "\n")
	}
	b.WriteString("\n")
}

func renderMissingDeps(b *strings.Builder, deps []MissingDep) {
	if len(deps) == 0 {
		return
	}

	b.WriteString(styles.WarningStyle.Render(fmt.Sprintf("Missing %d dependency(ies):", len(deps))))
	b.WriteString("\n")
	for _, dep := range deps {
		b.WriteString("  " + styles.WarningStyle.Render(dep.Name) + "  " + styles.SubtextStyle.Render(dep.InstallHint))
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

func renderAvailableUpdates(b *strings.Builder, updates []UpdateInfo) {
	if len(updates) == 0 {
		return
	}

	b.WriteString(styles.HeadingStyle.Render("Available Updates"))
	b.WriteString("\n")
	for _, u := range updates {
		line := fmt.Sprintf("  %s %s -> %s", u.Name, u.InstalledVersion, u.LatestVersion)
		b.WriteString(styles.WarningStyle.Render(line))
		if u.UpdateHint != "" {
			b.WriteString("  " + styles.SubtextStyle.Render(u.UpdateHint))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

func renderCompleteFailed(data CompletePayload) string {
	var b strings.Builder

	b.WriteString(styles.ErrorStyle.Render("Installation completed with errors."))
	b.WriteString("\n\n")

	b.WriteString(styles.HeadingStyle.Render("Failed steps"))
	b.WriteString("\n")
	for _, step := range data.FailedSteps {
		b.WriteString("  " + styles.ErrorStyle.Render("✗ "+step.ID))
		b.WriteString("\n")
		lines := strings.Split(step.Error, "\n")
		if len(lines) > maxErrorLines {
			lines = lines[:maxErrorLines]
			lines = append(lines, "... (truncated)")
		}
		for _, line := range lines {
			b.WriteString("    " + styles.SubtextStyle.Render(line))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")

	if data.RollbackPerformed {
		message := "Rollback completed — previous configuration restored."
		if !data.RollbackComplete {
			message = "Rollback partially completed — manual recovery may be required."
		}
		b.WriteString(styles.WarningStyle.Render(message))
		b.WriteString("\n\n")
	}

	renderManualActions(&b, data.ManualActions)
	renderMissingDeps(&b, data.MissingDeps)
	renderAvailableUpdates(&b, data.AvailableUpdates)

	b.WriteString(styles.HeadingStyle.Render("What to do"))
	b.WriteString("\n")
	b.WriteString(styles.UnselectedStyle.Render("  1. Check the error messages above"))
	b.WriteString("\n")
	b.WriteString(styles.UnselectedStyle.Render("  2. Fix the underlying issue (missing deps, permissions, etc.)"))
	b.WriteString("\n")
	b.WriteString(styles.UnselectedStyle.Render("  3. Run gentle-ai again to retry"))
	b.WriteString("\n\n")

	b.WriteString(styles.HelpStyle.Render("Press Enter to exit."))

	return b.String()
}
