package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
)

// EngramChoiceDefault means use the default Engram data location (~/.engram).
const EngramChoiceDefault = 0

// EngramChoiceMigrate means move existing data to a new location.
const EngramChoiceMigrate = 1

// EngramChoiceStartFresh means use a new location without migrating.
const EngramChoiceStartFresh = 2

// EngramDataDirRenderArgs holds all the state needed to render the Engram data
// directory configuration screen.
type EngramDataDirRenderArgs struct {
	CurrentDir       string
	HasExistingData  bool
	Choice           int // 0=default, 1=migrate, 2=start-fresh
	CustomPath       string
	Cursor           int
	InputPos         int
	ErrMsg           string
}

// EngramDataDirOptionCount returns the number of selectable rows on the Engram
// data directory screen, including the text-input row, Continue, and Back.
func EngramDataDirOptionCount(hasExistingData bool, choice int) int {
	count := 3 // default, start-fresh, continue
	if hasExistingData {
		count++ // migrate
	}
	if choice != EngramChoiceDefault {
		count++ // path-input
	}
	count++ // back
	return count
}

// EngramDataDirTextRow returns the row index of the text input field,
// or -1 when the text input is hidden (Choice == Default).
func EngramDataDirTextRow(hasExistingData bool, choice int) int {
	if choice == EngramChoiceDefault {
		return -1
	}
	if hasExistingData {
		return 3 // default, migrate, start-fresh, path-input
	}
	return 2 // default, start-fresh, path-input
}

// EngramDataDirContinueRow returns the row index of the Continue button.
func EngramDataDirContinueRow(hasExistingData bool, choice int) int {
	if choice == EngramChoiceDefault {
		if hasExistingData {
			return 3 // default, migrate, start-fresh, continue
		}
		return 2 // default, start-fresh, continue
	}
	if hasExistingData {
		return 4
	}
	return 3
}

// EngramDataDirChoiceFromCursor maps a cursor position to a choice constant,
// accounting for whether the Migrate option is present.
func EngramDataDirChoiceFromCursor(hasExistingData bool, cursor int) int {
	if cursor == 0 {
		return EngramChoiceDefault
	}
	if hasExistingData {
		if cursor == 1 {
			return EngramChoiceMigrate
		}
		return EngramChoiceStartFresh
	}
	return EngramChoiceStartFresh
}

// EngramDataDirCursorFromChoice maps a choice constant to the cursor position
// that selects it, accounting for whether the Migrate option is present.
func EngramDataDirCursorFromChoice(hasExistingData bool, choice int) int {
	if choice == EngramChoiceDefault {
		return 0
	}
	if hasExistingData {
		if choice == EngramChoiceMigrate {
			return 1
		}
		return 2
	}
	return 1
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

	// Row 0: Use default location
	rows = append(rows, engramRow{
		Label:      "Use default location (~/.engram)",
		IsRadio:    true,
		IsSelected: args.Choice == EngramChoiceDefault,
	})

	// Row 1: Migrate (conditional)
	if args.HasExistingData {
		rows = append(rows, engramRow{
			Label:      "Migrate existing data to a new location",
			IsRadio:    true,
			IsSelected: args.Choice == EngramChoiceMigrate,
		})
	}

	// Row 1 or 2: Start fresh
	rows = append(rows, engramRow{
		Label:      "Start fresh at a new location",
		IsRadio:    true,
		IsSelected: args.Choice == EngramChoiceStartFresh,
	})

	// Text input row (only shown when a custom location is chosen).
	if args.Choice != EngramChoiceDefault {
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
