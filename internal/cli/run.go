package cli

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/claude"
	codexagent "github.com/gentleman-programming/gentle-ai/v4/internal/agents/codex"
	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/kimi"
	opencodeagent "github.com/gentleman-programming/gentle-ai/v4/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/backup"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/agentguidance"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/agenthooks"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/communitytool"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/engram"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/gga"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/legacyassets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/mcp"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/opencodeagents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/opencodedefault"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/opencodeplugin"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/opencoderuntimeplugins"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/permissions"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/persona"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/reviewassets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/skills"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/telemetryruntime"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/theme"
	"github.com/gentleman-programming/gentle-ai/v4/internal/installcmd"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	opencodeactivation "github.com/gentleman-programming/gentle-ai/v4/internal/opencode"
	"github.com/gentleman-programming/gentle-ai/v4/internal/pipeline"
	"github.com/gentleman-programming/gentle-ai/v4/internal/planner"
	"github.com/gentleman-programming/gentle-ai/v4/internal/state"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
	"github.com/gentleman-programming/gentle-ai/v4/internal/verify"
)

type InstallResult struct {
	Selection     model.Selection
	Resolved      planner.ResolvedPlan
	Review        planner.ReviewPayload
	Plan          pipeline.StagePlan
	Execution     pipeline.ExecutionResult
	Verify        verify.Report
	Dependencies  system.DependencyReport
	PiCodeGraph   *communitytool.PiCodeGraphResult
	ManualActions []string
	DryRun        bool

	Background              OpenCodeBackgroundResolution
	BackgroundPolicyEnabled bool

	PiBackground PiBackgroundResolution
}

var (
	osUserHomeDir                = os.UserHomeDir
	osSetenv                     = os.Setenv
	osStat                       = os.Stat
	runCommand                   = executeCommand
	cmdLookPath                  = exec.LookPath
	streamCommandOutput          = true
	goEnv                        = defaultGoEnv
	installCommunityTool         = communitytool.Install
	installCommunityToolWithHome = communitytool.InstallWithHome
	injectInstallPersona         = defaultInjectInstallPersona
	injectSyncPersona            = defaultInjectSyncPersona
	pathEnvEntries               = func(profile system.PlatformProfile) []string {
		return splitPathForOS(os.Getenv("PATH"), profile.OS)
	}
	addUserPath          = system.AddToUserPath
	ensureUserPathFirst  = system.PrioritizeUserPath
	userPathEntries      = system.UserPathEntries
	cleanupGGAInstallDir = gga.CleanupInstallDir

	// ggaAvailableCheck is an optional override for ggaAvailable behavior.
	// When set, it is called instead of the default filesystem check.
	ggaAvailableCheck func(system.PlatformProfile) bool

	// engramDownloadFn is the function used to download the engram binary on non-brew platforms.
	// Package-level var for testability — tests can replace this to avoid real HTTP calls.
	// Always uses the stable (release) path; beta channel at install time is handled
	// separately via installBetaEngramFromMain.
	engramDownloadFn = func(profile system.PlatformProfile) (string, error) {
		return engram.DownloadLatestBinary(profile, false)
	}

	// verifyEngramVersion resolves the installed engram binary version, threaded
	// into InjectOptions.Version (Decision 1 gate) and used to compute the
	// per-slug --protocol forwarding verdict. Package-level var for
	// testability — tests replace this to avoid depending on a real
	// installed engram binary. Overridden to a safe fake for the whole
	// package's test run (see TestMain in protocol_probe_test.go).
	verifyEngramVersion        = engram.VerifyVersion
	verifyEngramVersionCommand = engram.VerifyVersionCommand

	// probeEngramProtocolFlag detects whether the installed engram binary
	// supports the --protocol verbosity flag (design.md Decision 4).
	// Package-level var for testability — same rationale as verifyEngramVersion.
	probeEngramProtocolFlag        = engram.ProbeProtocolFlag
	probeEngramProtocolFlagCommand = engram.ProbeProtocolFlagCommand

	// AppVersion is the gentle-ai version that will be written into backup manifests.
	// It is set by app.go before any CLI operation so that every backup created during
	// an install or sync records which version of gentle-ai made it.
	// Default "dev" matches the ldflags default in app.Version.
	AppVersion = "dev"
)

// defaultInjectInstallPersona keeps non-OpenCode installs on persona.Inject and
// routes only an explicitly selected OpenCode settings file through
// persona.InjectAtSettingsPath.
func defaultInjectInstallPersona(homeDir string, adapter agents.Adapter, id model.PersonaID, selectedSettingsPath string) (persona.InjectionResult, error) {
	if selectedSettingsPath == "" {
		return persona.Inject(homeDir, adapter, id)
	}
	return persona.InjectAtSettingsPath(homeDir, adapter, id, selectedSettingsPath)
}

// defaultInjectSyncPersona mirrors defaultInjectInstallPersona for sync.
func defaultInjectSyncPersona(homeDir string, adapter agents.Adapter, id model.PersonaID, selectedSettingsPath string) (persona.InjectionResult, error) {
	if selectedSettingsPath == "" {
		return persona.InjectForSync(homeDir, adapter, id)
	}
	return persona.InjectForSyncAtSettingsPath(homeDir, adapter, id, selectedSettingsPath)
}

// SetCommandOutputStreaming toggles whether command stdout/stderr is streamed
// directly to the terminal. It returns a restore function.
func SetCommandOutputStreaming(enabled bool) func() {
	previous := streamCommandOutput
	streamCommandOutput = enabled
	return func() {
		streamCommandOutput = previous
	}
}

// Test seams are restored with t.Cleanup; their tests are not parallel.
var deriveManagedAssetWriter = managedAssetDigest
var installStagePlan = func(runtime *installRuntime) pipeline.StagePlan { return runtime.stagePlan() }

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
	homeDir, err := osUserHomeDir()
	if err != nil {
		return InstallResult{}, fmt.Errorf("resolve user home directory: %w", err)
	}
	persistedState, stateErr := state.Read(homeDir)
	if errors.Is(stateErr, os.ErrNotExist) {
		persistedState = state.InstallState{}
	} else if stateErr != nil {
		return InstallResult{}, fmt.Errorf("persist install state preflight: %w", stateErr)
	}
	background, err := resolveOpenCodeBackgroundCLI(flags.OpenCodeBackgroundSubagentsSet, flags.OpenCodeBackgroundSubagents, persistedState)
	if err != nil {
		return InstallResult{}, err
	}
	backgroundActivation, err := prepareOpenCodeBackgroundActivation(homeDir, &background, containsAgent(resolved.Agents, model.AgentOpenCode))
	if err != nil {
		return InstallResult{}, fmt.Errorf("prepare OpenCode background activation: %w", err)
	}
	piBackground, err := resolvePiBackgroundCLI(flags.PiBackgroundSubagentsSet, flags.PiBackgroundSubagents, persistedState)
	if err != nil {
		return InstallResult{}, err
	}
	piBackgroundProjection := preparePiBackgroundProjection(homeDir, &piBackground, containsAgent(resolved.Agents, model.AgentPi))

	review := planner.BuildReviewPayload(input.Selection, resolved)
	stagePlan := buildStagePlan(input.Selection, resolved)
	if backgroundActivation != nil {
		stagePlan.Apply = append(stagePlan.Apply, noopStep{id: "opencode:background-activation"})
	}
	if piBackgroundProjection != nil {
		stagePlan.Apply = append(stagePlan.Apply, noopStep{id: "pi:background-projection"})
	}

	result := InstallResult{
		Selection:    input.Selection,
		Resolved:     resolved,
		Review:       review,
		Plan:         stagePlan,
		Dependencies: detection.Dependencies,
		DryRun:       input.DryRun,
		Background:   background,
		PiBackground: piBackground,
	}
	result.Background.activationPlan = backgroundActivation
	if backgroundActivation != nil {
		result.Background.Activation = backgroundActivation.Report()
		result.BackgroundPolicyEnabled = backgroundActivation.Capability().Ready() && background.Effective == model.OpenCodeBackgroundOn
	}

	if input.DryRun {
		return result, nil
	}

	if input.Scope == ScopeGlobal {
		fmt.Fprintf(os.Stderr,
			"WARNING: installing with --scope=global (default). Agent config files (system prompts, skills/, agents/, etc.)\n"+
				"will be written to each selected agent's global config directory and will affect ALL workspaces for those agents on this machine.\n"+
				"To install only into the current workspace, rerun with --scope=workspace.\n\n")
	}
	runtime, err := newInstallRuntime(homeDir, input.Scope, input.Channel, input.Selection, resolved, profile)
	if err != nil {
		return result, err
	}
	defer runtime.state.cleanupCompatibilityTransaction()
	defer runtime.state.cleanupRollbackSnapshot()

	// Print dependency warnings before the pipeline starts (CLI only).
	// The TUI surfaces these on the complete screen instead.
	if !detection.Dependencies.AllPresent {
		fmt.Fprintf(os.Stderr, "WARNING: missing dependencies: %s\n\n%s\n",
			strings.Join(detection.Dependencies.MissingRequired, ", "),
			system.FormatMissingDepsMessage(detection.Dependencies))
	}
	runtime.background = background
	runtime.backgroundActivation = backgroundActivation
	runtime.runtimeReady = backgroundActivation != nil && backgroundActivation.Capability().Ready()
	runtime.piBackgroundProjection = piBackgroundProjection

	stagePlan = installStagePlan(runtime)
	result.Plan = stagePlan

	orchestrator := pipeline.NewOrchestrator(pipeline.DefaultRollbackPolicy())
	result.Execution = orchestrator.Execute(stagePlan)
	if result.Execution.Err != nil {
		return result, fmt.Errorf("execute install pipeline: %w", result.Execution.Err)
	}
	result.PiCodeGraph = runtime.state.piCodeGraph
	result.ManualActions = append(result.ManualActions, runtime.state.nativeReviewActions...)
	result.Verify = runPostApplyVerification(postApplyVerificationInput{
		HomeDir:      homeDir,
		WorkspaceDir: runtime.workspaceDir,
		Scope:        input.Scope,
		Selection:    input.Selection,
		Resolved:     resolved,
		State:        runtime.state,
	})
	result.Verify = withPostInstallNotes(result.Verify, resolved)
	result.Verify = withOpenCodeBackgroundPending(result.Verify, background, runtime.runtimeReady, resolved.Agents)
	result.Verify = withOpenCodeBackgroundActivationNote(result.Verify, background, resolved.Agents)
	if plan := piBackground.projectionPlan; plan != nil && plan.skipReason != "" {
		result.Verify.FinalNote += "\n\nPi background projection skipped: " + plan.skipReason
	}
	result.BackgroundPolicyEnabled = runtime.runtimeReady && background.Effective == model.OpenCodeBackgroundOn
	if backgroundActivation != nil {
		result.Background.Activation = backgroundActivation.Report()
	}
	if !result.Verify.Ready {
		verificationErr := fmt.Errorf("post-apply verification failed:\n%s", verify.RenderReport(result.Verify))
		rollback := orchestrator.Rollback(result.Execution)
		if rollback.Err != nil {
			verificationErr = errors.Join(verificationErr, rollback.Err)
		}
		return result, verificationErr
	}

	// Persist the user's agent selection and model assignments so that future
	// `sync` runs target only the installed agents and preserve model choices.
	agentIDs := make([]string, 0, len(input.Selection.Agents))
	for _, a := range input.Selection.Agents {
		agentIDs = append(agentIDs, string(a))
	}

	// When the user ran `gentle-ai install --agent X` (explicit agent flag),
	// merge into the existing state so that previously installed agents and
	// model assignments are preserved. A full install (no --agent flag) keeps
	// overwrite semantics so the TUI selection is the source of truth.
	claudePhaseState := claudePhaseAssignmentsToState(input.Selection.ClaudePhaseAssignments)
	newState := state.InstallState{
		InstalledAgents:             agentIDs,
		InstalledBinaryVersion:      AppVersion,
		CommunityTools:              communityToolIDsToStrings(input.Selection.CommunityTools),
		CommunityToolsConfigured:    true,
		ClaudeModelAssignments:      claudeLegacyAssignmentsForState(input.Selection.ClaudeModelAssignments, claudePhaseState),
		ClaudePhaseAssignments:      claudePhaseState,
		KiroModelAssignments:        kiroAliasesToStrings(input.Selection.KiroModelAssignments),
		CodexModelAssignments:       codexEffortsToStrings(input.Selection.CodexModelAssignments),
		CodexOrchestratorAssignment: codexOrchestratorToState(input.Selection.CodexOrchestratorAssignment),
		CodexCarrilModelAssignments: input.Selection.CodexCarrilModelAssignments,
		CodexPhaseModelAssignments:  input.Selection.CodexPhaseModelAssignments,
		ModelAssignments:            modelAssignmentsToState(input.Selection.ModelAssignments),
		Persona:                     string(input.Selection.Persona),
	}
	newState.SetSelection(input.Selection)
	if background.Persist != "" {
		newState.BackgroundIntent = background.Persist
	}
	if piBackground.Persist != "" {
		newState.PiBackgroundIntent = piBackground.Persist
	}
	writer, err := deriveManagedAssetWriter()
	if err != nil {
		return result, rollbackPostApplyError(orchestrator, result.Execution, fmt.Errorf("derive managed asset writer identity: %w", err))
	}
	if err := persistInstallState(homeDir, newState, agentIDs, flags, writer); err != nil {
		persistErr := fmt.Errorf("persist install state: %w", err)
		rollback := orchestrator.Rollback(result.Execution)
		if rollback.Err != nil {
			persistErr = errors.Join(persistErr, rollback.Err)
		}
		return result, persistErr
	}

	TelemetryTrigger(homeDir)
	return result, nil
}

func rollbackPostApplyError(orchestrator *pipeline.Orchestrator, execution pipeline.ExecutionResult, cause error) error {
	rollback := orchestrator.Rollback(execution)
	return errors.Join(cause, rollback.Err)
}

func persistInstallState(homeDir string, newState state.InstallState, agentIDs []string, flags InstallFlags, writer string) error {
	return withInstallStateLock(homeDir, func() error {
		if len(flags.Agents) > 0 {
			merged, mergeErr := mergeExplicitAgentInstallState(homeDir, newState, agentIDs, flags)
			if mergeErr != nil {
				return fmt.Errorf("merge explicit agent install state: %w", mergeErr)
			}
			newState = merged
		} else {
			existing, err := state.Read(homeDir)
			if errors.Is(err, os.ErrNotExist) {
				existing = state.InstallState{}
			} else if err != nil {
				return err
			}
			newState = mergeFullInstallState(existing, newState)
		}
		newState.ManagedAssetDigest = writer
		return state.WriteReconciled(homeDir, newState)
	})
}

func mergeFullInstallState(existing, fresh state.InstallState) state.InstallState {
	merged := existing
	merged.InstalledAgents = fresh.InstalledAgents
	merged.SelectionConfigured, merged.Components, merged.Skills = fresh.SelectionConfigured, fresh.Components, fresh.Skills
	merged.Preset, merged.SDDMode, merged.StrictTDD = fresh.Preset, fresh.SDDMode, false
	merged.CommunityTools, merged.CommunityToolsConfigured = fresh.CommunityTools, fresh.CommunityToolsConfigured
	merged.ClaudeModelAssignments, merged.ClaudePhaseAssignments = fresh.ClaudeModelAssignments, fresh.ClaudePhaseAssignments
	merged.KiroModelAssignments, merged.CodexModelAssignments = fresh.KiroModelAssignments, fresh.CodexModelAssignments
	merged.CodexOrchestratorAssignment = fresh.CodexOrchestratorAssignment
	merged.CodexCarrilModelAssignments, merged.CodexPhaseModelAssignments = fresh.CodexCarrilModelAssignments, fresh.CodexPhaseModelAssignments
	merged.ModelAssignments, merged.Persona = fresh.ModelAssignments, fresh.Persona
	if fresh.BackgroundIntent != "" {
		merged.BackgroundIntent = fresh.BackgroundIntent
	}
	if fresh.PiBackgroundIntent != "" {
		merged.PiBackgroundIntent = fresh.PiBackgroundIntent
	}
	return merged
}

// mergeExplicitAgentInstallState merges a fresh single-agent install's state
// into the previously persisted ~/.gentle-ai/state.json (so `install --agent
// X` preserves other previously installed agents and model assignments).
//
// When the existing state file is simply absent (first install, or an agent
// installed before state persistence existed), there is nothing to merge
// from — newState is used as-is. Any OTHER read failure (corrupted JSON,
// unreadable file) returns an error instead of silently falling through: the
// caller must not report a successful install while never persisting state
// (install/sync surface audit finding 2). Blindly merging from unreadable
// data would itself be wrong, so this does not attempt best-effort recovery
// — it fails loudly instead, matching the original conservative intent of
// refusing to merge from data that cannot be trusted.
func mergeExplicitAgentInstallState(homeDir string, newState state.InstallState, agentIDs []string, flags InstallFlags) (state.InstallState, error) {
	existing, readErr := state.Read(homeDir)
	if readErr != nil {
		if errors.Is(readErr, os.ErrNotExist) {
			return newState, nil
		}
		return state.InstallState{}, fmt.Errorf("read existing install state to merge agents %v: %w", agentIDs, readErr)
	}

	merged := state.MergeAgents(existing, agentIDs)
	if newState.ModelAssignments != nil {
		merged.ModelAssignments = newState.ModelAssignments
	}
	if newState.ClaudeModelAssignments != nil {
		merged.ClaudeModelAssignments = newState.ClaudeModelAssignments
	}
	if newState.ClaudePhaseAssignments != nil {
		merged.ClaudePhaseAssignments = newState.ClaudePhaseAssignments
		merged.ClaudeModelAssignments = nil
	}
	if newState.KiroModelAssignments != nil {
		merged.KiroModelAssignments = newState.KiroModelAssignments
	}
	if newState.CodexOrchestratorAssignment != nil {
		merged.CodexOrchestratorAssignment = newState.CodexOrchestratorAssignment
	}
	if newState.CodexModelAssignments != nil {
		merged.CodexModelAssignments = newState.CodexModelAssignments
	}
	if newState.CodexCarrilModelAssignments != nil {
		merged.CodexCarrilModelAssignments = newState.CodexCarrilModelAssignments
	}
	if newState.CodexPhaseModelAssignments != nil {
		merged.CodexPhaseModelAssignments = newState.CodexPhaseModelAssignments
	}
	if merged.SelectionConfigured {
		if len(flags.Components) > 0 {
			merged.Components = newState.Components
		}
		if len(flags.Skills) > 0 {
			merged.Skills = newState.Skills
		}
		if flags.Preset != "" {
			merged.Preset = newState.Preset
		}
		if flags.SDDMode != "" {
			merged.SDDMode = newState.SDDMode
		}
	}
	if flags.Persona != "" || merged.Persona == "" {
		merged.Persona = newState.Persona
	}
	if newState.BackgroundIntent != "" {
		merged.BackgroundIntent = newState.BackgroundIntent
	}
	if newState.PiBackgroundIntent != "" {
		merged.PiBackgroundIntent = newState.PiBackgroundIntent
	}
	return merged, nil
}

