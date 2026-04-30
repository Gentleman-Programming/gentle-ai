package cli

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/agents"
	"github.com/gentleman-programming/gentle-ai/internal/agents/kimi"
	"github.com/gentleman-programming/gentle-ai/internal/assets"
	"github.com/gentleman-programming/gentle-ai/internal/backup"
	"github.com/gentleman-programming/gentle-ai/internal/components/engram"
	"github.com/gentleman-programming/gentle-ai/internal/components/gga"
	"github.com/gentleman-programming/gentle-ai/internal/components/mcp"
	"github.com/gentleman-programming/gentle-ai/internal/components/opencodeplugin"
	"github.com/gentleman-programming/gentle-ai/internal/components/permissions"
	"github.com/gentleman-programming/gentle-ai/internal/components/persona"
	"github.com/gentleman-programming/gentle-ai/internal/components/sdd"
	"github.com/gentleman-programming/gentle-ai/internal/components/skills"
	"github.com/gentleman-programming/gentle-ai/internal/components/theme"
	"github.com/gentleman-programming/gentle-ai/internal/installcmd"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/pipeline"
	"github.com/gentleman-programming/gentle-ai/internal/planner"
	"github.com/gentleman-programming/gentle-ai/internal/state"
	"github.com/gentleman-programming/gentle-ai/internal/system"
	"github.com/gentleman-programming/gentle-ai/internal/verify"
)

type InstallResult struct {
	Selection    model.Selection
	Resolved     planner.ResolvedPlan
	Review       planner.ReviewPayload
	Plan         pipeline.StagePlan
	Execution    pipeline.ExecutionResult
	Verify       verify.Report
	Dependencies system.DependencyReport
	DryRun       bool
}

var (
	osUserHomeDir       = os.UserHomeDir
	osSetenv            = os.Setenv
	osStat              = os.Stat
	runCommand          = executeCommand
	cmdLookPath         = exec.LookPath
	streamCommandOutput = true

	// ggaAvailableCheck is an optional override for ggaAvailable behavior.
	// When set, it is called instead of the default filesystem check.
	ggaAvailableCheck func(system.PlatformProfile) bool

	// engramDownloadFn is the function used to download the engram binary on non-brew platforms.
	// Package-level var for testability — tests can replace this to avoid real HTTP calls.
	engramDownloadFn = engram.DownloadLatestBinary

	// AppVersion is the gentle-ai version that will be written into backup manifests.
	// It is set by app.go before any CLI operation so that every backup created during
	// an install or sync records which version of gentle-ai made it.
	// Default "dev" matches the ldflags default in app.Version.
	AppVersion = "dev"
)

// SetCommandOutputStreaming toggles whether command stdout/stderr is streamed
// directly to the terminal. It returns a restore function.
func SetCommandOutputStreaming(enabled bool) func() {
	previous := streamCommandOutput
	streamCommandOutput = enabled
	return func() {
		streamCommandOutput = previous
	}
}

func RunInstall(args []string, detection system.DetectionResult) (InstallResult, error) {
	flags, err := ParseInstallFlags(args)
	if err != nil {
		return InstallResult{}, err
	}

	input, err := NormalizeInstallFlags(flags, detection)
	if err != nil {
		return InstallResult{}, err
	}

	resolved, err := planner.NewResolver(planner.MVPGraph()).Resolve(input.Selection)
	if err != nil {
		return InstallResult{}, err
	}
	profile := ResolveInstallProfile(detection)
	resolved.PlatformDecision = planner.PlatformDecisionFromProfile(profile)

	review := planner.BuildReviewPayload(input.Selection, resolved)
	stagePlan := buildStagePlan(input.Selection, resolved)

	result := InstallResult{
		Selection:    input.Selection,
		Resolved:     resolved,
		Review:       review,
		Plan:         stagePlan,
		Dependencies: detection.Dependencies,
		DryRun:       input.DryRun,
	}

	if input.DryRun {
		return result, nil
	}

	homeDir, err := osUserHomeDir()
	if err != nil {
		return result, fmt.Errorf("resolve user home directory: %w", err)
	}

	runtime, err := newInstallRuntime(homeDir, input.Selection, resolved, profile)
	if err != nil {
		return result, err
	}

	// Print dependency warnings before the pipeline starts (CLI only).
	// The TUI surfaces these on the complete screen instead.
	if !detection.Dependencies.AllPresent {
		fmt.Fprintf(os.Stderr, "WARNING: missing dependencies: %s\n\n%s\n",
			strings.Join(detection.Dependencies.MissingRequired, ", "),
			system.FormatMissingDepsMessage(detection.Dependencies))
	}

	stagePlan = runtime.stagePlan()
	result.Plan = stagePlan

	orchestrator := pipeline.NewOrchestrator(pipeline.DefaultRollbackPolicy())
	result.Execution = orchestrator.Execute(stagePlan)
	if result.Execution.Err != nil {
		return result, fmt.Errorf("execute install pipeline: %w", result.Execution.Err)
	}

	result.Verify = runPostApplyVerification(homeDir, input.Selection, resolved)
	result.Verify = withPostInstallNotes(result.Verify, resolved)
	if !result.Verify.Ready {
		return result, fmt.Errorf("post-apply verification failed:\n%s", verify.RenderReport(result.Verify))
	}

	// Persist the user's agent selection and model assignments so that future
	// `sync` runs target only the installed agents and preserve model choices.
	agentIDs := make([]string, 0, len(input.Selection.Agents))
	for _, a := range input.Selection.Agents {
		agentIDs = append(agentIDs, string(a))
	}
	// Non-fatal: a state write failure must not break an otherwise successful install.
	_ = state.Write(homeDir, state.InstallState{
		InstalledAgents:        agentIDs,
		ClaudeModelAssignments: claudeAliasesToStrings(input.Selection.ClaudeModelAssignments),
		ModelAssignments:       modelAssignmentsToState(input.Selection.ModelAssignments),
	})

	return result, nil
}

