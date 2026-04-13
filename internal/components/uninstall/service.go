package uninstall

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/agents"
	"github.com/gentleman-programming/gentle-ai/internal/assets"
	"github.com/gentleman-programming/gentle-ai/internal/backup"
	"github.com/gentleman-programming/gentle-ai/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/internal/components/gga"
	"github.com/gentleman-programming/gentle-ai/internal/components/sdd"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/state"
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

type subAgentCapability interface {
	SupportsSubAgents() bool
	SubAgentsDir(homeDir string) string
	EmbeddedSubAgentsDir() string
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
	}
	sddPhaseAgents = []string{
		"sdd-orchestrator",
		"sdd-init",
		"sdd-explore",
		"sdd-propose",
		"sdd-spec",
		"sdd-design",
		"sdd-tasks",
		"sdd-apply",
		"sdd-verify",
		"sdd-archive",
		"sdd-onboard",
	}
	sddSkillPhaseIDs = sddPhaseAgents[1:]
)

type operation struct {
	typeID opType
	path   string
	apply  func(path string) (changed bool, removed bool, err error)
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

	plan, err := s.buildPlan(agentIDs, components)
	if err != nil {
		return Result{}, err
	}

	stateRemovals := stateAgentsToRemove(agentIDs, components)
	return s.executePlan(plan, stateRemovals)
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
				key := operationKey(op)
				if existing, ok := operationsByKey[key]; ok && op.typeID == opRewriteFile {
					// Merge rewrite operations on the same file so both
					// mutations apply (e.g. persona + engram on system prompt).
					operationsByKey[key] = mergeRewriteOps(existing, op)
				} else {
					operationsByKey[key] = op
				}
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

	for _, op := range p.operations {
		changed, removed, err := op.apply(op.path)
		if err != nil {
			return result, err
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

	removed, err := updateStateAfterUninstall(s.homeDir, agentsToRemove)
	if err != nil {
		return result, err
	}
	result.AgentsRemovedFromState = removed
	result.ManualActions = dedupeSortedStrings(result.ManualActions)
	return result, nil
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
			entries, err := fs.ReadDir(assets.FS, "opencode/commands")
			if err != nil {
				return nil, nil, fmt.Errorf("read embedded opencode commands: %w", err)
			}
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				path := filepath.Join(commandsDir, entry.Name())
				targets = append(targets, path)
				ops = append(ops, removeFile(path))
			}
			ops = append(ops, removeDirIfEmpty(commandsDir))
		}
		if path := adapter.SettingsPath(homeDir); path != "" && adapter.Agent() == model.AgentOpenCode {
			targets = append(targets, path)
			paths := make([]jsonPath, 0, len(sddPhaseAgents))
			for _, agentKey := range sddPhaseAgents {
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

			ops = append(ops, rewriteJSONFile(path, paths...))

			pluginPath := filepath.Join(homeDir, ".config", "opencode", "plugins", "background-agents.ts")
			targets = append(targets, pluginPath)
			ops = append(ops, removeFile(pluginPath), removeDirIfEmpty(filepath.Dir(pluginPath)))

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
		if cap, ok := adapter.(subAgentCapability); ok && cap.SupportsSubAgents() {
			agentsDir := cap.SubAgentsDir(homeDir)
			entries, err := fs.ReadDir(assets.FS, cap.EmbeddedSubAgentsDir())
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
			_, err = filemerge.WriteFileAtomic(path, updated, 0o644)
			if err != nil {
				return false, false, err
			}
			return true, false, nil
		},
	}
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
	ids := append([]string(nil), sddSkillPhaseIDs...)
	return append(ids, "judgment-day")
}

func globalBackupTargets(homeDir string) []string {
	return []string{
		gga.ConfigPath(homeDir),
		gga.AgentsTemplatePath(homeDir),
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
	if err := state.Write(homeDir, updated); err != nil {
		return nil, fmt.Errorf("write install state: %w", err)
	}
	return removed, nil
}
