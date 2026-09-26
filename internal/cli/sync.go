package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v3/internal/agents"
	opencodeagent "github.com/gentleman-programming/gentle-ai/v3/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/v3/internal/backup"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/agentguidance"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/communitytool"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/engram"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/gga"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/mcp"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/opencodedefault"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/opencodeplugin"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/opencoderuntimeplugins"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/permissions"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/persona"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/reviewassets"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/skills"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/telemetryruntime"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/theme"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
	opencodeactivation "github.com/gentleman-programming/gentle-ai/v3/internal/opencode"
	"github.com/gentleman-programming/gentle-ai/v3/internal/pipeline"
	"github.com/gentleman-programming/gentle-ai/v3/internal/state"
	"github.com/gentleman-programming/gentle-ai/v3/internal/system"
	"github.com/gentleman-programming/gentle-ai/v3/internal/telemetry"
	"github.com/gentleman-programming/gentle-ai/v3/internal/verify"
)

// SyncFlags holds parsed CLI flags for the sync command.
type SyncFlags struct {
	Agents             []string
	Skills             []string
	SDDMode            string
	SDDProfileStrategy string
	StrictTDD          bool
	IncludePermissions bool
	IncludeTheme       bool
	DryRun             bool

	OpenCodeBackgroundSubagents    string
	OpenCodeBackgroundSubagentsSet bool

	PiBackgroundSubagents    string
	PiBackgroundSubagentsSet bool
	// Profiles remains available to internal callers with persisted selections.
	Profiles       []model.Profile
	skillsSet      bool
	sddModeSet     bool
	strictTDDSet   bool
	permissionsSet bool
	themeSet       bool
}

// SyncResult holds the outcome of a sync execution.
type SyncResult struct {
	Agents    []model.AgentID
	Selection model.Selection
	Plan      pipeline.StagePlan
	Execution pipeline.ExecutionResult
	Verify    verify.Report
	DryRun    bool
	// NoOp is true when no managed asset changes were needed:
	// either no agents were discovered/provided, or all managed assets
	// were already current (idempotent re-sync).
	NoOp bool
	// FilesChanged is the number of deduplicated managed file paths
	// whose persisted content or existence changed during this sync.
	// Zero means all assets were already current.
	FilesChanged int
	// ChangedFiles lists deduplicated absolute paths of managed files
	// processed during this sync. Paths appear once even when multiple
	// components touch the same file. It is nil when no files changed.
	ChangedFiles  []string
	ManualActions []string

	Background              OpenCodeBackgroundResolution
	BackgroundPolicyEnabled bool

	PiBackground PiBackgroundResolution
}

// ParseSyncFlags parses the CLI arguments for the sync subcommand.
func ParseSyncFlags(args []string) (SyncFlags, error) {
	var opts SyncFlags

	// Usage is captured (not discarded) so a mistyped flag error can carry the
	// FlagSet's own canonical usage text (install/sync surface audit finding
	// 4): the flag package's failf() calls fs.usage() on every parse error
	// (undefined flag, bad value, or -h/--help itself), so capturing
	// fs.Output() gives the exact registered-flag list without hand-listing
	// it anywhere that could drift.
	var usage bytes.Buffer
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.SetOutput(&usage)
	registerListFlag(fs, "agent", &opts.Agents)
	registerListFlag(fs, "agents", &opts.Agents)
	registerListFlag(fs, "skill", &opts.Skills)
	registerListFlag(fs, "skills", &opts.Skills)
	fs.BoolVar(&opts.StrictTDD, "strict-tdd", false, "retired: ODD uses applicable test-first development by default")
	fs.BoolVar(&opts.IncludePermissions, "include-permissions", false, "include permissions component in sync")
	fs.BoolVar(&opts.IncludeTheme, "include-theme", false, "include theme component in sync")
	fs.StringVar(&opts.OpenCodeBackgroundSubagents, "opencode-background-subagents", "", "--opencode-background-subagents=auto|on|off; env: GENTLE_AI_OPENCODE_BACKGROUND_SUBAGENTS; eligible versions use a managed launcher")
	fs.StringVar(&opts.PiBackgroundSubagents, "pi-background-subagents", "", "--pi-background-subagents=auto|on|off; env: GENTLE_AI_PI_BACKGROUND_SUBAGENTS; the resolved policy is projected for gentle-pi")
	fs.BoolVar(&opts.DryRun, "dry-run", false, "preview plan without executing")

	if err := fs.Parse(args); err != nil {
		// The flag package's own failf/usage() may have already printed the
		// error text once before the "Usage of sync:" block; trim that
		// duplicate so the wrapped error below does not repeat it.
		usageText := usage.String()
		if idx := strings.Index(usageText, "Usage of "); idx >= 0 {
			usageText = usageText[idx:]
		}
		usageText = strings.TrimRight(usageText, "\n")
		if usageText != "" {
			return SyncFlags{}, fmt.Errorf("%w — run `gentle-ai sync --help` for the supported flags:\n%s", err, usageText)
		}
		return SyncFlags{}, fmt.Errorf("%w — run `gentle-ai sync --help` for the supported flags", err)
	}
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "skill", "skills":
			opts.skillsSet = true
		case "strict-tdd":
			opts.strictTDDSet = true
		case "include-permissions":
			opts.permissionsSet = true
		case "include-theme":
			opts.themeSet = true
		case "opencode-background-subagents":
			opts.OpenCodeBackgroundSubagentsSet = true
		case "pi-background-subagents":
			opts.PiBackgroundSubagentsSet = true
		}
	})

	if opts.strictTDDSet {
		return SyncFlags{}, fmt.Errorf("--strict-tdd is retired: ODD uses applicable test-first development by default; rerun `gentle-ai sync` without --strict-tdd (retain any other flags)")
	}

	if fs.NArg() > 0 {
		return SyncFlags{}, fmt.Errorf("unexpected sync argument %q — pass agents with the --agent %s flag, not a positional argument", fs.Arg(0), fs.Arg(0))
	}

	return opts, nil
}

func PrintSyncHelp(w io.Writer) {
	fmt.Fprint(w, `USAGE
  gentle-ai sync [flags]

FLAGS
  --agent, --agents <list>           Agents to sync
  --skill, --skills <list>           Skills to sync
  --strict-tdd                       Retired (rejected); applicable test-first ODD is default
  --include-permissions              Include permissions component
  --include-theme                    Include theme component
  --opencode-background-subagents=auto|on|off
                                     Resolve OpenCode capability and manage a launcher when eligible; env: GENTLE_AI_OPENCODE_BACKGROUND_SUBAGENTS
                                     auto inherits managed on/off, unsupported/unknown stays foreground, off removes only owned launchers
  --pi-background-subagents=auto|on|off
                                     Project the resolved Pi background-subagent policy for gentle-pi; env: GENTLE_AI_PI_BACKGROUND_SUBAGENTS
                                     auto inherits managed on/off and never enables by itself; only managed policy files are ever overwritten
  --dry-run                          Preview plan without executing
  --help, -h                         Show this help
`)
}

// BuildSyncSelection builds a model.Selection for the sync command.
//
// Default sync scope: Engram, Context7, GGA, Skills, Persona.
// Excluded by default: Permissions, Theme (no markers; managed via JSON
// overlays where user customization cannot be safely diff-merged).
// Permissions and Theme can be opted-in via flags.
//
// Persona is included because its content lives between
// <!-- gentle-ai:persona --> markers — that block is harness-managed and
// must propagate embedded-asset changes across versions. Content outside
// the markers (user-authored sections) is preserved by InjectMarkdownSection.
//
// This is the reusable managed-asset sync contract. A future `upgrade --sync`
// flow can call this function to get the same managed-only selection semantics.
func BuildSyncSelection(flags SyncFlags, agentIDs []model.AgentID) model.Selection {
	// Order matters: Persona must run BEFORE Engram/MCP because those
	// components inject content with substrings (e.g. "## Personality",
	// "Senior Architect") that overlap with persona's legacy-block fingerprints.
	// Running persona last would cause its StripLegacyPersonaBlock pass to
	// detect the just-written managed sections as legacy and strip them.
	components := []model.ComponentID{
		model.ComponentPersona,
		model.ComponentEngram,
		model.ComponentContext7,
		model.ComponentGGA,
		model.ComponentSkills,
	}

	if flags.IncludePermissions {
		components = append(components, model.ComponentPermission)
	}
	if flags.IncludeTheme {
		components = append(components, model.ComponentTheme)
	}

	sddMode := model.SDDModeID(flags.SDDMode)

	var skillIDs []model.SkillID
	for _, raw := range flags.Skills {
		skillIDs = append(skillIDs, model.SkillID(raw))
	}

	return model.Selection{
		Agents:             agentIDs,
		Components:         components,
		SDDMode:            sddMode,
		SDDProfileStrategy: model.SDDProfileStrategyID(flags.SDDProfileStrategy),
		Skills:             skillIDs,
		Profiles:           flags.Profiles,
		// Preset is set to full-gentleman so selectedSkillIDs() returns the
		// correct default skill set when no explicit skills are provided.
		Preset: model.PresetFullGentleman,
		// Persona is left as zero-value here. RunSync resolves it from state.json
		// when present. A missing persona field resolves to neutral; invalid state
		// is rejected so sync cannot silently reactivate regional persona behavior.
	}
}

func RestorePersistedSelection(selection *model.Selection, persisted state.InstallState, flags SyncFlags) {
	if !persisted.SelectionConfigured {
		return
	}
	explicit := *selection
	persisted.RestoreSelection(selection)
	if flags.skillsSet {
		selection.Skills = explicit.Skills
		setSelectionComponent(selection, model.ComponentSkills, true, true)
	}
	setSelectionComponent(selection, model.ComponentPermission, flags.permissionsSet, flags.IncludePermissions)
	setSelectionComponent(selection, model.ComponentTheme, flags.themeSet, flags.IncludeTheme)
}

func setSelectionComponent(selection *model.Selection, component model.ComponentID, configured, included bool) {
	if !configured {
		return
	}
	filtered := selection.Components[:0]
	for _, current := range selection.Components {
		if current != component {
			filtered = append(filtered, current)
		}
	}
	selection.Components = filtered
	if included {
		selection.Components = append(selection.Components, component)
	}
}

// DiscoverAgents returns the agent IDs to sync. Persisted selections are
// authoritative, including an explicitly configured empty selection. Missing or
// incidental state falls back to filesystem discovery; unreadable state fails
// closed so sync cannot inject into unselected agent configuration.
//
// When --agents is provided explicitly, callers should pass those IDs directly
// instead of calling DiscoverAgents.
func DiscoverAgents(homeDir string) []model.AgentID {
	scope := agents.ReadSelectionScope(homeDir)
	switch scope.Mode {
	case agents.SelectionScopeConfigured:
		return scope.AgentIDs
	case agents.SelectionScopeUnavailable:
		return nil
	}

	// Fallback: filesystem discovery for legacy installs and incidental state
	// written before an installation records a user selection.
	reg, err := agents.NewDefaultRegistry()
	if err != nil {
		// Registry construction only fails if a duplicate adapter is registered,
		// which would indicate a programming error. Treat as no agents found
		// rather than propagating — callers treat an empty result as a no-op.
		return nil
	}

	installed := agents.DiscoverInstalled(reg, homeDir)
	ids := make([]model.AgentID, 0, len(installed))
	for _, a := range installed {
		ids = append(ids, a.ID)
	}
	return ids
}