func withPostInstallNotes(report verify.Report, resolved planner.ResolvedPlan) verify.Report {
	if hasComponent(resolved.OrderedComponents, model.ComponentGGA) && report.Ready {
		report.FinalNote = report.FinalNote + "\n\nGGA is now installed globally. To enable project hooks, run in each repo:\n- gga init\n- gga install"
	}
	report = withGoInstallPathNote(report, resolved)
	return report
}

// withGoInstallPathNote appends a PATH guidance note when engram was installed
// on a non-brew platform (Linux/Windows). Since engram is now installed via
// direct binary download to /usr/local/bin or ~/.local/bin, this note helps
// users who may need to add the install directory to their PATH.
func withGoInstallPathNote(report verify.Report, resolved planner.ResolvedPlan) verify.Report {
	if !hasComponent(resolved.OrderedComponents, model.ComponentEngram) {
		return report
	}
	if resolved.PlatformDecision.PackageManager == "brew" {
		return report
	}
	binDir := goInstallBinDir()
	if isInPATH(binDir) {
		return report
	}
	report.FinalNote = report.FinalNote + fmt.Sprintf(
		"\n\nThe engram binary was installed to %s via `go install`.\nAdd it to your PATH: %s",
		binDir,
		engramPathGuidance(os.Getenv("SHELL")),
	)
	return report
}

// goInstallBinDir returns the directory where `go install` places binaries.
// Resolution order: $GOBIN > $GOPATH/bin > $HOME/go/bin.
func goInstallBinDir() string {
	if gobin := os.Getenv("GOBIN"); gobin != "" {
		return gobin
	}
	if gopath := os.Getenv("GOPATH"); gopath != "" {
		return filepath.Join(gopath, "bin")
	}
	if home, err := osUserHomeDir(); err == nil {
		return filepath.Join(home, "go", "bin")
	}
	return filepath.Join("~", "go", "bin")
}

// isInPATH reports whether dir is present in the current PATH.
func isInPATH(dir string) bool {
	for _, entry := range filepath.SplitList(os.Getenv("PATH")) {
		if entry == dir {
			return true
		}
	}
	return false
}

func buildStagePlan(selection model.Selection, resolved planner.ResolvedPlan) pipeline.StagePlan {
	prepare := []pipeline.Step{
		noopStep{id: "prepare:system-check"},
		noopStep{id: "prepare:check-dependencies"},
	}
	apply := make([]pipeline.Step, 0, len(resolved.Agents)+len(resolved.OrderedComponents))

	for _, agent := range resolved.Agents {
		apply = append(apply, noopStep{id: "agent:" + string(agent)})
	}

	for _, component := range resolved.OrderedComponents {
		apply = append(apply, noopStep{id: "component:" + string(component)})
	}

	if len(selection.Agents) == 0 && len(resolved.OrderedComponents) == 0 {
		prepare = nil
	}

	return pipeline.StagePlan{Prepare: prepare, Apply: apply}
}

type installRuntime struct {
	homeDir      string
	workspaceDir string
	selection    model.Selection
	resolved     planner.ResolvedPlan
	profile      system.PlatformProfile
	backupRoot   string
	state        *runtimeState
}

type runtimeState struct {
	manifest backup.Manifest
}

func newInstallRuntime(homeDir string, selection model.Selection, resolved planner.ResolvedPlan, profile system.PlatformProfile) (*installRuntime, error) {
	backupRoot := filepath.Join(homeDir, ".gentle-ai", "backups")
	if err := os.MkdirAll(backupRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create backup root directory %q: %w", backupRoot, err)
	}

	workspaceDir, _ := os.Getwd()

	return &installRuntime{
		homeDir:      homeDir,
		workspaceDir: workspaceDir,
		selection:    selection,
		resolved:     resolved,
		profile:      profile,
		backupRoot:   backupRoot,
		state:        &runtimeState{},
	}, nil
}

