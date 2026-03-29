package app

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/update/upgrade"
)

// renderUpgradeReportForTest is a test helper that wraps upgrade.RenderUpgradeReport.
// It bridges the test assertions to the actual render function.
func renderUpgradeReportForTest(results []upgrade.ToolUpgradeResult, dryRun bool) string {
	return upgrade.RenderUpgradeReport(upgrade.UpgradeReport{
		Results: results,
		DryRun:  dryRun,
	})
}

// --- TestRunArgs_UpgradeDryRunFlag ---

// TestRunArgs_UpgradeDryRun verifies that `gentle-ai upgrade --dry-run` runs without
// error, outputs relevant messaging, and does NOT attempt any real installation.
// The environment has no tools installed, so no upgrades are available.
func TestRunArgs_UpgradeDryRun(t *testing.T) {
	var buf bytes.Buffer
	// RunArgs calls system.Detect which may fail in headless CI — we rely on the
	// subcommand path being short-circuited before the TUI is launched.
	err := RunArgs([]string{"upgrade", "--dry-run"}, &buf)
	if err != nil {
		if !strings.Contains(err.Error(), "update check failed") {
			t.Fatalf("RunArgs(upgrade --dry-run) error = %v", err)
		}
	}

	out := buf.String()

	// Must mention it is dry-run or no-op.
	if !strings.Contains(out, "dry") && !strings.Contains(out, "Dry") &&
		!strings.Contains(out, "no upgrade") && !strings.Contains(out, "No upgrade") &&
		!strings.Contains(out, "up to date") && !strings.Contains(out, "Up to date") &&
		!strings.Contains(out, "Update check incomplete") &&
		!strings.Contains(out, "0 upgrade") {
		t.Logf("upgrade --dry-run output:\n%s", out)
		t.Errorf("output should mention dry-run or no upgrades available")
	}

	// Must NOT mention install or sync.
	if strings.Contains(out, "Installing") || strings.Contains(out, "syncing") || strings.Contains(out, "Syncing") {
		t.Errorf("upgrade must not invoke install or sync pipelines; got: %s", out)
	}
}

// TestRunArgs_UpgradeNoArgs runs `gentle-ai upgrade` without flags.
// With no updates available in the test environment, it should exit cleanly.
func TestRunArgs_UpgradeNoArgs(t *testing.T) {
	var buf bytes.Buffer
	err := RunArgs([]string{"upgrade"}, &buf)
	// Allow error only if it's a network/check failure, not a missing command error.
	if err != nil {
		errStr := err.Error()
		// "unknown command" means the command is not wired — that's a failure.
		if strings.Contains(errStr, "unknown command") {
			t.Fatalf("upgrade command is not registered: %v", err)
		}
		// Network failures in CI are acceptable for this test.
		t.Logf("upgrade got non-fatal error (likely network): %v", err)
	}
}

// TestRunArgs_UpgradeToolFilter verifies that `gentle-ai upgrade engram` filters
// to only check/upgrade engram.
func TestRunArgs_UpgradeToolFilter(t *testing.T) {
	var buf bytes.Buffer
	err := RunArgs([]string{"upgrade", "engram"}, &buf)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "unknown command") {
			t.Fatalf("upgrade command is not registered: %v", err)
		}
		t.Logf("upgrade engram got non-fatal error (likely network/not installed): %v", err)
	}

	out := buf.String()
	// Output should only mention engram or no-upgrades, not gentle-ai or gga.
	// This is a soft check since the tool may not be installed.
	if strings.Contains(out, "gentle-ai") && !strings.Contains(out, "engram") {
		t.Errorf("filtering to engram should not show gentle-ai in output; got: %s", out)
	}
}

// TestRunArgs_UpgradeOutput_BinariesOnly verifies the output messaging states
// that upgrade is binary-only and does not re-run install/sync.
func TestRunArgs_UpgradeOutput_BinariesOnly(t *testing.T) {
	var buf bytes.Buffer
	err := RunArgs([]string{"upgrade", "--dry-run"}, &buf)
	if err != nil {
		if !strings.Contains(err.Error(), "update check failed") {
			t.Fatalf("upgrade --dry-run: %v", err)
		}
	}

	out := buf.String()
	t.Logf("upgrade --dry-run output:\n%s", out)

	// The output must not suggest install/sync is running.
	forbidden := []string{
		"Running install",
		"Syncing agent",
		"pipeline",
	}
	for _, word := range forbidden {
		if strings.Contains(out, word) {
			t.Errorf("output must not contain %q — upgrade should not invoke install/sync: %s", word, out)
		}
	}
}

// TestRenderUpgradeReport_Empty verifies the render function for an empty upgrade report.
func TestRenderUpgradeReport_Empty(t *testing.T) {
	// Import the render function via the update package.
	// If RenderUpgradeReport doesn't exist yet, this test fails.
	out := renderUpgradeReportForTest(nil, false)
	if out == "" {
		t.Errorf("RenderUpgradeReport with empty results should still produce some output")
	}
	if !strings.Contains(out, "upgrade") && !strings.Contains(out, "Upgrade") &&
		!strings.Contains(out, "up to date") && !strings.Contains(out, "Up to date") {
		t.Errorf("empty upgrade report should mention 'upgrade' or 'up to date', got: %q", out)
	}
}

