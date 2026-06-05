package sdd

import (
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/internal/assets"
	"github.com/gentleman-programming/gentle-ai/internal/components/filemerge"
)

// readSkillContent reads the embedded skill content for the given phase.
func readSkillContent(phase string) (string, error) {
	return assets.Read("skills/" + phase + "/SKILL.md")
}

// SharedPromptDir returns the directory where shared SDD prompt files are stored.
// The path is {homeDir}/.config/opencode/prompts/sdd.
func SharedPromptDir(homeDir string) string {
	return filepath.Join(homeDir, ".config", "opencode", "prompts", "sdd")
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
func WriteSharedPromptFiles(homeDir string, phaseCapabilities map[string]string) (bool, error) {
	promptDir := SharedPromptDir(homeDir)
	anyChanged := false

	for _, phase := range subAgentPhaseOrder {
		// Read the embedded skill content for this phase.
		skillContent, err := readSkillContent(phase)
		if err != nil {
			return false, err
		}

		// Determine which section to extract based on model capability.
		capability := "capable"
		if phaseCapabilities != nil {
			if cap, ok := phaseCapabilities[phase]; ok && cap != "" {
				capability = cap
			}
		}

		// Extract the section matching the capability (falls back to full content
		// if no matching section marker is found — correct behavior for phases
		// that don't yet have conditional sections).
		content := extractModelSection(skillContent, capability)

		path := filepath.Join(promptDir, phase+".md")
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