func (r *installRuntime) stagePlan() pipeline.StagePlan {
	targets := backupTargets(r.homeDir, r.selection, r.resolved)
	prepare := []pipeline.Step{
		checkDependenciesStep{id: "prepare:check-dependencies", profile: r.profile, homeDir: r.homeDir, selection: r.selection},
		prepareBackupStep{
			id:          "prepare:backup-snapshot",
			snapshotter: backup.NewSnapshotter(),
			snapshotDir: filepath.Join(r.backupRoot, time.Now().UTC().Format("20060102150405.000000000")),
			targets:     targets,
			state:       r.state,
			backupRoot:  r.backupRoot,
			source:      backup.BackupSourceInstall,
			description: "pre-install snapshot",
			appVersion:  AppVersion,
		},
	}

	apply := make([]pipeline.Step, 0, len(r.resolved.Agents)+len(r.resolved.OrderedComponents)+1)
	apply = append(apply, rollbackRestoreStep{id: "apply:rollback-restore", state: r.state})

	// Before installing components, ensure modular agents have their system prompt hub.
	// This ensures that SDD or Engram can inject their modules even if Persona is skipped.
	for _, agent := range r.resolved.Agents {
		if agent == model.AgentKimi {
			apply = append(apply, kimiSystemPromptHubStep{id: "agent:kimi-prompt-hub", homeDir: r.homeDir})
		}
	}

	for _, agent := range r.resolved.Agents {

		apply = append(apply, agentInstallStep{id: "agent:" + string(agent), agent: agent, homeDir: r.homeDir, profile: r.profile})
	}

	if containsAgent(r.resolved.Agents, model.AgentOpenCode) {
		for _, plugin := range r.selection.OpenCodePlugins {
			apply = append(apply, openCodePluginInstallStep{id: "opencode-plugin:" + string(plugin), plugin: plugin, homeDir: r.homeDir})
		}
	}

	for _, component := range r.resolved.OrderedComponents {
		apply = append(apply, componentApplyStep{
			id:           "component:" + string(component),
			component:    component,
			homeDir:      r.homeDir,
			workspaceDir: r.workspaceDir,
			agents:       r.resolved.Agents,
			selection:    r.selection,
			profile:      r.profile,
		})
	}

	return pipeline.StagePlan{Prepare: prepare, Apply: apply}
}

type prepareBackupStep struct {
	id          string
	snapshotter backup.Snapshotter
	snapshotDir string
	targets     []string
	state       *runtimeState

	// backupRoot is the parent directory of all backup snapshots.
	// When set, deduplication (IsDuplicate) and retention pruning (Prune) are
	// enabled. When empty, both are skipped (backward-compatible default).
	backupRoot string

	// source and description are optional metadata written into the manifest.
	// When set, they help users identify what created the backup.
	source      backup.BackupSource
	description string

	// appVersion is the gentle-ai version that created this backup.
	// When set, it is written into the manifest as CreatedByVersion.
	appVersion string
}

func (s prepareBackupStep) ID() string {
	return s.id
}

func (s prepareBackupStep) Run() error {
	// Deduplication: skip snapshot creation when content is identical to the
	// most recent backup. Only active when backupRoot is set.
	if s.backupRoot != "" {
		checksum, err := backup.ComputeChecksum(s.targets)
		if err == nil && checksum != "" {
			if dup, dupErr := backup.IsDuplicate(s.backupRoot, checksum); dupErr != nil {
				log.Printf("backup: check duplicate: %v", dupErr)
			} else if dup {
				// Content is identical to the most recent backup — skip creation.
				// state.manifest is left at its zero value; rollback is a no-op.
				return nil
			}
		}
	}

	manifest, err := s.snapshotter.Create(s.snapshotDir, s.targets)
	if err != nil {
		return fmt.Errorf("create backup snapshot: %w", err)
	}

	// Annotate with source metadata and version when provided, then re-write.
	// FileCount is already populated by Snapshotter.Create.
	if s.source != "" || s.appVersion != "" {
		manifest.Source = s.source
		manifest.Description = s.description
		manifest.CreatedByVersion = s.appVersion
		manifestPath := filepath.Join(s.snapshotDir, backup.ManifestFilename)
		if err := backup.WriteManifest(manifestPath, manifest); err != nil {
			// Non-fatal: metadata annotation failed but the snapshot is intact.
			// The backup is still usable — restore will work. We just lose the label.
			log.Printf("backup: annotate manifest: %v", err)
		}
	}

	s.state.manifest = manifest

	// Retention pruning: remove oldest unpinned backups beyond the limit.
	// Non-fatal: a prune failure must not prevent the install/sync from succeeding.
	if s.backupRoot != "" {
		if _, pruneErr := backup.Prune(s.backupRoot, backup.DefaultRetentionCount); pruneErr != nil {
			log.Printf("backup: prune: %v", pruneErr)
		}
	}

	return nil
}

type rollbackRestoreStep struct {
	id    string
	state *runtimeState
}

func (s rollbackRestoreStep) ID() string {
	return s.id
}

func (s rollbackRestoreStep) Run() error {
	return nil
}

func (s rollbackRestoreStep) Rollback() error {
	if len(s.state.manifest.Entries) == 0 {
		return nil
	}

	return backup.RestoreService{}.Restore(s.state.manifest)
}

type agentInstallStep struct {
	id      string
	agent   model.AgentID
	homeDir string
	profile system.PlatformProfile
}

type openCodePluginInstallStep struct {
	id      string
	plugin  model.OpenCodeCommunityPluginID
	homeDir string
}

func (s openCodePluginInstallStep) ID() string { return s.id }

func (s openCodePluginInstallStep) Run() error {
	_, err := opencodeplugin.Install(s.homeDir, s.plugin)
	return err
}

func (s agentInstallStep) ID() string {
	return s.id
}

