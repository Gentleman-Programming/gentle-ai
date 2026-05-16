package screens

import (
	"fmt"
	"path/filepath"
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

const EngramCustomPathDstIdx = -2

// EngramDirActionChoices builds the first-step management actions.
func EngramDirActionChoices() []EngramDirChoice {
	return []EngramDirChoice{
		{Label: "Migrate / Move Data", Op: model.EngramDataDirOpMove, DstIdx: -1},
		{Label: "Copy Data", Op: model.EngramDataDirOpCopy, DstIdx: -1},
		{Label: "Set Active Directory", Op: model.EngramDataDirOpSet, DstIdx: -1},
		{Label: "Delete current Data", Op: model.EngramDataDirOpDelete, DstIdx: -1},
	}
}

// EngramDirLocationChoices builds the second-step location list.
func EngramDirLocationChoices(locations []engram.Location, op model.EngramDataDirOp) []EngramDirChoice {
	var out []EngramDirChoice
	verb := "Use"
	if op == model.EngramDataDirOpMove {
		verb = "Move to"
	} else if op == model.EngramDataDirOpCopy {
		verb = "Copy to"
	} else if op == model.EngramDataDirOpSet {
		verb = "Use"
	}
	for i, loc := range locations {
		if loc.IsCurrent && op != model.EngramDataDirOpSet {
			continue
		}
		if op == model.EngramDataDirOpSet && !loc.IsCurrent && !loc.HasData {
			continue
		}
		label := verb + "  " + loc.Label
		if loc.IsCurrent {
			label += " " + styles.SuccessStyle.Render("(ACTIVE)")
		}
		out = append(out, EngramDirChoice{Label: label, Op: op, DstIdx: i})
	}
	if op == model.EngramDataDirOpSet {
		return out
	}
	out = append(out, EngramDirChoice{Label: "Type custom path…", Op: op, DstIdx: EngramCustomPathDstIdx})
	return out
}

// EngramDirInstallLocationChoices builds the install-time list for choosing
// where a fresh Engram data directory should live.
func EngramDirInstallLocationChoices(locations []engram.Location) []EngramDirChoice {
	out := make([]EngramDirChoice, 0, len(locations)+1)
	for i, loc := range locations {
		out = append(out, EngramDirChoice{
			Label:  "Use  " + loc.Label,
			Op:     model.EngramDataDirOpSet,
			DstIdx: i,
		})
	}
	out = append(out, EngramDirChoice{Label: "Type custom path…", Op: model.EngramDataDirOpSet, DstIdx: EngramCustomPathDstIdx})
	return out
}

// RenderEngramDataDir renders the main management screen shown from Welcome.
func RenderEngramDataDir(cursor int, currentDir string, dbSize int64, locations []engram.Location, selectedOp model.EngramDataDirOp, spaceErr string) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Manage Engram Data Directory"))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Current: " + engram.FormatDirLine(currentDir, dbSize)))
	b.WriteString("\n\n")
	if EngramDirOpNeedsLocation(selectedOp) {
		b.WriteString(styles.SubtextStyle.Render("Choose the destination for " + engramOpVerb(selectedOp) + "."))
		b.WriteString("\n\n")
	} else {
		b.WriteString(styles.SubtextStyle.Render("Choose what you want to do first. Destinations are shown on the next screen."))
		b.WriteString("\n\n")
	}
	if strings.TrimSpace(spaceErr) != "" {
		b.WriteString(styles.WarningStyle.Render(spaceErr))
		b.WriteString("\n\n")
	}

	choices := EngramDirActionChoices()
	if EngramDirOpNeedsLocation(selectedOp) {
		choices = EngramDirLocationChoices(locations, selectedOp)
	}
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
// shown before installing Engram.
func RenderEngramDataDirInstall(cursor int, detectedDir string, dbSize int64, locations []engram.Location, existingData bool, spaceErr string) string {
	var b strings.Builder

	if !existingData {
		b.WriteString(styles.TitleStyle.Render("Choose Engram Data Directory"))
		b.WriteString("\n\n")
		b.WriteString(styles.SubtextStyle.Render("No existing Engram data was found. Choose where new Engram data should live."))
		b.WriteString("\n\n")
		if strings.TrimSpace(spaceErr) != "" {
			b.WriteString(styles.WarningStyle.Render(spaceErr))
			b.WriteString("\n\n")
		}
		choices := EngramDirInstallLocationChoices(locations)
		labels := make([]string, len(choices))
		for i, c := range choices {
			labels[i] = c.Label
		}
		b.WriteString(renderOptions(labels, cursor))
		b.WriteString("\n")
		b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select • esc: go back"))
		return b.String()
	}

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

func RenderEngramDataDirCustomPath(op model.EngramDataDirOp, currentDir, path string, pos int, errMsg string) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Custom Engram Path"))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Action: " + engramOpVerb(op)))
	b.WriteString("\n")
	b.WriteString(styles.SubtextStyle.Render("Current: " + currentDir))
	b.WriteString("\n\n")
	b.WriteString("Path: " + renderPathInput(path, pos))
	b.WriteString("\n")
	if strings.TrimSpace(path) == "" {
		b.WriteString(styles.HelpStyle.Render(`Examples: D:\Engram • ~/Engram • /Volumes/Drive/Engram • /mnt/usb/Engram`))
		b.WriteString("\n")
	}
	if strings.TrimSpace(errMsg) != "" {
		b.WriteString("\n")
		b.WriteString(styles.WarningStyle.Render(errMsg))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("type/paste path • enter: continue • esc: go back"))

	return b.String()
}

