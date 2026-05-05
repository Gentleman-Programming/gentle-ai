package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/internal/backup"
	"github.com/gentleman-programming/gentle-ai/internal/cli"
	componentuninstall "github.com/gentleman-programming/gentle-ai/internal/components/uninstall"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/pipeline"
	"github.com/gentleman-programming/gentle-ai/internal/planner"
	"github.com/gentleman-programming/gentle-ai/internal/state"
	"github.com/gentleman-programming/gentle-ai/internal/system"
	"github.com/gentleman-programming/gentle-ai/internal/tui"
	"github.com/gentleman-programming/gentle-ai/internal/update"
	"github.com/gentleman-programming/gentle-ai/internal/update/upgrade"
	"github.com/gentleman-programming/gentle-ai/internal/verify"
)

// Version is set from main via ldflags at build time.
var Version = "dev"

var (
	updateCheckAll           = update.CheckAll
	updateCheckFiltered      = update.CheckFiltered
	upgradeExecute           = upgrade.Execute
	ensureCurrentOSSupported = system.EnsureCurrentOSSupported
	detectSystem             = system.Detect
)

func Run() error {
	return RunArgs(os.Args[1:], os.Stdout)
}

func RunArgs(args []string, stdout io.Writer) error {
	// Propagate the build-time version to the CLI and upgrade layers so backup
	// manifests record which version of gentle-ai created them.
	cli.AppVersion = Version
	upgrade.AppVersion = Version

	// Info commands: no system detection, no self-update, no platform validation.
	if len(args) > 0 {
		switch args[0] {
		case "version", "--version", "-v":
			_, _ = fmt.Fprintf(stdout, "gentle-ai %s\n", Version)
			return nil
		case "help", "--help", "-h":
			printHelp(stdout, Version)
			return nil
		case "uninstall":
			_, err := cli.RunUninstall(args[1:], stdout)
			return err
		}
	}

	if err := ensureCurrentOSSupported(); err != nil {
		return err
	}

	result, err := detectSystem(context.Background())
	if err != nil {
		return fmt.Errorf("detect system: %w", err)
	}

	if !result.System.Supported {
		return system.EnsureSupportedPlatform(result.System.Profile)
	}

	// Self-update: check for a newer gentle-ai release and apply it before
	// CLI/TUI dispatch. Errors are non-fatal — logged and swallowed.
	profile := cli.ResolveInstallProfile(result)
	if err := selfUpdate(context.Background(), Version, profile, stdout); err != nil {
		_, _ = fmt.Fprintf(stdout, "Warning: self-update failed: %v\n", err)
	}

	if len(args) == 0 {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve user home directory: %w", err)
		}

		m := tui.NewModel(result, Version)
		m.ExecuteFn = tuiExecute
		m.RestoreFn = tuiRestore
		m.DeleteBackupFn = func(manifest backup.Manifest) error {
			return backup.DeleteBackup(manifest)
		}
		m.RenameBackupFn = func(manifest backup.Manifest, newDesc string) error {
			return backup.RenameBackup(manifest, newDesc)
		}
		m.TogglePinFn = func(manifest backup.Manifest) error {
			return backup.TogglePin(manifest)
		}
		m.ListBackupsFn = ListBackups
		m.Backups = ListBackups()
		m.UpgradeFn = tuiUpgrade(profile, homeDir)
		m.SyncFn = tuiSync(homeDir)
		m.UninstallFn = tuiUninstall(homeDir)
		m.UninstallWithProfilesFn = tuiUninstallWithProfiles(homeDir)
		p := tea.NewProgram(m, tea.WithAltScreen())
		_, err = p.Run()
		return err
	}

	switch args[0] {
	case "update":
		profile := cli.ResolveInstallProfile(result)
		return runUpdate(context.Background(), Version, profile, stdout)
	case "upgrade":
		return runUpgrade(context.Background(), args[1:], result, stdout)
	case "install":
		installResult, err := cli.RunInstall(args[1:], result)
		if err != nil {
			return err
		}

		if installResult.DryRun {
			_, _ = fmt.Fprintln(stdout, cli.RenderDryRun(installResult))
		} else {
			_, _ = fmt.Fprint(stdout, verify.RenderReport(installResult.Verify))
		}

		return nil
	case "sync":
		syncResult, err := cli.RunSync(args[1:])
		if err != nil {
			return err
		}

		_, _ = fmt.Fprintln(stdout, cli.RenderSyncReport(syncResult))
		return nil
	case "uninstall":
		uninstallResult, err := cli.RunUninstall(args[1:], stdout)
		if err != nil {
			// If a backup was created before the failure, surface it so
			// the user can restore safely.
			if uninstallResult.Manifest.ID != "" {
				_, _ = fmt.Fprintln(stdout, cli.RenderUninstallReport(uninstallResult))
			}
			return err
		}
		if uninstallResult.Manifest.ID != "" {
			_, _ = fmt.Fprintln(stdout, cli.RenderUninstallReport(uninstallResult))
		}
		return nil
	case "restore":
		return cli.RunRestore(args[1:], stdout)
	default:
		return fmt.Errorf("unknown command %q — run 'gentle-ai help' for available commands", args[0])
	}
}

