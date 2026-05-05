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

// EngramChoiceCopy means copy existing data to another location without changing the current location.
const EngramChoiceCopy = 2

// EngramChoiceSetActive means point Engram at an existing data directory.
const EngramChoiceSetActive = 3

// EngramChoiceStartFresh means delete existing data and use a new location.
const EngramChoiceStartFresh = 4

// EngramDataDirRenderArgs holds all the state needed to render the Engram data
// directory configuration screen.
type EngramDataDirRenderArgs struct {
	CurrentDir      string
	HasExistingData bool
	Choice          int // 0=default, 1=migrate, 2=copy, 3=set-active, 4=start-fresh
	CustomPath      string
	Cursor          int
	InputPos        int
	ErrMsg          string

	// Preview info shown when a custom path is selected.
	FilesToMove    []string // list of files that will be moved, copied, or deleted
	TotalBytes     uint64   // total size of files
	TargetSpace    uint64   // available space at target path (0 = unknown)
	TargetSpaceErr string   // error getting target space ("" = ok)

	// DefaultDirSpace is shown on first-time install to reassure the user
	// there is enough space at the default location.
	DefaultDirSpace uint64

	// SuggestedLocations appears when the user needs to pick a custom path
	// (Migrate / Copy / StartFresh). It shows common destinations with available space.
	SuggestedLocations []engram.LocationSuggestion

	// PartialWarning is shown if an interrupted migration is detected.
	PartialWarning string
}

// EngramDataDirOptionCount returns the number of selectable rows on the main
// Engram data directory decision screen. The path input and suggestions live on
// a separate picker phase, so this screen stays focused on the primary action.
func EngramDataDirOptionCount(hasExistingData bool, choice int) int {
	return len(EngramDataDirChoices(hasExistingData)) + 1 // choices + Back
}

// EngramDataDirChoices returns the primary choices for the data-directory flow.
func EngramDataDirChoices(hasExistingData bool) []int {
	if !hasExistingData {
		return []int{EngramChoiceSetActive, EngramChoiceStartFresh}
	}
	return []int{EngramChoiceMigrate, EngramChoiceCopy, EngramChoiceSetActive, EngramChoiceStartFresh}
}

// EngramDataDirBackRow returns the row index of the Back option.
func EngramDataDirBackRow(hasExistingData bool) int {
	return len(EngramDataDirChoices(hasExistingData))
}

// EngramPathPickerOptionCount returns selectable rows in the path picker:
// text input, suggestions, and Back.
func EngramPathPickerOptionCount(suggestions []engram.LocationSuggestion) int {
	return len(suggestions) + 2
}

// EngramPathPickerBackRow returns the row index of Back in the path picker.
func EngramPathPickerBackRow(suggestions []engram.LocationSuggestion) int {
	return len(suggestions) + 1
}

// EngramDataDirChoiceFromCursor maps a cursor position to a choice constant.
func EngramDataDirChoiceFromCursor(hasExistingData bool, cursor int) int {
	choices := EngramDataDirChoices(hasExistingData)
	if cursor >= 0 && cursor < len(choices) {
		return choices[cursor]
	}
	return EngramChoiceFromFirstAvailable(hasExistingData)
}

func EngramChoiceFromFirstAvailable(hasExistingData bool) int {
	choices := EngramDataDirChoices(hasExistingData)
	if len(choices) == 0 {
		return EngramChoiceDefault
	}
	return choices[0]
}

// EngramDataDirCursorFromChoice maps a choice constant to the cursor position.
func EngramDataDirCursorFromChoice(hasExistingData bool, choice int) int {
	for idx, candidate := range EngramDataDirChoices(hasExistingData) {
		if candidate == choice {
			return idx
		}
	}
	return 0
}

// needsPathInput returns true when the given choice requires a custom path.
func NeedsPathInput(choice int) bool {
	return choice == EngramChoiceMigrate || choice == EngramChoiceCopy || choice == EngramChoiceSetActive || choice == EngramChoiceStartFresh
}

// RenderEngramDataDir renders the main Engram data directory decision screen.
func RenderEngramDataDir(args EngramDataDirRenderArgs) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("ENGRAM DATA DIRECTORY"))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Choose the high-level action for Engram persistent memory."))
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

	if !args.HasExistingData {
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
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select • esc: back"))

	if args.ErrMsg != "" {
		b.WriteString("\n")
		b.WriteString(styles.ErrorStyle.Render(args.ErrMsg))
	}

	return styles.FrameStyle.Render(b.String())
}

