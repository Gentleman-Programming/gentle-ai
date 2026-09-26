package uninstall

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/gentleman-programming/gentle-ai/v3/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v3/internal/agents/claude"
	"github.com/gentleman-programming/gentle-ai/v3/internal/agents/pi"
	"github.com/gentleman-programming/gentle-ai/v3/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v3/internal/backup"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/agentguidance"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/communitytool"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/engram"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/gga"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/opencodedefault"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/opencoderuntimeplugins"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/skills"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/telemetryruntime"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/theme"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
	opencodeactivation "github.com/gentleman-programming/gentle-ai/v3/internal/opencode"
	"github.com/gentleman-programming/gentle-ai/v3/internal/state"
	"github.com/gentleman-programming/gentle-ai/v3/internal/statecoord"
)

type Manager interface {
	PartialUninstall(agentIDs []model.AgentID, componentIDs []model.ComponentID) (Result, error)
	CompleteUninstall() (Result, error)
}

type Snapshotter interface {
	Create(snapshotDir string, paths []string) (backup.Manifest, error)
}

type Result struct {
	Manifest                         backup.Manifest
	BackupPath                       string
	ChangedFiles                     []string
	RemovedFiles                     []string
	RemovedDirectories               []string
	RetainedPiResources              []string
	OptionalPiPackageCleanupCommands []string
	ManualActions                    []string
	AgentsRemovedFromState           []model.AgentID
	// FailedAgents lists the agents whose cleanup did not complete. They are
	// deliberately left in state.json so the recorded state keeps matching the
	// disk, and each one is named in ManualActions with the command that
	// retries it.
	FailedAgents []model.AgentID
}

type Service struct {
	homeDir      string
	workspaceDir string
	backupRoot   string
	appVersion   string
	snapshotter  Snapshotter
	registry     *agents.Registry
	now          func() time.Time

	// engramUninstallScope controls whether Engram cleanup removes global
	// integration files/config (global) or project-local .engram data only.
	engramUninstallScope model.EngramUninstallScope
}

type opType int

const (
	opRewriteFile opType = iota
	opRemoveFile
	opRemoveTree
	opRemoveIfEmpty
)

var (
	allManagedComponents = []model.ComponentID{
		model.ComponentPersona,
		model.ComponentEngram,
		model.ComponentContext7,
		model.ComponentPermission,
		model.ComponentSkills,
		model.ComponentTheme,
		model.ComponentClaudeTheme,
		model.ComponentOpenCodeGentleLogo,
		model.ComponentGGA,
	}
	fullAgentRemovalComponents = []model.ComponentID{
		model.ComponentPersona,
		model.ComponentEngram,
		model.ComponentContext7,
		model.ComponentPermission,
		model.ComponentSkills,
		model.ComponentTheme,
		model.ComponentClaudeTheme,
		model.ComponentOpenCodeGentleLogo,
	}
)

type operation struct {
	typeID opType
	path   string
	// agents records which agents' cleanup contributed this operation. It is
	// what turns a flat operation list into a per-agent commit boundary: an
	// operation that fails only blocks the agents that own it. An operation
	// nobody owns blocks the whole batch, because nothing distinguishes whose
	// cleanup it was.
	agents []model.AgentID
	apply  func(path string) (changed bool, removed bool, err error)
}

// operationFailure records one operation that did not complete, so the run can
// keep going and still report exactly what was left undone.
type operationFailure struct {
	path   string
	agents []model.AgentID
	err    error
}

func NewService(homeDir, workspaceDir, appVersion string) (*Service, error) {
	registry, err := agents.NewDefaultRegistry()
	if err != nil {
		return nil, fmt.Errorf("create adapter registry: %w", err)
	}

	backupRoot := filepath.Join(homeDir, ".gentle-ai", "backups")
	if err := os.MkdirAll(backupRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create backup root %q: %w", backupRoot, err)
	}

	return &Service{
		homeDir:              homeDir,
		workspaceDir:         workspaceDir,
		backupRoot:           backupRoot,
		appVersion:           appVersion,
		snapshotter:          backup.NewSnapshotter(),
		registry:             registry,
		now:                  time.Now,
		engramUninstallScope: model.EngramUninstallScopeGlobal,
	}, nil
}

func PartialUninstall(homeDir, workspaceDir, appVersion string, agentIDs []string, componentIDs []string) (Result, error) {
	svc, err := NewService(homeDir, workspaceDir, appVersion)
	if err != nil {
		return Result{}, err
	}

	agentsTyped := make([]model.AgentID, 0, len(agentIDs))
	for _, agentID := range agentIDs {
		agentsTyped = append(agentsTyped, model.AgentID(agentID))
	}

	componentsTyped := make([]model.ComponentID, 0, len(componentIDs))
	for _, componentID := range componentIDs {
		componentsTyped = append(componentsTyped, model.ComponentID(componentID))
	}

	return svc.PartialUninstall(agentsTyped, componentsTyped)
}

func PartialUninstallWithProfileSelection(homeDir, workspaceDir, appVersion string, agentIDs []string, componentIDs []string, profileNames []string, engramScope model.EngramUninstallScope) (Result, error) {
	svc, err := NewService(homeDir, workspaceDir, appVersion)
	if err != nil {
		return Result{}, err
	}

	agentsTyped := make([]model.AgentID, 0, len(agentIDs))
	for _, agentID := range agentIDs {
		agentsTyped = append(agentsTyped, model.AgentID(agentID))
	}

	componentsTyped := make([]model.ComponentID, 0, len(componentIDs))
	for _, componentID := range componentIDs {
		componentsTyped = append(componentsTyped, model.ComponentID(componentID))
	}

	return svc.PartialUninstallWithProfiles(agentsTyped, componentsTyped, profileNames, engramScope)
}

func CompleteUninstall(homeDir, workspaceDir, appVersion string) (Result, error) {
	svc, err := NewService(homeDir, workspaceDir, appVersion)
	if err != nil {
		return Result{}, err
	}
	return svc.CompleteUninstall()
}

func (s *Service) PartialUninstall(agentIDs []model.AgentID, componentIDs []model.ComponentID) (Result, error) {
	s.engramUninstallScope = model.EngramUninstallScopeGlobal

	return s.partialUninstall(agentIDs, componentIDs)
}

func (s *Service) PartialUninstallWithProfiles(agentIDs []model.AgentID, componentIDs []model.ComponentID, profileNames []string, engramScope model.EngramUninstallScope) (Result, error) {
	// Profiles only applied to the retired SDD component; component validation
	// rejects that request before planning and preserves existing profiles.
	_ = profileNames
	s.SetEngramUninstallScope(engramScope)
	defer func() {
		s.engramUninstallScope = model.EngramUninstallScopeGlobal
	}()

	return s.partialUninstall(agentIDs, componentIDs)
}

// partialUninstall is the shared body of PartialUninstall and
// PartialUninstallWithProfiles. It resolves the requested components,
// downgrades any that are still shared with an agent this run is not
// removing (see reconcileSharedComponents), and executes the resulting plan.
func (s *Service) partialUninstall(agentIDs []model.AgentID, componentIDs []model.ComponentID) (Result, error) {
	if slices.Contains(componentIDs, model.ComponentSDD) {
		return Result{}, fmt.Errorf("component %q is retired and cannot be uninstalled; existing files and settings were preserved", model.ComponentSDD)
	}
	if len(agentIDs) == 0 {
		return Result{}, fmt.Errorf("partial uninstall requires at least one agent")
	}

	// explicitGGA tracks whether the caller itself named "gga" (as opposed to
	// it only being present because the component list defaulted to
	// allManagedComponents). It decides whether an unreadable install state
	// fails the whole command or is merely downgraded to a kept-GGA note.
	explicitGGA := slices.Contains(componentIDs, model.ComponentGGA)

	components := componentIDs
	if len(components) == 0 {
		components = slices.Clone(allManagedComponents)
	}
	components = expandVisualPolishUninstallComponents(components)

	components, sharedNote, err := s.reconcileSharedComponents(agentIDs, components, explicitGGA)
	if err != nil {
		return Result{}, err
	}

	plan, err := s.buildPlan(agentIDs, components)
	if err != nil {
		return Result{}, err
	}

	stateRemovals := stateAgentsToRemove(agentIDs, components)
	result, err := s.executePlan(plan, stateRemovals)
	if err != nil {
		return result, err
	}
	if sharedNote != "" {
		result.ManualActions = append(result.ManualActions, sharedNote)
	}
	return result, nil
}

