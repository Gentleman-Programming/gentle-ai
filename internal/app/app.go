package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v2/internal/backup"
	"github.com/gentleman-programming/gentle-ai/v2/internal/cli"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/opencodeplugin"
	componentuninstall "github.com/gentleman-programming/gentle-ai/v2/internal/components/uninstall"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/pipeline"
	"github.com/gentleman-programming/gentle-ai/v2/internal/planner"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
	"github.com/gentleman-programming/gentle-ai/v2/internal/skillregistry"
	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
	"github.com/gentleman-programming/gentle-ai/v2/internal/tui"
	"github.com/gentleman-programming/gentle-ai/v2/internal/update"
	"github.com/gentleman-programming/gentle-ai/v2/internal/update/upgrade"
	"github.com/gentleman-programming/gentle-ai/v2/internal/verify"
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
		args := []string(nil)
		if h, err := os.UserHomeDir(); err == nil {
			args = syncArgsForDiscoveredAgents(h)
		}
		_, err := cli.RunSync(args)
		return err
	}
)

func Run() error {
	return RunArgs(os.Args[1:], os.Stdout)
}

const nonInteractiveTUIError = "gentle-ai requires both stdin and stdout to be terminals (TTYs); use --version, gentle-ai update, or --help for non-interactive use"

func RunArgs(args []string, stdout io.Writer) error {
	if len(args) == 0 && (!isattyFn(os.Stdin.Fd()) || !isattyFn(os.Stdout.Fd())) {
		return errors.New(nonInteractiveTUIError)
	}

	// Propagate the build-time version to the CLI and upgrade layers so backup
	// manifests record which version of gentle-ai created them.
	cli.AppVersion = Version
	upgrade.AppVersion = Version

	// --yes as a global CLI flag for self-update is handled via GENTLE_AI_YES=1.
	// Per-subcommand --yes flags (e.g. restore --yes) are parsed by each subcommand.

	// Platform-independent commands: no system detection, self-update, or
	// platform validation.
	if len(args) > 0 {
		switch args[0] {
		case "version", "--version", "-v":
			_, _ = fmt.Fprintf(stdout, "gentle-ai %s\n", Version)
			return nil
		case "help", "--help", "-h":
			printHelp(stdout, Version)
			return nil
		case "bench-model-picker":
			if handled, err := runBenchModelPickerCommand(args[1:], stdout); handled {
				return err
			}
		case "uninstall":
			if len(args) >= 2 && args[1] == "opencode-plugin" {
				_, err := cli.RunUninstallOpenCodePlugin(args[2:], stdout)
				return err
			}
			return runUninstall(args[1:], stdout)
		case "skill-registry":
			return runSkillRegistry(args[1:], stdout)
		case "sdd-status":
			return cli.RunSDDStatus(args[1:], stdout)
		case "sdd-continue":
			return cli.RunSDDContinue(args[1:], stdout)
		case "sdd-attempt":
			// Content digests canonicalize at this boundary (#2523); the ledger stays strict (#2395).
			return cli.RunSDDAttempt(cli.CanonicalizeSDDAttemptRevisionArgs(args[1:]), stdout)
		case "sdd-verify-validate":
			return cli.RunSDDVerifyValidate(args[1:], stdout)
		case "codegraph":
			return cli.RunCodeGraph(args[1:], stdout)
		case "review":
			// The kill switch must stay reachable even when review authority
			// itself is disabled, so it is dispatched ahead of the facade.
			if len(args) >= 2 && args[1] == "mode" {
				return cli.RunReviewMode(args[2:], stdout)
			}
			return cli.RunReview(args[1:], stdout)
		case "review-start":
			return cli.RunReviewStart(args[1:], stdout)
		case "review-resume":
			return cli.RunReviewResume(args[1:], stdout)
		case "review-step":
			return cli.RunReviewStep(args[1:], stdout)
		case "review-bundle-export":
			return cli.RunReviewBundleExport(args[1:], stdout)
		case "review-bundle-import":
			return cli.RunReviewBundleImport(args[1:], stdout)
		case "review-validate":
			return cli.RunReviewValidateNonDeciding(args[1:], stdout)
		case "install":
			if hasHelpFlag(args[1:]) {
				cli.PrintInstallHelp(stdout)
				return nil
			}
		case "sync":
			if hasHelpFlag(args[1:]) {
				cli.PrintSyncHelp(stdout)
				return nil
			}
		}
	}

	// Issue #535: parse the upgrade command's arguments exactly once, before
	// any platform validation, system detection, self-update, update check,
	// backup, or execution effect. Unsupported pre-delimiter dash arguments
	// are rejected here with zero effects. The parsed value is forwarded to
	// runUpgrade below so the executor never reparses raw CLI args. The
	// help selection branch is added in the follow-up help-behavior change.
	var parsedUpgrade *upgradeArgs
	if len(args) > 0 && args[0] == "upgrade" {
		parsed, err := parseUpgradeArgs(args[1:])
		if err != nil {
			return err
		}
		parsedUpgrade = &parsed
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
			// TUI self-update path: the previous launch completed a gentle-ai
			// self-upgrade under the old binary and set PendingSync=true. We are
			// now running under the new binary; print the doctor advisory so the
			// user can verify ecosystem health against the post-upgrade state.
			// Print regardless of sync outcome — the advisory is informational.
			printPostUpgradeDoctorAdvisory(stdout)
		}

		m := tui.NewModel(result, Version, installedState)
		m.ExecuteFn = tuiExecuteWithBackground
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
		// Slice 3b — wire the 4-layer managed-uninstall runner used by the
		// standalone "Uninstall OpenCode Plugin" TUI shortcut. The TUI
		// model falls back to opencodeplugin.Uninstall when this field is
		// nil; assigning it explicitly here keeps the production wiring
		// visible at the same seam as the other injected functions.
		m.OpenCodePluginUninstallFn = opencodeplugin.Uninstall
		// The review store is clone-scoped, so the TUI acts on the repository
		// the user launched it from. Both closures resolve the working
		// directory at call time rather than at wiring time, so a survey and
		// the reset it authorized can never disagree about which clone they
		// mean.
		m.ReviewStoreResetSurveyFn = tuiReviewStoreSurvey
		m.ReviewStoreResetFn = tuiReviewStoreReset
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
		return runUpgrade(context.Background(), *parsedUpgrade, result, stdout)
	case "install":
		installResult, err := cli.RunInstall(args[1:], result)
		if err != nil {
			return err
		}

		if installResult.DryRun {
			_, _ = fmt.Fprintln(stdout, cli.RenderDryRun(installResult))
		} else {
			_, _ = fmt.Fprint(stdout, verify.RenderReport(installResult.Verify))
			_, _ = fmt.Fprint(stdout, cli.RenderInstallManualActions(installResult))
		}

		return nil
	case "sync":
		syncResult, err := cli.RunSync(args[1:])
		if err != nil {
			return err
		}

		_, _ = fmt.Fprintln(stdout, cli.RenderSyncReport(syncResult))
		return nil
	case "restore":
		return cli.RunRestore(args[1:], stdout)
	case "doctor":
		return cli.RunDoctor(context.Background(), stdout)
	default:
		return fmt.Errorf("unknown command %q — run 'gentle-ai help' for available commands", args[0])
	}
}