func withPostInstallNotes(report verify.Report, resolved planner.ResolvedPlan) verify.Report {
	report = withReadyAgentRunNote(report, resolved)
	report = withFailedVerificationNote(report, resolved)
	if hasComponent(resolved.OrderedComponents, model.ComponentGGA) && report.Ready {
		report.FinalNote = report.FinalNote + "\n\nGGA is now installed globally. To enable project hooks, run in each repo:\n- gga init\n- gga install"
	}
	report = withGoInstallPathNote(report, resolved)
	return report
}

// readyAgentRunCommands is the fixed, ordered set of installable agents that
// have a standalone runnable CLI command a user types to start building, and
// the exact command name checked by each adapter's own Detect (e.g.
// claude/adapter.go's lookPath("claude")). Kept intentionally narrow to the
// two agents the completion line originally named -- fisidj's finding
// (organic-dx Phase 3f task 3f.4) was specifically about those two, and
// expanding to every agent with a CLI binary is a separate, unreviewed
// decision left for a future task.
var readyAgentRunCommands = []struct {
	ID      model.AgentID
	Command string
}{
	{model.AgentClaudeCode, "claude"},
	{model.AgentOpenCode, "opencode"},
}

// withReadyAgentRunNote replaces the generic verify.ReadyMessage with one
// naming only the runnable agent commands actually selected this run. It is
// scoped to exactly that generic text so it never clobbers a FinalNote that
// was already customized (by a test fixture or an earlier note-builder).
func withReadyAgentRunNote(report verify.Report, resolved planner.ResolvedPlan) verify.Report {
	if !report.Ready || report.FinalNote != verify.ReadyMessage {
		return report
	}
	report.FinalNote = verify.ReadyMessageForCommands(runnableAgentCommands(resolved.Agents))
	return report
}

// withFailedVerificationNote replaces the generic verify.VerificationIssuesMessage
// with one naming the concrete command that retries the install for the
// agents that were actually resolved this run: `gentle-ai install --agent
// <agent1>,<agent2>`. There is no `repair` case in the CLI dispatcher
// (internal/app/app.go), so the old generic text named a command that could
// never succeed -- a false continuation worse than no note at all.
//
// It is scoped to exactly the generic failure text so it never clobbers a
// FinalNote that was already customized (by a test fixture or an earlier
// note-builder), mirroring withReadyAgentRunNote's ready-path guard.
func withFailedVerificationNote(report verify.Report, resolved planner.ResolvedPlan) verify.Report {
	if report.Ready || report.FinalNote != verify.VerificationIssuesMessage {
		return report
	}
	if len(resolved.Agents) == 0 {
		return report
	}
	names := make([]string, len(resolved.Agents))
	for i, agent := range resolved.Agents {
		names[i] = string(agent)
	}
	report.FinalNote = verify.VerificationIssuesMessageForCommand("gentle-ai install --agent " + strings.Join(names, ","))
	return report
}

func runnableAgentCommands(agentIDs []model.AgentID) []string {
	selected := make(map[model.AgentID]bool, len(agentIDs))
	for _, id := range agentIDs {
		selected[id] = true
	}
	commands := make([]string, 0, len(readyAgentRunCommands))
	for _, entry := range readyAgentRunCommands {
		if selected[entry.ID] {
			commands = append(commands, entry.Command)
		}
	}
	return commands
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

func defaultGoEnv(keys ...string) (map[string]string, error) {
	args := append([]string{"env"}, keys...)
	cmd := exec.Command("go", args...)
	system.EnsureCommandDir(cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimRight(string(out), "\r\n"), "\n")
	values := make(map[string]string, len(keys))
	for i, key := range keys {
		if i < len(lines) {
			values[key] = strings.TrimSpace(lines[i])
		}
	}
	return values, nil
}

func goInstallBinDirFromGoEnv() (string, error) {
	values, err := goEnv("GOBIN", "GOPATH")
	if err != nil {
		return "", err
	}
	if gobin := strings.TrimSpace(values["GOBIN"]); gobin != "" {
		return gobin, nil
	}
	if gopath := strings.TrimSpace(values["GOPATH"]); gopath != "" {
		return filepath.Join(gopath, "bin"), nil
	}
	return "", fmt.Errorf("go env returned empty GOBIN and GOPATH")
}

const engramBetaGoInstallPackage = "github.com/Gentleman-Programming/engram/cmd/engram@main"

func betaEngramRequiresGo(channel InstallChannel, component model.ComponentID) bool {
	return channel.IsBeta() && component == model.ComponentEngram
}

func installBetaEngramFromMain() (string, error) {
	if err := runCommand("go", "install", engramBetaGoInstallPackage); err != nil {
		return "", err
	}

	binDir, err := goInstallBinDirFromGoEnv()
	if err != nil {
		return "", fmt.Errorf("resolve go install bin dir: %w", err)
	}

	binaryName := "engram"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(binDir, binaryName)
	if err := prependToPath(binDir); err != nil {
		return "", err
	}
	return binaryPath, nil
}

func prependToPath(dir string) error {
	if dir == "" {
		return nil
	}
	if isInPATH(dir) {
		return nil
	}
	path := os.Getenv("PATH")
	if path == "" {
		return osSetenv("PATH", dir)
	}
	return osSetenv("PATH", dir+string(os.PathListSeparator)+path)
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
	scope        InstallScope
	selection    model.Selection
	resolved     planner.ResolvedPlan
	profile      system.PlatformProfile
	channel      InstallChannel
	backupRoot   string
	state        *runtimeState
	sdkConsent   *OpenCodeSDKConsent

	background           OpenCodeBackgroundResolution
	runtimeReady         bool
	backgroundActivation *opencodeactivation.ActivationPlan

	piBackgroundProjection *piBackgroundProjectionPlan
	progress               pipeline.ProgressFunc
}

type runtimeState struct {
	telemetryRollback        func() error
	manifest                 backup.Manifest
	rollbackSnapshotDir      string
	piCodeGraph              *communitytool.PiCodeGraphResult
	nativeReviewActions      []string
	compatibilityTransaction compatibilityRefreshTransaction

	// engramVersionResolved, engramVersion, and engramVersionErr cache the
	// single `engram version` invocation performed by componentApplyStep.Run
	// for ComponentEngram (Decision 1 gate), so the post-apply health check
	// (engramHealthChecks) can reuse the result instead of shelling out to
	// `engram version` a second time (JD-016).
	engramVersionResolved bool
	engramVersion         string
	engramVersionErr      error
}

func (s *runtimeState) cleanupRollbackSnapshot() {
	if s == nil || s.rollbackSnapshotDir == "" {
		return
	}
	if err := os.RemoveAll(s.rollbackSnapshotDir); err != nil {
		log.Printf("backup: remove transaction snapshot: %v", err)
		return
	}
	s.rollbackSnapshotDir = ""
}

func (s *runtimeState) cleanupCompatibilityTransaction() {
	if s == nil || s.compatibilityTransaction == nil {
		return
	}
	transaction := s.compatibilityTransaction
	s.compatibilityTransaction = nil
	if err := transaction.Close(); err != nil {
		log.Printf("compatibility: close transaction: %v", err)
	}
}

func (s *runtimeState) compatibilityChangedFiles() []string {
	if s == nil || s.compatibilityTransaction == nil {
		return nil
	}
	return s.compatibilityTransaction.ChangedFiles()
}

func newInstallRuntime(homeDir string, scope InstallScope, channel InstallChannel, selection model.Selection, resolved planner.ResolvedPlan, profile system.PlatformProfile) (*installRuntime, error) {
	backupRoot := filepath.Join(homeDir, ".gentle-ai", "backups")
	compatibilityTransaction, err := newCompatibilityRefreshTransaction(homeDir, resolved.OrderedComponents, selection)
	if err != nil {
		return nil, err
	}
	state := &runtimeState{compatibilityTransaction: compatibilityTransaction}
	if err := os.MkdirAll(backupRoot, 0o755); err != nil {
		state.cleanupCompatibilityTransaction()
		return nil, fmt.Errorf("create backup root directory %q: %w", backupRoot, err)
	}

	workspaceDir, _ := os.Getwd()

	return &installRuntime{
		homeDir:      homeDir,
		workspaceDir: workspaceDir,
		scope:        scope,
		selection:    selection,
		resolved:     resolved,
		profile:      profile,
		channel:      channel,
		backupRoot:   backupRoot,
		state:        state,
	}, nil
}

func (r *installRuntime) stagePlan() pipeline.StagePlan {
	targets, targetErr := backupTargets(r.homeDir, r.workspaceDir, r.scope, r.selection, r.resolved)
	prepare := []pipeline.Step{
		prepareBackupStep{
			id:          "prepare:backup-snapshot",
			snapshotter: backup.NewSnapshotter(),
			snapshotDir: filepath.Join(r.backupRoot, time.Now().UTC().Format("20060102150405.000000000")),
			targets:     targets,
			targetErr:   targetErr,
			state:       r.state,
			backupRoot:  r.backupRoot,
			source:      backup.BackupSourceInstall,
			description: "pre-install snapshot",
			appVersion:  AppVersion,
		},
	}

	apply := make([]pipeline.Step, 0, len(r.resolved.Agents)+len(r.selection.CommunityTools)+len(r.resolved.OrderedComponents)+1)
	telemetryDir := openCodeTelemetryConfigDir(r.homeDir, r.workspaceDir, r.scope, r.resolved.Agents)
	if telemetryDir != "" {
		prepare = append([]pipeline.Step{openCodeTelemetryStep{id: "prepare:opencode-telemetry", configDir: telemetryDir, checkOnly: true}}, prepare...)
	}
	if containsAgent(r.resolved.Agents, model.AgentOpenCode) {
		prepare = append([]pipeline.Step{openCodePluginDependencyPreflightStep{id: "prepare:opencode-plugin-dependency", homeDir: r.homeDir, consent: r.sdkConsent}}, prepare...)
	}
	// Check prerequisites before any SDK package-manager operation or telemetry.
	prepare = append([]pipeline.Step{checkDependenciesStep{id: "prepare:check-dependencies", profile: r.profile, homeDir: r.homeDir, selection: r.selection, resolved: r.resolved, channel: r.channel}}, prepare...)
	// The read-only settings refusal is prepended last so it runs first in the
	// prepare stage (issue #5035): an unsafe selected settings document must
	// fail before the SDK dependency install, telemetry, plugin, Persona,
	// Engram, or guidance steps can mutate any managed file.
	if containsAgent(r.resolved.Agents, model.AgentOpenCode) {
		prepare = append([]pipeline.Step{openCodeSettingsValidationStep{
			id:           "prepare:opencode-settings-validation",
			settingsPath: openCodeLoadedSettingsPath(r.homeDir, r.workspaceDir, opencodeagent.NewAdapter()),
			// Routing guidance is scheduled for every install agent.
			touchedKeys: openCodeSettingsWriterKeys(r.resolved.OrderedComponents, true),
		}}, prepare...)
	}
	apply = append(apply, rollbackRestoreStep{id: "apply:rollback-restore", state: r.state, homeDir: r.homeDir, workspaceDir: r.workspaceDir, telemetryConfigDir: telemetryDir})
	if telemetryDir != "" {
		apply = append(apply, openCodeTelemetryStep{id: "opencode:telemetry-runtime", configDir: telemetryDir, state: r.state})
	}
	if r.backgroundActivation != nil {
		apply = append(apply, openCodeBackgroundActivationStep{id: "opencode:background-activation", plan: r.backgroundActivation, state: r.state, ready: &r.runtimeReady})
	}
	if r.piBackgroundProjection != nil {
		apply = append(apply, piBackgroundProjectionStep{id: "pi:background-projection", plan: r.piBackgroundProjection})
	}

	// Before installing components, ensure modular agents have their system prompt hub.
	// This ensures that SDD or Engram can inject their modules even if Persona is skipped.
	for _, agent := range r.resolved.Agents {
		if agent == model.AgentKimi {
			apply = append(apply, kimiSystemPromptHubStep{id: "agent:kimi-prompt-hub", homeDir: r.homeDir})
		}
	}

	for _, agent := range r.resolved.Agents {
		apply = append(apply, agentInstallStep{
			id:       "agent:" + string(agent),
			agent:    agent,
			homeDir:  r.homeDir,
			profile:  r.profile,
			progress: r.progress,
		})
	}

	for _, agent := range r.resolved.Agents {
		if opencoderuntimeplugins.AgentReceivesManagedOpenCodePlugins(agent) {
			apply = append(apply, managedOpenCodePluginsInstallStep{id: "agent:managed-opencode-plugins:" + string(agent), agent: agent, homeDir: r.homeDir})
		}
	}

	for _, component := range r.resolved.OrderedComponents {
		if component == model.ComponentSDD {
			continue
		}
		step := componentApplyStep{
			id:           "component:" + string(component),
			component:    component,
			homeDir:      r.homeDir,
			workspaceDir: r.workspaceDir,
			scope:        r.scope,
			agents:       r.resolved.Agents,
			selection:    r.selection,
			profile:      r.profile,
			channel:      r.channel,
			state:        r.state,
		}
		step.backgroundPolicy = r.backgroundActivation != nil && r.backgroundActivation.Capability().Ready() && r.background.Effective == model.OpenCodeBackgroundOn
		apply = append(apply, step)
	}
	for _, agent := range r.resolved.Agents {
		if nativeReviewAgentSupported(agent) {
			apply = append(apply, nativeReviewAgentStep{id: "agent:native-review:" + string(agent), agent: agent, homeDir: r.homeDir, workspaceDir: r.workspaceDir, scope: r.scope, selection: r.selection, state: r.state})
		}
	}

	// Routing guidance is scheduled per agent and outside the component loop:
	// an agent that cannot choose between direct and delegated work is unusable,
	// so guidance must not depend on a component selection. It runs after the
	// components so their managed assets are in place before guidance is merged.
	for _, agent := range r.resolved.Agents {
		apply = append(apply, agentRoutingGuidanceStep{
			codexPhaseModels: r.selection.CodexPhaseModelAssignments,
			codexEfforts:     r.selection.CodexModelAssignments,
			codexCarrils:     r.selection.CodexCarrilModelAssignments,
			backgroundPolicy: r.backgroundActivation != nil && r.backgroundActivation.Capability().Ready() && r.background.Effective == model.OpenCodeBackgroundOn,
			legacySDD:        false,
			id:               "agent-guidance:" + string(agent),
			agent:            agent,
			homeDir:          r.homeDir,
			workspaceDir:     r.workspaceDir,
			scope:            r.scope,
		})
	}

	// Community tools run after persona and routing guidance, matching sync:
	// persona replaces whole prompt files for some agents, so CodeGraph guidance
	// injected before it would be dropped and re-added by the first sync.
	for _, tool := range r.selection.CommunityTools {
		apply = append(apply, communityToolInstallStep{id: "community-tool:" + string(tool), tool: tool, workspaceDir: r.workspaceDir, homeDir: r.homeDir, agents: r.resolved.Agents, state: r.state})
	}

	if needsCompatibilitySkillsRefresh(r.resolved.OrderedComponents) {
		apply = append(apply, compatibilitySkillsRefreshStep{
			id:          "component:compatibility-skills-refresh",
			homeDir:     r.homeDir,
			components:  r.resolved.OrderedComponents,
			selection:   r.selection,
			transaction: r.state.compatibilityTransaction,
			anchored:    usesAnchoredCompatibilityTransaction(),
		})
	}
	if containsAgent(r.resolved.Agents, model.AgentPi) {
		selected := r.selection.HasCommunityTool(model.CommunityToolCodeGraph)
		stepID := "community-tool:pi-codegraph-reconcile"
		if !selected {
			stepID = "community-tool:pi-codegraph-deselect"
		}
		apply = append(apply, piCodeGraphReconcileStep{id: stepID, homeDir: r.homeDir, workspaceDir: r.workspaceDir, selected: selected, state: r.state})
	}

	return pipeline.StagePlan{Prepare: prepare, Apply: apply}
}

type nativeReviewAgentStep struct {
	id, homeDir, workspaceDir string
	agent                     model.AgentID
	scope                     InstallScope
	selection                 model.Selection
	changedFiles              *[]string
	state                     *runtimeState
}

func (s nativeReviewAgentStep) ID() string { return s.id }

func nativeReviewAgentSupported(agent model.AgentID) bool {
	return reviewassets.NativeAgentsSupported(agent)
}

func (s nativeReviewAgentStep) Run() error {
	adapter := resolveAdapters([]model.AgentID{s.agent})[0]
	target := componentInjectionDirScoped(s.homeDir, s.workspaceDir, s.scope, adapter)
	res, err := reviewassets.InstallNativeAgents(target, adapter, reviewassets.InstallOptions{
		ClaudeModelAssignments:    s.selection.ClaudeModelAssignments,
		ClaudePhaseAssignments:    s.selection.ClaudePhaseAssignments,
		KiroModelAssignments:      s.selection.KiroModelAssignments,
		CodeGraphGuidanceMarkdown: nativeReviewCodeGraphGuidanceMarkdown(s.homeDir, s.selection.CommunityTools),
	})
	if err != nil {
		return fmt.Errorf("install native review agents for %q: %w", s.agent, err)
	}
	if s.changedFiles != nil {
		*s.changedFiles = append(*s.changedFiles, res.Files...)
	}
	if s.state != nil {
		for _, path := range res.Skipped {
			s.state.nativeReviewActions = append(s.state.nativeReviewActions, nativeReviewPreservedAction(path))
		}
	}
	return nil
}

func nativeReviewPreservedAction(path string) string {
	return fmt.Sprintf("Native review agent %s was preserved, not updated: ownership cannot be verified (missing ledger entry or differing recorded hash). This does not mean you customized the file. Keeping it unchanged is valid. To opt into management, back up this file and verify the backup, remove only this warned file, then rerun your existing gentle-ai install or gentle-ai sync command with the same runtime, scope, and model choices. See docs/rollback.md for per-file recovery. Gentle AI will not adopt or delete it automatically.", path)
}

type managedOpenCodePluginsInstallStep struct {
	id      string
	agent   model.AgentID
	homeDir string
}

// The installed package is the source of truth; a declared dependency alone
// does not make V2 plugin imports resolvable. Keep this check ahead of both
// telemetry and managed-plugin writes in install and sync prepare stages.
type openCodePluginDependencyPreflightStep struct {
	id, homeDir string
	consent     *OpenCodeSDKConsent
}

// OpenCodeSDKConsent describes one reviewed package-manager mutation, not a
// persisted preference. Only the TUI's separate confirmation supplies it.
type OpenCodeSDKConsent struct {
	ConfigDir      string
	Manager        string
	Dependency     string
	stateSignature [32]byte
	physicalDir    string
	dirInfo        os.FileInfo
	executable     string
	physicalExe    string
	exeInfo        os.FileInfo
	exeDigest      [32]byte
}

const openCodeSDKRegistry = "https://registry.npmjs.org"

// Tests shorten this deadline; production never waits indefinitely for a manager.
var openCodeSDKInstallTimeout = 2 * time.Minute

// OpenCodeSDKInstallProposal returns nil when no SDK operation is needed.
// A conflicting or unavailable package manager is never guessed.
func OpenCodeSDKInstallProposal(homeDir string) (*OpenCodeSDKConsent, error) {
	major, err := openCodeRuntimeMajorForManagedAssets()
	if err != nil {
		return nil, err
	}
	if major != opencodeactivation.RuntimeV2 {
		return nil, nil
	}
	dependency, err := major.PluginDependency()
	if err != nil {
		return nil, err
	}
	config := opencodeagent.NewAdapter().GlobalConfigDir(homeDir)
	if openCodeSDKInstalled(config, dependency) {
		return nil, nil
	}
	physicalDir, dirInfo, err := openCodeSDKConfigTarget(config)
	if err != nil {
		return nil, fmt.Errorf("OpenCode SDK automatic install cannot establish a stable config directory: %w; install %s manually before retrying", err, dependency)
	}
	if err := openCodeSDKCheckProjectConfig(config); err != nil {
		manager := openCodePluginPackageManager(config)
		if openCodeSDKManualBunOwner(config) {
			manager = "bun"
		} else if manager == "" {
			manager = "npm"
		}
		return nil, fmt.Errorf("automatic OpenCode SDK install refused: %w; run `%s` manually, then retry Gentle AI", err, openCodeSDKInstallContinuation(runtime.GOOS, config, manager, dependency))
	}
	executable, err := cmdLookPath("npm")
	if err != nil {
		return nil, fmt.Errorf("resolve approved npm executable: %w; install %s manually before retrying", err, dependency)
	}
	physicalExe, exeInfo, err := openCodeSDKPhysicalTarget(executable, false)
	if err != nil {
		return nil, fmt.Errorf("resolve approved npm executable: %w", err)
	}
	if blocker := openCodeSDKIsolationBlocker(executable, physicalExe); blocker != "" {
		// refusal:by-design world-action: credential isolation stays mandatory, so a manager that needs the user's shell environment runs manually
		return nil, fmt.Errorf("automatic OpenCode SDK install refused: npm on PATH %s and cannot run in the credential-isolated installer environment; run `%s` manually in your normal shell, then retry Gentle AI", blocker, openCodeSDKInstallContinuation(runtime.GOOS, config, "npm", dependency))
	}
	exeDigest, err := openCodeSDKExecutableDigest(physicalExe)
	if err != nil {
		return nil, fmt.Errorf("inspect approved npm executable: %w", err)
	}
	signature, err := openCodeSDKPackageState(config)
	if err != nil {
		return nil, err
	}
	return &OpenCodeSDKConsent{ConfigDir: config, Manager: "npm", Dependency: dependency, stateSignature: signature, physicalDir: physicalDir, dirInfo: dirInfo, executable: executable, physicalExe: physicalExe, exeInfo: exeInfo, exeDigest: exeDigest}, nil
}

func openCodeSDKConfigTarget(config string) (string, os.FileInfo, error) {
	if physical, info, err := openCodeSDKPhysicalTarget(config, true); err == nil {
		return physical, info, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", nil, err
	}
	// Resolve the nearest existing ancestor without creating a directory before consent.
	ancestor := config
	for {
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			// refusal:by-design operator-knowledge: no stable filesystem root exists for this target
			return "", nil, fmt.Errorf("no existing config ancestor")
		}
		ancestor = parent
		if _, err := os.Lstat(ancestor); err == nil {
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", nil, err
		}
	}
	physical, info, err := openCodeSDKPhysicalTarget(ancestor, true)
	if err != nil {
		return "", nil, err
	}
	rel, err := filepath.Rel(ancestor, config)
	if err != nil {
		return "", nil, err
	}
	return filepath.Join(physical, rel), info, nil
}