// reconcileSharedComponents downgrades components that are shared across all
// installed agents (currently just GGA's global config) when at least one
// agent outside agentIDs is still installed. Ownership is derived from the
// persisted install state (state.json's InstalledAgents) rather than probing
// disk, so it stays correct even when files were hand-edited (#3534).
//
// A missing state file is treated as "no other agent is recorded", matching
// the pre-existing behavior of an install predating state.json. Any other
// read failure (corrupt JSON, permission error, ...) fails closed toward
// preservation: GGA is kept and a note explains why, and the rest of the
// uninstall still proceeds — unless the caller explicitly asked for
// "--components gga", in which case there is nothing safe left to do for
// that explicit request and the error is returned instead.
func (s *Service) reconcileSharedComponents(agentIDs []model.AgentID, componentIDs []model.ComponentID, explicitGGA bool) ([]model.ComponentID, string, error) {
	if !slices.Contains(componentIDs, model.ComponentGGA) {
		return componentIDs, "", nil
	}

	remaining, err := otherInstalledAgents(s.homeDir, agentIDs)
	if err != nil {
		if explicitGGA {
			return nil, "", fmt.Errorf("check remaining installed agents for shared GGA config: %w", err)
		}
		note := fmt.Sprintf(
			"No action needed: kept the shared GGA config (%s) because the install state could not be read (%v), so it is unclear whether another installed agent still depends on it.",
			gga.ConfigPath(s.homeDir), err,
		)
		return withoutComponent(componentIDs, model.ComponentGGA), note, nil
	}
	if len(remaining) == 0 {
		return componentIDs, "", nil
	}

	note := fmt.Sprintf(
		"No action needed: kept the shared GGA config (%s) because installed agent(s) still use it: %s. It is removed automatically once those agents are also uninstalled.",
		gga.ConfigPath(s.homeDir), strings.Join(remaining, ", "),
	)
	return withoutComponent(componentIDs, model.ComponentGGA), note, nil
}

// withoutComponent returns componentIDs with every occurrence of excluded
// removed, preserving order.
func withoutComponent(componentIDs []model.ComponentID, excluded model.ComponentID) []model.ComponentID {
	kept := make([]model.ComponentID, 0, len(componentIDs))
	for _, componentID := range componentIDs {
		if componentID != excluded {
			kept = append(kept, componentID)
		}
	}
	return kept
}

// isStateNotFound reports whether err indicates the install state file does
// not exist. It uses errors.Is against fs.ErrNotExist (rather than
// os.IsNotExist) so a caller that wraps state.Read's error — e.g. via
// fmt.Errorf's %w — is still classified correctly instead of falling through
// to the "unreadable state" path.
func isStateNotFound(err error) bool {
	return errors.Is(err, fs.ErrNotExist)
}