// EngramPathPickerRenderArgs holds state for choosing a custom Engram path.
type EngramPathPickerRenderArgs struct {
	Choice             int
	CustomPath         string
	Cursor             int
	InputPos           int
	ErrMsg             string
	SuggestedLocations []engram.LocationSuggestion
	FilesToMove        []string
	TotalBytes         uint64
	TargetSpace        uint64
	TargetSpaceErr     string
}

// RenderEngramPathPicker renders the second-step keyboard path picker.
func RenderEngramPathPicker(args EngramPathPickerRenderArgs) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("ENGRAM DATA LOCATION"))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Paste or type an Engram folder path. Windows, macOS, and Linux paths are supported."))
	b.WriteString("\n\n")

	b.WriteString(renderEngramRow(engramRow{Label: "Path:", IsInput: true, InputText: args.CustomPath}, args.Cursor == 0, args.InputPos))
	if args.CustomPath == "" {
		b.WriteString(styles.HelpStyle.Render(`  Paste or type a full directory path, for example C:\engram, ~/engram, /Volumes/Drive/engram, or /mnt/usb/engram.`))
		b.WriteString("\n")
	}

	if len(args.SuggestedLocations) > 0 {
		b.WriteString("\n")
		b.WriteString(styles.LabelStyle.Render("Suggested locations:"))
		b.WriteString("\n")
		for idx, loc := range args.SuggestedLocations {
			line := loc.Label + " — " + loc.Path
			if loc.Space > 0 {
				line += " (" + engram.FormatBytes(loc.Space) + " available)"
			}
			b.WriteString(renderEngramLine("  "+line, args.Cursor == idx+1, false))
			b.WriteString("\n")
		}
	}

	backRow := EngramPathPickerBackRow(args.SuggestedLocations)
	b.WriteString(renderEngramLine("  Back", args.Cursor == backRow, false))
	b.WriteString("\n\n")

	if len(args.FilesToMove) > 0 {
		b.WriteString(styles.LabelStyle.Render(engramFilesHeading(args.Choice)))
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

	if args.Choice == EngramChoiceCopy {
		b.WriteString("\n")
		b.WriteString(styles.SubtextStyle.Render("Original data will stay at the current location. This creates a copy only."))
		b.WriteString("\n")
	}

	if args.Choice == EngramChoiceStartFresh {
		b.WriteString("\n")
		b.WriteString(styles.WarningStyle.Render("Warning: existing data will be deleted after confirmation. A new empty database will be created at this location."))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("type path • j/k: suggestions • enter: use location • esc: back"))

	if args.ErrMsg != "" {
		b.WriteString("\n")
		b.WriteString(styles.ErrorStyle.Render(args.ErrMsg))
	}

	return styles.FrameStyle.Render(b.String())
}

func engramFilesHeading(choice int) string {
	switch choice {
	case EngramChoiceMigrate:
		return "Files that will be moved:"
	case EngramChoiceCopy:
		return "Files that will be copied:"
	case EngramChoiceSetActive:
		return "Files at selected active directory:"
	case EngramChoiceStartFresh:
		return "Files that will be deleted:"
	default:
		return "Files that will be affected:"
	}
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
		rows = append(rows, engramRow{Label: "Set active Engram directory", IsRadio: true})
		rows = append(rows, engramRow{Label: "Start fresh at a new location", IsRadio: true})
	} else {
		rows = append(rows, engramRow{Label: "Move existing data to a new location", IsRadio: true})
		rows = append(rows, engramRow{Label: "Copy existing data to another location", IsRadio: true})
		rows = append(rows, engramRow{Label: "Set active Engram directory", IsRadio: true})
		rows = append(rows, engramRow{Label: "Start fresh at a new location", IsRadio: true})
	}

	rows = append(rows, engramRow{Label: "Back / keep current location"})
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
	Choice         int
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
		b.WriteString(styles.LabelStyle.Render(engramFilesHeading(args.Choice)))
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
			b.WriteString(styles.SelectedOptionStyle.Render("> " + label))
		} else {
			b.WriteString(styles.OptionStyle.Render("  " + label))
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
	b.WriteString(styles.SelectedOptionStyle.Render("> Continue"))
	b.WriteString("\n")

	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("enter: continue"))

	return styles.FrameStyle.Render(b.String())
}