func openCodeSDKExecutableDigest(path string) ([32]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return [32]byte{}, err
	}
	defer f.Close()
	var digest [32]byte
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return [32]byte{}, err
	}
	copy(digest[:], h.Sum(nil))
	return digest, nil
}

func openCodeSDKPhysicalTarget(path string, directory bool) (string, os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", nil, err
	}
	if directory && !info.IsDir() {
		// refusal:by-design operator-knowledge: redirected or missing config cannot be safely targeted
		return "", nil, fmt.Errorf("config path is not a real directory")
	}
	physical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", nil, err
	}
	physical, err = filepath.Abs(physical)
	if err != nil {
		return "", nil, err
	}
	info, err = os.Stat(path)
	if err != nil {
		return "", nil, err
	}
	if directory && !info.IsDir() {
		// refusal:by-design operator-knowledge: only a physical package directory may be targeted
		return "", nil, fmt.Errorf("config path is not a directory")
	}
	if !directory && !info.Mode().IsRegular() {
		// refusal:by-design operator-knowledge: package manager executable cannot be identified
		return "", nil, fmt.Errorf("package manager is not a regular executable")
	}
	// On Windows os.Stat records only the path and reads the volume serial and
	// file index on the first os.SameFile call. Left lazy, the identity approved
	// at proposal time would be read from whatever occupies the path at launch,
	// so a replaced executable or config directory would compare equal to
	// itself. Comparing the result with itself loads and caches the identity
	// now; on POSIX the device and inode are already captured and this holds.
	if !os.SameFile(info, info) {
		// refusal:by-design operator-knowledge: a target whose file identity cannot be read cannot be pinned for consent
		return "", nil, fmt.Errorf("package manager target identity cannot be read")
	}
	return physical, info, nil
}

func openCodeSDKCheckProjectConfig(config string) error {
	for _, name := range []string{"package.json", "bun.lock", "bun.lockb", "package-lock.json", "npm-shrinkwrap.json", ".npmrc", "bunfig.toml", "node_modules"} {
		_, err := os.Lstat(filepath.Join(config, name))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect %s: %w", name, err)
		}
		if err == nil {
			// refusal:by-design human-authority: existing dependency state is manual-only
			return fmt.Errorf("%s is present in the OpenCode package directory, including symlinks", name)
		}
	}
	return nil
}

func (c *OpenCodeSDKConsent) matches(current *OpenCodeSDKConsent) bool {
	return c != nil && current != nil && c.ConfigDir == current.ConfigDir && c.Manager == current.Manager &&
		c.Dependency == current.Dependency && c.stateSignature == current.stateSignature &&
		c.physicalDir == current.physicalDir && c.executable == current.executable &&
		c.physicalExe == current.physicalExe && c.exeDigest == current.exeDigest &&
		((c.dirInfo == nil && current.dirInfo == nil) || (c.dirInfo != nil && current.dirInfo != nil && os.SameFile(c.dirInfo, current.dirInfo))) &&
		os.SameFile(c.exeInfo, current.exeInfo)
}

func openCodeSDKPackageState(config string) ([32]byte, error) {
	h := sha256.New()
	for _, name := range []string{"package.json", "bun.lock", "bun.lockb", "package-lock.json", "npm-shrinkwrap.json", ".npmrc", "bunfig.toml"} {
		data, err := os.ReadFile(filepath.Join(config, name))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return [32]byte{}, fmt.Errorf("inspect %s: %w", name, err)
		}
		fmt.Fprintf(h, "%s:%t:%d:", name, err == nil, len(data))
		h.Write(data)
	}
	var signature [32]byte
	copy(signature[:], h.Sum(nil))
	return signature, nil
}

func openCodeSDKInstalled(config, dependency string) bool {
	manifest := filepath.Join(config, "node_modules", "@opencode", "plugin", "package.json")
	data, err := os.ReadFile(manifest)
	if err != nil || len(data) > 1<<20 {
		return false
	}
	var pkg struct {
		Version string `json:"version"`
	}
	return json.Unmarshal(data, &pkg) == nil && pkg.Version == strings.TrimPrefix(dependency, "@opencode/plugin@")
}

func (s openCodePluginDependencyPreflightStep) ID() string { return s.id }

func (s openCodePluginDependencyPreflightStep) Run() error {
	major, err := openCodeRuntimeMajorForManagedAssets()
	if err != nil {
		return err
	}
	if major != opencodeactivation.RuntimeV2 {
		return nil
	}
	dependency, err := major.PluginDependency()
	if err != nil {
		return err
	}
	config := opencodeagent.NewAdapter().GlobalConfigDir(s.homeDir)
	if openCodeSDKInstalled(config, dependency) {
		return nil
	}
	if s.consent != nil {
		proposal, err := OpenCodeSDKInstallProposal(s.homeDir)
		if err != nil {
			return err
		}
		if !s.consent.matches(proposal) {
			// refusal:by-design human-authority: changed ownership needs fresh explicit confirmation
			return fmt.Errorf("OpenCode SDK package ownership changed after confirmation; review the operation again before retrying")
		}
		if err := openCodeSDKRunApprovedManager(proposal); err != nil {
			return err
		}
		if !openCodeSDKInstalled(config, dependency) {
			// refusal:by-design world-action: the external manager did not materialize the requested package
			return fmt.Errorf("%s SDK install verification failed: %s completed without materializing %s in %s; package-manager changes are not covered by Gentle AI rollback; run `%s` manually in your normal shell, then retry", proposal.Manager, proposal.Manager, dependency, config, openCodeSDKInstallContinuation(runtime.GOOS, config, proposal.Manager, dependency))
		}
		return nil
	}
	if err := openCodeSDKCheckProjectConfig(config); err == nil {
		location := openCodeSDKInstallContinuation(runtime.GOOS, config, "npm", dependency)
		if runtime.GOOS == "windows" {
			// refusal:by-design world-action: PowerShell must run the displayed manual install
			return fmt.Errorf("OpenCode V2 requires installed %s before managed plugins can be written; in PowerShell run: %s; then retry Gentle AI", dependency, location)
		}
		// refusal:by-design world-action: the operator must run the displayed manual install
		return fmt.Errorf("OpenCode V2 requires installed %s before managed plugins can be written; run `%s`, then retry Gentle AI", dependency, location)
	}
	manager := openCodePluginPackageManager(config)
	if manager == "" {
		if openCodePluginLockfileConflict(config) {
			// refusal:by-design operator-knowledge: conflicting lockfiles cannot safely establish ownership without user choice
			return fmt.Errorf("OpenCode V2 requires installed %s before managed plugins can be written; bun and npm lockfiles conflict in %s, so choose and reconcile the owning package manager before retrying", dependency, config)
		}
		if openCodeSDKManualBunOwner(config) {
			return openCodeSDKManualBunError(config, dependency)
		}
		// refusal:by-design world-action: no executable package manager is available for the existing package ownership
		return fmt.Errorf("OpenCode V2 requires installed %s before managed plugins can be written; no compatible package manager is available for %s", dependency, config)
	}
	if manager == "bun" {
		return openCodeSDKManualBunError(config, dependency)
	}
	location := openCodeSDKInstallContinuation(runtime.GOOS, config, manager, dependency)
	if runtime.GOOS == "windows" {
		// refusal:by-design world-action: this command is generated for PowerShell, not cmd.exe
		return fmt.Errorf("OpenCode V2 requires installed %s before managed plugins can be written; in PowerShell run: %s; then retry Gentle AI", dependency, location)
	}
	// refusal:by-design world-action: the runnable package-manager command is selected from the user's package ownership and quoted config path at runtime
	return fmt.Errorf("OpenCode V2 requires installed %s before managed plugins can be written; run `%s`, then retry Gentle AI", dependency, location)
}

// Bun ownership is a manual-only route even when Bun is unavailable on PATH.
func openCodeSDKManualBunOwner(config string) bool {
	if data, err := os.ReadFile(filepath.Join(config, "package.json")); err == nil {
		var pkg struct {
			PackageManager string `json:"packageManager"`
		}
		if json.Unmarshal(data, &pkg) == nil && strings.TrimSpace(pkg.PackageManager) != "" {
			owner := strings.ToLower(strings.TrimSpace(pkg.PackageManager))
			return owner == "bun" || strings.HasPrefix(owner, "bun@")
		}
	}
	for _, name := range []string{"bun.lock", "bun.lockb"} {
		if _, err := os.Stat(filepath.Join(config, name)); err == nil {
			return true
		}
	}
	return false
}

func openCodeSDKManualBunError(config, dependency string) error {
	location := openCodeSDKInstallContinuation(runtime.GOOS, config, "bun", dependency)
	if runtime.GOOS == "windows" {
		// refusal:by-design world-action: Bun-owned packages require an operator-run PowerShell continuation
		return fmt.Errorf("OpenCode V2 requires installed %s before managed plugins can be written; Bun-owned packages cannot be provisioned automatically; in PowerShell run: %s; then retry Gentle AI", dependency, location)
	}
	// refusal:by-design world-action: Bun-owned packages require an operator-run continuation
	return fmt.Errorf("OpenCode V2 requires installed %s before managed plugins can be written; Bun-owned packages cannot be provisioned automatically; run `%s`, then retry Gentle AI", dependency, location)
}

// The process receives no ambient package-manager auth, user configuration,
// proxy or runtime injection variables. The project config check is repeated
// immediately before launch; OS-level path swaps after that check remain a
// platform race and are not claimed to be eliminated here.
func openCodeSDKRunApprovedManager(proposal *OpenCodeSDKConsent) error {
	if proposal == nil || proposal.Manager != "npm" {
		// refusal:by-design human-authority: no consent can authorize an automatic Bun operation
		return fmt.Errorf("automatic OpenCode SDK install is available only for npm-owned packages")
	}
	currentDir, dirInfo, err := openCodeSDKConfigTarget(proposal.ConfigDir)
	if err != nil || currentDir != proposal.physicalDir || !os.SameFile(dirInfo, proposal.dirInfo) {
		// refusal:by-design human-authority: physical target changed since confirmation
		return fmt.Errorf("OpenCode SDK config directory changed before launch; review it again and retry")
	}
	currentExe, exeInfo, err := openCodeSDKPhysicalTarget(proposal.executable, false)
	if err != nil || currentExe != proposal.physicalExe || !os.SameFile(exeInfo, proposal.exeInfo) {
		// refusal:by-design human-authority: executable target changed since confirmation
		return fmt.Errorf("OpenCode package manager executable changed before launch; review it again and retry")
	}
	digest, err := openCodeSDKExecutableDigest(currentExe)
	if err != nil || digest != proposal.exeDigest {
		// refusal:by-design human-authority: executable bytes changed since confirmation
		return fmt.Errorf("OpenCode package manager executable changed before launch; review it again and retry")
	}
	if err := openCodeSDKCheckProjectConfig(proposal.ConfigDir); err != nil {
		return fmt.Errorf("automatic OpenCode SDK install refused: %w; run `%s` manually, then retry Gentle AI", err, openCodeSDKInstallContinuation(runtime.GOOS, proposal.ConfigDir, proposal.Manager, proposal.Dependency))
	}
	state, err := openCodeSDKPackageState(proposal.ConfigDir)
	if err != nil || state != proposal.stateSignature {
		// refusal:by-design human-authority: package state changed after consent
		return fmt.Errorf("OpenCode package state changed before launch; review it again and retry")
	}
	if err := os.MkdirAll(proposal.ConfigDir, 0755); err != nil {
		return fmt.Errorf("create OpenCode config directory after SDK consent: %w", err)
	}
	physicalConfig, _, err := openCodeSDKPhysicalTarget(proposal.ConfigDir, true)
	if err != nil || physicalConfig != proposal.physicalDir {
		// refusal:by-design human-authority: changed physical target needs fresh consent
		return fmt.Errorf("OpenCode SDK config directory changed before launch; review it again and retry")
	}
	if err := openCodeSDKCheckProjectConfig(proposal.ConfigDir); err != nil {
		return fmt.Errorf("automatic OpenCode SDK install refused: %w; run `%s` manually, then retry Gentle AI", err, openCodeSDKInstallContinuation(runtime.GOOS, proposal.ConfigDir, proposal.Manager, proposal.Dependency))
	}
	isolated, err := os.MkdirTemp("", "gentle-ai-opencode-sdk-")
	if err != nil {
		return fmt.Errorf("prepare isolated SDK installer environment: %w", err)
	}
	defer os.RemoveAll(isolated)
	ctx, cancel := context.WithTimeout(context.Background(), openCodeSDKInstallTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, proposal.physicalExe, openCodeSDKInstallArgs(proposal.Dependency, proposal.physicalDir)...)
	cmd.Dir = proposal.ConfigDir
	cmd.Env = openCodeSDKIsolatedEnv(isolated, proposal)
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	cmd.WaitDelay = 200 * time.Millisecond
	err = cmd.Run()
	if err == nil {
		return nil
	}
	// Never reflect raw command errors or output: both may embed credentials.
	// Only the failure class (start, exit code, signal, deadline) is reported.
	// refusal:by-design world-action: package manager failed without exposing its possibly sensitive output
	return fmt.Errorf("%s SDK install failed: %s %s; package-manager changes are not covered by Gentle AI rollback; run `%s` manually in your normal shell, then retry", proposal.Manager, proposal.Manager, openCodeSDKFailureClass(err, ctx.Err(), openCodeSDKInstallTimeout), openCodeSDKInstallContinuation(runtime.GOOS, proposal.ConfigDir, proposal.Manager, proposal.Dependency))
}

func openCodeSDKIsolatedEnv(isolated string, proposal *OpenCodeSDKConsent) []string {
	path := strings.Join(openCodeSDKIsolatedPathDirs(proposal.executable, proposal.physicalExe), string(os.PathListSeparator))
	env := []string{
		"PATH=" + path, "HOME=" + isolated, "USERPROFILE=" + isolated,
		"XDG_CONFIG_HOME=" + isolated, "APPDATA=" + isolated, "LOCALAPPDATA=" + isolated,
		"TMPDIR=" + isolated, "TMP=" + isolated, "TEMP=" + isolated,
		"NPM_CONFIG_USERCONFIG=" + filepath.Join(isolated, "user.npmrc"),
		"NPM_CONFIG_GLOBALCONFIG=" + filepath.Join(isolated, "global.npmrc"),
		"NPM_CONFIG_CACHE=" + filepath.Join(isolated, "npm-cache"),
		"NPM_CONFIG_REGISTRY=" + openCodeSDKRegistry,
		"CI=true", "NO_COLOR=1",
	}
	if runtime.GOOS == "windows" {
		env = append(env, "SystemRoot="+os.Getenv("SystemRoot"))
	}
	return env
}

func openCodeSDKInstallContinuation(goos, config, manager, dependency string) string {
	arguments := "bun add " + dependency
	if manager == "npm" {
		arguments = "npm install --save --no-audit --no-fund " + dependency
	}
	if goos == "windows" {
		return "Set-Location -LiteralPath '" + strings.ReplaceAll(config, "'", "''") + "'; if ($?) { " + arguments + " }"
	}
	return "cd " + shellQuoteOpenCodePluginPath(config) + " && " + arguments
}

func openCodeSDKInstallArgs(dependency, config string) []string {
	return []string{"install", "--save", "--no-audit", "--no-fund", "--ignore-scripts", "--workspaces=false", "--prefix=" + config, "--registry=" + openCodeSDKRegistry, dependency}
}

