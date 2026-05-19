package screens

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/components/engram"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
)

const EngramCustomPathDstIdx = -2

// EngramDirChoice is one entry in an Engram data-directory option list.
type EngramDirChoice struct {
	Label  string
	Op     model.EngramDataDirOp
	DstIdx int
}

func EngramDirActionChoices() []EngramDirChoice {
	return []EngramDirChoice{
		{Label: "Migrate / Move Data", Op: model.EngramDataDirOpMove, DstIdx: -1},
		{Label: "Copy Data", Op: model.EngramDataDirOpCopy, DstIdx: -1},
		{Label: "Set Active Directory", Op: model.EngramDataDirOpSet, DstIdx: -1},
		{Label: "Delete current Data", Op: model.EngramDataDirOpDelete, DstIdx: -1},
	}
}

func EngramDirLocationChoices(locations []engram.Location, op model.EngramDataDirOp) []EngramDirChoice {
	verb := "Use"
	if op == model.EngramDataDirOpMove {
		verb = "Move to"
	}
	if op == model.EngramDataDirOpCopy {
		verb = "Copy to"
	}

	var out []EngramDirChoice
	for i, loc := range locations {
		if loc.IsCurrent && op != model.EngramDataDirOpSet {
			continue
		}
		label := verb + "  " + loc.Label
		if loc.IsCurrent {
			label += " " + styles.SuccessStyle.Render("(ACTIVE)")
		}
		out = append(out, EngramDirChoice{Label: label, Op: op, DstIdx: i})
	}
	if op != model.EngramDataDirOpSet {
		out = append(out, EngramDirChoice{Label: "Type custom path...", Op: op, DstIdx: EngramCustomPathDstIdx})
	}
	return out
}

func EngramDirInstallLocationChoices(locations []engram.Location) []EngramDirChoice {
	out := make([]EngramDirChoice, 0, len(locations)+1)
	for i, loc := range locations {
		out = append(out, EngramDirChoice{Label: "Use  " + loc.Label, Op: model.EngramDataDirOpSet, DstIdx: i})
	}
	out = append(out, EngramDirChoice{Label: "Type custom path...", Op: model.EngramDataDirOpSet, DstIdx: EngramCustomPathDstIdx})
	return out
}

func RenderEngramDataDir(cursor int, currentDir string, dbSize int64, locations []engram.Location, selectedOp model.EngramDataDirOp, spaceErr string) string {
	var b strings.Builder
	b.WriteString(styles.TitleStyle.Render("Manage Engram Data Directory"))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Current: " + engram.FormatDirLine(currentDir, dbSize)))
	b.WriteString("\n\n")
	if strings.TrimSpace(spaceErr) != "" {
		b.WriteString(styles.WarningStyle.Render(spaceErr))
		b.WriteString("\n\n")
	}

	choices := EngramDirActionChoices()
	if EngramDirOpNeedsLocation(selectedOp) {
		b.WriteString(styles.SubtextStyle.Render("Choose the destination for " + engramOpVerb(selectedOp) + "."))
		b.WriteString("\n\n")
		choices = EngramDirLocationChoices(locations, selectedOp)
	}
	labels := make([]string, len(choices))
	for i, choice := range choices {
		labels[i] = choice.Label
	}
	b.WriteString(renderOptions(labels, cursor))
	b.WriteString("\n")
	b.WriteString(styles.WarningStyle.Render("Ensure engram is not running before copying or moving data."))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select • esc: back"))
	return b.String()
}

func RenderEngramDataDirInstall(cursor int, detectedDir string, dbSize int64, locations []engram.Location, existingData bool, spaceErr string) string {
	var b strings.Builder
	if existingData {
		b.WriteString(styles.TitleStyle.Render("Existing Engram Data Found"))
		b.WriteString("\n\n")
		b.WriteString(styles.SubtextStyle.Render("Detected: " + engram.FormatDirLine(detectedDir, dbSize)))
		b.WriteString("\n\n")
		b.WriteString(renderOptions([]string{"Keep location  (use data as-is)", "Start fresh    (snapshot existing, then delete)"}, cursor))
		return b.String()
	}

	b.WriteString(styles.TitleStyle.Render("Choose Engram Data Directory"))
	b.WriteString("\n\n")
	if strings.TrimSpace(spaceErr) != "" {
		b.WriteString(styles.WarningStyle.Render(spaceErr))
		b.WriteString("\n\n")
	}
	choices := EngramDirInstallLocationChoices(locations)
	labels := make([]string, len(choices))
	for i, choice := range choices {
		labels[i] = choice.Label
	}
	b.WriteString(renderOptions(labels, cursor))
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
	if strings.TrimSpace(errMsg) != "" {
		b.WriteString("\n\n")
		b.WriteString(styles.WarningStyle.Render(errMsg))
	}
	b.WriteString("\n\n")
	b.WriteString(styles.HelpStyle.Render("type path • enter: continue • esc: back"))
	return b.String()
}

func RenderEngramDataDirConfirm(op model.EngramDataDirOp, currentDir, dstDir, backupRoot string, cursor int) string {
	var b strings.Builder
	b.WriteString(styles.TitleStyle.Render("Confirm - " + engramOpVerb(op)))
	b.WriteString("\n\n")
	switch op {
	case model.EngramDataDirOpCopy, model.EngramDataDirOpMove, model.EngramDataDirOpSet:
		b.WriteString(fmt.Sprintf("  %s\n    -> %s\n", styles.SubtextStyle.Render(currentDir), styles.SubtextStyle.Render(dstDir)))
	case model.EngramDataDirOpDelete, model.EngramDataDirOpFresh:
		b.WriteString(fmt.Sprintf("  Delete  %s\n", styles.WarningStyle.Render(currentDir)))
	}
	if strings.TrimSpace(backupRoot) != "" && op != model.EngramDataDirOpSet {
		b.WriteString("\n")
		b.WriteString(styles.SubtextStyle.Render("Backup: " + backupRoot))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(renderOptions([]string{"Confirm", "Cancel"}, cursor))
	return b.String()
}

func RenderEngramDataDirResult(op model.EngramDataDirOp, snapshotID, backupRoot string, err error) string {
	if err != nil {
		return styles.ErrorStyle.Render(engramOpVerb(op)+" failed") + "\n\n" + styles.WarningStyle.Render(err.Error())
	}
	var b strings.Builder
	b.WriteString(styles.SuccessStyle.Render(engramOpVerb(op) + " complete"))
	if snapshotID != "" {
		b.WriteString("\n\n")
		b.WriteString(styles.SubtextStyle.Render("Backup snapshot: " + snapshotID))
		if strings.TrimSpace(backupRoot) != "" {
			b.WriteString("\n")
			b.WriteString(styles.SubtextStyle.Render("Location: " + filepath.Join(backupRoot, snapshotID)))
		}
	}
	return b.String()
}

func renderPathInput(path string, pos int) string {
	runes := []rune(path)
	if pos < 0 {
		pos = 0
	}
	if pos > len(runes) {
		pos = len(runes)
	}
	return styles.SelectedStyle.Render("[" + string(runes[:pos]) + "|" + string(runes[pos:]) + "]")
}

func EngramDirOpNeedsLocation(op model.EngramDataDirOp) bool {
	return op == model.EngramDataDirOpMove || op == model.EngramDataDirOpCopy || op == model.EngramDataDirOpSet
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