// syncRuntime mirrors installRuntime but builds a sync-scoped StagePlan.
// It reuses backup/rollback infrastructure but only calls inject functions —
// no agentInstallStep, no engram setup, no persona.
type syncRuntime struct {
	homeDir              string
	workspaceDir         string
	selection            model.Selection
	agentIDs             []model.AgentID
	backupRoot           string
	state                *runtimeState
	managedPaths         []string
	changedFiles         []string // accumulates candidate paths reported by component injectors
	backgroundPolicy     bool
	backgroundActivation *opencodeactivation.ActivationPlan
	runtimeReady         bool

	piBackgroundProjection *piBackgroundProjectionPlan
}

func newSyncRuntime(homeDir string, selection model.Selection) (*syncRuntime, error) {
	backupRoot := filepath.Join(homeDir, ".gentle-ai", "backups")
	workspaceDir, _ := os.Getwd()
	compatibilityTransaction, err := newCompatibilityRefreshTransaction(homeDir, selection.Components, selection)
	if err != nil {
		return nil, err
	}

	runtime := &syncRuntime{
		homeDir:      homeDir,
		workspaceDir: workspaceDir,
		selection:    selection,
		agentIDs:     selection.Agents,
		backupRoot:   backupRoot,
		state:        &runtimeState{compatibilityTransaction: compatibilityTransaction},
	}
	return runtime, nil
}

func (r *syncRuntime) stagePlan() pipeline.StagePlan {
	adapters := resolveAdapters(r.agentIDs)
	targets, targetErr := syncBackupTargets(r.homeDir, r.workspaceDir, r.selection, adapters)
	r.managedPaths = targets

	prepare := []pipeline.Step{
		prepareBackupStep{
			id:          "prepare:backup-snapshot",
			snapshotter: backup.NewSnapshotter(),
			snapshotDir: filepath.Join(r.backupRoot, time.Now().UTC().Format("20060102150405.000000000")),
			targets:     targets,
			targetErr:   targetErr,
			state:       r.state,
			backupRoot:  r.backupRoot,
			source:      backup.BackupSourceSync,
			description: "pre-sync snapshot",
			appVersion:  AppVersion,
		},
	}

	telemetryDir := openCodeTelemetryConfigDir(r.homeDir, r.workspaceDir, ScopeGlobal, r.agentIDs)
	if telemetryDir != "" {
		prepare = append([]pipeline.Step{openCodeTelemetryStep{id: "prepare:opencode-telemetry", configDir: telemetryDir, checkOnly: true}}, prepare...)
	}
	apply := []pipeline.Step{
		rollbackRestoreStep{id: "apply:rollback-restore", state: r.state, homeDir: r.homeDir, workspaceDir: r.workspaceDir, telemetryConfigDir: telemetryDir},
	}
	if telemetryDir != "" {
		apply = append(apply, openCodeTelemetryStep{id: "sync:opencode:telemetry-runtime", configDir: telemetryDir, changedFiles: &r.changedFiles, state: r.state})
	}
	if r.backgroundActivation != nil {
		apply = append(apply, openCodeBackgroundActivationStep{id: "sync:opencode:background-activation", plan: r.backgroundActivation, state: r.state, ready: &r.runtimeReady})
	}
	if r.piBackgroundProjection != nil {
		apply = append(apply, piBackgroundProjectionStep{id: "sync:pi:background-projection", plan: r.piBackgroundProjection})
	}

	for _, component := range r.selection.Components {
		apply = append(apply, componentSyncStep{
			id:               "sync:component:" + string(component),
			component:        component,
			homeDir:          r.homeDir,
			workspaceDir:     r.workspaceDir,
			agents:           r.agentIDs,
			selection:        r.selection,
			changedFiles:     &r.changedFiles,
			backgroundPolicy: r.backgroundPolicy,
		})
	}
	if needsCompatibilitySkillsRefresh(r.selection.Components) {
		apply = append(apply, compatibilitySkillsRefreshStep{
			id:           "sync:compatibility-skills-refresh",
			homeDir:      r.homeDir,
			components:   r.selection.Components,
			selection:    r.selection,
			changedFiles: &r.changedFiles,
			transaction:  r.state.compatibilityTransaction,
			anchored:     usesAnchoredCompatibilityTransaction(),
		})
	}

	for _, agent := range r.agentIDs {
		if nativeReviewAgentSupported(agent) {
			apply = append(apply, nativeReviewAgentStep{id: "sync:agent:native-review:" + string(agent), agent: agent, homeDir: r.homeDir, workspaceDir: r.workspaceDir, scope: ScopeGlobal, selection: r.selection, changedFiles: &r.changedFiles, state: r.state})
		}
	}

	// Routing guidance is refreshed per agent and outside the component loop, for
	// the same reason install schedules it there: a persisted selection without
	// any optional component must still leave every agent able to choose an
	// implementation route (issue #1794). It runs after the components so their
	// managed assets are in place before guidance is merged.
	for _, agent := range r.agentIDs {
		apply = append(apply, agentRoutingGuidanceStep{
			codexPhaseModels: r.selection.CodexPhaseModelAssignments,
			codexEfforts:     r.selection.CodexModelAssignments,
			codexCarrils:     r.selection.CodexCarrilModelAssignments,
			backgroundPolicy: r.backgroundPolicy,
			legacySDD:        false,
			id:               "sync:agent-guidance:" + string(agent),
			agent:            agent,
			homeDir:          r.homeDir,
			workspaceDir:     r.workspaceDir,
			scope:            ScopeGlobal,
			changedFiles:     &r.changedFiles,
		})
	}

	// Model assignments belong to the retained OpenCode sync route, not the
	// retired SDD component. Run after guidance so both updates to opencode.json
	// are included in the same sync transaction.
	if len(r.selection.ModelAssignments) > 0 {
		for _, adapter := range adapters {
			if adapter.Agent() == model.AgentOpenCode {
				apply = append(apply, openCodeModelAssignmentSyncStep{
					path:        effectiveOpenCodeSettingsPath(r.homeDir, r.workspaceDir, ScopeGlobal, adapter),
					assignments: r.selection.ModelAssignments, changedFiles: &r.changedFiles,
				})
			}
		}
	}

	// Retire remaining markers only after routing has consumed them as ownership
	// proof to remove retired agents and refresh current roles. Assignments may
	// also write settings, so cleanup runs after those updates as well.
	for _, adapter := range adapters {
		if adapter.Agent() == model.AgentOpenCode {
			apply = append(apply, &openCodeMarkerMigrationSyncStep{
				path: effectiveOpenCodeSettingsPath(r.homeDir, r.workspaceDir, ScopeGlobal, adapter), changedFiles: &r.changedFiles,
			})
		}
	}

	// Managed OpenCode-compatible plugins are versioned runtime artifacts tied
	// to the installed binary (OpenCode and Kilocode receive them). When the
	// persisted selection lacks the SDD component, no SDD step is planned and
	// already-installed plugins would silently stay stale after upgrades
	// (issue #1440). Refresh installed copies explicitly; the step never
	// installs plugins that were never present. When SDD is selected, its
	// inject step already rewrites the plugins.
	if anyAgentReceivesManagedOpenCodePlugins(r.agentIDs) {
		apply = append(apply, openCodePluginRefreshSyncStep{
			id:           "sync:opencode:managed-plugins",
			homeDir:      r.homeDir,
			agents:       r.agentIDs,
			changedFiles: &r.changedFiles,
		})
	}

	if r.selection.HasCommunityTool(model.CommunityToolCodeGraph) {
		apply = append(apply, &codeGraphGuidanceSyncStep{
			id:           "sync:community-tool:codegraph-guidance",
			homeDir:      r.homeDir,
			runner:       codeGraphHomeRunner{homeDir: r.homeDir},
			changedFiles: &r.changedFiles,
		})
		apply = append(apply, piCodeGraphSyncStep{id: "sync:community-tool:pi-codegraph", homeDir: r.homeDir, workspaceDir: r.workspaceDir, changedFiles: &r.changedFiles})
	}

	return pipeline.StagePlan{Prepare: prepare, Apply: apply}
}