// TestRenderUpgradeReport_DryRun verifies that dry-run report mentions dry-run.
func TestRenderUpgradeReport_DryRun(t *testing.T) {
	out := renderUpgradeReportForTest(nil, true)
	if !strings.Contains(out, "dry") && !strings.Contains(out, "Dry") {
		t.Errorf("dry-run report should mention dry-run, got: %q", out)
	}
}

// TestRenderUpgradeReport_PerToolSemantics_Deterministic verifies that the render
// function produces deterministic output covering success, skip (dev-build), and
// manual-fallback cases without relying on network or installed binaries.
// This closes Gap 4 from the verify report: deterministic CLI coverage for
// per-tool success/skip/manual output semantics.
func TestRenderUpgradeReport_PerToolSemantics_Deterministic(t *testing.T) {
	tests := []struct {
		name           string
		results        []upgrade.ToolUpgradeResult
		dryRun         bool
		wantContains   []string
		wantNotContain []string
	}{
		{
			name: "succeeded tool shows old→new version",
			results: []upgrade.ToolUpgradeResult{
				{
					ToolName:   "engram",
					OldVersion: "0.3.0",
					NewVersion: "0.4.0",
					Status:     upgrade.UpgradeSucceeded,
				},
			},
			wantContains:   []string{"engram", "0.3.0", "0.4.0", "[ok]"},
			wantNotContain: []string{"FAILED", "manual update required"},
		},
		{
			name: "dev-build skipped shows skipped status not failure",
			results: []upgrade.ToolUpgradeResult{
				{
					ToolName:   "gentle-ai",
					OldVersion: "dev",
					NewVersion: "1.0.0",
					Status:     upgrade.UpgradeSkipped,
					ManualHint: "source build — upgrade manually or install a release binary",
				},
			},
			wantContains:   []string{"gentle-ai", "[--]", "source build"},
			wantNotContain: []string{"[!!]", "FAILED"},
		},
		{
			name: "manual fallback shows hint not failure",
			results: []upgrade.ToolUpgradeResult{
				{
					ToolName:   "gentle-ai",
					OldVersion: "1.0.0",
					NewVersion: "1.5.0",
					Status:     upgrade.UpgradeSkipped,
					ManualHint: "Download from https://github.com/Gentleman-Programming/gentle-ai/releases",
				},
			},
			wantContains:   []string{"gentle-ai", "manual update required", "github.com", "[--]"},
			wantNotContain: []string{"[!!]", "FAILED"},
		},
		{
			name: "real failure shows error details",
			results: []upgrade.ToolUpgradeResult{
				{
					ToolName:   "gga",
					OldVersion: "1.0.0",
					NewVersion: "2.0.0",
					Status:     upgrade.UpgradeFailed,
					Err:        errors.New("brew upgrade gga: exit status 1"),
				},
			},
			wantContains:   []string{"gga", "FAILED", "exit status 1", "[!!]"},
			wantNotContain: []string{"manual update required"},
		},
		{
			name:   "dry-run shows pending upgrades",
			dryRun: true,
			results: []upgrade.ToolUpgradeResult{
				{
					ToolName:   "engram",
					OldVersion: "0.3.0",
					NewVersion: "0.4.0",
					Status:     upgrade.UpgradeSkipped,
				},
			},
			wantContains:   []string{"dry", "engram", "0.3.0", "0.4.0"},
			wantNotContain: []string{"FAILED"},
		},
		{
			name: "mixed: success + skip + manual in same report",
			results: []upgrade.ToolUpgradeResult{
				{
					ToolName:   "engram",
					OldVersion: "0.3.0",
					NewVersion: "0.4.0",
					Status:     upgrade.UpgradeSucceeded,
				},
				{
					ToolName:   "gentle-ai",
					OldVersion: "dev",
					NewVersion: "1.5.0",
					Status:     upgrade.UpgradeSkipped,
					ManualHint: "source build — upgrade manually",
				},
				{
					ToolName:   "gga",
					OldVersion: "1.0.0",
					NewVersion: "2.0.0",
					Status:     upgrade.UpgradeSkipped,
					ManualHint: "Download from https://github.com/Gentleman-Programming/gga/releases",
				},
			},
			wantContains:   []string{"engram", "[ok]", "gentle-ai", "[--]", "gga", "1 succeeded", "2 skipped"},
			wantNotContain: []string{"FAILED", "[!!]"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := renderUpgradeReportForTest(tc.results, tc.dryRun)
			t.Logf("output:\n%s", out)

			for _, want := range tc.wantContains {
				if !strings.Contains(out, want) {
					t.Errorf("output must contain %q\nfull output:\n%s", want, out)
				}
			}
			for _, notWant := range tc.wantNotContain {
				if strings.Contains(out, notWant) {
					t.Errorf("output must NOT contain %q\nfull output:\n%s", notWant, out)
				}
			}
		})
	}
}
