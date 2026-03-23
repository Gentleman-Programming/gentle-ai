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

func Inject(homeDir string, adapter agents.Adapter, sddMode model.SDDModeID, modelAssignments ...map[string]model.ModelAssignment) (InjectionResult, error) {
	if !adapter.SupportsSystemPrompt() {
		return InjectionResult{}, nil
	}

	files := make([]string, 0)
	changed := false

	// 1. Inject SDD orchestrator into system prompt.
	switch adapter.SystemPromptStrategy() {
	case model.StrategyMarkdownSections:
		result, err := injectMarkdownSections(homeDir, adapter)
		if err != nil {
			return InjectionResult{}, err
		}
		changed = changed || result.Changed
		files = append(files, result.Files...)

	case model.StrategyFileReplace, model.StrategyAppendToFile, model.StrategyInstructionsFile:
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
	}

	// 2. Write slash commands (if the agent supports them).
	if adapter.SupportsSlashCommands() {
		commandsDir := adapter.CommandsDir(homeDir)
		if commandsDir != "" {
			commandEntries, err := fs.ReadDir(assets.FS, "opencode/commands")
			if err != nil {
				return InjectionResult{}, fmt.Errorf("read embedded opencode/commands: %w", err)
			}

			for _, entry := range commandEntries {
				if entry.IsDir() {
					continue
				}

				content := assets.MustRead("opencode/commands/" + entry.Name())
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

	// 2b. OpenCode /sdd-* commands reference agent: sdd-orchestrator.
	// Ensure that agent is present even when persona component is not installed.
	//
	// mergedSettingsBytes holds the final merged opencode.json bytes produced by
	// mergeJSONFile. We keep them in memory so the post-check (step 4) can validate
	// the merge result without re-reading from disk — on Windows/WSL2, the atomic
	// rename (temp → target) may not be immediately visible to a subsequent
	// os.ReadFile call due to VFS/NTFS metadata caching, which caused the spurious
	// "post-check: .../opencode.json missing sdd-apply sub-agent" error.
	var mergedSettingsBytes []byte
	if adapter.Agent() == model.AgentOpenCode {
		settingsPath := adapter.SettingsPath(homeDir)
		if settingsPath != "" {
			overlayContent, err := assets.Read(overlayAssetPath(sddMode))
			if err != nil {
				return InjectionResult{}, fmt.Errorf("read SDD overlay asset: %w", err)
			}

			// Inject model assignments into the overlay before merging.
			overlayBytes := []byte(overlayContent)
			var assignments map[string]model.ModelAssignment
			if len(modelAssignments) > 0 {
				assignments = modelAssignments[0]
			}
			if sddMode == model.SDDModeMulti && len(assignments) > 0 {
				overlayBytes, err = injectModelAssignments(overlayBytes, assignments)
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
			pluginResult, err := installOpenCodePlugins(homeDir)
			if err != nil {
				return InjectionResult{}, err
			}
			changed = changed || pluginResult.Changed
			files = append(files, pluginResult.Files...)
		}
	}

	// 3. Write SDD skill files (if the agent supports skills).
	if adapter.SupportsSkills() {
		skillDir := adapter.SkillsDir(homeDir)
		if skillDir != "" {
			sharedFiles := []string{
				"persistence-contract.md",
				"engram-convention.md",
				"openspec-convention.md",
				"sdd-phase-common.md",
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
			}

			for _, skill := range sddSkills {
				assetPath := "skills/" + skill + "/SKILL.md"
				content, readErr := assets.Read(assetPath)
				if readErr != nil {
					return InjectionResult{}, fmt.Errorf("required SDD skill %q: embedded asset not found: %w", skill, readErr)
				}
				if len(content) == 0 {
					return InjectionResult{}, fmt.Errorf("required SDD skill %q: embedded asset is empty", skill)
				}

				path := filepath.Join(skillDir, skill, "SKILL.md")
				writeResult, err := filemerge.WriteFileAtomic(path, []byte(content), 0o644)
				if err != nil {
					return InjectionResult{}, err
				}

				changed = changed || writeResult.Changed
				files = append(files, path)
			}
		}
	}

	// 4. Post-injection verification — catch silent failures.
	// Validate against the in-memory merged bytes rather than re-reading from
	// disk to avoid false negatives on Windows/WSL2 where a freshly-renamed
	// file may not be immediately readable via os.ReadFile.
	if adapter.Agent() == model.AgentOpenCode && len(mergedSettingsBytes) > 0 {
		settingsText := string(mergedSettingsBytes)
		if !strings.Contains(settingsText, `"sdd-orchestrator"`) {
			settingsPath := adapter.SettingsPath(homeDir)
			return InjectionResult{}, fmt.Errorf("post-check: %q missing sdd-orchestrator agent definition — OpenCode /sdd-* commands will fail", settingsPath)
		}
		if sddMode == model.SDDModeMulti && !strings.Contains(settingsText, `"sdd-apply"`) {
			settingsPath := adapter.SettingsPath(homeDir)
			return InjectionResult{}, fmt.Errorf("post-check: %q missing sdd-apply sub-agent — multi-mode overlay was not injected correctly", settingsPath)
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

// installOpenCodePlugins copies the background-agents plugin and installs its
// npm/bun dependency into ~/.config/opencode/. Returns an error with an
// actionable message if the package manager is present but the install fails.
// If no package manager is available, the install is skipped (soft failure).
func installOpenCodePlugins(homeDir string) (InjectionResult, error) {
	opencodeDir := filepath.Join(homeDir, ".config", "opencode")
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

	// If the SDD orchestrator section is already present (e.g., from the
	// gentleman persona asset which includes it), skip to avoid duplication.
	if hasSDDOrchestrator(existing) {
		return InjectionResult{Files: []string{promptPath}}, nil
	}

	if adapter.SystemPromptStrategy() == model.StrategyInstructionsFile && strings.TrimSpace(existing) == "" {
		existing = instructionsFrontmatter
	}

	// Use agent-specific SDD orchestrator content when available; fall back to generic.
	content := assets.MustRead(sddOrchestratorAsset(adapter.Agent()))

	updated := existing
	if len(updated) > 0 && !strings.HasSuffix(updated, "\n") {
		updated += "\n"
	}
	if len(updated) > 0 {
		updated += "\n"
	}
	updated += content

	writeResult, err := filemerge.WriteFileAtomic(promptPath, []byte(updated), 0o644)
	if err != nil {
		return InjectionResult{}, err
	}

	return InjectionResult{Changed: writeResult.Changed, Files: []string{promptPath}}, nil
}

const instructionsFrontmatter = "---\n" +
	"name: Gentle AI Persona\n" +
	"description: Gentleman persona with SDD orchestration and Engram protocol\n" +
	"applyTo: \"**\"\n" +
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

func injectMarkdownSections(homeDir string, adapter agents.Adapter) (InjectionResult, error) {
	promptPath := adapter.SystemPromptFile(homeDir)
	content := assets.MustRead("claude/sdd-orchestrator.md")

	existing, err := readFileOrEmpty(promptPath)
	if err != nil {
		return InjectionResult{}, err
	}

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

// injectModelAssignments injects "model" fields into sub-agent definitions
// within the overlay JSON before it is merged into the settings file.
func injectModelAssignments(overlayBytes []byte, assignments map[string]model.ModelAssignment) ([]byte, error) {
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

	for phase, assignment := range assignments {
		if assignment.ProviderID == "" || assignment.ModelID == "" {
			continue
		}
		agentDef, exists := agents[phase]
		if !exists {
			continue
		}
		agentMap, ok := agentDef.(map[string]any)
		if !ok {
			continue
		}
		agentMap["model"] = assignment.FullID()
	}

	result, err := json.MarshalIndent(overlay, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal overlay after model injection: %w", err)
	}
	return append(result, '\n'), nil
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
