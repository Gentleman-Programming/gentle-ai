package sdd

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/agents"
	"github.com/gentleman-programming/gentle-ai/internal/assets"
	"github.com/gentleman-programming/gentle-ai/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/internal/components/skills"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/opencode"
)

type InjectionResult struct {
	Changed bool
	Files   []string
}

type InjectOptions struct {
	OpenCodeModelAssignments map[string]model.ModelAssignment
	// ClaudeModelAssignments is the legacy model-only Claude assignment map.
	// Prefer ClaudePhaseAssignments for new callers that need per-phase effort.
	ClaudeModelAssignments      map[string]model.ClaudeModelAlias
	ClaudePhaseAssignments      map[string]model.ClaudePhaseAssignment
	KiroModelAssignments        map[string]model.KiroModelAlias
	CodexModelAssignments       map[string]model.CodexEffort
	CodexCarrilModelAssignments map[string]string // carril→model-id; nil = use defaults
	CodexPhaseModelAssignments  map[string]string // phase→model-id; non-empty = Custom per-phase mode; nil/empty = preset/carril mode

	// WorkspaceDir is the root of the current workspace (e.g. os.Getwd()).
	// When non-empty and the adapter implements workflowInjector, native
	// workflow files are copied to <workspaceDir>/.windsurf/workflows/.
	WorkspaceDir string

	// StrictTDD enables Strict TDD mode. When true, a
	// <!-- gentle-ai:strict-tdd-mode --> marker section is injected into
	// the agent's system prompt so agents know Strict TDD is active.
	StrictTDD bool

	// Profiles lists named SDD profiles to generate and merge into the
	// OpenCode settings file. The default profile (Name="" or Name="default")
	// is skipped — it is handled by the existing flow.
	Profiles []model.Profile

	// PreserveOpenCodeOrchestratorPrompt keeps the existing
	// opencode.json agent.gentle-orchestrator.prompt value during sync.
	// Used by external-single-active profile strategy integrations where
	// external tools extend orchestrator policy/prompt at runtime.
	PreserveOpenCodeOrchestratorPrompt bool

	// Capability is the model capability ("capable" or "small") used to
	// extract the appropriate section from SDD skill files. If empty,
	// skills.InjectWithCapability will be called with empty capability
	// (no section extraction, full content written).
	Capability string

	// triggerRulesContent is an internal field set by step 1c in Inject()
	// for OpenCode/Kilocode adapters. It holds the rendered trigger-rules
	// block so inlineOpenCodeSDDPrompts can append it to the gentle-orchestrator
	// prompt content without re-computing the render.
	triggerRulesContent string
}

// workflowInjector is an optional adapter capability: if an adapter
// implements this interface, sdd.Inject will copy the embedded workflow
// assets into the workspace directory provided via InjectOptions.WorkspaceDir.
// This intentionally does NOT extend agents.Adapter to avoid requiring all
// adapters to implement no-op stubs.
type workflowInjector interface {
	SupportsWorkflows() bool
	// WorkflowsDir returns the target filesystem directory where workflow files
	// should be written (e.g. <workspaceDir>/.windsurf/workflows/).
	WorkflowsDir(workspaceDir string) string
	// EmbeddedWorkflowsDir returns the path inside the embedded assets FS where
	// this adapter's workflow sources live (e.g. "windsurf/workflows").
	// This removes the hardcoded agent name from the injection step, making
	// the workflowInjector pattern reusable for future agents.
	EmbeddedWorkflowsDir() string
}

// kiroModelResolver is an optional adapter capability. When implemented,
// the subagent copy loop resolves KiroModelAlias values to native model IDs
// and stamps them into the agent frontmatter sentinel {{KIRO_MODEL}}.
// Adapters that do not implement this interface are unaffected.
type kiroModelResolver interface {
	KiroModelID(alias model.KiroModelAlias) string
}

// claudeModelResolver is an optional adapter capability. When implemented,
// the subagent copy loop stamps the resolved ClaudeModelAlias into the agent
// frontmatter sentinel {{CLAUDE_MODEL}}. Claude Code accepts "fable", "opus",
// "sonnet", and "haiku" directly as model values, so the resolver is effectively an
// identity function on the alias string — but the interface keeps the opt-in
// shape consistent with kiroModelResolver.
type claudeModelResolver interface {
	ClaudeModelID(alias model.ClaudeModelAlias) string
}

// codexModelResolver is an optional adapter capability. When implemented,
// injectFileAppend will replace the {{CODEX_PHASE_EFFORTS}} placeholder in the
// Codex SDD orchestrator asset with a rendered per-phase effort+model table
// derived from CodexModelAssignments and CodexCarrilModelAssignments in InjectOptions.
//
// Adapters that do NOT implement this interface are completely unaffected —
// the substitution only fires when the adapter satisfies this interface.
type codexModelResolver interface {
	RenderCodexPhaseEfforts(assignments map[string]model.CodexEffort, carrilModels map[string]string) string
}

// monorepoRootMarkers identify files/dirs that ONLY exist at the true root
// of a multi-package workspace. If any of these is found while walking up,
// we stop immediately — this is the authoritative project root.
var monorepoRootMarkers = []string{
	"pnpm-workspace.yaml",
	"pnpm-workspace.yml",
	"nx.json",
	"turbo.json",
	"lerna.json",
	"rush.json",
}

// strongProjectMarkers are definitive project roots that are not
// package.json (which can appear at every level in a monorepo).
var strongProjectMarkers = []string{
	".git",
	"go.mod",
	"Cargo.toml",
	"pyproject.toml",
	"pom.xml",
	"build.gradle",
}

// maxAncestorDepth is the maximum number of parent directories findProjectRoot
// will traverse before giving up. This prevents infinite loops on deeply-nested
// trees and ensures we stop well before reaching the filesystem root.
const maxAncestorDepth = 20

// bootstrapper is an optional adapter capability: if an adapter implements
// this interface, any injector that writes Jinja modules will first ensure
// the base template (entry point) exists.
type bootstrapper interface {
	BootstrapTemplate(homeDir string) error
}

// findProjectRoot walks upward from dir, looking for the best project root.
//
// Priority order:
//  1. Monorepo root markers (pnpm-workspace.yaml, nx.json, turbo.json, etc.) —
//     return immediately when found; these are authoritative workspace roots.
//  2. Strong markers (.git, go.mod, Cargo.toml, etc.) — return immediately;
//     these are unambiguous project roots.
//  3. Weak marker (package.json only) — record as candidate but keep walking
//     upward, since a monorepo marker may exist higher up.
//
// Walking upward means users can run gentle-ai from any subdirectory of their
// project (e.g. repo/packages/app) and still detect the correct workspace root.
// In a JS/TS monorepo, every package has package.json, so we must not stop at
// the first one — we keep walking to find the highest ancestor with package.json
// (or a monorepo root marker above it).
func findProjectRoot(dir string) (string, bool) {
	if dir == "" {
		return "", false
	}
	current := filepath.Clean(dir)
	var bestCandidate string // best weak (package.json-only) match found so far

	for i := 0; i < maxAncestorDepth; i++ {
		// Check monorepo root markers first — highest priority; return immediately.
		for _, marker := range monorepoRootMarkers {
			if _, err := os.Stat(filepath.Join(current, marker)); err == nil {
				return current, true
			}
		}
		// Check strong project markers — definitive roots; return immediately.
		for _, marker := range strongProjectMarkers {
			if _, err := os.Stat(filepath.Join(current, marker)); err == nil {
				return current, true
			}
		}
		// Weak marker: package.json — record but keep walking. Always update
		// to the highest ancestor with a package.json, since in a JS project
		// the root package.json is the authoritative project boundary.
		if _, err := os.Stat(filepath.Join(current, "package.json")); err == nil {
			bestCandidate = current
		}
		parent := filepath.Dir(current)
		if parent == current {
			// Reached filesystem root ("/" on Unix, "C:\" on Windows).
			break
		}
		current = parent
	}

	if bestCandidate != "" {
		return bestCandidate, true
	}
	return "", false
}

// overlayAssetPath returns the embedded asset path for the SDD agent overlay
// based on the selected SDD mode. Empty or SDDModeSingle uses the single
// orchestrator overlay; SDDModeMulti uses the multi-agent overlay.
func overlayAssetPath(sddMode model.SDDModeID) string {
	if sddMode == model.SDDModeMulti {
		return "opencode/sdd-overlay-multi.json"
	}
	return "opencode/sdd-overlay-single.json"
}

