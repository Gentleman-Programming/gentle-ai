package screens

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/components/engram"
)

func TestRenderEngramDataDir_WithData(t *testing.T) {
	args := EngramDataDirRenderArgs{
		CurrentDir:      "/home/user/.engram",
		HasExistingData: true,
		Choice:          EngramChoiceDefault,
		CustomPath:      "",
		Cursor:          0,
		InputPos:        0,
		ErrMsg:          "",
	}
	out := RenderEngramDataDir(args)
	if !strings.Contains(out, "ENGRAM DATA DIRECTORY") {
		t.Error("missing title")
	}
	if !strings.Contains(out, "/home/user/.engram") {
		t.Error("missing current dir")
	}
	if !strings.Contains(out, "Existing Engram data detected") {
		t.Error("missing existing data warning")
	}
	if !strings.Contains(out, "Keep data at current location") {
		t.Error("missing default option label")
	}
	if !strings.Contains(out, "Migrate data to a new location") {
		t.Error("missing migrate option")
	}
	if !strings.Contains(out, "Delete data and start fresh") {
		t.Error("missing start fresh option")
	}
	if !strings.Contains(out, "Clean/reset data") {
		t.Error("missing clean option")
	}
}

func TestRenderEngramDataDir_WithoutData_DefaultChoice(t *testing.T) {
	args := EngramDataDirRenderArgs{
		CurrentDir:      "/home/user/.engram",
		HasExistingData: false,
		Choice:          EngramChoiceDefault,
		CustomPath:      "",
		Cursor:          0,
		InputPos:        0,
		ErrMsg:          "",
		DefaultDirSpace: 45 * 1024 * 1024 * 1024, // 45 GB
	}
	out := RenderEngramDataDir(args)
	if !strings.Contains(out, "Engram will store your memories at") {
		t.Error("missing install location heading")
	}
	if !strings.Contains(out, "/home/user/.engram") {
		t.Error("missing current dir")
	}
	if !strings.Contains(out, "45.0 GB") {
		t.Error("missing available space")
	}
	if !strings.Contains(out, "Continue with this location") {
		t.Error("missing continue option")
	}
	if !strings.Contains(out, "Choose a different location") {
		t.Error("missing choose different option")
	}
	// Should NOT show migrate/clean options when no data
	if strings.Contains(out, "Migrate") {
		t.Error("should not show migrate option without data")
	}
}

func TestRenderEngramDataDir_WithoutData_CustomChoiceHidesHeading(t *testing.T) {
	args := EngramDataDirRenderArgs{
		CurrentDir:      "/home/user/.engram",
		HasExistingData: false,
		Choice:          EngramChoiceStartFresh,
		CustomPath:      "",
		Cursor:          0,
		InputPos:        0,
		ErrMsg:          "",
		DefaultDirSpace: 45 * 1024 * 1024 * 1024,
	}
	out := RenderEngramDataDir(args)
	if strings.Contains(out, "Engram will store your memories at") {
		t.Error("heading should be hidden when user chooses a different location")
	}
}

func TestRenderEngramDataDir_CustomPath(t *testing.T) {
	args := EngramDataDirRenderArgs{
		CurrentDir:      "/home/user/.engram",
		HasExistingData: false,
		Choice:          EngramChoiceStartFresh,
		CustomPath:      "/data/engram",
		Cursor:          EngramDataDirTextRow(false, EngramChoiceStartFresh),
		InputPos:        5,
		ErrMsg:          "",
	}
	out := RenderEngramDataDir(args)
	if !strings.Contains(out, "/data/engram") {
		t.Error("missing custom path")
	}
}

func TestRenderEngramDataDir_SuggestedLocations(t *testing.T) {
	args := EngramDataDirRenderArgs{
		CurrentDir:      "/home/user/.engram",
		HasExistingData: true,
		Choice:          EngramChoiceMigrate,
		CustomPath:      "",
		Cursor:          0,
		SuggestedLocations: []engram.LocationSuggestion{
			{Label: "Home", Path: "/home/user/.engram", Space: 45 * 1024 * 1024 * 1024},
			{Label: "External", Path: "/mnt/external/engram", Space: 120 * 1024 * 1024 * 1024},
		},
	}
	out := RenderEngramDataDir(args)
	if !strings.Contains(out, "Suggested locations:") {
		t.Error("missing suggested locations heading")
	}
	if !strings.Contains(out, "Home") {
		t.Error("missing Home suggestion")
	}
	if !strings.Contains(out, "External") {
		t.Error("missing External suggestion")
	}
	if !strings.Contains(out, "45.0 GB") {
		t.Error("missing Home space")
	}
	if !strings.Contains(out, "120.0 GB") {
		t.Error("missing External space")
	}
}

func TestRenderEngramDataDir_NoSuggestedLocationsWhenPathNotNeeded(t *testing.T) {
	args := EngramDataDirRenderArgs{
		CurrentDir:         "/home/user/.engram",
		HasExistingData:    true,
		Choice:             EngramChoiceDefault,
		SuggestedLocations: []engram.LocationSuggestion{{Label: "Home", Path: "/home/user/.engram"}},
	}
	out := RenderEngramDataDir(args)
	if strings.Contains(out, "Suggested locations:") {
		t.Error("should not show suggested locations when path input is not needed")
	}
}

