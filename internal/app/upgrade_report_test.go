package app

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/system"
	"github.com/gentleman-programming/gentle-ai/v3/internal/update"
	"github.com/gentleman-programming/gentle-ai/v3/internal/update/upgrade"
)

// stubUpgradeSeams pins detectSystem, updateCheckFiltered and
// upgradeExecuteWithOptions so RunUpgradeReport exercises the real
// classification without network, package managers or binary replacement.
func stubUpgradeSeams(t *testing.T, check []update.UpdateResult, report upgrade.UpgradeReport) {
	t.Helper()
	origDetect := detectSystem
	origCheckFiltered := updateCheckFiltered
	origUpgradeExecuteWithOptions := upgradeExecuteWithOptions
	t.Cleanup(func() {
		detectSystem = origDetect
		updateCheckFiltered = origCheckFiltered
		upgradeExecuteWithOptions = origUpgradeExecuteWithOptions
	})

	detectSystem = func(context.Context) (system.DetectionResult, error) {
		return system.DetectionResult{System: system.SystemInfo{Profile: system.PlatformProfile{
			OS:             "darwin",
			PackageManager: "brew",
			Supported:      true,
		}}}, nil
	}
	updateCheckFiltered = func(context.Context, string, system.PlatformProfile, []string) []update.UpdateResult {
		return check
	}
	upgradeExecuteWithOptions = func(context.Context, []update.UpdateResult, system.PlatformProfile, string, bool, upgrade.ExecuteOptions) upgrade.UpgradeReport {
		return report
	}
}

func TestRunUpgradeReport_ClassifiesOutcomes(t *testing.T) {
	tests := []struct {
		name            string
		results         []upgrade.ToolUpgradeResult
		wantStatus      string
		wantRestart     bool
		wantManualHint  string
		wantErr         bool
		wantToolStatus  []string
		wantToolCount   int
	}{
		{
			name: "all tools succeeded without self-tool restart",
			results: []upgrade.ToolUpgradeResult{
				{ToolName: "engram", Status: upgrade.UpgradeSucceeded, NewVersion: "0.4.0"},
				{ToolName: "gga", Status: upgrade.UpgradeSucceeded, NewVersion: "1.1.0"},
			},
			wantStatus:     "succeeded",
			wantToolStatus: []string{"succeeded", "succeeded"},
			wantToolCount:  2,
		},
		{
			name: "self-tool succeeded sets restart required",
			results: []upgrade.ToolUpgradeResult{
				{ToolName: "axiom", Status: upgrade.UpgradeSucceeded, NewVersion: "1.5.0"},
				{ToolName: "engram", Status: upgrade.UpgradeSucceeded, NewVersion: "0.4.0"},
			},
			wantStatus:     "succeeded",
			wantRestart:    true,
			wantToolStatus: []string{"succeeded", "succeeded"},
			wantToolCount:  2,
		},
		{
			name: "legacy self-tool name also sets restart required",
			results: []upgrade.ToolUpgradeResult{
				{ToolName: "gentle-ai", Status: upgrade.UpgradeSucceeded, NewVersion: "1.5.0"},
			},
			wantStatus:     "succeeded",
			wantRestart:    true,
			wantToolStatus: []string{"succeeded"},
			wantToolCount:  1,
		},
		{
			name: "self-tool manual fallback reports skipped and hint",
			results: []upgrade.ToolUpgradeResult{
				{ToolName: "axiom", Status: upgrade.UpgradeSkipped, ManualHint: "clone and build IGutierrezZ/axiom"},
			},
			wantStatus:     "skipped",
			wantManualHint: "clone and build IGutierrezZ/axiom",
			wantToolStatus: []string{"skipped"},
			wantToolCount:  1,
		},
		{
			name: "non-self tool skip does not set manual hint",
			results: []upgrade.ToolUpgradeResult{
				{ToolName: "engram", Status: upgrade.UpgradeSkipped, ManualHint: "detect version first"},
			},
			wantStatus:     "succeeded",
			wantToolStatus: []string{"skipped"},
			wantToolCount:  1,
		},
		{
			name: "any tool failure classifies the run as failed",
			results: []upgrade.ToolUpgradeResult{
				{ToolName: "engram", Status: upgrade.UpgradeFailed, Err: errors.New("exit status 1")},
				{ToolName: "axiom", Status: upgrade.UpgradeSucceeded, NewVersion: "1.5.0"},
			},
			wantStatus:     "failed",
			wantRestart:    true,
			wantErr:        true,
			wantToolStatus: []string{"failed", "succeeded"},
			wantToolCount:  2,
		},
		{
			name:           "empty result set is succeeded with no tools",
			results:        nil,
			wantStatus:     "succeeded",
			wantToolStatus: nil,
			wantToolCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check := []update.UpdateResult{{Tool: update.ToolInfo{Name: "engram"}, Status: update.UpdateAvailable}}
			stubUpgradeSeams(t, check, upgrade.UpgradeReport{Results: tt.results})

			var buf bytes.Buffer
			report, err := RunUpgradeReport(context.Background(), &buf)
			if tt.wantErr {
				if err == nil {
					t.Fatal("RunUpgradeReport() error = nil, want tool failure error")
				}
			} else if err != nil {
				t.Fatalf("RunUpgradeReport() error = %v, want nil", err)
			}

			if report.Status != tt.wantStatus {
				t.Errorf("Status = %q, want %q", report.Status, tt.wantStatus)
			}
			if report.RestartRequired != tt.wantRestart {
				t.Errorf("RestartRequired = %v, want %v", report.RestartRequired, tt.wantRestart)
			}
			if report.ManualHint != tt.wantManualHint {
				t.Errorf("ManualHint = %q, want %q", report.ManualHint, tt.wantManualHint)
			}
			if len(report.PerTool) != tt.wantToolCount {
				t.Fatalf("len(PerTool) = %d, want %d", len(report.PerTool), tt.wantToolCount)
			}
			for i, want := range tt.wantToolStatus {
				if report.PerTool[i].Status != want {
					t.Errorf("PerTool[%d].Status = %q, want %q", i, report.PerTool[i].Status, want)
				}
			}
		})
	}
}