func Inject(homeDir string, adapter agents.Adapter, sddMode model.SDDModeID, options ...InjectOptions) (InjectionResult, error) {
	if !adapter.SupportsSystemPrompt() {
		return InjectionResult{}, nil
	}
	if err := validateOpenClawWorkspacePath(homeDir, adapter); err != nil {
		return InjectionResult{}, err
	}

	var opts InjectOptions
	if len(options) > 0 {
		opts = options[0]
	}

	files := make([]string, 0)
	changed := false

	// 1. Inject SDD orchestrator into the global system prompt for agents that
	// rely on prompt files. OpenCode and Kilocode are handled differently: their
	// orchestrator instructions must be scoped to the OpenCode gentle-orchestrator agent only,
	// otherwise the SDD phase sub-agents inherit coordinator-only delegation rules.
	if adapter.Agent() != model.AgentOpenCode && adapter.Agent() != model.AgentKilocode {
		switch adapter.SystemPromptStrategy() {
		case model.StrategyMarkdownSections:
			result, err := injectMarkdownSections(homeDir, adapter, opts.ClaudeModelAssignments, opts.ClaudePhaseAssignments)
			if err != nil {
				return InjectionResult{}, err
			}
			changed = changed || result.Changed
			files = append(files, result.Files...)

		case model.StrategyFileReplace, model.StrategyAppendToFile, model.StrategyInstructionsFile, model.StrategySteeringFile:
			// For FileReplace/AppendToFile agents, the SDD orchestrator is included
			// in the generic persona asset. However, if the user chose neutral or
			// custom persona, the SDD content must still be injected. We append the
			// SDD orchestrator section to the existing system prompt file so it is
			// always present regardless of persona choice.
			result, err := injectFileAppend(homeDir, adapter, opts)
			if err != nil {
				return InjectionResult{}, err
			}
			changed = changed || result.Changed
			files = append(files, result.Files...)

		case model.StrategyJinjaModules:
			// Ensure the base template exists for Jinja-based agents.
			if bs, ok := adapter.(bootstrapper); ok {
				if err := bs.BootstrapTemplate(homeDir); err != nil {
					return InjectionResult{}, fmt.Errorf("bootstrap template: %w", err)
				}
			}

			// Write the SDD orchestrator as a standalone Jinja include module.
			// The static KIMI.md template references it via {% include "sdd-orchestrator.md" %}.
			configDir := adapter.GlobalConfigDir(homeDir)
			content := assets.MustRead(sddOrchestratorAsset(adapter.Agent()))
			modulePath := filepath.Join(configDir, "sdd-orchestrator.md")
			writeResult, err := filemerge.WriteFileAtomic(modulePath, []byte(content), 0o644)
			if err != nil {
				return InjectionResult{}, err
			}
			changed = changed || writeResult.Changed
			files = append(files, modulePath)
		}
	}

	// sectionTriggerRules is the section ID used for marker-based injection.
	// openMarker("trigger-rules") produces <!-- gentle-ai:trigger-rules -->.
	// No new marker constant is needed — filemerge derives it from the section ID string.
	const sectionTriggerRules = "trigger-rules"

	// 1c. Inject the trigger-rules section into every agent's system prompt.
	// Approach mirrors the strict-tdd-mode step (1b) with an additional path for
	// OpenCode/Kilocode whose content lives in the gentle-orchestrator agent prompt
	// (scoped to that agent only, not in a global AGENTS.md section).
	//
	// Decision (4.10): OpenCode and Kilocode deliver trigger-rules inside the
	// gentle-orchestrator prompt where all existing SDD content lives — this keeps
	// the rules in the always-loaded scope for those agents.
	//
	// Decision (4.11): Only Kimi uses StrategyJinjaModules today. If a future
	// adapter adopts Jinja modules it must add its own {% include "trigger-rules.md" %}
	// line and will be handled by the StrategyJinjaModules branch below.
	{
		rendered := RenderTriggerRules(catalog.DefaultTriggerRuleSet())

		if adapter.Agent() == model.AgentOpenCode || adapter.Agent() == model.AgentKilocode {
			// OpenCode / Kilocode: trigger-rules is appended to the gentle-orchestrator
			// prompt content inside opencode.json (handled by inlineOpenCodeSDDPrompts
			// via the triggerRulesContent variable set on InjectOptions — see below).
			// We store the rendered content in opts so inlineOpenCodeSDDPrompts can pick it up.
			opts.triggerRulesContent = rendered
		} else if adapter.SystemPromptStrategy() == model.StrategyJinjaModules {
			// Jinja agents (currently only Kimi): write the rendered block as a
			// standalone module file. The static KIMI.md template includes it via
			// {% include "trigger-rules.md" ignore missing %}.
			configDir := adapter.GlobalConfigDir(homeDir)
			modulePath := filepath.Join(configDir, "trigger-rules.md")
			writeResult, err := filemerge.WriteFileAtomic(modulePath, []byte(rendered), 0o644)
			if err != nil {
				return InjectionResult{}, err
			}
			changed = changed || writeResult.Changed
			files = append(files, modulePath)
		} else {
			// All other system-prompt agents: inject via marker-based section.
			promptPath := adapter.SystemPromptFile(homeDir)
			existing, readErr := readFileOrEmpty(promptPath)
			if readErr != nil {
				return InjectionResult{}, readErr
			}
			updated := filemerge.InjectMarkdownSection(existing, sectionTriggerRules, rendered)
			writeResult, writeErr := filemerge.WriteFileAtomic(promptPath, []byte(updated), 0o644)
			if writeErr != nil {
				return InjectionResult{}, writeErr
			}
			changed = changed || writeResult.Changed
			// Dedupe the path — it may already be present from step 1.
			alreadyInFiles := false
			for _, f := range files {
				if f == promptPath {
					alreadyInFiles = true
					break
				}
			}
			if !alreadyInFiles {
				files = append(files, promptPath)
			}
		}
	}

	// 1b. If StrictTDD is enabled, inject the strict-tdd-mode marker section
	// into the system prompt file so agents know Strict TDD is active.
	if opts.StrictTDD && adapter.Agent() != model.AgentOpenCode && adapter.Agent() != model.AgentKilocode {
		if adapter.SystemPromptStrategy() == model.StrategyJinjaModules {
			// Write the strict-tdd-mode marker as a standalone Jinja include module.
			// The static KIMI.md template references it via {% include "strict-tdd-mode.md" %}.
			configDir := adapter.GlobalConfigDir(homeDir)
			content := "Strict TDD Mode: enabled"
			modulePath := filepath.Join(configDir, "strict-tdd-mode.md")
			writeResult, err := filemerge.WriteFileAtomic(modulePath, []byte(content), 0o644)
			if err != nil {
				return InjectionResult{}, err
			}
			changed = changed || writeResult.Changed
			files = append(files, modulePath)
		} else {
			promptPath := adapter.SystemPromptFile(homeDir)
			strictTDDContent := "Strict TDD Mode: enabled"
			existing, readErr := readFileOrEmpty(promptPath)
			if readErr != nil {
				return InjectionResult{}, readErr
			}
			updated := filemerge.InjectMarkdownSection(existing, "strict-tdd-mode", strictTDDContent)
			writeResult, writeErr := filemerge.WriteFileAtomic(promptPath, []byte(updated), 0o644)
			if writeErr != nil {
				return InjectionResult{}, writeErr
			}
			changed = changed || writeResult.Changed
			// Only append path once (it may already be in files from step 1).
			alreadyInFiles := false
			for _, f := range files {
				if f == promptPath {
					alreadyInFiles = true
					break
				}
			}
			if !alreadyInFiles {
				files = append(files, promptPath)
			}
		}
	}

	// 2. Write slash commands (if the agent supports them).
	if adapter.SupportsSlashCommands() {
		commandsDir := adapter.CommandsDir(homeDir)
		if commandsDir != "" {
			commandsAssetDir := assets.SDDCommandsAssetDir(adapter.Agent())
			commandEntries, err := fs.ReadDir(assets.FS, commandsAssetDir)
			if err != nil {
				return InjectionResult{}, fmt.Errorf("read embedded %s: %w", commandsAssetDir, err)
			}

			for _, entry := range commandEntries {
				if entry.IsDir() {
					continue
				}

				content := assets.MustRead(commandsAssetDir + "/" + entry.Name())
				path := filepath.Join(commandsDir, entry.Name())
				writeResult, err := filemerge.WriteFileAtomic(path, []byte(content), 0o644)
				if err != nil {
					return InjectionResult{}, err
				}

				changed = changed || writeResult.Changed
				files = append(files, path)
			}
		}
	}

	// 2b. OpenCode /sdd-* commands reference agent: gentle-orchestrator.
	// Ensure that agent is present even when persona component is not installed.
	//
	// mergedSettingsBytes holds the final merged opencode.json bytes produced by
	// mergeJSONFile. We keep them in memory so the post-check (step 4) can validate
	// the merge result without re-reading from disk — on Windows/WSL2, the atomic
	// rename (temp → target) may not be immediately visible to a subsequent
	// os.ReadFile call due to VFS/NTFS metadata caching, which caused the spurious
	// "post-check: .../opencode.json missing sdd-apply sub-agent" error.
	var mergedSettingsBytes []byte
	if adapter.Agent() == model.AgentOpenCode || adapter.Agent() == model.AgentKilocode {
		settingsPath := adapter.SettingsPath(homeDir)
		if settingsPath != "" {
			overlayContent, err := assets.Read(overlayAssetPath(sddMode))
			if err != nil {
				return InjectionResult{}, fmt.Errorf("read SDD overlay asset: %w", err)
			}

			// Inject model assignments into the overlay before merging.
			// Models are ONLY written when the user explicitly chose them via
			// the TUI model picker (multi-mode). The overlay JSON itself must
			// NOT contain model fields — otherwise the deep merge overwrites
			// whatever the user already has in opencode.json.
			overlayBytes := []byte(overlayContent)
			// For multi-mode, write shared prompt files before inlining references.
			if sddMode == model.SDDModeMulti {
				// Build phase → capability map from model assignments.
				phaseCapabilities := make(map[string]string)
				for phase, assignment := range opts.OpenCodeModelAssignments {
					phaseCapabilities[phase] = model.ModelCapability(assignment.ModelID)
				}
				// Also include phase assignments from named profiles so their
				// prompt files are written with the correct section.
				for _, profile := range opts.Profiles {
					for phase, assignment := range profile.PhaseAssignments {
						if assignment.ModelID != "" {
							phaseCapabilities[phase] = model.ModelCapability(assignment.ModelID)
						}
					}
				}
				promptsChanged, promptsErr := WriteSharedPromptFiles(homeDir, phaseCapabilities)
				if promptsErr != nil {
					return InjectionResult{}, fmt.Errorf("write shared SDD prompt files: %w", promptsErr)
				}
				changed = changed || promptsChanged
			}

			overlayBytes, err = inlineOpenCodeSDDPrompts(overlayBytes, homeDir, settingsPath, opts.PreserveOpenCodeOrchestratorPrompt, opts.triggerRulesContent)
			if err != nil {
				return InjectionResult{}, fmt.Errorf("inline OpenCode SDD prompts: %w", err)
			}
			assignments := opts.OpenCodeModelAssignments
			if sddMode != model.SDDModeMulti {
				assignments = nil
			}

			var rootModelID string
			var existingAgentKeys map[string]bool
			if sddMode == model.SDDModeMulti {
				rootModelID, err = readOpenCodeRootModel(settingsPath)
				if err != nil {
					return InjectionResult{}, err
				}
				existingAgentKeys, err = readExistingAgentModels(settingsPath)
				if err != nil {
					return InjectionResult{}, err
				}
			}

			if sddMode == model.SDDModeMulti && (len(assignments) > 0 || rootModelID != "") {
				overlayBytes, err = injectModelAssignments(overlayBytes, assignments, rootModelID, existingAgentKeys)
				if err != nil {
					return InjectionResult{}, fmt.Errorf("inject model assignments: %w", err)
				}
			}

			overlayBytes, err = defaultOpenCodeShareDisabled(settingsPath, overlayBytes)
			if err != nil {
				return InjectionResult{}, fmt.Errorf("default OpenCode share mode: %w", err)
			}

			agentResult, err := mergeJSONFile(settingsPath, overlayBytes)
			if err != nil {
				return InjectionResult{}, err
			}
			changed = changed || agentResult.writeResult.Changed
			files = append(files, settingsPath)
			mergedSettingsBytes = agentResult.merged

			// Install OpenCode plugins (all SDD modes).
			pluginResult, err := installOpenCodePlugins(homeDir, adapter)
			if err != nil {
				return InjectionResult{}, err
			}
			changed = changed || pluginResult.Changed
			files = append(files, pluginResult.Files...)

			// Inject named profiles (if any). Each profile generates 11 agent
			// definitions (orchestrator + 10 phases) and merges them into
			// opencode.json. The default profile (empty name or "default") is
			// handled by the existing overlay flow above and is skipped here.
			for _, profile := range opts.Profiles {
				if profile.Name == "" || profile.Name == "default" {
					continue
				}
				profileOverlay, profileErr := GenerateProfileOverlay(profile, homeDir)
				if profileErr != nil {
					return InjectionResult{}, fmt.Errorf("generate profile overlay %q: %w", profile.Name, profileErr)
				}
				profileResult, profileErr := mergeJSONFile(settingsPath, profileOverlay)
				if profileErr != nil {
					return InjectionResult{}, fmt.Errorf("merge profile overlay %q: %w", profile.Name, profileErr)
				}
				changed = changed || profileResult.writeResult.Changed
				mergedSettingsBytes = profileResult.merged
			}
		}
	}

	// 3. Write SDD skill files (if the agent supports skills).
	if adapter.SupportsSkills() {
		skillDir := adapter.SkillsDir(homeDir)
		if skillDir != "" {
			sharedFiles := []string{
				"SKILL.md",
				"persistence-contract.md",
				"engram-convention.md",
				"openspec-convention.md",
				"sdd-phase-common.md",
				"sdd-status-contract.md",
				"skill-resolver.md",
			}
			sddSkillIDs := []model.SkillID{
				"sdd-init", "sdd-explore", "sdd-propose", "sdd-spec",
				"sdd-design", "sdd-tasks", "sdd-apply", "sdd-verify", "sdd-archive",
				"sdd-onboard", "judgment-day",
			}

			// Write shared skill files (not SDD-specific, but needed by SDD).
			// These are written directly, not via skills.Inject, since they are
			// not part of the skills component's injection scope.
			for _, fileName := range sharedFiles {
				assetPath := "skills/_shared/" + fileName
				content, readErr := assets.Read(assetPath)
				if readErr != nil {
					return InjectionResult{}, fmt.Errorf("required SDD shared file %q: embedded asset not found: %w", fileName, readErr)
				}
				if len(content) == 0 {
					return InjectionResult{}, fmt.Errorf("required SDD shared file %q: embedded asset is empty", fileName)
				}

				path := filepath.Join(skillDir, "_shared", fileName)
				writeResult, err := filemerge.WriteFileAtomic(path, []byte(content), 0o644)
				if err != nil {
					return InjectionResult{}, err
				}

				changed = changed || writeResult.Changed
				files = append(files, path)
			}

			// Write SDD skill files using skills.InjectWithCapability, which
			// extracts the appropriate model section from each skill file based on capability.
			// Default to "capable" when no specific capability is set.
			capability := opts.Capability
			if capability == "" {
				capability = "capable"
			}
			sddResult, sddErr := skills.InjectWithCapability(homeDir, adapter, sddSkillIDs, capability)
			if sddErr != nil {
				return InjectionResult{}, fmt.Errorf("inject SDD skills: %w", sddErr)
			}
			changed = changed || sddResult.Changed
			files = append(files, sddResult.Files...)
		}
	}

	// 3b. Write native workflow files (Windsurf Hybrid-First, and any future
	// agent that implements the workflowInjector optional interface).
	// findProjectRoot walks upward from WorkspaceDir so gentle-ai can be
	// invoked from any subdirectory (e.g. repo/internal/foo) and still inject
	// workflows at the real project root. Skips silently if no root is found
	// (e.g. running from home dir without a project).
	if wi, ok := adapter.(workflowInjector); ok && wi.SupportsWorkflows() {
		if projectRoot, found := findProjectRoot(opts.WorkspaceDir); found {
			workflowsDir := wi.WorkflowsDir(projectRoot)
			embedDir := wi.EmbeddedWorkflowsDir()
			entries, readErr := fs.ReadDir(assets.FS, embedDir)
			if readErr != nil {
				return InjectionResult{}, fmt.Errorf("read embedded %s: %w", embedDir, readErr)
			}

			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				content, readErr := assets.Read(embedDir + "/" + entry.Name())
				if readErr != nil {
					return InjectionResult{}, fmt.Errorf("read embedded workflow %q: %w", entry.Name(), readErr)
				}
				path := filepath.Join(workflowsDir, entry.Name())
				writeResult, err := filemerge.WriteFileAtomic(path, []byte(content), 0o644)
				if err != nil {
					return InjectionResult{}, fmt.Errorf("write workflow %q: %w", path, err)
				}
				changed = changed || writeResult.Changed
				files = append(files, path)
			}
		}
	}

	// 3c. Write native sub-agent files for adapters that support them. Sub-agent files are
	// written to the user's home directory (e.g. ~/.cursor/agents/), not to the
	// workspace, so no project-root detection is needed here.
	var agentsDir string
	if adapter.SupportsSubAgents() {
		agentsDir = adapter.SubAgentsDir(homeDir)
		if err := os.MkdirAll(agentsDir, 0o755); err != nil {
			return InjectionResult{}, fmt.Errorf("create agents dir: %w", err)
		}

		embeddedDir := adapter.EmbeddedSubAgentsDir()
		entries, err := assets.FS.ReadDir(embeddedDir)
		if err != nil {
			return InjectionResult{}, fmt.Errorf("read embedded agents dir: %w", err)
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			// Copy all files (not just .md) to support Kimi's YAML-based agents
			contentStr := assets.MustRead(embeddedDir + "/" + entry.Name())

			// Resolve {{KIRO_MODEL}} placeholder for adapters that support it (e.g. Kiro).
			// Non-Kiro adapters (Cursor, etc.) don't implement kiroModelResolver and are unaffected.
			if kmr, ok := adapter.(kiroModelResolver); ok {
				phase := strings.TrimSuffix(entry.Name(), ".md")
				alias := model.KiroModelAuto // safe default
				if opts.KiroModelAssignments != nil {
					if a, hasAlias := opts.KiroModelAssignments[phase]; hasAlias {
						alias = a
					} else if d, hasDefault := opts.KiroModelAssignments["default"]; hasDefault {
						alias = d
					}
				} else if opts.ClaudeModelAssignments != nil {
					// Backward-compatible fallback when Kiro-specific assignments are not provided.
					if a, hasAlias := opts.ClaudeModelAssignments[phase]; hasAlias {
						alias = model.KiroModelAlias(a)
					} else if d, hasDefault := opts.ClaudeModelAssignments["default"]; hasDefault {
						alias = model.KiroModelAlias(d)
					}
				}
				contentStr = strings.ReplaceAll(contentStr, "{{KIRO_MODEL}}", kmr.KiroModelID(alias))
			}

			// Resolve {{CLAUDE_MODEL}} placeholder for adapters that support it (e.g. Claude Code).
			// Non-Claude adapters don't implement claudeModelResolver and are unaffected.
			if cmr, ok := adapter.(claudeModelResolver); ok {
				phase := strings.TrimSuffix(entry.Name(), ".md")
				assignment := resolveClaudePhaseAssignment(opts.ClaudeModelAssignments, opts.ClaudePhaseAssignments, phase)
				contentStr = strings.ReplaceAll(contentStr, "{{CLAUDE_MODEL}}", cmr.ClaudeModelID(assignment.Model))
				contentStr = injectClaudeEffortFrontmatter(contentStr, assignment)
			}
			outPath := filepath.Join(agentsDir, entry.Name())
			writeResult, err := filemerge.WriteFileAtomic(outPath, []byte(contentStr), 0o644)
			if err != nil {
				return InjectionResult{}, fmt.Errorf("write agent %s: %w", entry.Name(), err)
			}
			changed = changed || writeResult.Changed
			if writeResult.Changed {
				files = append(files, outPath)
			}
		}

		// Post-check: verify critical agent files exist (either .md or .yaml)
		for _, phase := range []string{"sdd-apply", "sdd-verify"} {
			found := false
			for _, ext := range []string{".md", ".yaml"} {
				checkPath := filepath.Join(agentsDir, phase+ext)
				if info, err := os.Stat(checkPath); err == nil && info.Size() >= 10 {
					found = true
					break
				}
			}
			if !found {
				return InjectionResult{}, fmt.Errorf("post-check: sub-agent %q not written correctly (missing or truncated)", phase)
			}
		}
	}

	// 4. Install skill-registry startup automation for agents with runtime hooks.
	// This keeps `.atl/skill-registry.md` fresh without making the orchestrator
	// spend tokens rescanning skills on every session. The command itself is
	// fingerprint-cached, so normal startup is cheap.
	automationResult, err := installSkillRegistryAutomation(homeDir, adapter)
	if err != nil {
		return InjectionResult{}, err
	}
	changed = changed || automationResult.Changed
	files = append(files, automationResult.Files...)

	// 5. Post-injection verification — catch silent failures.
	// Primary: validate against the in-memory merged bytes to avoid false
	// negatives on Windows/WSL2 where a freshly-renamed file may not be
	// immediately visible via os.ReadFile.
	// Fallback: if the in-memory check fails, re-read from disk — the
	// opposite failure mode can also occur (in-memory buffer stale but
	// disk has the correct content).
	if adapter.Agent() == model.AgentOpenCode {
		settingsPath := adapter.SettingsPath(homeDir)
		settingsText := string(mergedSettingsBytes)

		// Fallback: if in-memory bytes are empty but the merge succeeded
		// (file was written), read from disk.
		if len(mergedSettingsBytes) == 0 {
			if diskBytes, readErr := os.ReadFile(settingsPath); readErr == nil {
				settingsText = string(diskBytes)
			}
		}

		if !hasOpenCodeAgentKey(settingsText, "gentle-orchestrator") {
			// In-memory check failed — try reading from disk as last resort.
			if diskBytes, readErr := os.ReadFile(settingsPath); readErr == nil {
				settingsText = string(diskBytes)
			}
			if !hasOpenCodeAgentKey(settingsText, "gentle-orchestrator") {
				return InjectionResult{}, fmt.Errorf("post-check: %q missing gentle-orchestrator agent definition — OpenCode /sdd-* commands will fail", settingsPath)
			}
		}
		if hasOpenCodeAgentKey(settingsText, "sdd-orchestrator") {
			if diskBytes, readErr := os.ReadFile(settingsPath); readErr == nil {
				settingsText = string(diskBytes)
			}
			if hasOpenCodeAgentKey(settingsText, "sdd-orchestrator") {
				return InjectionResult{}, fmt.Errorf("post-check: %q still contains legacy sdd-orchestrator agent definition after OpenCode SDD sync", settingsPath)
			}
		}
		if sddMode == model.SDDModeMulti && !strings.Contains(settingsText, `"sdd-apply"`) {
			if diskBytes, readErr := os.ReadFile(settingsPath); readErr == nil {
				settingsText = string(diskBytes)
			}
			if !strings.Contains(settingsText, `"sdd-apply"`) {
				return InjectionResult{}, fmt.Errorf("post-check: %q missing sdd-apply sub-agent — multi-mode overlay was not injected correctly", settingsPath)
			}
		}

		// Verify profile orchestrators were injected correctly.
		// For each named profile, check that sdd-orchestrator-{name} is present
		// in the merged settings. A missing key means the overlay merge silently failed.
		for _, profile := range opts.Profiles {
			if profile.Name == "" || profile.Name == "default" {
				continue
			}
			orchKey := `"sdd-orchestrator-` + profile.Name + `"`
			if !strings.Contains(settingsText, orchKey) {
				// Last-resort disk read.
				if diskBytes, readErr := os.ReadFile(settingsPath); readErr == nil {
					settingsText = string(diskBytes)
				}
				if !strings.Contains(settingsText, orchKey) {
					return InjectionResult{}, fmt.Errorf("post-check: %q missing profile orchestrator %q — profile overlay was not injected correctly", settingsPath, "sdd-orchestrator-"+profile.Name)
				}
			}
		}
	}

	if adapter.SupportsSkills() {
		skillDir := adapter.SkillsDir(homeDir)
		if skillDir != "" {
			for _, skill := range []string{"sdd-init", "sdd-apply", "sdd-verify"} {
				path := filepath.Join(skillDir, skill, "SKILL.md")
				info, err := os.Stat(path)
				if err != nil {
					return InjectionResult{}, fmt.Errorf("post-check: SDD skill %q not found on disk: %w", skill, err)
				}
				if info.Size() < 100 {
					return InjectionResult{}, fmt.Errorf("post-check: SDD skill %q is too small (%d bytes) — content may be empty or corrupt", skill, info.Size())
				}
			}
		}
	}

	return InjectionResult{Changed: changed, Files: files}, nil
}

