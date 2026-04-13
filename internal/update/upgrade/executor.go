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
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// backupExcludeSubdirs lists subdirectory base names that should be skipped
// when walking agent config root directories for backup. These directories
// contain runtime state, caches, or session data that is not configuration
// and can be extremely large (e.g. ~/.claude/projects/ can exceed 1 GB).
//
// Only the base name is matched — e.g. "projects" skips any directory named
// "projects" at any depth within the walked tree.
//
// Known limitation: some names are generic (e.g. "tasks", "debug", "cache",
// "plans") and could theoretically match legitimate config subdirectories in
// future agent versions. This is an accepted tradeoff — the risk of hanging
// the upgrade on multi-GB runtime dirs outweighs the risk of missing a
// niche config subdir. Skipped directories are logged at debug level.
//
// Must not be mutated after init. Tests must not modify this map; use a local
// copy or pass a separate map to enumerateFilesInDir instead.
var backupExcludeSubdirs = map[string]bool{
	// === Shared across agents ===
	"backups":      true, // backup snapshots themselves — never recurse into backups
	"cache":        true, // cached data
	"debug":        true, // debug logs
	"downloads":    true, // downloaded files
	"plugins":      true, // MCP plugin binaries (can be 60+ MB)
	"sessions":     true, // conversation session data
	"tasks":        true, // task tracking state
	"telemetry":    true, // telemetry data
	"node_modules": true, // npm dependencies (OpenCode, any Node-based agent)

	// === Claude Code (~/.claude/) ===
	"file-history":    true, // file change tracking
	"ide":             true, // IDE integration state
	"paste-cache":     true, // clipboard cache
	"plans":           true, // conversation plans
	"projects":        true, // per-project conversation state (can be 1+ GB)
	"session-env":     true, // session environment snapshots
	"shell-snapshots": true, // shell state snapshots
	"troubleshooting": true, // troubleshooting artifacts

	// === Gemini CLI / Antigravity (~/.gemini/, ~/.gemini/antigravity/) ===
	"browser_recordings":          true, // Antigravity browser recordings (can be 3+ GB)
	"antigravity-browser-profile": true, // Chromium profile data (250+ MB)
	"brain":                       true, // Antigravity memory/brain data (300+ MB)
	"conversations":               true, // Gemini conversation history
	"context_state":               true, // Gemini context state
	"html_artifacts":              true, // generated HTML artifacts
}

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
// Runtime/cache subdirectories listed in backupExcludeSubdirs are skipped to
// prevent the backup from walking gigabytes of non-config data.
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

	// Enumerate all regular files under each root dir, skipping non-config subdirs.
	paths := make([]string, 0)
	for _, dir := range configDirs {
		files, err := enumerateFilesInDir(dir, backupExcludeSubdirs)
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
// excludeDirNames is a set of directory base names to skip entirely at ANY depth.
// When a directory's base name matches, the entire subtree is pruned via
// filepath.SkipDir. The names in this set are chosen to be unambiguously
// runtime/cache directories (e.g. "projects", "browser_recordings", "node_modules")
// that would never be confused with legitimate config directories.
// Skipped directories are logged at debug level for auditability.
//
// Symlink handling:
//   - Symlinks to directories (including Windows junctions/reparse points) are
//     skipped entirely — their targets are not traversed. This prevents backup
//     failures when agent config directories contain junctioned skill directories
//     (e.g. ~/.claude/skills → some other directory).
//   - Symlinks to regular files ARE included — this supports dotfile managers
//     (stow, chezmoi, bare git) where config files like CLAUDE.md may be symlinks
//     to files in a dotfiles repository.
func enumerateFilesInDir(dir string, excludeDirNames map[string]bool) ([]string, error) {
	var files []string
	cleanDir := filepath.Clean(dir)

	err := filepath.WalkDir(cleanDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// Log unreadable entries but don't abort the walk — partial backup
			// is better than no backup.
			log.Printf("backup: skipping unreadable path %s: %v", path, err)
			return nil
		}
		// Symlink handling: skip directory symlinks, include file symlinks.
		if d.Type()&os.ModeSymlink != 0 {
			// Resolve the symlink to determine if it points to a file or directory.
			resolved, statErr := os.Stat(path)
			if statErr != nil {
				// Broken symlink — skip silently.
				return nil
			}
			if resolved.IsDir() {
				// Symlink to directory — skip to avoid traversing into external trees.
				return nil
			}
			// Symlink to regular file — include it (supports dotfile managers).
			files = append(files, path)
			return nil
		}
		// Skip excluded directories at any depth. The root dir itself is never
		// excluded (path == cleanDir on the first callback invocation).
		if d.IsDir() && path != cleanDir && excludeDirNames[strings.ToLower(d.Name())] {
			log.Printf("backup: excluding directory %s (matched exclude list)", path)
			return filepath.SkipDir
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
//   - Status VersionUnknown → report as UpgradeSkipped with ManualHint (manual attention required)
//   - Status UpToDate, NotInstalled, CheckFailed → omitted from report
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
	// Separate tools into executable (UpdateAvailable), dev-build (DevBuild), and
	// version-unknown tools. Non-actionable but user-visible states are included in
	// the report as UpgradeSkipped so the upgrade flow never fails silently.
	var executable []update.UpdateResult
	var devBuilds []update.UpdateResult
	var versionUnknowns []update.UpdateResult
	for _, r := range results {
		switch r.Status {
		case update.UpdateAvailable:
			executable = append(executable, r)
		case update.DevBuild:
			devBuilds = append(devBuilds, r)
		case update.VersionUnknown:
			versionUnknowns = append(versionUnknowns, r)
			// UpToDate, NotInstalled, CheckFailed → omit from report
		}
	}

	// If nothing is executable, dev-built, or version-unknown, return empty report.
	if len(executable) == 0 && len(devBuilds) == 0 && len(versionUnknowns) == 0 {
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
			if wErr := backup.WriteManifest(manifestPath, manifest); wErr != nil {
				log.Printf("backup: failed to write upgrade metadata to manifest: %v", wErr)
				backupWarning = fmt.Sprintf("backup created but metadata update failed: %s", wErr)
				sp.FinishSkipped()
			} else {
				sp.Finish(true)
			}
			backupID = manifest.ID
		}
	}

	// Build results slice: dev-build skips first (no exec), then executable tools.
	toolResults := make([]ToolUpgradeResult, 0, len(executable)+len(devBuilds)+len(versionUnknowns))

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

	// VersionUnknown tools: surface them as skipped so the user gets a clear hint
	// instead of a silent omission from the upgrade report.
	for _, r := range versionUnknowns {
		toolResults = append(toolResults, ToolUpgradeResult{
			ToolName:   r.Tool.Name,
			OldVersion: r.InstalledVersion,
			NewVersion: r.LatestVersion,
			Method:     effectiveMethod(r.Tool, profile),
			Status:     UpgradeSkipped,
			ManualHint: fmt.Sprintf("installed binary was found but its version could not be determined — check `%s` and reinstall if it is a stale source/dev build", detectCommandHint(r.Tool)),
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

func detectCommandHint(tool update.ToolInfo) string {
	if len(tool.DetectCmd) == 0 {
		return tool.Name
	}

	return strings.Join(tool.DetectCmd, " ")
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