// syncBackupTargets returns the file paths that need to be backed up
// before sync executes. Uses syncComponentPaths so that the backup/verify
// contract matches the actual files sync touches (which differ from install
// for ComponentPersona, see syncComponentPaths). One deliberate exception:
// persona backup also captures the non-selected managed output-style file so a
// failed persona switch can be rolled back (verification still declares only
// the selected file).
func syncBackupTargets(homeDir, workspaceDir string, selection model.Selection, adapters []agents.Adapter) ([]string, error) {
	paths := map[string]struct{}{}
	for _, component := range selection.Components {
		if component == model.ComponentSDD {
			continue
		}
		for _, path := range syncComponentPathsWithWorkspace(homeDir, workspaceDir, selection, adapters, component) {
			paths[path] = struct{}{}
		}
		if component == model.ComponentContext7 {
			for _, path := range claudeMCPSettingsCleanupPaths(homeDir, workspaceDir, ScopeGlobal, adapters) {
				paths[path] = struct{}{}
			}
		}
		if component == model.ComponentEngram {
			for _, adapter := range adapters {
				if adapter.Agent() == model.AgentClaudeCode {
					paths[adapter.MCPConfigPath(homeDir, "engram")] = struct{}{}
				}
			}
		}
		if component == model.ComponentPersona {
			plan := persona.ResourcePlanFor(selection.Persona)
			for _, adapter := range adapters {
				if adapter.Agent() == model.AgentPi {
					paths[adapter.SystemPromptFile(homeDir)] = struct{}{}
				}
				if adapter.Agent() == model.AgentOpenCode || adapter.Agent() == model.AgentKilocode {
					// Persona sync can remove stale managed agent state from settings.
					// This target is backup-only: syncPersonaPaths intentionally does
					// not make best-effort cleanup a post-sync verification target.
					path := adapter.SettingsPath(componentInjectionDir(homeDir, workspaceDir, adapter))
					if adapter.Agent() == model.AgentOpenCode {
						path = effectiveOpenCodeSettingsPath(homeDir, workspaceDir, ScopeGlobal, adapter)
					}
					if path != "" {
						paths[path] = struct{}{}
					}
				}
				if !adapter.SupportsOutputStyles() {
					continue
				}
				for _, path := range plan.OutputStylePaths(adapter.OutputStyleDir(componentInjectionDir(homeDir, workspaceDir, adapter))).Backup {
					paths[path] = struct{}{}
				}
			}
		}
	}
	for _, adapter := range adapters {
		if adapter.Agent() == model.AgentOpenCode {
			paths[effectiveOpenCodeSettingsPath(homeDir, workspaceDir, ScopeGlobal, adapter)] = struct{}{}
		}
	}
	if len(selection.ModelAssignments) > 0 {
		for _, adapter := range adapters {
			if adapter.Agent() == model.AgentOpenCode {
				paths[effectiveOpenCodeSettingsPath(homeDir, workspaceDir, ScopeGlobal, adapter)] = struct{}{}
			}
		}
	}
	// Routing guidance is refreshed per agent outside the component loop, at
	// ScopeGlobal like the step itself. A persisted selection whose components
	// do not cover the same file would otherwise be rewritten without a
	// snapshot and could never be rolled back (issue #1794).
	for _, adapter := range adapters {
		if path := agentguidance.StrictTDDPath(routingGuidanceDir(homeDir, workspaceDir, ScopeGlobal, adapter), adapter.Agent()); path != "" {
			paths[path] = struct{}{}
		}
	}
	for _, path := range routingGuidancePaths(homeDir, workspaceDir, ScopeGlobal, adapters) {
		paths[path] = struct{}{}
	}
	for _, adapter := range adapters {
		if names := reviewassets.NativeAgentFileNames(adapter.Agent()); len(names) > 0 {
			dir := adapter.SubAgentsDir(componentInjectionDirScoped(homeDir, workspaceDir, ScopeGlobal, adapter))
			paths[filepath.Join(dir, reviewassets.OwnershipLedgerFilename)] = struct{}{}
			for _, name := range names {
				paths[filepath.Join(dir, name)] = struct{}{}
			}
		}
		if adapter.Agent() == model.AgentPi {
			paths[adapter.SystemPromptFile(homeDir)] = struct{}{}
		}
		if adapter.Agent() == model.AgentCodex {
			paths[filepath.Join(adapter.GlobalConfigDir(homeDir), "hooks.json")] = struct{}{}
		}
	}
	if configDir := openCodeTelemetryConfigDir(homeDir, workspaceDir, ScopeGlobal, selection.Agents); configDir != "" {
		for _, path := range telemetryruntime.ManagedPaths(configDir) {
			paths[path] = struct{}{}
		}
	}
	// Managed OpenCode-compatible plugin paths are part of sync's
	// backup/snapshot contract whenever a plugin-receiving agent (OpenCode,
	// Kilocode) is synced, independent of the SDD component: the
	// openCodePluginRefreshSyncStep may rewrite installed copies (issue #1440).
	for _, adapter := range adapters {
		if !opencoderuntimeplugins.AgentReceivesManagedOpenCodePlugins(adapter.Agent()) {
			continue
		}
		pluginsDir := filepath.Join(adapter.GlobalConfigDir(homeDir), "plugins")
		for _, name := range opencoderuntimeplugins.OpenCodePluginLifecycleNames(adapter.Agent()) {
			paths[filepath.Join(pluginsDir, name)] = struct{}{}
		}
	}
	adapterSkillPaths, err := syncAdapterSkillBackupTargets(homeDir, workspaceDir, selection, adapters)
	if err != nil {
		return nil, err
	}
	for _, path := range adapterSkillPaths {
		paths[path] = struct{}{}
	}
	if !usesAnchoredCompatibilityTransaction() && needsCompatibilitySkillsRefresh(selection.Components) {
		skillDir, ok, err := compatibilitySkillsDir(homeDir)
		if err != nil {
			return nil, err
		}
		if ok {
			compatibilityPaths, err := compatibilitySkillFiles(skillDir, selection.Components, selection)
			if err != nil {
				return nil, err
			}
			for _, path := range compatibilityPaths {
				paths[path] = struct{}{}
			}
		}
	}
	if selection.HasCommunityTool(model.CommunityToolCodeGraph) {
		for _, path := range communitytool.CodeGraphManagedPaths(homeDir) {
			paths[path] = struct{}{}
		}
	}
	for _, path := range communitytool.PiCodeGraphPaths(homeDir, workspaceDir) {
		paths[path] = struct{}{}
	}
	if containsAgent(selection.Agents, model.AgentOpenCode) {
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

func syncAdapterSkillBackupTargets(homeDir, workspaceDir string, selection model.Selection, adapters []agents.Adapter) ([]string, error) {
	var paths []string
	for _, adapter := range adapters {
		if !adapter.SupportsSkills() {
			continue
		}
		if slices.Contains(selection.Components, model.ComponentSkills) {
			skillDir := adapter.SkillsDir(componentInjectionDir(homeDir, workspaceDir, adapter))
			if skillDir == "" {
				continue
			}
			ordinary, err := skills.DirectoryPaths(skillDir, selectedSkillIDs(selection), "")
			if err != nil {
				return nil, fmt.Errorf("enumerate %s skill backup targets: %w", adapter.Agent(), err)
			}
			paths = append(paths, ordinary...)
			support, err := skillSupportBackupTargets(componentInjectionDir(homeDir, workspaceDir, adapter), adapter, selection)
			if err != nil {
				return nil, err
			}
			paths = append(paths, support...)
		}
	}
	return paths, nil
}

// syncComponentPaths declares the file paths sync writes for a given component.
//
// For most components the contract is identical to install (componentPaths).
// ComponentPersona is the exception: sync calls persona.InjectForSync rather
// than merging persona definitions. Its narrow OpenCode cleanup remains a
// backup-only transaction target, not a post-sync verification path.
func syncComponentPaths(homeDir string, selection model.Selection, adapters []agents.Adapter, component model.ComponentID) []string {
	return syncComponentPathsWithWorkspace(homeDir, "", selection, adapters, component)
}

func syncComponentPathsWithWorkspace(homeDir, workspaceDir string, selection model.Selection, adapters []agents.Adapter, component model.ComponentID) []string {
	if component == model.ComponentPersona {
		return syncPersonaPathsWithWorkspace(homeDir, workspaceDir, selection, adapters)
	}
	return componentPathsWithWorkspace(homeDir, workspaceDir, selection, adapters, component)
}

// syncPersonaPaths returns the file paths that ComponentPersona writes during
// sync. Mirrors persona.InjectForSync and the Pi runtime config writer:
//   - Step 1: SystemPromptFile (the marker-bound markdown block — CLAUDE.md /
//     AGENTS.md / equivalent).
//   - Step 3: managed output-style overlay (only when the agent supports it).
//   - Pi: the home-level gentle-pi persona state file.
//
// Step 2 does not merge OpenCode/Kilocode persona definitions during sync. A
// narrow stale-state cleanup is tracked separately as a backup-only target.
func syncPersonaPaths(homeDir string, selection model.Selection, adapters []agents.Adapter) []string {
	return syncPersonaPathsWithWorkspace(homeDir, "", selection, adapters)
}

func syncPersonaPathsWithWorkspace(homeDir, workspaceDir string, selection model.Selection, adapters []agents.Adapter) []string {
	if selection.Persona == model.PersonaCustom {
		return nil
	}
	paths := []string{}
	for _, adapter := range adapters {
		if adapter.Agent() == model.AgentPi {
			paths = append(paths, persona.PiPersonaConfigPath(homeDir))
			continue
		}
		targetDir := componentInjectionDir(homeDir, workspaceDir, adapter)
		if adapter.Agent() == model.AgentOpenClaw {
			paths = append(paths, filepath.Join(targetDir, "SOUL.md"))
			continue
		}
		if !adapter.SupportsSystemPrompt() {
			continue
		}
		if adapter.SystemPromptStrategy() != model.StrategyJinjaModules {
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
	}
	return paths
}

// componentSyncStep is the sync-specific apply step.
// Unlike componentApplyStep, it ONLY calls inject functions —
// no binary install, no engram setup, no persona injection.
//
// changedFiles is a shared slice pointer. Each step appends candidate paths
// from its aggregate InjectionResult when any file changed. RunSync compares
// candidates with pre-sync snapshots before exposing persisted changes.
// openCodeMarkerMigrationSyncStep retires only known legacy generated-agent
// markers. The pipeline snapshot includes its settings path for rollback.
type openCodeMarkerMigrationSyncStep struct {
	path         string
	changedFiles *[]string
	before       []byte
	mode         os.FileMode
	changed      bool
}

func (s *openCodeMarkerMigrationSyncStep) ID() string { return "sync:opencode:legacy-markers" }

func (s *openCodeMarkerMigrationSyncStep) Run() error {
	info, err := os.Lstat(s.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat OpenCode settings: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("refuse non-regular OpenCode settings %q: inspect the path and use a regular settings file (not a symlink), then rerun gentle-ai sync", s.path)
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		return fmt.Errorf("read OpenCode settings: %w", err)
	}
	names := append([]string{"gentle-orchestrator"}, opencodeactivation.GentleAIODDPhases()...)
	names = append(names, opencodeactivation.JDPhases()...)
	names = append(names, opencodeactivation.ReviewPhases()...)
	// Older installations used sdd-* names; these are eligible only when
	// their definition still carries the exact retired ownership marker.
	names = append(names, "sdd-orchestrator", "sdd-init", "sdd-explore", "sdd-propose", "sdd-spec", "sdd-design", "sdd-tasks", "sdd-apply", "sdd-verify", "sdd-archive", "sdd-onboard")
	updated, err := filemerge.RemoveLegacyOpenCodeAgentMarkers(s.path, raw, names)
	if err != nil {
		return fmt.Errorf("migrate OpenCode agent markers: %w", err)
	}
	if bytes.Equal(raw, updated) {
		return nil
	}
	s.before, s.mode = raw, info.Mode().Perm()
	result, err := filemerge.WriteFileAtomic(s.path, updated, s.mode)
	s.changed = result.Changed
	if result.Changed && s.changedFiles != nil {
		*s.changedFiles = append(*s.changedFiles, s.path)
	}
	if err != nil {
		return fmt.Errorf("write OpenCode agent markers: %w", err)
	}
	return nil
}

func (s *openCodeMarkerMigrationSyncStep) Rollback() error {
	if !s.changed {
		return nil
	}
	_, err := filemerge.WriteFileAtomic(s.path, s.before, s.mode)
	return err
}

// openCodeModelAssignmentSyncStep persists picker choices independently of the
// retired SDD component. Only current picker identities are eligible; legacy
// saved SDD keys must not be resurrected by an ordinary sync.
type openCodeModelAssignmentSyncStep struct {
	path         string
	assignments  map[string]model.ModelAssignment
	changedFiles *[]string
}

func (s openCodeModelAssignmentSyncStep) ID() string { return "sync:opencode:model-assignments" }

func (s openCodeModelAssignmentSyncStep) Run() error {
	// Refuse leaf links before discovery/read: following one can read and then
	// overwrite a target not covered by the sync snapshot. The atomic writer
	// independently refuses leaf links at publication time.
	info, err := os.Lstat(s.path)
	if err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("refuse non-regular OpenCode settings %q: move the symlink or directory aside, place a regular file at this path, then rerun `gentle-ai sync`", s.path)
	}
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("stat OpenCode settings: %w", err)
	}
	custom, err := opencodedefault.DiscoverCustomAgents(s.path)
	if err != nil {
		return fmt.Errorf("discover OpenCode custom agents: %w", err)
	}
	allowed := map[string]bool{"gentle-orchestrator": true, "general": true, "explore": true}
	for _, name := range opencodeactivation.GentleAIODDPhases() {
		allowed[name] = true
	}
	for _, name := range opencodeactivation.JDPhases() {
		allowed[name] = true
	}
	for _, name := range opencodeactivation.ReviewPhases() {
		allowed[name] = true
	}
	for _, name := range custom {
		allowed[name] = true
	}
	agents := make(map[string]any)
	for name, assignment := range s.assignments {
		if !allowed[name] || assignment.ProviderID == "" || assignment.ModelID == "" {
			continue
		}
		entry := map[string]any{"model": assignment.FullID()}
		// Omitting variant on an effortless custom assignment preserves the
		// user's own value; an explicit picker effort may replace it (#3262).
		if assignment.Effort != "" {
			entry["variant"] = assignment.Effort
		}
		agents[name] = entry
	}
	if len(agents) == 0 {
		return nil
	}
	original, err := os.ReadFile(s.path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read OpenCode settings: %w", err)
	}
	if len(original) > 0 {
		if _, err := filemerge.UnmarshalJSONObject(original); err != nil {
			return fmt.Errorf("refuse to change malformed OpenCode settings: %w", err)
		}
	}
	overlay, err := json.Marshal(map[string]any{"agent": agents})
	if err != nil {
		return fmt.Errorf("marshal OpenCode model assignments: %w", err)
	}
	updated, err := filemerge.MergeJSONObjectsForPath(s.path, original, overlay)
	if err != nil {
		return fmt.Errorf("merge OpenCode model assignments: %w", err)
	}
	if bytes.Equal(original, updated) {
		return nil
	}
	mode := os.FileMode(0644)
	if info != nil {
		mode = info.Mode().Perm()
	}
	result, err := filemerge.WriteFileAtomic(s.path, updated, mode)
	// Publication can land before a subsequent durability check fails. Report
	// the landed path so transaction rollback has the correct evidence.
	if result.Changed {
		*s.changedFiles = append(*s.changedFiles, s.path)
	}
	if err != nil {
		return fmt.Errorf("write OpenCode model assignments: %w", err)
	}
	return nil
}

type componentSyncStep struct {
	id           string
	component    model.ComponentID
	homeDir      string
	workspaceDir string
	agents       []model.AgentID
	selection    model.Selection
	changedFiles *[]string // accumulates absolute paths of files that actually changed

	backgroundPolicy bool
}

type codeGraphGuidanceSyncStep struct {
	id           string
	homeDir      string
	runner       communitytool.Runner
	changedFiles *[]string
	before       map[string]syncFileSnapshot
}

type piCodeGraphSyncStep struct {
	id, homeDir, workspaceDir string
	changedFiles              *[]string
}

// openCodePluginRefreshSyncStep refreshes already-installed managed
// OpenCode-compatible plugins (OpenCode, Kilocode) from the embedded assets
// when the SDD component is not part of the sync selection (issue #1440).
// It never creates plugins that were never installed.
type openCodePluginRefreshSyncStep struct {
	id           string
	homeDir      string
	agents       []model.AgentID
	changedFiles *[]string
}

func (s openCodePluginRefreshSyncStep) ID() string { return s.id }

func (s openCodePluginRefreshSyncStep) Run() error {
	for _, adapter := range resolveAdapters(s.agents) {
		if !opencoderuntimeplugins.AgentReceivesManagedOpenCodePlugins(adapter.Agent()) {
			continue
		}
		// The legacy background plugin identifies an older managed installation:
		// migrate it to the retained plugin set rather than treating it as an
		// empty installation. Otherwise refresh only already-installed plugins.
		legacy := filepath.Join(adapter.GlobalConfigDir(s.homeDir), "plugins", "background-agents.ts")
		_, legacyErr := os.Lstat(legacy)
		var res opencoderuntimeplugins.Result
		var err error
		if adapter.Agent() == model.AgentOpenCode && legacyErr == nil {
			res, err = opencoderuntimeplugins.Install(s.homeDir, adapter)
		} else {
			if legacyErr != nil && !os.IsNotExist(legacyErr) {
				return legacyErr
			}
			res, err = opencoderuntimeplugins.Refresh(s.homeDir, adapter)
		}
		if err != nil {
			return fmt.Errorf("sync managed OpenCode plugins: %w", err)
		}
		if s.changedFiles != nil && res.Changed {
			*s.changedFiles = append(*s.changedFiles, res.Files...)
		}
	}
	return nil
}

// anyAgentReceivesManagedOpenCodePlugins reports whether any synced agent
// receives the managed OpenCode-compatible plugins from the SDD injector.
func anyAgentReceivesManagedOpenCodePlugins(agentIDs []model.AgentID) bool {
	for _, id := range agentIDs {
		if opencoderuntimeplugins.AgentReceivesManagedOpenCodePlugins(id) {
			return true
		}
	}
	return false
}

var refreshPiCodeGraphIfConfigured = communitytool.RefreshPiCodeGraphIfConfigured

func (s piCodeGraphSyncStep) ID() string { return s.id }
func (s piCodeGraphSyncStep) Run() error {
	result, configured, err := refreshPiCodeGraphIfConfigured(s.homeDir, s.workspaceDir)
	if err != nil {
		return fmt.Errorf("sync Pi CodeGraph: %w", err)
	}
	if configured && result.Changed && s.changedFiles != nil {
		*s.changedFiles = append(*s.changedFiles, result.Files...)
	}
	return nil
}

func (s *codeGraphGuidanceSyncStep) ID() string {
	return s.id
}

func (s *codeGraphGuidanceSyncStep) Run() (runErr error) {
	before, err := snapshotSyncFiles(communitytool.CodeGraphManagedPaths(s.homeDir))
	if err != nil {
		return err
	}
	s.before = before
	defer func() {
		if runErr != nil {
			runErr = errors.Join(runErr, restoreSyncFiles(s.before))
		}
	}()

	status := communitytool.DetectStatus(model.CommunityToolCodeGraph, s.homeDir, communitytool.DetectorFunc(cmdLookPath))
	if status.CLI == communitytool.AvailabilityAvailable && communitytool.NeedsOpenCodeCodeGraphReconcile(s.homeDir) {
		reconciled, err := communitytool.ReconcileOpenCodeCodeGraph(s.homeDir, s.runner)
		if err != nil {
			return fmt.Errorf("sync OpenCode CodeGraph wiring: %w", err)
		}
		if s.changedFiles != nil && reconciled.Changed {
			*s.changedFiles = append(*s.changedFiles, reconciled.Files...)
		}
	}

	res, configured, err := communitytool.RefreshCodeGraphGuidanceIfConfigured(s.homeDir, communitytool.DetectorFunc(cmdLookPath))
	if err != nil {
		return fmt.Errorf("sync CodeGraph guidance: %w", err)
	}
	if !configured {
		res, err = communitytool.CleanLegacyCodeGraphGuidance(s.homeDir)
		if err != nil {
			return fmt.Errorf("sync legacy CodeGraph guidance cleanup: %w", err)
		}
	}
	if s.changedFiles != nil && res.Changed {
		*s.changedFiles = append(*s.changedFiles, res.Files...)
	}
	return nil
}

func (s *codeGraphGuidanceSyncStep) Rollback() error {
	return restoreSyncFiles(s.before)
}

type codeGraphHomeRunner struct {
	homeDir string
}

func (r codeGraphHomeRunner) Run(name string, args ...string) error {
	command := exec.Command(name, args...)
	system.EnsureCommandDir(command)
	actualHome, _ := os.UserHomeDir()
	if filepath.Clean(r.homeDir) != filepath.Clean(actualHome) {
		command.Env = overrideCommandEnvironment(os.Environ(), map[string]string{
			"HOME":            r.homeDir,
			"XDG_CONFIG_HOME": codeGraphConfigHome(r.homeDir),
		})
	}
	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message != "" {
			return fmt.Errorf("%w: %s", err, message)
		}
	}
	return err
}

