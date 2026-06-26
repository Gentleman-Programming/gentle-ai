package screens

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/update"
	"github.com/gentleman-programming/gentle-ai/internal/update/upgrade"
)

// ─── RenderUpgradeSync states ──────────────────────────────────────────────

// TestRenderUpgradeSync_ConfirmState verifies the default confirmation screen
// (updateCheckDone=true, not running, no results) shows the two-step description.
func TestRenderUpgradeSync_ConfirmState(t *testing.T) {
	out := RenderUpgradeSync(nil, nil, nil, nil, nil, false /*operationRunning*/, true /*updateCheckDone*/, 0, 0)

	lower := strings.ToLower(out)
	// Must mention both operations.
	if !strings.Contains(lower, "upgrade") {
		t.Errorf("RenderUpgradeSync(confirm) should mention 'upgrade'; got:\n%s", out)
	}
	if !strings.Contains(lower, "sync") {
		t.Errorf("RenderUpgradeSync(confirm) should mention 'sync'; got:\n%s", out)
	}
	// Must show a prompt.
	if !strings.Contains(lower, "enter") && !strings.Contains(lower, "begin") {
		t.Errorf("RenderUpgradeSync(confirm) should show enter/begin prompt; got:\n%s", out)
	}
}

// TestRenderUpgradeSync_RunningUpgradePhase verifies that while the upgrade is
// running (operationRunning=true, upgradeReport=nil), the screen shows an
// "upgrading" indicator.
func TestRenderUpgradeSync_RunningUpgradePhase(t *testing.T) {
	out := RenderUpgradeSync(nil, nil, nil, nil, nil, true /*operationRunning*/, true, 0, 0)

	lower := strings.ToLower(out)
	if !strings.Contains(lower, "upgrading") && !strings.Contains(lower, "please wait") {
		t.Errorf("RenderUpgradeSync(upgrading) should show progress; got:\n%s", out)
	}
}

// TestRenderUpgradeSync_RunningSyncPhase verifies that when upgrade is done but
// sync is still running (operationRunning=true, upgradeReport!=nil), the screen
// shows the upgrade complete indicator and sync progress.
func TestRenderUpgradeSync_RunningSyncPhase(t *testing.T) {
	report := &upgrade.UpgradeReport{
		Results: []upgrade.ToolUpgradeResult{
			{ToolName: "engram", OldVersion: "v1.0.0", NewVersion: "v2.0.0", Status: upgrade.UpgradeSucceeded},
		},
	}

	out := RenderUpgradeSync(nil, report, nil, nil, nil, true /*operationRunning*/, true, 0, 0)

	lower := strings.ToLower(out)
	// Upgrade done indicator.
	if !strings.Contains(lower, "upgrade complete") {
		t.Errorf("RenderUpgradeSync(sync phase) should show 'upgrade complete'; got:\n%s", out)
	}
	// Sync in progress.
	if !strings.Contains(lower, "syncing") && !strings.Contains(lower, "please wait") {
		t.Errorf("RenderUpgradeSync(sync phase) should show sync progress; got:\n%s", out)
	}
}

// TestRenderUpgradeSync_CombinedResult verifies that when both operations are
// done (operationRunning=false, upgradeReport!=nil), the screen shows both
// upgrade and sync results.
func TestRenderUpgradeSync_CombinedResult(t *testing.T) {
	report := &upgrade.UpgradeReport{
		Results: []upgrade.ToolUpgradeResult{
			{ToolName: "engram", OldVersion: "v1.0.0", NewVersion: "v2.0.0", Status: upgrade.UpgradeSucceeded},
		},
	}
	syncFiles := []string{"a", "b", "c"}

	out := RenderUpgradeSync(nil, report, syncFiles, nil, nil, false /*operationRunning*/, true, 0, 0)

	// Must mention both result sections.
	if !strings.Contains(out, "Upgrade Results") {
		t.Errorf("RenderUpgradeSync(combined) should show 'Upgrade Results'; got:\n%s", out)
	}
	if !strings.Contains(out, "Sync Results") {
		t.Errorf("RenderUpgradeSync(combined) should show 'Sync Results'; got:\n%s", out)
	}
	// Sync file count.
	if !strings.Contains(out, "3") {
		t.Errorf("RenderUpgradeSync(combined) should show sync file count '3'; got:\n%s", out)
	}
	// Verify individual sync file paths are rendered.
	for _, f := range syncFiles {
		if !strings.Contains(out, f) {
			t.Errorf("RenderUpgradeSync should render sync file path %q in output; got:\n%s", f, out)
		}
	}
}

