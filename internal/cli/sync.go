package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/agents"
	"github.com/gentleman-programming/gentle-ai/internal/backup"
	"github.com/gentleman-programming/gentle-ai/internal/components/engram"
	"github.com/gentleman-programming/gentle-ai/internal/components/gga"
	"github.com/gentleman-programming/gentle-ai/internal/components/mcp"
	"github.com/gentleman-programming/gentle-ai/internal/components/permissions"
	"github.com/gentleman-programming/gentle-ai/internal/components/sdd"
	"github.com/gentleman-programming/gentle-ai/internal/components/skills"
	"github.com/gentleman-programming/gentle-ai/internal/components/theme"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/pipeline"
	"github.com/gentleman-programming/gentle-ai/internal/state"
	"github.com/gentleman-programming/gentle-ai/internal/verify"
)

// SyncFlags holds parsed CLI flags for the sync command.
type SyncFlags struct {
	Agents             []string
	Skills             []string
	SDDMode            string
	IncludePermissions bool
	IncludeTheme       bool
	DryRun             bool
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
	// FilesChanged is the number of managed files actually written or updated
	// during this sync. Zero means all assets were already current.
	FilesChanged int
}

// ParseSyncFlags parses the CLI arguments for the sync subcommand.
func ParseSyncFlags(args []string) (SyncFlags, error) {
	var opts SyncFlags

	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.SetOutput(ioDiscard{})
	registerListFlag(fs, "agent", &opts.Agents)
	registerListFlag(fs, "agents", &opts.Agents)
	registerListFlag(fs, "skill", &opts.Skills)
	registerListFlag(fs, "skills", &opts.Skills)
	fs.StringVar(&opts.SDDMode, "sdd-mode", "", "SDD orchestrator mode: single or multi (default: single)")
	fs.BoolVar(&opts.IncludePermissions, "include-permissions", false, "include permissions component in sync")
	fs.BoolVar(&opts.IncludeTheme, "include-theme", false, "include theme component in sync")
	fs.BoolVar(&opts.DryRun, "dry-run", false, "preview plan without executing")

	if err := fs.Parse(args); err != nil {
		return SyncFlags{}, err
	}

	if fs.NArg() > 0 {
		return SyncFlags{}, fmt.Errorf("unexpected sync argument %q", fs.Arg(0))
	}

	return opts, nil
}

// BuildSyncSelection builds a model.Selection for the sync command.
//
// Default sync scope: SDD, Engram, Context7, GGA, Skills.
// Excluded by default: Persona, Permissions, Theme (user-config-adjacent).
// Permissions and Theme can be opted-in via flags.
//
// This is the reusable managed-asset sync contract. A future `upgrade --sync`
// flow can call this function to get the same managed-only selection semantics.
func BuildSyncSelection(flags SyncFlags, agentIDs []model.AgentID) model.Selection {
	components := []model.ComponentID{
		model.ComponentSDD,
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
		Agents:     agentIDs,
		Components: components,
		SDDMode:    sddMode,
		Skills:     skillIDs,
		// Preset is set to full-gentleman so selectedSkillIDs() returns the
		// correct default skill set when no explicit skills are provided.
		Preset: model.PresetFullGentleman,
	}
}

// DiscoverAgents returns the agent IDs to sync.
//
// Discovery order:
//  1. Persisted state (~/.gentle-ai/state.json) — written at install time.
//     When present and non-empty, only the agents the user explicitly installed
//     are returned. This prevents sync from injecting into every IDE config dir
//     that happens to exist on the system (issue #107).
//  2. Filesystem fallback — delegates to agents.DiscoverInstalled with the
//     default registry. Used when state.json is absent (users who installed
//     before state persistence was added) or empty.
//
// When --agents is provided explicitly, callers should pass those IDs directly
// instead of calling DiscoverAgents.
func DiscoverAgents(homeDir string) []model.AgentID {
	// Try reading persisted state first.
	s, err := state.Read(homeDir)
	if err == nil && len(s.InstalledAgents) > 0 {
		ids := make([]model.AgentID, 0, len(s.InstalledAgents))
		for _, a := range s.InstalledAgents {
			ids = append(ids, model.AgentID(a))
		}
		return ids
	}

	// Fallback: filesystem discovery (backward compat for users who installed
	// before state persistence was added).
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
	homeDir      string
	workspaceDir string
	selection    model.Selection
	agentIDs     []model.AgentID
	backupRoot   string
	state        *runtimeState
	filesChanged int // accumulates changed-file count across all component steps
}

func newSyncRuntime(homeDir string, selection model.Selection) (*syncRuntime, error) {
	backupRoot := filepath.Join(homeDir, ".gentle-ai", "backups")
	if err := os.MkdirAll(backupRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create backup root directory %q: %w", backupRoot, err)
	}

	workspaceDir, _ := os.Getwd()

	return &syncRuntime{
		homeDir:      homeDir,
		workspaceDir: workspaceDir,
		selection:    selection,
		agentIDs:     selection.Agents,
		backupRoot:   backupRoot,
		state:        &runtimeState{},
	}, nil
}

