package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/components/engram"
	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
)

// EngramChoiceDefault means keep data at the default/current location.
const EngramChoiceDefault = 0

// EngramChoiceMigrate means move existing data to a new location.
const EngramChoiceMigrate = 1

// EngramChoiceStartFresh means delete existing data and use a new location.
const EngramChoiceStartFresh = 2

// EngramChoiceClean means delete existing data but keep the current location.
const EngramChoiceClean = 3

// EngramDataDirRenderArgs holds all the state needed to render the Engram data
// directory configuration screen.
type EngramDataDirRenderArgs struct {
	CurrentDir      string
	HasExistingData bool
	Choice          int // 0=default, 1=migrate, 2=start-fresh, 3=clean
	CustomPath      string
	Cursor          int
	InputPos        int
	ErrMsg          string

	// Preview info shown when a custom path is selected.
	FilesToMove    []string // list of files that will be affected
	TotalBytes     uint64   // total size of files
	TargetSpace    uint64   // available space at target path (0 = unknown)
	TargetSpaceErr string   // error getting target space ("" = ok)

	// DefaultDirSpace is shown on first-time install to reassure the user
	// there is enough space at the default location.
	DefaultDirSpace uint64

	// SuggestedLocations appears when the user needs to pick a custom path
	// (Migrate / StartFresh). It shows common destinations with available space.
	SuggestedLocations []engram.LocationSuggestion

	// PartialWarning is shown if an interrupted migration is detected.
	PartialWarning string
}

// EngramDataDirOptionCount returns the number of selectable rows on the Engram
// data directory screen, including the text-input row, Continue, and Back.
func EngramDataDirOptionCount(hasExistingData bool, choice int) int {
	if !hasExistingData {
		if choice == EngramChoiceDefault {
			// default, custom, continue, back
			return 4
		}
		// default, custom, path-input, continue, back
		return 5
	}
	// default, migrate, start-fresh, clean, continue, back
	count := 6
	if choice == EngramChoiceMigrate || choice == EngramChoiceStartFresh {
		count++ // path-input
	}
	return count
}

// EngramDataDirTextRow returns the row index of the text input field,
// or -1 when the text input is hidden.
func EngramDataDirTextRow(hasExistingData bool, choice int) int {
	if !hasExistingData {
		if choice == EngramChoiceDefault {
			return -1
		}
		return 2 // default, custom, path-input
	}
	// hasExistingData
	switch choice {
	case EngramChoiceDefault, EngramChoiceClean:
		return -1
	case EngramChoiceMigrate:
		return 2 // default, migrate, path-input
	case EngramChoiceStartFresh:
		return 3 // default, migrate, start-fresh, path-input
	}
	return -1
}

// EngramDataDirContinueRow returns the row index of the Continue button.
func EngramDataDirContinueRow(hasExistingData bool, choice int) int {
	if !hasExistingData {
		if choice == EngramChoiceDefault {
			return 2 // default, custom, continue
		}
		return 3 // default, custom, path-input, continue
	}
	// hasExistingData
	switch choice {
	case EngramChoiceDefault:
		return 4 // default, migrate, start-fresh, clean, continue
	case EngramChoiceMigrate:
		return 3 // default, migrate, path-input, continue
	case EngramChoiceStartFresh:
		return 4 // default, migrate, start-fresh, path-input, continue
	case EngramChoiceClean:
		return 4 // default, migrate, start-fresh, clean, continue
	}
	return 3
}

// EngramDataDirChoiceFromCursor maps a cursor position to a choice constant.
func EngramDataDirChoiceFromCursor(hasExistingData bool, cursor int) int {
	if !hasExistingData {
		switch cursor {
		case 0:
			return EngramChoiceDefault
		case 1:
			return EngramChoiceStartFresh // custom location
		}
		return EngramChoiceDefault
	}
	// hasExistingData
	switch cursor {
	case 0:
		return EngramChoiceDefault
	case 1:
		return EngramChoiceMigrate
	case 2:
		return EngramChoiceStartFresh
	case 3:
		return EngramChoiceClean
	}
	return EngramChoiceDefault
}