func TestRenderUpgradeSync_LongManualHintUsesWidth(t *testing.T) {
	longHint := `upgrade "gentle-ai" on Windows requires manual update: irm https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/main/scripts/install.ps1 | iex`
	report := &upgrade.UpgradeReport{Results: []upgrade.ToolUpgradeResult{
		{ToolName: "gentle-ai", Status: upgrade.UpgradeSkipped, ManualHint: longHint},
	}}

	out := stripANSI(RenderUpgradeSyncWithWidth(nil, report, nil, nil, nil, false, true, 0, 0, 80))
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		if !strings.Contains(line, "requires manual update:") {
			continue
		}
		if i+1 >= len(lines) || !strings.Contains(lines[i+1], "irm") {
			t.Fatalf("hint command should start on the line after the preamble; got:\n%s", out)
		}
		if !strings.Contains(out, "install.ps1") || !strings.Contains(out, "| iex") {
			t.Fatalf("full manual command should remain visible; got:\n%s", out)
		}
		for _, wrapped := range lines[i+1:] {
			if strings.TrimSpace(wrapped) == "" {
				return
			}
			if len(wrapped) > 80 {
				t.Fatalf("manual hint line exceeds terminal width: len=%d line=%q\noutput:\n%s", len(wrapped), wrapped, out)
			}
		}
		return
	}
	t.Fatalf("hint preamble should appear in output; got:\n%s", out)
}

func TestRenderUpgradeSync_SkipsSyncWhenGentleAIUpgraded(t *testing.T) {
	report := &upgrade.UpgradeReport{Results: []upgrade.ToolUpgradeResult{
		{ToolName: "gentle-ai", OldVersion: "v1.36.1", NewVersion: "v1.36.2", Status: upgrade.UpgradeSucceeded},
	}}

	out := RenderUpgradeSync(nil, report, nil, nil, nil, false, true, 0, 0)
	lower := strings.ToLower(out)
	if !strings.Contains(lower, "sync skipped") {
		t.Fatalf("RenderUpgradeSync() should say sync was skipped after gentle-ai upgrade:\n%s", out)
	}
	if !strings.Contains(lower, "restart gentle-ai") {
		t.Fatalf("RenderUpgradeSync() should ask for restart after gentle-ai upgrade:\n%s", out)
	}
	if strings.Contains(lower, "no files needed updating") {
		t.Fatalf("RenderUpgradeSync() should not pretend sync ran after gentle-ai upgrade:\n%s", out)
	}
}

func TestRenderUpgrade_ShowsRestartNoticeWhenGentleAIUpgraded(t *testing.T) {
	report := &upgrade.UpgradeReport{Results: []upgrade.ToolUpgradeResult{
		{ToolName: "gentle-ai", OldVersion: "v1.36.1", NewVersion: "v1.36.2", Status: upgrade.UpgradeSucceeded},
	}}

	out := RenderUpgrade(nil, report, nil, false, true, 0, 0)
	if !strings.Contains(strings.ToLower(out), "restart gentle-ai") {
		t.Fatalf("RenderUpgrade() should show restart notice after gentle-ai upgrade:\n%s", out)
	}
}

// TestRenderUpgradeSync_CombinedResultEmptyUpgradeReport verifies that the
// combined results view still renders when the upgrade report has no tool
// results because everything is already up to date.
func TestRenderUpgradeSync_CombinedResultEmptyUpgradeReport(t *testing.T) {
	report := &upgrade.UpgradeReport{}

	out := RenderUpgradeSync(nil, report, nil, nil, nil, false, true, 0, 0)

	lower := strings.ToLower(out)
	if !strings.Contains(lower, "up to date") {
		t.Errorf("RenderUpgradeSync(empty upgrade report) should contain 'up to date'; got:\n%s", out)
	}
	if !strings.Contains(out, "Sync Results") {
		t.Errorf("RenderUpgradeSync(empty upgrade report) should show 'Sync Results'; got:\n%s", out)
	}
	if !strings.Contains(lower, "no files needed updating") {
		t.Errorf("RenderUpgradeSync(empty upgrade report) should preserve sync results path; got:\n%s", out)
	}
}