func runUpdate(ctx context.Context, currentVersion string, profile system.PlatformProfile, stdout io.Writer) error {
	results := updateCheckAll(ctx, currentVersion, profile)
	_, _ = fmt.Fprint(stdout, update.RenderCLI(results))
	return updateCheckError(results)
}

// runUpgrade handles the `gentle-ai upgrade [--dry-run] [tool...]` command.
//
// This command:
//   - Checks for available updates for managed tools (gentle-ai, engram, gga)
//   - Snapshots agent config paths before execution (config preservation by design)
//   - Executes binary-only upgrades; does NOT invoke install or sync pipelines
//   - Skips gentle-ai itself when running as a dev build (version="dev")
//   - Falls back to manual guidance for unsafe platforms (Windows binary self-replace)
func runUpgrade(ctx context.Context, args []string, detection system.DetectionResult, stdout io.Writer) error {
	dryRun := false
	var toolFilter []string

	for _, arg := range args {
		switch {
		case arg == "--dry-run" || arg == "-n":
			dryRun = true
		case !strings.HasPrefix(arg, "-"):
			toolFilter = append(toolFilter, arg)
		}
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}

	profile := cli.ResolveInstallProfile(detection)

	// Check for available updates (filtered to requested tools if specified).
	sp := upgrade.NewSpinner(stdout, "Checking for updates")
	checkResults := updateCheckFiltered(ctx, Version, profile, toolFilter)
	checkErr := updateCheckError(checkResults)
	sp.Finish(checkErr == nil)
	if checkErr != nil {
		_, _ = fmt.Fprint(stdout, update.RenderCLI(checkResults))
		return checkErr
	}

	// Execute upgrades (no-op if nothing is UpdateAvailable).
	report := upgradeExecute(ctx, checkResults, profile, homeDir, dryRun, stdout)

	_, _ = fmt.Fprint(stdout, upgrade.RenderUpgradeReport(report))

	// Return error only if any tool failed (not for skipped/manual).
	var errs []error
	for _, r := range report.Results {
		if r.Status == upgrade.UpgradeFailed && r.Err != nil {
			errs = append(errs, fmt.Errorf("upgrade failed for %q: %w", r.ToolName, r.Err))
		}
	}

	return errors.Join(errs...)
}

func updateCheckError(results []update.UpdateResult) error {
	failed := update.CheckFailures(results)
	if len(failed) == 0 {
		return nil
	}

	return fmt.Errorf("update check failed for: %s", strings.Join(failed, ", "))
}

// tuiExecute creates a real install runtime and runs the pipeline with progress reporting.
func tuiExecute(
	selection model.Selection,
	resolved planner.ResolvedPlan,
	detection system.DetectionResult,
	onProgress pipeline.ProgressFunc,
) pipeline.ExecutionResult {
	restoreCommandOutput := cli.SetCommandOutputStreaming(false)
	defer restoreCommandOutput()

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return pipeline.ExecutionResult{Err: fmt.Errorf("resolve user home directory: %w", err)}
	}

	profile := cli.ResolveInstallProfile(detection)
	resolved.PlatformDecision = planner.PlatformDecisionFromProfile(profile)

	stagePlan, err := cli.BuildRealStagePlan(homeDir, selection, resolved, profile)
	if err != nil {
		return pipeline.ExecutionResult{Err: fmt.Errorf("build stage plan: %w", err)}
	}

	orchestrator := pipeline.NewOrchestrator(
		pipeline.DefaultRollbackPolicy(),
		pipeline.WithFailurePolicy(pipeline.ContinueOnError),
		pipeline.WithProgressFunc(onProgress),
	)

	execResult := orchestrator.Execute(stagePlan)
	if execResult.Err == nil {
		// Persist the user's agent selection and model assignments so that future
		// `sync` runs target only the installed agents and preserve model choices.
		agentIDs := make([]string, 0, len(selection.Agents))
		for _, a := range selection.Agents {
			agentIDs = append(agentIDs, string(a))
		}
		// Non-fatal: a state write failure must not break an otherwise successful install.
		_ = state.Write(homeDir, state.InstallState{
			InstalledAgents:        agentIDs,
			ClaudeModelAssignments: claudeAliasesToStrings(selection.ClaudeModelAssignments),
			ModelAssignments:       modelAssignmentsToState(selection.ModelAssignments),
			Persona:                string(selection.Persona),
		})
	}

	return execResult
}

