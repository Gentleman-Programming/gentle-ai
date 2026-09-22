package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/gentleman-programming/gentle-ai/v3/internal/cli"
	"github.com/gentleman-programming/gentle-ai/v3/internal/system"
	"github.com/gentleman-programming/gentle-ai/v3/internal/update"
	"github.com/gentleman-programming/gentle-ai/v3/internal/update/upgrade"
)

// Upgrade status literals for UpgradeRunReport.Status and
// UpgradeToolOutcome.Status. They mirror upgrade.ToolUpgradeStatus so
// consumers that only read the structured report need no upgrade import (D-06).
const (
	UpgradeStatusSucceeded = "succeeded"
	UpgradeStatusFailed    = "failed"
	UpgradeStatusSkipped   = "skipped"
)

// UpgradeToolOutcome is the structured outcome of one tool inside a
// binary-only upgrade run (D-06). Status carries the executor status verbatim:
// "succeeded", "failed" or "skipped".
type UpgradeToolOutcome struct {
	ToolName   string
	Status     string
	NewVersion string
	// ManualHint is populated when Status is "skipped" and the executor
	// supplied an actionable manual fallback (ManualFallbackError.Hint).
	ManualHint string
}

// UpgradeRunReport is the structured result of a binary-only upgrade run
// (D-06). It is derived from executor structure, never from parsed prose.
//
// Status is "failed" when any tool failed, "skipped" when the self-tool ended
// in a manual fallback (nothing failed, but the fork binary was not replaced),
// and "succeeded" otherwise.
type UpgradeRunReport struct {
	PerTool []UpgradeToolOutcome
	// SelfToolName is the registry name of the self-tool after init()
	// ("axiom" in cmd/axiom, "gentle-ai" before the rename mutates it).
	SelfToolName string
	// RestartRequired is true only when the self-tool terminated in
	// UpgradeSucceeded, i.e. the running binary was replaced (REQ-22.6).
	// It is derived from the shared identity predicate (D-08).
	RestartRequired bool
	// ManualHint is populated when the self-tool outcome is "skipped".
	ManualHint string
	Status     string
}

// ResolveSyncSkip encodes the post-upgrade sync skip rule (REQ-22.6): when the
// running axiom binary was replaced, sync MUST NOT run and MUST be reported
// with an explicit reason. A fatal upgrade failure also skips sync. This is the
// single source of truth for Web UI and TUI (D-08).
//
//	u.RestartRequired    -> true,  "restart-required"
//	u.Status == "failed" -> true,  "upgrade-failed"
//	otherwise            -> false, ""
func ResolveSyncSkip(u UpgradeRunReport) (bool, string) {
	if u.RestartRequired {
		return true, "restart-required"
	}
	if u.Status == UpgradeStatusFailed {
		return true, "upgrade-failed"
	}
	return false, ""
}

// RunUpgradeReport runs the same binary-only upgrade the `upgrade` verb runs
// (never install, never sync) and returns the structured outcome. It performs
// system detection internally so the dashboard upgrade->sync sequence can call
// it without reproducing RunArgs dispatch. stdout receives exactly the bytes
// the upgrade verb prints.
//
// The returned error is the upgrade verb's error: only per-tool failures are
// fatal. Manual fallbacks are "skipped", never "failed".
func RunUpgradeReport(ctx context.Context, stdout io.Writer) (UpgradeRunReport, error) {
	detection, err := detectSystem(ctx)
	if err != nil {
		return UpgradeRunReport{Status: UpgradeStatusFailed}, fmt.Errorf("detect system: %w", err)
	}
	return runUpgradeReport(ctx, upgradeArgs{}, detection, stdout)
}