func TestRunUpgradeReport_RestartRequiredOnlyForSelfToolSuccess(t *testing.T) {
	tests := []struct {
		name        string
		toolName    string
		status      upgrade.ToolUpgradeStatus
		wantRestart bool
	}{
		{name: "self-tool axiom succeeded", toolName: "axiom", status: upgrade.UpgradeSucceeded, wantRestart: true},
		{name: "self-tool gentle-ai succeeded", toolName: "gentle-ai", status: upgrade.UpgradeSucceeded, wantRestart: true},
		{name: "self-tool skipped", toolName: "axiom", status: upgrade.UpgradeSkipped, wantRestart: false},
		{name: "self-tool failed", toolName: "axiom", status: upgrade.UpgradeFailed, wantRestart: false},
		{name: "other tool succeeded", toolName: "engram", status: upgrade.UpgradeSucceeded, wantRestart: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubUpgradeSeams(t,
				[]update.UpdateResult{{Tool: update.ToolInfo{Name: tt.toolName}, Status: update.UpdateAvailable}},
				upgrade.UpgradeReport{Results: []upgrade.ToolUpgradeResult{{
					ToolName: tt.toolName,
					Status:   tt.status,
				}}})

			report, err := RunUpgradeReport(context.Background(), &bytes.Buffer{})
			if err != nil {
				t.Fatalf("RunUpgradeReport() error = %v", err)
			}
			if report.RestartRequired != tt.wantRestart {
				t.Fatalf("RestartRequired = %v, want %v", report.RestartRequired, tt.wantRestart)
			}
		})
	}
}

func TestResolveSyncSkip(t *testing.T) {
	tests := []struct {
		name       string
		report     UpgradeRunReport
		wantSkip   bool
		wantReason string
	}{
		{
			name:     "restart required skips sync",
			report:   UpgradeRunReport{Status: "succeeded", RestartRequired: true},
			wantSkip: true, wantReason: "restart-required",
		},
		{
			name:     "fatal upgrade failure skips sync",
			report:   UpgradeRunReport{Status: "failed"},
			wantSkip: true, wantReason: "upgrade-failed",
		},
		{
			name:     "happy path runs sync",
			report:   UpgradeRunReport{Status: "succeeded"},
			wantSkip: false, wantReason: "",
		},
		{
			name:     "manual self-tool fallback without replacement runs sync",
			report:   UpgradeRunReport{Status: "skipped", ManualHint: "build manually"},
			wantSkip: false, wantReason: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skip, reason := ResolveSyncSkip(tt.report)
			if skip != tt.wantSkip {
				t.Fatalf("ResolveSyncSkip() skip = %v, want %v", skip, tt.wantSkip)
			}
			if reason != tt.wantReason {
				t.Fatalf("ResolveSyncSkip() reason = %q, want %q", reason, tt.wantReason)
			}
		})
	}
}

func TestRunUpgradeReport_CheckFailureIsFatalWithoutExecution(t *testing.T) {
	origDetect := detectSystem
	origCheckFiltered := updateCheckFiltered
	origUpgradeExecuteWithOptions := upgradeExecuteWithOptions
	t.Cleanup(func() {
		detectSystem = origDetect
		updateCheckFiltered = origCheckFiltered
		upgradeExecuteWithOptions = origUpgradeExecuteWithOptions
	})

	detectSystem = func(context.Context) (system.DetectionResult, error) {
		return system.DetectionResult{}, nil
	}
	updateCheckFiltered = func(context.Context, string, system.PlatformProfile, []string) []update.UpdateResult {
		return []update.UpdateResult{{Tool: update.ToolInfo{Name: "engram"}, Status: update.CheckFailed}}
	}
	upgradeExecuteWithOptions = func(context.Context, []update.UpdateResult, system.PlatformProfile, string, bool, upgrade.ExecuteOptions) upgrade.UpgradeReport {
		t.Fatal("upgradeExecuteWithOptions must not run after a failed update check")
		return upgrade.UpgradeReport{}
	}

	var buf bytes.Buffer
	report, err := RunUpgradeReport(context.Background(), &buf)
	if err == nil {
		t.Fatal("RunUpgradeReport() error = nil, want update check failure")
	}
	if !strings.Contains(err.Error(), "update check failed") {
		t.Fatalf("RunUpgradeReport() error = %v, want update check failure", err)
	}
	if report.Status != "failed" {
		t.Fatalf("Status = %q, want failed", report.Status)
	}
}