// tuiRestore restores a backup from its manifest.
func tuiRestore(manifest backup.Manifest) error {
	return backup.RestoreService{}.Restore(manifest)
}

// tuiUpgrade returns a tui.UpgradeFunc that wraps upgrade.Execute.
// The profile and homeDir are captured from the call site so the closure
// is self-contained and requires no extra parameters at call time.
func tuiUpgrade(profile system.PlatformProfile, homeDir string) tui.UpgradeFunc {
	return func(ctx context.Context, results []update.UpdateResult) upgrade.UpgradeReport {
		return upgradeExecute(ctx, results, profile, homeDir, false)
	}
}

// tuiSync returns a tui.SyncFunc that performs a full managed-asset sync.
// It mirrors the RunSync CLI path: discovers installed agents from persisted
// state (or filesystem fallback), builds the default sync selection, and
// delegates to RunSyncWithSelection.
//
// When overrides is non-nil, model assignments are merged into the selection
// so that the "Configure Models" TUI flow persists its choices to disk.
func tuiSync(homeDir string) tui.SyncFunc {
	return func(overrides *model.SyncOverrides) (int, error) {
		agentIDs := cli.DiscoverAgents(homeDir)
		selection := cli.BuildSyncSelection(cli.SyncFlags{}, agentIDs)

		// Load persisted model assignments so a plain sync (no overrides)
		// preserves the user's previous choices instead of falling back
		// to the "balanced" preset.
		loadPersistedAssignments(homeDir, &selection)

		applyOverrides(&selection, overrides)

		result, err := cli.RunSyncWithSelection(homeDir, selection)
		if err != nil {
			return 0, err
		}

		// Persist model assignments that were actually used (from overrides
		// or loaded from state) so the next sync preserves them too.
		persistAssignments(homeDir, selection)

		return result.FilesChanged, nil
	}
}

// tuiUninstall returns a tui.UninstallFunc that mirrors the CLI uninstall path
// for selected agents/components, but without interactive flag parsing.
func tuiUninstall(homeDir string) tui.UninstallFunc {
	return func(agentIDs []model.AgentID, componentIDs []model.ComponentID) (componentuninstall.Result, error) {
		workspaceDir, err := os.Getwd()
		if err != nil {
			return componentuninstall.Result{}, fmt.Errorf("resolve workspace directory: %w", err)
		}
		return cli.RunUninstallWithSelection(homeDir, workspaceDir, agentIDs, componentIDs)
	}
}

func tuiUninstallWithProfiles(homeDir string) tui.UninstallWithProfilesFunc {
	return func(agentIDs []model.AgentID, componentIDs []model.ComponentID, profileNames []string, engramScope model.EngramUninstallScope) (componentuninstall.Result, error) {
		workspaceDir, err := os.Getwd()
		if err != nil {
			return componentuninstall.Result{}, fmt.Errorf("resolve workspace directory: %w", err)
		}
		return cli.RunUninstallWithSelectionAndProfiles(homeDir, workspaceDir, agentIDs, componentIDs, profileNames, engramScope)
	}
}

// applyOverrides merges non-nil fields from overrides into selection.
// A nil overrides pointer is a no-op.
func applyOverrides(selection *model.Selection, overrides *model.SyncOverrides) {
	if overrides == nil {
		return
	}
	if overrides.ModelAssignments != nil {
		selection.ModelAssignments = overrides.ModelAssignments
	}
	if overrides.ClaudeModelAssignments != nil {
		selection.ClaudeModelAssignments = overrides.ClaudeModelAssignments
	}
	if overrides.KiroModelAssignments != nil {
		selection.KiroModelAssignments = overrides.KiroModelAssignments
	}
	if overrides.SDDMode != "" {
		selection.SDDMode = overrides.SDDMode
	}
	if overrides.StrictTDD != nil {
		selection.StrictTDD = *overrides.StrictTDD
	}
	if len(overrides.Profiles) > 0 {
		selection.Profiles = overrides.Profiles
		// Profiles are an OpenCode multi-mode feature — if profiles are being
		// created/synced, SDDModeMulti is required so that WriteSharedPromptFiles
		// runs and the {file:...} prompt references resolve correctly.
		if selection.SDDMode == "" {
			selection.SDDMode = model.SDDModeMulti
		}
	}
}