func (s agentInstallStep) Run() error {
	adapter, err := agents.NewAdapter(s.agent)
	if err != nil {
		return fmt.Errorf("create adapter for %q: %w", s.agent, err)
	}

	if !adapter.SupportsAutoInstall() {
		return nil
	}

	installed, _, _, _, err := adapter.Detect(context.Background(), s.homeDir)
	if err != nil {
		return fmt.Errorf("detect agent %q: %w", s.agent, err)
	}
	if installed {
		return nil
	}

	if err := installcmd.ValidateAgentInstallPreflight(s.profile, s.agent); err != nil {
		return fmt.Errorf("preflight for agent %q: %w", s.agent, err)
	}

	commands, err := adapter.InstallCommand(s.profile)
	if err != nil {
		return fmt.Errorf("resolve install command for %q: %w", s.agent, err)
	}
	if len(commands) == 0 {
		return fmt.Errorf("install command for %q resolved to an empty sequence (unsupported platform or resolver misconfiguration)", s.agent)
	}

	return runCommandSequence(commands)
}

type kimiSystemPromptHubStep struct {
	id      string
	homeDir string
}

func (s kimiSystemPromptHubStep) ID() string {
	return s.id
}

func (s kimiSystemPromptHubStep) Run() error {
	return kimi.NewAdapter().BootstrapTemplate(s.homeDir)
}

type componentApplyStep struct {
	id           string
	component    model.ComponentID
	homeDir      string
	workspaceDir string
	agents       []model.AgentID
	selection    model.Selection
	profile      system.PlatformProfile
}

func (s componentApplyStep) ID() string {
	return s.id
}

// resolveAdapters creates adapters for each agent ID, skipping unsupported ones.
func resolveAdapters(agentIDs []model.AgentID) []agents.Adapter {
	adapters := make([]agents.Adapter, 0, len(agentIDs))
	for _, id := range agentIDs {
		adapter, err := agents.NewAdapter(id)
		if err != nil {
			continue
		}
		adapters = append(adapters, adapter)
	}
	return adapters
}

func (s componentApplyStep) Run() error {
	adapters := resolveAdapters(s.agents)

	switch s.component {
	case model.ComponentEngram:
		if _, err := cmdLookPath("engram"); err != nil {
			// Engram not on PATH — install it.
			if s.profile.PackageManager == "brew" {
				// macOS (or Linux with Homebrew): use brew tap + brew install.
				commands, err := engram.InstallCommand(s.profile)
				if err != nil {
					return fmt.Errorf("resolve install command for component %q: %w", s.component, err)
				}
				if err := runCommandSequence(commands); err != nil {
					return err
				}
			} else {
				// Linux / Windows: download the pre-built binary from GitHub Releases.
				// No Go required — engram ships pre-built binaries.
				binaryPath, err := engramDownloadFn(s.profile)
				if err != nil {
					return fmt.Errorf("download engram binary: %w", err)
				}
				// Add the install directory to PATH so subsequent commands
				// (engram setup, engram.Inject → resolveEngramCommand) can find it.
				// On Windows this also persists the change to the user registry via PowerShell.
				binDir := filepath.Dir(binaryPath)
				if err := system.AddToUserPath(binDir); err != nil {
					// Non-fatal: warn but continue — the binary was downloaded successfully.
					fmt.Fprintf(os.Stderr, "WARNING: could not add %s to PATH: %v\n", binDir, err)
				}
			}
		}
		setupMode := engram.ParseSetupMode(os.Getenv(engram.SetupModeEnvVar))
		setupStrict := engram.ParseSetupStrict(os.Getenv(engram.SetupStrictEnvVar))
		attemptedSlugs := make(map[string]struct{}, len(adapters))
		for _, adapter := range adapters {
			if engram.ShouldAttemptSetup(setupMode, adapter.Agent()) {
				slug, _ := engram.SetupAgentSlug(adapter.Agent())
				if _, seen := attemptedSlugs[slug]; !seen {
					if err := runCommand("engram", "setup", slug); err != nil {
						if setupStrict {
							return fmt.Errorf("engram setup for %q: %w", adapter.Agent(), err)
						}
					}
					attemptedSlugs[slug] = struct{}{}
				}
			}
			if _, err := engram.Inject(s.homeDir, adapter); err != nil {
				return fmt.Errorf("inject engram for %q: %w", adapter.Agent(), err)
			}
		}
		return nil
	case model.ComponentContext7:
		for _, adapter := range adapters {
			if _, err := mcp.Inject(s.homeDir, adapter); err != nil {
				return fmt.Errorf("inject context7 for %q: %w", adapter.Agent(), err)
			}
		}
		return nil
	case model.ComponentPersona:
		for _, adapter := range adapters {
			if _, err := persona.Inject(s.homeDir, adapter, s.selection.Persona); err != nil {
				return fmt.Errorf("inject persona for %q: %w", adapter.Agent(), err)
			}
		}
		return nil
	case model.ComponentPermission:
		for _, adapter := range adapters {
			if _, err := permissions.Inject(s.homeDir, adapter); err != nil {
				return fmt.Errorf("inject permissions for %q: %w", adapter.Agent(), err)
			}
		}
		return nil
	case model.ComponentSDD:
		for _, adapter := range adapters {
			opts := sdd.InjectOptions{
				OpenCodeModelAssignments: s.selection.ModelAssignments,
				ClaudeModelAssignments:   s.selection.ClaudeModelAssignments,
				KiroModelAssignments:     s.selection.KiroModelAssignments,
				WorkspaceDir:             s.workspaceDir,
				StrictTDD:                s.selection.StrictTDD,
			}
			if _, err := sdd.Inject(s.homeDir, adapter, s.selection.SDDMode, opts); err != nil {
				return fmt.Errorf("inject sdd for %q: %w", adapter.Agent(), err)
			}
		}
		return nil
	case model.ComponentSkills:
		skillIDs := selectedSkillIDs(s.selection)
		if len(skillIDs) == 0 {
			return nil
		}
		for _, adapter := range adapters {
			if _, err := skills.Inject(s.homeDir, adapter, skillIDs); err != nil {
				return fmt.Errorf("inject skills for %q: %w", adapter.Agent(), err)
			}
		}
		return nil
	case model.ComponentGGA:
		if !ggaAvailable(s.profile) {
			// GGA not found on any known PATH — install it.
			commands, err := gga.InstallCommand(s.profile)
			if err != nil {
				return fmt.Errorf("resolve install command for component %q: %w", s.component, err)
			}
			installErr := runCommandSequence(commands)
			if installErr != nil {
				if ggaAvailable(s.profile) {
					// The GGA install script uses `set -e` and `read -p` for
					// the "already installed" confirmation. Without a TTY
					// (common in automated/re-run scenarios), `read` fails
					// with exit code 1 and `set -e` kills the script before
					// it can exit 0. If GGA is actually available after the
					// script ran, the install succeeded functionally — treat
					// as success but warn the user.
					fmt.Fprintf(os.Stderr, "WARNING: gga install command reported an error but gga is available — continuing. Error was: %v\n", installErr)
				} else {
					return installErr
				}
			}
		}
		if err := gga.EnsureRuntimeAssets(s.homeDir); err != nil {
			return fmt.Errorf("ensure gga runtime assets: %w", err)
		}
		if runtime.GOOS == "windows" {
			if err := gga.EnsurePowerShellShim(s.homeDir); err != nil {
				return fmt.Errorf("ensure gga powershell shim: %w", err)
			}
			// Add GGA bin dir to the user PATH persistently on Windows.
			// GGA's install.sh drops the binary into ~/bin which is not on PATH by default.
			ggaBinDir := filepath.Join(s.homeDir, "bin")
			if err := system.AddToUserPath(ggaBinDir); err != nil {
				// Non-fatal: warn but continue — GGA was installed successfully.
				fmt.Fprintf(os.Stderr, "WARNING: could not add %s to PATH: %v\n", ggaBinDir, err)
			}
		}
		if _, err := gga.Inject(s.homeDir, s.agents); err != nil {
			return fmt.Errorf("inject gga config: %w", err)
		}
		return nil
	case model.ComponentTheme:
		for _, adapter := range adapters {
			if _, err := theme.Inject(s.homeDir, adapter); err != nil {
				return fmt.Errorf("inject theme for %q: %w", adapter.Agent(), err)
			}
		}
		return nil
	default:
		return fmt.Errorf("component %q is not supported in install runtime", s.component)
	}
}

