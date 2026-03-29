// Package upgrade provides the upgrade executor for managed tools.
// It sits ON TOP of the read-only internal/update package and is deliberately
// isolated from install, pipeline, planner, and config-sync code paths.
//
// Import boundary: this package MUST NOT import:
//   - github.com/gentleman-programming/gentle-ai/internal/pipeline
//   - github.com/gentleman-programming/gentle-ai/internal/planner
//   - github.com/gentleman-programming/gentle-ai/internal/cli
package upgrade

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/agents"
	"github.com/gentleman-programming/gentle-ai/internal/backup"
	"github.com/gentleman-programming/gentle-ai/internal/components/gga"
	"github.com/gentleman-programming/gentle-ai/internal/system"
	"github.com/gentleman-programming/gentle-ai/internal/update"
)

// Package-level vars for testability — same pattern as internal/update/detect.go.
// execCommand is used as: execCommand(name, args...) — identical signature to exec.Command.
// Swapping this var in tests controls which commands are actually run.
var execCommand = exec.Command

// snapshotCreator is the function used to create a backup snapshot before
// upgrade execution. Swapping this var in tests allows forcing snapshot
// failures to verify end-to-end warning surfacing in UpgradeReport.
var snapshotCreator = func(snapshotDir string, paths []string) (backup.Manifest, error) {
	return backup.NewSnapshotter().Create(snapshotDir, paths)
}

// AppVersion is the gentle-ai version written into backup manifests created by
// the upgrade executor. Set by app.go before calling Execute so that upgrade
// backups record the version that created them.
// Default "dev" matches the ldflags default in app.Version.
var AppVersion = "dev"

// configPathsForBackup returns the agent config file paths that the backup
// snapshot must include before any upgrade execution.
//
// Roots are derived from two sources:
//  1. Canonical managed agent roots — via agents.ConfigRootsForBackup using the
//     default registry. This automatically covers all registered adapters and
//     picks up new agents without manual list maintenance.
//  2. Approved GGA extras — gga.ConfigPath and gga.RuntimeLibDir are not adapter-
//     managed but must still be backed up. They are appended separately and do
//     not affect the canonical managed set used by sync.
//
// Only files (not directories) are included — Snapshotter.Create rejects dirs.
// Non-existent directories are silently skipped.
func configPathsForBackup(homeDir string) []string {
	reg, err := agents.NewDefaultRegistry()
	if err != nil {
		// Programming error — registry construction failed. Fall back gracefully.
		reg = nil
	}

	// Collect config root dirs: canonical agent roots first.
	var configDirs []string
	if reg != nil {
		configDirs = append(configDirs, agents.ConfigRootsForBackup(reg, homeDir)...)
	}

	// Approved GGA extras — outside the canonical managed agent set.
	// gga.ConfigPath returns the config *file* path; its parent dir is the root to walk.
	ggaConfigDir := filepath.Dir(gga.ConfigPath(homeDir))
	ggaLibDir := gga.RuntimeLibDir(homeDir)
	configDirs = append(configDirs, ggaConfigDir, ggaLibDir)

	// Enumerate all regular files under each root dir.
	paths := make([]string, 0)
	for _, dir := range configDirs {
		files, err := enumerateFilesInDir(dir)
		if err != nil {
			// Directory doesn't exist or can't be read — silently skip.
			continue
		}
		paths = append(paths, files...)
	}

	return paths
}

// enumerateFilesInDir returns the paths of all regular files (recursively) in dir.
// Returns an error if dir cannot be read (e.g. it doesn't exist).
//
// Symlinks and Windows junctions (reparse points) are skipped — they are not
// included in the returned paths and their targets are not traversed. This
// prevents backup failures when agent config directories contain junctioned skill
// directories (e.g. ~/.claude/skills → some other directory).
//
// On Unix, symlinks to directories appear with d.Type()&os.ModeSymlink != 0.
// On Windows, junctions appear similarly. Both are excluded by this check.
func enumerateFilesInDir(dir string) ([]string, error) {
	var files []string

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// Skip unreadable entries within the dir — don't abort the walk.
			return nil
		}
		// Skip symlinks and Windows junction/reparse points.
		// Symlinks to directories would be included as non-directory entries by
		// WalkDir but os.Stat resolves them to directories, causing "is a directory"
		// errors when snapshotPath attempts to copy them as files.
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if !d.IsDir() {
			files = append(files, path)
		}
		return nil
	})

	return files, err
}