func validateOpenClawWorkspacePath(workspaceDir string, adapter agents.Adapter) error {
	if adapter.Agent() == model.AgentOpenClaw && strings.TrimSpace(workspaceDir) == "" {
		return fmt.Errorf("openclaw workspace path is required for workspace-first injection")
	}
	return nil
}

func inlineOpenCodeSDDPrompts(overlayBytes []byte, homeDir, settingsPath string, preserveExistingOrchestratorPrompt bool, triggerRulesContent string) ([]byte, error) {
	var overlay map[string]any
	if err := json.Unmarshal(overlayBytes, &overlay); err != nil {
		return nil, fmt.Errorf("unmarshal OpenCode SDD overlay: %w", err)
	}

	agentsRaw, ok := overlay["agent"]
	if !ok {
		return overlayBytes, nil
	}
	agentsMap, ok := agentsRaw.(map[string]any)
	if !ok {
		return overlayBytes, nil
	}

	// Inline the orchestrator prompt (always inlined, not a file reference),
	// unless an external strategy requested preserving the existing prompt.
	orchestratorRaw, ok := agentsMap["gentle-orchestrator"]
	if !ok {
		return overlayBytes, nil
	}
	orchestratorMap, ok := orchestratorRaw.(map[string]any)
	if !ok {
		return overlayBytes, nil
	}
	if preserveExistingOrchestratorPrompt {
		existingPrompt, err := readOpenCodeAgentPrompt(settingsPath, "gentle-orchestrator")
		if err != nil {
			return nil, err
		}
		if existingPrompt == "" {
			existingPrompt, err = readOpenCodeAgentPrompt(settingsPath, "sdd-orchestrator")
			if err != nil {
				return nil, err
			}
		}
		if existingPrompt == "" {
			existingPrompt, err = readMisnamedOpenCodeGentlemanSDDPrompt(settingsPath)
			if err != nil {
				return nil, err
			}
		}
		if existingPrompt != "" {
			orchestratorMap["prompt"] = migratePreservedOpenCodeOrchestratorPrompt(existingPrompt)
		} else {
			orchestratorMap["prompt"] = assets.MustRead(sddOrchestratorAsset(model.AgentOpenCode))
		}
	} else {
		orchestratorMap["prompt"] = assets.MustRead(sddOrchestratorAsset(model.AgentOpenCode))
	}

	// Append the trigger-rules section to the orchestrator prompt when provided.
	// This keeps the rules in the always-loaded scope for OpenCode/Kilocode agents
	// (the orchestrator prompt is the only per-agent content they read at session start).
	if triggerRulesContent != "" {
		if existingPrompt, ok := orchestratorMap["prompt"].(string); ok {
			orchestratorMap["prompt"] = filemerge.InjectMarkdownSection(existingPrompt, "trigger-rules", triggerRulesContent)
		}
	}

	// Replace sub-agent prompt placeholders with {file:<absolutePath>} references.
	// The placeholder format is __PROMPT_FILE_{phase}__ where {phase} is the agent name.
	if homeDir != "" {
		promptDir := SharedPromptDir(homeDir)
		for _, phase := range subAgentPhaseOrder {
			agentRaw, exists := agentsMap[phase]
			if !exists {
				continue
			}
			agentMap, ok := agentRaw.(map[string]any)
			if !ok {
				continue
			}
			placeholder := "__PROMPT_FILE_" + phase + "__"
			if prompt, _ := agentMap["prompt"].(string); prompt == placeholder {
				agentMap["prompt"] = "{file:" + filepath.ToSlash(filepath.Join(promptDir, phase+".md")) + "}"
			}
		}
	}

	result, err := json.MarshalIndent(overlay, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal OpenCode SDD overlay: %w", err)
	}

	return append(result, '\n'), nil
}

