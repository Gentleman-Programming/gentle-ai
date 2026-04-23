package screens

import (
	"strings"
	"testing"
)

func TestRenderEngramDataDir_KeepCurrent(t *testing.T) {
	args := EngramDataDirRenderArgs{
		CurrentDir:      "/home/user/.engram",
		HasExistingData: true,
		Choice:          0,
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
}

func TestRenderEngramDataDir_CustomPath(t *testing.T) {
	args := EngramDataDirRenderArgs{
		CurrentDir:      "/home/user/.engram",
		HasExistingData: false,
		Choice:          1,
		CustomPath:      "/data/engram",
		Cursor:          2, // on text input row
		InputPos:        5,
		ErrMsg:          "",
	}
	out := RenderEngramDataDir(args)
	if !strings.Contains(out, "/data/engram") {
		t.Error("missing custom path")
	}
}

func TestEngramDataDirOptionCount(t *testing.T) {
	if got := EngramDataDirOptionCount(false, EngramChoiceKeep); got != 4 {
		t.Errorf("EngramDataDirOptionCount(false, keep) = %d, want 4", got)
	}
	if got := EngramDataDirOptionCount(true, EngramChoiceKeep); got != 5 {
		t.Errorf("EngramDataDirOptionCount(true, keep) = %d, want 5", got)
	}
	if got := EngramDataDirOptionCount(false, EngramChoiceStartFresh); got != 5 {
		t.Errorf("EngramDataDirOptionCount(false, startFresh) = %d, want 5", got)
	}
	if got := EngramDataDirOptionCount(true, EngramChoiceMigrate); got != 6 {
		t.Errorf("EngramDataDirOptionCount(true, migrate) = %d, want 6", got)
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