// Execute evaluates UpdateResults, snapshots config before execution, then runs
// the appropriate upgrade strategy for each eligible tool.
//
// Reporting rules:
//   - Status UpdateAvailable → attempt upgrade; report Succeeded/Failed/Skipped(manual)
//   - Status DevBuild → report as UpgradeSkipped with ManualHint (dev/source build)
//   - Status UpToDate, NotInstalled, CheckFailed, VersionUnknown → omitted from report
//   - dryRun=true → no exec; eligible tools reported as UpgradeSkipped
//
// The backup snapshot is created before any exec call — this is the architectural
// guarantee that config is safe even if an upgrade fails mid-way.
func Execute(ctx context.Context, results []update.UpdateResult, profile system.PlatformProfile, homeDir string, dryRun bool, progress ...io.Writer) UpgradeReport {
	// progress writer for real-time status output (optional, defaults to no-op).
	var pw io.Writer = io.Discard
	if len(progress) > 0 && progress[0] != nil {
		pw = progress[0]
	}
	// Separate tools into executable (UpdateAvailable) and dev-build (DevBuild).
	// DevBuild tools are included in the report as UpgradeSkipped with a clear hint.
	var executable []update.UpdateResult
	var devBuilds []update.UpdateResult
	for _, r := range results {
		switch r.Status {
		case update.UpdateAvailable:
			executable = append(executable, r)
		case update.DevBuild:
			devBuilds = append(devBuilds, r)
			// UpToDate, NotInstalled, CheckFailed, VersionUnknown → omit from report
		}
	}

	// If nothing is executable or dev-built, return empty report.
	if len(executable) == 0 && len(devBuilds) == 0 {
		return UpgradeReport{DryRun: dryRun}
	}

	// Create backup snapshot BEFORE any execution (only when there are executables).
	backupID := ""
	backupWarning := ""
	if !dryRun && len(executable) > 0 {
		sp := NewSpinner(pw, "Creating pre-upgrade backup")
		snapshotDir := filepath.Join(homeDir, ".gentle-ai", "backups",
			fmt.Sprintf("upgrade-%s", time.Now().UTC().Format("20060102T150405Z")))
		manifest, err := snapshotCreator(snapshotDir, configPathsForBackup(homeDir))
		if err != nil {
			sp.Finish(false)
			backupWarning = fmt.Sprintf("pre-upgrade backup failed — upgrade will run without a backup: %s", err)
		} else {
			manifest.Source = backup.BackupSourceUpgrade
			manifest.Description = "pre-upgrade snapshot"
			manifest.CreatedByVersion = AppVersion
			manifestPath := filepath.Join(snapshotDir, backup.ManifestFilename)
			_ = backup.WriteManifest(manifestPath, manifest)
			backupID = manifest.ID
			sp.Finish(true)
		}
	}

	// Build results slice: dev-build skips first (no exec), then executable tools.
	toolResults := make([]ToolUpgradeResult, 0, len(executable)+len(devBuilds))

	// Dev-build tools: always UpgradeSkipped with a source-build hint.
	for _, r := range devBuilds {
		toolResults = append(toolResults, ToolUpgradeResult{
			ToolName:   r.Tool.Name,
			OldVersion: r.InstalledVersion,
			NewVersion: r.LatestVersion,
			Method:     effectiveMethod(r.Tool, profile),
			Status:     UpgradeSkipped,
			ManualHint: fmt.Sprintf("source build — upgrade manually or install a release binary from https://github.com/Gentleman-Programming/%s/releases", r.Tool.Repo),
		})
	}

	// Executable tools: run upgrade strategy.
	for _, r := range executable {
		method := effectiveMethod(r.Tool, profile)
		msg := fmt.Sprintf("Upgrading %s via %s (%s → %s)", r.Tool.Name, method, r.InstalledVersion, r.LatestVersion)
		sp := NewSpinner(pw, msg)
		toolResult := executeOne(ctx, r, profile, dryRun)
		switch toolResult.Status {
		case UpgradeSucceeded:
			sp.Finish(true)
		case UpgradeSkipped:
			// Intentional skip (manual fallback, dry-run, dev-build) — NOT a failure.
			// Render with skip marker (--) instead of failure marker (✗).
			sp.FinishSkipped()
		default:
			sp.Finish(false)
		}
		toolResults = append(toolResults, toolResult)
	}

	return UpgradeReport{
		BackupID:      backupID,
		BackupWarning: backupWarning,
		Results:       toolResults,
		DryRun:        dryRun,
	}
}

// executeOne runs the upgrade for a single tool.
func executeOne(ctx context.Context, r update.UpdateResult, profile system.PlatformProfile, dryRun bool) ToolUpgradeResult {
	base := ToolUpgradeResult{
		ToolName:   r.Tool.Name,
		OldVersion: r.InstalledVersion,
		NewVersion: r.LatestVersion,
		Method:     effectiveMethod(r.Tool, profile),
	}

	if dryRun {
		base.Status = UpgradeSkipped
		return base
	}

	err := runStrategy(ctx, r, profile)
	if err != nil {
		// Distinguish manual fallback (informational skip) from real failures.
		if hint, ok := AsManualFallback(err); ok {
			base.Status = UpgradeSkipped
			base.ManualHint = hint
			// Err is intentionally nil: a manual skip is not an error condition.
		} else {
			base.Status = UpgradeFailed
			base.Err = err
		}
	} else {
		base.Status = UpgradeSucceeded
	}

	return base
}

// effectiveMethod resolves the actual upgrade strategy for a tool on a given platform.
// On brew-managed platforms, brew takes precedence over the tool's declared method.
func effectiveMethod(tool update.ToolInfo, profile system.PlatformProfile) update.InstallMethod {
	if profile.PackageManager == "brew" {
		return update.InstallBrew
	}
	return tool.InstallMethod
}
