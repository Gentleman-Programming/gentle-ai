package screens

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/update"
	"github.com/gentleman-programming/gentle-ai/internal/update/upgrade"
)

// ─── RenderUpgrade states ──────────────────────────────────────────────────

// TestRenderUpgrade_CheckingState verifies that when the update check has not
// completed yet, the upgrade screen shows a "Checking" indicator.
func TestRenderUpgrade_CheckingState(t *testing.T) {
	out := RenderUpgrade(nil, nil, nil, false, false /*updateCheckDone=false*/, 0, 0)

	if !strings.Contains(strings.ToLower(out), "check") {
		t.Errorf("RenderUpgrade(!updateCheckDone) should contain 'check'; got:\n%s", out)
	}
}

// TestRenderUpgrade_AllUpToDate verifies that when the update check is done and
// all tools are up to date, the screen shows an "up to date" message.
func TestRenderUpgrade_AllUpToDate(t *testing.T) {
	results := []update.UpdateResult{
		{
			Tool:             update.ToolInfo{Name: "gentle-ai"},
			InstalledVersion: "v1.0.0",
			LatestVersion:    "v1.0.0",
			Status:           update.UpToDate,
		},
	}

	out := RenderUpgrade(results, nil, nil, false, true /*updateCheckDone*/, 0, 0)

	lower := strings.ToLower(out)
	if !strings.Contains(lower, "up to date") {
		t.Errorf("RenderUpgrade(all up to date) should contain 'up to date'; got:\n%s", out)
	}
}

// TestRenderUpgrade_UpdatesAvailable verifies that when an update is available,
// the screen shows the tool name and version information.
func TestRenderUpgrade_UpdatesAvailable(t *testing.T) {
	results := []update.UpdateResult{
		{
			Tool:             update.ToolInfo{Name: "gentle-ai"},
			InstalledVersion: "v1.0.0",
			LatestVersion:    "v2.0.0",
			Status:           update.UpdateAvailable,
		},
	}

	out := RenderUpgrade(results, nil, nil, false, true /*updateCheckDone*/, 0, 0)

	if !strings.Contains(out, "gentle-ai") {
		t.Errorf("RenderUpgrade(updates available) should contain tool name 'gentle-ai'; got:\n%s", out)
	}
	if !strings.Contains(out, "v1.0.0") || !strings.Contains(out, "v2.0.0") {
		t.Errorf("RenderUpgrade(updates available) should contain version info; got:\n%s", out)
	}
}

// TestRenderUpgrade_ResultState verifies that when an upgrade report is
// available, the screen shows upgrade results.
func TestRenderUpgrade_ResultState(t *testing.T) {
	report := &upgrade.UpgradeReport{
		Results: []upgrade.ToolUpgradeResult{
			{
				ToolName:   "gentle-ai",
				OldVersion: "v1.0.0",
				NewVersion: "v2.0.0",
				Status:     upgrade.UpgradeSucceeded,
			},
		},
	}

	out := RenderUpgrade(nil, report, nil, false, true, 0, 0)

	lower := strings.ToLower(out)
	if !strings.Contains(lower, "upgraded") && !strings.Contains(lower, "summary") &&
		!strings.Contains(out, "gentle-ai") {
		t.Errorf("RenderUpgrade(report) should show upgrade results; got:\n%s", out)
	}
}

// TestRenderUpgrade_ResultStateEmptyReport verifies that an empty upgrade report
// still renders the completed result path with an "up to date" message.
func TestRenderUpgrade_ResultStateEmptyReport(t *testing.T) {
	report := &upgrade.UpgradeReport{}

	out := RenderUpgrade(nil, report, nil, false, true, 0, 0)

	lower := strings.ToLower(out)
	if !strings.Contains(lower, "up to date") {
		t.Errorf("RenderUpgrade(empty report) should contain 'up to date'; got:\n%s", out)
	}
	if !strings.Contains(lower, "return") {
		t.Errorf("RenderUpgrade(empty report) should contain return hint; got:\n%s", out)
	}
}

// TestRenderUpgrade_RunningState verifies that while an upgrade is running
// (operationRunning=true, report=nil), the screen shows a spinner/progress indicator.
func TestRenderUpgrade_RunningState(t *testing.T) {
	out := RenderUpgrade(nil, nil, nil, true /*operationRunning*/, true, 0, 0)

	lower := strings.ToLower(out)
	if !strings.Contains(lower, "upgrading") && !strings.Contains(lower, "please wait") {
		t.Errorf("RenderUpgrade(running) should show 'upgrading' or 'please wait'; got:\n%s", out)
	}
}

// TestRenderUpgrade_ErrorState verifies that when upgradeErr is set and report
// is nil, the screen shows the error message and a "return" hint.
func TestRenderUpgrade_ErrorState(t *testing.T) {
	upgradeErr := fmt.Errorf("upgrade failed: connection timeout")
	out := RenderUpgrade(nil, nil, upgradeErr, false /*operationRunning*/, true /*updateCheckDone*/, 0, 0)

	if !strings.Contains(out, "upgrade failed: connection timeout") {
		t.Errorf("RenderUpgrade(upgradeErr) should contain error text; got:\n%s", out)
	}
	if !strings.Contains(strings.ToLower(out), "return") {
		t.Errorf("RenderUpgrade(upgradeErr) should contain 'return' hint; got:\n%s", out)
	}
}

// TestRenderUpgrade_TitleAlwaysPresent verifies that the "Upgrade Tools" title
// is always present regardless of state.
func TestRenderUpgrade_TitleAlwaysPresent(t *testing.T) {
	states := []struct {
		name             string
		results          []update.UpdateResult
		report           *upgrade.UpgradeReport
		operationRunning bool
		updateCheckDone  bool
	}{
		{"checking", nil, nil, false, false},
		{"up to date", nil, nil, false, true},
		{"running", nil, nil, true, true},
	}

	for _, s := range states {
		t.Run(s.name, func(t *testing.T) {
			out := RenderUpgrade(s.results, s.report, nil, s.operationRunning, s.updateCheckDone, 0, 0)
			if !strings.Contains(out, "Upgrade Tools") {
				t.Errorf("RenderUpgrade state=%q should contain 'Upgrade Tools'; got:\n%s", s.name, out)
			}
		})
	}
}