func (r *syncRuntime) stagePlan() pipeline.StagePlan {
	adapters := resolveAdapters(r.agentIDs)
	targets := syncBackupTargets(r.homeDir, r.selection, adapters)

	prepare := []pipeline.Step{
		prepareBackupStep{
			id:          "prepare:backup-snapshot",
			snapshotter: backup.NewSnapshotter(),
			snapshotDir: filepath.Join(r.backupRoot, time.Now().UTC().Format("20060102150405.000000000")),
			targets:     targets,
			state:       r.state,
			source:      backup.BackupSourceSync,
			description: "pre-sync snapshot",
			appVersion:  AppVersion,
		},
	}

	apply := []pipeline.Step{
		rollbackRestoreStep{id: "apply:rollback-restore", state: r.state},
	}

	for _, component := range r.selection.Components {
		apply = append(apply, componentSyncStep{
			id:           "sync:component:" + string(component),
			component:    component,
			homeDir:      r.homeDir,
			workspaceDir: r.workspaceDir,
			agents:       r.agentIDs,
			selection:    r.selection,
			filesChanged: &r.filesChanged,
		})
	}

	return pipeline.StagePlan{Prepare: prepare, Apply: apply}
}

// syncBackupTargets returns the file paths that need to be backed up
// before sync executes. Uses the same componentPaths logic as install.
func syncBackupTargets(homeDir string, selection model.Selection, adapters []agents.Adapter) []string {
	paths := map[string]struct{}{}
	for _, component := range selection.Components {
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

// componentSyncStep is the sync-specific apply step.
// Unlike componentApplyStep, it ONLY calls inject functions —
// no binary install, no engram setup, no persona injection.
//
// filesChanged is a shared counter pointer. Each step increments it by the
// number of files that were actually written (i.e., whose content changed).
// This lets RunSync detect a true no-op when all assets are already current.
type componentSyncStep struct {
	id           string
	component    model.ComponentID
	homeDir      string
	workspaceDir string
	agents       []model.AgentID
	selection    model.Selection
	filesChanged *int
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
		for _, adapter := range adapters {
			res, err := engram.Inject(s.homeDir, adapter)
			if err != nil {
				return fmt.Errorf("sync engram for %q: %w", adapter.Agent(), err)
			}
			s.countChanged(boolToInt(res.Changed))
		}
		return nil

	case model.ComponentContext7:
		for _, adapter := range adapters {
			res, err := mcp.Inject(s.homeDir, adapter)
			if err != nil {
				return fmt.Errorf("sync context7 for %q: %w", adapter.Agent(), err)
			}
			s.countChanged(boolToInt(res.Changed))
		}
		return nil

	case model.ComponentSDD:
		for _, adapter := range adapters {
			opts := sdd.InjectOptions{
				OpenCodeModelAssignments: s.selection.ModelAssignments,
				ClaudeModelAssignments:   s.selection.ClaudeModelAssignments,
				WorkspaceDir:             s.workspaceDir,
			}
			res, err := sdd.Inject(s.homeDir, adapter, s.selection.SDDMode, opts)
			if err != nil {
				return fmt.Errorf("sync sdd for %q: %w", adapter.Agent(), err)
			}
			s.countChanged(boolToInt(res.Changed))
		}
		return nil

	case model.ComponentSkills:
		skillIDs := selectedSkillIDs(s.selection)
		if len(skillIDs) == 0 {
			return nil
		}
		for _, adapter := range adapters {
			res, err := skills.Inject(s.homeDir, adapter, skillIDs)
			if err != nil {
				return fmt.Errorf("sync skills for %q: %w", adapter.Agent(), err)
			}
			s.countChanged(boolToInt(res.Changed))
		}
		return nil

	case model.ComponentGGA:
		// Sync: ensure runtime assets are current and inject config.
		// NO binary install.
		if err := gga.EnsureRuntimeAssets(s.homeDir); err != nil {
			return fmt.Errorf("sync gga runtime assets: %w", err)
		}
		res, err := gga.Inject(s.homeDir, s.agents)
		if err != nil {
			return fmt.Errorf("sync gga config: %w", err)
		}
		// Count GGA files changed based on individual Changed flags.
		s.countChanged(boolToInt(res.ConfigChanged) + boolToInt(res.AgentsChanged))
		return nil

	case model.ComponentPermission:
		// Opt-in only — reached when --include-permissions is set.
		for _, adapter := range adapters {
			res, err := permissions.Inject(s.homeDir, adapter)
			if err != nil {
				return fmt.Errorf("sync permissions for %q: %w", adapter.Agent(), err)
			}
			s.countChanged(boolToInt(res.Changed))
		}
		return nil

	case model.ComponentTheme:
		// Opt-in only — reached when --include-theme is set.
		for _, adapter := range adapters {
			res, err := theme.Inject(s.homeDir, adapter)
			if err != nil {
				return fmt.Errorf("sync theme for %q: %w", adapter.Agent(), err)
			}
			s.countChanged(boolToInt(res.Changed))
		}
		return nil

	default:
		// Persona and any unknown components are out of sync scope.
		return fmt.Errorf("component %q is not supported in sync runtime", s.component)
	}
}

// countChanged adds n to the shared filesChanged counter (nil-safe).
func (s componentSyncStep) countChanged(n int) {
	if s.filesChanged != nil && n > 0 {
		*s.filesChanged += n
	}
}

// boolToInt converts a boolean to 0 or 1.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// RunSyncWithSelection is the programmatic entry point for sync.
// It skips flag parsing and agent discovery — the caller provides the homeDir
// and a fully-built Selection (agents + components + options).
// This is the function the TUI calls directly to avoid CLI flag parsing.
func RunSyncWithSelection(homeDir string, selection model.Selection) (SyncResult, error) {
	agentIDs := selection.Agents

	result := SyncResult{
		Agents:    agentIDs,
		Selection: selection,
	}

	// No-op path: no agents were discovered or provided.
	// Per spec: "No managed assets to sync — system completes without modifying
	// unrelated files and reports that no managed sync actions were needed."
	if len(agentIDs) == 0 {
		result.NoOp = true
		return result, nil
	}

	rt, err := newSyncRuntime(homeDir, selection)
	if err != nil {
		return result, err
	}

	stagePlan := rt.stagePlan()
	result.Plan = stagePlan

	orchestrator := pipeline.NewOrchestrator(pipeline.DefaultRollbackPolicy())
	result.Execution = orchestrator.Execute(stagePlan)
	if result.Execution.Err != nil {
		return result, fmt.Errorf("execute sync pipeline: %w", result.Execution.Err)
	}

	// Capture how many managed assets were actually changed.
	result.FilesChanged = rt.filesChanged

	// True no-op: agents were discovered but all managed assets were already
	// current — no file was written or updated. Per spec scenario:
	// "No managed assets to sync — system completes without modifying files
	// and reports that no managed sync actions were needed."
	if result.FilesChanged == 0 {
		result.NoOp = true
	}

	// Post-apply verification reuses the same component paths as install.
	result.Verify = runPostSyncVerification(homeDir, selection)
	if !result.Verify.Ready {
		return result, fmt.Errorf("post-sync verification failed:\n%s", verify.RenderReport(result.Verify))
	}

	return result, nil
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
		agentIDs = asAgentIDs(flags.Agents)
	} else {
		agentIDs = DiscoverAgents(homeDir)
	}
	agentIDs = unique(agentIDs)

	selection := BuildSyncSelection(flags, agentIDs)

	if flags.DryRun {
		// Build the plan for inspection, skip execution.
		result := SyncResult{
			Agents:    agentIDs,
			Selection: selection,
			DryRun:    true,
		}
		if len(agentIDs) == 0 {
			result.NoOp = true
			return result, nil
		}
		rt, err := newSyncRuntime(homeDir, selection)
		if err != nil {
			return result, err
		}
		result.Plan = rt.stagePlan()
		return result, nil
	}

	result, err := RunSyncWithSelection(homeDir, selection)
	if err != nil {
		return result, err
	}
	result.DryRun = false
	return result, nil
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

	if result.NoOp {
		fmt.Fprintln(&b, "gentle-ai sync — no managed sync actions needed")
		if len(result.Agents) == 0 {
			fmt.Fprintln(&b, "No agents were discovered or specified. Nothing to sync.")
		} else {
			fmt.Fprintf(&b, "Agents: %s\n", joinAgentIDs(result.Agents))
			fmt.Fprintln(&b, "All managed assets are already up to date. No files changed.")
		}
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

	if !result.Verify.Ready {
		fmt.Fprintln(&b, "")
		fmt.Fprintln(&b, "Post-sync verification:")
		fmt.Fprint(&b, verify.RenderReport(result.Verify))
	}

	return strings.TrimRight(b.String(), "\n")
}

// runPostSyncVerification verifies that managed files exist after sync.
func runPostSyncVerification(homeDir string, selection model.Selection) verify.Report {
	checks := make([]verify.Check, 0)
	adapters := resolveAdapters(selection.Agents)

	for _, component := range selection.Components {
		for _, path := range componentPaths(homeDir, selection, adapters, component) {
			currentPath := path
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

	return verify.BuildReport(verify.RunChecks(context.Background(), checks))
}
