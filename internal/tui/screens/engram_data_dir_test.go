package screens

import (
	"strings"
	"testing"
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

func TestRenderEngramDataDir_WithoutData(t *testing.T) {
	args := EngramDataDirRenderArgs{
		CurrentDir:      "/home/user/.engram",
		HasExistingData: false,
		Choice:          EngramChoiceDefault,
		CustomPath:      "",
		Cursor:          0,
		InputPos:        0,
		ErrMsg:          "",
	}
	out := RenderEngramDataDir(args)
	if !strings.Contains(out, "Use default location") {
		t.Error("missing default option")
	}
	if !strings.Contains(out, "Use custom location") {
		t.Error("missing custom option")
	}
	// Should NOT show migrate/clean options when no data
	if strings.Contains(out, "Migrate") {
		t.Error("should not show migrate option without data")
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

func TestEngramDataDirOptionCount(t *testing.T) {
	// Without existing data
	if got := EngramDataDirOptionCount(false, EngramChoiceDefault); got != 4 {
		t.Errorf("EngramDataDirOptionCount(false, default) = %d, want 4", got)
	}
	if got := EngramDataDirOptionCount(false, EngramChoiceStartFresh); got != 4 {
		t.Errorf("EngramDataDirOptionCount(false, startFresh) = %d, want 4", got)
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
