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

// GenerateAgentFile produces .agent.md content with YAML frontmatter and markdown body
// for a VS Code Copilot sub-agent. The profile name is used to suffix the agent name
// for named profiles (e.g., "sdd-apply-cheap"), and omitted for the default profile.
func GenerateAgentFile(phase string, profile model.Profile) string {
	agentName := phase
	if profile.Name != "" && profile.Name != "default" {
		agentName = phase + "-" + profile.Name
	}

	description := sddPhaseDescriptions[phase]
	if description == "" {
		description = "SDD " + phase + " executor"
	}

	// Build YAML frontmatter
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("name: %s\n", agentName))
	sb.WriteString(fmt.Sprintf("description: >\n  %s\n", description))

	// Model resolution: if the phase has a model assignment, resolve it
	if assignment, ok := profile.PhaseAssignments[phase]; ok {
		modelID := VSCodeModelID(assignment)
		if modelID != "" {
			sb.WriteString(fmt.Sprintf("model: \"%s\"\n", modelID))
		}
	}
	// If no assignment, the model field is omitted — Copilot uses its default

	sb.WriteString("readonly: false\n")
	sb.WriteString("background: false\n")
	// Phase executors are NOT user-invocable — they are dispatched by the orchestrator
	sb.WriteString("user-invocable: false\n")
	sb.WriteString("---\n\n")

	// Markdown body — SDD phase executor instructions
	sb.WriteString(fmt.Sprintf("You are the SDD **%s** executor. Do this phase's work yourself. Do NOT delegate further.\n", phase))
	sb.WriteString("You are not the orchestrator. Do NOT call task/delegate. Do NOT launch sub-agents.\n\n")
	sb.WriteString("## Instructions\n\n")
	sb.WriteString(fmt.Sprintf("Read the skill file at `~/.copilot/skills/sdd-%s/SKILL.md` and follow it exactly.\n", phaseWithoutPrefix(phase)))
	sb.WriteString("Also read shared conventions at `~/.copilot/skills/_shared/sdd-phase-common.md`.\n")

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

// GenerateVSCodeProfileFiles writes 10 .agent.md files (one per SDD phase)
// for a named VS Code profile to the agents directory. Returns a list of written
// file paths. Default profile (name="" or name="default") is handled by the
// existing 3c block in inject.go and should NOT go through this function.
func GenerateVSCodeProfileFiles(profile model.Profile, agentsDir string) ([]string, error) {
	if profile.Name == "" || profile.Name == "default" {
		return nil, fmt.Errorf("GenerateVSCodeProfileFiles: default profile is handled by the generic sub-agent path, not profile generation")
	}

	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		return nil, fmt.Errorf("create agents dir: %w", err)
	}

	var files []string

	for _, phase := range sddPhases {
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

// VSCodeStaticModels returns the static list of VS Code Copilot model entries
// as (modelSubstr, displayName) pairs for the TUI model picker.
// The order matches vscModelEntries — most specific entries first.
func VSCodeStaticModels() []VSCodeModelEntry {
	result := make([]VSCodeModelEntry, len(vscModelEntries))
	for i, e := range vscModelEntries {
		result[i] = VSCodeModelEntry{ModelSubstr: e.substr, DisplayName: e.display}
	}
	return result
}

// VSCodeModelEntry is a public representation of one VS Code model option.
type VSCodeModelEntry struct {
	ModelSubstr string // model ID substring used for matching
	DisplayName string // human-friendly display name shown in the TUI
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