// EngramDataDirCursorFromChoice maps a choice constant to the cursor position.
func EngramDataDirCursorFromChoice(hasExistingData bool, choice int) int {
	if !hasExistingData {
		switch choice {
		case EngramChoiceDefault:
			return 0
		case EngramChoiceStartFresh:
			return 1
		}
		return 0
	}
	// hasExistingData
	switch choice {
	case EngramChoiceDefault:
		return 0
	case EngramChoiceMigrate:
		return 1
	case EngramChoiceStartFresh:
		return 2
	case EngramChoiceClean:
		return 3
	}
	return 0
}

// needsPathInput returns true when the given choice requires a custom path.
func NeedsPathInput(choice int) bool {
	return choice == EngramChoiceMigrate || choice == EngramChoiceStartFresh
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

	if args.PartialWarning != "" {
		b.WriteString(styles.WarningStyle.Render(args.PartialWarning))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// First-time install: show the default path prominently when the user
	// hasn't chosen a custom location yet.
	if !args.HasExistingData && args.Choice == EngramChoiceDefault {
		b.WriteString(styles.LabelStyle.Render("Engram will store your memories at:"))
		b.WriteString("\n")
		b.WriteString("  " + styles.ValueStyle.Render(args.CurrentDir))
		b.WriteString("\n")
		if args.DefaultDirSpace > 0 {
			b.WriteString("  " + styles.SubtextStyle.Render("Available space: "+engram.FormatBytes(args.DefaultDirSpace)))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	rows := buildEngramDataDirRows(args)
	for idx, row := range rows {
		focused := idx == args.Cursor
		b.WriteString(renderEngramRow(row, focused, args.InputPos))
	}

	b.WriteString("\n")

	// Show suggested locations when a custom path is needed.
	if NeedsPathInput(args.Choice) && len(args.SuggestedLocations) > 0 {
		b.WriteString(styles.LabelStyle.Render("Suggested locations:"))
		b.WriteString("\n")
		for _, loc := range args.SuggestedLocations {
			line := "  • " + loc.Label
			if loc.Space > 0 {
				line += " — " + engram.FormatBytes(loc.Space) + " available"
			}
			b.WriteString(styles.DimStyle.Render(line))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Show preview of what will happen for custom-path choices.
	if NeedsPathInput(args.Choice) && len(args.FilesToMove) > 0 {
		b.WriteString(styles.LabelStyle.Render("Files that will be affected:"))
		b.WriteString("\n")
		for _, f := range args.FilesToMove {
			b.WriteString(styles.DimStyle.Render("  • " + f))
			b.WriteString("\n")
		}
		b.WriteString(styles.LabelStyle.Render("Total size: "))
		b.WriteString(styles.ValueStyle.Render(engram.FormatBytes(args.TotalBytes)))
		b.WriteString("\n")

		if args.TargetSpaceErr != "" {
			b.WriteString(styles.ErrorStyle.Render("Cannot check target space: " + args.TargetSpaceErr))
			b.WriteString("\n")
		} else if args.TargetSpace > 0 {
			b.WriteString(styles.LabelStyle.Render("Available at target: "))
			b.WriteString(styles.ValueStyle.Render(engram.FormatBytes(args.TargetSpace)))
			b.WriteString("\n")
		}

		b.WriteString("\n")
		if args.Choice == EngramChoiceMigrate {
			b.WriteString(styles.SubtextStyle.Render("The files above will be copied to the new location, verified, then removed from the original location."))
		} else {
			b.WriteString(styles.SubtextStyle.Render("All existing data above will be permanently deleted, then a new empty database will be created at the new location."))
		}
		b.WriteString("\n")
	}

	// Show contextual warning for destructive options.
	if args.HasExistingData {
		switch args.Choice {
		case EngramChoiceStartFresh:
			b.WriteString(styles.WarningStyle.Render("Warning: existing data will be deleted. A new empty database will be created at the path you specify."))
			b.WriteString("\n")
		case EngramChoiceClean:
			b.WriteString(styles.WarningStyle.Render("Warning: all existing Engram data will be permanently deleted. This cannot be undone."))
			b.WriteString("\n")
		}
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

	if !args.HasExistingData {
		// No existing data — simple install flow.
		rows = append(rows, engramRow{
			Label:      "Continue with this location",
			IsRadio:    true,
			IsSelected: args.Choice == EngramChoiceDefault,
		})
		rows = append(rows, engramRow{
			Label:      "Choose a different location",
			IsRadio:    true,
			IsSelected: args.Choice == EngramChoiceStartFresh,
		})
	} else {
		// Existing data found — show all options.
		rows = append(rows, engramRow{
			Label:      "Keep data at current location",
			IsRadio:    true,
			IsSelected: args.Choice == EngramChoiceDefault,
		})
		rows = append(rows, engramRow{
			Label:      "Migrate data to a new location",
			IsRadio:    true,
			IsSelected: args.Choice == EngramChoiceMigrate,
		})
		rows = append(rows, engramRow{
			Label:      "Delete data and start fresh at a new location",
			IsRadio:    true,
			IsSelected: args.Choice == EngramChoiceStartFresh,
		})
		rows = append(rows, engramRow{
			Label:      "Clean/reset data (keep at current location)",
			IsRadio:    true,
			IsSelected: args.Choice == EngramChoiceClean,
		})
	}

	// Text input row (only for choices that need a custom path).
	if NeedsPathInput(args.Choice) {
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

// EngramConfirmRenderArgs holds state for the confirmation screen.
type EngramConfirmRenderArgs struct {
	Title          string
	Message        string
	Warning        string // optional warning line
	Cursor         int    // 0=Confirm, 1=Cancel
	ErrMsg         string
	FilesToMove    []string // list of files that will be affected
	TotalBytes     uint64   // total size
	TargetSpace    uint64   // available space at target
	TargetSpaceErr string
}

// EngramConfirmOptionCount returns the number of selectable rows.
func EngramConfirmOptionCount() int {
	return 2 // Confirm, Cancel
}

// RenderEngramConfirm renders the confirmation dialog.
func RenderEngramConfirm(args EngramConfirmRenderArgs) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render(args.Title))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render(args.Message))
	b.WriteString("\n")

	if len(args.FilesToMove) > 0 {
		b.WriteString("\n")
		b.WriteString(styles.LabelStyle.Render("Files that will be affected:"))
		b.WriteString("\n")
		for _, f := range args.FilesToMove {
			b.WriteString(styles.DimStyle.Render("  • " + f))
			b.WriteString("\n")
		}
		b.WriteString(styles.LabelStyle.Render("Total size: "))
		b.WriteString(styles.ValueStyle.Render(engram.FormatBytes(args.TotalBytes)))
		b.WriteString("\n")
	}

	if args.TargetSpaceErr != "" {
		b.WriteString(styles.ErrorStyle.Render("Cannot check target space: " + args.TargetSpaceErr))
		b.WriteString("\n")
	} else if args.TargetSpace > 0 {
		b.WriteString(styles.LabelStyle.Render("Available at target: "))
		b.WriteString(styles.ValueStyle.Render(engram.FormatBytes(args.TargetSpace)))
		b.WriteString("\n")
	}

	if args.Warning != "" {
		b.WriteString("\n")
		b.WriteString(styles.WarningStyle.Render(args.Warning))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	for i, label := range []string{"Confirm", "Cancel"} {
		focused := i == args.Cursor
		if focused {
			b.WriteString(styles.SelectedOptionStyle.Render("> ● " + label))
		} else {
			b.WriteString(styles.OptionStyle.Render("  ○ " + label))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select • esc: cancel"))

	if args.ErrMsg != "" {
		b.WriteString("\n")
		b.WriteString(styles.ErrorStyle.Render(args.ErrMsg))
	}

	return styles.FrameStyle.Render(b.String())
}

// EngramFeedbackRenderArgs holds state for the feedback screen.
type EngramFeedbackRenderArgs struct {
	Title   string
	Message string
	Details string // optional detail lines (e.g. "2 files moved, 1.2 MB")
	Cursor  int    // always 0 (Continue)
}

// EngramFeedbackOptionCount returns the number of selectable rows.
func EngramFeedbackOptionCount() int {
	return 1 // Continue
}

// RenderEngramFeedback renders the success feedback screen.
func RenderEngramFeedback(args EngramFeedbackRenderArgs) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render(args.Title))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render(args.Message))
	b.WriteString("\n")

	if args.Details != "" {
		b.WriteString("\n")
		b.WriteString(styles.ValueStyle.Render(args.Details))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.SelectedOptionStyle.Render("> ● Continue"))
	b.WriteString("\n")

	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("enter: continue"))

	return styles.FrameStyle.Render(b.String())
}