func openCodePluginPackageManager(config string) string {
	preferred := ""
	if data, err := os.ReadFile(filepath.Join(config, "package.json")); err == nil {
		var pkg struct {
			PackageManager string `json:"packageManager"`
		}
		if json.Unmarshal(data, &pkg) == nil {
			name := strings.ToLower(strings.TrimSpace(pkg.PackageManager))
			switch {
			case name == "bun" || strings.HasPrefix(name, "bun@"):
				preferred = "bun"
			case name == "npm" || strings.HasPrefix(name, "npm@"):
				preferred = "npm"
			case name != "":
				return ""
			}
		}
	}
	if preferred == "" {
		if openCodePluginLockfileConflict(config) {
			return ""
		}
		for _, entry := range []struct{ name, manager string }{
			{"bun.lock", "bun"}, {"bun.lockb", "bun"}, {"package-lock.json", "npm"}, {"npm-shrinkwrap.json", "npm"},
		} {
			if _, err := os.Stat(filepath.Join(config, entry.name)); err == nil {
				preferred = entry.manager
				break
			}
		}
	}
	if preferred != "" {
		if _, err := cmdLookPath(preferred); err == nil {
			return preferred
		}
		return ""
	}
	for _, name := range []string{"bun", "npm"} {
		if _, err := cmdLookPath(name); err == nil {
			return name
		}
	}
	return ""
}

func openCodePluginLockfileConflict(config string) bool {
	if data, err := os.ReadFile(filepath.Join(config, "package.json")); err == nil {
		var pkg struct {
			PackageManager string `json:"packageManager"`
		}
		if json.Unmarshal(data, &pkg) == nil && strings.TrimSpace(pkg.PackageManager) != "" {
			return false
		}
	}
	owned := func(names ...string) bool {
		for _, name := range names {
			if _, err := os.Stat(filepath.Join(config, name)); err == nil {
				return true
			}
		}
		return false
	}
	return owned("bun.lock", "bun.lockb") && owned("package-lock.json", "npm-shrinkwrap.json")
}

func shellQuoteOpenCodePluginPath(path string) string {
	return "'" + strings.ReplaceAll(path, "'", "'\"'\"'") + "'"
}

func (s managedOpenCodePluginsInstallStep) ID() string { return s.id }
func (s managedOpenCodePluginsInstallStep) Run() error {
	adapter, err := agents.NewAdapter(s.agent)
	if err != nil {
		return err
	}
	_, err = opencoderuntimeplugins.Install(s.homeDir, adapter)
	return err
}

// legacyTriggerRulesSection is the retired managed section that used to carry
// prompt-owned WorkRun ceremony. Nothing authors it anymore, so any copy still
// on disk is a stale instruction to invoke authority that no longer exists —
// it has to be removed, not refreshed.
const legacyTriggerRulesSection = "trigger-rules"

// routingReviewContract renders the native review execution contract the
// orchestrator guidance embeds. reviewassets already depends on agentguidance,
// so the installer passes it through RoutingOptions.ReviewContract instead of
// agentguidance importing it. It is a variable only so tests can prove the
// routing step fails closed without it.
var routingReviewContract agentguidance.ReviewContractSource = reviewassets.ReviewExecutionContractFor

// agentRoutingGuidanceStep delivers the organic routing guidance for one agent.
//
// It is deliberately not a component step. Routing guidance is what lets an
// agent choose between direct, delegated, and proposed work at all, so every
// configured agent receives it whether or not the optional SDD component was
// selected (issue #1794).
type agentRoutingGuidanceStep struct {
	codexPhaseModels map[string]string
	codexEfforts     map[string]model.CodexEffort
	codexCarrils     map[string]string
	backgroundPolicy bool
	legacySDD        bool
	id               string
	agent            model.AgentID
	homeDir          string
	workspaceDir     string
	scope            InstallScope

	// changedFiles is the shared sync accumulator. Install leaves it nil and
	// reports progress through the pipeline instead.
	changedFiles *[]string
}

func (s agentRoutingGuidanceStep) ID() string { return s.id }

func (s agentRoutingGuidanceStep) Run() error {
	adapter, err := agents.NewAdapter(s.agent)
	if err != nil {
		return fmt.Errorf("create adapter for %q: %w", s.agent, err)
	}

	retainedHooks, err := agenthooks.InstallRetainedClaudeHooks(s.homeDir, adapter)
	if err != nil {
		return fmt.Errorf("install retained Claude hooks for %q: %w", s.agent, err)
	}
	if s.changedFiles != nil && retainedHooks.Changed {
		*s.changedFiles = append(*s.changedFiles, retainedHooks.Files...)
	}
	telemetryHooks, err := agenthooks.InstallCodexTelemetry(s.homeDir, adapter)
	if err != nil {
		return fmt.Errorf("install Codex telemetry hooks for %q: %w", s.agent, err)
	}
	if s.changedFiles != nil && telemetryHooks.Changed {
		*s.changedFiles = append(*s.changedFiles, telemetryHooks.Files...)
	}
	hooks, err := agenthooks.InstallSkillRegistry(s.homeDir, adapter)
	if err != nil {
		return fmt.Errorf("install skill-registry hook for %q: %w", s.agent, err)
	}
	if s.changedFiles != nil && hooks.Changed {
		*s.changedFiles = append(*s.changedFiles, hooks.Files...)
	}

	// Pi owns its prompt delivery, but old installs left only allowlisted
	// Gentle AI sections in APPEND_SYSTEM.md. Retire those before skipping prompt
	// injection so install and every sync path converge without recreating routing.
	if adapter.Agent() == model.AgentPi {
		result, err := agentguidance.RetirePiSystemPromptBlocks(s.homeDir, adapter)
		if err != nil {
			return fmt.Errorf("retire stale Pi system prompt blocks: %w", err)
		}
		if s.changedFiles != nil && result.Changed {
			*s.changedFiles = append(*s.changedFiles, result.Files...)
		}
		return nil
	}
	if !adapter.SupportsSystemPrompt() {
		return nil
	}

	targetDir := routingGuidanceDir(s.homeDir, s.workspaceDir, s.scope, adapter)

	var defaultAgentPlan *opencodedefault.InstallPlan
	if s.agent == model.AgentOpenCode {
		settingsPath := routingGuidanceOptions(s.homeDir, s.workspaceDir, adapter).SettingsPath
		// The default-agent plan must observe settings before the managed
		// orchestrator is merged below; see opencodedefault.PrepareInstall.
		defaultAgentPlan, err = opencodedefault.PrepareInstall(settingsPath)
		if err != nil {
			return fmt.Errorf("prepare OpenCode default agent: %w", err)
		}
		changed, _, err := migrateLegacyOpenCodeAgents(settingsPath, model.AgentOpenCode)
		if changed && s.changedFiles != nil {
			*s.changedFiles = append(*s.changedFiles, settingsPath)
		}
		if err != nil {
			return fmt.Errorf("migrate legacy OpenCode agents: %w", err)
		}
	}
	// legacyRemovedKilo carries the review agents the v3.7.0 marker migration
	// removed in this run, so their task permissions can be retired below.
	var legacyRemovedKilo []string
	if s.agent == model.AgentKilocode {
		// Kilo received the same marked v3.7.0 overlay (#4471). Migrate before
		// routing injection so a marked orchestrator prompt is not preserved.
		settingsPath := adapter.SettingsPath(targetDir)
		changed, removed, err := migrateLegacyOpenCodeAgents(settingsPath, model.AgentKilocode)
		legacyRemovedKilo = removed
		if changed && s.changedFiles != nil {
			*s.changedFiles = append(*s.changedFiles, settingsPath)
		}
		if err != nil {
			return fmt.Errorf("migrate legacy Kilo agents: %w", err)
		}
	}

	options := routingGuidanceOptions(s.homeDir, s.workspaceDir, adapter)

	// Strip first: an installation upgraded from an older release still carries
	// the retired block, and leaving it beside fresh guidance would hand the
	// agent two conflicting sets of instructions.
	stripped, err := stripLegacyTriggerRules(targetDir, adapter, options.SettingsPath)
	if err != nil {
		return err
	}

	if s.agent == model.AgentCodex {
		options.CodexPhaseModelAssignments = s.codexPhaseModels
		options.CodexModelAssignments = s.codexEfforts
		options.CodexCarrilModelAssignments = s.codexCarrils
	}
	injected, err := agentguidance.InjectRoutingWithOptions(targetDir, s.agent, options)
	if err != nil {
		return fmt.Errorf("inject routing guidance for %q: %w", s.agent, err)
	}

	s.recordChanged(stripped)
	s.recordChanged(injected)
	strict, err := agentguidance.InjectStrictTDDWithOptions(targetDir, s.agent, false, options)
	if err != nil {
		return fmt.Errorf("retire legacy strict TDD guidance for %q: %w", s.agent, err)
	}
	s.recordChanged(strict)
	if s.agent == model.AgentOpenCode && !s.legacySDD {
		policy, err := agentguidance.ApplyOpenCodeBackgroundPolicy(options.SettingsPath, s.backgroundPolicy)
		if err != nil {
			return fmt.Errorf("apply OpenCode background policy: %w", err)
		}
		s.recordChanged(policy)
	}
	if s.agent == model.AgentKilocode {
		// Kilo reads an OpenCode-compatible config; v3.7.0 disabled sharing
		// there too, only when the user had not chosen a mode (#4471).
		settingsPath := adapter.SettingsPath(targetDir)
		shareChanged, err := opencodedefault.ApplyShareDefault(settingsPath)
		if err != nil {
			return fmt.Errorf("default Kilo share mode: %w", err)
		}
		if shareChanged && s.changedFiles != nil {
			*s.changedFiles = append(*s.changedFiles, settingsPath)
		}
		rolesChanged, err := installOpenCodeReviewProviderRoles(settingsPath, model.AgentKilocode)
		if err != nil {
			return fmt.Errorf("install Kilo review provider roles: %w", err)
		}
		parityChanged, err := installOpenCodeFamilyParityAgents(settingsPath, model.AgentKilocode)
		if err != nil {
			return fmt.Errorf("install Kilo parity agents: %w", err)
		}
		retiredChanged, err := retireOpenCodeFamilyReviewAgents(settingsPath, model.AgentKilocode, legacyRemovedKilo)
		if err != nil {
			return fmt.Errorf("retire Kilo review agents: %w", err)
		}
		if (rolesChanged || parityChanged || retiredChanged) && s.changedFiles != nil {
			*s.changedFiles = append(*s.changedFiles, settingsPath)
		}
	}
	if s.agent == model.AgentOpenCode {
		changed, err := installOpenCodeReviewProviderRoles(options.SettingsPath, model.AgentOpenCode)
		if err != nil {
			return fmt.Errorf("install OpenCode review provider roles: %w", err)
		}
		if changed && s.changedFiles != nil {
			*s.changedFiles = append(*s.changedFiles, options.SettingsPath)
		}
		parityChanged, err := installOpenCodeODDParityAgents(options.SettingsPath)
		if err != nil {
			return fmt.Errorf("install OpenCode ODD parity agents: %w", err)
		}
		if parityChanged && s.changedFiles != nil {
			*s.changedFiles = append(*s.changedFiles, options.SettingsPath)
		}
		// default_agent and share were owned by the retired SDD overlay; the
		// OpenCode routing owner restores them with v3.7.0 semantics (#4471).
		defaultChanged, err := defaultAgentPlan.Apply()
		if err != nil {
			return fmt.Errorf("set OpenCode default agent: %w", err)
		}
		if defaultChanged && s.changedFiles != nil {
			*s.changedFiles = append(*s.changedFiles, options.SettingsPath, opencodedefault.OwnershipPath(options.SettingsPath))
		}
		shareChanged, err := opencodedefault.ApplyShareDefault(options.SettingsPath)
		if err != nil {
			return fmt.Errorf("default OpenCode share mode: %w", err)
		}
		if shareChanged && s.changedFiles != nil {
			*s.changedFiles = append(*s.changedFiles, options.SettingsPath)
		}
	}
	return nil
}

// migrateLegacyOpenCodeAgents runs inside the routing apply step, after the
// transaction snapshot and before guidance and parity overlays. The v3.7.0
// marker is ownership proof: retired SDD roles and built-in overrides are
// removed entirely (their model/variant no longer have a meaningful target).
// Current roles are reset to model/variant only, so subsequent writers install
// their current prompt and permissions without retaining obsolete keys. The
// current role set is per runtime: Kilo shares the OpenCode config format but
// not the full role set (see opencodeagents.Roles).
//
// It also reports the review agents it removed under the marker, which is the
// ownership proof retireOpenCodeFamilyReviewAgents needs to drop their
// orchestrator task permissions in the same run.
func migrateLegacyOpenCodeAgents(settingsPath string, agent model.AgentID) (bool, []string, error) {
	raw, err := os.ReadFile(settingsPath)
	if os.IsNotExist(err) {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, err
	}
	root, err := filemerge.UnmarshalJSONObject(raw)
	if err != nil {
		return false, nil, err
	}
	agents, _ := root["agent"].(map[string]any)
	current := map[string]bool{"gentle-orchestrator": true}
	for _, name := range opencodeagents.Roles(agent) {
		current[name] = true
	}
	rdd := model.SupportsReceiptDrivenDevelopment(agent)
	changed := false
	var removedReview []string
	for name, value := range agents {
		entry, ok := value.(map[string]any)
		if !ok || entry["__managed_by"] != "gentle-ai/sdd" {
			continue
		}
		changed = true
		switch {
		case name == "general", name == "explore", strings.HasPrefix(name, "sdd-"):
			delete(agents, name)
		case !rdd && opencodeagents.IsReview(name):
			// The v3.7.0 marker proves ownership of a review agent this
			// runtime no longer receives.
			delete(agents, name)
			removedReview = append(removedReview, name)
		case current[name]:
			fresh := map[string]any{}
			for _, field := range []string{"model", "variant"} {
				if value, ok := entry[field]; ok {
					fresh[field] = value
				}
			}
			agents[name] = fresh
		default:
			delete(entry, "__managed_by")
		}
	}
	if !changed {
		return false, nil, nil
	}
	// Match the existing OpenCode writers: JSONC input is accepted and the
	// settings document is normalized to JSON on write, retaining permission
	// rule order and all unrelated top-level values.
	encoded, err := filemerge.MarshalJSONPreservingPermissions(raw, root)
	if err != nil {
		return false, nil, err
	}
	result, err := filemerge.WriteFileAtomic(settingsPath, append(encoded, '\n'), filemerge.ExistingFileMode(settingsPath, 0o644))
	// WriteFileAtomic may publish the replacement and still report an error;
	// keep its Changed state so the caller records the file either way.
	return result.Changed, removedReview, err
}

// retireOpenCodeFamilyReviewAgents removes the review agents earlier releases
// wrote to a runtime without receipt-driven development. An entry is removed
// only while it still has the exact managed shape (a model or variant the user
// assigned does not make it theirs); any other entry under a review name is the
// user's and is preserved together with its orchestrator task permission.
//
// A task permission is dropped only for a review agent this run removed as
// Gentle AI's: here in the managed shape, or earlier in the same run under the
// v3.7.0 marker (legacyRemoved). A permission whose agent was already absent
// carries no ownership proof and is the user's.
func retireOpenCodeFamilyReviewAgents(settingsPath string, agent model.AgentID, legacyRemoved []string) (bool, error) {
	if model.SupportsReceiptDrivenDevelopment(agent) {
		return false, nil
	}
	raw, err := os.ReadFile(settingsPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	root, err := filemerge.UnmarshalJSONObject(raw)
	if err != nil {
		return false, err
	}
	agents, _ := root["agent"].(map[string]any)
	if agents == nil {
		return false, nil
	}
	changed := false
	removed := map[string]bool{}
	for _, name := range legacyRemoved {
		removed[name] = true
	}
	for _, name := range opencodeagents.ReviewNames() {
		entry, ok := agents[name].(map[string]any)
		if !ok {
			continue
		}
		owned, err := opencodeagents.Shape(name, entry)
		if err != nil {
			return false, err
		}
		if owned {
			delete(agents, name)
			removed[name] = true
			changed = true
		}
	}
	orchestrator, _ := agents["gentle-orchestrator"].(map[string]any)
	permission, _ := orchestrator["permission"].(map[string]any)
	if task, ok := permission["task"].(map[string]any); ok {
		for _, name := range opencodeagents.ReviewNames() {
			if _, present := agents[name]; present || !removed[name] {
				continue
			}
			if _, granted := task[name]; granted {
				delete(task, name)
				changed = true
			}
		}
	}
	if !changed {
		return false, nil
	}
	encoded, err := filemerge.MarshalJSONPreservingPermissions(raw, root)
	if err != nil {
		return false, err
	}
	result, err := filemerge.WriteFileAtomic(settingsPath, append(encoded, '\n'), filemerge.ExistingFileMode(settingsPath, 0o644))
	return result.Changed, err
}

// Provider STATUS can issue these roles without SDD. Keep their task permissions
// with the OpenCode routing owner, not with the retired SDD overlay.
func installOpenCodeReviewProviderRoles(settingsPath string, agent model.AgentID) (bool, error) {
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		return false, err
	}
	task := map[string]any{}
	for _, name := range opencodeagents.Roles(agent) {
		task[name] = "allow"
	}
	roles := map[string]any{
		"gentle-orchestrator": map[string]any{"permission": map[string]any{"task": task}},
	}
	if model.SupportsReceiptDrivenDevelopment(agent) {
		roles["review-refuter"] = opencodeagents.Refuter()
	}
	if agent == model.AgentOpenCode {
		roles["review-validator"] = opencodeagents.Validator()
	}
	overlay, err := json.Marshal(map[string]any{"agent": roles})
	if err != nil {
		return false, err
	}
	merged, err := filemerge.MergeJSONObjects(raw, overlay)
	if err != nil {
		return false, err
	}
	result, err := filemerge.WriteFileAtomic(settingsPath, merged, filemerge.ExistingFileMode(settingsPath, 0o644))
	return result.Changed, err
}

// installOpenCodeODDParityAgents installs the ODD/JD/review-lens subagents at
// functional parity with Gentle Shell's global agents (#4471). Prior to this,
// only gentle-orchestrator, gentleman, review-refuter, and review-validator
// survived the SDD overlay's retirement; JD and the four review lenses were
// dropped as collateral.
func installOpenCodeODDParityAgents(settingsPath string) (bool, error) {
	return installOpenCodeFamilyParityAgents(settingsPath, model.AgentOpenCode)
}

func installOpenCodeFamilyParityAgents(settingsPath string, agent model.AgentID) (bool, error) {
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		return false, err
	}
	specs := opencodeagents.Parity(agent)
	agentsOverlay := make(map[string]any, len(specs))
	for _, spec := range specs {
		entry, err := opencodeagents.Entry(spec)
		if err != nil {
			return false, err
		}
		agentsOverlay[spec.Name] = entry
	}
	overlay, err := json.Marshal(map[string]any{"agent": agentsOverlay})
	if err != nil {
		return false, err
	}
	merged, err := filemerge.MergeJSONObjects(raw, overlay)
	if err != nil {
		return false, err
	}
	result, err := filemerge.WriteFileAtomic(settingsPath, merged, filemerge.ExistingFileMode(settingsPath, 0o644))
	return result.Changed, err
}

