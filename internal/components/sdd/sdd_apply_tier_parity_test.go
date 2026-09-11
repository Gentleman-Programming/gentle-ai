package sdd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

// TestSDDApplyTierParityAtRenderedInstalledBoundaries verifies that both capable and
// small model capability tiers preserve the rules.apply phase-rule instruction in
// rendered shared prompts and installed skill outputs without leaking section markers.
func TestSDDApplyTierParityAtRenderedInstalledBoundaries(t *testing.T) {
	const phaseRuleInstruction = "Apply any `rules.apply` from `openspec/config.yaml`"

	tests := []struct {
		name       string
		capability string
	}{
		{
			name:       "capable",
			capability: "capable",
		},
		{
			name:       "small",
			capability: "small",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()

			// Boundary 1: Shared prompt file rendered by WriteSharedPromptFiles
			phaseCaps := map[string]string{"sdd-apply": tt.capability}
			if _, err := WriteSharedPromptFiles(home, phaseCaps); err != nil {
				t.Fatalf("WriteSharedPromptFiles(%s) error = %v", tt.capability, err)
			}
			promptPath := filepath.Join(SharedPromptDir(home), "sdd-apply.md")
			promptBytes, err := os.ReadFile(promptPath)
			if err != nil {
				t.Fatalf("ReadFile(%q) error = %v", promptPath, err)
			}

			// Boundary 2: Agent skill directory installed by InjectSkillDirectoryForAgent
			skillDir := filepath.Join(home, "installed-skills")
			if _, err := InjectSkillDirectoryForAgent(skillDir, tt.capability, model.AgentClaudeCode); err != nil {
				t.Fatalf("InjectSkillDirectoryForAgent(%s) error = %v", tt.capability, err)
			}
			skillPath := filepath.Join(skillDir, "sdd-apply", "SKILL.md")
			skillBytes, err := os.ReadFile(skillPath)
			if err != nil {
				t.Fatalf("ReadFile(%q) error = %v", skillPath, err)
			}

			boundaries := map[string]string{
				"shared_prompt":   string(promptBytes),
				"installed_skill": string(skillBytes),
			}

			for boundary, content := range boundaries {
				if !strings.Contains(content, phaseRuleInstruction) {
					t.Errorf("[%s] %s missing phase rule contract %q", tt.name, boundary, phaseRuleInstruction)
				}
				for _, leak := range []string{
					"<!-- section:model-",
					"<!-- /section:model-",
					"section:model-capable",
					"section:model-small",
				} {
					if strings.Contains(content, leak) {
						t.Errorf("[%s] %s leaked model-section marker %q", tt.name, boundary, leak)
					}
				}
			}
		})
	}
}