func codeGraphConfigHome(homeDir string) string {
	return filepath.Dir(opencodeagent.ConfigPath(homeDir))
}

func overrideCommandEnvironment(environment []string, overrides map[string]string) []string {
	result := make([]string, 0, len(environment)+len(overrides))
	for _, entry := range environment {
		key, _, found := strings.Cut(entry, "=")
		if found {
			if _, overridden := overrides[key]; overridden {
				continue
			}
		}
		result = append(result, entry)
	}
	for key, value := range overrides {
		result = append(result, key+"="+value)
	}
	return result
}

func (s componentSyncStep) ID() string {
	return s.id
}

func (s componentSyncStep) Run() error {
	adapters := resolveAdapters(s.agents)

	switch s.component {
	case model.ComponentEngram:
		// Sync: inject MCP config + system prompt protocol only.
		// NO binary install. NO engram setup.
		//
		// Resolve the installed engram version exactly like the install path
		// (internal/cli/run.go) so InjectOptions.Version feeds the same
		// Decision 1 slim/full gate (bug #1824): without it every sync
		// silently re-inflated the slim Claude Code engram-protocol section
		// back to the full one. Errors are intentionally ignored — an empty
		// version safely falls back to the full section.
		engramVersion, _ := resolveEngramVersion("engram")
		engramOpts := engram.InjectOptions{
			CodexOrchestratorAssignment: s.selection.CodexOrchestratorAssignment,
			CodexCarrilModelAssignments: s.selection.CodexCarrilModelAssignments,
			CodexModelAssignments:       s.selection.CodexModelAssignments,
			Version:                     engramVersion,
		}
		for _, adapter := range adapters {
			engramOpts.OpenCodeSettingsPath = effectiveOpenCodeSettingsPath(s.homeDir, s.workspaceDir, ScopeGlobal, adapter)
			var res engram.InjectionResult
			var err error
			if adapter.Agent() == model.AgentOpenClaw {
				res, err = engram.InjectWithPromptDir(s.homeDir, componentInjectionDir(s.homeDir, s.workspaceDir, adapter), adapter)
			} else {
				targetDir := componentInjectionDir(s.homeDir, s.workspaceDir, adapter)
				res, err = engram.InjectWithOptions(targetDir, adapter, engramOpts)
			}
			if err != nil {
				return fmt.Errorf("sync engram for %q: %w", adapter.Agent(), err)
			}
			s.countChanged(boolToInt(res.Changed), res.Files...)
		}
		return nil

	case model.ComponentContext7:
		for _, adapter := range adapters {
			targetDir := componentInjectionDir(s.homeDir, s.workspaceDir, adapter)
			var res mcp.InjectionResult
			var err error
			if adapter.Agent() == model.AgentOpenCode {
				res, err = mcp.InjectAtSettingsPath(s.homeDir, targetDir, adapter, effectiveOpenCodeSettingsPath(s.homeDir, s.workspaceDir, ScopeGlobal, adapter))
			} else {
				res, err = mcp.Inject(s.homeDir, targetDir, adapter)
			}
			if err != nil {
				return fmt.Errorf("sync context7 for %q: %w", adapter.Agent(), err)
			}
			s.countChanged(boolToInt(res.Changed), res.Files...)
		}
		return nil

	case model.ComponentSDD:
		// Preserve legacy state parsing without resurrecting retired SDD assets.
		return nil

	case model.ComponentSkills:
		skillIDs := selectedSkillIDs(s.selection)
		if len(skillIDs) == 0 {
			return nil
		}
		for _, adapter := range adapters {
			res, err := skills.Inject(componentInjectionDir(s.homeDir, s.workspaceDir, adapter), adapter, skillIDs)
			if err != nil {
				return fmt.Errorf("sync skills for %q: %w", adapter.Agent(), err)
			}
			s.countChanged(boolToInt(res.Changed), res.Files...)
		}
		return nil

	case model.ComponentGGA:
		// Sync: ensure runtime assets are current and inject config.
		// NO binary install.
		if err := gga.EnsureRuntimeAssets(s.homeDir); err != nil {
			return fmt.Errorf("sync gga runtime assets: %w", err)
		}
		if runtime.GOOS == "windows" {
			if err := gga.EnsurePowerShellShim(s.homeDir); err != nil {
				return fmt.Errorf("ensure gga powershell shim: %w", err)
			}
		}
		res, err := gga.Inject(s.homeDir, s.agents)
		if err != nil {
			return fmt.Errorf("sync gga config: %w", err)
		}
		// Count GGA files changed based on individual Changed flags.
		total := boolToInt(res.ConfigChanged) + boolToInt(res.AgentsChanged)
		var ggaFiles []string
		if res.ConfigChanged && res.ConfigFile != "" {
			ggaFiles = append(ggaFiles, res.ConfigFile)
		}
		if res.AgentsChanged && res.AgentsFile != "" {
			ggaFiles = append(ggaFiles, res.AgentsFile)
		}
		s.countChanged(total, ggaFiles...)
		return nil

	case model.ComponentPermission:
		// Opt-in only — reached when --include-permissions is set.
		for _, adapter := range adapters {
			var res permissions.InjectionResult
			var err error
			if adapter.Agent() == model.AgentOpenCode {
				res, err = permissions.InjectAtPath(effectiveOpenCodeSettingsPath(s.homeDir, s.workspaceDir, ScopeGlobal, adapter), adapter)
			} else {
				res, err = permissions.Inject(s.homeDir, adapter)
			}
			if err != nil {
				return fmt.Errorf("sync permissions for %q: %w", adapter.Agent(), err)
			}
			s.countChanged(boolToInt(res.Changed), res.Files...)
		}
		return nil

	case model.ComponentPersona:
		// Sync regenerates the persona block between
		// <!-- gentle-ai:persona --> markers and (when supported) refreshes
		// the Gentleman output-style overlay. We deliberately skip the
		// OpenCode/Kilocode agent definition in opencode.json — that JSON
		// merge conflicts with SDD's writes to the same settings file and
		// remains an install-only concern.
		for _, adapter := range adapters {
			if adapter.Agent() == model.AgentPi {
				res, err := persona.InjectPiPersona(s.homeDir, s.selection.Persona)
				if err != nil {
					return fmt.Errorf("sync persona for %q: %w", adapter.Agent(), err)
				}
				s.countChanged(boolToInt(res.Changed), res.Files...)
				continue
			}
			targetDir := componentInjectionDir(s.homeDir, s.workspaceDir, adapter)
			selectedSettingsPath := ""
			if adapter.Agent() == model.AgentOpenCode {
				selectedSettingsPath = effectiveOpenCodeSettingsPath(s.homeDir, s.workspaceDir, ScopeGlobal, adapter)
			}
			res, err := injectSyncPersona(targetDir, adapter, s.selection.Persona, selectedSettingsPath)
			if err != nil {
				return fmt.Errorf("sync persona for %q: %w", adapter.Agent(), err)
			}
			s.countChanged(boolToInt(res.Changed), res.Files...)
		}
		return nil

	case model.ComponentTheme:
		// Opt-in only — reached when --include-theme is set.
		for _, adapter := range adapters {
			var res theme.InjectionResult
			var err error
			if adapter.Agent() == model.AgentOpenCode {
				res, err = theme.InjectAtPath(effectiveOpenCodeSettingsPath(s.homeDir, s.workspaceDir, ScopeGlobal, adapter))
			} else {
				res, err = theme.Inject(s.homeDir, adapter)
			}
			if err != nil {
				return fmt.Errorf("sync theme for %q: %w", adapter.Agent(), err)
			}
			s.countChanged(boolToInt(res.Changed), res.Files...)
		}
		return nil

	case model.ComponentClaudeTheme:
		for _, adapter := range adapters {
			res, err := theme.InjectVisualThemes(s.homeDir, adapter)
			if err != nil {
				return fmt.Errorf("sync visual themes for %q: %w", adapter.Agent(), err)
			}
			s.countChanged(boolToInt(res.Changed), res.Files...)
		}
		return nil

	case model.ComponentOpenCodeGentleLogo:
		if !containsAgent(s.agents, model.AgentOpenCode) {
			return nil
		}
		res, err := opencodeplugin.Install(s.homeDir, model.OpenCodePluginGentleLogo)
		if err != nil {
			return fmt.Errorf("sync OpenCode Gentle Logo plugin: %w", err)
		}
		s.countChanged(boolToInt(res.Changed), res.Files...)
		return nil

	default:
		return fmt.Errorf("component %q is not supported in sync runtime", s.component)
	}
}