func (s agentRoutingGuidanceStep) recordChanged(result agentguidance.Result) {
	if s.changedFiles == nil || !result.Changed {
		return
	}
	*s.changedFiles = append(*s.changedFiles, result.Files...)
}

// stripLegacyTriggerRules removes the retired section from the scope the agent
// actually loads, mirroring the three delivery strategies routing guidance uses.
//
// Removal reuses filemerge.InjectMarkdownSection with empty content, which is
// already the defined "delete this section" operation, so no second merge
// implementation exists that could drift from the injector.
//
// settingsPath is the caller-resolved settings authority the routing injector
// writes (OpenCode's effective loaded file); when empty, orchestrator-prompt
// adapters keep their targetDir-derived settings path.
func stripLegacyTriggerRules(targetDir string, adapter agents.Adapter, settingsPath string) (agentguidance.Result, error) {
	switch {
	case adapter.Agent() == model.AgentOpenCode || adapter.Agent() == model.AgentKilocode:
		if settingsPath == "" {
			settingsPath = adapter.SettingsPath(targetDir)
		}
		return stripLegacyTriggerRulesFromOrchestrator(settingsPath)
	case adapter.SystemPromptStrategy() == model.StrategyJinjaModules:
		return removeLegacyTriggerRulesModule(filepath.Join(adapter.GlobalConfigDir(targetDir), legacyTriggerRulesSection+".md"))
	default:
		return stripLegacyTriggerRulesFromPrompt(adapter.SystemPromptFile(targetDir))
	}
}

func stripLegacyTriggerRulesFromPrompt(promptPath string) (agentguidance.Result, error) {
	if strings.TrimSpace(promptPath) == "" {
		return agentguidance.Result{}, nil
	}

	existing, err := os.ReadFile(promptPath)
	if err != nil {
		if os.IsNotExist(err) {
			return agentguidance.Result{}, nil
		}
		return agentguidance.Result{}, fmt.Errorf("read system prompt %q: %w", promptPath, err)
	}

	updated := filemerge.InjectMarkdownSection(string(existing), legacyTriggerRulesSection, "")
	if updated == string(existing) {
		return agentguidance.Result{}, nil
	}

	writeResult, err := filemerge.WriteFileAtomic(promptPath, []byte(updated), 0o644)
	if err != nil {
		return agentguidance.Result{}, err
	}
	return agentguidance.Result{Changed: writeResult.Changed, Files: []string{promptPath}}, nil
}

// removeLegacyTriggerRulesModule deletes the standalone include module used by
// Jinja adapters. Their router template includes it with "ignore missing", so
// deleting the file is exactly the section removal for that strategy.
func removeLegacyTriggerRulesModule(modulePath string) (agentguidance.Result, error) {
	if strings.TrimSpace(modulePath) == "" {
		return agentguidance.Result{}, nil
	}

	if err := os.Remove(modulePath); err != nil {
		if os.IsNotExist(err) {
			return agentguidance.Result{}, nil
		}
		return agentguidance.Result{}, fmt.Errorf("remove legacy %q module %q: %w", legacyTriggerRulesSection, modulePath, err)
	}
	return agentguidance.Result{Changed: true, Files: []string{modulePath}}, nil
}

// stripLegacyTriggerRulesFromOrchestrator removes the section from the managed
// orchestrator prompt inside an agent settings document.
//
// Every unexpected shape yields a silent no-op rather than an error: this is
// best-effort cleanup, and the routing injector that runs immediately after is
// the fail-closed authority on an unreadable settings document. JSONC settings
// (opencode.jsonc) are rewritten with the JSONC-preserving merge, so comments
// and trailing commas around untouched members survive; a JSONC refusal is the
// same silent no-op, because the injector fails closed on identical conditions.
func stripLegacyTriggerRulesFromOrchestrator(settingsPath string) (agentguidance.Result, error) {
	if strings.TrimSpace(settingsPath) == "" {
		return agentguidance.Result{}, nil
	}

	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return agentguidance.Result{}, nil
		}
		return agentguidance.Result{}, fmt.Errorf("read agent settings %q: %w", settingsPath, err)
	}

	prompt, ok := managedOrchestratorPromptFromSettings(raw)
	if !ok {
		return agentguidance.Result{}, nil
	}

	updated := filemerge.InjectMarkdownSection(prompt, legacyTriggerRulesSection, "")
	if updated == prompt {
		return agentguidance.Result{}, nil
	}

	overlay, err := json.Marshal(map[string]any{
		"agent": map[string]any{
			opencodedefault.ManagedAgent: map[string]any{"prompt": updated},
		},
	})
	if err != nil {
		return agentguidance.Result{}, fmt.Errorf("encode legacy %q removal for %q: %w", legacyTriggerRulesSection, settingsPath, err)
	}

	merged, err := filemerge.MergeJSONObjectsForPath(settingsPath, raw, overlay)
	if err != nil {
		if strings.HasSuffix(settingsPath, ".jsonc") {
			// A JSONC refusal (duplicate keys, an escaped spelling of a touched
			// key, or comments inside the touched agent value) must leave the
			// user's document byte-for-byte intact. Cleanup stays best-effort:
			// the routing injector that runs immediately after fails closed on
			// the same conditions, so a silent no-op loses nothing.
			return agentguidance.Result{}, nil
		}
		return agentguidance.Result{}, fmt.Errorf("merge legacy %q removal into %q: %w", legacyTriggerRulesSection, settingsPath, err)
	}

	writeResult, err := filemerge.WriteFileAtomic(settingsPath, merged, filemerge.ExistingFileMode(settingsPath, 0o644))
	if err != nil {
		return agentguidance.Result{}, err
	}
	return agentguidance.Result{Changed: writeResult.Changed, Files: []string{settingsPath}}, nil
}

func managedOrchestratorPromptFromSettings(raw []byte) (string, bool) {
	settings, err := filemerge.UnmarshalJSONObject(raw)
	if err != nil {
		return "", false
	}
	agentsMap, ok := settings["agent"].(map[string]any)
	if !ok {
		return "", false
	}
	orchestrator, ok := agentsMap[opencodedefault.ManagedAgent].(map[string]any)
	if !ok {
		return "", false
	}
	prompt, ok := orchestrator["prompt"].(string)
	return prompt, ok
}

type piCodeGraphReconcileStep struct {
	id, homeDir, workspaceDir string
	selected                  bool
	state                     *runtimeState
}

var reconcilePiCodeGraph = communitytool.ReconcilePiCodeGraph

func (s piCodeGraphReconcileStep) ID() string { return s.id }
func (s piCodeGraphReconcileStep) Run() error {
	result, err := reconcilePiCodeGraph(communitytool.PiCodeGraphOptions{HomeDir: s.homeDir, WorkspaceDir: s.workspaceDir, Selected: s.selected})
	result, err = communitytool.PreservePiCodeGraphPending(result, err)
	if err == nil && s.state != nil {
		s.state.piCodeGraph = &result
	}
	return err
}

// Rollback removes only the manifest-owned Pi CodeGraph artifacts created by
// this late pipeline step. This covers overlays discovered after package
// installation, which cannot be part of the pre-install static snapshot.
func (s piCodeGraphReconcileStep) Rollback() error {
	_, err := communitytool.UninstallPiCodeGraph(s.homeDir)
	return err
}

type prepareBackupStep struct {
	id          string
	snapshotter backup.Snapshotter
	snapshotDir string
	targets     []string
	targetErr   error
	state       *runtimeState

	// backupRoot is the parent directory of all backup snapshots.
	// When set, deduplication (DuplicateManifest) and retention pruning (Prune) are
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

func manifestTargetsMatch(manifest backup.Manifest, targets []string) bool {
	current := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		current[filepath.Clean(target)] = struct{}{}
	}
	historical := make(map[string]struct{}, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		historical[entry.OriginalPath] = struct{}{}
	}
	if len(current) != len(historical) {
		return false
	}
	for target := range current {
		if _, ok := historical[target]; !ok {
			return false
		}
	}
	return true
}

