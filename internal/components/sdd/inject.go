package sdd

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/agents"
	"github.com/gentleman-programming/gentle-ai/internal/assets"
	"github.com/gentleman-programming/gentle-ai/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

type InjectionResult struct {
	Changed bool
	Files   []string
}

type InjectOptions struct {
	OpenCodeModelAssignments map[string]model.ModelAssignment
	ClaudeModelAssignments   map[string]model.ClaudeModelAlias
	KiroModelAssignments     map[string]model.ClaudeModelAlias

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
// the subagent copy loop resolves ClaudeModelAlias values to native model IDs
// and stamps them into the agent frontmatter sentinel {{KIRO_MODEL}}.
// Adapters that do not implement this interface are unaffected.
type kiroModelResolver interface {
	KiroModelID(alias model.ClaudeModelAlias) string
}

// claudeModelResolver is an optional adapter capability. When implemented,
// the subagent copy loop stamps the resolved ClaudeModelAlias into the agent
// frontmatter sentinel {{CLAUDE_MODEL}}. Claude Code accepts "opus", "sonnet",
// and "haiku" directly as model values, so the resolver is effectively an
// identity function on the alias string — but the interface keeps the opt-in
// shape consistent with kiroModelResolver.
type claudeModelResolver interface {
	ClaudeModelID(alias model.ClaudeModelAlias) string
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

var (
	npmLookPath = exec.LookPath
	npmRun      = func(dir string, args ...string) ([]byte, error) {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		// CombinedOutput captures stdout+stderr so we can surface actionable
		// error messages on failure. Do not set Stdout/Stderr separately.
		return cmd.CombinedOutput()
	}
)

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
			result, err := injectMarkdownSections(homeDir, adapter, opts.ClaudeModelAssignments)
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
			result, err := injectFileAppend(homeDir, adapter)
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
				promptsChanged, promptsErr := WriteSharedPromptFiles(homeDir)
				if promptsErr != nil {
					return InjectionResult{}, fmt.Errorf("write shared SDD prompt files: %w", promptsErr)
				}
				changed = changed || promptsChanged
			}

			overlayBytes, err = inlineOpenCodeSDDPrompts(overlayBytes, homeDir, settingsPath, opts.PreserveOpenCodeOrchestratorPrompt)
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
				"skill-resolver.md",
			}

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

			sddSkills := []string{
				"sdd-init", "sdd-explore", "sdd-propose", "sdd-spec",
				"sdd-design", "sdd-tasks", "sdd-apply", "sdd-verify", "sdd-archive",
				"sdd-onboard", "judgment-day",
			}

			for _, skill := range sddSkills {
				embedDir := "skills/" + skill
				entries, readDirErr := fs.ReadDir(assets.FS, embedDir)
				if readDirErr != nil {
					return InjectionResult{}, fmt.Errorf("required SDD skill %q: embedded directory not found: %w", skill, readDirErr)
				}
				if len(entries) == 0 {
					return InjectionResult{}, fmt.Errorf("required SDD skill %q: embedded directory is empty", skill)
				}

				for _, entry := range entries {
					if entry.IsDir() {
						continue
					}
					assetPath := embedDir + "/" + entry.Name()
					content, readErr := assets.Read(assetPath)
					if readErr != nil {
						return InjectionResult{}, fmt.Errorf("required SDD skill %q file %q: embedded asset not found: %w", skill, entry.Name(), readErr)
					}
					if len(content) == 0 {
						return InjectionResult{}, fmt.Errorf("required SDD skill %q file %q: embedded asset is empty", skill, entry.Name())
					}

					path := filepath.Join(skillDir, skill, entry.Name())
					writeResult, err := filemerge.WriteFileAtomic(path, []byte(content), 0o644)
					if err != nil {
						return InjectionResult{}, err
					}

					changed = changed || writeResult.Changed
					files = append(files, path)
				}
			}
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
				alias := model.ClaudeModelSonnet // safe default
				if opts.KiroModelAssignments != nil {
					if a, hasAlias := opts.KiroModelAssignments[phase]; hasAlias {
						alias = a
					} else if d, hasDefault := opts.KiroModelAssignments["default"]; hasDefault {
						alias = d
					}
				} else if opts.ClaudeModelAssignments != nil {
					// Backward-compatible fallback when Kiro-specific assignments are not provided.
					if a, hasAlias := opts.ClaudeModelAssignments[phase]; hasAlias {
						alias = a
					} else if d, hasDefault := opts.ClaudeModelAssignments["default"]; hasDefault {
						alias = d
					}
				}
				contentStr = strings.ReplaceAll(contentStr, "{{KIRO_MODEL}}", kmr.KiroModelID(alias))
			}

			// Resolve {{CLAUDE_MODEL}} placeholder for adapters that support it (e.g. Claude Code).
			// Non-Claude adapters don't implement claudeModelResolver and are unaffected.
			if cmr, ok := adapter.(claudeModelResolver); ok {
				phase := strings.TrimSuffix(entry.Name(), ".md")
				alias := resolveClaudeModelAlias(opts.ClaudeModelAssignments, phase)
				contentStr = strings.ReplaceAll(contentStr, "{{CLAUDE_MODEL}}", cmr.ClaudeModelID(alias))
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

	// 4. Post-injection verification — catch silent failures.
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

func inlineOpenCodeSDDPrompts(overlayBytes []byte, homeDir, settingsPath string, preserveExistingOrchestratorPrompt bool) ([]byte, error) {
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
				agentMap["prompt"] = "{file:" + filepath.Join(promptDir, phase+".md") + "}"
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
	return replacer.Replace(prompt)
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

// installOpenCodePlugins copies the background-agents plugin and installs its
// npm/bun dependency into the agent's global config directory. Returns an error
// with an actionable message if the package manager is present but the install
// fails. If no package manager is available, the install is skipped (soft failure).
func installOpenCodePlugins(homeDir string, adapter agents.Adapter) (InjectionResult, error) {
	opencodeDir := adapter.GlobalConfigDir(homeDir)
	pluginsDir := filepath.Join(opencodeDir, "plugins")

	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		return InjectionResult{}, fmt.Errorf("create plugins dir: %w", err)
	}

	content := assets.MustRead("opencode/plugins/background-agents.ts")
	pluginPath := filepath.Join(pluginsDir, "background-agents.ts")

	writeResult, err := filemerge.WriteFileAtomic(pluginPath, []byte(content), 0o644)
	if err != nil {
		return InjectionResult{}, fmt.Errorf("write plugin: %w", err)
	}

	files := []string{pluginPath}
	changed := writeResult.Changed

	// Install dependency — prefer bun (OpenCode uses it), fall back to npm.
	// If neither is available, skip with a soft no-op (npm/bun not installed).
	// If a package manager IS found and the install fails, surface the error.
	depPkg := "unique-names-generator"
	nmPath := filepath.Join(opencodeDir, "node_modules", depPkg)

	// Only run the install if the package is not already present.
	pkgMissing := false
	pkgMgrRan := false
	if _, statErr := os.Stat(nmPath); os.IsNotExist(statErr) {
		pkgMissing = true
		var installErr error
		pkgMgrRan, installErr = runPkgInstall(opencodeDir, depPkg)
		if installErr != nil {
			return InjectionResult{}, installErr
		}
	}

	// Post-install validation: if a package manager ran and claimed success,
	// confirm the package actually landed on disk.
	if pkgMissing && pkgMgrRan {
		if _, statErr := os.Stat(nmPath); os.IsNotExist(statErr) {
			// Package manager reported success but the package still isn't there.
			// This is unusual (e.g. bun wrote to a different location). Surface it.
			return InjectionResult{}, fmt.Errorf(
				"post-install check: %q was not found after install in %q — "+
					"the background-agents plugin will fail to load.\n"+
					"Fix: run `cd %s && bun add %s` (or npm install %s) manually",
				depPkg, nmPath, opencodeDir, depPkg, depPkg,
			)
		}
	}

	return InjectionResult{Changed: changed, Files: files}, nil
}

// runPkgInstall installs a node package in the given directory using bun (if
// available) or npm. Returns (true, nil) on success, (false, nil) if no
// package manager is found (soft skip), or (true, error) with a descriptive,
// actionable message if a package manager was found but the install failed.
func runPkgInstall(dir, pkg string) (ran bool, err error) {
	// Prefer bun — OpenCode ships with bun.lock and recommends bun.
	if bunPath, lookErr := npmLookPath("bun"); lookErr == nil {
		out, runErr := npmRun(dir, bunPath, "add", pkg)
		if runErr != nil {
			return true, fmt.Errorf(
				"bun add %s failed in %s: %w\nOutput: %s\nFix: run `cd %s && bun add %s` manually",
				pkg, dir, runErr, strings.TrimSpace(string(out)), dir, pkg,
			)
		}
		return true, nil
	}

	// Fall back to npm.
	if npmPath, lookErr := npmLookPath("npm"); lookErr == nil {
		out, runErr := npmRun(dir, npmPath, "install", "--save", pkg)
		if runErr != nil {
			return true, fmt.Errorf(
				"npm install %s failed in %s: %w\nOutput: %s\nFix: run `cd %s && npm install %s` manually",
				pkg, dir, runErr, strings.TrimSpace(string(out)), dir, pkg,
			)
		}
		return true, nil
	}

	// No package manager available — soft skip.
	return false, nil
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
	case model.AgentOpenCode:
		return "opencode/sdd-orchestrator.md"
	default:
		return "generic/sdd-orchestrator.md"
	}
}