func migratePreservedOpenCodeOrchestratorPrompt(prompt string) string {
	if prompt == "" {
		return prompt
	}

	replacer := strings.NewReplacer(
		"Bind this to the dedicated `sdd-orchestrator` agent only.",
		"Bind this to the dedicated `gentle-orchestrator` agent only.",
		"agent.sdd-orchestrator.model",
		"agent.gentle-orchestrator.model",
	)
	return ensurePreservedOpenCodeDelegationHardGates(ensurePreservedOpenCodeOrchestratorPreflight(replacer.Replace(prompt)))
}

func ensurePreservedOpenCodeDelegationHardGates(prompt string) string {
	delegation := `

<!-- gentle-ai:delegation-hard-gates-migration -->
### Mandatory Delegation Triggers (Non-Skippable)

These gates are non-skippable hard gates, not recommendations. They are TOTALMENTE obligatorio: do not skip them, do not weaken them, and do not replace delegation-required gates with inline execution. Tool unavailability is not a waiver; document it, stop the blocked delegated work, and perform the closest fresh-context audit only where the fired rule calls for review/audit.

Semantic guard: **delegate** means using OpenCode's native Task tool to invoke a configured sub-agent. Running local scripts, Python, or Bash inline is execution, not delegation.

Do not pass these rules to child agents as permission to spawn more agents; children receive concrete role work and must not orchestrate.

1. **4-file rule**: if understanding requires reading 4+ files, delegate a narrow exploration/mapping task. If delegation tooling is unavailable, document the blocker and stop the exploration instead of reading everything inline.
2. **Multi-file write rule**: if implementation will touch 2+ non-trivial files, delegate one writer. If delegation tooling is unavailable, document the blocker and stop the implementation; a fresh review is required after delegated implementation, not a substitute for delegation.
3. **PR rule**: before commit, push, or PR after code changes, run a fresh-context review unless the diff is trivial docs/text.
4. **Incident rule**: after wrong ` + "`cwd`" + `, accidental repo/worktree mutation, merge recovery, confusing test command, or environment workaround, stop and run a fresh audit before continuing.
5. **Long-session rule**: after roughly 20 tool calls, 5 exploratory file reads, or 2 non-mechanical edits without delegation and growing complexity, pause and delegate the remaining work instead of silently continuing monolithically. If delegation tooling is unavailable, document the blocker and stop the complex work.
6. **Fresh review rule**: use fresh context for adversarial review of diffs, conflicts, PR readiness, and incidents; use continuity/forked context only for implementation work that needs inherited state.
<!-- /gentle-ai:delegation-hard-gates-migration -->
`

	if strings.Contains(prompt, "Mandatory Delegation Triggers") &&
		strings.Contains(prompt, "non-skippable hard gates") &&
		strings.Contains(prompt, "TOTALMENTE obligatorio") &&
		strings.Contains(prompt, "4-file rule") &&
		strings.Contains(prompt, "Multi-file write rule") &&
		strings.Contains(prompt, "PR rule") &&
		strings.Contains(prompt, "Incident rule") &&
		strings.Contains(prompt, "Long-session rule") &&
		strings.Contains(prompt, "Fresh review rule") &&
		strings.Contains(prompt, "Semantic guard") &&
		strings.Contains(prompt, "execution, not delegation") &&
		strings.Contains(prompt, "fresh review is required after delegated implementation, not a substitute for delegation") {
		return prompt
	}

	start := "<!-- gentle-ai:delegation-hard-gates-migration -->"
	end := "<!-- /gentle-ai:delegation-hard-gates-migration -->"
	if startIdx := strings.Index(prompt, start); startIdx >= 0 {
		if relEndIdx := strings.Index(prompt[startIdx:], end); relEndIdx >= 0 {
			endIdx := startIdx + relEndIdx + len(end)
			return strings.TrimRight(prompt[:startIdx], "\n") + delegation + prompt[endIdx:]
		}
	}

	return strings.TrimRight(prompt, "\n") + delegation
}