// otherInstalledAgents returns the sorted IDs of agents recorded as installed
// in state.json that are not in agentIDs. It answers "which agents remain
// installed after this uninstall" for components shared across every
// installed agent.
func otherInstalledAgents(homeDir string, agentIDs []model.AgentID) ([]string, error) {
	current, err := state.Read(homeDir)
	if err != nil {
		if isStateNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	removing := make(map[string]struct{}, len(agentIDs))
	for _, agentID := range agentIDs {
		removing[string(agentID)] = struct{}{}
	}

	remaining := make([]string, 0, len(current.InstalledAgents))
	for _, installed := range current.InstalledAgents {
		if _, ok := removing[installed]; ok {
			continue
		}
		remaining = append(remaining, installed)
	}
	slices.Sort(remaining)
	return remaining, nil
}

func expandVisualPolishUninstallComponents(components []model.ComponentID) []model.ComponentID {
	shouldExpand := false
	visualPolish := model.VisualPolishComponents()
	for _, component := range components {
		if component != model.ComponentClaudeTheme && slices.Contains(visualPolish, component) {
			shouldExpand = true
		}
	}
	if !shouldExpand {
		return components
	}

	expanded := slices.Clone(components)
	for _, component := range model.VisualPolishComponents() {
		if !slices.Contains(expanded, component) {
			expanded = append(expanded, component)
		}
	}
	return expanded
}

func (s *Service) SetEngramUninstallScope(scope model.EngramUninstallScope) {
	if scope == model.EngramUninstallScopeProject {
		s.engramUninstallScope = model.EngramUninstallScopeProject
		return
	}
	s.engramUninstallScope = model.EngramUninstallScopeGlobal
}

func (s *Service) CompleteUninstall() (Result, error) {
	s.engramUninstallScope = model.EngramUninstallScopeGlobal

	allAgents := s.registry.SupportedAgents()
	plan, err := s.buildPlan(allAgents, allManagedComponents)
	if err != nil {
		return Result{}, err
	}
	result, err := s.executePlan(plan, allAgents)
	if err != nil {
		return result, err
	}

	result.ManualActions = append(result.ManualActions, "To completely remove gentle-ai from your system, delete the executable (e.g., rm -f $(which gentle-ai))")
	return result, nil
}

type plan struct {
	backupTargets []string
	operations    []operation
}

func (s *Service) buildPlan(agentIDs []model.AgentID, componentIDs []model.ComponentID) (plan, error) {
	if slices.Contains(componentIDs, model.ComponentSDD) {
		return plan{}, fmt.Errorf("component %q is retired and cannot be uninstalled; existing files and settings were preserved", model.ComponentSDD)
	}
	// Fail closed before component planning can select a different JSON/JSONC file.
	if slices.Contains(agentIDs, model.AgentOpenCode) {
		path, exists, err := opencodeactivation.GlobalAuthorityState(s.homeDir)
		if err != nil {
			if !os.IsNotExist(err) {
				return plan{}, fmt.Errorf("preflight OpenCode write authority: %w", err)
			}
			// File doesn't exist is acceptable for uninstall planning.
		} else if exists && path != "" {
			// File exists and is valid; ensure it's backed up for removal.
		}
	}
	backupTargets := map[string]struct{}{}
	operationsByKey := map[string]operation{}

	for _, agentID := range agentIDs {
		adapter, ok := s.registry.Get(agentID)
		if !ok {
			return plan{}, fmt.Errorf("unsupported agent %q", agentID)
		}

		for _, componentID := range componentIDs {
			ops, targets, err := s.componentOperations(adapter, componentID)
			if err != nil {
				return plan{}, fmt.Errorf("plan uninstall for %q/%q: %w", agentID, componentID, err)
			}
			for _, target := range targets {
				files, err := expandBackupTarget(target)
				if err != nil {
					return plan{}, fmt.Errorf("expand backup target %q: %w", target, err)
				}
				for _, file := range files {
					backupTargets[file] = struct{}{}
				}
			}
			for _, op := range ops {
				op.agents = []model.AgentID{agentID}
				key := operationKey(op)
				existing, ok := operationsByKey[key]
				if !ok {
					operationsByKey[key] = op
					continue
				}
				if op.typeID == opRewriteFile {
					// Merge rewrite operations on the same file so both
					// mutations apply (e.g. persona + engram on system prompt).
					operationsByKey[key] = mergeRewriteOps(existing, op)
					continue
				}
				// Deduplicated non-rewrite operation: one apply still stands
				// for every agent that asked for it, so ownership widens.
				existing.agents = appendUniqueAgents(existing.agents, op.agents...)
				operationsByKey[key] = existing
			}
		}
	}

	for _, target := range globalBackupTargets(s.homeDir) {
		files, err := expandBackupTarget(target)
		if err != nil {
			return plan{}, fmt.Errorf("expand backup target %q: %w", target, err)
		}
		for _, file := range files {
			backupTargets[file] = struct{}{}
		}
	}

	backupTargets[state.Path(s.homeDir)] = struct{}{}
	if slices.Contains(agentIDs, model.AgentPi) {
		for _, target := range communitytool.PiCodeGraphPaths(s.homeDir, s.workspaceDir) {
			backupTargets[target] = struct{}{}
		}
		if piAdapter, ok := s.registry.Get(model.AgentPi); ok {
			backupTargets[piAdapter.SystemPromptFile(s.homeDir)] = struct{}{}
		}
	}
	if removesAllAgentComponents(componentIDs) {
		for _, agentID := range agentIDs {
			adapter, _ := s.registry.Get(agentID)
			var path string
			switch agentID {
			case model.AgentClaudeCode:
				path = adapter.SettingsPath(s.homeDir)
			case model.AgentCodex:
				path = filepath.Join(adapter.GlobalConfigDir(s.homeDir), "hooks.json")
			}
			if path != "" {
				backupTargets[path] = struct{}{}
				op := rewriteSkillRegistryHook(path)
				op.agents = []model.AgentID{agentID}
				key := operationKey(op)
				if existing, ok := operationsByKey[key]; ok {
					op = mergeRewriteOps(existing, op)
				}
				operationsByKey[key] = op
			}
		}
	}
	if slices.Contains(agentIDs, model.AgentOpenCode) && removesAllAgentComponents(componentIDs) {
		adapter, _ := s.registry.Get(model.AgentOpenCode)
		// Only a complete OpenCode removal rolls back defaults owned by an
		// earlier install. Snapshot the record alongside settings before writes.
		for _, path := range settingsTargets(s.homeDir, adapter) {
			defaultPlan, err := opencodedefault.PrepareUninstall(path)
			if err != nil {
				return plan{}, err
			}
			backupTargets[path] = struct{}{}
			backupTargets[opencodedefault.OwnershipPath(path)] = struct{}{}
			op := operation{typeID: opRewriteFile, path: path, agents: []model.AgentID{model.AgentOpenCode}, apply: func(path string) (bool, bool, error) {
				raw, err := os.ReadFile(path)
				if err != nil && !os.IsNotExist(err) {
					return false, false, err
				}
				return defaultPlan.Apply(raw, err == nil)
			}}
			key := operationKey(op)
			if existing, ok := operationsByKey[key]; ok {
				op = mergeRewriteOps(op, existing)
			}
			operationsByKey[key] = op
		}
		configDir := adapter.GlobalConfigDir(s.homeDir)
		authorityFile, authorityExists, err := opencodeactivation.GlobalAuthorityState(s.homeDir)
		if err != nil {
			return plan{}, fmt.Errorf("preflight OpenCode write authority: %w", err)
		}
		if authorityExists {
			authorityBytes, err := os.ReadFile(authorityFile)
			if err != nil {
				return plan{}, err
			}
			backupTargets[authorityFile] = struct{}{}
			operationsByKey[operationKey(operation{typeID: opRemoveFile, path: authorityFile})] = operation{
				typeID: opRemoveFile, path: authorityFile, agents: []model.AgentID{model.AgentOpenCode},
				apply: func(path string) (bool, bool, error) {
					info, err := os.Lstat(path)
					if err != nil {
						return false, false, err
					}
					if !info.Mode().IsRegular() {
						return false, false, fmt.Errorf("OpenCode write authority is not regular: %s", path)
					}
					var beforeStat syscall.Stat_t
					if err := syscall.Stat(path, &beforeStat); err != nil {
						return false, false, fmt.Errorf("stat OpenCode write authority: %w", err)
					}
					current, err := os.ReadFile(path)
					if err != nil {
						return false, false, err
					}
					if !bytes.Equal(current, authorityBytes) {
						return false, false, fmt.Errorf("OpenCode write authority changed before removal: %s", path)
					}
					// Detect concurrent replacement: a file replaced with
					// identical content gets a new inode, which the previous
					// bytes.Equal check would miss.
					var afterStat syscall.Stat_t
					if err := syscall.Stat(path, &afterStat); err != nil {
						return false, false, fmt.Errorf("stat OpenCode write authority after read: %w", err)
					}
					if afterStat.Ino != beforeStat.Ino || afterStat.Dev != beforeStat.Dev {
						return false, false, fmt.Errorf("OpenCode write authority replaced concurrently: %s", path)
					}
					if err := os.Remove(path); err != nil {
						return false, false, err
					}
					return true, true, nil
				},
			}
		}
		if err := telemetryruntime.CheckManaged(configDir); err != nil {
			return plan{}, err
		}
		for _, op := range removeOwnedTelemetryRuntime(configDir) {
			backupTargets[op.path] = struct{}{}
			operationsByKey[operationKey(op)] = op
		}
		for _, op := range retainedOpenCodePluginOperations(adapter, s.homeDir) {
			backupTargets[op.path] = struct{}{}
			operationsByKey[operationKey(op)] = op
		}
		for _, path := range opencodeactivation.LauncherPaths(s.homeDir, runtime.GOOS) {
			backupTargets[path] = struct{}{}
			operationsByKey[operationKey(removeOwnedOpenCodeLauncher(path))] = removeOwnedOpenCodeLauncher(path)
		}
	}

	orderedTargets := make([]string, 0, len(backupTargets))
	for target := range backupTargets {
		orderedTargets = append(orderedTargets, target)
	}
	slices.Sort(orderedTargets)

	operations := make([]operation, 0, len(operationsByKey))
	for _, op := range operationsByKey {
		operations = append(operations, op)
	}
	slices.SortFunc(operations, compareOperations)

	return plan{backupTargets: orderedTargets, operations: operations}, nil
}

func (s *Service) executePlan(p plan, agentsToRemove []model.AgentID) (Result, error) {
	snapshotDir := filepath.Join(s.backupRoot, s.now().UTC().Format("20060102150405.000000000"))
	manifest, err := s.snapshotter.Create(snapshotDir, p.backupTargets)
	if err != nil {
		return Result{}, fmt.Errorf("create uninstall snapshot: %w", err)
	}

	manifest.Source = backup.BackupSourceUninstall
	manifest.Description = "pre-uninstall snapshot"
	manifest.CreatedByVersion = s.appVersion
	if err := backup.WriteManifest(filepath.Join(snapshotDir, backup.ManifestFilename), manifest); err != nil {
		return Result{}, fmt.Errorf("write uninstall manifest metadata: %w", err)
	}

	result := Result{
		Manifest:   manifest,
		BackupPath: snapshotDir,
	}
	// A batch has no per-agent transaction, so aborting at the first failure
	// left later agents untouched, state.json stale, and no record of how far
	// the run got. Every operation is attempted, failures are attributed to the
	// agents that own them, and only agents with no failed operation commit
	// their state removal.
	failures := make([]operationFailure, 0)

	// Pi ownership hashes include the shared MCP file. Remove its managed entry
	// before other component cleanup (notably Engram) mutates that file, otherwise
	// an unrelated mutation is indistinguishable from user drift.
	if slices.Contains(agentsToRemove, model.AgentPi) {
		piResult, piErr := communitytool.UninstallPiCodeGraph(s.homeDir)
		if piErr != nil {
			failures = append(failures, operationFailure{
				path:   firstOrEmpty(communitytool.PiCodeGraphPaths(s.homeDir, s.workspaceDir)),
				agents: []model.AgentID{model.AgentPi},
				err:    fmt.Errorf("remove Pi CodeGraph integration: %w", piErr),
			})
		} else {
			result.ChangedFiles = append(result.ChangedFiles, piResult.Files...)
			result.ManualActions = append(result.ManualActions, piResult.ManualActions...)
		}

		// Pi's SupportsSystemPrompt() gate keeps componentOperations() from
		// ever queuing a rewrite for its SystemPromptFile, so a stale
		// gentle-ai block left there by an older install is never cleaned up
		// by the generic persona rewrite ops above. Retire it directly.
		if piAdapter, ok := s.registry.Get(model.AgentPi); ok {
			promptPath := piAdapter.SystemPromptFile(s.homeDir)
			retireResult, retireErr := agentguidance.RetirePiSystemPromptBlocks(s.homeDir, piAdapter)
			if retireErr != nil {
				failures = append(failures, operationFailure{
					path:   promptPath,
					agents: []model.AgentID{model.AgentPi},
					err:    fmt.Errorf("retire stale Pi system prompt blocks: %w", retireErr),
				})
			} else if retireResult.Changed {
				if _, statErr := os.Stat(promptPath); os.IsNotExist(statErr) {
					result.RemovedFiles = append(result.RemovedFiles, promptPath)
				} else {
					result.ChangedFiles = append(result.ChangedFiles, retireResult.Files...)
				}
			}
		}
	}

	// Keep selector authority until state update completes; it is stored here
	// and removed below, after updateStateAfterUninstall, so that the sidecar
	// still exists when the state update fails.
	var authorityRemoval *operation
	for _, op := range p.operations {
		if op.typeID == opRemoveFile && filepath.Base(op.path) == ".gentle-ai-opencode-write-authority.json" && slices.Contains(op.agents, model.AgentOpenCode) {
			copy := op
			authorityRemoval = &copy
			continue
		}
		changed, removed, err := op.apply(op.path)
		if err != nil {
			failures = append(failures, operationFailure{path: op.path, agents: op.agents, err: err})
			continue
		}
		if op.typeID == opRemoveIfEmpty && !removed {
			if note, ok := manualActionForNonEmptyDirectory(op.path); ok {
				result.ManualActions = append(result.ManualActions, note)
			}
		}
		if !changed {
			continue
		}
		switch op.typeID {
		case opRewriteFile:
			result.ChangedFiles = append(result.ChangedFiles, op.path)
		case opRemoveFile:
			if removed {
				result.RemovedFiles = append(result.RemovedFiles, op.path)
			}
		case opRemoveTree, opRemoveIfEmpty:
			if removed {
				result.RemovedDirectories = append(result.RemovedDirectories, op.path)
			}
		}
	}

	if slices.Contains(agentsToRemove, model.AgentPi) {
		result.RetainedPiResources = retainedPiResources(s.homeDir, s.workspaceDir)
		result.OptionalPiPackageCleanupCommands = optionalPiPackageCleanupCommands()
	}

	result.FailedAgents = failedAgents(failures, agentsToRemove)
	result.ManualActions = append(result.ManualActions, failureManualActions(failures, agentsToRemove, s.homeDir)...)

	committable := make([]model.AgentID, 0, len(agentsToRemove))
	for _, agentID := range agentsToRemove {
		if !slices.Contains(result.FailedAgents, agentID) {
			committable = append(committable, agentID)
		}
	}

	removed, stateErr := updateStateAfterUninstall(s.homeDir, committable)
	result.AgentsRemovedFromState = removed
	result.ManualActions = dedupeSortedStrings(result.ManualActions)

	errs := make([]error, 0, len(failures)+1)
	for _, failure := range failures {
		errs = append(errs, failure.err)
	}
	if stateErr != nil {
		errs = append(errs, stateErr)
	}
	if len(errs) > 0 {
		return result, errors.Join(errs...)
	}

	// Remove the selector authority sidecar last, after state update has
	// committed, so that a failing state update leaves the sidecar intact
	// for retry (see #1809).
	if authorityRemoval != nil {
		openCodeFailed := false
		for _, failure := range failures {
			if len(failure.agents) == 0 || slices.Contains(failure.agents, model.AgentOpenCode) {
				openCodeFailed = true
				break
			}
		}
		if !openCodeFailed {
			changed, removed, err := authorityRemoval.apply(authorityRemoval.path)
			if err != nil {
				errs = append(errs, fmt.Errorf("remove OpenCode write authority: %w", err))
			} else if changed && removed {
				result.RemovedFiles = append(result.RemovedFiles, authorityRemoval.path)
			}
		}
	}
	if len(errs) > 0 {
		return result, errors.Join(errs...)
	}
	return result, nil
}

// failedAgents reports which agents cannot claim a completed uninstall. A
// failure that no agent owns blocks every agent in the batch: nothing
// distinguishes whose cleanup it belonged to, so none of them may be recorded
// as removed.
func failedAgents(failures []operationFailure, batch []model.AgentID) []model.AgentID {
	if len(failures) == 0 {
		return nil
	}
	blocked := make([]model.AgentID, 0, len(batch))
	for _, failure := range failures {
		if len(failure.agents) == 0 {
			blocked = appendUniqueAgents(blocked, batch...)
			continue
		}
		blocked = appendUniqueAgents(blocked, failure.agents...)
	}
	if len(blocked) == 0 {
		return nil
	}

	// Report in the order the caller named the agents so the summary reads in
	// the same order as the invocation, with any agent outside the batch last.
	ordered := make([]model.AgentID, 0, len(blocked))
	for _, agentID := range batch {
		if slices.Contains(blocked, agentID) {
			ordered = append(ordered, agentID)
		}
	}
	return appendUniqueAgents(ordered, blocked...)
}

func failureManualActions(failures []operationFailure, batch []model.AgentID, homeDir string) []string {
	actions := make([]string, 0, len(failures))
	for _, failure := range failures {
		owners := failure.agents
		if len(owners) == 0 {
			owners = batch
		}
		labels := make([]string, 0, len(owners))
		retry := make([]string, 0, len(owners))
		for _, agentID := range owners {
			labels = append(labels, string(agentID))
			retry = append(retry, "--agent "+string(agentID))
		}
		scope := strings.Join(labels, ", ")
		if scope == "" {
			scope = "this uninstall"
		}
		location := failure.path
		if location == "" {
			location = homeDir
		}
		command := "gentle-ai uninstall --all --yes"
		if len(retry) > 0 {
			command = "gentle-ai uninstall " + strings.Join(retry, " ") + " --yes"
		}
		actions = append(actions, fmt.Sprintf(
			"Uninstall did not complete for %s at %s: %v. Those agents are still recorded in %s. Resolve the file, then rerun `%s`.",
			scope, location, failure.err, state.Path(homeDir), command))
	}
	return actions
}

func appendUniqueAgents(existing []model.AgentID, additions ...model.AgentID) []model.AgentID {
	for _, agentID := range additions {
		if !slices.Contains(existing, agentID) {
			existing = append(existing, agentID)
		}
	}
	return existing
}

func firstOrEmpty(items []string) string {
	if len(items) == 0 {
		return ""
	}
	return items[0]
}

// retainedPiResources returns existing Pi-owned runtime and configuration paths
// that gentle-ai deliberately leaves intact because they can be shared with Pi,
// gentle-pi packages, or user-managed configuration.
func retainedPiResources(homeDir, workspaceDir string) []string {
	paths := []string{
		filepath.Join(homeDir, ".pi", "agent", "agents"),
		filepath.Join(homeDir, ".pi", "agent", "chains"),
		filepath.Join(homeDir, ".pi", "agent", "gentle-ai"),
		filepath.Join(homeDir, ".pi", "agent", "subagents.json"),
		filepath.Join(homeDir, ".pi", "gentle-ai"),
	}
	if workspaceDir != "" {
		paths = append(paths, filepath.Join(workspaceDir, ".pi", "gentle-ai"))
	}

	retained := make([]string, 0, len(paths))
	for _, path := range paths {
		if _, err := os.Lstat(path); err == nil || !os.IsNotExist(err) {
			retained = append(retained, path)
		}
	}
	return retained
}

// optionalPiPackageCleanupCommands mirrors the Pi adapter's canonical package
// sources. Each command remains separate because Pi 0.85.1 supports
// `pi remove <source>`, not a bulk remove form.
func optionalPiPackageCleanupCommands() []string {
	sources := pi.ManagedPackageSources()
	commands := make([]string, 0, len(sources))
	for _, source := range sources {
		commands = append(commands, "pi remove "+source)
	}
	return commands
}

func manualActionForNonEmptyDirectory(path string) (string, bool) {
	if path == "" {
		return "", false
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return "", false
	}
	if len(entries) == 0 {
		return "", false
	}
	return fmt.Sprintf("Remove manually if no longer needed: %s (directory still contains non-managed files)", path), true
}

func dedupeSortedStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	cloned := slices.Clone(items)
	slices.Sort(cloned)
	return slices.Compact(cloned)
}