func ensureGoAvailableAfterInstall(profile system.PlatformProfile) error {
	if _, err := cmdLookPath("go"); err == nil {
		return nil
	}

	if profile.OS != "windows" {
		return fmt.Errorf("go was installed but is still not available in PATH")
	}

	for _, candidate := range windowsGoCandidates() {
		if candidate == "" {
			continue
		}
		if _, err := osStat(candidate); err == nil {
			binDir := filepath.Dir(candidate)
			currentPath := os.Getenv("PATH")
			if currentPath == "" {
				return osSetenv("PATH", binDir)
			}
			return osSetenv("PATH", binDir+string(os.PathListSeparator)+currentPath)
		}
	}

	return fmt.Errorf("go was installed but is still not available in PATH; restart the terminal and retry")
}

func windowsGoCandidates() []string {
	programFiles := os.Getenv("ProgramFiles")
	programFilesX86 := os.Getenv("ProgramFiles(x86)")

	return []string{
		filepath.Join(programFiles, "Go", "bin", "go.exe"),
		filepath.Join(programFilesX86, "Go", "bin", "go.exe"),
		`C:\Program Files\Go\bin\go.exe`,
	}
}

// BuildRealStagePlan creates a StagePlan with real backup, agent install, and component apply steps.
// It is used by both the CLI and TUI paths.
func BuildRealStagePlan(homeDir string, selection model.Selection, resolved planner.ResolvedPlan, profile system.PlatformProfile) (pipeline.StagePlan, error) {
	backupRoot := filepath.Join(homeDir, ".gentle-ai", "backups")
	if err := os.MkdirAll(backupRoot, 0o755); err != nil {
		return pipeline.StagePlan{}, fmt.Errorf("create backup root directory %q: %w", backupRoot, err)
	}

	runtime, err := newInstallRuntime(homeDir, selection, resolved, profile)
	if err != nil {
		return pipeline.StagePlan{}, err
	}

	return runtime.stagePlan(), nil
}

// ResolveInstallProfile returns the platform profile from detection, defaulting to darwin/brew.
func ResolveInstallProfile(detection system.DetectionResult) system.PlatformProfile {
	if detection.System.Profile.OS != "" {
		return detection.System.Profile
	}

	return system.PlatformProfile{
		OS:             "darwin",
		PackageManager: "brew",
		Supported:      true,
	}
}