// runUpgradeReport holds the binary-only upgrade logic shared by the CLI verb
// and RunUpgradeReport. It executes exactly what runUpgrade executed before the
// structured report existed and additionally classifies the outcome (D-06).
// Issue #535: args are the once-parsed upgradeArgs; this function never
// reparses raw CLI arguments.
func runUpgradeReport(ctx context.Context, args upgradeArgs, detection system.DetectionResult, stdout io.Writer) (UpgradeRunReport, error) {
	dryRun := args.dryRun
	noBackup := args.noBackup
	toolFilter := args.toolFilter

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return UpgradeRunReport{Status: UpgradeStatusFailed}, fmt.Errorf("resolve home directory: %w", err)
	}

	profile := cli.ResolveInstallProfile(detection)

	// Check for available updates (filtered to requested tools if specified).
	sp := upgrade.NewSpinner(stdout, "Checking for updates")
	checkResults := updateCheckFiltered(ctx, Version, profile, toolFilter)
	checkErr := updateCheckError(checkResults)
	sp.Finish(checkErr == nil)
	if checkErr != nil {
		_, _ = fmt.Fprint(stdout, update.RenderCLI(checkResults))
		return UpgradeRunReport{Status: UpgradeStatusFailed}, checkErr
	}

	// Execute upgrades (no-op if nothing is UpdateAvailable). Use the options
	// seam so CLI-only flags (e.g. --no-backup) remain testable without invoking
	// real package-manager strategies.
	execReport := upgradeExecuteWithOptions(ctx, checkResults, profile, homeDir, dryRun, upgrade.ExecuteOptions{
		Progress:          stdout,
		BackupDiagnostics: stdout,
		SkipBackup:        noBackup,
	})

	_, _ = fmt.Fprint(stdout, upgrade.RenderUpgradeReport(execReport))

	structured := classifyUpgradeRun(execReport)

	// Return error only if any tool failed (not for skipped/manual).
	var errs []error
	for _, r := range execReport.Results {
		if r.Status == upgrade.UpgradeFailed && r.Err != nil {
			errs = append(errs, fmt.Errorf("upgrade failed for %q: %w", r.ToolName, r.Err))
		}
	}

	if err := errors.Join(errs...); err != nil {
		return structured, err
	}
	if !dryRun {
		if latestVersion, ok := gentleAIUpgradeSucceeded(execReport); ok {
			if err := restartAfterGentleAIUpgrade(latestVersion, stdout); err != nil {
				return structured, err
			}
			// CLI upgrade path: print the doctor advisory so the user can verify
			// ecosystem health against the post-upgrade state. Informational only;
			// does not run any checks or change exit status.
			printPostUpgradeDoctorAdvisory(stdout)
			return structured, nil
		}
	}
	return structured, nil
}

// classifyUpgradeRun derives the structured report from executor results. The
// restart flag and the manual hint come from the shared identity predicate, so
// a rename of the self-tool can never disable them (REQ-22.3, D-08).
func classifyUpgradeRun(report upgrade.UpgradeReport) UpgradeRunReport {
	structured := UpgradeRunReport{
		SelfToolName: selfToolRegistryName(),
		Status:       UpgradeStatusSucceeded,
	}
	anyFailed := false
	selfSkipped := false

	for _, r := range report.Results {
		structured.PerTool = append(structured.PerTool, UpgradeToolOutcome{
			ToolName:   r.ToolName,
			Status:     string(r.Status),
			NewVersion: r.NewVersion,
			ManualHint: r.ManualHint,
		})
		if r.Status == upgrade.UpgradeFailed {
			anyFailed = true
		}
		if !update.IsSelfToolName(r.ToolName) {
			continue
		}
		switch r.Status {
		case upgrade.UpgradeSucceeded:
			structured.RestartRequired = true
		case upgrade.UpgradeSkipped:
			selfSkipped = true
			structured.ManualHint = r.ManualHint
		}
	}

	switch {
	case anyFailed:
		structured.Status = UpgradeStatusFailed
	case selfSkipped:
		structured.Status = UpgradeStatusSkipped
	default:
		structured.Status = UpgradeStatusSucceeded
	}
	return structured
}

// selfToolRegistryName returns the registry name of the self-tool as the
// running binary knows it ("axiom" once cmd/axiom init() has mutated the
// registry entry).
func selfToolRegistryName() string {
	for _, tool := range update.Tools {
		if update.IsSelfToolName(tool.Name) {
			return tool.Name
		}
	}
	return ""
}