// TestRenderUpgradeSync_CombinedResultWithSyncError verifies that a sync error
// is shown in the combined result.
func TestRenderUpgradeSync_CombinedResultWithSyncError(t *testing.T) {
	report := &upgrade.UpgradeReport{
		Results: []upgrade.ToolUpgradeResult{},
	}
	syncErr := fmt.Errorf("permission denied writing config")

	out := RenderUpgradeSync(nil, report, nil, nil, syncErr, false, true, 0, 0)

	lower := strings.ToLower(out)
	if !strings.Contains(lower, "fail") && !strings.Contains(lower, "error") {
		t.Errorf("RenderUpgradeSync(sync error) should show failure indicator; got:\n%s", out)
	}
	if !strings.Contains(out, syncErr.Error()) {
		t.Errorf("RenderUpgradeSync(sync error) should show error text; got:\n%s", out)
	}
}

// TestRenderUpgradeSync_CombinedResultWithUpgradeError verifies that an
// upgrade error is shown in the combined result (upgradeErr != nil, report nil).
func TestRenderUpgradeSync_CombinedResultWithUpgradeError(t *testing.T) {
	upgradeErr := fmt.Errorf("network timeout during upgrade")

	out := RenderUpgradeSync(nil, nil, []string{"a", "b"}, upgradeErr, nil, false, true, 0, 0)

	if !strings.Contains(out, "Upgrade Results") {
		t.Errorf("RenderUpgradeSync(upgradeErr) should show 'Upgrade Results'; got:\n%s", out)
	}
	if !strings.Contains(out, upgradeErr.Error()) {
		t.Errorf("RenderUpgradeSync(upgradeErr) should show error text %q; got:\n%s", upgradeErr.Error(), out)
	}
	if !strings.Contains(out, "Sync Results") {
		t.Errorf("RenderUpgradeSync(upgradeErr) should show 'Sync Results'; got:\n%s", out)
	}
}

// TestRenderUpgradeSync_TitleAlwaysPresent verifies the screen title is shown.
func TestRenderUpgradeSync_TitleAlwaysPresent(t *testing.T) {
	states := []struct {
		name             string
		report           *upgrade.UpgradeReport
		operationRunning bool
		updateCheckDone  bool
	}{
		{"confirm", nil, false, true},
		{"checking", nil, false, false},
		{"upgrading", nil, true, true},
	}

	for _, s := range states {
		t.Run(s.name, func(t *testing.T) {
			out := RenderUpgradeSync(nil, s.report, nil, nil, nil, s.operationRunning, s.updateCheckDone, 0, 0)
			if !strings.Contains(out, "Upgrade + Sync") {
				t.Errorf("RenderUpgradeSync state=%q should contain 'Upgrade + Sync'; got:\n%s", s.name, out)
			}
		})
	}
}

// TestRenderUpgradeSync_CheckingState verifies that when updateCheckDone=false
// and not running, the screen shows a "checking" indicator.
func TestRenderUpgradeSync_CheckingState(t *testing.T) {
	results := []update.UpdateResult{}
	out := RenderUpgradeSync(results, nil, nil, nil, nil, false, false /*updateCheckDone=false*/, 0, 0)

	lower := strings.ToLower(out)
	if !strings.Contains(lower, "check") {
		t.Errorf("RenderUpgradeSync(!updateCheckDone) should show 'check'; got:\n%s", out)
	}
}

// TestRenderUpgradeSync_TruncatesLargeFileList verifies that when more than
// maxFilesToShow sync files changed, the output shows a truncation indicator.
func TestRenderUpgradeSync_TruncatesLargeFileList(t *testing.T) {
	report := &upgrade.UpgradeReport{
		Results: []upgrade.ToolUpgradeResult{
			{ToolName: "engram", OldVersion: "v1.0.0", NewVersion: "v2.0.0", Status: upgrade.UpgradeSucceeded},
		},
	}
	files := make([]string, maxFilesToShow+3)
	for i := range files {
		files[i] = fmt.Sprintf("file-%d.txt", i)
	}

	out := RenderUpgradeSync(nil, report, files, nil, nil, false, true, 0, 0)

	if !strings.Contains(out, "file-0.txt") {
		t.Errorf("should render first file; got:\n%s", out)
	}
	if !strings.Contains(out, "and 3 more") {
		t.Errorf("should show truncation message; got:\n%s", out)
	}
}