func settingsTargets(homeDir string, adapter agents.Adapter) []string {
	path := adapter.SettingsPath(homeDir)
	if path == "" {
		return nil
	}
	if adapter.Agent() != model.AgentOpenCode {
		return []string{path}
	}
	targets := []string{opencodeactivation.EffectiveSettingsPath(homeDir, "")}
	legacyJSON := filepath.Join(homeDir, ".config", "opencode", "opencode.json")
	if legacyJSON != targets[0] {
		if info, err := os.Stat(legacyJSON); err == nil && !info.IsDir() {
			targets = append(targets, legacyJSON)
		}
	}
	return dedupeSortedStrings(targets)
}

func (s *Service) componentOperations(adapter agents.Adapter, componentID model.ComponentID) ([]operation, []string, error) {
	ops := make([]operation, 0)
	targets := make([]string, 0)
	homeDir := s.homeDir

	switch componentID {
	case model.ComponentPersona:
		if adapter.SupportsSystemPrompt() {
			path := adapter.SystemPromptFile(homeDir)
			targets = append(targets, path)
			ops = append(ops, rewriteMarkdownFile(path, func(content string) (string, bool) {
				updated, sectionsChanged := removeMarkdownSections(content, "persona")
				updated, personaChanged := removeManagedPersonaPreamble(updated)
				return updated, sectionsChanged || personaChanged
			}))
		}
		if adapter.SupportsOutputStyles() {
			path := filepath.Join(adapter.OutputStyleDir(homeDir), "gentleman.md")
			targets = append(targets, path)
			ops = append(ops, removeFile(path))
			ops = append(ops, removeDirIfEmpty(adapter.OutputStyleDir(homeDir)))
		}
		for _, path := range settingsTargets(homeDir, adapter) {
			targets = append(targets, path)
			jsonPaths := []jsonPath{{"outputStyle"}}
			if adapter.Agent() == model.AgentOpenCode {
				jsonPaths = append(jsonPaths, jsonPath{"agent", "gentleman"})
			}
			ops = append(ops, rewriteJSONFile(path, jsonPaths...))
		}
	case model.ComponentContext7:
		targets = append(targets, context7Targets(adapter, homeDir)...)
		ops = append(ops, context7Operations(adapter, homeDir)...)
	case model.ComponentEngram:
		if s.engramUninstallScope == model.EngramUninstallScopeProject {
			projectDataPath := filepath.Join(s.workspaceDir, ".engram")
			if strings.TrimSpace(s.workspaceDir) != "" {
				targets = append(targets, projectDataPath)
				ops = append(ops, removeTree(projectDataPath))
			}
			break
		}

		targets = append(targets, engramTargets(adapter, homeDir)...)
		ops = append(ops, engramOperations(adapter, homeDir)...)
		if adapter.SupportsSystemPrompt() {
			path := adapter.SystemPromptFile(homeDir)
			targets = append(targets, path)
			ops = append(ops, rewriteMarkdownFile(path, func(content string) (string, bool) {
				return removeMarkdownSections(content, "engram-protocol")
			}))
		}
	case model.ComponentPermission:
		for _, path := range settingsTargets(homeDir, adapter) {
			targets = append(targets, path)
			switch adapter.Agent() {
			case model.AgentClaudeCode:
				ops = append(ops, rewriteJSONFile(path, jsonPath{"permissions"}))
			case model.AgentOpenCode:
				ops = append(ops, rewriteJSONFile(path, jsonPath{"permission"}))
			case model.AgentGeminiCLI:
				ops = append(ops, rewriteJSONFile(path, jsonPath{"general", "defaultApprovalMode"}))
			case model.AgentVSCodeCopilot:
				ops = append(ops, rewriteJSONFile(path, jsonPath{"chat.tools.autoApprove"}))
			}
		}
	case model.ComponentTheme:
		for _, path := range settingsTargets(homeDir, adapter) {
			targets = append(targets, path)
			ops = append(ops, rewriteJSONFile(path, jsonPath{"theme"}))
		}
	case model.ComponentClaudeTheme:
		for _, path := range theme.VisualThemePaths(homeDir, adapter) {
			targets = append(targets, path)
			ops = append(ops, removeFile(path))
		}
		if paths := theme.VisualThemePaths(homeDir, adapter); len(paths) > 0 {
			ops = append(ops, removeDirIfEmpty(filepath.Dir(paths[0])))
		}
	case model.ComponentOpenCodeGentleLogo:
		if adapter.Agent() != model.AgentOpenCode {
			break
		}
		pluginPath := filepath.Join(homeDir, ".config", "opencode", "tui-plugins", "gentle-logo.tsx")
		targets = append(targets, pluginPath)
		ops = append(ops, removeFile(pluginPath), removeDirIfEmpty(filepath.Dir(pluginPath)))
	case model.ComponentSkills:
		if !adapter.SupportsSkills() {
			break
		}
		skillDir := adapter.SkillsDir(homeDir)
		if skillDir == "" {
			break
		}
		entries, err := fs.ReadDir(assets.FS, "skills")
		if err != nil {
			return nil, nil, fmt.Errorf("read embedded skills: %w", err)
		}
		for _, entry := range entries {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), "sdd-") || entry.Name() == "_shared" {
				continue
			}
			dirPath := filepath.Join(skillDir, entry.Name())
			targets = append(targets, dirPath)
			ops = append(ops, removeTree(dirPath), removeDirIfEmpty(skillDir))
		}
		// _shared may hold user-authored notes, so only the embedded references
		// the skills component writes are removed, never the whole directory.
		shared, err := skills.SharedReferencePaths(skillDir)
		if err != nil {
			return nil, nil, err
		}
		for _, path := range shared {
			targets = append(targets, path)
			ops = append(ops, removeFile(path))
		}
		// Upgraded installs may still hold the obsolete generated marker; install
		// only ever removes it as a regular file, so uninstall does the same.
		marker := skills.LegacySharedMarkerPath(skillDir)
		targets = append(targets, marker)
		ops = append(ops, removeRegularFile(marker))
		ops = append(ops, removeDirIfEmpty(filepath.Join(skillDir, "_shared")))
		commands, err := skills.AllSkillCommandPaths(homeDir, adapter)
		if err != nil {
			return nil, nil, err
		}
		for _, path := range commands {
			targets = append(targets, path)
			ops = append(ops, removeFile(path), removeDirIfEmpty(filepath.Dir(path)))
		}
	case model.ComponentGGA:
		for _, path := range globalBackupTargets(homeDir) {
			targets = append(targets, path)
			ops = append(ops, removeFile(path))
		}
		ops = append(ops, removeDirIfEmpty(filepath.Dir(gga.ConfigPath(homeDir))))
	default:
		return nil, nil, fmt.Errorf("unsupported component ID %q", componentID)
	}

	return ops, targets, nil
}