// ggaAvailable reports whether the gga binary is reachable. gga is often
// installed to ~/.local/bin (the default for install.sh on Linux and macOS)
// or ~/bin (the default for install.sh on Windows), which may not be on PATH.
// On macOS with Homebrew, gga may be in /opt/homebrew/bin or /usr/local/bin.
// We check the filesystem directly to avoid spawning a subprocess and to work
// regardless of whether the install directory has been added to PATH.
func ggaAvailable(profile system.PlatformProfile) bool {
	// Allow test override.
	if ggaAvailableCheck != nil {
		return ggaAvailableCheck(profile)
	}
	if _, err := cmdLookPath("gga"); err == nil {
		return true
	}
	homeDir, err := osUserHomeDir()
	if err != nil {
		return false
	}
	if _, err := osStat(filepath.Join(homeDir, ".local", "bin", "gga")); err == nil {
		return true
	}
	// Check well-known Homebrew prefixes for macOS (arm64 and x86).
	// gga may be installed via brew but not yet in the shell PATH
	// (e.g. new terminal session, Rosetta environment mismatch).
	if profile.OS == "darwin" || profile.PackageManager == "brew" {
		for _, brewBin := range []string{
			"/opt/homebrew/bin/gga",
			"/usr/local/bin/gga",
		} {
			if _, err := osStat(brewBin); err == nil {
				return true
			}
		}
	}
	if profile.OS == "windows" {
		if _, err := osStat(filepath.Join(homeDir, "bin", "gga")); err == nil {
			return true
		}
	}
	return false
}

// runCommandSequence runs each command in the sequence one at a time, stopping on first error.
func runCommandSequence(commands [][]string) error {
	if len(commands) == 0 {
		return fmt.Errorf("empty command sequence")
	}

	for _, command := range commands {
		if len(command) == 0 {
			return fmt.Errorf("empty command in sequence")
		}

		if err := runCommand(command[0], command[1:]...); err != nil {
			return fmt.Errorf("run command %q: %w", strings.Join(command, " "), err)
		}
	}

	return nil
}

func executeCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)

	if streamCommandOutput {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		if len(output) > 0 {
			return fmt.Errorf("%w\noutput:\n%s", err, strings.TrimSpace(string(output)))
		}
		return err
	}

	return nil
}

// selectedSkillIDs returns the skill IDs to install. If the selection
// has explicit skills, those are used; otherwise skills are derived from the preset.
func selectedSkillIDs(selection model.Selection) []model.SkillID {
	if len(selection.Skills) > 0 {
		return selection.Skills
	}

	return skills.SkillsForPreset(selection.Preset)
}

func backupTargets(homeDir string, selection model.Selection, resolved planner.ResolvedPlan) []string {
	paths := map[string]struct{}{}
	adapters := resolveAdapters(resolved.Agents)

	for _, component := range resolved.OrderedComponents {
		for _, path := range componentPaths(homeDir, selection, adapters, component) {
			paths[path] = struct{}{}
		}
	}

	targets := make([]string, 0, len(paths))
	for path := range paths {
		targets = append(targets, path)
	}

	return targets
}

