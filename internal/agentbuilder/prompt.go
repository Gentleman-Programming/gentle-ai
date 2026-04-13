package agentbuilder

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

const systemPromptBase = `You are an expert AI agent skill designer for the Gentleman AI ecosystem.
Your task is to generate a complete SKILL.md file for a custom sub-agent skill.

The SKILL.md MUST include these exact sections in order:
1. # {Title} — A clear, descriptive title for the skill
2. ## Description — What this skill does and its purpose
3. ## Trigger — When to activate this skill (specific phrases, contexts, or conditions)
4. ## Instructions — Step-by-step instructions the agent must follow when this skill is active
5. ## Rules — Hard constraints and guardrails (what the agent must/must not do)
6. ## Examples — Concrete usage examples demonstrating the skill in action

Requirements:
- Write in clear, direct language that an AI agent can execute
- Instructions must be actionable and specific
- Rules must be unambiguous constraints
- Examples must be realistic and cover edge cases
- The Trigger section must be precise enough to avoid false activations

Engram Integration:
- After completing significant work triggered by this skill, the agent MUST call mem_save
- Include a "PROACTIVE SAVE TRIGGERS" list in the Instructions section
- Reference the Engram persistent memory protocol for cross-session continuity

Output ONLY the raw SKILL.md content, starting with "# {Title}".
Do NOT wrap the output in code fences or add any preamble.`

// ComposePrompt builds the full prompt sent to the generation engine.
// It combines the system instructions with the user's intent and optional SDD context.
func ComposePrompt(userInput string, sddConfig *SDDIntegration, installedAgents []model.AgentID) string {
	var sb strings.Builder

	sb.WriteString(systemPromptBase)
	sb.WriteString("\n\n")

	// Installed agents context.
	if len(installedAgents) > 0 {
		sb.WriteString("## Installed Agents Context\n")
		sb.WriteString("This skill will be installed for the following agents:\n")
		for _, a := range installedAgents {
			sb.WriteString(fmt.Sprintf("- %s\n", string(a)))
		}
		sb.WriteString("\n")
	}

	// SDD integration context (conditional).
	if sddConfig != nil && sddConfig.Mode != SDDStandalone {
		sb.WriteString("## SDD Integration Context\n")
		switch sddConfig.Mode {
		case SDDPhaseSupport:
			sb.WriteString(fmt.Sprintf(
				"This skill provides support for the existing SDD phase: %s\n"+
					"It must reference and complement the existing phase without replacing it.\n"+
					"Include a section explaining how it interacts with `sdd-%s` triggers.\n",
				sddConfig.TargetPhase, sddConfig.TargetPhase,
			))
		case SDDNewPhase:
			sb.WriteString(fmt.Sprintf(
				"This skill introduces a NEW SDD phase named: %s\n"+
					"It must integrate with the SDD dependency graph as a first-class phase.\n"+
					"Include a Trigger that follows the pattern: When the orchestrator launches you for the %s phase.\n"+
					"The phase name to use in triggers: %s\n",
				sddConfig.PhaseName, sddConfig.PhaseName, sddConfig.PhaseName,
			))
		}
		sb.WriteString("\n")
	}

	// User's intent.
	sb.WriteString("## User Request\n")
	sb.WriteString(userInput)
	sb.WriteString("\n")

	return sb.String()
}
