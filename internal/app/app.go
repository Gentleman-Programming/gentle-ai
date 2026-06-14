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
	"github.com/gentleman-programming/gentle-ai/internal/skillregistry"
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
	updateCheckAll            = update.CheckAll
	updateCheckFiltered       = update.CheckFiltered
	upgradeExecute            = upgrade.Execute
	upgradeExecuteWithOptions = upgrade.ExecuteWithOptions
	selfUpdateFn              = selfUpdate
	ensureCurrentOSSupported  = system.EnsureCurrentOSSupported
	detectSystem              = system.Detect
	runTUI                    = func(m tea.Model, opts ...tea.ProgramOption) (tea.Model, error) {
		p := tea.NewProgram(m, opts...)
		return p.Run()
	}
	// deferredSyncFn is the function called when PendingSync=true is found on
	// launch. Swappable for tests; production value calls cli.RunSync directly.
	// cli.RunSync is idempotent (re-reads state + re-applies configs each call),
	// so retrying on failure (spec scenario "deferred sync fails → retry") is safe.
	deferredSyncFn = func() error {
		_, err := cli.RunSync(nil)
		return err
	}
)

func Run() error {
	return RunArgs(os.Args[1:], os.Stdout)
}

func RunArgs(args []string, stdout io.Writer) error {
	// Propagate the build-time version to the CLI and upgrade layers so backup
	// manifests record which version of gentle-ai created them.
	cli.AppVersion = Version
	upgrade.AppVersion = Version

	// --yes as a global CLI flag for self-update is handled via GENTLE_AI_YES=1.
	// Per-subcommand --yes flags (e.g. restore --yes) are parsed by each subcommand.

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
		case "skill-registry":
			return runSkillRegistry(args[1:], stdout)
		case "sdd-status":
			return cli.RunSDDStatus(args[1:], stdout)
		case "sdd-continue":
			return cli.RunSDDContinue(args[1:], stdout)
		case "install":
			if hasHelpFlag(args[1:]) {
				cli.PrintInstallHelp(stdout)
				return nil
			}
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

	var (
		profile         system.PlatformProfile
		profileResolved bool
	)
	resolveProfile := func() system.PlatformProfile {
		if !profileResolved {
			profile = cli.ResolveInstallProfile(result)
			profileResolved = true
		}
		return profile
	}

	// Self-update: check for a newer gentle-ai release and apply it before
	// CLI/TUI dispatch. Errors are non-fatal — logged and swallowed.
	// Skip auto-upgrade on TUI entry (len(args) == 0) to avoid silently
	// replacing the binary while the user expects a clean TUI launch (#696).
	isTUIFlow := len(args) == 0
	if !isTUIFlow && !isExplicitUpdateFlow(args) {
		if err := selfUpdateFn(context.Background(), Version, resolveProfile(), stdout); err != nil {
			_, _ = fmt.Fprintf(stdout, "Warning: self-update failed: %v\n", err)
		}
	}

	if len(args) == 0 {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve user home directory: %w", err)
		}

		// Load persisted state so the TUI pre-selects the agents the user
		// previously chose instead of re-selecting every detected config dir.
		// A missing or unreadable state file is not an error — NewModel falls
		// back to filesystem detection for first-time installs.
		installedState, _ := state.Read(homeDir)

		// Deferred sync: if a previous gentle-ai self-upgrade set PendingSync=true,
		// run sync now with the new binary before entering the TUI. On success,
		// clear the flag. On failure, log and leave the flag set for idempotent
		// retry on the next launch (per spec scenario "deferred sync fails → retry").
		// This is non-fatal — a sync failure must never block the TUI from opening.
		if installedState.PendingSync {
			if err := deferredSyncFn(); err != nil {
				_, _ = fmt.Fprintf(stdout, "Warning: deferred sync failed: %v\n", err)
				// Leave PendingSync=true so the next launch retries.
			} else {
				installedState.PendingSync = false
				if writeErr := state.Write(homeDir, installedState); writeErr != nil {
					// Best-effort: surface the failure so it's not silently swallowed.
					// Idempotent re-sync on the next launch is acceptable.
					_, _ = fmt.Fprintf(stdout, "Warning: failed to clear PendingSync flag: %v\n", writeErr)
				}
			}
		}

		m := tui.NewModel(result, Version, installedState)
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
		m.UpgradeFn = tuiUpgrade(resolveProfile(), homeDir)
		m.SyncFn = tuiSync(homeDir)
		m.UninstallFn = tuiUninstall(homeDir)
		m.UninstallWithProfilesFn = tuiUninstallWithProfiles(homeDir)
		finalModel, err := runTUI(m, tea.WithAltScreen())
		if err != nil {
			return err
		}
		if latestVersion, ok := gentleAIUpgradeVersionFromTUI(finalModel); ok {
			return restartAfterGentleAIUpgrade(latestVersion, stdout)
		}
		return nil
	}

	switch args[0] {
	case "update":
		return runUpdate(context.Background(), Version, resolveProfile(), stdout)
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
	case "doctor":
		return cli.RunDoctor(context.Background(), stdout)
	default:
		return fmt.Errorf("unknown command %q — run 'gentle-ai help' for available commands", args[0])
	}
}

func hasHelpFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return true
		}
	}

	return false
}

func gentleAIUpgradeVersionFromTUI(finalModel tea.Model) (string, bool) {
	switch m := finalModel.(type) {
	case tui.Model:
		return m.GentleAIUpgradeVersion()
	case *tui.Model:
		if m == nil {
			return "", false
		}
		return m.GentleAIUpgradeVersion()
	default:
		return "", false
	}
}

func runSkillRegistry(args []string, stdout io.Writer) error {
	if len(args) == 0 || args[0] != "refresh" {
		return fmt.Errorf("usage: gentle-ai skill-registry refresh [--cwd <dir>] [--force] [--quiet] [--no-gitignore]")
	}

	cwd := ""
	force := false
	quiet := false
	ensureGitignore := true
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--force", "-f":
			force = true
		case "--quiet", "-q":
			quiet = true
		case "--no-gitignore":
			ensureGitignore = false
		case "--cwd":
			if i+1 >= len(args) {
				return fmt.Errorf("--cwd requires a value")
			}
			cwd = args[i+1]
			i++
		default:
			return fmt.Errorf("unknown skill-registry argument %q", args[i])
		}
	}
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("resolve cwd: %w", err)
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}
	if ensureGitignore {
		if err := skillregistry.EnsureATLIgnored(cwd); err != nil {
			return err
		}
	}
	result, err := skillregistry.Regenerate(cwd, home, force)
	if err != nil {
		return err
	}
	if !quiet {
		if result.Regenerated {
			_, _ = fmt.Fprintf(stdout, "Skill registry refreshed (%d skills): %s\n", result.SkillCount, result.Registry)
		} else {
			_, _ = fmt.Fprintf(stdout, "Skill registry up to date (%s): %s\n", result.Reason, result.Registry)
		}
	}
	return nil
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
	noBackup := false
	var toolFilter []string

	for _, arg := range args {
		switch {
		case arg == "--dry-run" || arg == "-n":
			dryRun = true
		case arg == "--no-backup":
			noBackup = true
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

	// Execute upgrades (no-op if nothing is UpdateAvailable). Use the options
	// seam so CLI-only flags (e.g. --no-backup) remain testable without invoking
	// real package-manager strategies.
	report := upgradeExecuteWithOptions(ctx, checkResults, profile, homeDir, dryRun, upgrade.ExecuteOptions{
		Progress:          stdout,
		BackupDiagnostics: stdout,
		SkipBackup:        noBackup,
	})

	_, _ = fmt.Fprint(stdout, upgrade.RenderUpgradeReport(report))

	// Return error only if any tool failed (not for skipped/manual).
	var errs []error
	for _, r := range report.Results {
		if r.Status == upgrade.UpgradeFailed && r.Err != nil {
			errs = append(errs, fmt.Errorf("upgrade failed for %q: %w", r.ToolName, r.Err))
		}
	}

	if err := errors.Join(errs...); err != nil {
		return err
	}
	if !dryRun {
		if latestVersion, ok := gentleAIUpgradeSucceeded(report); ok {
			return restartAfterGentleAIUpgrade(latestVersion, stdout)
		}
	}
	return nil
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

	stagePlan, err := cli.BuildRealStagePlan(homeDir, cli.ScopeGlobal, selection, resolved, profile)
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
		claudePhaseState := claudePhaseAssignmentsToState(selection.ClaudePhaseAssignments)
		_ = state.Write(homeDir, state.InstallState{
			InstalledAgents:             agentIDs,
			ClaudeModelAssignments:      claudeLegacyAssignmentsForState(selection.ClaudeModelAssignments, claudePhaseState),
			ClaudePhaseAssignments:      claudePhaseState,
			KiroModelAssignments:        kiroAliasesToStrings(selection.KiroModelAssignments),
			CodexModelAssignments:       codexEffortsToStrings(selection.CodexModelAssignments),
			CodexCarrilModelAssignments: selection.CodexCarrilModelAssignments,
			CodexPhaseModelAssignments:  selection.CodexPhaseModelAssignments,
			ModelAssignments:            modelAssignmentsToState(selection.ModelAssignments),
			Persona:                     string(selection.Persona),
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
	return func(overrides *model.SyncOverrides) ([]string, error) {
		agentIDs := syncAgentIDs(homeDir, overrides)
		selection := cli.BuildSyncSelection(cli.SyncFlags{}, agentIDs)

		// Load persisted model assignments so a plain sync (no overrides)
		// preserves the user's previous choices instead of falling back
		// to the "balanced" preset.
		loadPersistedAssignments(homeDir, &selection)

		applyOverrides(&selection, overrides)

		result, err := cli.RunSyncWithSelection(homeDir, selection)
		if err != nil {
			return nil, err
		}

		// Persist model assignments that were actually used (from overrides
		// or loaded from state) so the next sync preserves them too.
		persistAssignments(homeDir, selection)

		return result.ChangedFiles, nil
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

func syncAgentIDs(homeDir string, overrides *model.SyncOverrides) []model.AgentID {
	if overrides == nil || len(overrides.TargetAgents) == 0 {
		return cli.DiscoverAgents(homeDir)
	}

	seen := make(map[model.AgentID]bool, len(overrides.TargetAgents))
	ids := make([]model.AgentID, 0, len(overrides.TargetAgents))
	for _, id := range overrides.TargetAgents {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
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
	if overrides.ClaudePhaseAssignments != nil {
		selection.ClaudePhaseAssignments = overrides.ClaudePhaseAssignments
		selection.ClaudeModelAssignments = nil
	}
	if overrides.KiroModelAssignments != nil {
		selection.KiroModelAssignments = overrides.KiroModelAssignments
	}
	if overrides.CodexModelAssignments != nil {
		selection.CodexModelAssignments = overrides.CodexModelAssignments
	}
	if overrides.CodexCarrilModelAssignments != nil {
		selection.CodexCarrilModelAssignments = overrides.CodexCarrilModelAssignments
	}
	if overrides.CodexPhaseModelAssignments != nil {
		selection.CodexPhaseModelAssignments = overrides.CodexPhaseModelAssignments
	}
	if overrides.SDDMode != "" {
		selection.SDDMode = overrides.SDDMode
	}
	if overrides.SDDProfileStrategy != "" {
		selection.SDDProfileStrategy = overrides.SDDProfileStrategy
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
	if len(selection.ClaudePhaseAssignments) == 0 && len(s.ClaudePhaseAssignments) > 0 {
		m := make(map[string]model.ClaudePhaseAssignment, len(s.ClaudePhaseAssignments))
		for k, v := range s.ClaudePhaseAssignments {
			if k == "orchestrator" {
				continue
			}
			a := model.ClaudePhaseAssignment{Model: model.ClaudeModelAlias(v.Model), Effort: model.ClaudeEffort(v.Effort)}
			if a.Valid() {
				m[k] = a
			}
		}
		selection.ClaudePhaseAssignments = m
	}
	if len(selection.ClaudeModelAssignments) == 0 && len(selection.ClaudePhaseAssignments) == 0 && len(s.ClaudeModelAssignments) > 0 {
		m := make(map[string]model.ClaudeModelAlias, len(s.ClaudeModelAssignments))
		for k, v := range s.ClaudeModelAssignments {
			// Claude Code controls the main session/orchestrator model itself.
			// Keep persisted assignments scoped to Agent tool calls only.
			if k == "orchestrator" {
				continue
			}
			m[k] = model.ClaudeModelAlias(v)
		}
		selection.ClaudeModelAssignments = m
	}
	if len(selection.KiroModelAssignments) == 0 && len(s.KiroModelAssignments) > 0 {
		m := make(map[string]model.KiroModelAlias, len(s.KiroModelAssignments))
		for k, v := range s.KiroModelAssignments {
			m[k] = model.KiroModelAlias(v)
		}
		selection.KiroModelAssignments = m
	}
	if len(selection.CodexModelAssignments) == 0 && len(s.CodexModelAssignments) > 0 {
		m := make(map[string]model.CodexEffort, len(s.CodexModelAssignments))
		for k, v := range s.CodexModelAssignments {
			m[k] = model.CodexEffort(v)
		}
		selection.CodexModelAssignments = m
	}
	if len(selection.CodexCarrilModelAssignments) == 0 && len(s.CodexCarrilModelAssignments) > 0 {
		m := make(map[string]string, len(s.CodexCarrilModelAssignments))
		for k, v := range s.CodexCarrilModelAssignments {
			m[k] = v
		}
		selection.CodexCarrilModelAssignments = m
	}
	if len(selection.CodexPhaseModelAssignments) == 0 && len(s.CodexPhaseModelAssignments) > 0 {
		m := make(map[string]string, len(s.CodexPhaseModelAssignments))
		for k, v := range s.CodexPhaseModelAssignments {
			m[k] = v
		}
		selection.CodexPhaseModelAssignments = m
	}
	if len(selection.ModelAssignments) == 0 && len(s.ModelAssignments) > 0 {
		m := make(map[string]model.ModelAssignment, len(s.ModelAssignments))
		for k, v := range s.ModelAssignments {
			m[k] = model.ModelAssignment{ProviderID: v.ProviderID, ModelID: v.ModelID, Effort: v.Effort}
		}
		selection.ModelAssignments = m
	}
}

// persistAssignments writes the model assignments from selection back to
// state.json using a read-merge-write pattern so that other fields
// (InstalledAgents) are not lost.
//
// For CodexPhaseModelAssignments the function distinguishes three states:
//   - nil: not provided (partial sync) — leave the existing state value untouched.
//   - non-nil, len > 0: new per-phase assignments — write them.
//   - non-nil, len == 0: explicit clear signal (preset selected) — delete the key.
func persistAssignments(homeDir string, selection model.Selection) {
	hasAssignmentSignal := selection.ClaudeModelAssignments != nil ||
		selection.ClaudePhaseAssignments != nil ||
		selection.KiroModelAssignments != nil ||
		selection.ModelAssignments != nil ||
		selection.CodexModelAssignments != nil ||
		selection.CodexCarrilModelAssignments != nil ||
		selection.CodexPhaseModelAssignments != nil
	if len(selection.ClaudeModelAssignments) == 0 && len(selection.ClaudePhaseAssignments) == 0 && len(selection.KiroModelAssignments) == 0 && len(selection.ModelAssignments) == 0 && len(selection.CodexModelAssignments) == 0 && len(selection.CodexCarrilModelAssignments) == 0 && len(selection.CodexPhaseModelAssignments) == 0 && !hasAssignmentSignal {
		return
	}
	current, err := state.Read(homeDir)
	if err != nil {
		// State file may not exist yet (e.g. pre-state users). Other read
		// failures, such as invalid JSON, must not overwrite existing state.
		if !errors.Is(err, os.ErrNotExist) {
			return
		}
		current = state.InstallState{}
	}
	if selection.ClaudeModelAssignments != nil {
		if len(selection.ClaudeModelAssignments) > 0 {
			current.ClaudeModelAssignments = claudeAliasesToStrings(selection.ClaudeModelAssignments)
		} else {
			current.ClaudeModelAssignments = nil
		}
	}
	if selection.ClaudePhaseAssignments != nil {
		if len(selection.ClaudePhaseAssignments) > 0 {
			current.ClaudePhaseAssignments = claudePhaseAssignmentsToState(selection.ClaudePhaseAssignments)
		} else {
			current.ClaudePhaseAssignments = nil
		}
		current.ClaudeModelAssignments = nil
	}
	if selection.KiroModelAssignments != nil {
		if len(selection.KiroModelAssignments) > 0 {
			current.KiroModelAssignments = kiroAliasesToStrings(selection.KiroModelAssignments)
		} else {
			current.KiroModelAssignments = nil
		}
	}
	if selection.CodexModelAssignments != nil {
		if len(selection.CodexModelAssignments) > 0 {
			current.CodexModelAssignments = codexEffortsToStrings(selection.CodexModelAssignments)
		} else {
			current.CodexModelAssignments = nil
		}
	}
	if selection.CodexCarrilModelAssignments != nil {
		if len(selection.CodexCarrilModelAssignments) > 0 {
			current.CodexCarrilModelAssignments = selection.CodexCarrilModelAssignments
		} else {
			current.CodexCarrilModelAssignments = nil
		}
	}
	// non-nil, len > 0 → write; non-nil, len == 0 → clear (explicit preset signal); nil → leave untouched.
	if selection.CodexPhaseModelAssignments != nil {
		if len(selection.CodexPhaseModelAssignments) > 0 {
			current.CodexPhaseModelAssignments = selection.CodexPhaseModelAssignments
		} else {
			current.CodexPhaseModelAssignments = nil
		}
	}
	if selection.ModelAssignments != nil {
		if len(selection.ModelAssignments) > 0 {
			current.ModelAssignments = modelAssignmentsToState(selection.ModelAssignments)
		} else {
			current.ModelAssignments = nil
		}
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
		// Claude Code owns the main session/orchestrator model; do not persist it
		// as a Gentle AI model assignment.
		if k == "orchestrator" {
			continue
		}
		out[k] = string(v)
	}
	return out
}

func claudeLegacyAssignmentsForState(
	legacy map[string]model.ClaudeModelAlias,
	phase map[string]state.ClaudePhaseAssignmentState,
) map[string]string {
	if len(phase) > 0 {
		return nil
	}
	return claudeAliasesToStrings(legacy)
}

func claudePhaseAssignmentsToState(m map[string]model.ClaudePhaseAssignment) map[string]state.ClaudePhaseAssignmentState {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]state.ClaudePhaseAssignmentState, len(m))
	for k, v := range m {
		if k == "orchestrator" || !v.Valid() {
			continue
		}
		out[k] = state.ClaudePhaseAssignmentState{Model: string(v.Model), Effort: string(v.Effort)}
	}
	return out
}

func kiroAliasesToStrings(m map[string]model.KiroModelAlias) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = string(v)
	}
	return out
}

// codexEffortsToStrings converts a typed CodexEffort map to plain strings
// for JSON serialisation in state.json.
func codexEffortsToStrings(m map[string]model.CodexEffort) map[string]string {
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
		out[k] = state.ModelAssignmentState{ProviderID: v.ProviderID, ModelID: v.ModelID, Effort: v.Effort}
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

// isExplicitUpdateFlow reports whether the current invocation is already in the
// explicit update/upgrade path. In those cases, self-update must be skipped to
// avoid preempting the user's requested command behavior.
func isExplicitUpdateFlow(args []string) bool {
	if len(args) == 0 {
		return false
	}
	return args[0] == "update" || args[0] == "upgrade"
}