func ensurePreservedOpenCodeOrchestratorPreflight(prompt string) string {
	preflight := `

<!-- gentle-ai:sdd-session-preflight-migration -->
### SDD Session Preflight (HARD GATE)

Before executing ANY SDD command or natural-language SDD request, ensure this session has an explicit ` + "`SDD Session Preflight`" + ` decision block.

Required preflight choices: execution mode, artifact store, chained PR strategy, and review budget.

Ask the user directly with a compact, numbered preflight prompt. Match the user's current language for all user-facing prose. If the user writes Spanish, ask the preflight in Spanish. Keep option codes (` + "`A1`" + `, ` + "`B1`" + `, ` + "`C1`" + `, ` + "`D1`" + `) and canonical values unchanged. Do NOT ask the user to type raw keys like ` + "`execution mode`" + `, ` + "`artifact store`" + `, ` + "`chained PR strategy`" + `, or ` + "`review budget`" + `. Do NOT mention non-existent tools. Do NOT invent informal values; use only the canonical values after the user chooses.

Do NOT mix languages inside one preflight prompt: headings, option titles, descriptions, and follow-up text must all be in the user's current language. If the current language is Spanish, use the Spanish localized shape below as the neutral fallback; if an active persona defines a direct-conversation Spanish style, adapt only user-facing prose to that persona while preserving option codes and canonical values. Do not translate only the intro while keeping English labels like ` + "`Pace`" + `, ` + "`Artifacts`" + `, ` + "`Review`" + `, ` + "`recommended`" + `, ` + "`forecast`" + `, or ` + "`budget`" + `.

Use this shape for English users, or translate user-facing prose to the user's current language while preserving option codes. Translation means the whole shape: headings, option titles, and descriptions together.

` + "```text" + `
Before continuing with SDD, choose one option per group.
Reply with "use recommended" or with codes like: A1, B1, C1, D1.

A. Pace
   A1 Interactive (recommended): show each phase and wait for confirmation before continuing.
   A2 Automatic: run phases back-to-back and stop only on high risk.

B. Artifacts
   B1 OpenSpec (recommended): repo files, traceable in review.
   B2 Engram: faster, no spec files in the repo.
   B3 Both: OpenSpec files plus Engram copy.

C. PRs
   C1 Ask me (recommended): stop and ask if the forecast exceeds the budget.
   C2 Single PR: try to keep the change in one PR.
   C3 Chained: split into chained PRs from the start.
   C4 Auto: decide from the size forecast.

D. Review
   D1 400 lines (recommended): stop if forecast exceeds 400 changed lines.
   D2 800 lines: more permissive; useful for medium changes.
   D3 Other: ask for the number afterwards.
` + "```" + `

After asking this, STOP and wait for the user's answer.

If the user's current language is Spanish, use this localized shape:

` + "```text" + `
Antes de continuar con SDD, elija una opción por grupo.
Responda con "usar recomendado" o con códigos como: A1, B1, C1, D1.

A. Ritmo
   A1 Interactivo (recomendado): mostrar cada fase y esperar confirmación antes de continuar.
   A2 Automático: ejecutar las fases seguidas y frenar solo ante riesgo alto.

B. Artefactos
   B1 OpenSpec (recomendado): archivos en el repo, trazables en revisión.
   B2 Engram: más rápido, sin archivos de especificación en el repo.
   B3 Ambos: archivos OpenSpec más copia en Engram.

C. PRs
   C1 Preguntarme (recomendado): frenar y preguntar si la estimación supera el presupuesto.
   C2 Un solo PR: intentar mantener el cambio en un PR.
   C3 Encadenados: separar en PRs encadenados desde el inicio.
   C4 Auto: decidir según la estimación de tamaño.

D. Revisión
   D1 400 líneas (recomendado): frenar si la estimación supera 400 líneas cambiadas.
   D2 800 líneas: más permisivo; útil para cambios medianos.
   D3 Otro: preguntar el número después.
` + "```" + `

Map answers to canonical values: A1/Interactive -> ` + "`interactive`" + `; A2/Automatic -> ` + "`auto`" + `; B1/OpenSpec -> ` + "`openspec`" + `; B2/Engram -> ` + "`engram`" + `; B3/Both -> ` + "`both`" + `; C1/Ask me -> ` + "`ask-always`" + `; C2/Single PR -> ` + "`single-pr-default`" + `; C3/Chained -> ` + "`force-chained`" + `; C4/Auto -> ` + "`auto-forecast`" + `; D1/400 lines -> ` + "`review_budget_lines: 400`" + `; D2/800 lines -> ` + "`review_budget_lines: 800`" + `; D3/Other -> ask one follow-up for the number.

Hard gate rules:

- ` + "`openspec/config.yaml`" + `, existing SDD artifacts, previous ` + "`sdd-init`" + ` results, or installed SDD assets do NOT satisfy session preflight.
- If the session has no preflight block, ask the localized user-facing preflight prompt above and STOP. Do not run init, delegate phases, edit files, or apply tasks in the same turn.
- For a new feature request that says to use SDD, start at preflight -> init guard -> explore/proposal. Never launch ` + "`sdd-apply`" + ` just because the user asked to implement a feature.
- In ` + "`interactive`" + ` mode, pause after each delegated phase returns, summarize the phase, ask before launching the next phase, and STOP. Match the user's language and active persona for direct conversation only; for Spanish neutral fallback ask: "¿Quiere ajustar algo o continuamos?". Do not run /sdd-ff phases back-to-back unless execution mode is ` + "`auto`" + `.
- Interactive approval is phase-scoped. Words like "continue", "dale", or "go on" approve only the immediate next phase, not the rest of the SDD pipeline. Do not treat a generated artifact as approved until the user has had a chance to review or explicitly delegate that review.
- Before the ` + "`sdd-propose`" + ` phase in interactive mode, offer the user a proposal question round instead of silently deciding whether the proposal is clear enough. Ask 3–5 concrete product questions to improve the PRD/proposal by uncovering business rules, implications, impact, edge cases, product tradeoffs, and decision gaps; then summarize assumptions and ask whether the user wants corrections or a second question round. Do not ask about test commands, PR shape, changed-line budget, or other harness mechanics at proposal time unless the user explicitly asks to discuss delivery.
<!-- /gentle-ai:sdd-session-preflight-migration -->
`

	if strings.Contains(prompt, "### SDD Session Preflight (HARD GATE)") &&
		strings.Contains(prompt, "openspec/config.yaml") &&
		strings.Contains(prompt, "Never launch `sdd-apply`") &&
		strings.Contains(prompt, "Match the user's current language") &&
		strings.Contains(prompt, "Do NOT mix languages inside one preflight prompt") &&
		strings.Contains(prompt, "Spanish localized shape below as the neutral fallback") &&
		strings.Contains(prompt, "pause after each delegated phase returns") &&
		strings.Contains(prompt, "approve only the immediate next phase") &&
		strings.Contains(prompt, "proposal question round") &&
		strings.Contains(prompt, "business rules, implications, impact, edge cases") &&
		strings.Contains(prompt, "Before continuing with SDD") &&
		!strings.Contains(prompt, "question` tool") &&
		!containsOpenCodeOrchestratorLanguageLeak(prompt) {
		return prompt
	}

	start := "<!-- gentle-ai:sdd-session-preflight-migration -->"
	end := "<!-- /gentle-ai:sdd-session-preflight-migration -->"
	if startIdx := strings.Index(prompt, start); startIdx >= 0 {
		if relEndIdx := strings.Index(prompt[startIdx:], end); relEndIdx >= 0 {
			endIdx := startIdx + relEndIdx + len(end)
			return strings.TrimRight(prompt[:startIdx], "\n") + preflight + prompt[endIdx:]
		}
	}

	return strings.TrimRight(prompt, "\n") + preflight
}

func containsOpenCodeOrchestratorLanguageLeak(prompt string) bool {
	for _, leak := range []string{
		"elegí",
		"Respondé",
		"¿Querés ajustar algo o continuamos?",
		"If the current language is Spanish, use the Spanish localized shape below verbatim",
	} {
		if strings.Contains(prompt, leak) {
			return true
		}
	}
	return false
}

func readOpenCodeAgentPrompt(settingsPath, agentKey string) (string, error) {
	if strings.TrimSpace(settingsPath) == "" || strings.TrimSpace(agentKey) == "" {
		return "", nil
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read OpenCode settings %q: %w", settingsPath, err)
	}

	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return "", nil
	}

	agentsRaw, ok := root["agent"]
	if !ok {
		return "", nil
	}
	agentsMap, ok := agentsRaw.(map[string]any)
	if !ok {
		return "", nil
	}
	agentRaw, ok := agentsMap[agentKey]
	if !ok {
		return "", nil
	}
	agentMap, ok := agentRaw.(map[string]any)
	if !ok {
		return "", nil
	}
	prompt, _ := agentMap["prompt"].(string)
	return prompt, nil
}

func readMisnamedOpenCodeGentlemanSDDPrompt(settingsPath string) (string, error) {
	if strings.TrimSpace(settingsPath) == "" {
		return "", nil
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read OpenCode settings %q: %w", settingsPath, err)
	}

	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return "", nil
	}
	agentsRaw, ok := root["agent"]
	if !ok {
		return "", nil
	}
	agentsMap, ok := agentsRaw.(map[string]any)
	if !ok {
		return "", nil
	}
	agentRaw, ok := agentsMap["gentleman"]
	if !ok || !looksLikeOpenCodeSDDConductor(agentRaw) {
		return "", nil
	}
	agentMap, ok := agentRaw.(map[string]any)
	if !ok {
		return "", nil
	}
	prompt, _ := agentMap["prompt"].(string)
	return prompt, nil
}

func installSkillRegistryAutomation(homeDir string, adapter agents.Adapter) (InjectionResult, error) {
	if adapter.Agent() == model.AgentCodex {
		hooksPath := filepath.Join(adapter.GlobalConfigDir(homeDir), "hooks.json")
		changed, err := ensureCodexSkillRegistryHook(hooksPath)
		if err != nil {
			return InjectionResult{}, fmt.Errorf("install Codex skill-registry hook: %w", err)
		}
		return InjectionResult{Changed: changed, Files: []string{hooksPath}}, nil
	}
	if adapter.Agent() != model.AgentClaudeCode {
		return InjectionResult{}, nil
	}
	settingsPath := adapter.SettingsPath(homeDir)
	if settingsPath == "" {
		return InjectionResult{}, nil
	}
	changed, err := ensureClaudeSkillRegistryHook(settingsPath)
	if err != nil {
		return InjectionResult{}, fmt.Errorf("install Claude skill-registry hook: %w", err)
	}
	return InjectionResult{Changed: changed, Files: []string{settingsPath}}, nil
}

func ensureCodexSkillRegistryHook(hooksPath string) (bool, error) {
	root := map[string]any{}
	if data, err := os.ReadFile(hooksPath); err == nil && len(strings.TrimSpace(string(data))) > 0 {
		if err := json.Unmarshal(data, &root); err != nil {
			return false, fmt.Errorf("parse Codex hooks %q: %w", hooksPath, err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return false, err
	}

	const command = `gentle-ai skill-registry refresh --quiet --no-gitignore --cwd "$PWD" || true`
	if claudeHookExists(root, command) {
		return false, nil
	}

	hooksRaw, hasHooks := root["hooks"]
	hooksMap, _ := hooksRaw.(map[string]any)
	if hasHooks && hooksMap == nil {
		return false, fmt.Errorf("Codex hooks %q has unsupported hooks shape: want object", hooksPath)
	}
	if hooksMap == nil {
		hooksMap = map[string]any{}
	}

	sessionRaw, hasSessionStart := hooksMap["SessionStart"]
	sessionStart, _ := sessionRaw.([]any)
	if hasSessionStart && sessionStart == nil {
		return false, fmt.Errorf("Codex hooks %q has unsupported hooks.SessionStart shape: want array", hooksPath)
	}
	sessionStart = append(sessionStart, map[string]any{
		"matcher": "startup|resume|clear|compact",
		"hooks": []any{
			map[string]any{
				"type":          "command",
				"command":       command,
				"timeout":       30,
				"statusMessage": "Refreshing skill registry",
			},
		},
	})
	hooksMap["SessionStart"] = sessionStart
	root["hooks"] = hooksMap

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return false, err
	}
	out = append(out, '\n')
	if err := os.MkdirAll(filepath.Dir(hooksPath), 0o755); err != nil {
		return false, err
	}
	wr, err := filemerge.WriteFileAtomic(hooksPath, out, 0o644)
	if err != nil {
		return false, err
	}
	return wr.Changed, nil
}

func ensureClaudeSkillRegistryHook(settingsPath string) (bool, error) {
	root := map[string]any{}
	if data, err := os.ReadFile(settingsPath); err == nil && len(strings.TrimSpace(string(data))) > 0 {
		if err := json.Unmarshal(data, &root); err != nil {
			return false, fmt.Errorf("parse Claude settings %q: %w", settingsPath, err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return false, err
	}

	const command = `gentle-ai skill-registry refresh --quiet --no-gitignore --cwd "${CLAUDE_PROJECT_DIR:-$PWD}" || true`
	if claudeHookExists(root, command) {
		return false, nil
	}

	hooksRaw, hasHooks := root["hooks"]
	hooksMap, _ := hooksRaw.(map[string]any)
	if hasHooks && hooksMap == nil {
		return false, fmt.Errorf("Claude settings %q has unsupported hooks shape: want object", settingsPath)
	}
	if hooksMap == nil {
		hooksMap = map[string]any{}
	}
	promptRaw, hasUserPromptSubmit := hooksMap["UserPromptSubmit"]
	userPromptSubmit, _ := promptRaw.([]any)
	if hasUserPromptSubmit && userPromptSubmit == nil {
		return false, fmt.Errorf("Claude settings %q has unsupported hooks.UserPromptSubmit shape: want array", settingsPath)
	}
	userPromptSubmit = append(userPromptSubmit, map[string]any{
		"matcher": "",
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": command,
			},
		},
	})
	hooksMap["UserPromptSubmit"] = userPromptSubmit
	root["hooks"] = hooksMap

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return false, err
	}
	out = append(out, '\n')
	wr, err := filemerge.WriteFileAtomic(settingsPath, out, 0o644)
	if err != nil {
		return false, err
	}
	return wr.Changed, nil
}

