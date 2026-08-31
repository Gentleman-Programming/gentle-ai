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
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/claude"
	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/backup"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/communitytool"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/engram"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/gga"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/opencodedefault"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/sdd"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/theme"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	opencodeactivation "github.com/gentleman-programming/gentle-ai/v2/internal/opencode"
	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
)

type Manager interface {
	PartialUninstall(agentIDs []model.AgentID, componentIDs []model.ComponentID) (Result, error)
	CompleteUninstall() (Result, error)
}

type Snapshotter interface {
	Create(snapshotDir string, paths []string) (backup.Manifest, error)
}

type Result struct {
	Manifest               backup.Manifest
	BackupPath             string
	ChangedFiles           []string
	RemovedFiles           []string
	RemovedDirectories     []string
	ManualActions          []string
	AgentsRemovedFromState []model.AgentID
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

	// profileNamesToRemove scopes SDD profile cleanup for this uninstall run.
	// When profileSelectionScoped=false, SDD cleanup removes all detected profiles
	// (legacy behavior). When true, only profileNamesToRemove are removed.
	profileNamesToRemove   []string
	profileSelectionScoped bool

	// engramUninstallScope controls whether Engram cleanup removes global
	// integration files/config (global) or project-local .engram data only.
	engramUninstallScope model.EngramUninstallScope
}

type workflowCapability interface {
	SupportsWorkflows() bool
	WorkflowsDir(workspaceDir string) string
	EmbeddedWorkflowsDir() string
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
		model.ComponentSDD,
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
		model.ComponentSDD,
		model.ComponentSkills,
		model.ComponentTheme,
		model.ComponentClaudeTheme,
		model.ComponentOpenCodeGentleLogo,
	}
	configuredAgents = []string{
		"gentle-orchestrator",
		"sdd-orchestrator", // legacy key — kept for backward-compat cleanup
		"sdd-init",
		"sdd-explore",
		"sdd-research",
		"sdd-propose",
		"sdd-spec",
		"sdd-design",
		"sdd-tasks",
		"sdd-apply",
		"sdd-verify",
		"sdd-archive",
		"sdd-onboard",
		"jd-judge-a",
		"jd-judge-b",
		"jd-fix-agent",
	}
	// sddSkillPhaseIDs contains SDD skill phase IDs only (used for skill dir cleanup).
	// Derived from configuredAgents: excludes the orchestrator (not a skill) and any
	// non-skill agents (e.g. jd-*). When new phases or agents are added to
	// configuredAgents, this list stays in sync automatically.
	sddSkillPhaseIDs func() []string = func() []string {
		skills := make([]string, 0, len(configuredAgents))
		for _, id := range configuredAgents {
			if strings.HasPrefix(id, "sdd-") && id != "sdd-orchestrator" {
				skills = append(skills, id)
			}
		}
		return skills
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
	s.profileNamesToRemove = nil
	s.profileSelectionScoped = false
	s.engramUninstallScope = model.EngramUninstallScopeGlobal

	if len(agentIDs) == 0 {
		return Result{}, fmt.Errorf("partial uninstall requires at least one agent")
	}

	components := componentIDs
	if len(components) == 0 {
		components = slices.Clone(allManagedComponents)
	}
	components = expandVisualPolishUninstallComponents(components)

	plan, err := s.buildPlan(agentIDs, components)
	if err != nil {
		return Result{}, err
	}

	stateRemovals := stateAgentsToRemove(agentIDs, components)
	return s.executePlan(plan, stateRemovals)
}