func componentPaths(homeDir string, selection model.Selection, adapters []agents.Adapter, component model.ComponentID) []string {
	paths := []string{}
	for _, adapter := range adapters {
		switch component {
		case model.ComponentEngram:
			switch adapter.MCPStrategy() {
			case model.StrategySeparateMCPFiles:
				paths = append(paths, adapter.MCPConfigPath(homeDir, "engram"))
			case model.StrategyMergeIntoSettings:
				if p := adapter.SettingsPath(homeDir); p != "" {
					paths = append(paths, p)
				}
			case model.StrategyMCPConfigFile:
				if p := adapter.MCPConfigPath(homeDir, "engram"); p != "" {
					paths = append(paths, p)
				}
				if adapter.Agent() == model.AgentAntigravity {
					if p := adapter.SettingsPath(homeDir); p != "" {
						paths = append(paths, p)
					}
				}
			case model.StrategyTOMLFile:
				if p := adapter.MCPConfigPath(homeDir, "engram"); p != "" {
					paths = append(paths, p)
				}
			}
			if adapter.SystemPromptStrategy() == model.StrategyMarkdownSections {
				paths = append(paths, adapter.SystemPromptFile(homeDir))
			}
		case model.ComponentSDD:
			// Jinja modular hubs (e.g. Kimi KIMI.md) are appended once below so SDD+Persona
			// do not duplicate the same system prompt path.
			if adapter.SupportsSystemPrompt() && adapter.SystemPromptStrategy() != model.StrategyJinjaModules {
				paths = append(paths, adapter.SystemPromptFile(homeDir))
			}
			if adapter.SupportsSlashCommands() {
				for _, command := range sdd.OpenCodeCommands() {
					paths = append(paths, filepath.Join(adapter.CommandsDir(homeDir), command.Name+".md"))
				}
			}
			if adapter.Agent() == model.AgentOpenCode {
				if p := adapter.SettingsPath(homeDir); p != "" {
					paths = append(paths, p)
				}
				paths = append(paths, filepath.Join(homeDir, ".config", "opencode", "plugins", "background-agents.ts"))
				// Shared prompt files in ~/.config/opencode/prompts/sdd/ — back these up
				// so a sync does not silently overwrite user-customized prompt content.
				// These files are only written for multi-mode (SDDModeMulti), so we only
				// include them in the path list when that mode is active. This prevents
				// false-negative verification failures in single/empty mode syncs.
				if selection.SDDMode == model.SDDModeMulti {
					promptDir := sdd.SharedPromptDir(homeDir)
					for _, phase := range sdd.SharedPromptPhases() {
						paths = append(paths, filepath.Join(promptDir, phase+".md"))
					}
				}
			}
			if adapter.SupportsSkills() {
				skillDir := adapter.SkillsDir(homeDir)
				if skillDir != "" {
					paths = append(paths,
						filepath.Join(skillDir, "_shared", "persistence-contract.md"),
						filepath.Join(skillDir, "_shared", "engram-convention.md"),
						filepath.Join(skillDir, "_shared", "openspec-convention.md"),
						filepath.Join(skillDir, "_shared", "sdd-phase-common.md"),
						filepath.Join(skillDir, "_shared", "skill-resolver.md"),
						filepath.Join(skillDir, "sdd-init", "SKILL.md"),
						filepath.Join(skillDir, "sdd-explore", "SKILL.md"),
						filepath.Join(skillDir, "sdd-propose", "SKILL.md"),
						filepath.Join(skillDir, "sdd-spec", "SKILL.md"),
						filepath.Join(skillDir, "sdd-design", "SKILL.md"),
						filepath.Join(skillDir, "sdd-tasks", "SKILL.md"),
						filepath.Join(skillDir, "sdd-apply", "SKILL.md"),
						filepath.Join(skillDir, "sdd-verify", "SKILL.md"),
						filepath.Join(skillDir, "sdd-archive", "SKILL.md"),
					)
				}
			}
			paths = append(paths, sddSubAgentPaths(homeDir, adapter)...)
		case model.ComponentSkills:
			for _, skillID := range selectedSkillIDs(selection) {
				path := skills.SkillPathForAgent(homeDir, adapter, skillID)
				if path != "" {
					paths = append(paths, path)
				}
			}
		case model.ComponentContext7:
			switch adapter.MCPStrategy() {
			case model.StrategySeparateMCPFiles:
				paths = append(paths, adapter.MCPConfigPath(homeDir, "context7"))
			case model.StrategyMergeIntoSettings:
				if p := adapter.SettingsPath(homeDir); p != "" {
					paths = append(paths, p)
				}
			case model.StrategyMCPConfigFile:
				if p := adapter.MCPConfigPath(homeDir, "context7"); p != "" {
					paths = append(paths, p)
				}
			case model.StrategyTOMLFile:
				// Codex uses TOML for Engram but Context7 is not injected via TOML.
				// No path to report — Context7 injection is skipped for TOML agents.
			}
		case model.ComponentPersona:
			if selection.Persona == model.PersonaCustom {
				break
			}
			if adapter.SupportsSystemPrompt() && adapter.SystemPromptStrategy() != model.StrategyJinjaModules {
				paths = append(paths, adapter.SystemPromptFile(homeDir))
			}
			if selection.Persona == model.PersonaGentleman {
				if adapter.SupportsOutputStyles() {
					paths = append(paths, adapter.OutputStyleDir(homeDir)+"/gentleman.md")
					if p := adapter.SettingsPath(homeDir); p != "" {
						paths = append(paths, p)
					}
				}
			}
		case model.ComponentPermission:
			if p := adapter.SettingsPath(homeDir); p != "" {
				paths = append(paths, p)
			}
		case model.ComponentGGA:
			paths = append(paths, gga.ConfigPath(homeDir))
			paths = append(paths, gga.AgentsTemplatePath(homeDir))
		case model.ComponentTheme:
			if p := adapter.SettingsPath(homeDir); p != "" {
				paths = append(paths, p)
			}
		}
	}

	// Always ensure the main system prompt file is included for verification if the agent
	// supports modular system prompts (like Kimi), even if no specific component
	// (like Persona) was selected. This prevents false negatives when the skeleton
	// is bootstrapped but not explicitly owned by any other component path list.
	for _, adapter := range adapters {
		if adapter.SystemPromptStrategy() == model.StrategyJinjaModules {
			paths = append(paths, adapter.SystemPromptFile(homeDir))
		}
	}

	return paths
}

func sddSubAgentPaths(homeDir string, adapter agents.Adapter) []string {
	if !adapter.SupportsSubAgents() {
		return nil
	}

	entries, err := assets.FS.ReadDir(adapter.EmbeddedSubAgentsDir())
	if err != nil {
		return nil
	}

	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		paths = append(paths, filepath.Join(adapter.SubAgentsDir(homeDir), entry.Name()))
	}

	return paths
}