func TestEngramDataDirOptionCount(t *testing.T) {
	// Without existing data
	if got := EngramDataDirOptionCount(false, EngramChoiceDefault); got != 4 {
		t.Errorf("EngramDataDirOptionCount(false, default) = %d, want 4", got)
	}
	if got := EngramDataDirOptionCount(false, EngramChoiceStartFresh); got != 5 {
		t.Errorf("EngramDataDirOptionCount(false, startFresh) = %d, want 5", got)
	}

	// With existing data
	if got := EngramDataDirOptionCount(true, EngramChoiceDefault); got != 6 {
		t.Errorf("EngramDataDirOptionCount(true, default) = %d, want 6", got)
	}
	if got := EngramDataDirOptionCount(true, EngramChoiceMigrate); got != 7 {
		t.Errorf("EngramDataDirOptionCount(true, migrate) = %d, want 7", got)
	}
	if got := EngramDataDirOptionCount(true, EngramChoiceStartFresh); got != 7 {
		t.Errorf("EngramDataDirOptionCount(true, startFresh) = %d, want 7", got)
	}
	if got := EngramDataDirOptionCount(true, EngramChoiceClean); got != 6 {
		t.Errorf("EngramDataDirOptionCount(true, clean) = %d, want 6", got)
	}
}

func TestEngramDataDirContinueRow_WithExistingData(t *testing.T) {
	tests := []struct {
		name   string
		choice int
		want   int
	}{
		{"default", EngramChoiceDefault, 4},
		{"migrate", EngramChoiceMigrate, 3},
		{"start fresh", EngramChoiceStartFresh, 4},
		{"clean", EngramChoiceClean, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EngramDataDirContinueRow(true, tt.choice); got != tt.want {
				t.Errorf("EngramDataDirContinueRow(true, %d) = %d, want %d", tt.choice, got, tt.want)
			}
		})
	}
}

func TestRenderEngramTextInput(t *testing.T) {
	out := renderEngramTextInput("hello", true, 2)
	if !strings.Contains(out, "he") {
		t.Error("missing 'he' prefix")
	}
	if !strings.Contains(out, "llo") {
		t.Error("missing 'llo' suffix")
	}
}

func TestRenderEngramConfirm(t *testing.T) {
	out := RenderEngramConfirm(EngramConfirmRenderArgs{
		Title:   "CONFIRM CLEAN",
		Message: "This will permanently delete all Engram data.",
		Warning: "This cannot be undone.",
		Cursor:  0,
	})
	if !strings.Contains(out, "CONFIRM CLEAN") {
		t.Error("missing title")
	}
	if !strings.Contains(out, "permanently delete") {
		t.Error("missing message")
	}
	if !strings.Contains(out, "cannot be undone") {
		t.Error("missing warning")
	}
	if !strings.Contains(out, "Confirm") {
		t.Error("missing Confirm option")
	}
	if !strings.Contains(out, "Cancel") {
		t.Error("missing Cancel option")
	}
}

func TestRenderEngramConfirm_CancelFocused(t *testing.T) {
	out := RenderEngramConfirm(EngramConfirmRenderArgs{
		Title:   "CONFIRM CLEAN",
		Message: "This will permanently delete all Engram data.",
		Cursor:  1,
	})
	if !strings.Contains(out, "> ● Cancel") {
		t.Error("Cancel should be focused")
	}
	if !strings.Contains(out, "○ Confirm") {
		t.Error("Confirm should not be focused")
	}
}

func TestRenderEngramConfirm_ShowsFilesForClean(t *testing.T) {
	out := RenderEngramConfirm(EngramConfirmRenderArgs{
		Title:       "CONFIRM CLEAN DATA",
		Message:     "This will permanently delete all Engram data at:\n/home/user/.engram",
		Warning:     "This cannot be undone. All memory will be lost.",
		Cursor:      0,
		FilesToMove: []string{"engram.db (4.0 KB)", "engram.db-wal (1.0 KB)"},
		TotalBytes:  5 * 1024,
	})
	if !strings.Contains(out, "Files that will be affected:") {
		t.Error("missing file list heading")
	}
	if !strings.Contains(out, "engram.db") {
		t.Error("missing engram.db in file list")
	}
	if !strings.Contains(out, "5.0 KB") {
		t.Error("missing total size")
	}
}

func TestEngramConfirmOptionCount(t *testing.T) {
	if got := EngramConfirmOptionCount(); got != 2 {
		t.Errorf("EngramConfirmOptionCount() = %d, want 2", got)
	}
}

func TestRenderEngramFeedback(t *testing.T) {
	out := RenderEngramFeedback(EngramFeedbackRenderArgs{
		Title:   "DATA CLEANED",
		Message: "All Engram data has been permanently deleted.",
		Details: "",
	})
	if !strings.Contains(out, "DATA CLEANED") {
		t.Error("missing title")
	}
	if !strings.Contains(out, "permanently deleted") {
		t.Error("missing message")
	}
	if !strings.Contains(out, "Continue") {
		t.Error("missing Continue option")
	}
}

func TestRenderEngramFeedback_WithDetails(t *testing.T) {
	out := RenderEngramFeedback(EngramFeedbackRenderArgs{
		Title:   "MIGRATION COMPLETE",
		Message: "Engram data has been moved successfully.",
		Details: "2 files moved, 1.2 MB transferred",
	})
	if !strings.Contains(out, "MIGRATION COMPLETE") {
		t.Error("missing title")
	}
	if !strings.Contains(out, "2 files moved") {
		t.Error("missing details")
	}
}

func TestEngramFeedbackOptionCount(t *testing.T) {
	if got := EngramFeedbackOptionCount(); got != 1 {
		t.Errorf("EngramFeedbackOptionCount() = %d, want 1", got)
	}
}
