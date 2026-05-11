package vscode

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/assets"
	"github.com/gentleman-programming/gentle-ai/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

// vscModelEntries maps model ID substrings to VS Code Copilot display names.
// Unknown models fall back to "ProviderID/ModelID" for traceability.
// Empty model ID means no model field — Copilot uses its default.
// Entries are checked in order; longer/more-specific substrings must come first
// to avoid partial matches. Critical pairs today:
//   - "gpt-4o-mini" before "gpt-4o"
//   - "gpt-4.1-mini" before "gpt-4.1"
//
// Future risk: when a new Claude Sonnet (e.g. "claude-sonnet-4-5") or any other
// versioned successor lands, place the more specific entry BEFORE the broader
// "claude-sonnet-4" entry — otherwise the broader substring wins.
var vscModelEntries = []struct {
	substr  string
	display string
}{
	{"claude-sonnet-4", "Claude Sonnet 4 (copilot)"},
	{"claude-opus-4-5", "Claude Opus 4.5 (copilot)"},
	{"claude-haiku-4-5", "Claude Haiku 4.5 (copilot)"},
	{"gemini-2.5-pro", "Gemini 2.5 Pro (copilot)"},
	{"gemini-2.5-flash", "Gemini 2.5 Flash (copilot)"},
	{"gpt-4.1-mini", "GPT 4.1 Mini (copilot)"},
	{"gpt-4o-mini", "GPT 4o Mini (copilot)"},
	{"gpt-4.1", "GPT 4.1 (copilot)"},
	{"gpt-4o", "GPT 4o (copilot)"},
}

// VSCodeModelID maps a ModelAssignment (provider/model) to a VS Code Copilot
// display name. Known models get friendly display names; unknown models get
// the full ProviderID/ModelID as a fallback. Empty ModelID returns empty string
// (meaning: omit the model field entirely — Copilot uses its default).
func VSCodeModelID(m model.ModelAssignment) string {
	if m.ModelID == "" {
		return ""
	}
	for _, entry := range vscModelEntries {
		if strings.Contains(m.ModelID, entry.substr) {
			return entry.display
		}
	}
	// Fallback: use full qualified ID for traceability
	return m.ProviderID + "/" + m.ModelID
}