func runPostApplyVerification(homeDir string, selection model.Selection, resolved planner.ResolvedPlan) verify.Report {
	checks := make([]verify.Check, 0)
	adapters := resolveAdapters(resolved.Agents)

	seenPath := make(map[string]struct{})
	var uniqueFilePaths []string
	for _, component := range resolved.OrderedComponents {
		for _, path := range componentPaths(homeDir, selection, adapters, component) {
			if path == "" {
				continue
			}
			if _, dup := seenPath[path]; dup {
				continue
			}
			seenPath[path] = struct{}{}
			uniqueFilePaths = append(uniqueFilePaths, path)
		}
	}

	for _, currentPath := range uniqueFilePaths {
		path := currentPath
		checks = append(checks, verify.Check{
			ID:          "verify:file:" + path,
			Description: "required file exists",
			Run: func(context.Context) error {
				if _, err := os.Stat(path); err != nil {
					return err
				}
				return nil
			},
		})
	}

	if hasComponent(resolved.OrderedComponents, model.ComponentEngram) {
		checks = append(checks, engramHealthChecks()...)
	}
	checks = append(checks, antigravityCollisionCheck(resolved.Agents)...)

	return verify.BuildReport(verify.RunChecks(context.Background(), checks))
}

func hasComponent(components []model.ComponentID, target model.ComponentID) bool {
	for _, c := range components {
		if c == target {
			return true
		}
	}
	return false
}

func containsAgent(agents []model.AgentID, target model.AgentID) bool {
	for _, agent := range agents {
		if agent == target {
			return true
		}
	}
	return false
}

func engramHealthChecks() []verify.Check {
	return []verify.Check{
		{
			ID:          "verify:engram:binary",
			Description: "engram binary on PATH (restart shell if missing)",
			Soft:        true,
			Run: func(context.Context) error {
				if err := engram.VerifyInstalled(); err != nil {
					return fmt.Errorf("%w\nIf engram was installed via `go install`, add it to PATH:\n  %s", err, engramPathGuidance(os.Getenv("SHELL")))
				}
				return nil
			},
		},
		{
			ID:          "verify:engram:version",
			Description: "engram version returns valid output",
			Soft:        true,
			Run: func(context.Context) error {
				if err := engram.VerifyInstalled(); err != nil {
					// Binary not on PATH — skip version check gracefully.
					return nil
				}
				_, err := engram.VerifyVersion()
				return err
			},
		},
	}
}

// antigravityCollisionCheck returns a soft verify check that warns the user
// when both Antigravity and Gemini CLI are selected. Both agents write to
// ~/.gemini/GEMINI.md — content is merged (not overwritten) but the user
// should be aware.
func antigravityCollisionCheck(agents []model.AgentID) []verify.Check {
	hasAntigravity := false
	hasGemini := false
	for _, id := range agents {
		if id == model.AgentAntigravity {
			hasAntigravity = true
		}
		if id == model.AgentGeminiCLI {
			hasGemini = true
		}
	}
	if !hasAntigravity || !hasGemini {
		return nil
	}
	return []verify.Check{
		{
			ID:          "verify:antigravity:rules-collision",
			Description: "Antigravity and Gemini CLI share ~/.gemini/GEMINI.md",
			Soft:        true,
			Run: func(context.Context) error {
				return fmt.Errorf(
					"both Antigravity and Gemini CLI write rules to ~/.gemini/GEMINI.md\n" +
						"Content is merged, not overwritten — rules from both agents coexist in the same file.\n" +
						"This is expected behavior. No action required unless you want to separate them manually.",
				)
			},
		},
	}
}

func engramPathGuidance(shellPath string) string {
	binDir := goInstallBinDir()
	if strings.Contains(shellPath, "fish") {
		return fmt.Sprintf("set -Ux fish_user_paths %s $fish_user_paths", binDir)
	}
	if strings.Contains(shellPath, "zsh") {
		return fmt.Sprintf("echo 'export PATH=\"%s:$PATH\"' >> ~/.zshrc && source ~/.zshrc", binDir)
	}
	if strings.Contains(shellPath, "bash") {
		return fmt.Sprintf("echo 'export PATH=\"%s:$PATH\"' >> ~/.bashrc && source ~/.bashrc", binDir)
	}
	return fmt.Sprintf("Add %s to your shell PATH and restart the terminal.", binDir)
}

// checkDependenciesStep verifies that required system dependencies are present.
// It logs warnings for missing optional deps but only fails if required deps are missing.
type checkDependenciesStep struct {
	id        string
	profile   system.PlatformProfile
	homeDir   string
	selection model.Selection
}

func (s checkDependenciesStep) ID() string {
	return s.id
}

func (s checkDependenciesStep) Run() error {
	// Run detection but do NOT write to stdout/stderr — this step runs
	// inside the Bubble Tea alternate screen in TUI mode, so any raw
	// output corrupts the display (see issue #2). Missing deps are
	// surfaced on the TUI complete screen and by the actual install steps
	// failing with real error messages.
	_ = system.DetectDependencies(context.Background(), s.profile)
	for _, agent := range s.selection.Agents {
		adapter, err := agents.NewAdapter(agent)
		if err != nil {
			return fmt.Errorf("create adapter for %q: %w", agent, err)
		}

		if !adapter.SupportsAutoInstall() {
			continue
		}

		if s.homeDir != "" {
			installed, _, _, _, err := adapter.Detect(context.Background(), s.homeDir)
			if err != nil {
				return fmt.Errorf("detect agent %q: %w", agent, err)
			}
			if installed {
				continue
			}
		}

		if err := installcmd.ValidateAgentInstallPreflight(s.profile, agent); err != nil {
			return fmt.Errorf("preflight for agent %q: %w", agent, err)
		}
	}
	return nil
}

type noopStep struct {
	id string
}

func (s noopStep) ID() string {
	return s.id
}

func (s noopStep) Run() error {
	return nil
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