func context7Targets(adapter agents.Adapter, homeDir string) []string {
	switch adapter.MCPStrategy() {
	case model.StrategySeparateMCPFiles:
		if adapter.Agent() == model.AgentClaudeCode {
			return []string{claude.UserConfigPath(homeDir), adapter.SettingsPath(homeDir), adapter.MCPConfigPath(homeDir, "context7")}
		}
		return []string{adapter.MCPConfigPath(homeDir, "context7")}
	case model.StrategyMergeIntoSettings, model.StrategyMCPConfigFile:
		if adapter.Agent() == model.AgentOpenCode {
			return settingsTargets(homeDir, adapter)
		}
		return []string{adapter.MCPConfigPath(homeDir, "context7")}
	default:
		return nil
	}
}

func context7Operations(adapter agents.Adapter, homeDir string) []operation {
	switch adapter.MCPStrategy() {
	case model.StrategySeparateMCPFiles:
		if adapter.Agent() == model.AgentClaudeCode {
			legacyPath := adapter.MCPConfigPath(homeDir, "context7")
			return []operation{rewriteClaudeUserConfig(homeDir, jsonPath{"mcpServers", "context7"}), rewriteJSONFile(adapter.SettingsPath(homeDir), jsonPath{"mcpServers", "context7"}), removeManagedContext7File(legacyPath), removeDirIfEmpty(filepath.Dir(legacyPath))}
		}
		path := adapter.MCPConfigPath(homeDir, "context7")
		return []operation{removeFile(path), removeDirIfEmpty(filepath.Dir(path))}
	case model.StrategyMergeIntoSettings:
		if adapter.Agent() == model.AgentOpenCode {
			ops := make([]operation, 0)
			for _, path := range settingsTargets(homeDir, adapter) {
				ops = append(ops, rewriteJSONFile(path, jsonPath{"mcp", "context7"}))
			}
			return ops
		}
		path := adapter.SettingsPath(homeDir)
		return []operation{rewriteJSONFile(path, jsonPath{"mcpServers", "context7"})}
	case model.StrategyMCPConfigFile:
		path := adapter.MCPConfigPath(homeDir, "context7")
		switch adapter.Agent() {
		case model.AgentVSCodeCopilot:
			return []operation{rewriteJSONFile(path, jsonPath{"servers", "context7"})}
		case model.AgentAntigravity:
			return []operation{rewriteJSONFile(path, jsonPath{"mcpServers", "context7"})}
		default:
			return []operation{rewriteJSONFile(path, jsonPath{"mcpServers", "context7"})}
		}
	default:
		return nil
	}
}

