package model

import "testing"

func TestAgentAntigravity(t *testing.T) {
	if AgentAntigravity != "antigravity" {
		t.Errorf("AgentAntigravity = %q, want %q", AgentAntigravity, "antigravity")
	}
}

func TestConstantsUniqueness(t *testing.T) {
	t.Run("AgentIDs", func(t *testing.T) {
		agents := []AgentID{
			AgentClaudeCode,
			AgentOpenCode,
			AgentKilocode,
			AgentGeminiCLI,
			AgentCursor,
			AgentVSCodeCopilot,
			AgentCodex,
			AgentAntigravity,
			AgentWindsurf,
			AgentKimi,
			AgentQwenCode,
			AgentKiroIDE,
			AgentOpenClaw,
			AgentPi,
			AgentTrae,
		}

		seen := make(map[AgentID]bool)
		for _, a := range agents {
			if a == "" {
				t.Error("Found empty AgentID")
			}
			if seen[a] {
				t.Errorf("Duplicate AgentID found: %q", a)
			}
			seen[a] = true
		}
	})

	t.Run("ComponentIDs", func(t *testing.T) {
		components := []ComponentID{
			ComponentEngram,
			ComponentSDD,
			ComponentSkills,
			ComponentContext7,
			ComponentPersona,
			ComponentPermission,
			ComponentGGA,
			ComponentTheme,
			ComponentClaudeTheme,
			ComponentOpenCodeGentleLogo,
		}

		seen := make(map[ComponentID]bool)
		for _, c := range components {
			if c == "" {
				t.Error("Found empty ComponentID")
			}
			if seen[c] {
				t.Errorf("Duplicate ComponentID found: %q", c)
			}
			seen[c] = true
		}
	})

	t.Run("SkillIDs", func(t *testing.T) {
		skills := []SkillID{
			SkillSDDInit,
			SkillSDDApply,
			SkillSDDVerify,
			SkillSDDExplore,
			SkillSDDPropose,
			SkillSDDSpec,
			SkillSDDDesign,
			SkillSDDTasks,
			SkillSDDArchive,
			SkillSDDOnboard,
			SkillGoTesting,
			SkillCreator,
			SkillImprover,
			SkillBranchPR,
			SkillIssueCreation,
			SkillSkillRegistry,
			SkillChainedPR,
			SkillCognitiveDoc,
			SkillCommentWriter,
			SkillWorkUnitCommits,
		}

		seen := make(map[SkillID]bool)
		for _, s := range skills {
			if s == "" {
				t.Error("Found empty SkillID")
			}
			if seen[s] {
				t.Errorf("Duplicate SkillID found: %q", s)
			}
			seen[s] = true
		}
	})

	t.Run("PersonaIDs", func(t *testing.T) {
		personas := []PersonaID{
			PersonaGentleman,
			PersonaGentlemanNeutralArtifacts,
			PersonaNeutral,
			PersonaCustom,
		}

		seen := make(map[PersonaID]bool)
		for _, p := range personas {
			if p == "" {
				t.Error("Found empty PersonaID")
			}
			if seen[p] {
				t.Errorf("Duplicate PersonaID found: %q", p)
			}
			seen[p] = true
		}
	})
}
