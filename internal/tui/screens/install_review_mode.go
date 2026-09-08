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
	return []string{"RDD ON", "RDD OFF", "Back"}
}

// RenderInstallReviewMode explains the optional global RDD preference before the
// installer presents its final confirmation. The selection remains transient
// until the installation pipeline succeeds.
func RenderInstallReviewMode(status reviewtransaction.RDDModeStatus, err error, cursor int) string {
	var b strings.Builder
	b.WriteString(styles.TitleStyle.Render("Receipt-Driven Development") + "\n\n")
	b.WriteString(styles.SubtextStyle.Render("RDD provides bounded, independent review of a frozen change candidate, with recorded evidence and traceability.") + "\n")
	b.WriteString(styles.SubtextStyle.Render("It adds review evidence and a bounded correction process, with additional review time and potential model cost.") + "\n\n")
	b.WriteString(styles.HeadingStyle.Render("Choose RDD ON or RDD OFF for the global setting after this installation succeeds.") + "\n")
	b.WriteString(styles.SubtextStyle.Render("RDD is optional and defaults to OFF when no global preference is configured.") + "\n")
	b.WriteString(styles.SubtextStyle.Render("Existing clone-local overrides remain unchanged and can make a clone's effective mode differ from this global setting.") + "\n")
	b.WriteString(styles.SubtextStyle.Render("Review evidence does not authorize commits, pushes, pull requests, or releases; repository policy still governs delivery.") + "\n\n")

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
		return "RDD is currently ON globally. Choose explicitly to keep or change it after installation."
	case reviewtransaction.RDDModeOff:
		return "RDD is currently OFF globally. Choose explicitly to keep or change it after installation."
	default:
		return "No global RDD preference is configured. RDD defaults to OFF until you explicitly choose otherwise."
	}
}