func claudeHookExists(root map[string]any, command string) bool {
	hooksMap, ok := root["hooks"].(map[string]any)
	if !ok {
		return false
	}
	for _, key := range []string{"UserPromptSubmit", "SessionStart"} {
		hookEntries, ok := hooksMap[key].([]any)
		if !ok {
			continue
		}
		if claudeHookListContains(hookEntries, command) {
			return true
		}
	}
	return false
}

func claudeHookListContains(hookEntries []any, command string) bool {
	for _, item := range hookEntries {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		hooks, ok := itemMap["hooks"].([]any)
		if !ok {
			continue
		}
		for _, hook := range hooks {
			hookMap, ok := hook.(map[string]any)
			if ok && hookMap["command"] == command {
				return true
			}
		}
	}
	return false
}

// installOpenCodePlugins copies the OpenCode-compatible plugins that gentle-ai
// still manages by default. Native OpenCode subagents replace the legacy
// background-agents plugin, so that legacy cleanup is scoped to OpenCode only.
func installOpenCodePlugins(homeDir string, adapter agents.Adapter) (InjectionResult, error) {
	opencodeDir := adapter.GlobalConfigDir(homeDir)
	pluginsDir := filepath.Join(opencodeDir, "plugins")

	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		return InjectionResult{}, fmt.Errorf("create plugins dir: %w", err)
	}

	var files []string
	var changed bool

	if adapter.Agent() == model.AgentOpenCode {
		legacyPluginPath := filepath.Join(pluginsDir, "background-agents.ts")
		if err := os.Remove(legacyPluginPath); err != nil {
			if !os.IsNotExist(err) {
				return InjectionResult{}, fmt.Errorf("remove legacy OpenCode plugin %s: %w", legacyPluginPath, err)
			}
		} else {
			changed = true
			files = append(files, legacyPluginPath)
		}
	}

	for _, name := range []string{"model-variants.ts", "skill-registry.ts"} {
		content := assets.MustRead("opencode/plugins/" + name)
		pluginPath := filepath.Join(pluginsDir, name)

		writeResult, err := filemerge.WriteFileAtomic(pluginPath, []byte(content), 0o644)
		if err != nil {
			return InjectionResult{}, fmt.Errorf("write plugin %s: %w", name, err)
		}

		files = append(files, pluginPath)
		if writeResult.Changed {
			changed = true
		}
	}

	return InjectionResult{Changed: changed, Files: files}, nil
}

type mergeJSONResult struct {
	writeResult filemerge.WriteResult
	// merged holds the final JSON bytes that were written to disk.
	// Callers should validate against this in-memory copy instead of
	// re-reading the file from disk — on Windows/WSL2, the atomic rename
	// (temp → target) may not be immediately visible to a subsequent
	// os.ReadFile call due to VFS/NTFS metadata caching.
	merged []byte
}

func mergeJSONFile(path string, overlay []byte) (mergeJSONResult, error) {
	baseJSON, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return mergeJSONResult{}, fmt.Errorf("read json file %q: %w", path, err)
		}
		baseJSON = nil
	}

	baseJSON, err = migrateLegacyOpenCodeAgentsKey(baseJSON)
	if err != nil {
		return mergeJSONResult{}, fmt.Errorf("migrate opencode agents key: %w", err)
	}
	baseJSON, err = migrateLegacyOpenCodeSDDOrchestrator(baseJSON)
	if err != nil {
		return mergeJSONResult{}, fmt.Errorf("migrate opencode sdd orchestrator agent: %w", err)
	}

	merged, err := filemerge.MergeJSONObjects(baseJSON, overlay)
	if err != nil {
		return mergeJSONResult{}, err
	}

	writeResult, err := filemerge.WriteFileAtomic(path, merged, 0o644)
	if err != nil {
		return mergeJSONResult{}, err
	}

	return mergeJSONResult{writeResult: writeResult, merged: merged}, nil
}

// defaultOpenCodeShareDisabled adds a defensive OpenCode default for SDD
// installs: disable session sharing unless the user already chose a share mode.
//
// SDD multi-agent mode creates child sessions for native sub-agents. In
// OpenCode 1.15.x, session creation can route through SessionShare.create when
// sharing is enabled/automatic, and that path has been observed to fail with a
// SQLite FOREIGN KEY constraint error for child sessions. Keeping the default
// local avoids breaking sub-agent startup while preserving explicit user config
// such as "share": "manual" or "share": "auto".
func defaultOpenCodeShareDisabled(settingsPath string, overlay []byte) ([]byte, error) {
	if openCodeSettingsHasShare(settingsPath) {
		return overlay, nil
	}

	root := map[string]any{}
	if err := json.Unmarshal(overlay, &root); err != nil {
		return nil, fmt.Errorf("unmarshal overlay json: %w", err)
	}
	if _, exists := root["share"]; !exists {
		root["share"] = "disabled"
	}

	encoded, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal overlay json: %w", err)
	}
	return append(encoded, '\n'), nil
}

func openCodeSettingsHasShare(settingsPath string) bool {
	content, err := os.ReadFile(settingsPath)
	if err != nil || len(strings.TrimSpace(string(content))) == 0 {
		return false
	}

	root := map[string]any{}
	if err := json.Unmarshal(content, &root); err != nil {
		return false
	}
	_, exists := root["share"]
	return exists
}

// migrateLegacyOpenCodeSDDOrchestrator removes legacy or accidentally renamed
// base OpenCode SDD conductor agents. The base SDD coordinator is now the
// gentle-orchestrator primary agent; named profile agents such as
// sdd-orchestrator-cheap intentionally remain untouched because they are
// generated profile-specific coordinators. The old OpenCode "gentleman" agent
// key is revoked and is removed during sync; if it clearly contains the old SDD
// conductor prompt and no gentle-orchestrator exists yet, its prompt is migrated
// before the revoked key is deleted.
func migrateLegacyOpenCodeSDDOrchestrator(baseJSON []byte) ([]byte, error) {
	if len(strings.TrimSpace(string(baseJSON))) == 0 {
		return baseJSON, nil
	}

	root := map[string]any{}
	if err := json.Unmarshal(baseJSON, &root); err != nil {
		return baseJSON, nil
	}

	agentsRaw, ok := root["agent"]
	if !ok {
		return baseJSON, nil
	}
	agentsMap, ok := agentsRaw.(map[string]any)
	if !ok {
		return baseJSON, nil
	}

	legacy, hasLegacy := agentsMap["sdd-orchestrator"]
	revokedGentleman, hasRevokedGentleman := agentsMap["gentleman"]
	gentlemanLooksLikeConductor := hasRevokedGentleman && looksLikeOpenCodeSDDConductor(revokedGentleman)
	if !hasLegacy && !hasRevokedGentleman {
		return baseJSON, nil
	}
	if !hasLegacy && gentlemanLooksLikeConductor {
		legacy = revokedGentleman
		hasLegacy = true
	}

	if _, hasGentleOrchestrator := agentsMap["gentle-orchestrator"]; !hasGentleOrchestrator && hasLegacy {
		agentsMap["gentle-orchestrator"] = legacy
	}
	delete(agentsMap, "sdd-orchestrator")
	if hasRevokedGentleman {
		delete(agentsMap, "gentleman")
	}

	encoded, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

func looksLikeOpenCodeSDDConductor(agentRaw any) bool {
	agentMap, ok := agentRaw.(map[string]any)
	if !ok {
		return false
	}
	for _, field := range []string{"description", "prompt"} {
		value, _ := agentMap[field].(string)
		if strings.Contains(value, "SDD Orchestrator") || strings.Contains(value, "SDD conductor") {
			return true
		}
	}
	permissionRaw, ok := agentMap["permission"].(map[string]any)
	if !ok {
		return false
	}
	taskRaw, ok := permissionRaw["task"].(map[string]any)
	if !ok {
		return false
	}
	replaceRaw, ok := taskRaw["__replace__"].(map[string]any)
	if !ok {
		return false
	}
	_, allowsApply := replaceRaw["sdd-apply"]
	_, allowsVerify := replaceRaw["sdd-verify"]
	return allowsApply && allowsVerify
}

func hasOpenCodeAgentKey(settingsText, agentKey string) bool {
	root := map[string]any{}
	if err := json.Unmarshal([]byte(settingsText), &root); err != nil {
		return false
	}
	agentsRaw, ok := root["agent"]
	if !ok {
		return false
	}
	agentsMap, ok := agentsRaw.(map[string]any)
	if !ok {
		return false
	}
	_, exists := agentsMap[agentKey]
	return exists
}

// migrateLegacyOpenCodeAgentsKey normalizes old OpenCode schema that used
// "agents" to the current "agent" key. It keeps existing agent entries and
// merges legacy ones without overriding current definitions.
func migrateLegacyOpenCodeAgentsKey(baseJSON []byte) ([]byte, error) {
	if len(strings.TrimSpace(string(baseJSON))) == 0 {
		return baseJSON, nil
	}

	root := map[string]any{}
	if err := json.Unmarshal(baseJSON, &root); err != nil {
		// Preserve prior behavior for non-JSON/non-parseable inputs.
		return baseJSON, nil
	}

	legacyRaw, hasLegacy := root["agents"]
	if !hasLegacy {
		return baseJSON, nil
	}

	legacy, ok := legacyRaw.(map[string]any)
	if !ok {
		delete(root, "agents")
		encoded, err := json.MarshalIndent(root, "", "  ")
		if err != nil {
			return nil, err
		}
		return append(encoded, '\n'), nil
	}

	current := map[string]any{}
	if currentRaw, hasCurrent := root["agent"]; hasCurrent {
		if parsedCurrent, ok := currentRaw.(map[string]any); ok {
			current = parsedCurrent
		}
	}

	for key, value := range legacy {
		if _, exists := current[key]; !exists {
			current[key] = value
		}
	}

	root["agent"] = current
	delete(root, "agents")

	encoded, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, err
	}

	return append(encoded, '\n'), nil
}

// sddOrchestratorMarkers are used to detect if SDD content was already injected
// (e.g., via a persona file or a previous SDD injection). Keep legacy and
// current headings to remain backward compatible across upstream syncs.
var sddOrchestratorMarkers = []string{
	"## Agent Teams Orchestrator",
	"## Spec-Driven Development (SDD) Orchestrator",
	"## Spec-Driven Development (SDD)",
	"# SDD Orchestrator for Cascade",
}

