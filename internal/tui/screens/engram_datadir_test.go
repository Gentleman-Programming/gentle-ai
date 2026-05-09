package screens

import (
	"errors"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/components/engram"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestRenderEngramDataDir_HasTitle(t *testing.T) {
	locs := []engram.Location{
		{Path: "/home/user/.engram", Label: "~/.engram", Available: -1, IsCurrent: true},
	}
	out := RenderEngramDataDir(0, "/home/user/.engram", 1024, locs, "")
	if !strings.Contains(out, "Manage Engram Data Directory") {
		t.Error("expected title in output")
	}
}

func TestRenderEngramDataDir_ShowsCurrentDir(t *testing.T) {
	locs := []engram.Location{
		{Path: "/home/user/.engram", Label: "~/.engram", Available: -1, IsCurrent: true},
	}
	out := RenderEngramDataDir(0, "/home/user/.engram", 0, locs, "")
	if !strings.Contains(out, "/home/user/.engram") {
		t.Error("expected current dir in output")
	}
}

func TestRenderEngramDataDir_MultipleLocations(t *testing.T) {
	locs := []engram.Location{
		{Path: "/home/user/.engram", Label: "~/.engram", Available: 1024 * 1024, IsCurrent: true},
		{Path: "/mnt/external/Engram", Label: "/mnt/external/Engram", Available: 1024 * 1024 * 1024, IsCurrent: false},
	}
	out := RenderEngramDataDir(0, "/home/user/.engram", 0, locs, "")
	if !strings.Contains(out, "Copy to") {
		t.Error("expected Copy to option for non-current location")
	}
	if !strings.Contains(out, "Move to") {
		t.Error("expected Move to option for non-current location")
	}
}

func TestEngramDirChoices_AlwaysHasKeepAndDelete(t *testing.T) {
	locs := []engram.Location{
		{Path: "/a", Label: "/a", IsCurrent: true},
	}
	choices := EngramDirChoices(locs)

	if choices[0].Op != model.EngramDataDirOpKeep {
		t.Errorf("first choice should be Keep, got %q", choices[0].Op)
	}
	last := choices[len(choices)-1]
	if last.Op != model.EngramDataDirOpDelete {
		t.Errorf("last choice should be Delete, got %q", last.Op)
	}
}

func TestEngramDirChoices_NonCurrentLocations(t *testing.T) {
	locs := []engram.Location{
		{Path: "/a", Label: "A", IsCurrent: true},
		{Path: "/b", Label: "B", IsCurrent: false},
	}
	choices := EngramDirChoices(locs)
	// Keep + Copy/Move for /b + Delete = 4
	if len(choices) != 4 {
		t.Errorf("len(choices) = %d, want 4", len(choices))
	}
	var ops []model.EngramDataDirOp
	for _, c := range choices {
		ops = append(ops, c.Op)
	}
	if ops[1] != model.EngramDataDirOpCopy {
		t.Errorf("choices[1].Op = %q, want Copy", ops[1])
	}
	if ops[2] != model.EngramDataDirOpMove {
		t.Errorf("choices[2].Op = %q, want Move", ops[2])
	}
}

func TestRenderEngramDataDirInstall_HasTitle(t *testing.T) {
	out := RenderEngramDataDirInstall(0, "/home/user/.engram", 512)
	if !strings.Contains(out, "Existing Engram Data Found") {
		t.Error("expected title in install screen output")
	}
}

func TestRenderEngramDataDirConfirm_Copy(t *testing.T) {
	out := RenderEngramDataDirConfirm(model.EngramDataDirOpCopy, "/src", "/dst", 0)
	if !strings.Contains(out, "Copy") {
		t.Error("expected Copy in confirm output")
	}
	if !strings.Contains(out, "/src") || !strings.Contains(out, "/dst") {
		t.Error("expected src and dst paths in confirm output")
	}
}

func TestRenderEngramDataDirConfirm_Delete(t *testing.T) {
	out := RenderEngramDataDirConfirm(model.EngramDataDirOpDelete, "/src", "", 0)
	if !strings.Contains(out, "Delete") {
		t.Error("expected Delete in confirm output")
	}
}

func TestRenderEngramDataDirResult_Success(t *testing.T) {
	out := RenderEngramDataDirResult(model.EngramDataDirOpCopy, "snap-abc123", nil)
	if !strings.Contains(out, "snap-abc123") {
		t.Error("expected snapshot ID in success output")
	}
	if strings.Contains(out, "failed") {
		t.Error("should not contain 'failed' on success")
	}
}

func TestRenderEngramDataDirResult_Error(t *testing.T) {
	out := RenderEngramDataDirResult(model.EngramDataDirOpDelete, "", errors.New("disk full"))
	if !strings.Contains(out, "disk full") {
		t.Error("expected error message in failure output")
	}
	if !strings.Contains(out, "failed") {
		t.Error("expected 'failed' in error output")
	}
}
