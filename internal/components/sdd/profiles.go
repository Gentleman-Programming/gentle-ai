package sdd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/assets"
	"github.com/gentleman-programming/gentle-ai/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

// profileNameRegex matches valid profile name slugs: lowercase alphanumeric + hyphens,
// must start and end with alphanumeric character (no trailing hyphens).
var profileNameRegex = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// reservedProfileNames are names that may not be used as profile names.
var reservedProfileNames = map[string]bool{
	"default":          true,
	"sdd-orchestrator": true,
}

// ValidateProfileName returns an error if the profile name is not a valid
// slug (lowercase alphanumeric + hyphens, no underscores, no spaces, non-empty,
// not a reserved word). Profile names are expected to already be lowercased by
// the TUI before reaching this function.
func ValidateProfileName(name string) error {
	if name == "" {
		return fmt.Errorf("profile name must not be empty")
	}
	if reservedProfileNames[name] {
		return fmt.Errorf("profile name %q is reserved", name)
	}
	if !profileNameRegex.MatchString(name) {
		return fmt.Errorf("profile name %q must match ^[a-z0-9]([a-z0-9-]*[a-z0-9])?$ (lowercase, hyphens only, no trailing hyphens, no underscores or spaces)", name)
	}
	return nil
}

// profilePhaseOrder defines the SDD sub-agent phases for profile generation.
// This is the canonical source of truth — prompts.go and profile_delete.go
// both derive from this via ProfilePhaseOrder().
var profilePhaseOrder = []string{
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

// ProfilePhaseOrder returns the ordered list of SDD sub-agent phase names.
// Use this instead of duplicating the slice in other packages.
func ProfilePhaseOrder() []string {
	return append([]string(nil), profilePhaseOrder...)
}

// ProfileAgentKeys returns the 11 agent keys for the given profile name.
// When name is empty, it returns the default (unsuffixed) keys.
// When name is non-empty, each key is suffixed with "-{name}".
func ProfileAgentKeys(name string) []string {
	suffix := ""
	if name != "" {
		suffix = "-" + name
	}

	keys := make([]string, 0, 11)
	keys = append(keys, "sdd-orchestrator"+suffix)
	for _, phase := range profilePhaseOrder {
		keys = append(keys, phase+suffix)
	}
	return keys
}

// DetectProfiles reads opencode.json at settingsPath and returns all named
// SDD profiles found in the agent map. The default profile (bare sdd-orchestrator
// without suffix) is NOT included in the result. Returns an empty slice if the
// file does not exist or contains no named profiles. Results are sorted by name.
func DetectProfiles(settingsPath string) ([]model.Profile, error) {
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []model.Profile{}, nil
		}
		return nil, fmt.Errorf("read settings %q: %w", settingsPath, err)
	}

	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parse settings %q: %w", settingsPath, err)
	}

	agentRaw, ok := root["agent"]
	if !ok {
		return []model.Profile{}, nil
	}
	agentMap, ok := agentRaw.(map[string]any)
	if !ok {
		return []model.Profile{}, nil
	}

	// Scan for sdd-orchestrator-{name} keys (exclude bare sdd-orchestrator).
	const orchPrefix = "sdd-orchestrator-"
	profileNames := make([]string, 0)
	seen := make(map[string]bool)
	for key := range agentMap {
		if !strings.HasPrefix(key, orchPrefix) {
			continue
		}
		profileName := key[len(orchPrefix):]
		if profileName == "" || seen[profileName] {
			continue
		}
		seen[profileName] = true
		profileNames = append(profileNames, profileName)
	}

	if len(profileNames) == 0 {
		return []model.Profile{}, nil
	}

	sort.Strings(profileNames)

	profiles := make([]model.Profile, 0, len(profileNames))
	for _, profileName := range profileNames {
		orchKey := "sdd-orchestrator-" + profileName
		orchRaw := agentMap[orchKey]
		orchMap, _ := orchRaw.(map[string]any)

		orchModel := extractModelFromAgent(orchMap)
		phaseAssignments := make(map[string]model.ModelAssignment)
		for _, phase := range profilePhaseOrder {
			agentKey := phase + "-" + profileName
			agentRaw := agentMap[agentKey]
			agentMap2, _ := agentRaw.(map[string]any)
			if m := extractModelFromAgent(agentMap2); m.ProviderID != "" {
				phaseAssignments[phase] = m
			}
		}

		profiles = append(profiles, model.Profile{
			Name:              profileName,
			OrchestratorModel: orchModel,
			PhaseAssignments:  phaseAssignments,
		})
	}

	return profiles, nil
}