func runUninstall(args []string, stdout io.Writer) error {
	result, err := cli.RunUninstall(args, stdout)
	if result.Manifest.ID != "" {
		_, _ = fmt.Fprintln(stdout, cli.RenderUninstallReport(result))
	}
	return err
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
	if len(args) == 0 {
		return fmt.Errorf("usage: gentle-ai skill-registry <refresh|list> [flags]")
	}
	switch args[0] {
	case "refresh":
		return runSkillRegistryRefresh(args[1:], stdout)
	case "list":
		return runSkillRegistryList(args[1:], stdout)
	default:
		return fmt.Errorf("unknown skill-registry command %q (want refresh or list)", args[0])
	}
}

// resolveSkillRegistryDirs resolves the working directory (defaulting to the
// process cwd) and the user home directory used to locate skills.
func resolveSkillRegistryDirs(cwd string) (string, string, error) {
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return "", "", fmt.Errorf("resolve cwd: %w", err)
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", fmt.Errorf("resolve home directory: %w", err)
	}
	return cwd, home, nil
}

func runSkillRegistryRefresh(args []string, stdout io.Writer) error {
	cwd := ""
	force := false
	quiet := false
	ensureGitignore := true
	for i := 0; i < len(args); i++ {
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
			return fmt.Errorf("unknown skill-registry refresh argument %q", args[i])
		}
	}
	cwd, home, err := resolveSkillRegistryDirs(cwd)
	if err != nil {
		return err
	}
	// Startup hooks run refresh from whatever directory the host resolved; a
	// brand-new non-project directory can resolve to "/", $HOME, or a
	// markerless folder. Never initialize there: skip silently under --quiet
	// (a startup hook must not scream) and with a one-line notice otherwise.
	if reason := skillregistry.RefreshSkip(cwd, home); reason != skillregistry.SkipNone {
		if !quiet {
			_, _ = fmt.Fprintf(stdout, "Skill registry refresh skipped (%s): %s is not a project root; run it from a project directory (one containing .git or .atl), or create the project first.\n", reason, cwd)
		}
		return nil
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

func runSkillRegistryList(args []string, stdout io.Writer) error {
	cwd := ""
	asJSON := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			asJSON = true
		case "--cwd":
			if i+1 >= len(args) {
				return fmt.Errorf("--cwd requires a value")
			}
			cwd = args[i+1]
			i++
		default:
			return fmt.Errorf("unknown skill-registry list argument %q", args[i])
		}
	}
	cwd, home, err := resolveSkillRegistryDirs(cwd)
	if err != nil {
		return err
	}
	entries := skillregistry.List(cwd, home)

	if asJSON {
		type row struct {
			Name        string `json:"name"`
			Scope       string `json:"scope"`
			Description string `json:"description"`
			Path        string `json:"path"`
		}
		rows := make([]row, 0, len(entries))
		for _, e := range entries {
			rows = append(rows, row{
				Name:        e.Name,
				Scope:       skillregistry.ScopeForPath(cwd, e.Path),
				Description: e.Description,
				Path:        e.Path,
			})
		}
		data, err := json.MarshalIndent(rows, "", "  ")
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(stdout, string(data))
		return nil
	}

	if len(entries) == 0 {
		_, _ = fmt.Fprintln(stdout, "No skills found.")
		return nil
	}
	for _, e := range entries {
		_, _ = fmt.Fprintf(stdout, "%s\t%s\t%s\n", e.Name, skillregistry.ScopeForPath(cwd, e.Path), e.Path)
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
//   - Falls back to source-install guidance where official binaries are unavailable
//
// Issue #535: runUpgrade consumes a structured upgradeArgs value parsed once
// in RunArgs. It forwards the parsed flags and tool filters to the update
// check and executor exactly once and never reparses raw CLI arguments.
func runUpgrade(ctx context.Context, args upgradeArgs, detection system.DetectionResult, stdout io.Writer) error {
	dryRun := args.dryRun
	noBackup := args.noBackup
	toolFilter := args.toolFilter

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
			if err := restartAfterGentleAIUpgrade(latestVersion, stdout); err != nil {
				return err
			}
			// CLI upgrade path: print the doctor advisory so the user can verify
			// ecosystem health against the post-upgrade state. Informational only;
			// does not run any checks or change exit status.
			printPostUpgradeDoctorAdvisory(stdout)
			return nil
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
var appUserHomeDir = os.UserHomeDir

func tuiExecuteWithBackground(
	selection model.Selection,
	resolved planner.ResolvedPlan,
	detection system.DetectionResult,
	background model.OpenCodeBackgroundIntent,
	backgroundPersist model.OpenCodeBackgroundIntent,
	piBackground model.PiBackgroundIntent,
	piBackgroundPersist model.PiBackgroundIntent,
	onProgress pipeline.ProgressFunc,
) pipeline.ExecutionResult {
	restoreCommandOutput := cli.SetCommandOutputStreaming(false)
	defer restoreCommandOutput()

	homeDir, err := appUserHomeDir()
	if err != nil {
		return pipeline.ExecutionResult{Err: fmt.Errorf("resolve user home directory: %w", err)}
	}

	profile := cli.ResolveInstallProfile(detection)
	resolved.PlatformDecision = planner.PlatformDecisionFromProfile(profile)

	execResult, orchestrator := cli.ExecuteTUIInstallWithBackgroundAndOrchestrator(homeDir, selection, resolved, profile, background, piBackground, onProgress)
	if execResult.Err == nil {
		// Persist the user's agent selection and model assignments so that future
		// `sync` runs target only the installed agents and preserve model choices.
		agentIDs := make([]string, 0, len(selection.Agents))
		for _, a := range selection.Agents {
			agentIDs = append(agentIDs, string(a))
		}
		claudePhaseState := claudePhaseAssignmentsToState(selection.ClaudePhaseAssignments)
		installState, readErr := state.Read(homeDir)
		if errors.Is(readErr, os.ErrNotExist) {
			installState = state.InstallState{}
		} else if readErr != nil {
			execResult.Err = fmt.Errorf("read persisted install state: %w", readErr)
			if orchestrator != nil {
				rollback := orchestrator.Rollback(execResult)
				if rollback.Err != nil {
					execResult.Err = errors.Join(execResult.Err, rollback.Err)
				}
			}
			return execResult
		}
		installState.InstalledAgents = agentIDs
		installState.CommunityTools = appCommunityToolIDsToStrings(selection.CommunityTools)
		installState.CommunityToolsConfigured = true
		installState.ClaudeModelAssignments = claudeLegacyAssignmentsForState(selection.ClaudeModelAssignments, claudePhaseState)
		installState.ClaudePhaseAssignments = claudePhaseState
		installState.KiroModelAssignments = kiroAliasesToStrings(selection.KiroModelAssignments)
		installState.CodexModelAssignments = codexEffortsToStrings(selection.CodexModelAssignments)
		installState.CodexOrchestratorAssignment = codexOrchestratorToState(selection.CodexOrchestratorAssignment)
		installState.CodexCarrilModelAssignments = selection.CodexCarrilModelAssignments
		installState.CodexPhaseModelAssignments = selection.CodexPhaseModelAssignments
		installState.ModelAssignments = modelAssignmentsToState(selection.ModelAssignments)
		installState.Persona = string(selection.Persona)
		installState.SetSelection(selection)
		if backgroundPersist != "" {
			installState.BackgroundIntent = backgroundPersist
		}
		if piBackgroundPersist != "" {
			installState.PiBackgroundIntent = piBackgroundPersist
		}
		if writeErr := state.WriteReconciled(homeDir, installState); writeErr != nil {
			execResult.Err = fmt.Errorf("persist install state: %w", writeErr)
			if orchestrator != nil {
				rollback := orchestrator.Rollback(execResult)
				if rollback.Err != nil {
					execResult.Err = errors.Join(execResult.Err, rollback.Err)
				}
			}
		}
	}

	return execResult
}

func appCommunityToolIDsToStrings(tools []model.CommunityToolID) []string {
	if tools == nil {
		return nil
	}
	result := make([]string, 0, len(tools))
	for _, tool := range tools {
		result = append(result, string(tool))
	}
	return result
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
		syncFlags := cli.SyncFlags{IncludePermissions: syncShouldIncludePermissions(agentIDs)}
		selection := cli.BuildSyncSelection(syncFlags, agentIDs)

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
		if err := persistAssignments(homeDir, selection); err != nil {
			return nil, fmt.Errorf("persist model assignments: %w", err)
		}

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

func syncShouldIncludePermissions(agentIDs []model.AgentID) bool {
	for _, id := range agentIDs {
		if id == model.AgentCodex {
			return true
		}
	}
	return false
}

func syncArgsForDiscoveredAgents(homeDir string) []string {
	if persisted, err := state.Read(homeDir); err == nil && persisted.SelectionConfigured {
		if (model.Selection{Components: persisted.Components}).HasComponent(model.ComponentPermission) {
			return []string{"--include-permissions"}
		}
		return nil
	}
	if syncShouldIncludePermissions(cli.DiscoverAgents(homeDir)) {
		return []string{"--include-permissions"}
	}
	return nil
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
	if overrides.ClearCodexOrchestratorAssignment {
		selection.CodexOrchestratorAssignment = nil
		selection.ClearCodexOrchestratorAssignment = true
	} else if overrides.CodexOrchestratorAssignment != nil {
		selection.CodexOrchestratorAssignment = overrides.CodexOrchestratorAssignment
		selection.ClearCodexOrchestratorAssignment = false
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
	cli.RestorePersistedSelection(selection, s, cli.SyncFlags{})
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
		selection.CodexCarrilModelAssignments = model.MigrateLegacyCodexCarrilDefaults(s.CodexCarrilModelAssignments)
	}
	if len(selection.CodexPhaseModelAssignments) == 0 && len(s.CodexPhaseModelAssignments) > 0 {
		m := make(map[string]string, len(s.CodexPhaseModelAssignments))
		for k, v := range s.CodexPhaseModelAssignments {
			m[k] = v
		}
		selection.CodexPhaseModelAssignments = m
	}
	if !selection.ClearCodexOrchestratorAssignment && selection.CodexOrchestratorAssignment == nil && s.CodexOrchestratorAssignment != nil {
		selection.CodexOrchestratorAssignment = codexOrchestratorFromState(s.CodexOrchestratorAssignment)
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
func persistAssignments(homeDir string, selection model.Selection) error {
	hasAssignmentSignal := selection.ClaudeModelAssignments != nil ||
		selection.ClaudePhaseAssignments != nil ||
		selection.KiroModelAssignments != nil ||
		selection.ModelAssignments != nil ||
		selection.CodexModelAssignments != nil ||
		selection.CodexOrchestratorAssignment != nil ||
		selection.ClearCodexOrchestratorAssignment ||
		selection.CodexCarrilModelAssignments != nil ||
		selection.CodexPhaseModelAssignments != nil
	if len(selection.ClaudeModelAssignments) == 0 && len(selection.ClaudePhaseAssignments) == 0 && len(selection.KiroModelAssignments) == 0 && len(selection.ModelAssignments) == 0 && len(selection.CodexModelAssignments) == 0 && len(selection.CodexCarrilModelAssignments) == 0 && len(selection.CodexPhaseModelAssignments) == 0 && !hasAssignmentSignal {
		return nil
	}
	current, err := state.Read(homeDir)
	if err != nil {
		// State file may not exist yet (e.g. pre-state users). Other read
		// failures, such as invalid JSON, must not overwrite existing state.
		if !errors.Is(err, os.ErrNotExist) {
			return nil
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
	if selection.ClearCodexOrchestratorAssignment {
		current.CodexOrchestratorAssignment = nil
	} else if selection.CodexOrchestratorAssignment != nil {
		current.CodexOrchestratorAssignment = codexOrchestratorToState(selection.CodexOrchestratorAssignment)
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
	return state.Write(homeDir, current)
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

func codexOrchestratorToState(a *model.CodexOrchestratorAssignment) *state.CodexOrchestratorAssignmentState {
	if a == nil {
		return nil
	}
	return &state.CodexOrchestratorAssignmentState{Model: a.Model, Effort: string(a.Effort)}
}

func codexOrchestratorFromState(a *state.CodexOrchestratorAssignmentState) *model.CodexOrchestratorAssignment {
	if a == nil {
		return nil
	}
	return &model.CodexOrchestratorAssignment{Model: a.Model, Effort: model.CodexEffort(a.Effort)}
}

// tuiReviewStoreSurvey reports what a review store reset would remove for the
// clone the TUI was launched from. It is read-only.
func tuiReviewStoreSurvey() (reviewtransaction.StoreResetReport, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return reviewtransaction.StoreResetReport{}, err
	}
	return reviewtransaction.SurveyReviewStore(context.Background(), cwd, reviewtransaction.StoreResetRequest{})
}

// tuiReviewStoreReset applies the reset for that same clone.
//
// It never sets IncludeInFlight, and never sets IncludeAdapterReviews either.
// The TUI has no keystroke that destroys a review somebody is in the middle of,
// nor one that destroys a store this command cannot classify: the confirmation
// screen refuses outright and prints the CLI invocation that overrides it, so
// every override stays an act the user has to type out.
func tuiReviewStoreReset() (reviewtransaction.StoreResetReport, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return reviewtransaction.StoreResetReport{}, err
	}
	return reviewtransaction.ResetReviewStore(context.Background(), cwd, reviewtransaction.StoreResetRequest{})
}
