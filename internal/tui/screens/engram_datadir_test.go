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
	out := RenderEngramDataDir(0, "/home/user/.engram", 1024, locs, model.EngramDataDirOpNone, "")
	if !strings.Contains(out, "Manage Engram Data Directory") {
		t.Error("expected title in output")
	}
}

func TestRenderEngramDataDir_ShowsCurrentDir(t *testing.T) {
	locs := []engram.Location{
		{Path: "/home/user/.engram", Label: "~/.engram", Available: -1, IsCurrent: true},
	}
	out := RenderEngramDataDir(0, "/home/user/.engram", 0, locs, model.EngramDataDirOpNone, "")
	if !strings.Contains(out, "/home/user/.engram") {
		t.Error("expected current dir in output")
	}
}

func TestRenderEngramDataDir_ActionMenuHidesLocations(t *testing.T) {
	locs := []engram.Location{
		{Path: "/home/user/.engram", Label: "~/.engram", Available: 1024 * 1024, IsCurrent: true},
		{Path: "/mnt/external/Engram", Label: "/mnt/external/Engram", Available: 1024 * 1024 * 1024, IsCurrent: false},
	}
	out := RenderEngramDataDir(0, "/home/user/.engram", 0, locs, model.EngramDataDirOpNone, "")
	for _, want := range []string{"Migrate / Move Data", "Copy Data", "Set Active Directory", "Delete current Data"} {
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
	out := RenderEngramDataDir(0, "/home/user/.engram", 0, locs, model.EngramDataDirOpMove, "")
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
		model.EngramDataDirOpSet,
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
	if len(choices) != 2 {
		t.Fatalf("len(choices) = %d, want 2", len(choices))
	}
	if choices[0].Op != model.EngramDataDirOpCopy {
		t.Errorf("choices[0].Op = %q, want Copy", choices[0].Op)
	}
	if choices[0].DstIdx != 1 {
		t.Errorf("choices[0].DstIdx = %d, want 1", choices[0].DstIdx)
	}
	if choices[1].DstIdx != EngramCustomPathDstIdx {
		t.Errorf("custom path DstIdx = %d, want %d", choices[1].DstIdx, EngramCustomPathDstIdx)
	}
}

func TestEngramDirLocationChoices_SetShowsCurrentAsActive(t *testing.T) {
	locs := []engram.Location{
		{Path: "/a", Label: "A", IsCurrent: true},
		{Path: "/b", Label: "B", IsCurrent: false, HasData: true},
	}
	choices := EngramDirLocationChoices(locs, model.EngramDataDirOpSet)
	if len(choices) != 2 {
		t.Fatalf("len(choices) = %d, want current + found dir", len(choices))
	}
	if choices[0].DstIdx != 0 || !strings.Contains(choices[0].Label, "(ACTIVE)") {
		t.Fatalf("first set choice should show current ACTIVE, got %+v", choices[0])
	}
	if !strings.Contains(choices[1].Label, "Use  B") {
		t.Fatalf("second set choice should offer other known dir, got %+v", choices[1])
	}
}

func TestEngramDirLocationChoices_SetHidesEmptySuggestions(t *testing.T) {
	locs := []engram.Location{
		{Path: "/a", Label: "A", IsCurrent: true},
		{Path: "/drive/Engram", Label: "Drive suggestion", IsCurrent: false, HasData: false},
		{Path: "/found/Engram", Label: "Found data dir", IsCurrent: false, HasData: true},
	}
	choices := EngramDirLocationChoices(locs, model.EngramDataDirOpSet)
	if len(choices) != 2 {
		t.Fatalf("len(choices) = %d, want active + found only", len(choices))
	}
	if strings.Contains(choices[1].Label, "Drive suggestion") {
		t.Fatalf("set active should not show empty drive suggestions: %+v", choices)
	}
	if choices[1].DstIdx != 2 || !strings.Contains(choices[1].Label, "Found data dir") {
		t.Fatalf("second set choice should be found data dir, got %+v", choices[1])
	}
}

func TestEngramDirInstallLocationChoices_ShowsLocationsAndCustomPath(t *testing.T) {
	locs := []engram.Location{
		{Path: "/home/user/.engram", Label: "~/.engram"},
		{Path: "/mnt/drive/Engram", Label: "/mnt/drive/Engram"},
	}
	choices := EngramDirInstallLocationChoices(locs)
	if len(choices) != 3 {
		t.Fatalf("len(choices) = %d, want locations + custom", len(choices))
	}
	if !strings.Contains(choices[0].Label, "~/.engram") || choices[0].Op != model.EngramDataDirOpSet {
		t.Fatalf("first choice should use default data dir, got %+v", choices[0])
	}
	if choices[2].DstIdx != EngramCustomPathDstIdx {
		t.Fatalf("last choice should be custom path, got %+v", choices[2])
	}
}

func TestRenderEngramDataDirCustomPath_ShowsCrossPlatformExamples(t *testing.T) {
	out := RenderEngramDataDirCustomPath(model.EngramDataDirOpMove, "/home/user/.engram", "", 0, "")
	for _, want := range []string{`D:\Engram`, "~/Engram", "/Volumes/Drive/Engram", "/mnt/usb/Engram"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing example %q in output:\n%s", want, out)
		}
	}
}