func (s prepareBackupStep) Run() error {
	if s.targetErr != nil {
		return fmt.Errorf("resolve backup targets: %w", s.targetErr)
	}
	// Deduplication: skip snapshot creation when content is identical to the
	// most recent backup. Only active when backupRoot is set.
	if s.backupRoot != "" {
		checksum, err := backup.ComputeChecksum(s.targets)
		if err == nil && checksum != "" {
			if manifest, duplicate, dupErr := backup.DuplicateManifest(s.backupRoot, checksum); dupErr != nil {
				log.Printf("backup: check duplicate: %v", dupErr)
			} else if duplicate && manifestTargetsMatch(manifest, s.targets) {
				rollbackDir, err := os.MkdirTemp("", "gentle-ai-rollback-*")
				if err != nil {
					return fmt.Errorf("create transaction snapshot directory: %w", err)
				}
				manifest, err := s.snapshotter.Create(rollbackDir, s.targets)
				if err != nil {
					_ = os.RemoveAll(rollbackDir)
					return fmt.Errorf("create transaction snapshot: %w", err)
				}
				s.state.manifest = manifest
				s.state.rollbackSnapshotDir = rollbackDir
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
	id                 string
	state              *runtimeState
	homeDir            string
	workspaceDir       string
	telemetryConfigDir string // Selected adapter authority, potentially outside HOME via XDG.
}

type openCodeBackgroundActivationStep struct {
	id    string
	plan  *opencodeactivation.ActivationPlan
	state *runtimeState
	ready *bool
}

func (s openCodeBackgroundActivationStep) ID() string { return s.id }

func (s openCodeBackgroundActivationStep) Run() error {
	if err := s.plan.Apply(); err != nil {
		return fmt.Errorf("apply managed OpenCode background activation: %w", err)
	}
	if s.ready != nil {
		*s.ready = s.plan.Capability().Ready()
	}
	return nil
}

func (s openCodeBackgroundActivationStep) Rollback() error { return s.plan.Rollback() }

func (s rollbackRestoreStep) ID() string {
	return s.id
}

func (s rollbackRestoreStep) Run() error {
	return nil
}

func (s rollbackRestoreStep) Rollback() error {
	defer s.state.cleanupRollbackSnapshot()
	manifest := s.state.manifest
	var telemetryErr error
	roots := rollbackRoots(s.homeDir, s.workspaceDir)
	if s.telemetryConfigDir != "" {
		roots = append(roots, s.telemetryConfigDir)
		// The retained journals, never the generic backup, own this pair's rollback.
		// Exclude even when its apply step failed or never ran: restoring an absent
		// snapshot entry would otherwise delete a concurrent user's new file.
		protected := telemetryruntime.ManagedPaths(s.telemetryConfigDir)
		manifest.Entries = nil
		for _, entry := range s.state.manifest.Entries {
			if filepath.Clean(entry.OriginalPath) != protected[0] && filepath.Clean(entry.OriginalPath) != protected[1] {
				manifest.Entries = append(manifest.Entries, entry)
			}
		}
		if s.state.telemetryRollback != nil {
			telemetryErr = s.state.telemetryRollback()
		}
	}
	if len(manifest.Entries) == 0 {
		return telemetryErr
	}
	return errors.Join(telemetryErr, (backup.RestoreService{Roots: roots}).Restore(manifest))
}

// rollbackRoots returns the directories this install/sync run could
// legitimately have written under, for validating rollback's manifest
// entries against a caller-known root instead of anything the manifest
// itself declares.
//
// homeDir is always included. workspaceDir is included too when set and
// distinct from homeDir: componentInjectionDirScoped resolves most
// component targets there under ScopeWorkspace, while project tools retain
// their cwd independently of agent artifact scope.
// Both values are exactly what backupTargets/syncBackupTargets used to
// compute what this run actually snapshotted, so allowing rollback to
// write within them is not wider than what this run could already do.
func rollbackRoots(homeDir, workspaceDir string) []string {
	roots := []string{homeDir}
	if workspaceDir != "" && workspaceDir != homeDir {
		roots = append(roots, workspaceDir)
	}
	return roots
}

type agentInstallStep struct {
	id       string
	agent    model.AgentID
	homeDir  string
	profile  system.PlatformProfile
	progress pipeline.ProgressFunc
}

func openCodeTelemetryConfigDir(home, workspace string, scope InstallScope, agentIDs []model.AgentID) string {
	if !containsAgent(agentIDs, model.AgentOpenCode) {
		return ""
	}
	adapter := opencodeagent.NewAdapter()
	return adapter.GlobalConfigDir(componentInjectionDirScoped(home, workspace, scope, adapter))
}

type openCodeTelemetryStep struct {
	state        *runtimeState
	id           string
	configDir    string
	changedFiles *[]string
	checkOnly    bool
}

// openCodeSettingsValidationStep validates the effective OpenCode settings
// before install/sync mutations. Non-OpenCode plans never include this step.
type openCodeSettingsValidationStep struct {
	id           string
	settingsPath string
	touchedKeys  []string
}

func (s openCodeSettingsValidationStep) ID() string { return s.id }

func (s openCodeSettingsValidationStep) Run() error {
	return opencodeactivation.ValidateSettingsForWriters(s.settingsPath, s.touchedKeys)
}

// openCodeSettingsWriterKeys lists the top-level OpenCode settings keys the
// planned JSONC-preserving writers touch: routing guidance and Persona write
// agent, Engram and Context7 write mcp, and the Permission and Theme
// components write permission and theme. The preflight refuses unsafe JSONC
// only inside those values, so a key no selected writer touches never blocks
// the run.
func openCodeSettingsWriterKeys(components []model.ComponentID, routing bool) []string {
	var keys []string
	if routing || slices.Contains(components, model.ComponentPersona) {
		keys = append(keys, "agent")
	}
	if slices.Contains(components, model.ComponentEngram) || slices.Contains(components, model.ComponentContext7) {
		keys = append(keys, "mcp")
	}
	if slices.Contains(components, model.ComponentPermission) {
		keys = append(keys, "permission")
	}
	if slices.Contains(components, model.ComponentTheme) {
		keys = append(keys, "theme")
	}
	return keys
}

func (s openCodeTelemetryStep) ID() string { return s.id }
func (s openCodeTelemetryStep) Run() error {
	if s.checkOnly {
		return telemetryruntime.CheckManaged(s.configDir)
	}
	major, err := opencodeactivation.DetectRuntimeMajor(context.Background())
	if err != nil {
		return err
	}
	changed, rollback, err := telemetryruntime.ReconcileForMajorWithRollback(s.configDir, major)
	if s.state != nil {
		s.state.telemetryRollback = rollback
	}
	if s.changedFiles != nil {
		*s.changedFiles = append(*s.changedFiles, changed...)
	}
	return err
}

func (s agentInstallStep) ID() string {
	return s.id
}

// Run executes Pi's package installation commands only. Other selected
// agents remain config targets regardless of whether their runtime is present.
//
// The `pi` binary itself is never installed by gentle-ai
// (validatePiInstallPreflight refuses if it is not already on PATH), but once
// it is present, its own `pi install ...` subcommands install gentle-ai's Pi
// package stack through that already-present tool.
func (s agentInstallStep) Run() error {
	if s.agent != model.AgentPi {
		return nil
	}

	adapter, err := agents.NewAdapter(s.agent)
	if err != nil {
		return fmt.Errorf("create adapter for %q: %w", s.agent, err)
	}

	if _, _, _, _, err := adapter.Detect(context.Background(), s.homeDir); err != nil {
		return fmt.Errorf("detect agent %q: %w", s.agent, err)
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

	return runCommandSequenceWithProgress(commands, s.progress, s.id)
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
	scope        InstallScope
	agents       []model.AgentID
	selection    model.Selection
	profile      system.PlatformProfile
	channel      InstallChannel
	state        *runtimeState

	backgroundPolicy bool
}

type communityToolInstallStep struct {
	id           string
	tool         model.CommunityToolID
	workspaceDir string
	homeDir      string
	agents       []model.AgentID
	state        *runtimeState
}

func (s communityToolInstallStep) ID() string { return s.id }

func (s communityToolInstallStep) Run() error {
	result, err := installCommunityToolWithHome(s.tool, s.workspaceDir, s.homeDir, communitytool.RunnerFunc(runCommand), communitytool.DetectorFunc(cmdLookPath))
	if err != nil {
		return fmt.Errorf("install community tool %q: %w", s.tool, err)
	}
	if result.PiCodeGraph != nil && s.state != nil {
		s.state.piCodeGraph = result.PiCodeGraph
	}
	return nil
}

func (s componentApplyStep) ID() string {
	return s.id
}

// computeSlugSlimVerdicts implements the Per-slug forwarding semantics
// (design.md Decision 4): a slug only forwards --protocol=slim when every
// adapter sharing it independently verifies slim (safest-wins AND
// semantics). isSlim is injected so tests can pin the AND logic with a
// synthetic slim+full pair sharing a slug (JD-017), independent of the real
// IsVerifiedSlimAdapter matrix (which today only ever verifies Claude Code).
func computeSlugSlimVerdicts(agentIDs []model.AgentID, isSlim func(model.AgentID) bool) map[string]bool {
	verdicts := make(map[string]bool, len(agentIDs))
	seen := make(map[string]bool, len(agentIDs))
	for _, agent := range agentIDs {
		slug, ok := engram.SetupAgentSlug(agent)
		if !ok {
			continue
		}
		verdict := isSlim(agent)
		if !seen[slug] {
			verdicts[slug] = verdict
			seen[slug] = true
		} else {
			verdicts[slug] = verdicts[slug] && verdict
		}
	}
	return verdicts
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

func shouldRefreshWindowsEngram(profile system.PlatformProfile, resolvedPath string, pathEntries []string) bool {
	if profile.OS != "windows" || profile.PackageManager == "brew" || strings.TrimSpace(resolvedPath) == "" {
		return false
	}
	return len(engramBinaryDirsOnPath(pathEntries, profile.OS)) > 1
}

func ensureRepairableWindowsEngramShadowing(profile system.PlatformProfile, installedPath, managedDir string) error {
	userEntries, err := userPathEntries(profile.OS)
	if err != nil {
		return fmt.Errorf("read user PATH: %w", err)
	}

	staleDir := filepath.Dir(installedPath)
	if !pathEntriesContainDir(userEntries, staleDir) {
		return fmt.Errorf("%s is not in the user PATH, so user-scoped PATH repair cannot guarantee future shells will resolve %s before %s", staleDir, managedDir, staleDir)
	}

	return nil
}

func pathEntriesContainDir(entries []string, dir string) bool {
	dir = strings.Trim(strings.TrimSpace(dir), `"`)
	if dir == "" {
		return false
	}
	for _, entry := range entries {
		entry = strings.Trim(strings.TrimSpace(entry), `"`)
		if entry == "" {
			continue
		}
		if strings.EqualFold(filepath.Clean(entry), filepath.Clean(dir)) {
			return true
		}
	}
	return false
}

func engramBinaryDirsOnPath(pathEntries []string, goos string) []string {
	var dirs []string
	for _, entry := range pathEntries {
		entry = strings.Trim(strings.TrimSpace(entry), `"`)
		if entry == "" {
			continue
		}
		binaryName := "engram"
		if goos == "windows" {
			binaryName = "engram.exe"
		}
		candidate := filepath.Join(entry, binaryName)
		if _, err := os.Stat(candidate); err == nil {
			dirs = append(dirs, entry)
		}
	}
	return dirs
}

func resolveEngramVersion(command string) (string, error) {
	if strings.TrimSpace(command) == "" || command == "engram" {
		return verifyEngramVersion()
	}
	return verifyEngramVersionCommand(command)
}

func resolveEngramProtocolFlag(ctx context.Context, command string) (string, error) {
	if strings.TrimSpace(command) == "" || command == "engram" {
		return probeEngramProtocolFlag(ctx)
	}
	return probeEngramProtocolFlagCommand(ctx, command)
}

func splitPathForOS(value, goos string) []string {
	separator := string(os.PathListSeparator)
	if goos == "windows" {
		separator = ";"
	}
	if value == "" {
		return nil
	}
	return strings.Split(value, separator)
}

func (s componentApplyStep) Run() error {
	adapters := resolveAdapters(s.agents)

	switch s.component {
	case model.ComponentEngram:
		engramCommand := "engram"
		var installErr error
		if betaEngramRequiresGo(s.channel, s.component) {
			binaryPath, err := installBetaEngramFromMain()
			if err != nil {
				return fmt.Errorf("install beta engram from main: %w", err)
			}
			engramCommand = binaryPath
		} else if installedPath, found := resolveEngramInstalledPath(s.profile); !found {
			// Engram not on PATH — install it.
			if s.profile.PackageManager == "brew" {
				// macOS (or Linux with Homebrew): use brew tap + brew install.
				commands, err := engram.InstallCommand(s.profile)
				if err != nil {
					return fmt.Errorf("resolve install command for component %q: %w", s.component, err)
				}
				commands = withResolvedBrewCommand(commands)
				installErr = runCommandSequence(commands)
				if installErr == nil {
					if installedPath, found := resolveEngramInstalledPath(s.profile); found {
						engramCommand = installedPath
					}
				}
			} else if binaryPath, err := engramDownloadFn(s.profile); err != nil {
				// Linux / Windows: download the pre-built binary from GitHub Releases.
				// No Go required — engram ships pre-built binaries.
				installErr = fmt.Errorf("download engram binary: %w", err)
			} else {
				// Add the install directory to PATH so subsequent commands
				// (engram setup, engram.Inject → resolveEngramCommand) can find it.
				// On Windows this also persists the change to the user registry via PowerShell.
				binDir := filepath.Dir(binaryPath)
				if err := addUserPath(binDir); err != nil {
					// Non-fatal: warn but continue — the binary was downloaded successfully.
					fmt.Fprintf(os.Stderr, "WARNING: could not add %s to PATH: %v\n", binDir, err)
				}
				engramCommand = binaryPath
			}
			if installErr != nil {
				if s.selection.HasComponent(model.ComponentEngram) {
					return installErr
				}
				// engram was auto-added as an sdd dependency, not requested. Its
				// failure must not abort the components the user asked for, and
				// no configuration may point at a binary that does not exist:
				// report the missing binary as a warning with its own install
				// command and leave the engram step entirely (#3725).
				fmt.Fprintf(os.Stderr, "WARNING: engram could not be installed: %v\nInstall it with `%s`.\n", installErr, engramInstallCommand(s.agents))
				if s.state != nil {
					s.state.engramVersionResolved = true
					s.state.engramVersionErr = installErr
				}
				return nil
			}
		} else if shouldRefreshWindowsEngram(s.profile, installedPath, pathEnvEntries(s.profile)) {
			binaryPath, err := engramDownloadFn(s.profile)
			if err != nil {
				return fmt.Errorf("refresh shadowed engram binary: %w", err)
			}
			engramCommand = binaryPath
			binDir := filepath.Dir(binaryPath)
			if err := ensureRepairableWindowsEngramShadowing(s.profile, installedPath, binDir); err != nil {
				return fmt.Errorf("repair Windows Engram PATH shadowing: refreshed managed Engram at %s, but cannot safely repair PATH order: %w. Move %s before %s in your user PATH or remove the stale Machine/System PATH entry, then rerun install", binaryPath, err, binDir, filepath.Dir(installedPath))
			}
			if err := ensureUserPathFirst(binDir); err != nil {
				return fmt.Errorf("repair Windows Engram PATH shadowing: refreshed managed Engram at %s, but could not move %s ahead of stale PATH entry %s: %w. Move %s before %s in your user PATH, then rerun install", binaryPath, binDir, installedPath, err, binDir, filepath.Dir(installedPath))
			}
			fmt.Fprintf(os.Stderr, "WARNING: multiple engram.exe entries were found on PATH and %s resolved first. Refreshed managed Engram at %s and moved %s ahead of the stale entry in the user PATH.\n", installedPath, binaryPath, binDir)
		} else {
			engramCommand = installedPath
		}
		// Refuse unsafe selected OpenCode settings before `engram setup`, whose
		// external writes the later merge refusal could not undo.
		for _, adapter := range adapters {
			if adapter.Agent() != model.AgentOpenCode {
				continue
			}
			if err := engram.ValidateOpenCodeSettings(openCodeLoadedSettingsPath(s.homeDir, s.workspaceDir, adapter)); err != nil {
				return fmt.Errorf("inject engram for %q: %w", adapter.Agent(), err)
			}
		}
		setupMode := engram.ParseSetupMode(os.Getenv(engram.SetupModeEnvVar))
		setupStrict := engram.ParseSetupStrict(os.Getenv(engram.SetupStrictEnvVar))

		// Resolve the installed engram version once (Decision 1 gate). Errors are
		// intentionally ignored for gating purposes: an empty version string
		// safely falls back to the full protocol section and the full setup
		// verdict for every adapter. The result (including the error) is cached
		// on s.state so the post-apply health check reuses it instead of
		// shelling out to `engram version` a second time (JD-016).
		engramVersion, versionErr := resolveEngramVersion(engramCommand)
		if s.state != nil {
			s.state.engramVersionResolved = true
			s.state.engramVersion = engramVersion
			s.state.engramVersionErr = versionErr
		}

		// Probe --protocol support once before the adapter loop (Decision 4),
		// but only when at least one selected adapter will actually attempt
		// `engram setup` under setupMode (JD-013): under
		// GENTLE_AI_ENGRAM_SETUP_MODE=off, ShouldAttemptSetup is false for
		// every adapter, no setup invocation ever happens, and the probe's
		// result would never be used — so skip the (up to 5s) probe
		// entirely rather than run it unconditionally.
		willAttemptSetup := false
		for _, adapter := range adapters {
			if installErr == nil && engram.ShouldAttemptSetup(setupMode, adapter.Agent()) {
				willAttemptSetup = true
				break
			}
		}
		protocolFlagSupported := false
		if willAttemptSetup {
			if stdout, err := resolveEngramProtocolFlag(context.Background(), engramCommand); err == nil {
				protocolFlagSupported = strings.Contains(stdout, "--protocol")
			}
		}

		// Compute the safest-wins verdict per setup slug (Per-slug
		// forwarding semantics, design.md): a slug only forwards
		// --protocol=slim when every adapter sharing it independently
		// verifies slim. Extracted into computeSlugSlimVerdicts (JD-017) so
		// the AND semantics can be pinned with a synthetic divergent-slug
		// case independent of the RunInstall integration path.
		agentIDs := make([]model.AgentID, 0, len(adapters))
		for _, adapter := range adapters {
			agentIDs = append(agentIDs, adapter.Agent())
		}
		slugSlimVerdict := computeSlugSlimVerdicts(agentIDs, func(agent model.AgentID) bool {
			return engram.IsVerifiedSlimAdapter(agent, engramVersion)
		})

		attemptedSlugs := make(map[string]struct{}, len(adapters))
		for _, adapter := range adapters {
			if willAttemptSetup && engram.ShouldAttemptSetup(setupMode, adapter.Agent()) {
				slug, _ := engram.SetupAgentSlug(adapter.Agent())
				if _, seen := attemptedSlugs[slug]; !seen {
					setupArgs := []string{"setup", slug}
					if protocolFlagSupported {
						mode := "full"
						if slugSlimVerdict[slug] {
							mode = "slim"
						}
						setupArgs = append(setupArgs, "--protocol="+mode)
					}
					if err := runCommand(engramCommand, setupArgs...); err != nil {
						if setupStrict {
							return fmt.Errorf("engram setup for %q: %w", adapter.Agent(), err)
						}
					}
					attemptedSlugs[slug] = struct{}{}
				}
			}
			engramOpts := engram.InjectOptions{
				OpenCodeSettingsPath:        openCodeLoadedSettingsPath(s.homeDir, s.workspaceDir, adapter),
				CodexOrchestratorAssignment: s.selection.CodexOrchestratorAssignment,
				CodexCarrilModelAssignments: s.selection.CodexCarrilModelAssignments,
				CodexModelAssignments:       s.selection.CodexModelAssignments,
				Version:                     engramVersion,
			}
			var err error
			if adapter.Agent() == model.AgentOpenClaw {
				_, err = engram.InjectWithPromptDir(s.homeDir, componentInjectionDirScoped(s.homeDir, s.workspaceDir, s.scope, adapter), adapter)
			} else {
				targetDir := componentInjectionDirScoped(s.homeDir, s.workspaceDir, s.scope, adapter)
				if s.scope == ScopeWorkspace {
					_, err = engram.InjectWorkspaceWithOptions(targetDir, adapter, engramOpts)
				} else {
					_, err = engram.InjectWithOptions(targetDir, adapter, engramOpts)
				}
			}
			if err != nil {
				return fmt.Errorf("inject engram for %q: %w", adapter.Agent(), err)
			}
		}
		return nil
	case model.ComponentContext7:
		for _, adapter := range adapters {
			targetDir := componentInjectionDirScoped(s.homeDir, s.workspaceDir, s.scope, adapter)
			var err error
			if adapter.Agent() == model.AgentOpenCode {
				_, err = mcp.InjectAtSettingsPath(s.homeDir, targetDir, adapter, openCodeLoadedSettingsPath(s.homeDir, s.workspaceDir, adapter))
			} else {
				_, err = mcp.Inject(s.homeDir, targetDir, adapter)
			}
			if err != nil {
				return fmt.Errorf("inject context7 for %q: %w", adapter.Agent(), err)
			}
		}
		return nil
	case model.ComponentPersona:
		for _, adapter := range adapters {
			if adapter.Agent() == model.AgentPi {
				for _, rootDir := range piPersonaConfigRoots(s.homeDir, s.workspaceDir, s.scope) {
					if _, err := persona.InjectPiPersona(rootDir, s.selection.Persona); err != nil {
						return fmt.Errorf("inject persona for %q: %w", adapter.Agent(), err)
					}
				}
				continue
			}
			targetDir := componentInjectionDirScoped(s.homeDir, s.workspaceDir, s.scope, adapter)
			selectedSettingsPath := ""
			if adapter.Agent() == model.AgentOpenCode {
				selectedSettingsPath = openCodeLoadedSettingsPath(s.homeDir, s.workspaceDir, adapter)
			}
			if _, err := injectInstallPersona(targetDir, adapter, s.selection.Persona, selectedSettingsPath); err != nil {
				return fmt.Errorf("inject persona for %q: %w", adapter.Agent(), err)
			}
		}
		return nil
	case model.ComponentPermission:
		for _, adapter := range adapters {
			var err error
			if adapter.Agent() == model.AgentOpenCode {
				_, err = permissions.InjectAtPath(openCodeLoadedSettingsPath(s.homeDir, s.workspaceDir, adapter), adapter)
			} else {
				_, err = permissions.Inject(s.homeDir, adapter)
			}
			if err != nil {
				return fmt.Errorf("inject permissions for %q: %w", adapter.Agent(), err)
			}
		}
		return nil
	case model.ComponentSDD:
		// Legacy persisted selections are readable, but their retired assets are not reinstalled.
		return nil
	case model.ComponentSkills:
		skillIDs := selectedSkillIDs(s.selection)
		if len(skillIDs) == 0 {
			return nil
		}
		for _, adapter := range adapters {
			targetDir := componentInjectionDirScoped(s.homeDir, s.workspaceDir, s.scope, adapter)
			if _, err := skills.Inject(targetDir, adapter, skillIDs); err != nil {
				return fmt.Errorf("inject skills for %q: %w", adapter.Agent(), err)
			}
		}
		return nil
	case model.ComponentGGA:
		if !ggaAvailable(s.profile) {
			// GGA not found on any known PATH — install it.
			if s.profile.OS == "windows" {
				if err := cleanupGGAInstallDir(); err != nil {
					return err
				}
			}
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
			if err := gga.EnsureCommandShim(s.homeDir); err != nil {
				return fmt.Errorf("ensure gga command shim: %w", err)
			}
			// Add GGA bin dir to the user PATH persistently on Windows.
			// GGA's install.sh drops the binary into ~/bin which is not on PATH by default.
			ggaBinDir := filepath.Join(s.homeDir, "bin")
			if err := addUserPath(ggaBinDir); err != nil {
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
			var err error
			if adapter.Agent() == model.AgentOpenCode {
				_, err = theme.InjectAtPath(openCodeLoadedSettingsPath(s.homeDir, s.workspaceDir, adapter))
			} else {
				_, err = theme.Inject(s.homeDir, adapter)
			}
			if err != nil {
				return fmt.Errorf("inject theme for %q: %w", adapter.Agent(), err)
			}
		}
		return nil
	case model.ComponentClaudeTheme:
		for _, adapter := range adapters {
			if _, err := theme.InjectVisualThemes(s.homeDir, adapter); err != nil {
				return fmt.Errorf("inject visual themes for %q: %w", adapter.Agent(), err)
			}
		}
		return nil
	case model.ComponentOpenCodeGentleLogo:
		if !containsAgent(s.agents, model.AgentOpenCode) {
			return nil
		}
		if _, err := opencodeplugin.Install(s.homeDir, model.OpenCodePluginGentleLogo); err != nil {
			return fmt.Errorf("install OpenCode Gentle Logo plugin: %w", err)
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

// The seam lets native tests add a post-publication failure while still using
// the public TUI execution boundary and the real compatibility writer.
var tuiInstallStagePlan = func(runtime *installRuntime) pipeline.StagePlan {
	return runtime.stagePlan()
}

// ExecuteTUIInstallWithBackgroundAndOrchestrator runs a TUI install and returns
// the orchestrator so downstream persistence failures can be compensated.
// After successful persistence, callers must call orchestrator.Finish() to
// release a deduplicated temporary rollback snapshot.
func ExecuteTUIInstallWithBackgroundAndOrchestrator(homeDir string, selection model.Selection, resolved planner.ResolvedPlan, profile system.PlatformProfile, background model.OpenCodeBackgroundIntent, piBackground model.PiBackgroundIntent, onProgress pipeline.ProgressFunc, consent ...*OpenCodeSDKConsent) (pipeline.ExecutionResult, *pipeline.Orchestrator) {
	return executeTUIInstallWithBackground(homeDir, selection, resolved, profile, background, piBackground, onProgress, consent...)
}

func executeTUIInstallWithBackground(homeDir string, selection model.Selection, resolved planner.ResolvedPlan, profile system.PlatformProfile, background model.OpenCodeBackgroundIntent, piBackground model.PiBackgroundIntent, onProgress pipeline.ProgressFunc, consent ...*OpenCodeSDKConsent) (pipeline.ExecutionResult, *pipeline.Orchestrator) {
	runtime, err := newInstallRuntime(homeDir, ScopeGlobal, ChannelStable, selection, resolved, profile)
	if err != nil {
		return pipeline.ExecutionResult{Err: err}, nil
	}
	defer runtime.state.cleanupCompatibilityTransaction()
	backgroundResolution := OpenCodeBackgroundResolution{
		Intent:    background,
		Effective: background,
	}
	backgroundActivation, err := prepareOpenCodeBackgroundActivation(homeDir, &backgroundResolution, containsAgent(resolved.Agents, model.AgentOpenCode))
	if err != nil {
		return pipeline.ExecutionResult{Err: fmt.Errorf("prepare OpenCode background activation: %w", err)}, nil
	}
	runtime.background = backgroundResolution
	runtime.progress = onProgress
	if len(consent) > 0 && consent[0] != nil {
		if !containsAgent(resolved.Agents, model.AgentOpenCode) {
			// refusal:by-design human-authority: invocation consent cannot apply to a different selection
			return pipeline.ExecutionResult{Err: fmt.Errorf("OpenCode SDK consent does not match selected agents")}, nil
		}
		runtime.sdkConsent = consent[0]
	}
	runtime.backgroundActivation = backgroundActivation
	runtime.runtimeReady = backgroundActivation != nil && backgroundActivation.Capability().Ready()
	piBackgroundResolution := PiBackgroundResolution{
		Intent:    piBackground,
		Effective: piBackground,
		managed:   piBackground == model.PiBackgroundOn || piBackground == model.PiBackgroundOff,
	}
	runtime.piBackgroundProjection = preparePiBackgroundProjection(homeDir, &piBackgroundResolution, containsAgent(resolved.Agents, model.AgentPi))
	orchestrator := pipeline.NewOrchestrator(pipeline.DefaultRollbackPolicy(), pipeline.WithFailurePolicy(pipeline.ContinueOnError), pipeline.WithProgressFunc(onProgress), pipeline.WithTerminalCleanup(runtime.state.cleanupRollbackSnapshot))
	result := orchestrator.Execute(tuiInstallStagePlan(runtime))
	if runtime.state.piCodeGraph != nil {
		result.ManualActions = append(result.ManualActions, runtime.state.piCodeGraph.ManualActions...)
	}
	result.ManualActions = append(result.ManualActions, runtime.state.nativeReviewActions...)
	return result, orchestrator
}

// RenderInstallManualActions renders non-fatal completion actions after the
// normal verification report so CLI users receive the same drift guidance.
func RenderInstallManualActions(result InstallResult) string {
	actions := append([]string(nil), result.ManualActions...)
	if result.PiCodeGraph != nil {
		actions = append(actions, result.PiCodeGraph.ManualActions...)
	}
	if len(actions) == 0 {
		return ""
	}
	return "\nManual actions required:\n- " + strings.Join(actions, "\n- ") + "\n"
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

func isExecutableFile(path string) bool {
	info, err := osStat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	// Windows has no POSIX executable permission bit (os.FileMode.Perm()
	// never carries 0o111 there), so a regular file at a known binary path
	// is treated as usable without a permission check.
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode().Perm()&0o111 != 0
}

func standardHomebrewExecutable(name string) (string, bool) {
	for _, binDir := range []string{
		"/opt/homebrew/bin",
		"/usr/local/bin",
		"/home/linuxbrew/.linuxbrew/bin",
	} {
		path := filepath.Join(binDir, name)
		if isExecutableFile(path) {
			return path, true
		}
	}
	return "", false
}

// resolveEngramInstalledPath finds an existing Engram even when the installer's
// inherited PATH omits a standard Homebrew prefix (#4020).
func resolveEngramInstalledPath(profile system.PlatformProfile) (string, bool) {
	if path, err := cmdLookPath("engram"); err == nil {
		return path, true
	}
	if profile.OS == "darwin" || profile.PackageManager == "brew" {
		return standardHomebrewExecutable("engram")
	}
	return "", false
}

// resolveBrewCommand avoids environment-sensitive shell profile probing. The
// inherited PATH and Homebrew's documented standard prefixes are sufficient.
func resolveBrewCommand() string {
	if path, err := cmdLookPath("brew"); err == nil {
		return path
	}
	if path, found := standardHomebrewExecutable("brew"); found {
		return path
	}
	return "brew"
}

func withResolvedBrewCommand(commands [][]string) [][]string {
	brewPath := ""
	rewritten := make([][]string, len(commands))
	for i, command := range commands {
		if len(command) > 0 && command[0] == "brew" {
			if brewPath == "" {
				brewPath = resolveBrewCommand()
			}
			rewritten[i] = append([]string{brewPath}, command[1:]...)
			continue
		}
		rewritten[i] = command
	}
	return rewritten
}

// runCommandSequence runs each command in the sequence one at a time, stopping on first error.
func runCommandSequence(commands [][]string) error {
	return runCommandSequenceWithProgress(commands, nil, "")
}

// runCommandSequenceWithProgress is the common command runner used by agent
// installation steps. The optional callback reports each command as a generic
// pipeline progress event; the command producer remains responsible for the
// command content, so the pipeline does not need to know any agent-specific
// package names.
func runCommandSequenceWithProgress(commands [][]string, progress pipeline.ProgressFunc, stepPrefix string) error {
	if len(commands) == 0 {
		return fmt.Errorf("empty command sequence")
	}

	for _, command := range commands {
		if len(command) == 0 {
			return fmt.Errorf("empty command in sequence")
		}

		stepID := ""
		if stepPrefix != "" {
			stepID = stepPrefix + ":" + strings.Join(command, " ")
		}
		if progress != nil {
			progress(pipeline.ProgressEvent{StepID: stepID, Status: pipeline.StepStatusRunning})
		}

		err := runCommand(command[0], command[1:]...)
		if err != nil {
			if progress != nil {
				progress(pipeline.ProgressEvent{StepID: stepID, Status: pipeline.StepStatusFailed, Err: err})
			}
			return fmt.Errorf("run command %q: %w", strings.Join(command, " "), err)
		}
		if progress != nil {
			progress(pipeline.ProgressEvent{StepID: stepID, Status: pipeline.StepStatusSucceeded})
		}
	}

	return nil
}

// homebrewNoSideEffectEnv are the environment variables executeCommand adds
// to a brew invocation so gentle-ai's own tap/install/reinstall steps never
// trigger Homebrew's slow, network-dependent auto-update or its
// post-install cache cleanup as a side effect of an unrelated install.
var homebrewNoSideEffectEnv = []string{
	"HOMEBREW_NO_AUTO_UPDATE=1",
	"HOMEBREW_NO_INSTALL_CLEANUP=1",
}

// commandEnv returns the environment executeCommand should use for name,
// derived from base (typically os.Environ()). When name's base is "brew"
// (matching both the literal command and resolveBrewCommand's resolved
// absolute path) and the invocation is not the user-requested "brew
// update"/"brew upgrade" path (see internal/update/upgrade's own
// brewUpgrade), it adds homebrewNoSideEffectEnv, never overriding a value
// already present in base — an explicit user override always wins.
func commandEnv(name string, args []string, base []string) []string {
	if filepath.Base(name) != "brew" {
		return base
	}
	if len(args) > 0 && (args[0] == "update" || args[0] == "upgrade") {
		return base
	}

	// Copy so appending never writes into the caller's backing array.
	env := append([]string(nil), base...)
	for _, kv := range homebrewNoSideEffectEnv {
		key := strings.SplitN(kv, "=", 2)[0]
		if !envHasKey(env, key) {
			env = append(env, kv)
		}
	}
	return env
}

// envHasKey reports whether env already sets key, regardless of value.
func envHasKey(env []string, key string) bool {
	prefix := key + "="
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			return true
		}
	}
	return false
}

func executeCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Env = commandEnv(name, args, os.Environ())
	system.EnsureCommandDir(cmd)

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

func backupTargets(homeDir, workspaceDir string, scope InstallScope, selection model.Selection, resolved planner.ResolvedPlan) ([]string, error) {
	paths := map[string]struct{}{}
	adapters := resolveAdapters(resolved.Agents)
	if configDir := openCodeTelemetryConfigDir(homeDir, workspaceDir, scope, resolved.Agents); configDir != "" {
		for _, path := range telemetryruntime.ManagedPaths(configDir) {
			paths[path] = struct{}{}
		}
	}

	for _, component := range resolved.OrderedComponents {
		if component == model.ComponentSDD {
			continue
		}
		for _, path := range componentPathsWithWorkspaceScoped(homeDir, workspaceDir, scope, selection, adapters, component) {
			paths[path] = struct{}{}
		}
		if component == model.ComponentContext7 {
			for _, path := range claudeMCPSettingsCleanupPaths(homeDir, workspaceDir, scope, adapters) {
				paths[path] = struct{}{}
			}
		}
		if component == model.ComponentEngram && scope == ScopeGlobal {
			for _, adapter := range adapters {
				if adapter.Agent() == model.AgentClaudeCode {
					paths[adapter.MCPConfigPath(homeDir, "engram")] = struct{}{}
				}
			}
		}
		// Persona backup captures every managed output-style file, not just the
		// selected one, so a failed persona switch can restore the file its
		// cleanup removed. Backup-only: verification stays on the selected file.
		if component == model.ComponentPersona {
			plan := persona.ResourcePlanFor(selection.Persona)
			for _, adapter := range adapters {
				if adapter.Agent() == model.AgentOpenCode || adapter.Agent() == model.AgentKilocode {
					// Persona can merge or clean a managed agent in settings during
					// install. This is backup-only: ComponentPersona verification
					// does not promise a settings write for every persona path.
					path := adapter.SettingsPath(componentPathDirScoped(homeDir, workspaceDir, scope, adapter, model.ComponentPersona))
					if adapter.Agent() == model.AgentOpenCode {
						path = openCodeLoadedSettingsPath(homeDir, workspaceDir, adapter)
					}
					if path != "" {
						paths[path] = struct{}{}
					}
				}
				if !adapter.SupportsOutputStyles() {
					continue
				}
				for _, path := range plan.OutputStylePaths(adapter.OutputStyleDir(componentPathDirScoped(homeDir, workspaceDir, scope, adapter, model.ComponentPersona))).Backup {
					paths[path] = struct{}{}
				}
			}
		}
	}
	// Routing guidance is delivered per agent outside the component loop, so a
	// selection whose components do not happen to cover the same file would be
	// rewritten without ever having been snapshotted (issue #1794).
	for _, path := range routingGuidancePaths(homeDir, workspaceDir, scope, adapters) {
		paths[path] = struct{}{}
	}
	for _, adapter := range adapters {
		if path := agentguidance.StrictTDDPath(routingGuidanceDir(homeDir, workspaceDir, scope, adapter), adapter.Agent()); path != "" {
			paths[path] = struct{}{}
		}
	}
	for _, adapter := range adapters {
		if adapter.Agent() == model.AgentCodex {
			paths[filepath.Join(adapter.GlobalConfigDir(homeDir), "hooks.json")] = struct{}{}
		}
		// Native review and Judgment Day agents are installed independently of SDD.
		// Retired review agents are listed too: the installer may remove them.
		if names := reviewassets.NativeAgentFileNames(adapter.Agent()); len(names) > 0 {
			dir := adapter.SubAgentsDir(componentInjectionDirScoped(homeDir, workspaceDir, scope, adapter))
			paths[filepath.Join(dir, reviewassets.OwnershipLedgerFilename)] = struct{}{}
			for _, name := range names {
				paths[filepath.Join(dir, name)] = struct{}{}
			}
		}
		if adapter.Agent() == model.AgentOpenCode {
			pluginsDir := filepath.Join(adapter.GlobalConfigDir(homeDir), "plugins")
			for _, name := range opencoderuntimeplugins.ManagedOpenCodePluginNames() {
				paths[filepath.Join(pluginsDir, name)] = struct{}{}
			}
		}
	}
	adapterSkillPaths, err := adapterSkillBackupTargets(homeDir, workspaceDir, scope, selection, adapters)
	if err != nil {
		return nil, err
	}
	for _, path := range adapterSkillPaths {
		paths[path] = struct{}{}
	}
	if !usesAnchoredCompatibilityTransaction() && needsCompatibilitySkillsRefresh(resolved.OrderedComponents) {
		skillDir, ok, err := compatibilitySkillsDir(homeDir)
		if err != nil {
			return nil, err
		}
		if ok {
			compatibilityPaths, err := compatibilitySkillFiles(skillDir, resolved.OrderedComponents, selection)
			if err != nil {
				return nil, err
			}
			for _, path := range compatibilityPaths {
				paths[path] = struct{}{}
			}
		}
	}
	if containsAgent(resolved.Agents, model.AgentPi) {
		for _, adapter := range adapters {
			if adapter.Agent() == model.AgentPi {
				paths[adapter.SystemPromptFile(homeDir)] = struct{}{}
			}
		}
		for _, path := range communitytool.PiCodeGraphPaths(homeDir, workspaceDir) {
			paths[path] = struct{}{}
		}
	}
	if selection.HasCommunityTool(model.CommunityToolCodeGraph) {
		for _, path := range communitytool.CodeGraphManagedPaths(homeDir) {
			paths[path] = struct{}{}
		}
	}
	if containsAgent(resolved.Agents, model.AgentOpenCode) {
		for _, path := range opencodeactivation.LauncherPaths(homeDir, runtime.GOOS) {
			paths[path] = struct{}{}
		}
	}

	targets := make([]string, 0, len(paths))
	for path := range paths {
		targets = append(targets, path)
	}
	sort.Strings(targets)
	return targets, nil
}

func adapterSkillBackupTargets(homeDir, workspaceDir string, scope InstallScope, selection model.Selection, adapters []agents.Adapter) ([]string, error) {
	var paths []string
	for _, adapter := range adapters {
		if !adapter.SupportsSkills() {
			continue
		}
		skillDir := adapter.SkillsDir(componentInjectionDirScoped(homeDir, workspaceDir, scope, adapter))
		if skillDir == "" {
			continue
		}
		if slices.Contains(selection.Components, model.ComponentSkills) {
			ordinary, err := skills.DirectoryPaths(skillDir, selectedSkillIDs(selection), "")
			if err != nil {
				return nil, fmt.Errorf("enumerate %s skill backup targets: %w", adapter.Agent(), err)
			}
			paths = append(paths, ordinary...)
			support, err := skillSupportBackupTargets(componentInjectionDirScoped(homeDir, workspaceDir, scope, adapter), adapter, selection)
			if err != nil {
				return nil, err
			}
			paths = append(paths, support...)
		}
	}
	return paths, nil
}

// skillSupportBackupTargets declares the shared references, skill commands,
// and obsolete shared marker the skills component writes or removes.
func skillSupportBackupTargets(targetDir string, adapter agents.Adapter, selection model.Selection) ([]string, error) {
	support, err := skills.SupportFilePaths(targetDir, adapter, selectedSkillIDs(selection))
	if err != nil {
		return nil, fmt.Errorf("enumerate %s skill support backup targets: %w", adapter.Agent(), err)
	}
	if skillDir := adapter.SkillsDir(targetDir); skillDir != "" {
		support = append(support, skills.LegacySharedMarkerPath(skillDir))
	}
	return support, nil
}

// claudeMCPSettingsCleanupPaths returns legacy Claude settings files that MCP
// injection may rewrite while removing an inert mcpServers block. These paths
// belong in the rollback snapshot, but not in post-apply verification because
// cleanup is best-effort and the file may not exist.
func claudeMCPSettingsCleanupPaths(homeDir, workspaceDir string, scope InstallScope, adapters []agents.Adapter) []string {
	paths := []string{}
	for _, adapter := range adapters {
		if adapter.Agent() != model.AgentClaudeCode || adapter.MCPStrategy() != model.StrategySeparateMCPFiles {
			continue
		}
		targetDir := componentPathDirScoped(homeDir, workspaceDir, scope, adapter, model.ComponentContext7)
		if path := adapter.SettingsPath(targetDir); path != "" {
			paths = append(paths, path)
		}
	}
	return paths
}

// routingGuidancePaths declares the files agentRoutingGuidanceStep rewrites for
// the given adapters.
//
// The target directory is resolved exactly the way that step resolves it, and
// the paths themselves come from the injector's own delivery dispatch, so the
// backup contract cannot drift from what is actually written.
func routingGuidancePaths(homeDir, workspaceDir string, scope InstallScope, adapters []agents.Adapter) []string {
	paths := []string{}
	for _, adapter := range adapters {
		// Mirrors the same skip agentRoutingGuidanceStep.Run() applies: an
		// agent without a managed system prompt (Pi) gets no routing guidance
		// write, so it must not be declared as a backup target either.
		if !adapter.SupportsSystemPrompt() {
			continue
		}
		targetDir := routingGuidanceDir(homeDir, workspaceDir, scope, adapter)
		options := routingGuidanceOptions(homeDir, workspaceDir, adapter)
		var routing []string
		var err error
		if options.SettingsPath == "" {
			routing, err = agentguidance.RoutingPaths(targetDir, adapter.Agent())
		} else {
			routing, err = agentguidance.RoutingPathsWithOptions(targetDir, adapter.Agent(), options)
		}
		if err != nil {
			// The guidance step resolves the same delivery and fails loudly when
			// it runs. Declaring a target we could not resolve would only add a
			// path to the snapshot that is never written.
			continue
		}
		paths = append(paths, routing...)
		if adapter.Agent() == model.AgentOpenCode && options.SettingsPath != "" {
			paths = append(paths, opencodedefault.OwnershipPath(options.SettingsPath))
		}
	}
	return paths
}

func componentPathsWithWorkspaceScoped(homeDir, workspaceDir string, scope InstallScope, selection model.Selection, adapters []agents.Adapter, component model.ComponentID) []string {
	paths := []string{}
	for _, adapter := range adapters {
		targetDir := componentPathDirScoped(homeDir, workspaceDir, scope, adapter, component)
		switch component {
		case model.ComponentEngram:
			switch adapter.MCPStrategy() {
			case model.StrategySeparateMCPFiles:
				if adapter.Agent() == model.AgentClaudeCode && scope == ScopeGlobal {
					paths = append(paths, claude.UserConfigPath(homeDir))
				} else {
					paths = append(paths, adapter.MCPConfigPath(targetDir, "engram"))
				}
			case model.StrategyMergeIntoSettings:
				if adapter.Agent() == model.AgentOpenCode {
					paths = append(paths, openCodeLoadedSettingsPath(homeDir, workspaceDir, adapter))
					break
				}
				// MCP settings are always merged into the global config file, not the
				// workspace-scoped directory. For OpenClaw, SettingsPath(targetDir)
				// would yield <workspace>/.openclaw/openclaw.json, but engram injection
				// writes to the canonical ~/.openclaw/openclaw.json (homeDir). Use
				// homeDir here so the verification path matches the actual write target.
				if p := adapter.SettingsPath(homeDir); p != "" {
					paths = append(paths, p)
				}
			case model.StrategyMCPConfigFile:
				if p := adapter.MCPConfigPath(targetDir, "engram"); p != "" {
					paths = append(paths, p)
				}
				if adapter.Agent() == model.AgentAntigravity {
					if p := adapter.SettingsPath(homeDir); p != "" {
						paths = append(paths, p)
					}
				}
			case model.StrategyMergeIntoYAML:
				if p := adapter.MCPConfigPath(targetDir, "engram"); p != "" {
					paths = append(paths, p)
				}
			case model.StrategyTOMLFile:
				if p := adapter.MCPConfigPath(targetDir, "engram"); p != "" {
					paths = append(paths, p)
					// Track the gentle-ai files written alongside the Codex config.toml
					// so they are restored on rollback and removed on uninstall.
					codexHomeDir := filepath.Dir(p)
					paths = append(paths,
						filepath.Join(codexHomeDir, "engram-instructions.md"),
						filepath.Join(codexHomeDir, "engram-compact-prompt.md"),
					)
					paths = append(paths, codexagent.SddProfilePaths(codexHomeDir)...)
				}
			}
			if adapter.SystemPromptStrategy() == model.StrategyMarkdownSections {
				paths = append(paths, adapter.SystemPromptFile(targetDir))
			}
		case model.ComponentSDD:
			// Jinja modular hubs (e.g. Kimi KIMI.md) are appended once below so SDD+Persona
			// do not duplicate the same system prompt path. OpenCode-compatible adapters
			// write the SDD orchestrator to their settings file instead (#3975).
			if adapter.SupportsSystemPrompt() &&
				!opencoderuntimeplugins.AgentReceivesManagedOpenCodePlugins(adapter.Agent()) &&
				adapter.SystemPromptStrategy() != model.StrategyJinjaModules {
				paths = append(paths, adapter.SystemPromptFile(targetDir))
			}
			if adapter.SupportsSlashCommands() {
				paths = append(paths, legacyassets.SlashCommandPaths(adapter.Agent(), adapter.CommandsDir(targetDir))...)
			}
			if adapter.Agent() == model.AgentOpenCode {
				// The retired SDD step writes no OpenCode settings; the routing
				// guidance owner declares the default-agent ownership file (#5025).
				paths = append(paths, openCodeSDDPluginPaths(adapter, targetDir)...)
				// Shared prompt files in the selected OpenCode config scope — back these up
				// so a sync does not silently overwrite user-customized prompt content.
				// These files are only written for multi-mode (SDDModeMulti), so we only
				// include them in the path list when that mode is active. This prevents
				// false-negative verification failures in single/empty mode syncs.
				if selection.SDDMode == model.SDDModeMulti {
					promptDir := legacyassets.SharedPromptDir(targetDir)
					for _, phase := range legacyassets.SharedPromptPhases() {
						paths = append(paths, filepath.Join(promptDir, phase+".md"))
					}
				}
			}
			paths = append(paths, sddSubAgentPaths(targetDir, adapter)...)
			if adapter.Agent() == model.AgentCodex {
				// SDD installs the Codex skill-registry hook outside the skills
				// directory, so it must share the component's backup contract.
				paths = append(paths, filepath.Join(adapter.GlobalConfigDir(homeDir), "hooks.json"))
			}
		case model.ComponentSkills:
			for _, skillID := range selectedSkillIDs(selection) {
				if skills.IsSDDSkill(skillID) {
					continue
				}
				path := skills.SkillPathForAgent(targetDir, adapter, skillID)
				if path != "" {
					paths = append(paths, path)
				}
			}
			if len(selectedSkillIDs(selection)) > 0 {
				// Enumeration only fails on a corrupt build; the skills step then
				// fails loudly while writing, so no path is declared here.
				if support, err := skills.SupportFilePaths(targetDir, adapter, selectedSkillIDs(selection)); err == nil {
					paths = append(paths, support...)
				}
			}
		case model.ComponentContext7:
			switch adapter.MCPStrategy() {
			case model.StrategySeparateMCPFiles:
				if adapter.Agent() == model.AgentClaudeCode {
					if targetDir == homeDir {
						// Context7 injection writes ~/.claude.json (issue #1868).
						paths = append(paths, claude.UserConfigPath(homeDir))
					} else {
						// Workspace scope writes <project-root>/.mcp.json, the file
						// Claude Code loads project-scoped MCP servers from (issue #2213).
						// The legacy .claude/settings.json block is a best-effort
						// cleanup, not a guaranteed write, so it is not declared as a
						// verification target (declaring it would force a false-negative
						// when the file never existed).
						paths = append(paths, filepath.Join(targetDir, ".mcp.json"))
					}
					break
				}
				paths = append(paths, adapter.MCPConfigPath(targetDir, "context7"))
			case model.StrategyMergeIntoSettings:
				p := adapter.SettingsPath(targetDir)
				if adapter.Agent() == model.AgentOpenCode {
					p = openCodeLoadedSettingsPath(homeDir, workspaceDir, adapter)
				}
				if p != "" {
					paths = append(paths, p)
				}
			case model.StrategyMCPConfigFile:
				if p := adapter.MCPConfigPath(targetDir, "context7"); p != "" {
					paths = append(paths, p)
				}
			case model.StrategyTOMLFile, model.StrategyMergeIntoYAML:
				if p := adapter.MCPConfigPath(targetDir, "context7"); p != "" {
					paths = append(paths, p)
				}
			}
		case model.ComponentPersona:
			if selection.Persona == model.PersonaCustom {
				break
			}
			if adapter.Agent() == model.AgentPi {
				paths = append(paths, piPersonaConfigPaths(homeDir, workspaceDir, scope)...)
				break
			}
			if adapter.Agent() == model.AgentOpenClaw {
				paths = append(paths, filepath.Join(targetDir, "SOUL.md"))
				break
			}
			if adapter.SupportsSystemPrompt() && adapter.SystemPromptStrategy() != model.StrategyJinjaModules {
				paths = append(paths, adapter.SystemPromptFile(targetDir))
			}
			if adapter.SupportsOutputStyles() {
				if stylePaths := persona.ResourcePlanFor(selection.Persona).OutputStylePaths(adapter.OutputStyleDir(targetDir)); stylePaths.Write != "" {
					paths = append(paths, stylePaths.Write)
					if p := adapter.SettingsPath(targetDir); p != "" {
						paths = append(paths, p)
					}
				}
			}
		case model.ComponentPermission:
			p := permissions.TargetPath(homeDir, adapter)
			if adapter.Agent() == model.AgentOpenCode {
				p = openCodeLoadedSettingsPath(homeDir, workspaceDir, adapter)
			}
			if p != "" {
				paths = append(paths, p)
			}
		case model.ComponentGGA:
			paths = append(paths, gga.ConfigPath(homeDir))
			paths = append(paths, gga.AgentsTemplatePath(homeDir))
		case model.ComponentTheme:
			p := adapter.SettingsPath(homeDir)
			if adapter.Agent() == model.AgentOpenCode {
				p = openCodeLoadedSettingsPath(homeDir, workspaceDir, adapter)
			}
			if p != "" {
				paths = append(paths, p)
			}
		case model.ComponentClaudeTheme:
			paths = append(paths, theme.VisualThemePaths(homeDir, adapter)...)
		case model.ComponentOpenCodeGentleLogo:
			if adapter.Agent() == model.AgentOpenCode {
				paths = append(paths,
					filepath.Join(homeDir, ".config", "opencode", "tui-plugins", "gentle-logo.tsx"),
					filepath.Join(homeDir, ".config", "opencode", "tui.json"),
				)
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

// effectiveOpenCodeSettingsPath selects the one OpenCode settings authority for
// a component operation. Global sync/install uses the project-over-global
// resolver; an explicit workspace scope remains workspace-managed.
func effectiveOpenCodeSettingsPath(homeDir, workspaceDir string, scope InstallScope, adapter agents.Adapter) string {
	targetDir := componentInjectionDirScoped(homeDir, workspaceDir, scope, adapter)
	if adapter.Agent() != model.AgentOpenCode || scope == ScopeWorkspace {
		return adapter.SettingsPath(targetDir)
	}
	return opencodeactivation.EffectiveSettingsPath(homeDir, workspaceDir)
}

// openCodeLoadedSettingsPath returns the one settings document OpenCode
// actually loads: the effective project-over-global authority, regardless of
// install scope. OpenCode never reads <workspace>/.config/opencode/opencode.json,
// so a workspace-scoped install must not write settings there (issue #1825).
// Settings writers (Persona, Permission, Context7, Theme, Engram), their backup
// targets, and their declared paths all resolve through here.
func openCodeLoadedSettingsPath(homeDir, workspaceDir string, adapter agents.Adapter) string {
	return effectiveOpenCodeSettingsPath(homeDir, workspaceDir, ScopeGlobal, adapter)
}

// routingGuidanceOptions carries OpenCode's caller-resolved effective settings
// authority into the routing-guidance adapter. Routing remains global because
// OpenCode does not load workspace-scoped orchestrator guidance; other agents
// retain their existing targetDir-derived routing path. Every runtime receives
// the review contract source its orchestrator render needs.
func routingGuidanceOptions(homeDir, workspaceDir string, adapter agents.Adapter) agentguidance.RoutingOptions {
	options := agentguidance.RoutingOptions{ReviewContract: routingReviewContract}
	if adapter.Agent() == model.AgentOpenCode {
		options.SettingsPath = effectiveOpenCodeSettingsPath(homeDir, workspaceDir, ScopeGlobal, adapter)
	}
	return options
}

// routingGuidanceDir resolves the installation root routing guidance is
// delivered under. Orchestrator-prompt adapters retain their home-level adapter
// root for legacy cleanup and ordinary delivery; OpenCode's effective settings
// authority is supplied separately through routingGuidanceOptions. Every other
// agent keeps the ordinary scoped resolution. The guidance step and the backup
// contract both resolve through here so the snapshot cannot drift from what
// the injector writes.
func routingGuidanceDir(homeDir, workspaceDir string, scope InstallScope, adapter agents.Adapter) string {
	if agentguidance.DeliversThroughOrchestratorPrompt(adapter.Agent()) {
		return homeDir
	}
	return componentInjectionDirScoped(homeDir, workspaceDir, scope, adapter)
}

// componentInjectionDirScoped returns the directory to inject component files for the given adapter,
// taking the install scope into account. When scope is ScopeWorkspace, agent-scoped
// components write to workspaceDir instead of the selected agent's global config root.
func componentInjectionDirScoped(homeDir, workspaceDir string, scope InstallScope, adapter agents.Adapter) string {
	return ResolveAgentConfigDir(scope, homeDir, workspaceDir)
}

// piPersonaConfigRoots returns the roots whose Pi persona state is managed by
// install. Like every scoped component, global install writes only to home.
func piPersonaConfigRoots(homeDir, workspaceDir string, scope InstallScope) []string {
	return []string{ResolveAgentConfigDir(scope, homeDir, workspaceDir)}
}

func piPersonaConfigPaths(homeDir, workspaceDir string, scope InstallScope) []string {
	roots := piPersonaConfigRoots(homeDir, workspaceDir, scope)
	paths := make([]string, 0, len(roots))
	for _, root := range roots {
		paths = append(paths, persona.PiPersonaConfigPath(root))
	}
	return paths
}

func nativeReviewCodeGraphGuidanceMarkdown(homeDir string, selected []model.CommunityToolID) string {
	if !shouldInjectNativeReviewCodeGraphGuidance(homeDir, selected) {
		return ""
	}
	return communitytool.CodeGraphGuidanceMarkdown()
}

func shouldInjectNativeReviewCodeGraphGuidance(homeDir string, selected []model.CommunityToolID) bool {
	for _, tool := range selected {
		if tool == model.CommunityToolCodeGraph {
			return true
		}
	}
	return false
}

func communityToolIDsToStrings(tools []model.CommunityToolID) []string {
	if tools == nil {
		return nil
	}
	result := make([]string, 0, len(tools))
	for _, tool := range tools {
		result = append(result, string(tool))
	}
	return result
}

func componentPathDir(homeDir, workspaceDir string, adapter agents.Adapter, component model.ComponentID) string {
	return componentPathDirScoped(homeDir, workspaceDir, ScopeGlobal, adapter, component)
}

func componentPathDirScoped(homeDir, workspaceDir string, scope InstallScope, adapter agents.Adapter, component model.ComponentID) string {
	switch component {
	case model.ComponentEngram, model.ComponentSDD, model.ComponentPersona, model.ComponentSkills, model.ComponentContext7:
		return componentInjectionDirScoped(homeDir, workspaceDir, scope, adapter)
	default:
		return homeDir
	}
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

func openCodeSDDPluginPaths(adapter agents.Adapter, targetDir string) []string {
	// Legacy background-agents is removed during installation and therefore has
	// an absence check. The retired reviewer plugin is part of the rollback
	// snapshot but not post-apply verification because migration removes it.
	// The plugin writer resolves the config directory through the adapter, so
	// verification must ask the same resolver (#3219).
	pluginsDir := filepath.Join(adapter.GlobalConfigDir(targetDir), "plugins")
	paths := []string{filepath.Join(pluginsDir, "background-agents.ts")}
	for _, name := range opencoderuntimeplugins.ManagedOpenCodePluginNames() {
		paths = append(paths, filepath.Join(pluginsDir, name))
	}
	return paths
}

// verificationComponentPaths excludes retired Codex SDD profiles and Pi's
// mcp.json from active health checks. Pi Engram is native-only (gentle-engram),
// so Engram provisioning writes mcp.json only to migrate mcp-adapter.json
// servers and its absence is healthy (#5103). The shared component path
// inventory retains both for snapshots and rollback of user-owned files.
func verificationComponentPaths(homeDir, workspaceDir string, scope InstallScope, selection model.Selection, adapters []agents.Adapter, component model.ComponentID) []string {
	paths := componentPathsWithWorkspaceScoped(homeDir, workspaceDir, scope, selection, adapters, component)
	if component != model.ComponentEngram {
		return paths
	}
	excluded := make(map[string]bool)
	for _, adapter := range adapters {
		targetDir := componentPathDirScoped(homeDir, workspaceDir, scope, adapter, component)
		switch adapter.Agent() {
		case model.AgentCodex:
			if config := adapter.MCPConfigPath(targetDir, "engram"); config != "" {
				for _, profile := range codexagent.SddProfilePaths(filepath.Dir(config)) {
					excluded[profile] = true
				}
			}
		case model.AgentPi:
			if config := adapter.MCPConfigPath(targetDir, "engram"); config != "" {
				excluded[config] = true
			}
		}
	}
	active := make([]string, 0, len(paths))
	for _, path := range paths {
		if !excluded[path] {
			active = append(active, path)
		}
	}
	return active
}

type postApplyVerificationInput struct {
	HomeDir      string
	WorkspaceDir string
	Scope        InstallScope
	Selection    model.Selection
	Resolved     planner.ResolvedPlan
	State        *runtimeState
}

func runPostApplyVerification(input postApplyVerificationInput) verify.Report {
	checks := make([]verify.Check, 0)
	adapters := resolveAdapters(input.Resolved.Agents)

	seenPath := make(map[string]struct{})
	var uniqueFilePaths []string
	for _, component := range input.Resolved.OrderedComponents {
		if component == model.ComponentEngram && !input.Selection.HasComponent(model.ComponentEngram) &&
			input.State != nil && input.State.engramVersionErr != nil {
			// An auto-added engram that could not be installed wrote nothing
			// (#3725); its health check below carries the warning and the
			// install command, so its files are not required here.
			continue
		}
		for _, path := range verificationComponentPaths(input.HomeDir, input.WorkspaceDir, input.Scope, input.Selection, adapters, component) {
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
		if isRetiredManagedPath(path) {
			checks = append(checks, verify.Check{
				ID:          "verify:file:" + path,
				Description: "retired managed file removed",
				Run: func(context.Context) error {
					if _, err := os.Stat(path); err != nil {
						if os.IsNotExist(err) {
							return nil
						}
						return err
					}
					return fmt.Errorf("retired managed file still exists; rerun `gentle-ai sync` to finish retiring it")
				},
			})
			continue
		}
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

	if hasComponent(input.Resolved.OrderedComponents, model.ComponentEngram) {
		checks = append(checks, engramHealthChecks(input.State, input.Resolved.Agents)...)
	}
	checks = append(checks, antigravityCollisionCheck(input.Resolved.Agents)...)
	checks = append(checks, openCodeConfigChecks(input.HomeDir, input.WorkspaceDir, input.Resolved.Agents)...)

	return verify.BuildReport(verify.RunChecks(context.Background(), checks))
}

func openCodeConfigChecks(homeDir, workspaceDir string, agentIDs []model.AgentID) []verify.Check {
	if !containsAgent(agentIDs, model.AgentOpenCode) {
		return nil
	}
	return []verify.Check{{
		ID: "verify:opencode:config-layers", Description: "OpenCode model write authority", Soft: true,
		Run: func(context.Context) error {
			snapshot, err := opencodeactivation.ResolveRuntimeConfigForHome(homeDir, workspaceDir)
			if err != nil {
				return err
			}
			if len(snapshot.Diagnostics) > 0 {
				return errors.New(strings.Join(snapshot.Diagnostics, "\n"))
			}
			return nil
		},
	}}
}

// isRetiredManagedPath reports whether path names a managed file that install
// and sync remove instead of write, so verification checks its absence.
func isRetiredManagedPath(path string) bool {
	return isLegacyOpenCodeBackgroundAgentsPlugin(path) || legacyassets.IsLegacyClaudeCommandPath(path)
}

func isLegacyOpenCodeBackgroundAgentsPlugin(path string) bool {
	path = filepath.Clean(path)
	pluginsDir := filepath.Dir(path)
	opencodeDir := filepath.Dir(pluginsDir)
	if filepath.Base(path) != "background-agents.ts" || filepath.Base(pluginsDir) != "plugins" || filepath.Base(opencodeDir) != "opencode" {
		return false
	}
	if filepath.Base(filepath.Dir(opencodeDir)) == ".config" {
		return true
	}
	// A home installed with XDG_CONFIG_HOME keeps its OpenCode config under
	// $XDG_CONFIG_HOME/opencode instead of ~/.config/opencode (#3219).
	xdgConfigHome := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME"))
	return filepath.IsAbs(xdgConfigHome) && opencodeDir == filepath.Join(filepath.Clean(xdgConfigHome), "opencode")
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

// engramHealthChecks builds the post-apply engram soft checks. When state
// already carries a resolved `engram version` result (componentApplyStep.Run
// resolves it once for the Decision 1 gate whenever ComponentEngram is
// applied), the version check reuses that result instead of shelling out to
// `engram version` a second time (JD-016). The fallback path (state nil or
// not yet resolved) still routes through the verifyEngramVersion seam var
// rather than calling engram.VerifyVersion() directly, so it stays fakeable
// in tests.
func engramHealthChecks(state *runtimeState, agentIDs []model.AgentID) []verify.Check {
	return []verify.Check{
		{
			ID:          "verify:engram:binary",
			Description: "engram binary on PATH (restart shell if missing)",
			Soft:        true,
			Run: func(context.Context) error {
				if err := engram.VerifyInstalled(); err != nil {
					return fmt.Errorf("%w\nInstall it with `%s`.\nIf engram was installed via `go install`, add it to PATH:\n  %s", err, engramInstallCommand(agentIDs), engramPathGuidance(os.Getenv("SHELL")))
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
				if state != nil && state.engramVersionResolved {
					return state.engramVersionErr
				}
				_, err := verifyEngramVersion()
				return err
			},
		},
	}
}

// engramInstallCommand names the install continuation for a missing engram
// binary so the warning that reports it is actionable on its own.
func engramInstallCommand(agentIDs []model.AgentID) string {
	return fmt.Sprintf("gentle-ai install --agent %s --components engram", joinAgentIDs(agentIDs))
}

// antigravityCollisionCheck returns a soft verify check that warns the user
// when Antigravity and Gemini CLI are selected together. These agents
// intentionally share ~/.gemini/GEMINI.md because Antigravity uses a
// Gemini-compatible prompt surface; the last synced agent routing guidance
// controls the shared gentle-ai:agent-routing section.
func antigravityCollisionCheck(agents []model.AgentID) []verify.Check {
	hasAntigravitySurface := false
	hasGemini := false
	for _, id := range agents {
		if id == model.AgentAntigravity {
			hasAntigravitySurface = true
		}
		if id == model.AgentGeminiCLI {
			hasGemini = true
		}
	}
	if !hasAntigravitySurface || !hasGemini {
		return nil
	}
	return []verify.Check{
		{
			ID:          "verify:antigravity:rules-collision",
			Description: "Antigravity and Gemini CLI share ~/.gemini/GEMINI.md",
			Soft:        true,
			Run: func(context.Context) error {
				return fmt.Errorf(
					"Antigravity and Gemini CLI write rules to ~/.gemini/GEMINI.md\n" +
						"Antigravity intentionally uses the Gemini-compatible global prompt surface; the last synced agent routing guidance controls the shared gentle-ai:agent-routing section.\n" +
						"Prefer Antigravity for new installs; keep Gemini CLI selected only when you intentionally want that legacy prompt to be the active one.",
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
	resolved  planner.ResolvedPlan
	channel   InstallChannel
}

func (s checkDependenciesStep) ID() string {
	return s.id
}

func (s checkDependenciesStep) Run() error {
	for _, component := range s.resolved.OrderedComponents {
		if betaEngramRequiresGo(s.channel, component) {
			if _, err := cmdLookPath("go"); err != nil {
				return fmt.Errorf("beta Engram requires Go; stopped before backup or component apply. Run `%s`, restart the terminal, then rerun your original `gentle-ai install` command", system.InstallHintForDep("go", s.profile))
			}
			break
		}
	}

	// Run detection but do NOT write to stdout/stderr — this step runs
	// inside the Bubble Tea alternate screen in TUI mode, so any raw
	// output corrupts the display (see issue #2). Missing deps are
	// surfaced on the TUI complete screen and by the actual install steps
	// failing with real error messages.
	_ = system.DetectDependencies(context.Background(), s.profile)
	for _, agent := range s.selection.Agents {
		// Only Pi executes package commands (its already-present `pi`
		// subcommands and npm-based Engram initialization — see
		// agentInstallStep). Other selected agents receive configuration without
		// runtime acquisition, so their missing package managers must not block
		// this pipeline.
		if agent != model.AgentPi {
			continue
		}

		adapter, err := agents.NewAdapter(agent)
		if err != nil {
			return fmt.Errorf("create adapter for %q: %w", agent, err)
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

// kiroAliasesToStrings converts a typed KiroModelAlias map to plain strings
// for JSON serialisation in state.json.
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
