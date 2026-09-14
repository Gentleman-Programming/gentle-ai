package sdd

import (
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/agentguidance"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

const (
	claudeCodeGraphToolGrant = "mcp__codegraph__codegraph_explore"
	kiroCodeGraphToolGrant   = "@codegraph"
)

// readSkillContent reads the embedded skill content for the given phase.
func readSkillContent(phase string) (string, error) {
	return assets.Read("skills/" + phase + "/SKILL.md")
}

// SharedPromptDir returns the shared SDD prompt directory beside OpenCode's
// settings file, including when XDG_CONFIG_HOME overrides the default root.
func SharedPromptDir(homeDir string) string {
	return filepath.Join(opencode.ConfigPath(homeDir), "prompts", "sdd")
}

// SharedPromptFileRef returns a prompt file reference relative to the settings
// file that will contain it.
func SharedPromptFileRef(settingsPath, homeDir, phase string) (string, error) {
	return sharedPromptFileRef(settingsPath, homeDir, phase, filepath.Rel)
}

func sharedPromptFileRef(settingsPath, homeDir, phase string, rel func(string, string) (string, error)) (string, error) {
	promptPath := filepath.Join(SharedPromptDir(homeDir), phase+".md")
	relativePath, err := rel(filepath.Dir(settingsPath), promptPath)
	if err != nil {
		return "{file:" + filepath.ToSlash(promptPath) + "}", nil
	}
	relativePath = filepath.ToSlash(relativePath)
	if !strings.HasPrefix(relativePath, ".") {
		relativePath = "./" + relativePath
	}
	return "{file:" + relativePath + "}", nil
}

// subAgentPhaseOrder is an alias for profilePhaseOrder (defined in profiles.go),
// kept for backward compatibility with any code in this file that references it.
// Both variables are in the same package and represent the same canonical list.
var subAgentPhaseOrder = profilePhaseOrder

// SharedPromptPhases returns the ordered list of phase names that have shared
// prompt files in SharedPromptDir(). Used by backup target enumeration and any
// caller that needs to enumerate all prompt files without importing internal vars.
func SharedPromptPhases() []string {
	return ProfilePhaseOrder()
}

// sharedPromptPhaseCapabilities derives the per-phase model capability map for
// one run: global assignments first, then named profile assignments (profiles
// win, mirroring resolveProfileAssignment precedence). WriteSharedPromptFiles
// and the research projection verification consume this same map, so the bytes
// written and the bytes verified cannot disagree.
func sharedPromptPhaseCapabilities(assignments map[string]model.ModelAssignment, profiles []model.Profile) map[string]string {
	phaseCapabilities := make(map[string]string)
	for phase, assignment := range assignments {
		phaseCapabilities[phase] = model.ModelCapability(assignment.ModelID)
	}
	for _, profile := range profiles {
		for phase, assignment := range profile.PhaseAssignments {
			if assignment.ModelID != "" {
				phaseCapabilities[phase] = model.ModelCapability(assignment.ModelID)
			}
		}
	}
	return phaseCapabilities
}

// sharedPromptCapability resolves the effective render capability for one phase
// with the same defaulting WriteSharedPromptFiles applies.
func sharedPromptCapability(phaseCapabilities map[string]string, phase string) string {
	if capability, ok := phaseCapabilities[phase]; ok && capability != "" {
		return capability
	}
	return "capable"
}

// renderSharedPromptFile renders the exact bytes one shared prompt file will
// carry, plus its canonical path. Both WriteSharedPromptFiles and the research
// projection verification call this renderer with the same per-run capability
// map, and inject writes the files whenever a named profile overlay can
// reference them, so the bytes loaded at runtime and the bytes verified cannot
// drift (#4088).
func renderSharedPromptFile(homeDir, phase, capability, codeGraphGuidance string) (string, string, error) {
	skillContent, err := readSkillContent(phase)
	if err != nil {
		return "", "", err
	}
	content := extractModelSection(skillContent, capability)
	content = injectCodeGraphGuidanceIntoPrompt(content, codeGraphGuidance)
	// OpenCode phases reference these shared files via {file:...}
	// indirection, which the in-settings injection deliberately skips —
	// the contract must land here or those executors would miss it.
	content = injectLanguageContractIntoPrompt(content)
	content = agentguidance.InjectRemoteAuthorization(content)
	path := filepath.Join(SharedPromptDir(homeDir), phase+".md")
	return path, content, nil
}

// WriteSharedPromptFiles writes the 10 SDD sub-agent prompt files to
// {homeDir}/.config/opencode/prompts/sdd/. The content for each phase is extracted
// from the embedded skill file, filtered to the section matching the phase's
// model capability ("capable" or "small").
//
// The phaseCapabilities map controls which section is extracted per phase:
//   - "capable" sections are used for high-capability models
//   - "small" sections are used for small/fast models (e.g., flash, mini)
//   - If a phase is missing from the map, "capable" is used as default
//
// Returns (true, nil) if any file was created or changed, (false, nil) if all
// files already match (idempotent). Uses WriteFileAtomic so the operation is
// safe to repeat.
func WriteSharedPromptFiles(homeDir string, phaseCapabilities map[string]string, codeGraphGuidance ...string) (bool, error) {
	anyChanged := false
	guidance := ""
	if len(codeGraphGuidance) > 0 {
		guidance = codeGraphGuidance[0]
	}

	for _, phase := range subAgentPhaseOrder {
		path, content, err := renderSharedPromptFile(homeDir, phase, sharedPromptCapability(phaseCapabilities, phase), guidance)
		if err != nil {
			return false, err
		}
		result, err := filemerge.WriteFileAtomic(path, []byte(content), 0o644)
		if err != nil {
			return false, err
		}

		if result.Changed {
			anyChanged = true
		}
	}

	return anyChanged, nil
}

func injectCodeGraphGuidanceIntoPrompt(prompt, guidance string) string {
	if strings.TrimSpace(guidance) == "" {
		return prompt
	}
	return filemerge.InjectMarkdownSection(prompt, "codegraph-guidance", guidance)
}

// agentLanguageContract returns the canonical executor language contract
// (issue #1702 defect 4). Single source of truth: injected into every
// rendered sub-agent prompt so executors spawned inside a non-English
// conversation never mimic its dialect when writing artifacts.
func agentLanguageContract() string {
	return assets.MustRead("generic/agent-language-contract.md")
}

// injectLanguageContractIntoPrompt appends the canonical language contract
// as a managed markdown section. Marker-bound, so re-rendering an already
// injected prompt is a no-op (same mechanism as the CodeGraph guidance).
func injectLanguageContractIntoPrompt(prompt string) string {
	return filemerge.InjectMarkdownSection(prompt, "agent-language-contract", strings.TrimSpace(agentLanguageContract()))
}

func injectCodeGraphToolGrantIntoPrompt(prompt string, agentID model.AgentID, guidance string) string {
	if strings.TrimSpace(guidance) == "" {
		return prompt
	}

	grant := ""
	switch agentID {
	case model.AgentClaudeCode:
		grant = claudeCodeGraphToolGrant
	case model.AgentKiroIDE:
		grant = kiroCodeGraphToolGrant
	default:
		return prompt
	}

	frontmatterEnd := strings.Index(prompt, "\n---\n")
	if frontmatterEnd < 0 {
		return prompt
	}
	lines := strings.Split(prompt[:frontmatterEnd], "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, "tools:") {
			continue
		}
		if strings.Contains(line, grant) {
			return prompt
		}
		// A deliberately empty tools contract (tool-free reviewers, issue
		// #3168/#3648) must stay empty: appending the grant would produce
		// unparseable frontmatter and contradict the agent's own contract.
		if value := strings.TrimSpace(strings.TrimPrefix(line, "tools:")); value == "" || value == "[]" {
			return prompt
		}
		if agentID == model.AgentClaudeCode {
			lines[i] = line + ", " + grant
		} else if strings.HasSuffix(line, "]") {
			lines[i] = strings.TrimSuffix(line, "]") + `, "` + grant + `"]`
		} else {
			return prompt
		}
		return strings.Join(lines, "\n") + prompt[frontmatterEnd:]
	}
	return prompt
}