func hasSDDOrchestrator(content string) bool {
	for _, marker := range sddOrchestratorMarkers {
		if strings.Contains(content, marker) {
			return true
		}
	}
	return false
}

// sddOrchestratorAsset returns the embedded asset path for the SDD orchestrator
// content based on the agent. Agent-specific assets take priority; generic is fallback.
func sddOrchestratorAsset(agent model.AgentID) string {
	switch agent {
	case model.AgentClaudeCode:
		return "claude/sdd-orchestrator.md"
	case model.AgentGeminiCLI:
		return "gemini/sdd-orchestrator.md"
	case model.AgentCodex:
		return "codex/sdd-orchestrator.md"
	case model.AgentAntigravity:
		return "antigravity/sdd-orchestrator.md"
	case model.AgentWindsurf:
		return "windsurf/sdd-orchestrator.md"
	case model.AgentCursor:
		return "cursor/sdd-orchestrator.md"
	case model.AgentKimi:
		return "kimi/sdd-orchestrator.md"
	case model.AgentQwenCode:
		return "qwen/sdd-orchestrator.md"
	case model.AgentKiroIDE:
		return "kiro/sdd-orchestrator.md"
	case model.AgentHermes:
		return "hermes/sdd-orchestrator.md"
	case model.AgentOpenCode, model.AgentKilocode:
		return "opencode/sdd-orchestrator.md"
	default:
		return "generic/sdd-orchestrator.md"
	}
}

func injectFileAppend(homeDir string, adapter agents.Adapter, opts InjectOptions) (InjectionResult, error) {
	promptPath := adapter.SystemPromptFile(homeDir)

	existing, err := readFileOrEmpty(promptPath)
	if err != nil {
		return InjectionResult{}, err
	}

	if adapter.SystemPromptStrategy() == model.StrategyInstructionsFile && strings.TrimSpace(existing) == "" {
		existing = instructionsFrontmatter
	}

	if adapter.SystemPromptStrategy() == model.StrategySteeringFile && strings.TrimSpace(existing) == "" {
		existing = steeringFrontmatter
	}

	// Use agent-specific SDD orchestrator content when available; fall back to generic.
	content := assets.MustRead(sddOrchestratorAsset(adapter.Agent()))

	// Codex-only: substitute {{CODEX_PHASE_EFFORTS}} with a rendered per-phase
	// effort table. Only fires when the adapter implements codexModelResolver.
	// All other FileReplace adapters (Gemini, Cursor, etc.) are unaffected.
	if cmr, ok := adapter.(codexModelResolver); ok {
		var rendered string
		if len(opts.CodexPhaseModelAssignments) > 0 {
			// Custom per-phase mode: render a per-phase table (phase | model | effort).
			rendered = model.RenderCodexPhaseEffortsByPhase(opts.CodexPhaseModelAssignments, opts.CodexModelAssignments)
		} else {
			// Preset / carril mode: render the standard per-carril table.
			rendered = cmr.RenderCodexPhaseEfforts(opts.CodexModelAssignments, opts.CodexCarrilModelAssignments)
		}
		content = strings.ReplaceAll(content, "{{CODEX_PHASE_EFFORTS}}", rendered)
		// Post-check: fail loudly if any placeholder token remains unresolved.
		if strings.Contains(content, "{{") {
			return InjectionResult{}, fmt.Errorf("inject(codex): unresolved placeholder token '{{' remains in AGENTS.md content after substitution")
		}
	}

	// If there is a bare (un-marked) legacy orchestrator block, strip it first
	// so InjectMarkdownSection can re-inject the current canonical content.
	if hasLegacyBareOrchestrator(existing) {
		existing = stripBareOrchestratorForFilePrompt(existing)
	}

	updated := filemerge.InjectMarkdownSection(existing, "sdd-orchestrator", content)

	writeResult, err := filemerge.WriteFileAtomic(promptPath, []byte(updated), 0o644)
	if err != nil {
		return InjectionResult{}, err
	}

	return InjectionResult{Changed: writeResult.Changed, Files: []string{promptPath}}, nil
}

func hasLegacyBareOrchestrator(content string) bool {
	markedIdx := strings.Index(content, "<!-- gentle-ai:sdd-orchestrator -->")
	if markedIdx >= 0 {
		prefix := content[:markedIdx]
		if strings.Contains(prefix, "# Agent Teams Lite — Orchestrator Instructions") {
			return true
		}
	}

	firstHeading := -1
	for _, marker := range sddOrchestratorMarkers {
		idx := strings.Index(content, marker)
		if idx >= 0 && (firstHeading == -1 || idx < firstHeading) {
			firstHeading = idx
		}
	}
	if firstHeading < 0 {
		return false
	}

	if markedIdx < 0 {
		return true
	}

	// Legacy bare content exists when an orchestrator heading appears before the
	// canonical marker-based section.
	return firstHeading < markedIdx
}

// stripBareOrchestratorForFilePrompt removes an un-marked SDD orchestrator
// block from file-replace/append/instructions prompt files.
//
// Unlike CLAUDE.md markdown-section files, these prompt files often carry the
// whole orchestrator as a contiguous block followed by other managed sections
// (for example engram-protocol markers). The legacy block also contains many
// "##" headings, so trimming until the next "##" is not enough.
//
// Strategy:
//   - start at the first known orchestrator heading
//   - end at the next managed marker ("<!-- gentle-ai:") if present, else EOF
//   - preserve content before/after and normalize surrounding blank lines
func stripBareOrchestratorForFilePrompt(content string) string {
	if markedIdx := strings.Index(content, "<!-- gentle-ai:sdd-orchestrator -->"); markedIdx >= 0 {
		prefix := content[:markedIdx]
		if start := strings.Index(prefix, "# Agent Teams Lite — Orchestrator Instructions"); start >= 0 {
			before := strings.TrimRight(content[:start], "\n")
			after := strings.TrimLeft(content[markedIdx:], "\n")
			if before == "" {
				if strings.HasSuffix(after, "\n") {
					return after
				}
				return after + "\n"
			}
			result := before + "\n\n" + after
			if !strings.HasSuffix(result, "\n") {
				result += "\n"
			}
			return result
		}
	}

	start := -1
	for _, marker := range sddOrchestratorMarkers {
		idx := strings.Index(content, marker)
		if idx >= 0 && (start == -1 || idx < start) {
			start = idx
		}
	}
	if start < 0 {
		return content
	}

	end := len(content)
	if rel := strings.Index(content[start:], "<!-- gentle-ai:"); rel >= 0 {
		end = start + rel
	}

	before := strings.TrimRight(content[:start], "\n")
	after := strings.TrimLeft(content[end:], "\n")

	if before == "" && after == "" {
		return ""
	}
	if before == "" {
		if strings.HasSuffix(after, "\n") {
			return after
		}
		return after + "\n"
	}
	if after == "" {
		return before + "\n"
	}

	result := before + "\n\n" + after
	if !strings.HasSuffix(result, "\n") {
		result += "\n"
	}
	return result
}

const instructionsFrontmatter = "---\n" +
	"name: Gentle AI Persona\n" +
	"description: Gentleman persona with SDD orchestration and Engram protocol\n" +
	"applyTo: \"**\"\n" +
	"---\n"

const steeringFrontmatter = "---\n" +
	"inclusion: always\n" +
	"---\n"

// stripBareOrchestratorSection removes an un-marked "## Agent Teams Orchestrator"
// (or legacy equivalent) block from content. It finds the first matching heading
// and removes everything from that line to the next same-level (##) heading or
// the end of file. This is used to migrate files that contain bare orchestrator
// content (e.g. copied from docs) before injecting the canonical marker-based version.
func stripBareOrchestratorSection(content string) string {
	lines := strings.Split(content, "\n")

	startLine := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		for _, marker := range sddOrchestratorMarkers {
			if trimmed == marker {
				startLine = i
				break
			}
		}
		if startLine >= 0 {
			break
		}
	}

	if startLine < 0 {
		return content
	}

	// Find end: next ## heading (same or higher level) after startLine, or EOF.
	endLine := len(lines)
	for i := startLine + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "## ") {
			endLine = i
			break
		}
	}

	// Rebuild: keep lines before startLine and lines from endLine onward.
	before := lines[:startLine]
	after := lines[endLine:]

	// Trim trailing blank lines from the before section to avoid double newlines.
	for len(before) > 0 && strings.TrimSpace(before[len(before)-1]) == "" {
		before = before[:len(before)-1]
	}

	var parts []string
	if len(before) > 0 {
		parts = append(parts, strings.Join(before, "\n"))
	}
	if len(after) > 0 {
		afterStr := strings.Join(after, "\n")
		// Trim leading blank lines from the after section.
		afterStr = strings.TrimLeft(afterStr, "\n")
		if afterStr != "" {
			parts = append(parts, afterStr)
		}
	}

	result := strings.Join(parts, "\n\n")
	if result != "" && !strings.HasSuffix(result, "\n") {
		result += "\n"
	}
	return result
}

func injectMarkdownSections(homeDir string, adapter agents.Adapter, legacyAssignments map[string]model.ClaudeModelAlias, phaseAssignments map[string]model.ClaudePhaseAssignment) (InjectionResult, error) {
	promptPath := adapter.SystemPromptFile(homeDir)
	content := assets.MustRead(sddOrchestratorAsset(adapter.Agent()))
	if adapter.Agent() == model.AgentClaudeCode && (len(legacyAssignments) > 0 || len(phaseAssignments) > 0) {
		var err error
		content, err = injectClaudePhaseAssignments(content, legacyAssignments, phaseAssignments)
		if err != nil {
			return InjectionResult{}, err
		}
	}

	existing, err := readFileOrEmpty(promptPath)
	if err != nil {
		return InjectionResult{}, err
	}

	// Strip legacy Agent Teams Lite block (from standalone ATL installer).
	existing = filemerge.StripLegacyATLBlock(existing)

	// If bare (un-marked) orchestrator content exists but the HTML markers are
	// not present, strip the bare block first. This migrates legacy files to the
	// canonical marker-based state without duplicating the section.
	if hasSDDOrchestrator(existing) && !strings.Contains(existing, "<!-- gentle-ai:sdd-orchestrator -->") {
		existing = stripBareOrchestratorSection(existing)
	}

	updated := filemerge.InjectMarkdownSection(existing, "sdd-orchestrator", content)

	writeResult, err := filemerge.WriteFileAtomic(promptPath, []byte(updated), 0o644)
	if err != nil {
		return InjectionResult{}, err
	}

	return InjectionResult{Changed: writeResult.Changed, Files: []string{promptPath}}, nil
}

var claudeModelAssignmentRowOrder = []string{
	"sdd-explore",
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
	"default",
}