// extractModelFromAgent reads the "model" field from an agent definition map
// and parses it into a ModelAssignment. Returns zero-value if missing or malformed.
func extractModelFromAgent(agentMap map[string]any) model.ModelAssignment {
	if agentMap == nil {
		return model.ModelAssignment{}
	}
	modelStr, _ := agentMap["model"].(string)
	if modelStr == "" {
		return model.ModelAssignment{}
	}

	// Try colon separator first (standard: "anthropic:claude-sonnet-4"), then slash.
	idx := strings.Index(modelStr, ":")
	if idx <= 0 {
		idx = strings.Index(modelStr, "/")
	}
	if idx <= 0 {
		return model.ModelAssignment{}
	}
	providerID := modelStr[:idx]
	modelID := modelStr[idx+1:]
	if modelID == "" {
		return model.ModelAssignment{}
	}
	return model.ModelAssignment{ProviderID: providerID, ModelID: modelID}
}

// GenerateProfileOverlay builds an OpenCode agent overlay JSON for the given
// profile. The overlay contains 11 agent definitions:
//   - sdd-orchestrator-{name}: primary mode, inlined orchestrator prompt (with suffixed
//     sub-agent references and model assignments table), permissions scoped to *-{name}
//   - sdd-{phase}-{name} (10 agents): subagent mode, hidden, file reference to
//     the shared prompt at SharedPromptDir(homeDir)/sdd-{phase}.md
func GenerateProfileOverlay(profile model.Profile, homeDir string) ([]byte, error) {
	if profile.Name == "" || profile.Name == "default" {
		return nil, fmt.Errorf("GenerateProfileOverlay: profile name must be non-empty and not 'default'")
	}

	suffix := "-" + profile.Name
	orchestratorKey := "sdd-orchestrator" + suffix

	// Build the orchestrator prompt: start with the base asset, inject model
	// assignments table, then suffix sub-agent references.
	orchestratorPrompt, err := buildProfileOrchestratorPrompt(profile)
	if err != nil {
		return nil, fmt.Errorf("build orchestrator prompt for profile %q: %w", profile.Name, err)
	}

	// Build the agent map.
	agentMap := make(map[string]any, 11)

	// Orchestrator entry
	orchEntry := map[string]any{
		"mode":        "primary",
		"description": "Agent Teams Orchestrator (" + profile.Name + " profile) - coordinates sub-agents, never does work inline",
		"prompt":      orchestratorPrompt,
		"permission": map[string]any{
			"task": map[string]any{
				"*":              "deny",
				"sdd-*" + suffix: "allow",
			},
		},
		"tools": map[string]any{
			"read":            true,
			"write":           true,
			"edit":            true,
			"bash":            true,
			"delegate":        true,
			"delegation_read": true,
			"delegation_list": true,
		},
	}
	if profile.OrchestratorModel.ProviderID != "" && profile.OrchestratorModel.ModelID != "" {
		orchEntry["model"] = profile.OrchestratorModel.FullID()
	}
	agentMap[orchestratorKey] = orchEntry

	// Sub-agent entries
	promptDir := SharedPromptDir(homeDir)
	phaseDescriptions := map[string]string{
		"sdd-init":    "Bootstrap SDD context and project configuration",
		"sdd-explore": "Investigate codebase and think through ideas",
		"sdd-propose": "Create change proposals from explorations",
		"sdd-spec":    "Write detailed specifications from proposals",
		"sdd-design":  "Create technical design from proposals",
		"sdd-tasks":   "Break down specs and designs into implementation tasks",
		"sdd-apply":   "Implement code changes from task definitions",
		"sdd-verify":  "Validate implementation against specs",
		"sdd-archive": "Archive completed change artifacts",
		"sdd-onboard": "Guide user through a complete SDD cycle using their real codebase",
	}

	for _, phase := range profilePhaseOrder {
		key := phase + suffix
		entry := map[string]any{
			"mode":        "subagent",
			"hidden":      true,
			"description": phaseDescriptions[phase],
			"prompt":      "{file:" + filepath.Join(promptDir, phase+".md") + "}",
			"tools": map[string]any{
				"read":  true,
				"write": true,
				"edit":  true,
				"bash":  true,
			},
		}
		if assignment, ok := profile.PhaseAssignments[phase]; ok && assignment.ProviderID != "" && assignment.ModelID != "" {
			entry["model"] = assignment.FullID()
		}
		agentMap[key] = entry
	}

	overlay := map[string]any{
		"agent": agentMap,
	}

	result, err := json.MarshalIndent(overlay, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal profile overlay: %w", err)
	}
	return append(result, '\n'), nil
}