func (s *Service) PartialUninstallWithProfiles(agentIDs []model.AgentID, componentIDs []model.ComponentID, profileNames []string, engramScope model.EngramUninstallScope) (Result, error) {
	s.SetProfileNamesToRemove(profileNames)
	s.SetEngramUninstallScope(engramScope)
	defer func() {
		s.profileNamesToRemove = nil
		s.profileSelectionScoped = false
		s.engramUninstallScope = model.EngramUninstallScopeGlobal
	}()

	if len(agentIDs) == 0 {
		return Result{}, fmt.Errorf("partial uninstall requires at least one agent")
	}

	components := componentIDs
	if len(components) == 0 {
		components = slices.Clone(allManagedComponents)
	}
	components = expandVisualPolishUninstallComponents(components)

	plan, err := s.buildPlan(agentIDs, components)
	if err != nil {
		return Result{}, err
	}

	stateRemovals := stateAgentsToRemove(agentIDs, components)
	return s.executePlan(plan, stateRemovals)
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

func (s *Service) SetProfileNamesToRemove(profileNames []string) {
	s.profileNamesToRemove = dedupeSortedStrings(profileNames)
	s.profileSelectionScoped = true
}

func (s *Service) SetEngramUninstallScope(scope model.EngramUninstallScope) {
	if scope == model.EngramUninstallScopeProject {
		s.engramUninstallScope = model.EngramUninstallScopeProject
		return
	}
	s.engramUninstallScope = model.EngramUninstallScopeGlobal
}

func (s *Service) CompleteUninstall() (Result, error) {
	s.profileNamesToRemove = nil
	s.profileSelectionScoped = false
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
	}
	if slices.Contains(agentIDs, model.AgentOpenCode) && removesAllAgentComponents(componentIDs) {
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
	}

	for _, op := range p.operations {
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
		if path := adapter.SettingsPath(homeDir); path != "" {
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
		if path := adapter.SettingsPath(homeDir); path != "" {
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
		if path := adapter.SettingsPath(homeDir); path != "" {
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
	case model.ComponentSDD:
		if adapter.SupportsSystemPrompt() {
			path := adapter.SystemPromptFile(homeDir)
			targets = append(targets, path)
			ops = append(ops, rewriteMarkdownFile(path, func(content string) (string, bool) {
				return removeMarkdownSections(content, "sdd-orchestrator", "strict-tdd-mode")
			}))
		}
		if adapter.SupportsSlashCommands() {
			commandsDir := adapter.CommandsDir(homeDir)
			commandsAssetDir := assets.SDDCommandsAssetDir(adapter.Agent())
			entries, err := fs.ReadDir(assets.FS, commandsAssetDir)
			if err != nil {
				return nil, nil, fmt.Errorf("read embedded %s: %w", commandsAssetDir, err)
			}
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				path := filepath.Join(commandsDir, entry.Name())
				targets = append(targets, path)
				ops = append(ops, removeFile(path))
				if legacy := sdd.LegacyClaudeCommandPath(adapter.Agent(), commandsDir, entry.Name()); legacy != "" {
					targets = append(targets, legacy)
					ops = append(ops, removeFile(legacy))
				}
			}
			ops = append(ops, removeDirIfEmpty(commandsDir))
		}
		if path := adapter.SettingsPath(homeDir); path != "" && adapter.Agent() == model.AgentClaudeCode {
			targets = append(targets, path)
			ops = append(ops, rewriteSkillRegistryHook(path))
		}
		if adapter.Agent() == model.AgentCodex {
			path := filepath.Join(adapter.GlobalConfigDir(homeDir), "hooks.json")
			targets = append(targets, path)
			ops = append(ops, rewriteSkillRegistryHook(path))
		}
		if path := adapter.SettingsPath(homeDir); path != "" && adapter.Agent() == model.AgentOpenCode {
			defaultPlan, err := opencodedefault.PrepareUninstall(path)
			if err != nil {
				return nil, nil, err
			}
			targets = append(targets, path, opencodedefault.OwnershipPath(path))
			paths := make([]jsonPath, 0, len(configuredAgents))
			for _, agentKey := range configuredAgents {
				paths = append(paths, jsonPath{"agent", agentKey})
			}

			// Remove named SDD profile agents (suffixed keys). If a profile subset was
			// selected in the uninstall flow, remove only those profiles; otherwise,
			// preserve legacy behavior and remove all detected profiles.
			if s.profileSelectionScoped {
				for _, profileName := range s.profileNamesToRemove {
					for _, agentKey := range sdd.ProfileAgentKeys(profileName) {
						paths = append(paths, jsonPath{"agent", agentKey})
					}
				}
			} else if profiles, err := sdd.DetectProfiles(path); err == nil {
				for _, profile := range profiles {
					for _, agentKey := range sdd.ProfileAgentKeys(profile.Name) {
						paths = append(paths, jsonPath{"agent", agentKey})
					}
				}
			}

			ops = append(ops, rewriteOpenCodeSDDSettings(path, defaultPlan, paths...))

			// The SDD plugin writer resolves the config directory through the
			// adapter and owns the plugin list; uninstall must match it (#3219).
			pluginDir := filepath.Join(adapter.GlobalConfigDir(homeDir), "plugins")
			for _, name := range append([]string{"background-agents.ts"}, sdd.OpenCodePluginLifecycleNames(adapter.Agent())...) {
				pluginPath := filepath.Join(pluginDir, name)
				targets = append(targets, pluginPath)
				ops = append(ops, removeFile(pluginPath))
			}
			ops = append(ops, removeDirIfEmpty(pluginDir))

			modelVariantsCacheDir := filepath.Join(homeDir, ".gentle-ai", "cache")
			for _, cachePath := range modelVariantsCachePaths(modelVariantsCacheDir) {
				targets = append(targets, cachePath)
				ops = append(ops, removeFile(cachePath))
			}

			depDir := filepath.Join(homeDir, ".config", "opencode", "node_modules", "unique-names-generator")
			targets = append(targets, depDir)
			ops = append(ops, removeTree(depDir), removeDirIfEmpty(filepath.Dir(depDir)))
		}
		if adapter.SupportsSkills() {
			skillDir := adapter.SkillsDir(homeDir)
			sharedDir := filepath.Join(skillDir, "_shared")
			targets = append(targets, sharedDir)
			ops = append(ops, removeTree(sharedDir))
			for _, skillID := range managedSDDSkillIDs() {
				dirPath := filepath.Join(skillDir, skillID)
				targets = append(targets, dirPath)
				ops = append(ops, removeTree(dirPath))
			}
			ops = append(ops, removeDirIfEmpty(skillDir))
		}
		if cap, ok := adapter.(workflowCapability); ok && cap.SupportsWorkflows() && s.workspaceDir != "" {
			workflowsDir := cap.WorkflowsDir(s.workspaceDir)
			entries, err := fs.ReadDir(assets.FS, cap.EmbeddedWorkflowsDir())
			if err != nil {
				return nil, nil, fmt.Errorf("read embedded workflows: %w", err)
			}
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				path := filepath.Join(workflowsDir, entry.Name())
				targets = append(targets, path)
				ops = append(ops, removeFile(path))
			}
			ops = append(ops, removeDirIfEmpty(workflowsDir), removeDirIfEmpty(filepath.Dir(workflowsDir)))
		}
		if adapter.SupportsSubAgents() {
			agentsDir := adapter.SubAgentsDir(homeDir)
			entries, err := fs.ReadDir(assets.FS, adapter.EmbeddedSubAgentsDir())
			if err != nil {
				return nil, nil, fmt.Errorf("read embedded sub-agents: %w", err)
			}
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				path := filepath.Join(agentsDir, entry.Name())
				targets = append(targets, path)
				ops = append(ops, removeFile(path))
			}
			ops = append(ops, removeDirIfEmpty(agentsDir))
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
		path := adapter.SettingsPath(homeDir)
		if adapter.Agent() == model.AgentOpenCode {
			return []operation{rewriteJSONFile(path, jsonPath{"mcp", "context7"})}
		}
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
		targets = append(targets, adapter.SettingsPath(homeDir))
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
		path := adapter.SettingsPath(homeDir)
		if adapter.Agent() == model.AgentOpenCode {
			return []operation{rewriteJSONFile(path, jsonPath{"mcp", "engram"})}
		}
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

func rewriteOpenCodeSDDSettings(path string, plan *opencodedefault.UninstallPlan, jsonPaths ...jsonPath) operation {
	return operation{typeID: opRewriteFile, path: path, apply: func(path string) (bool, bool, error) {
		raw, err := readManagedFile(path)
		exists := err == nil
		if err != nil && !os.IsNotExist(err) {
			return false, false, err
		}
		updated := raw
		if exists {
			updated, _, err = removeJSONPaths(raw, jsonPaths...)
			if err != nil {
				return false, false, err
			}
		}
		return plan.Apply(updated, exists)
	}}
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
	for _, hookKey := range []string{"UserPromptSubmit", "SessionStart"} {
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
				if ok && strings.Contains(cmd, "gentle-ai skill-registry refresh") {
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
	return strings.Compare(a.path, b.path)
}

func managedSDDSkillIDs() []string {
	ids := append([]string(nil), sddSkillPhaseIDs()...)
	return append(ids, "judgment-day")
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

	current, err := state.Read(homeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read install state: %w", err)
	}

	removeSet := make(map[string]struct{}, len(toRemove))
	for _, agentID := range toRemove {
		removeSet[string(agentID)] = struct{}{}
	}

	kept := make([]string, 0, len(current.InstalledAgents))
	removed := make([]model.AgentID, 0, len(toRemove))
	for _, installed := range current.InstalledAgents {
		if _, ok := removeSet[installed]; ok {
			removed = append(removed, model.AgentID(installed))
			continue
		}
		kept = append(kept, installed)
	}
	if len(removed) == 0 {
		return nil, nil
	}

	updated := current
	updated.InstalledAgents = kept
	if slices.Contains(toRemove, model.AgentOpenCode) {
		updated.BackgroundIntent = ""
	}
	if err := state.Write(homeDir, updated); err != nil {
		return nil, fmt.Errorf("write install state: %w", err)
	}
	return removed, nil
}