func engramTargets(adapter agents.Adapter, homeDir string) []string {
	targets := make([]string, 0, 3)
	switch adapter.MCPStrategy() {
	case model.StrategySeparateMCPFiles:
		if adapter.Agent() == model.AgentClaudeCode {
			return []string{claude.UserConfigPath(homeDir), adapter.MCPConfigPath(homeDir, "engram")}
		}
		targets = append(targets, adapter.MCPConfigPath(homeDir, "engram"))
	case model.StrategyMergeIntoSettings:
		targets = append(targets, settingsTargets(homeDir, adapter)...)
	case model.StrategyMCPConfigFile:
		targets = append(targets, adapter.MCPConfigPath(homeDir, "engram"))
	case model.StrategyTOMLFile:
		targets = append(targets,
			adapter.MCPConfigPath(homeDir, "engram"),
			filepath.Join(homeDir, ".codex", "engram-instructions.md"),
			filepath.Join(homeDir, ".codex", "engram-compact-prompt.md"),
		)
	}
	return targets
}

func engramOperations(adapter agents.Adapter, homeDir string) []operation {
	switch adapter.MCPStrategy() {
	case model.StrategySeparateMCPFiles:
		path := adapter.MCPConfigPath(homeDir, "engram")
		if adapter.Agent() == model.AgentClaudeCode {
			return []operation{rewriteClaudeUserConfig(homeDir, jsonPath{"mcpServers", "engram"}), removeManagedEngramFile(path)}
		}
		return []operation{removeFile(path), removeDirIfEmpty(filepath.Dir(path))}
	case model.StrategyMergeIntoSettings:
		if adapter.Agent() == model.AgentOpenCode {
			ops := make([]operation, 0)
			for _, path := range settingsTargets(homeDir, adapter) {
				ops = append(ops, rewriteJSONFile(path, jsonPath{"mcp", "engram"}))
			}
			return ops
		}
		path := adapter.SettingsPath(homeDir)
		return []operation{rewriteJSONFile(path, jsonPath{"mcpServers", "engram"})}
	case model.StrategyMCPConfigFile:
		path := adapter.MCPConfigPath(homeDir, "engram")
		if adapter.Agent() == model.AgentVSCodeCopilot {
			return []operation{rewriteJSONFile(path, jsonPath{"servers", "engram"})}
		}
		return []operation{rewriteJSONFile(path, jsonPath{"mcpServers", "engram"})}
	case model.StrategyTOMLFile:
		configPath := adapter.MCPConfigPath(homeDir, "engram")
		instructionsPath := filepath.Join(homeDir, ".codex", "engram-instructions.md")
		compactPath := filepath.Join(homeDir, ".codex", "engram-compact-prompt.md")
		return []operation{
			rewriteTOMLFile(configPath, cleanCodexTOML),
			removeFile(instructionsPath),
			removeFile(compactPath),
			removeDirIfEmpty(filepath.Dir(instructionsPath)),
		}
	default:
		return nil
	}
}

func rewriteMarkdownFile(path string, mutate func(content string) (string, bool)) operation {
	return operation{
		typeID: opRewriteFile,
		path:   path,
		apply: func(path string) (bool, bool, error) {
			content, err := readFileOrEmpty(path)
			if err != nil {
				return false, false, err
			}
			eol := detectEOL(content)
			updated, changed := mutate(content)
			if !changed {
				return false, false, nil
			}
			updated = restoreEOL(updated, eol)
			if strings.TrimSpace(updated) == "" {
				if err := removeFileIfExists(path); err != nil {
					return false, false, err
				}
				return true, true, nil
			}
			_, err = filemerge.WriteFileAtomic(path, []byte(updated), 0o644)
			if err != nil {
				return false, false, err
			}
			return true, false, nil
		},
	}
}

func rewriteJSONFile(path string, jsonPaths ...jsonPath) operation {
	return operation{
		typeID: opRewriteFile,
		path:   path,
		apply: func(path string) (bool, bool, error) {
			raw, err := readManagedFile(path)
			if err != nil {
				if os.IsNotExist(err) {
					return false, false, nil
				}
				return false, false, fmt.Errorf("read json file %q: %w", path, err)
			}
			updated, changed, err := removeJSONPaths(raw, jsonPaths...)
			if err != nil {
				return false, false, fmt.Errorf("clean json file %q: %w", path, err)
			}
			if !changed {
				return false, false, nil
			}
			if jsonIsEmptyObject(updated) {
				if err := removeFileIfExists(path); err != nil {
					return false, false, err
				}
				return true, true, nil
			}
			// Preserve the file's existing mode: ~/.claude.json is injected
			// with 0600 because it holds the OAuth session, and an uninstall
			// rewrite must not widen it.
			perm := os.FileMode(0o644)
			if info, statErr := os.Lstat(path); statErr == nil {
				perm = info.Mode().Perm()
			}
			_, err = filemerge.WriteFileAtomic(path, updated, perm)
			if err != nil {
				return false, false, err
			}
			return true, false, nil
		},
	}
}

// rewriteClaudeUserConfig removes managed entries from ~/.claude.json. Unlike
// rewriteJSONFile it never deletes the file when the result is empty: the
// registry belongs to Claude Code, so uninstall only ever writes the emptied
// object back with the file's mode preserved.
func rewriteClaudeUserConfig(homeDir string, jsonPaths ...jsonPath) operation {
	path := claude.UserConfigPath(homeDir)
	return operation{
		typeID: opRewriteFile,
		path:   path,
		apply: func(path string) (bool, bool, error) {
			raw, err := readManagedFile(path)
			if err != nil {
				if os.IsNotExist(err) {
					return false, false, nil
				}
				return false, false, fmt.Errorf("read json file %q: %w", path, err)
			}
			updated, changed, err := removeJSONPaths(raw, jsonPaths...)
			if err != nil {
				return false, false, fmt.Errorf("clean json file %q: %w", path, err)
			}
			if !changed {
				return false, false, nil
			}
			perm := os.FileMode(0o600)
			if info, statErr := os.Lstat(path); statErr == nil {
				perm = info.Mode().Perm()
			}
			if _, err := filemerge.WriteFileAtomic(path, updated, perm); err != nil {
				return false, false, err
			}
			return true, false, nil
		},
	}
}