func isMarkdownSubAgentPromptFile(fileName string) bool {
	if filepath.Ext(fileName) != ".md" {
		return false
	}
	return !strings.HasPrefix(filepath.Base(fileName), ".")
}

func injectCodeGraphGuidanceIntoOpenCodeSubagentPrompts(agentMap map[string]any, guidance string) {
	if strings.TrimSpace(guidance) == "" {
		return
	}
	for _, agentRaw := range agentMap {
		agent, ok := agentRaw.(map[string]any)
		if !ok {
			continue
		}
		if mode, _ := agent["mode"].(string); mode == "primary" {
			continue
		}
		permission, _ := agent["permission"].(map[string]any)
		if permission["bash"] == "deny" {
			continue
		}
		prompt, ok := agent["prompt"].(string)
		if !ok || strings.HasPrefix(prompt, "{file:") {
			continue
		}
		agent["prompt"] = injectCodeGraphGuidanceIntoPrompt(prompt, guidance)
	}
}

// injectRemoteAuthorizationIntoSubagentPrompts covers inline executors, including
// tool-free reviewers. Primary routing and shared prompt files own their injection.
func injectRemoteAuthorizationIntoSubagentPrompts(agentMap map[string]any) {
	for _, raw := range agentMap {
		agent, ok := raw.(map[string]any)
		if !ok || agent["mode"] == "primary" {
			continue
		}
		prompt, ok := agent["prompt"].(string)
		if !ok || strings.HasPrefix(prompt, "{file:") {
			continue
		}
		agent["prompt"] = agentguidance.InjectRemoteAuthorization(prompt)
	}
}

// injectLanguageContractIntoOpenCodeSubagentPrompts applies the canonical
// executor language contract to OpenCode's JSON-embedded sub-agent prompts.
// Primary-mode agents keep the persona channel's own rules; {file:...}
// indirections are resolved elsewhere and stay untouched.
func injectLanguageContractIntoOpenCodeSubagentPrompts(agentMap map[string]any) {
	for _, agentRaw := range agentMap {
		agent, ok := agentRaw.(map[string]any)
		if !ok {
			continue
		}
		if mode, _ := agent["mode"].(string); mode == "primary" {
			continue
		}
		prompt, ok := agent["prompt"].(string)
		if !ok || strings.HasPrefix(prompt, "{file:") {
			continue
		}
		agent["prompt"] = injectLanguageContractIntoPrompt(prompt)
	}
}
