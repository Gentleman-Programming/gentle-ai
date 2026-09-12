package screens

import (
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/styles"
)

func InstallReviewModeOptions(err error) []string {
	if err != nil {
		return []string{"Back"}
	}
	return []string{"Enable RDD", "Disable RDD", "Back"}
}

// RenderInstallReviewMode explains the optional global RDD preference before the
// installer presents its final confirmation. The selection remains transient
// until the installation pipeline succeeds.
func RenderInstallReviewMode(status reviewtransaction.RDDModeStatus, err error, cursor int) string {
	var b strings.Builder
	b.WriteString(styles.TitleStyle.Render("Receipt-Driven Development") + "\n\n")
	b.WriteString(styles.SubtextStyle.Render("RDD adds an independent review of your code changes to help catch bugs and regressions before they reach your project.") + "\n")
	b.WriteString(styles.SubtextStyle.Render("It records review findings and verifies corrections, helping you understand what was checked and build confidence in your changes.") + "\n\n")
	b.WriteString(styles.HeadingStyle.Render("Would you like to enable RDD?") + "\n")
	b.WriteString(styles.SubtextStyle.Render("Your choice applies globally after installation succeeds. Existing project-specific settings are preserved.") + "\n\n")

	if err != nil {
		b.WriteString(styles.ErrorStyle.Render("Could not read the configured global RDD mode. No choice will be assumed or saved.") + "\n")
		b.WriteString(styles.ErrorStyle.Render("  "+err.Error()) + "\n\n")
	} else if status.Schema != "" {
		b.WriteString(styles.SubtextStyle.Render(installReviewModeStatusLabel(status)) + "\n\n")
	} else {
		b.WriteString(styles.SubtextStyle.Render("Loading the configured global RDD mode...") + "\n\n")
	}

	b.WriteString(renderOptions(InstallReviewModeOptions(err), cursor) + "\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: choose • esc: back"))
	return b.String()
}

func installReviewModeStatusLabel(status reviewtransaction.RDDModeStatus) string {
	switch status.Global {
	case reviewtransaction.RDDModeOn:
		return "RDD is currently ON."
	case reviewtransaction.RDDModeOff:
		return "RDD is currently OFF."
	default:
		return "No global RDD preference is configured. RDD defaults to OFF until you explicitly choose otherwise."
	}
}