func TestRenderEngramDataDirCustomPath_ShowsTypedPathAndError(t *testing.T) {
	out := RenderEngramDataDirCustomPath(model.EngramDataDirOpCopy, "/src", "/dst/Engram", 4, "path required")
	if !strings.Contains(out, "/dst") || !strings.Contains(out, "/Engram") {
		t.Fatal("expected typed path in output")
	}
	if !strings.Contains(out, "path required") {
		t.Fatal("expected error in output")
	}
}

func TestRenderEngramDataDirInstall_HasTitle(t *testing.T) {
	out := RenderEngramDataDirInstall(0, "/home/user/.engram", 512, nil, true, "")
	if !strings.Contains(out, "Existing Engram Data Found") {
		t.Error("expected title in install screen output")
	}
}

func TestRenderEngramDataDirInstall_NoDataShowsDefaultAndCustomPath(t *testing.T) {
	locs := []engram.Location{
		{Path: "/home/user/.engram", Label: "~/.engram"},
		{Path: "/mnt/drive/Engram", Label: "/mnt/drive/Engram"},
	}
	out := RenderEngramDataDirInstall(0, "/home/user/.engram", 0, locs, false, "")
	for _, want := range []string{"Choose Engram Data Directory", "No existing Engram data was found", "~/.engram", "/mnt/drive/Engram", "Type custom path"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in install screen output:\n%s", want, out)
		}
	}
}

func TestRenderEngramDataDirConfirm_Copy(t *testing.T) {
	out := RenderEngramDataDirConfirm(model.EngramDataDirOpCopy, "/src", "/dst", "/home/user/.gentle-ai/backups", 0)
	if !strings.Contains(out, "Copy") {
		t.Error("expected Copy in confirm output")
	}
	if !strings.Contains(out, "/src") || !strings.Contains(out, "/dst") {
		t.Error("expected src and dst paths in confirm output")
	}
	if !strings.Contains(out, "/home/user/.gentle-ai/backups") {
		t.Error("expected backup root in confirm output")
	}
}

func TestRenderEngramDataDirConfirm_Delete(t *testing.T) {
	out := RenderEngramDataDirConfirm(model.EngramDataDirOpDelete, "/src", "", "/home/user/.gentle-ai/backups", 0)
	for _, want := range []string{"Yes, delete data", "No, go back", "All data files", "/home/user/.gentle-ai/backups", "directory itself is left in place"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in confirm output:\n%s", want, out)
		}
	}
}

func TestRenderEngramDataDirConfirm_SetHasNoSnapshotNote(t *testing.T) {
	out := RenderEngramDataDirConfirm(model.EngramDataDirOpSet, "/src", "/dst", "/home/user/.gentle-ai/backups", 0)
	if !strings.Contains(out, "Active") || !strings.Contains(out, "/dst") {
		t.Fatal("expected active directory handoff in confirm output")
	}
	if strings.Contains(out, "snapshot backup") || strings.Contains(out, ".gentle-ai/backups") {
		t.Fatal("set active should not claim it creates a snapshot")
	}
}

func TestRenderEngramDataDirResult_Success(t *testing.T) {
	out := RenderEngramDataDirResult(model.EngramDataDirOpCopy, "snap-abc123", "/home/user/.gentle-ai/backups", nil)
	if !strings.Contains(out, "snap-abc123") {
		t.Error("expected snapshot ID in success output")
	}
	if !strings.Contains(out, "Backup snapshot") || !strings.Contains(out, ".gentle-ai") || !strings.Contains(out, "backups") {
		t.Error("expected backup label and location in success output")
	}
	if strings.Contains(out, "failed") {
		t.Error("should not contain 'failed' on success")
	}
}

func TestRenderEngramDataDirResult_Error(t *testing.T) {
	out := RenderEngramDataDirResult(model.EngramDataDirOpDelete, "", "/home/user/.gentle-ai/backups", errors.New("disk full"))
	if !strings.Contains(out, "disk full") {
		t.Error("expected error message in failure output")
	}
	if !strings.Contains(out, "failed") {
		t.Error("expected 'failed' in error output")
	}
}