func rewriteSkillRegistryHook(path string) operation {
	return operation{
		typeID: opRewriteFile,
		path:   path,
		apply: func(path string) (bool, bool, error) {
			raw, err := readManagedFile(path)
			if err != nil {
				if os.IsNotExist(err) {
					return false, false, nil
				}
				return false, false, fmt.Errorf("read skill-registry hook config %q: %w", path, err)
			}
			updated, changed, err := removeSkillRegistryHook(raw)
			if err != nil {
				return false, false, fmt.Errorf("clean skill-registry hook %q: %w", path, err)
			}
			if !changed {
				return false, false, nil
			}
			if jsonIsEmptyObject(updated) {
				if err := removeFileIfExists(path); err != nil {
					return false, false, err
				}
				return true, true, nil
			}
			_, err = filemerge.WriteFileAtomic(path, updated, 0o644)
			if err != nil {
				return false, false, err
			}
			return true, false, nil
		},
	}
}

// Only exact installed commands are removed; arbitrary user commands remain intact.
func managedRetainedHookCommand(cmd string) bool {
	return cmd == `gentle-ai skill-registry refresh --quiet --no-gitignore --cwd "${CLAUDE_PROJECT_DIR:-$PWD}" || true` ||
		cmd == `gentle-ai skill-registry refresh --quiet --no-gitignore --cwd "$PWD" || true` ||
		cmd == "gentle-ai review stop-hook --agent "+string(model.AgentClaudeCode) ||
		cmd == "gentle-ai telemetry runtime claude --json" ||
		cmd == "gentle-ai telemetry runtime codex --json"
}

func removeSkillRegistryHook(raw []byte) ([]byte, bool, error) {
	root := map[string]any{}
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, false, err
	}
	hooksMap, ok := root["hooks"].(map[string]any)
	if !ok {
		return raw, false, nil
	}
	changed := false
	for _, hookKey := range []string{"UserPromptSubmit", "SessionStart", "Stop", "SubagentStop", "PreToolUse", "PostToolUse", "SessionEnd"} {
		entries, ok := hooksMap[hookKey].([]any)
		if !ok {
			continue
		}
		keptEntries := make([]any, 0, len(entries))
		for _, entry := range entries {
			entryMap, ok := entry.(map[string]any)
			if !ok {
				keptEntries = append(keptEntries, entry)
				continue
			}
			hooks, ok := entryMap["hooks"].([]any)
			if !ok {
				keptEntries = append(keptEntries, entry)
				continue
			}
			keptHooks := make([]any, 0, len(hooks))
			for _, hook := range hooks {
				hookMap, ok := hook.(map[string]any)
				cmd, _ := hookMap["command"].(string)
				if ok && managedRetainedHookCommand(cmd) {
					changed = true
					continue
				}
				keptHooks = append(keptHooks, hook)
			}
			if len(keptHooks) == 0 {
				changed = true
				continue
			}
			entryMap["hooks"] = keptHooks
			keptEntries = append(keptEntries, entryMap)
		}
		if len(keptEntries) == 0 {
			delete(hooksMap, hookKey)
		} else {
			hooksMap[hookKey] = keptEntries
		}
	}
	if !changed {
		return raw, false, nil
	}
	if len(hooksMap) == 0 {
		delete(root, "hooks")
	}
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, false, err
	}
	return append(out, '\n'), true, nil
}

func rewriteTOMLFile(path string, mutate func(content string) (string, bool)) operation {
	return operation{
		typeID: opRewriteFile,
		path:   path,
		apply: func(path string) (bool, bool, error) {
			content, err := readFileOrEmpty(path)
			if err != nil {
				return false, false, err
			}
			eol := detectEOL(content)
			updated, changed := mutate(content)
			if !changed {
				return false, false, nil
			}
			updated = restoreEOL(updated, eol)
			if strings.TrimSpace(updated) == "" {
				if err := removeFileIfExists(path); err != nil {
					return false, false, err
				}
				return true, true, nil
			}
			_, err = filemerge.WriteFileAtomic(path, []byte(updated), 0o644)
			if err != nil {
				return false, false, err
			}
			return true, false, nil
		},
	}
}

// retainedOpenCodePluginOperations is an agent-removal boundary, independent of
// legacy SDD and skills. Unknown or modified plugin bytes are never removed.
func retainedOpenCodePluginOperations(adapter agents.Adapter, homeDir string) []operation {
	pluginDir := filepath.Join(adapter.GlobalConfigDir(homeDir), "plugins")
	ops := make([]operation, 0)
	for _, name := range append([]string{"background-agents.ts"}, opencoderuntimeplugins.OpenCodePluginLifecycleNames(adapter.Agent())...) {
		path := filepath.Join(pluginDir, name)
		ops = append(ops, removeEmbeddedOpenCodePlugin(path, name))
	}
	ops = append(ops, removeDirIfEmpty(pluginDir))
	for _, path := range modelVariantsCachePaths(filepath.Join(homeDir, ".gentle-ai", "cache")) {
		op := removeFile(path)
		op.agents = []model.AgentID{model.AgentOpenCode}
		ops = append(ops, op)
	}
	for i := range ops {
		ops[i].agents = []model.AgentID{model.AgentOpenCode}
	}
	return ops
}

func removeEmbeddedOpenCodePlugin(path, name string) operation {
	return operation{typeID: opRemoveFile, path: path, agents: []model.AgentID{model.AgentOpenCode}, apply: func(path string) (bool, bool, error) {
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			return false, false, nil
		}
		if err != nil {
			return false, false, err
		}
		if !info.Mode().IsRegular() {
			return false, false, nil
		}
		installed, err := os.ReadFile(path)
		if err != nil {
			return false, false, err
		}
		for _, dir := range []string{"opencode/plugins/", "opencode/plugins-v2/"} {
			managed, err := assets.Read(dir + name)
			if err == nil && bytes.Equal(installed, []byte(managed)) {
				if err := os.Remove(path); err != nil {
					return false, false, err
				}
				return true, true, nil
			}
		}
		return false, false, nil
	}}
}

func modelVariantsCachePaths(cacheDir string) []string {
	paths := []string{
		filepath.Join(cacheDir, "model-variants.json"),
		filepath.Join(cacheDir, "model-variants.json.tmp"),
	}
	matches, err := filepath.Glob(filepath.Join(cacheDir, "model-variants.json.*.tmp"))
	if err != nil {
		return paths
	}
	for _, path := range matches {
		if !isModelVariantsRandomTempName(filepath.Base(path)) {
			continue
		}
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		paths = append(paths, path)
	}
	return paths
}

func isModelVariantsRandomTempName(name string) bool {
	const prefix = "model-variants.json."
	const suffix = ".tmp"
	if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, suffix) {
		return false
	}
	token := strings.TrimSuffix(strings.TrimPrefix(name, prefix), suffix)
	if len(token) != 6 {
		return false
	}
	for _, char := range token {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')) {
			return false
		}
	}
	return true
}

func removeManagedContext7File(path string) operation {
	return operation{
		typeID: opRemoveFile,
		path:   path,
		apply: func(path string) (bool, bool, error) {
			content, err := os.ReadFile(path)
			if err != nil {
				if os.IsNotExist(err) {
					return false, false, nil
				}
				return false, false, err
			}
			if !isManagedContext7ServerJSON(content) {
				return false, false, nil
			}
			if err := removeFileIfExists(path); err != nil {
				return false, false, err
			}
			return true, true, nil
		},
	}
}

func removeManagedEngramFile(path string) operation {
	return operation{
		typeID: opRemoveFile,
		path:   path,
		apply: func(path string) (bool, bool, error) {
			removed, err := engram.RemoveManagedLegacyClaudeConfig(path)
			return removed, removed, err
		},
	}
}

func isManagedContext7ServerJSON(content []byte) bool {
	var root map[string]any
	if err := json.Unmarshal(content, &root); err != nil {
		return false
	}
	if command, _ := root["command"].(string); command != "npx" {
		return false
	}
	rawArgs, ok := root["args"].([]any)
	if !ok || len(rawArgs) != 4 {
		return false
	}
	args := make([]string, 0, len(rawArgs))
	for _, raw := range rawArgs {
		arg, ok := raw.(string)
		if !ok {
			return false
		}
		args = append(args, arg)
	}
	return args[0] == "-y" &&
		strings.HasPrefix(args[1], "--package=@upstash/context7-mcp@") &&
		args[2] == "--" &&
		args[3] == "context7-mcp"
}

