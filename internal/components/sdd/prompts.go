package sdd

import (
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/internal/components/filemerge"
)

// SharedPromptDir returns the directory where shared SDD prompt files are stored.
// The path is {homeDir}/.config/opencode/prompts/sdd.
func SharedPromptDir(homeDir string) string {
	return filepath.Join(homeDir, ".config", "opencode", "prompts", "sdd")
}

// subAgentPromptContent contains the inline prompt string for each SDD sub-agent phase.
// These are the executor-scoped prompts that tell each sub-agent to read its skill file
// and execute the phase work directly (not delegate).
var subAgentPromptContent = map[string]string{
	"sdd-init":    "You are an SDD executor for the init phase, not the orchestrator. Do this phase's work yourself. Do NOT delegate, Do NOT call task/delegate, and Do NOT launch sub-agents. Read your skill file at ~/.config/opencode/skills/sdd-init/SKILL.md and follow it exactly.",
	"sdd-explore": "You are an SDD executor for the explore phase, not the orchestrator. Do this phase's work yourself. Do NOT delegate, Do NOT call task/delegate, and Do NOT launch sub-agents. Read your skill file at ~/.config/opencode/skills/sdd-explore/SKILL.md and follow it exactly.",
	"sdd-propose": "You are an SDD executor for the propose phase, not the orchestrator. Do this phase's work yourself. Do NOT delegate, Do NOT call task/delegate, and Do NOT launch sub-agents. Read your skill file at ~/.config/opencode/skills/sdd-propose/SKILL.md and follow it exactly.",
	"sdd-spec":    "You are an SDD executor for the spec phase, not the orchestrator. Do this phase's work yourself. Do NOT delegate, Do NOT call task/delegate, and Do NOT launch sub-agents. Read your skill file at ~/.config/opencode/skills/sdd-spec/SKILL.md and follow it exactly.",
	"sdd-design":  "You are an SDD executor for the design phase, not the orchestrator. Do this phase's work yourself. Do NOT delegate, Do NOT call task/delegate, and Do NOT launch sub-agents. Read your skill file at ~/.config/opencode/skills/sdd-design/SKILL.md and follow it exactly.",
	"sdd-tasks":   "You are an SDD executor for the tasks phase, not the orchestrator. Do this phase's work yourself. Do NOT delegate, Do NOT call task/delegate, and Do NOT launch sub-agents. Read your skill file at ~/.config/opencode/skills/sdd-tasks/SKILL.md and follow it exactly.",
	"sdd-apply":   "You are an SDD executor for the apply phase, not the orchestrator. Do this phase's work yourself. Do NOT delegate, Do NOT call task/delegate, and Do NOT launch sub-agents. Read your skill file at ~/.config/opencode/skills/sdd-apply/SKILL.md and follow it exactly.",
	"sdd-verify":  "You are an SDD executor for the verify phase, not the orchestrator. Do this phase's work yourself. Do NOT delegate, Do NOT call task/delegate, and Do NOT launch sub-agents. Read your skill file at ~/.config/opencode/skills/sdd-verify/SKILL.md and follow it exactly.",
	"sdd-archive": "You are an SDD executor for the archive phase, not the orchestrator. Do this phase's work yourself. Do NOT delegate, Do NOT call task/delegate, and Do NOT launch sub-agents. Read your skill file at ~/.config/opencode/skills/sdd-archive/SKILL.md and follow it exactly.",
	"sdd-onboard": "You are an SDD executor for the onboard phase, not the orchestrator. Do this phase's work yourself. Do NOT delegate, Do NOT call task/delegate, and Do NOT launch sub-agents. Read your skill file at ~/.config/opencode/skills/sdd-onboard/SKILL.md and follow it exactly.",
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
// {homeDir}/.config/opencode/prompts/sdd/. Returns (true, nil) if any file
// was created or changed, (false, nil) if all files already match (idempotent).
// Uses WriteFileAtomic so the operation is safe to repeat.
func WriteSharedPromptFiles(homeDir string) (bool, error) {
	promptDir := SharedPromptDir(homeDir)
	anyChanged := false

	for _, phase := range subAgentPhaseOrder {
		content, ok := subAgentPromptContent[phase]
		if !ok {
			continue
		}

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