func injectFileAppend(homeDir string, adapter agents.Adapter) (InjectionResult, error) {
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

func injectMarkdownSections(homeDir string, adapter agents.Adapter, assignments map[string]model.ClaudeModelAlias) (InjectionResult, error) {
	promptPath := adapter.SystemPromptFile(homeDir)
	content := assets.MustRead("claude/sdd-orchestrator.md")
	if len(assignments) > 0 {
		var err error
		content, err = injectClaudeModelAssignments(content, assignments)
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
	"orchestrator",
	"sdd-explore",
	"sdd-propose",
	"sdd-spec",
	"sdd-design",
	"sdd-tasks",
	"sdd-apply",
	"sdd-verify",
	"sdd-archive",
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
	"default":      "Non-SDD general delegation",
}

func injectClaudeModelAssignments(content string, assignments map[string]model.ClaudeModelAlias) (string, error) {
	const openMarker = "<!-- gentle-ai:sdd-model-assignments -->"
	const closeMarker = "<!-- /gentle-ai:sdd-model-assignments -->"

	start := strings.Index(content, openMarker)
	end := strings.Index(content, closeMarker)
	if start == -1 || end == -1 || end < start {
		return "", fmt.Errorf("sdd orchestrator asset missing model assignment markers")
	}

	merged := model.ClaudeModelPresetBalanced()
	for key, alias := range assignments {
		if alias.Valid() {
			merged[key] = alias
		}
	}

	replacement := renderClaudeModelAssignmentsSection(merged)
	start += len(openMarker)
	return content[:start] + "\n" + replacement + content[end:], nil
}

func resolveClaudeModelAlias(assignments map[string]model.ClaudeModelAlias, phase string) model.ClaudeModelAlias {
	merged := model.ClaudeModelPresetBalanced()
	for key, alias := range assignments {
		if alias.Valid() {
			merged[key] = alias
		}
	}

	if alias, ok := merged[phase]; ok && alias.Valid() {
		return alias
	}
	if alias, ok := merged["default"]; ok && alias.Valid() {
		return alias
	}
	return model.ClaudeModelSonnet
}

func renderClaudeModelAssignmentsSection(assignments map[string]model.ClaudeModelAlias) string {
	var b strings.Builder
	b.WriteString("## Model Assignments\n\n")
	b.WriteString("Read this table at session start (or before first delegation), cache it for the session, and pass the mapped alias in every Agent tool call via the `model` parameter. If a phase is missing, use the `default` row. If you do not have access to the assigned model (for example, no Opus access), substitute `sonnet` and continue.\n\n")
	b.WriteString("| Phase | Default Model | Reason |\n")
	b.WriteString("|-------|---------------|--------|\n")
	for _, key := range claudeModelAssignmentRowOrder {
		alias := assignments[key]
		if !alias.Valid() {
			alias = model.ClaudeModelSonnet
		}
		b.WriteString(fmt.Sprintf("| %s | %s | %s |\n", key, alias, claudeModelAssignmentReasons[key]))
	}
	b.WriteString("\n")
	return b.String()
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
//     agent does not silently inherit the orchestrator model at runtime
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
		case existingAgentKeys[phase]:
			// 2. Agent already exists in user's config — let merge preserve whatever they have
			// (don't touch the overlay for this agent's model)
		case rootModelID != "":
			// 3. Fresh install or new agent: use root model as default to break inheritance
			agentMap["model"] = rootModelID
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