func removeFile(path string) operation {
	return operation{
		typeID: opRemoveFile,
		path:   path,
		apply: func(path string) (bool, bool, error) {
			_, statErr := os.Stat(path)
			if statErr != nil {
				if os.IsNotExist(statErr) {
					return false, false, nil
				}
				return false, false, statErr
			}
			if err := removeFileIfExists(path); err != nil {
				return false, false, err
			}
			return true, true, nil
		},
	}
}

// removeRegularFile removes path only when it is a regular file, leaving
// symlinks, directories, and other non-regular entries untouched.
func removeRegularFile(path string) operation {
	return operation{
		typeID: opRemoveFile,
		path:   path,
		apply: func(path string) (bool, bool, error) {
			info, err := os.Lstat(path)
			if os.IsNotExist(err) {
				return false, false, nil
			}
			if err != nil {
				return false, false, err
			}
			if !info.Mode().IsRegular() {
				return false, false, nil
			}
			if err := removeFileIfExists(path); err != nil {
				return false, false, err
			}
			return true, true, nil
		},
	}
}

func removeTree(path string) operation {
	return operation{
		typeID: opRemoveTree,
		path:   path,
		apply: func(path string) (bool, bool, error) {
			if _, err := os.Stat(path); err != nil {
				if os.IsNotExist(err) {
					return false, false, nil
				}
				return false, false, err
			}
			if err := os.RemoveAll(path); err != nil {
				return false, false, fmt.Errorf("remove directory tree %q: %w", path, err)
			}
			return true, true, nil
		},
	}
}

func removeDirIfEmpty(path string) operation {
	return operation{
		typeID: opRemoveIfEmpty,
		path:   path,
		apply: func(path string) (bool, bool, error) {
			if path == "" {
				return false, false, nil
			}
			removed, err := removeDirIfEmptyRecursive(path)
			return removed, removed, err
		},
	}
}

func removeDirIfEmptyRecursive(path string) (bool, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if len(entries) != 0 {
		return false, nil
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("remove empty directory %q: %w", path, err)
	}
	return true, nil
}

func readFileOrEmpty(path string) (string, error) {
	data, err := readManagedFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read file %q: %w", path, err)
	}
	return string(data), nil
}

func removeFileIfExists(path string) error {
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove file %q: %w", path, err)
	}
	return nil
}

func expandBackupTarget(path string) ([]string, error) {
	if path == "" {
		return nil, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{path}, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return []string{path}, nil
	}

	files := make([]string, 0)
	err = filepath.WalkDir(path, func(current string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		files = append(files, current)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		// Directory exists but contains no files; return no backup targets.
		// The snapshotter expects file paths, not empty directories.
		return []string{}, nil
	}
	return files, nil
}

func operationKey(op operation) string {
	return fmt.Sprintf("%d:%s", op.typeID, op.path)
}

// mergeRewriteOps composes two rewrite operations on the same file path.
// Both apply functions run sequentially: 'a' writes first, then 'b' reads
// the updated file from disk and applies its own mutation. This works because
// rewriteMarkdownFile and rewriteJSONFile always read fresh from disk.
func mergeRewriteOps(a, b operation) operation {
	return operation{
		typeID: opRewriteFile,
		path:   a.path,
		agents: appendUniqueAgents(slices.Clone(a.agents), b.agents...),
		apply: func(path string) (bool, bool, error) {
			changed1, removed1, err1 := a.apply(path)
			if err1 != nil {
				return changed1, removed1, err1
			}
			// If the first op removed the file entirely, the second op
			// has nothing left to rewrite.
			if removed1 {
				return changed1, removed1, nil
			}
			changed2, removed2, err2 := b.apply(path)
			return changed1 || changed2, removed2, err2
		},
	}
}

func compareOperations(a, b operation) int {
	if a.typeID != b.typeID {
		return int(a.typeID) - int(b.typeID)
	}
	if a.typeID == opRemoveIfEmpty {
		// Deepest path first: a child directory such as skills/_shared must be
		// evaluated before its parent, or the parent is still non-empty when
		// checked and survives an uninstall that emptied it.
		return strings.Compare(b.path, a.path)
	}
	return strings.Compare(a.path, b.path)
}

func removesAllAgentComponents(componentIDs []model.ComponentID) bool {
	if len(componentIDs) == 0 {
		return true
	}
	for _, componentID := range fullAgentRemovalComponents {
		if !slices.Contains(componentIDs, componentID) {
			return false
		}
	}
	return true
}

func globalBackupTargets(homeDir string) []string {
	return []string{
		gga.ConfigPath(homeDir),
		gga.AgentsTemplatePath(homeDir),
	}
}

// One transactional removal of the pair, projected as two file results for the
// existing uninstall reporter. Validation happens at execution, not plan time.
func removeOwnedTelemetryRuntime(configDir string) []operation {
	var attempted bool
	var removed []string
	var err error
	var operations []operation
	for _, path := range telemetryruntime.ManagedPaths(configDir) {
		operations = append(operations, operation{
			typeID: opRemoveFile, path: path, agents: []model.AgentID{model.AgentOpenCode},
			apply: func(path string) (bool, bool, error) {
				if !attempted {
					attempted = true
					removed, err = telemetryruntime.RemoveManaged(configDir)
				}
				changed := slices.Contains(removed, path)
				return changed, changed, err
			},
		})
	}
	return operations
}

func removeOwnedOpenCodeLauncher(path string) operation {
	return operation{
		typeID: opRemoveFile,
		path:   path,
		agents: []model.AgentID{model.AgentOpenCode},
		apply: func(path string) (bool, bool, error) {
			data, err := os.ReadFile(path)
			if os.IsNotExist(err) {
				return false, false, nil
			}
			if err != nil {
				return false, false, err
			}
			if !bytes.Contains(data, []byte(opencodeactivation.OwnershipMarker)) {
				return false, false, nil
			}
			if err := os.Remove(path); err != nil {
				return false, false, err
			}
			return true, true, nil
		},
	}
}

func stateAgentsToRemove(agentIDs []model.AgentID, componentIDs []model.ComponentID) []model.AgentID {
	selected := make(map[model.ComponentID]struct{}, len(componentIDs))
	for _, componentID := range componentIDs {
		selected[componentID] = struct{}{}
	}
	for _, required := range fullAgentRemovalComponents {
		if required == model.ComponentSDD {
			continue
		}
		if _, ok := selected[required]; !ok {
			return nil
		}
	}
	return slices.Clone(agentIDs)
}

func updateStateAfterUninstall(homeDir string, toRemove []model.AgentID) ([]model.AgentID, error) {
	if len(toRemove) == 0 {
		return nil, nil
	}

	// removed must be declared outside the closure so it survives the lock;
	// it is only assigned once the corresponding write has succeeded.
	var removed []model.AgentID
	err := statecoord.WithLock(homeDir, func() error {
		// Re-read the latest state inside the lock so a concurrent writer's
		// change is not clobbered by a stale snapshot taken before the lock.
		current, err := state.Read(homeDir)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return fmt.Errorf("read install state: %w", err)
		}

		removeSet := make(map[string]struct{}, len(toRemove))
		for _, agentID := range toRemove {
			removeSet[string(agentID)] = struct{}{}
		}

		kept := make([]string, 0, len(current.InstalledAgents))
		removedHere := make([]model.AgentID, 0, len(toRemove))
		for _, installed := range current.InstalledAgents {
			if _, ok := removeSet[installed]; ok {
				removedHere = append(removedHere, model.AgentID(installed))
				continue
			}
			kept = append(kept, installed)
		}
		if len(removedHere) == 0 {
			return nil
		}

		updated := current
		updated.InstalledAgents = kept
		if slices.Contains(toRemove, model.AgentOpenCode) {
			updated.BackgroundIntent = ""
		}
		if err := state.Write(homeDir, updated); err != nil {
			return fmt.Errorf("write install state: %w", err)
		}
		removed = removedHere
		return nil
	})
	if err != nil {
		return nil, err
	}
	return removed, nil
}