// countChanged records candidate changed paths from an aggregate injector result.
func (s componentSyncStep) countChanged(n int, files ...string) {
	if s.changedFiles != nil && n > 0 {
		*s.changedFiles = append(*s.changedFiles, files...)
	}
}

// dedupPaths removes duplicate and empty paths while preserving first-seen order.
func dedupPaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if strings.TrimSpace(p) == "" {
			continue
		}
		if _, ok := seen[p]; !ok {
			seen[p] = struct{}{}
			out = append(out, p)
		}
	}
	return out
}

type syncFileSnapshot struct {
	exists       bool
	data         []byte
	mode         os.FileMode
	symlink      bool
	linkTarget   string
	targetPath   string
	targetExists bool
}

var writeSyncFileAtomic = filemerge.WriteFileAtomic

func syncRestoreWriteMode(mode os.FileMode) os.FileMode {
	if mode.Perm() == 0 {
		return 0o600
	}
	return mode
}

func snapshotSyncFiles(paths []string) (map[string]syncFileSnapshot, error) {
	snapshots := make(map[string]syncFileSnapshot, len(paths))
	for _, path := range dedupPaths(paths) {
		info, err := os.Lstat(path)
		if err != nil {
			if os.IsNotExist(err) {
				snapshots[path] = syncFileSnapshot{}
				continue
			}
			return nil, fmt.Errorf("inspect managed sync file %q: %w", path, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return nil, fmt.Errorf("read managed sync symlink %q: %w", path, err)
			}
			targetPath := linkTarget
			if !filepath.IsAbs(targetPath) {
				targetPath = filepath.Join(filepath.Dir(path), targetPath)
			}
			targetPath, targetInfo, targetExists, err := resolveSyncSymlinkTarget(targetPath)
			if err != nil {
				return nil, fmt.Errorf("resolve managed sync symlink target for %q: %w", path, err)
			}
			if !targetExists {
				snapshots[path] = syncFileSnapshot{exists: true, symlink: true, linkTarget: linkTarget, targetPath: targetPath}
				continue
			}
			data, err := os.ReadFile(targetPath)
			if err != nil {
				return nil, fmt.Errorf("snapshot managed sync symlink target %q: %w", targetPath, err)
			}
			snapshots[path] = syncFileSnapshot{
				exists:       true,
				data:         data,
				mode:         targetInfo.Mode().Perm(),
				symlink:      true,
				linkTarget:   linkTarget,
				targetPath:   targetPath,
				targetExists: true,
			}
			continue
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("snapshot managed sync file %q: %w", path, err)
		}
		snapshots[path] = syncFileSnapshot{exists: true, data: data, mode: info.Mode().Perm()}
	}
	return snapshots, nil
}

func resolveSyncSymlinkTarget(path string) (string, os.FileInfo, bool, error) {
	current, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", nil, false, err
	}
	seen := map[string]struct{}{}
	for {
		if _, exists := seen[current]; exists {
			return "", nil, false, fmt.Errorf("symlink cycle at %q", current)
		}
		seen[current] = struct{}{}
		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) {
				return current, nil, false, nil
			}
			return "", nil, false, err
		}
		if info.Mode()&os.ModeSymlink == 0 {
			return current, info, true, nil
		}
		next, err := os.Readlink(current)
		if err != nil {
			return "", nil, false, err
		}
		if !filepath.IsAbs(next) {
			next = filepath.Join(filepath.Dir(current), next)
		}
		current = filepath.Clean(next)
	}
}

func restoreSyncFiles(snapshots map[string]syncFileSnapshot) error {
	var restoreErr error
	for path, snapshot := range snapshots {
		if !snapshot.exists {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				restoreErr = errors.Join(restoreErr, fmt.Errorf("remove newly created sync file %q: %w", path, err))
			}
			continue
		}
		mode := snapshot.mode
		writeMode := syncRestoreWriteMode(mode)
		if snapshot.symlink {
			if snapshot.targetExists {
				if _, err := writeSyncFileAtomic(snapshot.targetPath, snapshot.data, writeMode); err != nil {
					restoreErr = errors.Join(restoreErr, fmt.Errorf("restore sync symlink target %q: %w", snapshot.targetPath, err))
					continue
				}
				if err := os.Chmod(snapshot.targetPath, mode); err != nil {
					restoreErr = errors.Join(restoreErr, fmt.Errorf("restore sync symlink target mode %q: %w", snapshot.targetPath, err))
					continue
				}
			} else {
				if err := os.Remove(snapshot.targetPath); err != nil && !os.IsNotExist(err) {
					restoreErr = errors.Join(restoreErr, fmt.Errorf("remove newly created sync symlink target %q: %w", snapshot.targetPath, err))
					continue
				}
			}
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				restoreErr = errors.Join(restoreErr, fmt.Errorf("replace managed sync symlink %q: %w", path, err))
				continue
			}
			if err := os.Symlink(snapshot.linkTarget, path); err != nil {
				restoreErr = errors.Join(restoreErr, fmt.Errorf("restore managed sync symlink %q: %w", path, err))
			}
			continue
		}
		if _, err := writeSyncFileAtomic(path, snapshot.data, writeMode); err != nil {
			restoreErr = errors.Join(restoreErr, fmt.Errorf("restore sync file %q: %w", path, err))
			continue
		}
		if err := os.Chmod(path, mode); err != nil {
			restoreErr = errors.Join(restoreErr, fmt.Errorf("restore sync file mode %q: %w", path, err))
		}
	}
	return restoreErr
}

func changedSyncFiles(candidates []string, before map[string]syncFileSnapshot) ([]string, error) {
	var changed []string
	for _, path := range dedupPaths(candidates) {
		after, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				if before[path].exists {
					changed = append(changed, path)
				}
				continue
			}
			return nil, fmt.Errorf("compare managed sync file %q: %w", path, err)
		}
		previous := before[path]
		if !previous.exists || !bytes.Equal(after, previous.data) {
			changed = append(changed, path)
		}
	}
	return changed, nil
}

