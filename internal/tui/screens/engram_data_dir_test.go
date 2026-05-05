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
	if strings.Contains(out, "Keep data at current location") {
		t.Error("keep data should be folded into the back row, not shown as a redundant action")
	}
	if !strings.Contains(out, "Move existing data to a new location") {
		t.Error("missing migrate option")
	}
	if !strings.Contains(out, "Copy existing data to another location") {
		t.Error("missing copy option")
	}
	if !strings.Contains(out, "Set active Engram directory") {
		t.Error("missing set active option")
	}
	if !strings.Contains(out, "Start fresh at a new location") {
		t.Error("missing start fresh option")
	}
	if strings.Contains(out, "Clean/reset data") {
		t.Error("clean/reset should not be a main option")
	}
	if strings.Contains(out, "Continue") {
		t.Error("main decision screen should not show Continue")
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
	if strings.Contains(out, "Keep this location") {
		t.Error("keep location should be folded into the back row, not shown as a redundant action")
	}
	if !strings.Contains(out, "Set active Engram directory") {
		t.Error("missing set active option without local data")
	}
	if !strings.Contains(out, "Start fresh at a new location") {
		t.Error("missing start fresh option")
	}
	// Should NOT show migrate/copy/clean options when no data.
	if strings.Contains(out, "Migrate") {
		t.Error("should not show migrate option without data")
	}
	if strings.Contains(out, "Copy existing data") {
		t.Error("should not show copy option without data")
	}
}

func TestRenderEngramDataDir_WithoutData_CustomChoiceKeepsMainScreenFocused(t *testing.T) {
	args := EngramDataDirRenderArgs{
		CurrentDir:      "/home/user/.engram",
		HasExistingData: false,
		Choice:          EngramChoiceStartFresh,
		DefaultDirSpace: 45 * 1024 * 1024 * 1024,
	}
	out := RenderEngramDataDir(args)
	if strings.Contains(out, "Path:") {
		t.Error("main screen should not show path input")
	}
	if strings.Contains(out, "Suggested locations:") {
		t.Error("main screen should not show suggestions")
	}
}

func TestRenderEngramPathPicker_EmptyInputShowsCrossPlatformHelper(t *testing.T) {
	out := RenderEngramPathPicker(EngramPathPickerRenderArgs{Choice: EngramChoiceStartFresh, CustomPath: "", Cursor: 0})
	if !strings.Contains(out, "Paste or type a full directory path") {
		t.Fatalf("missing empty-input helper; output:\n%s", out)
	}
	for _, example := range []string{`C:\engram`, "~/engram", "/Volumes/Drive/engram", "/mnt/usb/engram"} {
		if !strings.Contains(out, example) {
			t.Fatalf("missing path example %q in helper; output:\n%s", example, out)
		}
	}
	if strings.Contains(out, "/data/engram") || strings.Contains(out, "~/.engram") {
		t.Fatalf("path input should not be prefilled with a suggested/default path; output:\n%s", out)
	}
}

func TestRenderEngramPathPicker_CustomPath(t *testing.T) {
	args := EngramPathPickerRenderArgs{
		Choice:     EngramChoiceStartFresh,
		CustomPath: "/data/engram",
		Cursor:     0,
		InputPos:   5,
	}
	out := RenderEngramPathPicker(args)
	if !strings.Contains(out, "/data/engram") {
		t.Error("missing custom path")
	}
}

func TestRenderEngramPathPicker_CopyExplainsOriginalStaysPut(t *testing.T) {
	out := RenderEngramPathPicker(EngramPathPickerRenderArgs{
		Choice:      EngramChoiceCopy,
		CustomPath:  "/copy/engram",
		Cursor:      0,
		FilesToMove: []string{"engram.db (4 B)"},
		TotalBytes:  4,
	})
	if !strings.Contains(out, "Original data will stay at the current location") {
		t.Fatalf("copy path picker should explain source remains, output:\n%s", out)
	}
}

func TestRenderEngramPathPicker_SuggestedLocations(t *testing.T) {
	args := EngramPathPickerRenderArgs{
		Choice:     EngramChoiceMigrate,
		CustomPath: "",
		Cursor:     0,
		SuggestedLocations: []engram.LocationSuggestion{
			{Label: "Home", Path: "/home/user/.engram", Space: 45 * 1024 * 1024 * 1024},
			{Label: "External", Path: "/mnt/external/engram", Space: 120 * 1024 * 1024 * 1024},
		},
	}
	out := RenderEngramPathPicker(args)
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
	if got := EngramDataDirOptionCount(false, EngramChoiceDefault); got != 3 {
		t.Errorf("EngramDataDirOptionCount(false, default) = %d, want 3", got)
	}
	if got := EngramDataDirOptionCount(false, EngramChoiceStartFresh); got != 3 {
		t.Errorf("EngramDataDirOptionCount(false, startFresh) = %d, want 3", got)
	}
	if got := EngramDataDirOptionCount(true, EngramChoiceDefault); got != 5 {
		t.Errorf("EngramDataDirOptionCount(true, default) = %d, want 5", got)
	}
	if got := EngramDataDirOptionCount(true, EngramChoiceMigrate); got != 5 {
		t.Errorf("EngramDataDirOptionCount(true, migrate) = %d, want 5", got)
	}
	if got := EngramDataDirOptionCount(true, EngramChoiceCopy); got != 5 {
		t.Errorf("EngramDataDirOptionCount(true, copy) = %d, want 5", got)
	}
}

