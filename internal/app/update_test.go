package app

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/system"
	"github.com/gentleman-programming/gentle-ai/internal/update"
	"github.com/gentleman-programming/gentle-ai/internal/update/upgrade"
)

func TestRunUpdate_ReturnsErrorWhenChecksFail(t *testing.T) {
	origCheckAll := updateCheckAll
	t.Cleanup(func() {
		updateCheckAll = origCheckAll
	})

	updateCheckAll = func(context.Context, string, system.PlatformProfile) []update.UpdateResult {
		return []update.UpdateResult{{
			Tool:   update.ToolInfo{Name: "engram"},
			Status: update.CheckFailed,
		}}
	}

	var buf bytes.Buffer
	err := runUpdate(context.Background(), "1.0.0", system.PlatformProfile{OS: "darwin", PackageManager: "brew"}, &buf)
	if err == nil {
		t.Fatal("runUpdate() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "update check failed for: engram") {
		t.Fatalf("runUpdate() error = %v, want update check failure", err)
	}

	out := buf.String()
	if strings.Contains(out, "All tools are up to date!") {
		t.Fatalf("runUpdate() output incorrectly claimed tools are up to date:\n%s", out)
	}
	if !strings.Contains(out, "Update check incomplete") {
		t.Fatalf("runUpdate() output missing incomplete check warning:\n%s", out)
	}
}

func TestRunUpgrade_ReturnsErrorBeforeExecutingWhenChecksFail(t *testing.T) {
	origCheckFiltered := updateCheckFiltered
	origUpgradeExecute := upgradeExecute
	t.Cleanup(func() {
		updateCheckFiltered = origCheckFiltered
		upgradeExecute = origUpgradeExecute
	})

	called := false
	updateCheckFiltered = func(context.Context, string, system.PlatformProfile, []string) []update.UpdateResult {
		return []update.UpdateResult{
			{
				Tool:   update.ToolInfo{Name: "engram"},
				Status: update.CheckFailed,
			},
			{
				Tool:             update.ToolInfo{Name: "gga"},
				InstalledVersion: "1.0.0",
				LatestVersion:    "2.0.0",
				Status:           update.UpdateAvailable,
			},
		}
	}
	upgradeExecute = func(context.Context, []update.UpdateResult, system.PlatformProfile, string, bool, ...io.Writer) upgrade.UpgradeReport {
		called = true
		return upgrade.UpgradeReport{}
	}

	var buf bytes.Buffer
	err := runUpgrade(context.Background(), nil, system.DetectionResult{System: system.SystemInfo{Profile: system.PlatformProfile{OS: "darwin", PackageManager: "brew", Supported: true}}}, &buf)
	if err == nil {
		t.Fatal("runUpgrade() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "update check failed for: engram") {
		t.Fatalf("runUpgrade() error = %v, want update check failure", err)
	}
	if called {
		t.Fatal("runUpgrade() executed upgrades despite failed checks")
	}

	out := buf.String()
	if !strings.Contains(out, "Update Check") {
		t.Fatalf("runUpgrade() output missing check report:\n%s", out)
	}
	if strings.Contains(out, "All tools are up to date!") {
		t.Fatalf("runUpgrade() output incorrectly claimed tools are up to date:\n%s", out)
	}
	if strings.Contains(out, "Upgrade\n") {
		t.Fatalf("runUpgrade() should stop before rendering upgrade report:\n%s", out)
	}
}