// boolToInt converts a boolean to 0 or 1.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// applyResolvedPersona fills selection.Persona when it was not explicitly set.
// It accepts the already-loaded persisted persona string (from state.json)
// so no disk I/O happens inside this function.
//
// Resolution order:
//  1. Explicit: if selection.Persona is non-empty, it is left untouched.
//  2. Persisted: the persisted string is normalized via normalizePersona.
//  3. Fallback: PersonaNeutral for default-safe behavior when the persona field
//     is empty or the state file is absent. Other read/validation errors are
//     rejected by validatePersistedSyncState before this function is called.
func applyResolvedPersona(selection *model.Selection, persisted string) {
	if selection.Persona != "" {
		return
	}
	if persisted != "" {
		if id, _, err := normalizePersona(persisted); err == nil {
			selection.Persona = id
			return
		}
		// The sync entry points reject unknown persisted values before resolution.
	}
	// Default-safe fallback for state files written before persona persistence.
	selection.Persona = model.PersonaNeutral
}

// migratePersistedPersonaAlias rewrites a persisted legacy
// gentleman-neutral-artifacts persona to neutral, printing the remap notice
// once. State that predates persona persistence, explicit gentleman state,
// and unreadable state are untouched.
//
// The persisted parameter is only an advisory snapshot: the rewrite re-reads
// the latest state inside the canonical install-state lock and re-checks the
// alias there, so a concurrent writer's change between the advisory read and
// the write is never clobbered.
func migratePersistedPersonaAlias(homeDir string, persisted *state.InstallState, persistedErr error) error {
	if persistedErr != nil || persisted == nil || persisted.Persona != string(model.PersonaGentlemanNeutralArtifacts) {
		return nil
	}
	remapped := false
	if err := withInstallStateLock(homeDir, func() error {
		latest, readErr := state.Read(homeDir)
		if readErr != nil {
			// The advisory caller read succeeded, so a missing file now means a
			// concurrent delete removed the state: nothing is left to migrate.
			if os.IsNotExist(readErr) {
				return nil
			}
			return fmt.Errorf("read persisted installation state: %w", readErr)
		}
		if latest.Persona != string(model.PersonaGentlemanNeutralArtifacts) {
			return nil
		}
		latest.Persona = string(model.PersonaNeutral)
		if err := state.Write(homeDir, latest); err != nil {
			return fmt.Errorf("persist remapped persona: %w", err)
		}
		remapped = true
		return nil
	}); err != nil {
		return err
	}
	// Notice only after the rewrite is durably persisted: a failed write must
	// not tell the user the remap happened.
	if remapped {
		fmt.Fprintln(personaNoticeWriter, personaAliasRemapNotice)
	}
	return nil
}

// validatePersistedSyncState rejects state that cannot safely drive sync.
// A missing state file is allowed for fresh homes; a decoded state without a
// persona remains compatible with legacy installations.
func validatePersistedSyncState(persisted state.InstallState, readErr error) error {
	// guard:population persisted-sync-state-integrity fail-closed: legitimate persisted sync state is a missing file or decoded state with an empty or supported persona; read/decode errors, whitespace-only values, and unsupported persona values remain excluded
	if readErr != nil {
		if os.IsNotExist(readErr) {
			return nil
		}
		return fmt.Errorf("read persisted installation state: %w", readErr)
	}

	if persisted.Persona == "" {
		if persisted.PersonaPresent {
			return fmt.Errorf("validate persisted persona: explicitly empty persona is not valid") // refusal:by-design operator-knowledge: only the operator can choose the intended persona to replace malformed persisted state
		}
		return nil
	}
	if strings.TrimSpace(persisted.Persona) == "" {
		return fmt.Errorf("validate persisted persona: whitespace-only persona is not valid") // refusal:by-design operator-knowledge: only the operator can choose the intended persona to replace malformed persisted state
	}
	if _, _, err := normalizePersona(persisted.Persona); err != nil {
		return fmt.Errorf("validate persisted persona: %w", err)
	}
	return nil
}

// RunSyncWithSelection is the programmatic entry point for sync.
// It skips flag parsing and agent discovery — the caller provides the homeDir
// and a fully-built Selection (agents + components + options).
// This is the function the TUI calls directly to avoid CLI flag parsing.
func RunSyncWithSelection(homeDir string, selection model.Selection) (SyncResult, error) {
	persistedState, persistedStateErr := state.Read(homeDir)
	if persistedStateErr != nil && !os.IsNotExist(persistedStateErr) {
		return SyncResult{Agents: selection.Agents, Selection: selection}, fmt.Errorf("read persisted installation state: %w", persistedStateErr)
	}
	background, err := resolveOpenCodeBackgroundCLI(false, "", persistedState)
	if err != nil {
		return SyncResult{Agents: selection.Agents, Selection: selection}, err
	}
	background.activationPlan, err = prepareOpenCodeBackgroundActivation(homeDir, &background, containsAgent(selection.Agents, model.AgentOpenCode))
	if err != nil {
		return SyncResult{Agents: selection.Agents, Selection: selection, Background: background}, fmt.Errorf("prepare OpenCode background activation: %w", err)
	}
	piBackground, err := resolvePiBackgroundCLI(false, "", persistedState)
	if err != nil {
		return SyncResult{Agents: selection.Agents, Selection: selection, Background: background}, err
	}
	preparePiBackgroundProjection(homeDir, &piBackground, containsAgent(selection.Agents, model.AgentPi))
	return runSyncWithSelection(homeDir, selection, background, piBackground)
}

var syncStagePlan = func(runtime *syncRuntime) pipeline.StagePlan { return runtime.stagePlan() }
var compareChangedSyncFiles = changedSyncFiles

func runSyncWithSelection(homeDir string, selection model.Selection, background OpenCodeBackgroundResolution, piBackground PiBackgroundResolution) (SyncResult, error) {
	agentIDs := selection.Agents
	// The read error is captured, not discarded: the persona alias migration
	// below must not rewrite state it could not read. Managed-asset provenance
	// re-reads under its own lock later (#2685), so this read stays advisory.
	persistedState, persistedStateErr := state.Read(homeDir)
	if err := validatePersistedSyncState(persistedState, persistedStateErr); err != nil {
		return SyncResult{Agents: agentIDs, Selection: selection}, err
	}
	restorePersistedCommunityTools(homeDir, &selection, persistedState)

	// Resolve persona from persisted state when the caller has not provided one.
	// RunSync already resolves persona before delegating here, so on the CLI path
	// selection.Persona is already set and applyResolvedPersona early-returns with
	// no disk read. On the TUI path the Selection has an empty Persona field, so
	// we read state once here and apply the persisted value (or neutral fallback).
	if selection.Persona == "" {
		var persistedPersona string
		persistedPersona = persistedState.Persona
		applyResolvedPersona(&selection, persistedPersona)
	}
	if len(selection.ModelAssignments) == 0 && len(persistedState.ModelAssignments) > 0 {
		workspaceDir, _ := os.Getwd()
		selection.ModelAssignments = restoreOpenCodeModelAssignmentsFromState(homeDir, workspaceDir, ScopeGlobal, persistedState, selection.SDDMode)
	}

	// Migrate a persisted legacy alias BEFORE any early return: a no-agent
	// no-op sync and a failing pipeline must still leave state.json remapped,
	// otherwise the one-time migration never fires for those users. State
	// records intent — the next sync applies the neutral assets.
	if err := migratePersistedPersonaAlias(homeDir, &persistedState, persistedStateErr); err != nil {
		return SyncResult{Agents: agentIDs, Selection: selection}, err
	}

	result := SyncResult{
		Agents:       agentIDs,
		Selection:    selection,
		Background:   background,
		PiBackground: piBackground,
	}

	result, noOp, err := zeroAgentSyncNoOp(homeDir, selection, result)
	if err != nil || noOp {
		return result, err
	}

	rt, err := newSyncRuntime(homeDir, selection)
	if err != nil {
		return result, err
	}
	defer rt.state.cleanupCompatibilityTransaction()
	defer rt.state.cleanupRollbackSnapshot()
	rt.backgroundActivation = background.activationPlan
	if rt.backgroundActivation != nil {
		rt.runtimeReady = rt.backgroundActivation.Capability().Ready()
		rt.backgroundPolicy = rt.runtimeReady && background.Effective == model.OpenCodeBackgroundOn
	} else {
		// Preserve the programmatic/TUI seam's historical behavior. CLI sync
		// supplies an activation plan; direct callers do not.
		rt.backgroundPolicy = background.Effective == model.OpenCodeBackgroundOn
	}
	rt.piBackgroundProjection = piBackground.projectionPlan

	stagePlan := syncStagePlan(rt)
	result.Plan = stagePlan
	before, err := snapshotSyncFiles(rt.managedPaths)
	if err != nil {
		return result, err
	}

	orchestrator := pipeline.NewOrchestrator(pipeline.DefaultRollbackPolicy())
	result.Execution = orchestrator.Execute(stagePlan)
	compatibilityChanged := rt.state.compatibilityChangedFiles()
	if result.Execution.Err != nil {
		return result, fmt.Errorf("execute sync pipeline: %w", result.Execution.Err)
	}
	result.ManualActions = append(result.ManualActions, rt.state.nativeReviewActions...)

	// Capture how many managed assets were actually changed.
	// Deduplicate paths — multiple components may touch the same file
	// (e.g. Engram and Context7 both merge into settings.json).
	result.ChangedFiles, err = compareChangedSyncFiles(rt.changedFiles, before)
	if err != nil {
		return result, rollbackPostApplyError(orchestrator, result.Execution, err)
	}
	result.ChangedFiles = dedupPaths(append(result.ChangedFiles, compatibilityChanged...))
	if background.activationPlan != nil {
		result.ChangedFiles = dedupPaths(append(result.ChangedFiles, background.activationPlan.ChangedPaths()...))
	}
	if piBackground.projectionPlan != nil {
		result.ChangedFiles = dedupPaths(append(result.ChangedFiles, piBackground.projectionPlan.ChangedPaths()...))
	}
	result.FilesChanged = len(result.ChangedFiles)

	// True no-op: agents were discovered but all managed assets were already
	// current — no file was written or updated. Per spec scenario:
	// "No managed assets to sync — system completes without modifying files
	// and reports that no managed sync actions were needed."
	if result.FilesChanged == 0 {
		result.NoOp = true
	}

	// Post-apply verification reuses the same component paths as install.
	result.Verify = runPostSyncVerification(homeDir, rt.workspaceDir, selection)
	configChecks := verify.RunChecks(context.Background(), openCodeConfigChecks(homeDir, rt.workspaceDir, agentIDs))
	result.Verify = verify.BuildReport(append(result.Verify.Checks, configChecks...))
	result.Verify = withFailedSyncVerificationNote(result.Verify)
	result.BackgroundPolicyEnabled = rt.runtimeReady && background.Effective == model.OpenCodeBackgroundOn
	if background.activationPlan != nil {
		result.Background.Activation = background.activationPlan.Report()
	}
	result.Verify = withOpenCodeBackgroundPending(result.Verify, background, rt.runtimeReady, agentIDs)
	if !result.Verify.Ready {
		verificationErr := fmt.Errorf("post-sync verification failed:\n%s", verify.RenderReport(result.Verify))
		rollback := orchestrator.Rollback(result.Execution)
		if rollback.Err != nil {
			verificationErr = errors.Join(verificationErr, rollback.Err)
		}
		return result, verificationErr
	}
	writer, err := deriveManagedAssetWriter()
	if err != nil {
		return result, rollbackPostApplyError(orchestrator, result.Execution, fmt.Errorf("derive managed asset writer identity: %w", err))
	}
	if err := persistSyncManagedAssetStateWithBackground(homeDir, selection, writer, background.Persist, piBackground.Persist); err != nil {
		persistErr := fmt.Errorf("persist sync managed asset state: %w", err)
		rollback := orchestrator.Rollback(result.Execution)
		if rollback.Err != nil {
			persistErr = errors.Join(persistErr, rollback.Err)
		}
		return result, persistErr
	}

	return result, nil
}

