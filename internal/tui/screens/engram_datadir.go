package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/components/engram"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
)

// EngramDirChoice is one entry in the data-directory management option list.
type EngramDirChoice struct {
	Label  string
	Op     model.EngramDataDirOp
	DstIdx int // index into the locations slice; -1 for Keep and Delete
}

// EngramDirChoices builds the ordered option list for ScreenEngramDataDir.
// Exported so the tui Model can map cursor → (op, dstIdx) without duplicating
// the construction logic.
func EngramDirChoices(locations []engram.Location) []EngramDirChoice {
	out := []EngramDirChoice{
		{Label: "Keep current location", Op: model.EngramDataDirOpKeep, DstIdx: -1},
	}
	for i, loc := range locations {
		if loc.IsCurrent {
			continue
		}
		out = append(out,
			EngramDirChoice{Label: "Copy to  " + loc.Label, Op: model.EngramDataDirOpCopy, DstIdx: i},
			EngramDirChoice{Label: "Move to  " + loc.Label, Op: model.EngramDataDirOpMove, DstIdx: i},
		)
	}
	out = append(out, EngramDirChoice{Label: "Delete current data", Op: model.EngramDataDirOpDelete, DstIdx: -1})
	return out
}

// RenderEngramDataDir renders the main management screen shown from Welcome.
func RenderEngramDataDir(cursor int, currentDir string, dbSize int64, locations []engram.Location, spaceErr string) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Manage Engram Data Directory"))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Current: " + engram.FormatDirLine(currentDir, dbSize)))
	b.WriteString("\n\n")
	if strings.TrimSpace(spaceErr) != "" {
		b.WriteString(styles.WarningStyle.Render(spaceErr))
		b.WriteString("\n\n")
	}

	choices := EngramDirChoices(locations)
	labels := make([]string, len(choices))
	for i, c := range choices {
		labels[i] = c.Label
	}
	b.WriteString(renderOptions(labels, cursor))
	b.WriteString("\n")
	b.WriteString(styles.WarningStyle.Render("⚠  Ensure engram is not running before copying or moving data."))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select • esc: back"))

	return b.String()
}

// RenderEngramDataDirInstall renders the install-time data directory screen
// shown when an existing data directory is detected.
func RenderEngramDataDirInstall(cursor int, detectedDir string, dbSize int64) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Existing Engram Data Found"))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Detected: " + engram.FormatDirLine(detectedDir, dbSize)))
	b.WriteString("\n\n")

	opts := []string{
		"Keep location  (use data as-is)",
		"Start fresh    (snapshot existing, then delete)",
	}
	b.WriteString(renderOptions(opts, cursor))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select • esc: back"))

	return b.String()
}

// RenderEngramDataDirConfirm renders the confirmation screen before a
// destructive (move, delete, fresh) or non-trivial (copy) operation.
func RenderEngramDataDirConfirm(op model.EngramDataDirOp, currentDir, dstDir string, cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Confirm — " + engramOpVerb(op)))
	b.WriteString("\n\n")

	switch op {
	case model.EngramDataDirOpCopy:
		b.WriteString(fmt.Sprintf("  Copy  %s\n", styles.SubtextStyle.Render(currentDir)))
		b.WriteString(fmt.Sprintf("    → %s\n", styles.SubtextStyle.Render(dstDir)))
	case model.EngramDataDirOpMove:
		b.WriteString(fmt.Sprintf("  Move  %s\n", styles.SubtextStyle.Render(currentDir)))
		b.WriteString(fmt.Sprintf("    → %s\n", styles.SubtextStyle.Render(dstDir)))
		b.WriteString(styles.WarningStyle.Render("  Source will be removed after successful copy."))
		b.WriteString("\n")
	case model.EngramDataDirOpDelete:
		b.WriteString(fmt.Sprintf("  Delete  %s\n", styles.WarningStyle.Render(currentDir)))
		b.WriteString(styles.WarningStyle.Render("  A snapshot backup will be created first."))
		b.WriteString("\n")
	case model.EngramDataDirOpFresh:
		b.WriteString(fmt.Sprintf("  Delete  %s\n", styles.WarningStyle.Render(currentDir)))
		b.WriteString(styles.WarningStyle.Render("  A snapshot backup will be created first."))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.SubtextStyle.Render("A snapshot backup is created before any change."))
	b.WriteString("\n\n")
	b.WriteString(renderOptions([]string{"Confirm", "Cancel"}, cursor))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("enter: confirm • esc: cancel"))

	return b.String()
}

// RenderEngramDataDirResult renders the success/error screen after an operation.
func RenderEngramDataDirResult(op model.EngramDataDirOp, snapshotID string, err error) string {
	var b strings.Builder

	if err != nil {
		b.WriteString(styles.ErrorStyle.Render("✗ " + engramOpVerb(op) + " failed"))
		b.WriteString("\n\n")
		b.WriteString(styles.WarningStyle.Render("  " + err.Error()))
		b.WriteString("\n\n")
		b.WriteString(styles.SubtextStyle.Render("  Source data was not modified."))
	} else {
		b.WriteString(styles.SuccessStyle.Render("✓ " + engramOpVerb(op) + " complete"))
		if snapshotID != "" {
			b.WriteString("\n\n")
			b.WriteString(styles.SubtextStyle.Render(fmt.Sprintf("  Snapshot: %s", snapshotID)))
		}
	}

	b.WriteString("\n\n")
	b.WriteString(styles.HelpStyle.Render("any key: back"))

	return b.String()
}

func engramOpVerb(op model.EngramDataDirOp) string {
	switch op {
	case model.EngramDataDirOpCopy:
		return "Copy data directory"
	case model.EngramDataDirOpMove:
		return "Move data directory"
	case model.EngramDataDirOpDelete:
		return "Delete data directory"
	case model.EngramDataDirOpFresh:
		return "Start fresh"
	default:
		return "Data directory operation"
	}
}