// buildProfileOrchestratorPrompt constructs the orchestrator prompt for a named
// profile. It:
//  1. Reads the base generic/sdd-orchestrator.md asset
//  2. Injects a model assignments table reflecting the profile's models
//  3. Replaces bare sub-agent references (e.g. sdd-init) with suffixed ones
//     (e.g. sdd-init-{name}) in the prompt text
func buildProfileOrchestratorPrompt(profile model.Profile) (string, error) {
	base := assets.MustRead("generic/sdd-orchestrator.md")

	// Inject model assignments table.
	const openMarker = "<!-- gentle-ai:sdd-model-assignments -->"
	const closeMarker = "<!-- /gentle-ai:sdd-model-assignments -->"

	start := strings.Index(base, openMarker)
	end := strings.Index(base, closeMarker)
	if start != -1 && end != -1 && end > start {
		table := renderProfileModelAssignmentsSection(profile)
		afterOpen := start + len(openMarker)
		base = base[:afterOpen] + "\n" + table + base[end:]
	}

	// Replace sub-agent references in the prompt text so the orchestrator
	// delegates to the suffixed agents (e.g. sdd-init-cheap instead of sdd-init).
	suffix := "-" + profile.Name
	for _, phase := range profilePhaseOrder {
		// Replace whole-word phase names to avoid partial replacements.
		// We wrap with known boundaries: space, backtick, single-quote, newline, slash.
		// Use a simple but safe approach: replace "sdd-{phase}" not already suffixed.
		base = replacePhaseRef(base, phase, phase+suffix)
	}
	// Also replace the orchestrator self-reference.
	base = replacePhaseRef(base, "sdd-orchestrator", "sdd-orchestrator"+suffix)

	return base, nil
}

// replacePhaseRef replaces occurrences of 'from' with 'to' in content.
// We only replace when 'from' appears as a bounded reference (not already part of
// a longer identifier). This uses the fact that phase names in the prompt appear
// after specific delimiters.
func replacePhaseRef(content, from, to string) string {
	// Skip if 'to' already appears (avoid double-replacement on re-runs).
	// We do a simple strings.Replace that replaces all non-suffixed occurrences.
	// Since 'to' = 'from' + suffix, and 'from' is a prefix of 'to', we need
	// to ensure we don't replace occurrences that are already 'to'.
	// Strategy: replace from→to only when not followed by the suffix itself.
	// Implemented via iterating and checking ahead.
	suffix := strings.TrimPrefix(to, from)
	if suffix == "" {
		return content
	}

	var sb strings.Builder
	remaining := content
	for {
		idx := strings.Index(remaining, from)
		if idx < 0 {
			sb.WriteString(remaining)
			break
		}
		// Check if already suffixed at this position.
		afterIdx := idx + len(from)
		if afterIdx <= len(remaining) && strings.HasPrefix(remaining[afterIdx:], suffix) {
			// Already suffixed — emit 'to' and skip past it.
			sb.WriteString(remaining[:afterIdx])
			remaining = remaining[afterIdx:]
			continue
		}
		sb.WriteString(remaining[:idx])
		sb.WriteString(to)
		remaining = remaining[afterIdx:]
	}
	return sb.String()
}