// loadPersistedAssignments reads previously-saved model assignments from
// state.json and populates the selection when the corresponding maps are empty.
// This ensures a plain `sync` (no TUI overrides, no CLI flags) preserves the
// user's last-known model choices.
func loadPersistedAssignments(homeDir string, selection *model.Selection) {
	s, err := state.Read(homeDir)
	if err != nil {
		return
	}
	if len(selection.ClaudeModelAssignments) == 0 && len(s.ClaudeModelAssignments) > 0 {
		m := make(map[string]model.ClaudeModelAlias, len(s.ClaudeModelAssignments))
		for k, v := range s.ClaudeModelAssignments {
			m[k] = model.ClaudeModelAlias(v)
		}
		selection.ClaudeModelAssignments = m
	}
	if len(selection.KiroModelAssignments) == 0 && len(s.KiroModelAssignments) > 0 {
		m := make(map[string]model.ClaudeModelAlias, len(s.KiroModelAssignments))
		for k, v := range s.KiroModelAssignments {
			m[k] = model.ClaudeModelAlias(v)
		}
		selection.KiroModelAssignments = m
	}
	if len(selection.ModelAssignments) == 0 && len(s.ModelAssignments) > 0 {
		m := make(map[string]model.ModelAssignment, len(s.ModelAssignments))
		for k, v := range s.ModelAssignments {
			m[k] = model.ModelAssignment{ProviderID: v.ProviderID, ModelID: v.ModelID}
		}
		selection.ModelAssignments = m
	}
}

// persistAssignments writes the model assignments from selection back to
// state.json using a read-merge-write pattern so that other fields
// (InstalledAgents) are not lost.
func persistAssignments(homeDir string, selection model.Selection) {
	if len(selection.ClaudeModelAssignments) == 0 && len(selection.KiroModelAssignments) == 0 && len(selection.ModelAssignments) == 0 {
		return
	}
	current, err := state.Read(homeDir)
	if err != nil {
		// State file may not exist yet (e.g. pre-state users).
		current = state.InstallState{}
	}
	if len(selection.ClaudeModelAssignments) > 0 {
		current.ClaudeModelAssignments = claudeAliasesToStrings(selection.ClaudeModelAssignments)
	}
	if len(selection.KiroModelAssignments) > 0 {
		current.KiroModelAssignments = claudeAliasesToStrings(selection.KiroModelAssignments)
	}
	if len(selection.ModelAssignments) > 0 {
		current.ModelAssignments = modelAssignmentsToState(selection.ModelAssignments)
	}
	_ = state.Write(homeDir, current)
}

// claudeAliasesToStrings converts a typed ClaudeModelAlias map to plain strings
// for JSON serialisation in state.json.
func claudeAliasesToStrings(m map[string]model.ClaudeModelAlias) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = string(v)
	}
	return out
}

// modelAssignmentsToState converts model.ModelAssignment maps to the
// state-serialisable form.
func modelAssignmentsToState(m map[string]model.ModelAssignment) map[string]state.ModelAssignmentState {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]state.ModelAssignmentState, len(m))
	for k, v := range m {
		out[k] = state.ModelAssignmentState{ProviderID: v.ProviderID, ModelID: v.ModelID}
	}
	return out
}

// ListBackups returns all backup manifests from the backup directory.
func ListBackups() []backup.Manifest {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	backupRoot := filepath.Join(homeDir, ".gentle-ai", "backups")
	entries, err := os.ReadDir(backupRoot)
	if err != nil {
		return nil
	}

	manifests := make([]backup.Manifest, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		manifestPath := filepath.Join(backupRoot, entry.Name(), backup.ManifestFilename)
		manifest, err := backup.ReadManifest(manifestPath)
		if err != nil {
			continue
		}
		manifests = append(manifests, manifest)
	}

	// Sort by creation time (newest first) — the IDs are timestamps.
	for i := 0; i < len(manifests); i++ {
		for j := i + 1; j < len(manifests); j++ {
			if manifests[j].CreatedAt.After(manifests[i].CreatedAt) {
				manifests[i], manifests[j] = manifests[j], manifests[i]
			}
		}
	}

	return manifests
}
