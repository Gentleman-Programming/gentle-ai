package screens

import (
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/styles"
)

func ReviewModeOptions(status reviewtransaction.RDDModeStatus, err error) []string {
	if err != nil && status.Schema == "" {
		return []string{"Back"}
	}
	if status.Global == reviewtransaction.RDDModeOn {
		return []string{"Disable globally", "Back"}
	}
	return []string{"Enable globally", "Back"}
}

func RenderReviewMode(status reviewtransaction.RDDModeStatus, err error, cursor int) string {
	var b strings.Builder
	b.WriteString(styles.TitleStyle.Render("Receipt-Driven Development") + "\n\n")
	b.WriteString(styles.SubtextStyle.Render("RDD runs a bounded review before delivery and records review evidence.") + "\n")
	b.WriteString(styles.SubtextStyle.Render("Delivery remains governed by repository policy, including required checks and branch protection.") + "\n\n")
	if status.Schema != "" {
		state := "RDD is currently DISABLED globally."
		if status.Global == reviewtransaction.RDDModeOn {
			state = "RDD is currently ENABLED globally."
		}
		b.WriteString(styles.HeadingStyle.Render(state) + "\n")
		b.WriteString(styles.SubtextStyle.Render("Individual clones can override this global setting.") + "\n\n")
		question := "Do you want to enable RDD globally?"
		if status.Global == reviewtransaction.RDDModeOn {
			question = "Do you want to disable RDD globally?"
		}
		b.WriteString("\n" + styles.SubtextStyle.Render(question) + "\n\n")
	}
	if err != nil {
		b.WriteString(styles.ErrorStyle.Render("✗ Could not load or update review mode") + "\n\n")
		b.WriteString(styles.ErrorStyle.Render("  "+err.Error()) + "\n\n")
	}
	b.WriteString(renderOptions(ReviewModeOptions(status, err), cursor) + "\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select • esc: back"))
	return b.String()
}