var claudeModelAssignmentReasons = map[string]string{
	"orchestrator": "Coordinates, makes decisions",
	"sdd-explore":  "Reads code, structural - not architectural",
	"sdd-propose":  "Architectural decisions",
	"sdd-spec":     "Structured writing",
	"sdd-design":   "Architecture decisions",
	"sdd-tasks":    "Mechanical breakdown",
	"sdd-apply":    "Implementation",
	"sdd-verify":   "Validation against spec",
	"sdd-archive":  "Copy and close",
	"sdd-onboard":  "Guided walkthrough, pedagogical",
	"jd-judge-a":   "Adversarial review — blind judge A",
	"jd-judge-b":   "Adversarial review — blind judge B",
	"jd-fix-agent": "Surgical fixes from confirmed issues",
	"default":      "Non-SDD general delegation",
}

func injectClaudeModelAssignments(content string, assignments map[string]model.ClaudeModelAlias) (string, error) {
	return injectClaudePhaseAssignments(content, assignments, nil)
}

func injectClaudePhaseAssignments(content string, legacyAssignments map[string]model.ClaudeModelAlias, phaseAssignments map[string]model.ClaudePhaseAssignment) (string, error) {
	const openMarker = "<!-- gentle-ai:sdd-model-assignments -->"
	const closeMarker = "<!-- /gentle-ai:sdd-model-assignments -->"

	start := strings.Index(content, openMarker)
	end := strings.Index(content, closeMarker)
	if start == -1 || end == -1 || end < start {
		return "", fmt.Errorf("sdd orchestrator asset missing model assignment markers")
	}

	merged := defaultClaudePhaseAssignments()
	for key, assignment := range model.ClaudePhaseAssignmentsFromLegacy(legacyAssignments) {
		merged[key] = assignment
	}
	for key, assignment := range phaseAssignments {
		if assignment.Valid() {
			merged[key] = assignment
		}
	}

	replacement := renderClaudeModelAssignmentsSection(merged)
	start += len(openMarker)
	return content[:start] + "\n" + replacement + content[end:], nil
}

func defaultClaudePhaseAssignments() map[string]model.ClaudePhaseAssignment {
	return model.ClaudePhaseAssignmentsFromLegacy(model.ClaudeModelPresetBalanced())
}

func resolveClaudeModelAlias(assignments map[string]model.ClaudeModelAlias, phase string) model.ClaudeModelAlias {
	return resolveClaudePhaseAssignment(assignments, nil, phase).Model
}

func resolveClaudePhaseAssignment(legacyAssignments map[string]model.ClaudeModelAlias, phaseAssignments map[string]model.ClaudePhaseAssignment, phase string) model.ClaudePhaseAssignment {
	merged := defaultClaudePhaseAssignments()
	for key, assignment := range model.ClaudePhaseAssignmentsFromLegacy(legacyAssignments) {
		merged[key] = assignment
	}
	for key, assignment := range phaseAssignments {
		if assignment.Valid() {
			merged[key] = assignment
		}
	}

	if assignment, ok := merged[phase]; ok && assignment.Valid() {
		return assignment
	}
	if assignment, ok := merged["default"]; ok && assignment.Valid() {
		return assignment
	}
	return model.ClaudePhaseAssignment{Model: model.ClaudeModelSonnet}
}

func injectClaudeEffortFrontmatter(content string, assignment model.ClaudePhaseAssignment) string {
	const placeholder = "{{CLAUDE_EFFORT_FRONTMATTER}}"
	line := renderClaudeEffortFrontmatter(assignment)
	if line == "" {
		content = strings.ReplaceAll(content, placeholder+"\r\n", "")
		content = strings.ReplaceAll(content, placeholder+"\n", "")
		return strings.ReplaceAll(content, placeholder, "")
	}
	return strings.ReplaceAll(content, placeholder, line)
}

func renderClaudeEffortFrontmatter(assignment model.ClaudePhaseAssignment) string {
	if assignment.Effort == model.ClaudeEffortDefault || !model.ClaudeEffortAllowedForModel(assignment.Model, assignment.Effort) {
		return ""
	}
	return "effort: " + string(assignment.Effort)
}

func renderClaudeModelAssignmentsSection(assignments map[string]model.ClaudePhaseAssignment) string {
	var b strings.Builder
	b.WriteString("## Model Assignments\n\n")
	b.WriteString("Read this table at session start (or before first delegation), cache it for the session, and pass the mapped alias in every Agent tool call via the `model` parameter. If a phase is missing, use the `default` row. If you do not have access to the assigned model (for example, no Opus access), substitute `sonnet` and continue.\n\n")
	b.WriteString("The Claude Code session model is controlled by Claude Code itself; Gentle AI does not configure the main orchestrator model. This table applies only to Agent tool calls for SDD phase sub-agents and general delegation.\n\n")
	b.WriteString("**Mandatory model gate:** Every Agent tool call MUST include `model`. Calling Agent without `model` is invalid. Before each Agent call, resolve the target phase to an alias from this table; for general/non-SDD delegation use `default`. If you are about to call Agent and have not chosen a `model`, STOP and choose the mapped alias first.\n\n")
	b.WriteString("| Phase | Default Model | Effort | Reason |\n")
	b.WriteString("|-------|---------------|--------|--------|\n")
	for _, key := range claudeModelAssignmentRowOrder {
		assignment := assignments[key]
		if !assignment.Valid() {
			assignment = model.ClaudePhaseAssignment{Model: model.ClaudeModelSonnet}
		}
		effort := string(assignment.Effort)
		if effort == "" {
			effort = "default"
		}
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n", key, assignment.Model, effort, claudeModelAssignmentReasons[key]))
	}
	b.WriteString("\n")
	return b.String()
}

// jdAgentSet is a package-level set for O(1) JD agent membership checks,
// consistent with the sddPhaseSet pattern in read_assignments.go.
var jdAgentSet = buildJDAgentSet()

func buildJDAgentSet() map[string]bool {
	phases := opencode.JDPhases()
	set := make(map[string]bool, len(phases))
	for _, p := range phases {
		set[p] = true
	}
	return set
}

// isJDAgent reports whether the agent name is a judgment-day workflow agent.
// JD agents are excluded from root model fallback to preserve independent
// model configuration for diversity of perspective between judges.
func isJDAgent(name string) bool {
	return jdAgentSet[name]
}

// injectModelAssignments injects "model" fields into sub-agent definitions
// within the overlay JSON before it is merged into the settings file.
//
// Decision tree for EACH sub-agent:
//  1. TUI assignment exists for this agent → use it (always wins)
//  2. Agent already exists as a key in the user's existing opencode.json
//     (existingAgentKeys) → skip; let the deep merge preserve whatever the
//     user already has (including no model at all — that's intentional)
//  3. Neither of the above AND rootModelID is set → inject rootModelID so the
//     agent does not silently inherit the orchestrator model at runtime, and
//     write variant="" to stay symmetric with case 1 and prevent stale variant
//     leakage on the deep merge.
//
// If none of the above conditions apply, nothing is written for that agent.
func injectModelAssignments(overlayBytes []byte, assignments map[string]model.ModelAssignment, rootModelID string, existingAgentKeys map[string]bool) ([]byte, error) {
	assignments = normalizeOpenCodeSDDModelAssignments(assignments)

	var overlay map[string]any
	if err := json.Unmarshal(overlayBytes, &overlay); err != nil {
		return nil, fmt.Errorf("unmarshal overlay for model injection: %w", err)
	}

	agentsRaw, ok := overlay["agent"]
	if !ok {
		return overlayBytes, nil
	}
	agents, ok := agentsRaw.(map[string]any)
	if !ok {
		return overlayBytes, nil
	}

	for phase, agentDef := range agents {
		agentMap, ok := agentDef.(map[string]any)
		if !ok {
			continue
		}

		assignment, hasExplicitAssignment := assignments[phase]

		switch {
		case hasExplicitAssignment && assignment.ProviderID != "" && assignment.ModelID != "":
			// 1. TUI choice always wins
			agentMap["model"] = assignment.FullID()
			if assignment.Effort != "" {
				agentMap["variant"] = assignment.Effort
			} else {
				agentMap["variant"] = ""
			}
		case existingAgentKeys[phase]:
			// 2. Agent already exists in user's config — let merge preserve whatever they have
			// (don't touch the overlay for this agent's model)
		case rootModelID != "":
			// 3. Fresh install or new agent: use root model as default to break inheritance.
			// Also clear variant explicitly so the overlay output stays symmetric
			// with case 1 — this prevents a stale variant from leaking through if
			// the embedded overlay or upstream pipeline ever carries a variant.
			// Exception: JD agents are excluded from root model propagation to support
			// independent model configuration and diversity of perspective between judges.
			if !isJDAgent(phase) {
				agentMap["model"] = rootModelID
				agentMap["variant"] = ""
			}
		}
	}

	result, err := json.MarshalIndent(overlay, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal overlay after model injection: %w", err)
	}
	return append(result, '\n'), nil
}

// normalizeOpenCodeSDDModelAssignments accepts the historical
// sdd-orchestrator assignment key as an input alias, but writes it to the
// current base coordinator key: gentle-orchestrator. Named profile keys remain unchanged.
func normalizeOpenCodeSDDModelAssignments(assignments map[string]model.ModelAssignment) map[string]model.ModelAssignment {
	if len(assignments) == 0 {
		return assignments
	}
	legacyAssignment, hasLegacy := assignments["sdd-orchestrator"]
	if !hasLegacy {
		return assignments
	}
	if _, hasGentleOrchestrator := assignments["gentle-orchestrator"]; hasGentleOrchestrator {
		return assignments
	}

	normalized := make(map[string]model.ModelAssignment, len(assignments))
	for key, assignment := range assignments {
		if key == "sdd-orchestrator" {
			continue
		}
		normalized[key] = assignment
	}
	normalized["gentle-orchestrator"] = legacyAssignment
	return normalized
}

// readOpenCodeRootModel reads the top-level "model" field from the opencode.json
// at path. Returns empty string if the file does not exist or has no model field.
func readOpenCodeRootModel(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read opencode root model from %q: %w", path, err)
	}

	root := map[string]any{}
	if err := json.Unmarshal(data, &root); err != nil {
		return "", nil
	}

	rootModelID, _ := root["model"].(string)
	return rootModelID, nil
}

// readExistingAgentModels reads opencode.json at path and returns a set of
// agent names that already exist as keys under the "agent" map, regardless of
// whether those agents have a "model" field. Returns an empty map if the file
// does not exist or has no "agent" key.
func readExistingAgentModels(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]bool{}, nil
		}
		return nil, fmt.Errorf("read existing agent keys from %q: %w", path, err)
	}

	root := map[string]any{}
	if err := json.Unmarshal(data, &root); err != nil {
		return map[string]bool{}, nil
	}

	agentRaw, ok := root["agent"]
	if !ok {
		return map[string]bool{}, nil
	}
	agentMap, ok := agentRaw.(map[string]any)
	if !ok {
		return map[string]bool{}, nil
	}

	result := make(map[string]bool, len(agentMap))
	for name := range agentMap {
		result[name] = true
	}
	return result, nil
}

func readFileOrEmpty(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read file %q: %w", path, err)
	}
	return string(data), nil
}