func persistSyncManagedAssetStateWithBackground(homeDir string, selection model.Selection, writer string, background model.OpenCodeBackgroundIntent, piBackground model.PiBackgroundIntent) error {
	return withInstallStateLock(homeDir, func() error {
		latest, err := state.Read(homeDir)
		if errors.Is(err, os.ErrNotExist) {
			latest = state.InstallState{}
		} else if err != nil {
			return fmt.Errorf(
				"read install state for managed asset provenance: %w; run `gentle-ai install` to rewrite %s",
				err, state.Path(homeDir))
		}

		shouldWrite := false
		// #2685: stamp the binary version that performed this sync, so doctor
		// can report managed assets older than the running binary instead of
		// the user discovering the skew mid-review at START preflight.
		if latest.InstalledBinaryVersion != AppVersion {
			latest.InstalledBinaryVersion = AppVersion
			shouldWrite = true
		}
		if latest.ManagedAssetDigest != writer {
			latest.ManagedAssetDigest = writer
			shouldWrite = true
		}
		if !latest.CommunityToolsConfigured && selection.CommunityTools != nil {
			latest.CommunityTools = communityToolIDsToStrings(selection.CommunityTools)
			latest.CommunityToolsConfigured = true
			shouldWrite = true
		}
		if background != "" && latest.BackgroundIntent != background {
			latest.BackgroundIntent = background
			shouldWrite = true
		}
		if piBackground != "" && latest.PiBackgroundIntent != piBackground {
			latest.PiBackgroundIntent = piBackground
			shouldWrite = true
		}
		now := time.Now().UTC()
		latest.LastSyncedAt = &now
		shouldWrite = true
		if !shouldWrite {
			return nil
		}
		if err := state.WriteReconciled(homeDir, latest); err != nil {
			return fmt.Errorf("persist managed asset provenance: %w", err)
		}
		return nil
	})
}

