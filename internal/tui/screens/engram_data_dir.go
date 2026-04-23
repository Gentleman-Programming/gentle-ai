package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
)

// EngramChoiceKeep means keep the current Engram data location.
const EngramChoiceKeep = 0

// EngramChoiceStartFresh means use a new location without migrating.
const EngramChoiceStartFresh = 1

// EngramChoiceMigrate means move existing data to the new location.
const EngramChoiceMigrate = 2

// EngramDataDirRenderArgs holds all the state needed to render the Engram data
// directory configuration screen.
type EngramDataDirRenderArgs struct {
	CurrentDir       string
	HasExistingData  bool
	Choice           int // 0=keep, 1=start-fresh, 2=migrate
	CustomPath       string
	Cursor           int
	InputPos         int
	ErrMsg           string
}

// EngramDataDirOptionCount returns the number of selectable rows on the Engram
// data directory screen, including the text-input row, Continue, and Back.
func EngramDataDirOptionCount(hasExistingData bool, choice int) int {
	count := 3 // keep, start-fresh, continue
	if hasExistingData {
		count++ // migrate
	}
	if choice != EngramChoiceKeep {
		count++ // path-input
	}
	count++ // back
	return count
}

// EngramDataDirTextRow returns the row index of the text input field,
// or -1 when the text input is hidden (Choice == Keep).
func EngramDataDirTextRow(hasExistingData bool, choice int) int {
	if choice == EngramChoiceKeep {
		return -1
	}
	if hasExistingData {
		return 3 // keep, start-fresh, migrate, path-input
	}
	return 2 // keep, start-fresh, path-input
}

// EngramDataDirContinueRow returns the row index of the Continue button.
func EngramDataDirContinueRow(hasExistingData bool, choice int) int {
	if choice == EngramChoiceKeep {
		if hasExistingData {
			return 3 // keep, start-fresh, migrate, continue
		}
		return 2 // keep, start-fresh, continue
	}
	if hasExistingData {
		return 4
	}
	return 3
}

// RenderEngramDataDir renders the Engram data directory selection screen.
func RenderEngramDataDir(args EngramDataDirRenderArgs) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("ENGRAM DATA DIRECTORY"))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Choose where Engram stores its persistent memory database."))
	b.WriteString("\n\n")

	b.WriteString(styles.LabelStyle.Render("Current location: "))
	b.WriteString(styles.ValueStyle.Render(args.CurrentDir))
	b.WriteString("\n")

	if args.HasExistingData {
		b.WriteString(styles.WarningStyle.Render("Existing Engram data detected at this location."))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	rows := buildEngramDataDirRows(args)
	for idx, row := range rows {
		focused := idx == args.Cursor
		b.WriteString(renderEngramRow(row, focused, args.InputPos))
	}

	b.WriteString("\n")
	if args.HasExistingData && args.Choice == EngramChoiceStartFresh {
		b.WriteString(styles.WarningStyle.Render("Warning: existing data will remain at the old location but will no longer be used."))
		b.WriteString("\n")
	}
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select/continue • esc: back"))

	if args.ErrMsg != "" {
		b.WriteString("\n")
		b.WriteString(styles.ErrorStyle.Render(args.ErrMsg))
	}

	return styles.FrameStyle.Render(b.String())
}

type engramRow struct {
	Label      string
	IsRadio    bool
	IsSelected bool
	IsInput    bool
	InputText  string
}

func buildEngramDataDirRows(args EngramDataDirRenderArgs) []engramRow {
	var rows []engramRow

	// Row 0: Keep current
	rows = append(rows, engramRow{
		Label:      "Keep current location",
		IsRadio:    true,
		IsSelected: args.Choice == 0,
	})

	// Row 1: Start fresh
	rows = append(rows, engramRow{
		Label:      "Start fresh at a new location",
		IsRadio:    true,
		IsSelected: args.Choice == 1,
	})

	// Row 2: Migrate (conditional)
	if args.HasExistingData {
		rows = append(rows, engramRow{
			Label:      "Migrate existing data to a new location",
			IsRadio:    true,
			IsSelected: args.Choice == 2,
		})
	}

	// Text input row (only shown when a custom location is chosen).
	if args.Choice != EngramChoiceKeep {
		path := args.CustomPath
		if path == "" {
			path = " "
		}
		rows = append(rows, engramRow{
			Label:     "Path:",
			IsInput:   true,
			InputText: path,
		})
	}

	// Continue
	rows = append(rows, engramRow{
		Label: "Continue",
	})

	// Back
	rows = append(rows, engramRow{
		Label: "Back",
	})

	return rows
}

func renderEngramRow(row engramRow, focused bool, inputPos int) string {
	var prefix string
	if row.IsRadio {
		if row.IsSelected {
			prefix = "● "
		} else {
			prefix = "○ "
		}
	} else if row.IsInput {
		prefix = "  "
	} else {
		prefix = "  "
	}

	content := prefix + row.Label
	if row.IsInput {
		content = prefix + row.Label + " " + renderEngramTextInput(row.InputText, focused, inputPos)
		return renderEngramLine(content, focused, false) + "\n"
	}

	return renderEngramLine(content, focused, row.IsSelected) + "\n"
}

func renderEngramLine(content string, focused, selected bool) string {
	if focused {
		return styles.SelectedOptionStyle.Render("> " + content)
	}
	if selected {
		return styles.SelectedOptionStyle.Render("  " + content)
	}
	return styles.OptionStyle.Render("  " + content)
}

func renderEngramTextInput(text string, focused bool, cursorPos int) string {
	if !focused {
		return styles.DimStyle.Render("[" + text + "]")
	}

	runes := []rune(text)
	if cursorPos > len(runes) {
		cursorPos = len(runes)
	}
	if cursorPos < 0 {
		cursorPos = 0
	}

	before := string(runes[:cursorPos])
	after := ""
	if cursorPos < len(runes) {
		after = string(runes[cursorPos+1:])
	}

	var cursorChar string
	if cursorPos < len(runes) {
		cursorChar = string(runes[cursorPos])
	} else {
		cursorChar = " "
	}

	return fmt.Sprintf("[%s%s%s]", before, styles.CursorStyle.Render(cursorChar), after)
}
