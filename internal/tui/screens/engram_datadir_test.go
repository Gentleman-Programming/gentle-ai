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
	out := RenderEngramDataDir(0, "/home/user/.engram", 1024, locs, model.EngramDataDirOpNone)
	if !strings.Contains(out, "Manage Engram Data Directory") {
		t.Error("expected title in output")
	}
}

func TestRenderEngramDataDir_ShowsCurrentDir(t *testing.T) {
	locs := []engram.Location{
		{Path: "/home/user/.engram", Label: "~/.engram", Available: -1, IsCurrent: true},
	}
	out := RenderEngramDataDir(0, "/home/user/.engram", 0, locs, model.EngramDataDirOpNone)
	if !strings.Contains(out, "/home/user/.engram") {
		t.Error("expected current dir in output")
	}
}

func TestRenderEngramDataDir_ActionMenuHidesLocations(t *testing.T) {
	locs := []engram.Location{
		{Path: "/home/user/.engram", Label: "~/.engram", Available: 1024 * 1024, IsCurrent: true},
		{Path: "/mnt/external/Engram", Label: "/mnt/external/Engram", Available: 1024 * 1024 * 1024, IsCurrent: false},
	}
	out := RenderEngramDataDir(0, "/home/user/.engram", 0, locs, model.EngramDataDirOpNone)
	for _, want := range []string{"Migrate / Move Data", "Copy Data", "Delete current Data"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected action %q", want)
		}
	}
	if strings.Contains(out, "/mnt/external/Engram") {
		t.Error("action menu should not show destination paths")
	}
}

func TestRenderEngramDataDir_DestinationMenuShowsRelevantLocations(t *testing.T) {
	locs := []engram.Location{
		{Path: "/home/user/.engram", Label: "~/.engram", IsCurrent: true},
		{Path: "/mnt/external/Engram", Label: "/mnt/external/Engram", IsCurrent: false},
	}
	out := RenderEngramDataDir(0, "/home/user/.engram", 0, locs, model.EngramDataDirOpMove)
	if !strings.Contains(out, "Move to") || !strings.Contains(out, "/mnt/external/Engram") {
		t.Fatal("expected move destination option")
	}
	if strings.Contains(out, "Copy Data") || strings.Contains(out, "Delete current Data") {
		t.Error("destination menu should not show unrelated top-level actions")
	}
}

func TestEngramDirActionChoices_Order(t *testing.T) {
	choices := EngramDirActionChoices()
	want := []model.EngramDataDirOp{
		model.EngramDataDirOpMove,
		model.EngramDataDirOpCopy,
		model.EngramDataDirOpDelete,
	}
	for i, op := range want {
		if choices[i].Op != op {
			t.Errorf("choices[%d].Op = %q, want %q", i, choices[i].Op, op)
		}
	}
}

func TestEngramDirLocationChoices_NonCurrentLocations(t *testing.T) {
	locs := []engram.Location{
		{Path: "/a", Label: "A", IsCurrent: true},
		{Path: "/b", Label: "B", IsCurrent: false},
	}
	choices := EngramDirLocationChoices(locs, model.EngramDataDirOpCopy)
	if len(choices) != 1 {
		t.Fatalf("len(choices) = %d, want 1", len(choices))
	}
	if choices[0].Op != model.EngramDataDirOpCopy {
		t.Errorf("choices[0].Op = %q, want Copy", choices[0].Op)
	}
	if choices[0].DstIdx != 1 {
		t.Errorf("choices[0].DstIdx = %d, want 1", choices[0].DstIdx)
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