// RunSync is the top-level sync entry point, parallel to RunInstall.
// It parses CLI flags, discovers agents, builds the selection, then delegates
// to RunSyncWithSelection for the actual sync execution.
func RunSync(args []string) (SyncResult, error) {
	flags, err := ParseSyncFlags(args)
	if err != nil {
		return SyncResult{}, err
	}

	homeDir, err := osUserHomeDir()
	if err != nil {
		return SyncResult{}, fmt.Errorf("resolve user home directory: %w", err)
	}

	// Resolve agents: explicit flag takes precedence over auto-discovery.
	var agentIDs []model.AgentID
	if len(flags.Agents) > 0 {
		parsed, err := asAgentIDs(flags.Agents)
		if err != nil {
			return SyncResult{}, err
		}
		agentIDs = parsed
	} else {
		agentIDs = DiscoverAgents(homeDir)
	}
	agentIDs = unique(agentIDs)

	selection := BuildSyncSelection(flags, agentIDs)

	// Read state once for both model-assignment restoration and persona resolution.
	// A missing state file is treated as a fresh home; other read/validation
	// errors stop sync before any persona mutation or asset write.
	persistedState, persistedStateErr := state.Read(homeDir)
	if err := validatePersistedSyncState(persistedState, persistedStateErr); err != nil {
		return SyncResult{Agents: agentIDs, Selection: selection}, err
	}
	background, err := resolveOpenCodeBackgroundCLI(flags.OpenCodeBackgroundSubagentsSet, flags.OpenCodeBackgroundSubagents, persistedState)
	if err != nil {
		return SyncResult{Agents: agentIDs, Selection: selection}, err
	}
	piBackground, err := resolvePiBackgroundCLI(flags.PiBackgroundSubagentsSet, flags.PiBackgroundSubagents, persistedState)
	if err != nil {
		return SyncResult{Agents: agentIDs, Selection: selection}, err
	}
	RestorePersistedSelection(&selection, persistedState, flags)
	restorePersistedCommunityTools(homeDir, &selection, persistedState)

	// Load persisted model assignments from state when not provided via flags.
	// Without this, every CLI sync falls back to defaults and would silently
	// overwrite the user's model choices.
	if len(selection.ClaudePhaseAssignments) == 0 && len(persistedState.ClaudePhaseAssignments) > 0 {
		m := make(map[string]model.ClaudePhaseAssignment, len(persistedState.ClaudePhaseAssignments))
		for k, v := range persistedState.ClaudePhaseAssignments {
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
	if len(selection.ClaudeModelAssignments) == 0 && len(selection.ClaudePhaseAssignments) == 0 && len(persistedState.ClaudeModelAssignments) > 0 {
		m := make(map[string]model.ClaudeModelAlias, len(persistedState.ClaudeModelAssignments))
		for k, v := range persistedState.ClaudeModelAssignments {
			// Claude Code controls the main session/orchestrator model itself.
			// Keep persisted assignments scoped to Agent tool calls only.
			if k == "orchestrator" {
				continue
			}
			m[k] = model.ClaudeModelAlias(v)
		}
		selection.ClaudeModelAssignments = m
	}
	if len(selection.KiroModelAssignments) == 0 && len(persistedState.KiroModelAssignments) > 0 {
		m := make(map[string]model.KiroModelAlias, len(persistedState.KiroModelAssignments))
		for k, v := range persistedState.KiroModelAssignments {
			m[k] = model.KiroModelAlias(v)
		}
		selection.KiroModelAssignments = m
	}
	if len(selection.ModelAssignments) == 0 && len(persistedState.ModelAssignments) > 0 {
		workspaceDir, _ := os.Getwd()
		selection.ModelAssignments = restoreOpenCodeModelAssignmentsFromState(homeDir, workspaceDir, ScopeGlobal, persistedState, selection.SDDMode)
	}
	if selection.CodexOrchestratorAssignment == nil && persistedState.CodexOrchestratorAssignment != nil {
		selection.CodexOrchestratorAssignment = codexOrchestratorFromState(persistedState.CodexOrchestratorAssignment)
	}

	// Restore Codex effort and carril model assignments from state so that
	// `gentle-ai sync` preserves the user's per-phase effort and per-carril
	// model choices instead of falling back to canonical defaults every time.
	// This mirrors the TUI path (loadPersistedAssignments in app.go).
	if len(selection.CodexModelAssignments) == 0 && len(persistedState.CodexModelAssignments) > 0 {
		m := make(map[string]model.CodexEffort, len(persistedState.CodexModelAssignments))
		for k, v := range persistedState.CodexModelAssignments {
			m[k] = model.CodexEffort(v)
		}
		selection.CodexModelAssignments = m
	}
	if len(selection.CodexCarrilModelAssignments) == 0 && len(persistedState.CodexCarrilModelAssignments) > 0 {
		selection.CodexCarrilModelAssignments = model.MigrateLegacyCodexCarrilDefaults(persistedState.CodexCarrilModelAssignments)
	}
	if len(selection.CodexPhaseModelAssignments) == 0 && len(persistedState.CodexPhaseModelAssignments) > 0 {
		m := make(map[string]string, len(persistedState.CodexPhaseModelAssignments))
		for k, v := range persistedState.CodexPhaseModelAssignments {
			m[k] = v
		}
		selection.CodexPhaseModelAssignments = m
	}

	// Resolve persona from the already-read state. This covers both the dry-run
	// branch (which returns early) and the normal path (which delegates to
	// RunSyncWithSelection — that function's early-return guard prevents a second
	// disk read on the CLI path).
	applyResolvedPersona(&selection, persistedState.Persona)

	if flags.DryRun {
		// Build the plan for inspection, skip execution.
		result := SyncResult{
			Agents:       agentIDs,
			Selection:    selection,
			DryRun:       true,
			Background:   background,
			PiBackground: piBackground,
		}
		result, noOp, err := zeroAgentSyncNoOp(homeDir, selection, result)
		if err != nil || noOp {
			return result, err
		}
		rt, err := newSyncRuntime(homeDir, selection)
		if err != nil {
			return result, err
		}
		defer rt.state.cleanupCompatibilityTransaction()
		backgroundActivation, activationErr := prepareOpenCodeBackgroundActivation(homeDir, &background, containsAgent(agentIDs, model.AgentOpenCode))
		if activationErr != nil {
			return result, fmt.Errorf("prepare OpenCode background activation: %w", activationErr)
		}
		background.activationPlan = backgroundActivation
		rt.backgroundActivation = backgroundActivation
		rt.runtimeReady = backgroundActivation != nil && backgroundActivation.Capability().Ready()
		rt.backgroundPolicy = rt.runtimeReady && background.Effective == model.OpenCodeBackgroundOn
		result.Background = background
		result.BackgroundPolicyEnabled = rt.backgroundPolicy
		rt.piBackgroundProjection = preparePiBackgroundProjection(homeDir, &piBackground, containsAgent(agentIDs, model.AgentPi))
		result.PiBackground = piBackground
		result.Plan = rt.stagePlan()
		for _, step := range result.Plan.Prepare {
			if prepare, ok := step.(prepareBackupStep); ok && prepare.targetErr != nil {
				return result, fmt.Errorf("resolve backup targets: %w", prepare.targetErr)
			}
		}
		return result, nil
	}

	backgroundActivation, err := prepareOpenCodeBackgroundActivation(homeDir, &background, containsAgent(agentIDs, model.AgentOpenCode))
	if err != nil {
		return SyncResult{Agents: agentIDs, Selection: selection, Background: background}, fmt.Errorf("prepare OpenCode background activation: %w", err)
	}
	background.activationPlan = backgroundActivation
	preparePiBackgroundProjection(homeDir, &piBackground, containsAgent(agentIDs, model.AgentPi))
	result, err := runSyncWithSelection(homeDir, selection, background, piBackground)
	if err != nil {
		return result, err
	}
	result.DryRun = false
	_ = telemetry.IncrementSyncs(homeDir)
	TelemetryTrigger(homeDir)
	return result, nil
}

func restoreOpenCodeModelAssignmentsFromState(homeDir, workspaceDir string, scope InstallScope, persistedState state.InstallState, sddMode model.SDDModeID) map[string]model.ModelAssignment {
	if len(persistedState.ModelAssignments) == 0 {
		return nil
	}
	presence := map[string]opencodeactivation.AssignmentPresence{}
	settingsPath := effectiveOpenCodeSettingsPath(homeDir, workspaceDir, scope, opencodeagent.NewAdapter())
	if settingsPath != "" {
		if _, err := os.Stat(settingsPath); err == nil {
			snapshot, err := opencodeactivation.ReadConfigSnapshot(settingsPath)
			if err == nil {
				presence = snapshot.Assignments
			}
		}
	}

	assignments := make(map[string]model.ModelAssignment, len(persistedState.ModelAssignments))
	for k, v := range persistedState.ModelAssignments {
		if current, exists := presence[k]; exists && current.Present {
			// Single mode intentionally generates managed agents without model fields,
			// so their absence is not evidence of a user clear. Multi mode writes
			// assignments into managed agents, making an absent/empty value explicit.
			if current.Cleared && !(sddMode == model.SDDModeSingle && current.Managed) {
				continue
			}
			if current.Assignment.ProviderID != "" || current.Assignment.ModelID != "" {
				continue
			}
		}
		assignments[k] = model.ModelAssignment{ProviderID: v.ProviderID, ModelID: v.ModelID, Effort: v.Effort}
	}
	if len(assignments) == 0 {
		return nil
	}
	return assignments
}

// zeroAgentSyncNoOp reports whether a sync without agents has no compatible
// shared-skill work to perform.
func zeroAgentSyncNoOp(homeDir string, selection model.Selection, result SyncResult) (SyncResult, bool, error) {
	if len(result.Agents) != 0 {
		return result, false, nil
	}
	refreshable, err := compatibilitySkillsRefreshable(homeDir, selection)
	if err != nil {
		return result, false, err
	}
	if refreshable {
		return result, false, nil
	}
	result.NoOp = true
	return result, true, nil
}

func restorePersistedCommunityTools(homeDir string, selection *model.Selection, persisted state.InstallState) {
	if selection.CommunityTools != nil {
		return
	}
	if persisted.CommunityToolsConfigured {
		selection.CommunityTools = make([]model.CommunityToolID, 0, len(persisted.CommunityTools))
		for _, tool := range persisted.CommunityTools {
			switch model.CommunityToolID(tool) {
			case model.CommunityToolCodeGraph:
				selection.CommunityTools = append(selection.CommunityTools, model.CommunityToolCodeGraph)
			}
		}
		return
	}
	if communitytool.HasManagedCodeGraphGuidance(homeDir) || hasManagedPiCodeGraphManifest(homeDir) {
		selection.CommunityTools = []model.CommunityToolID{model.CommunityToolCodeGraph}
	}
}

func hasManagedPiCodeGraphManifest(homeDir string) bool {
	path := communitytool.PiCodeGraphManifestPath(homeDir)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var manifest struct {
		MCPPath string `json:"mcpPath"`
		MCP     *struct {
			AfterHash string `json:"afterHash"`
		} `json:"mcp"`
	}
	return json.Unmarshal(data, &manifest) == nil && filepath.IsAbs(manifest.MCPPath) && manifest.MCP != nil && manifest.MCP.AfterHash != ""
}

// RenderSyncReport renders a human-readable summary of a sync execution.
//
// Unlike verify.RenderReport (which shows verification check statuses), this
// function reports the managed sync actions that were executed — matching the
// spec requirement to surface "what was done" rather than "what was checked".
//
// No-op cases:
//   - No agents were discovered or specified (NoOp=true, Agents empty).
//   - All managed assets were already current (NoOp=true, FilesChanged=0).
func RenderSyncReport(result SyncResult) string {
	var b strings.Builder
	backgroundReport := func() {
		for _, check := range result.Verify.Checks {
			if check.Status == verify.CheckStatusWarning {
				fmt.Fprintf(&b, "WARNING: %s\n", check.Error)
			}
		}
		if containsAgent(result.Agents, model.AgentPi) && result.PiBackground.Intent != "" {
			fmt.Fprintf(&b, "Pi background intent: %s (policy effective: %s)\n", result.PiBackground.Intent, result.PiBackground.Effective)
			if !result.PiBackground.managed {
				fmt.Fprintln(&b, "Pi background projection: unmanaged (no explicit policy)")
			} else if plan := result.PiBackground.projectionPlan; plan != nil && plan.skipReason != "" {
				fmt.Fprintln(&b, "Pi background projection skipped: "+plan.skipReason)
			}
		}
		if !containsAgent(result.Agents, model.AgentOpenCode) || result.Background.Intent == "" {
			return
		}
		fmt.Fprintf(&b, "OpenCode background intent: %s (policy effective: %s)\n", result.Background.Intent, result.Background.Effective)
		if result.Background.Effective == model.OpenCodeBackgroundOn {
			fmt.Fprintf(&b, "OpenCode background runtime ready: %t\n", result.BackgroundPolicyEnabled)
			fmt.Fprintln(&b, renderOpenCodeBackgroundActivation(result.Background))
		} else if result.Background.Effective == model.OpenCodeBackgroundOff && len(result.Background.Activation.LauncherPaths) > 0 {
			fmt.Fprintln(&b, renderOpenCodeBackgroundActivation(result.Background))
		}
	}

	if result.NoOp {
		fmt.Fprintln(&b, "gentle-ai sync — no managed sync actions needed")
		if len(result.Agents) == 0 {
			fmt.Fprintln(&b, "No agents were discovered or specified. Nothing to sync.")
		} else {
			fmt.Fprintf(&b, "Agents: %s\n", joinAgentIDs(result.Agents))
			if len(result.ManualActions) == 0 {
				fmt.Fprintln(&b, "All managed assets are already up to date. No files changed.")
			} else {
				fmt.Fprintln(&b, "No managed files changed; preserved native agents were not updated.")
			}
		}
		backgroundReport()
		renderSyncManualActions(&b, result.ManualActions)
		return strings.TrimRight(b.String(), "\n")
	}

	if result.DryRun {
		fmt.Fprintln(&b, "gentle-ai sync — dry-run")
		fmt.Fprintf(&b, "Agents: %s\n", joinAgentIDs(result.Agents))

		compParts := make([]string, 0, len(result.Selection.Components))
		for _, c := range result.Selection.Components {
			compParts = append(compParts, string(c))
		}
		if len(compParts) > 0 {
			fmt.Fprintf(&b, "Managed components: %s\n", strings.Join(compParts, ", "))
		}
		fmt.Fprintf(&b, "Prepare steps: %d\n", len(result.Plan.Prepare))
		fmt.Fprintf(&b, "Apply steps: %d\n", len(result.Plan.Apply))
		backgroundReport()
		return strings.TrimRight(b.String(), "\n")
	}

	fmt.Fprintln(&b, "gentle-ai sync — managed sync executed")
	fmt.Fprintf(&b, "Agents synced: %s\n", joinAgentIDs(result.Agents))

	compParts := make([]string, 0, len(result.Selection.Components))
	for _, c := range result.Selection.Components {
		compParts = append(compParts, string(c))
	}
	if len(compParts) > 0 {
		fmt.Fprintf(&b, "Managed components synced: %s\n", strings.Join(compParts, ", "))
	}

	// Report actual files changed — not the count of successful pipeline steps.
	// FilesChanged is 0 only when all assets were already current (no-op path
	// above handles that case). A non-zero value here reflects real writes.
	fmt.Fprintf(&b, "Sync actions executed: %d files changed\n", result.FilesChanged)

	if len(result.ChangedFiles) > 0 {
		for _, path := range result.ChangedFiles {
			fmt.Fprintf(&b, "  - %s\n", path)
		}
	}

	if !result.Verify.Ready {
		fmt.Fprintln(&b, "")
		fmt.Fprintln(&b, "Post-sync verification:")
		fmt.Fprint(&b, verify.RenderReport(result.Verify))
	}
	backgroundReport()
	renderSyncManualActions(&b, result.ManualActions)

	return strings.TrimRight(b.String(), "\n")
}

func renderSyncManualActions(b *strings.Builder, actions []string) {
	if len(actions) == 0 {
		return
	}
	fmt.Fprintln(b, "Manual actions required:")
	for _, action := range actions {
		fmt.Fprintf(b, "- %s\n", action)
	}
}

// withFailedSyncVerificationNote replaces the generic
// verify.VerificationIssuesMessage with one naming the concrete command that
// retries a failed sync: `gentle-ai sync`. Unlike the install path, sync has
// no per-agent retry command -- rerunning `gentle-ai sync` re-applies every
// discovered/persisted agent, so no agent list is needed.
//
// It is scoped to exactly the generic failure text so it never clobbers a
// FinalNote that was already customized, mirroring
// withFailedVerificationNote's install-path guard.
func withFailedSyncVerificationNote(report verify.Report) verify.Report {
	if report.Ready || report.FinalNote != verify.VerificationIssuesMessage {
		return report
	}
	report.FinalNote = verify.VerificationIssuesMessageForCommand("gentle-ai sync")
	return report
}

// runPostSyncVerification verifies that managed files exist after sync.
func runPostSyncVerification(homeDir, workspaceDir string, selection model.Selection) verify.Report {
	checks := make([]verify.Check, 0)
	adapters := resolveAdapters(selection.Agents)

	for _, component := range selection.Components {
		if component == model.ComponentSDD {
			// Legacy state remains readable, but retired assets are not installed.
			continue
		}
		paths := syncComponentPathsWithWorkspace(homeDir, workspaceDir, selection, adapters, component)
		if component == model.ComponentEngram {
			paths = verificationComponentPaths(homeDir, workspaceDir, ScopeGlobal, selection, adapters, component)
		}
		for _, path := range paths {
			currentPath := path
			if isRetiredManagedPath(currentPath) {
				checks = append(checks, verify.Check{
					ID:          "verify:sync:file:" + currentPath,
					Description: "retired managed file removed",
					Run: func(context.Context) error {
						if _, err := os.Stat(currentPath); err != nil {
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
				ID:          "verify:sync:file:" + currentPath,
				Description: "synced file exists",
				Run: func(context.Context) error {
					if _, err := os.Stat(currentPath); err != nil {
						return err
					}
					return nil
				},
			})
		}
	}
	for _, adapter := range adapters {
		if !opencoderuntimeplugins.AgentReceivesManagedOpenCodePlugins(adapter.Agent()) {
			continue
		}
		pluginsDir := filepath.Join(adapter.GlobalConfigDir(homeDir), "plugins")
		legacyPath := filepath.Join(pluginsDir, opencoderuntimeplugins.LegacyOpenCodeReviewPluginName)
		checks = append(checks, verify.Check{
			ID:          "verify:sync:file:" + legacyPath,
			Description: "legacy OpenCode review plugin removed",
			Run: func(context.Context) error {
				if _, err := os.Lstat(legacyPath); err == nil {
					return fmt.Errorf("legacy OpenCode review plugin still exists; rerun `gentle-ai sync` to complete the managed plugin migration")
				} else if !os.IsNotExist(err) {
					return err
				}
				return nil
			},
		})
	}

	return verify.BuildReport(verify.RunChecks(context.Background(), checks))
}