// SDD phases in canonical order (excludes orchestrator, which is handled specially).
var sddPhases = []string{
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

// sddPhaseDescriptions provides short descriptions for each SDD phase agent.
var sddPhaseDescriptions = map[string]string{
	"sdd-init":      "Initialize SDD context for the project",
	"sdd-explore":   "Investigate ideas and approaches before committing to a change",
	"sdd-propose":   "Draft a change proposal with intent and scope",
	"sdd-spec":      "Write requirements and acceptance scenarios",
	"sdd-design":    "Write architecture and file-change design",
	"sdd-tasks":     "Break down a change into implementation task checklist",
	"sdd-apply":     "Implement code changes from task definitions",
	"sdd-verify":    "Validate implementation against specs and design",
	"sdd-archive":   "Sync delta specs and archive completed change",
	"sdd-onboard":   "Guided end-to-end SDD walkthrough",
}

// OrchestratorPhase is the name of the orchestrator agent that coordinates
// dispatch to the 10 SDD phase executors. It must be in sync with the
// embedded template at internal/assets/vscode/agents/sdd-orchestrator.agent.md
// and with the OpenCode SDDOrchestratorPhase constant.
const OrchestratorPhase = "sdd-orchestrator"

// GenerateAgentFile produces .agent.md content with YAML frontmatter and markdown
// body for a VS Code Copilot agent. The profile name is used to suffix the agent
// name for named profiles (e.g., "sdd-apply-cheap"), and omitted for the default
// profile. When phase == OrchestratorPhase, the orchestrator template is used
// (with `tools: ['agent']`, an `agents:` whitelist, and `user-invocable: true`).
// All other phases produce phase executor agents with `user-invocable: false`.
func GenerateAgentFile(phase string, profile model.Profile) string {
	if phase == OrchestratorPhase {
		return generateOrchestratorAgent(profile)
	}

	agentName := phase
	if profile.Name != "" && profile.Name != "default" {
		agentName = phase + "-" + profile.Name
	}

	description := sddPhaseDescriptions[phase]
	if description == "" {
		description = "SDD " + phase + " executor"
	}

	var sb strings.Builder
	sb.WriteString("---\n")
	fmt.Fprintf(&sb, "name: %s\n", agentName)
	fmt.Fprintf(&sb, "description: >\n  %s\n", description)

	if assignment, ok := profile.PhaseAssignments[phase]; ok {
		if modelID := VSCodeModelID(assignment); modelID != "" {
			fmt.Fprintf(&sb, "model: \"%s\"\n", modelID)
		}
	}

	sb.WriteString("readonly: false\n")
	sb.WriteString("background: false\n")
	sb.WriteString("user-invocable: false\n")
	sb.WriteString("---\n\n")

	fmt.Fprintf(&sb, "You are the SDD **%s** executor. Do this phase's work yourself. Do NOT delegate further.\n", phase)
	sb.WriteString("You are not the orchestrator. Do NOT call task/delegate. Do NOT launch sub-agents.\n\n")
	sb.WriteString("## Instructions\n\n")
	fmt.Fprintf(&sb, "Read the skill file at `~/.copilot/skills/sdd-%s/SKILL.md` and follow it exactly.\n", phaseWithoutPrefix(phase))
	sb.WriteString("Also read shared conventions at `~/.copilot/skills/_shared/sdd-phase-common.md`.\n")

	return sb.String()
}

// generateOrchestratorAgent renders the SDD orchestrator agent for a profile.
// The orchestrator has tools: ['agent'] and an `agents:` whitelist so VS Code
// Copilot's main chat agent can dispatch through it deterministically rather
// than inferring the SDD sequence from sub-agent descriptions alone.
func generateOrchestratorAgent(profile model.Profile) string {
	suffix := ""
	if profile.Name != "" && profile.Name != "default" {
		suffix = "-" + profile.Name
	}

	var sb strings.Builder
	sb.WriteString("---\n")
	fmt.Fprintf(&sb, "name: %s%s\n", OrchestratorPhase, suffix)
	sb.WriteString("description: >\n  SDD workflow orchestrator — coordinates the 10 SDD phase executors in a strict, deterministic sequence.\n")

	if profile.OrchestratorModel.ModelID != "" {
		if modelID := VSCodeModelID(profile.OrchestratorModel); modelID != "" {
			fmt.Fprintf(&sb, "model: \"%s\"\n", modelID)
		}
	}

	sb.WriteString("tools: ['agent']\n")
	sb.WriteString("agents:\n")
	for _, phase := range sddPhases {
		fmt.Fprintf(&sb, "  - %s%s\n", phase, suffix)
	}
	sb.WriteString("readonly: false\n")
	sb.WriteString("background: false\n")
	sb.WriteString("user-invocable: true\n")
	sb.WriteString("---\n\n")

	sb.WriteString("You are the SDD workflow orchestrator for the Gentleman AI ecosystem in VS Code Copilot.\n\n")
	sb.WriteString("Your job is to coordinate the SDD phase executors in a strict, deterministic sequence. ")
	sb.WriteString("You do NOT perform phase work yourself — you delegate to the matching `sdd-*` sub-agent and synthesize their results back to the user.\n\n")

	sb.WriteString("## SDD phase sequence — substantial changes\n\n")
	sb.WriteString("For any non-trivial change, drive the user through this exact sequence. Do NOT skip phases.\n\n")
	steps := []struct {
		num   int
		phase string
		desc  string
	}{
		{1, "sdd-explore", "Survey the codebase, gather context, compare approaches. No files written yet."},
		{2, "sdd-propose", "Draft a change proposal with intent, scope, and approach."},
		{3, "sdd-spec", "Write requirements and acceptance scenarios derived from the proposal."},
		{4, "sdd-design", "Document the technical design and file-change plan."},
		{5, "sdd-tasks", "Break the change into an ordered task checklist."},
		{6, "sdd-apply", "Implement the tasks. When Strict TDD is enabled, the executor follows Red-Green-Refactor."},
		{7, "sdd-verify", "Validate the implementation against spec/design/tasks. Reports CRITICAL / WARNING / SUGGESTION findings."},
		{8, "sdd-archive", "Sync delta specs into the main spec set and close the change."},
	}
	for _, s := range steps {
		fmt.Fprintf(&sb, "%d. Delegate to `%s%s` — %s\n", s.num, s.phase, suffix, s.desc)
	}
	sb.WriteString("\n## SDD utility flows\n\n")
	fmt.Fprintf(&sb, "- Delegate to `sdd-init%s` when the project has not yet been initialized for SDD.\n", suffix)
	fmt.Fprintf(&sb, "- Delegate to `sdd-onboard%s` when the user asks for a guided end-to-end SDD walkthrough.\n\n", suffix)

	sb.WriteString("## Dispatch rules\n\n")
	sb.WriteString("1. One phase at a time. Wait for the sub-agent to finish and return before dispatching the next phase.\n")
	sb.WriteString("2. No skipping. If the user asks to jump phases, push back and explain why each phase is non-negotiable for a substantial change.\n")
	sb.WriteString("3. Synthesize between phases. Give the user a one-line summary of what each phase produced before continuing.\n")
	sb.WriteString("4. Stop on risk. If a phase returns CRITICAL findings or blockers, stop the chain and ask the user how to proceed.\n")
	sb.WriteString("5. Pass forward, not back. Each phase reads prior artifacts via the persistence backend (Engram or OpenSpec). Pass topic keys / file paths, not artifact content.\n")

	return sb.String()
}

// phaseWithoutPrefix strips the "sdd-" prefix from a phase name for skill directory lookup.
func phaseWithoutPrefix(phase string) string {
	return strings.TrimPrefix(phase, "sdd-")
}

// SDDPhases returns the canonical list of SDD phases (10 phases, no orchestrator).
func SDDPhases() []string {
	result := make([]string, len(sddPhases))
	copy(result, sddPhases)
	return result
}

// GenerateVSCodeProfileFiles writes 11 .agent.md files (the orchestrator plus
// the 10 SDD phase executors) for a named VS Code profile to the agents
// directory. Returns the list of file paths that actually changed on disk.
// Default profile (name="" or name="default") is handled by the existing 3c
// block in inject.go and must NOT go through this function.
func GenerateVSCodeProfileFiles(profile model.Profile, agentsDir string) ([]string, error) {
	if profile.Name == "" || profile.Name == "default" {
		return nil, fmt.Errorf("GenerateVSCodeProfileFiles: default profile is handled by the generic sub-agent path, not profile generation")
	}

	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		return nil, fmt.Errorf("create agents dir: %w", err)
	}

	var files []string

	// Render orchestrator first so it appears at the top of any directory listing.
	allPhases := append([]string{OrchestratorPhase}, sddPhases...)
	for _, phase := range allPhases {
		content := GenerateAgentFile(phase, profile)
		fileName := phase + "-" + profile.Name + ".agent.md"
		outPath := filepath.Join(agentsDir, fileName)

		writeResult, err := filemerge.WriteFileAtomic(outPath, []byte(content), 0o644)
		if err != nil {
			return nil, fmt.Errorf("write profile agent %q: %w", fileName, err)
		}
		if writeResult.Changed {
			files = append(files, outPath)
		}
	}

	return files, nil
}