func TestEngramDataDirFeatureParityRows(t *testing.T) {
	withData := RenderEngramDataDir(EngramDataDirRenderArgs{HasExistingData: true})
	checks := []struct {
		label string
		want  bool
	}{
		{"Keep data at current location", false},
		{"Back / keep current location", true},
		{"Move existing data to a new location", true},
		{"Copy existing data to another location", true},
		{"Set active Engram directory", true},
		{"Start fresh at a new location", true},
		{"Continue", false},
		{"Clean/reset data", false},
	}
	for _, tt := range checks {
		t.Run(tt.label, func(t *testing.T) {
			got := strings.Contains(withData, tt.label)
			if got != tt.want {
				t.Fatalf("contains(%q) = %v, want %v; output:\n%s", tt.label, got, tt.want, withData)
			}
		})
	}
}

func TestRenderEngramDataDir_NoActionPreselectedWithFilledBlob(t *testing.T) {
	for _, hasData := range []bool{false, true} {
		out := RenderEngramDataDir(EngramDataDirRenderArgs{HasExistingData: hasData, Choice: EngramChoiceStartFresh})
		if strings.Contains(out, "●") {
			t.Fatalf("main data-dir screen should not render a filled selected blob; hasData=%v output:\n%s", hasData, out)
		}
	}
}

func TestEngramFilesHeadingByChoice(t *testing.T) {
	tests := []struct {
		name   string
		choice int
		want   string
	}{
		{"migrate", EngramChoiceMigrate, "Files that will be moved:"},
		{"copy", EngramChoiceCopy, "Files that will be copied:"},
		{"set active", EngramChoiceSetActive, "Files at selected active directory:"},
		{"start fresh", EngramChoiceStartFresh, "Files that will be deleted:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := RenderEngramPathPicker(EngramPathPickerRenderArgs{Choice: tt.choice, FilesToMove: []string{"engram.db (1 B)"}})
			if !strings.Contains(out, tt.want) {
				t.Fatalf("missing heading %q in output:\n%s", tt.want, out)
			}
		})
	}
}

func TestEngramPathPickerOptionCount(t *testing.T) {
	suggestions := []engram.LocationSuggestion{{Label: "Home", Path: "/home/user/.engram"}, {Label: "External", Path: "/mnt/external/.engram"}}
	if got := EngramPathPickerOptionCount(suggestions); got != 4 {
		t.Errorf("EngramPathPickerOptionCount() = %d, want 4", got)
	}
	if got := EngramPathPickerBackRow(suggestions); got != 3 {
		t.Errorf("EngramPathPickerBackRow() = %d, want 3", got)
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

func TestRenderEngramConfirm_NoSelectedBlobs(t *testing.T) {
	out := RenderEngramConfirm(EngramConfirmRenderArgs{
		Title:   "CONFIRM COPY",
		Message: "Copy Engram data.",
		Cursor:  0,
	})
	if strings.Contains(out, "●") || strings.Contains(out, "○") {
		t.Fatalf("confirm screen should render focus only, not selected blobs; output:\n%s", out)
	}
	if !strings.Contains(out, "> Confirm") {
		t.Fatalf("confirm row should still be focused; output:\n%s", out)
	}
}

func TestRenderEngramConfirm_CancelFocused(t *testing.T) {
	out := RenderEngramConfirm(EngramConfirmRenderArgs{
		Title:   "CONFIRM CLEAN",
		Message: "This will permanently delete all Engram data.",
		Cursor:  1,
	})
	if !strings.Contains(out, "> Cancel") {
		t.Error("Cancel should be focused")
	}
	if strings.Contains(out, "●") || strings.Contains(out, "○") {
		t.Error("confirm screen should show focus only, not selected radio blobs")
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

func TestRenderEngramFeedback_WithDeletedDetails(t *testing.T) {
	out := RenderEngramFeedback(EngramFeedbackRenderArgs{
		Title:   "FRESH DATABASE CREATED",
		Message: "A new empty database will be created at the selected location.",
		Details: "2 files deleted, 7.0 KB removed",
	})
	if !strings.Contains(out, "2 files deleted") {
		t.Error("missing deleted file count")
	}
	if !strings.Contains(out, "7.0 KB removed") {
		t.Error("missing deleted byte size")
	}
}

func TestRenderEngramFeedback_NoSelectedBlob(t *testing.T) {
	out := RenderEngramFeedback(EngramFeedbackRenderArgs{Title: "DONE", Message: "Complete."})
	if strings.Contains(out, "●") || strings.Contains(out, "○") {
		t.Fatalf("feedback screen should render focus only, not selected blobs; output:\n%s", out)
	}
	if !strings.Contains(out, "> Continue") {
		t.Fatalf("continue row should still be focused; output:\n%s", out)
	}
}

func TestEngramFeedbackOptionCount(t *testing.T) {
	if got := EngramFeedbackOptionCount(); got != 1 {
		t.Errorf("EngramFeedbackOptionCount() = %d, want 1", got)
	}
}