// renderProfileModelAssignmentsSection renders the model assignments table for
// a named profile using the profile's model assignments.
func renderProfileModelAssignmentsSection(profile model.Profile) string {
	var b strings.Builder
	b.WriteString("## Model Assignments\n\n")
	b.WriteString("Read this table at session start (or before first delegation), cache it for the session, and pass the mapped alias in every Agent tool call via the `model` parameter. If a phase is missing, use the `default` row. If you lack access to the assigned model, substitute the next tier down and continue.\n\n")
	b.WriteString("| Phase | Model | Reason |\n")
	b.WriteString("|-------|-------|--------|\n")

	// Orchestrator row
	orchModel := "—"
	if profile.OrchestratorModel.ProviderID != "" {
		orchModel = profile.OrchestratorModel.FullID()
	}
	b.WriteString(fmt.Sprintf("| orchestrator | %s | Coordinates, makes decisions |\n", orchModel))

	// Phase rows
	phaseReasons := map[string]string{
		"sdd-init":    "Bootstrap SDD context",
		"sdd-explore": "Reads code, structural - not architectural",
		"sdd-propose": "Architectural decisions",
		"sdd-spec":    "Structured writing",
		"sdd-design":  "Architecture decisions",
		"sdd-tasks":   "Mechanical breakdown",
		"sdd-apply":   "Implementation",
		"sdd-verify":  "Validation against spec",
		"sdd-archive": "Copy and close",
		"sdd-onboard": "Guided walkthrough",
	}

	for _, phase := range profilePhaseOrder {
		phaseModel := "—"
		if m, ok := profile.PhaseAssignments[phase]; ok && m.ProviderID != "" {
			phaseModel = m.FullID()
		}
		reason := phaseReasons[phase]
		b.WriteString(fmt.Sprintf("| %s | %s | %s |\n", phase, phaseModel, reason))
	}
	b.WriteString("\n")
	return b.String()
}

// RemoveProfileAgents reads the opencode.json at settingsPath, removes all 11
// agent keys belonging to the named profile (sdd-orchestrator-{name} and
// sdd-{phase}-{name}), and atomically writes the result back.
//
// Returns an error if name is empty or "default" (cannot remove the default profile).
// If the profile's agent keys are not present, the operation is a no-op (no error).
func RemoveProfileAgents(settingsPath string, profileName string) error {
	if profileName == "" || profileName == "default" {
		return fmt.Errorf("RemoveProfileAgents: cannot remove default profile (name=%q)", profileName)
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No-op: file doesn't exist
		}
		return fmt.Errorf("read settings %q: %w", settingsPath, err)
	}

	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("parse settings %q: %w", settingsPath, err)
	}

	agentRaw, ok := root["agent"]
	if !ok {
		return nil // No-op: no agent section
	}
	agentMap, ok := agentRaw.(map[string]any)
	if !ok {
		return nil // No-op: malformed
	}

	// Delete the 11 profile keys, tracking how many were actually present.
	keysToDelete := ProfileAgentKeys(profileName)
	deleted := 0
	for _, key := range keysToDelete {
		if _, exists := agentMap[key]; exists {
			delete(agentMap, key)
			deleted++
		}
	}

	// If no keys were found and deleted, the profile doesn't exist — no-op.
	// Returning early avoids re-serializing the JSON, which would change key
	// ordering and trigger false change detection on subsequent reads.
	if deleted == 0 {
		return nil
	}

	root["agent"] = agentMap
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	out = append(out, '\n')

	_, err = filemerge.WriteFileAtomic(settingsPath, out, 0o644)
	return err
}
