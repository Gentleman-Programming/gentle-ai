package screens

import (
	"errors"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/components/engram"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestRenderEngramDataDir_ShowsTitleAndCurrentDir(t *testing.T) {
	locs := []engram.Location{{Path: "/home/user/.engram", Label: "~/.engram", IsCurrent: true}}
	out := RenderEngramDataDir(0, "/home/user/.engram", 0, locs, model.EngramDataDirOpNone, "")
	if !strings.Contains(out, "Manage Engram Data Directory") {
		t.Fatal("missing title")
	}
	if !strings.Contains(out, "/home/user/.engram") {
		t.Fatal("missing current dir")
	}
}

func TestRenderEngramDataDir_LocationMode(t *testing.T) {
	locs := []engram.Location{
		{Path: "/home/user/.engram", Label: "~/.engram", IsCurrent: true},
		{Path: "/mnt/Engram", Label: "/mnt/Engram"},
	}
	out := RenderEngramDataDir(0, "/home/user/.engram", 0, locs, model.EngramDataDirOpCopy, "")
	if !strings.Contains(out, "Copy to") {
		t.Fatal("missing copy location option")
	}
	if !strings.Contains(out, "Type custom path") {
		t.Fatal("missing custom path option")
	}
}

func TestRenderEngramDataDirInstall_Modes(t *testing.T) {
	locs := []engram.Location{{Path: "/home/user/.engram", Label: "~/.engram"}}
	noData := RenderEngramDataDirInstall(0, "/home/user/.engram", 0, locs, false, "")
	if !strings.Contains(noData, "Choose Engram Data Directory") {
		t.Fatal("missing no-data title")
	}
	existing := RenderEngramDataDirInstall(0, "/home/user/.engram", 0, locs, true, "")
	if !strings.Contains(existing, "Existing Engram Data Found") {
		t.Fatal("missing existing-data title")
	}
}

func TestRenderEngramDataDirCustomPath_ShowsError(t *testing.T) {
	out := RenderEngramDataDirCustomPath(model.EngramDataDirOpMove, "/src", "relative", 8, "Use an absolute path.")
	if !strings.Contains(out, "Use an absolute path") {
		t.Fatal("missing inline error")
	}
}

func TestRenderEngramDataDirConfirmAndResult(t *testing.T) {
	confirm := RenderEngramDataDirConfirm(model.EngramDataDirOpDelete, "/src", "", "/backups", 0)
	if !strings.Contains(confirm, "Delete") {
		t.Fatal("missing delete text")
	}
	success := RenderEngramDataDirResult(model.EngramDataDirOpCopy, "snap-1", "/backups", nil)
	if !strings.Contains(success, "snap-1") {
		t.Fatal("missing snapshot ID")
	}
	failed := RenderEngramDataDirResult(model.EngramDataDirOpMove, "", "", errors.New("disk full"))
	if !strings.Contains(failed, "disk full") {
		t.Fatal("missing error text")
	}
}