// RenderEngramDataDirConfirm renders the confirmation screen before a
// destructive (move, delete, fresh) or non-trivial (copy) operation.
func RenderEngramDataDirConfirm(op model.EngramDataDirOp, currentDir, dstDir, backupRoot string, cursor int) string {
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
	case model.EngramDataDirOpSet:
		b.WriteString(fmt.Sprintf("  Active  %s\n", styles.SubtextStyle.Render(currentDir)))
		b.WriteString(fmt.Sprintf("      → %s\n", styles.SubtextStyle.Render(dstDir)))
	case model.EngramDataDirOpDelete:
		b.WriteString(fmt.Sprintf("  Delete  %s\n", styles.WarningStyle.Render(currentDir)))
		b.WriteString(styles.WarningStyle.Render("  All data files inside that directory will be removed."))
		b.WriteString("\n")
		writeBackupLocation(&b, backupRoot)
		b.WriteString(styles.SubtextStyle.Render("  The directory itself is left in place."))
		b.WriteString("\n")
	case model.EngramDataDirOpFresh:
		b.WriteString(fmt.Sprintf("  Delete  %s\n", styles.WarningStyle.Render(currentDir)))
		b.WriteString(styles.WarningStyle.Render("  All data files inside that directory will be removed."))
		b.WriteString("\n")
		writeBackupLocation(&b, backupRoot)
		b.WriteString(styles.SubtextStyle.Render("  The directory itself is left in place."))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	if op != model.EngramDataDirOpSet && op != model.EngramDataDirOpDelete && op != model.EngramDataDirOpFresh {
		writeBackupLocation(&b, backupRoot)
		b.WriteString("\n\n")
	}
	options := []string{"Confirm", "Cancel"}
	if op == model.EngramDataDirOpDelete || op == model.EngramDataDirOpFresh {
		options = []string{"Yes, delete data", "No, go back"}
	}
	b.WriteString(renderOptions(options, cursor))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("enter: confirm • esc: cancel"))

	return b.String()
}

func writeBackupLocation(b *strings.Builder, backupRoot string) {
	if strings.TrimSpace(backupRoot) == "" {
		b.WriteString(styles.SubtextStyle.Render("  A snapshot backup is created under Gentle AI backups first."))
		b.WriteString("\n")
		return
	}
	b.WriteString(styles.SubtextStyle.Render("  A snapshot backup is created first under:"))
	b.WriteString("\n")
	b.WriteString(styles.SubtextStyle.Render("  " + backupRoot))
	b.WriteString("\n")
}

func renderPathInput(path string, pos int) string {
	runes := []rune(path)
	if pos < 0 {
		pos = 0
	}
	if pos > len(runes) {
		pos = len(runes)
	}
	before := string(runes[:pos])
	after := string(runes[pos:])
	return styles.SelectedStyle.Render("[" + before + "▌" + after + "]")
}

// RenderEngramDataDirResult renders the success/error screen after an operation.
func RenderEngramDataDirResult(op model.EngramDataDirOp, snapshotID, backupRoot string, err error) string {
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
			b.WriteString(styles.SubtextStyle.Render(fmt.Sprintf("  Backup snapshot: %s", snapshotID)))
			if strings.TrimSpace(backupRoot) != "" {
				b.WriteString("\n")
				b.WriteString(styles.SubtextStyle.Render("  Location: " + filepath.Join(backupRoot, snapshotID)))
			}
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
	case model.EngramDataDirOpSet:
		return "Set active data directory"
	case model.EngramDataDirOpDelete:
		return "Delete data directory"
	case model.EngramDataDirOpFresh:
		return "Start fresh"
	default:
		return "Data directory operation"
	}
}

func EngramDirOpNeedsLocation(op model.EngramDataDirOp) bool {
	return op == model.EngramDataDirOpMove ||
		op == model.EngramDataDirOpCopy ||
		op == model.EngramDataDirOpSet
}