// RemoveVSCodeProfileAgents removes all sdd-*-{profileName}.agent.md files from
// the agents directory. Default profile (name="" or "default") MUST NOT be removed
// and returns an error. Missing files are silently skipped (no error).
// Non-gentle-ai files in the agents directory are NOT touched.
func RemoveVSCodeProfileAgents(agentsDir, profileName string) error {
	if profileName == "" || profileName == "default" {
		return fmt.Errorf("cannot remove default profile")
	}

	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // nothing to remove
		}
		return fmt.Errorf("read agents dir: %w", err)
	}

	suffix := "-" + profileName + ".agent.md"
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		// Only remove files matching sdd-*-{profileName}.agent.md pattern
		if strings.HasPrefix(entry.Name(), "sdd-") && strings.HasSuffix(entry.Name(), suffix) {
			path := filepath.Join(agentsDir, entry.Name())
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove %q: %w", path, err)
			}
		}
	}

	return nil
}

// DetectVSCodeProfiles scans agentsDir for sdd-{phase}-{name}.agent.md files
// and returns deduplicated, sorted []model.Profile. An empty or missing
// directory is not an error — callers treat it as "no profiles yet".
func DetectVSCodeProfiles(agentsDir string) ([]model.Profile, error) {
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read agents dir: %w", err)
	}

	seen := make(map[string]struct{})
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Must match sdd-{phase}-{profileName}.agent.md
		// Strategy: strip known phase prefixes and .agent.md suffix to extract profile name.
		profileName := extractProfileName(name)
		if profileName == "" {
			continue
		}
		seen[profileName] = struct{}{}
	}

	if len(seen) == 0 {
		return nil, nil
	}

	profiles := make([]model.Profile, 0, len(seen))
	for name := range seen {
		profiles = append(profiles, model.Profile{Name: name})
	}

	// Sort deterministically by name.
	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].Name < profiles[j].Name
	})

	return profiles, nil
}

// extractProfileName parses a filename of the form sdd-{phase}-{profileName}.agent.md
// and returns the profile name. Returns "" if the file does not match the pattern.
func extractProfileName(filename string) string {
	const suffix = ".agent.md"
	if !strings.HasSuffix(filename, suffix) {
		return ""
	}
	if !strings.HasPrefix(filename, "sdd-") {
		return ""
	}
	// Strip suffix
	base := filename[:len(filename)-len(suffix)]
	// Try to match sdd-{phase}-{name}: look for a known phase prefix
	for _, phase := range sddPhases {
		phasePrefix := phase + "-"
		if strings.HasPrefix(base, phasePrefix) {
			profileName := base[len(phasePrefix):]
			if profileName != "" {
				return profileName
			}
		}
	}
	return ""
}

// ReadVSCodeAgentTemplate reads an embedded .agent.md template by phase name.
func ReadVSCodeAgentTemplate(phase string) (string, error) {
	return assets.Read("vscode/agents/" + phase + ".agent.md")
}

// ListVSCodeAgentTemplates returns the list of embedded VS Code agent template files.
func ListVSCodeAgentTemplates() ([]string, error) {
	entries, err := fs.ReadDir(assets.FS, "vscode/agents")
	if err != nil {
		return nil, fmt.Errorf("read embedded vscode/agents dir: %w", err)
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	return names, nil
}