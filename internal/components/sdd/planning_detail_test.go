package sdd

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

// These assert shipped instructions and installed bytes, not model behavior.
func TestPlanningDetailPreservesRequiredContentWithoutArtifactCaps(t *testing.T) {
	phases := []struct {
		name string
		keep []string
	}{
		{"sdd-propose", []string{"## Capabilities", "## Rollback Plan", "## Success Criteria", "READ it first and UPDATE it"}},
		{"sdd-design", []string{"**Alternatives considered**", "**Rationale**", "## Testing Strategy", "Applicable threat-matrix rows"}},
		{"sdd-spec", []string{"Given/When/Then", "Every requirement MUST have at least ONE scenario", "MODIFIED requirements MUST be the FULL block", "RENAMED requirements"}},
		{"sdd-tasks", []string{"Use hierarchical numbering", "regeneration MUST preserve the existing task list and numbering", "If the project uses TDD", "Use checklist format", "## Review Workload Forecast", "400 changed-line review budget", "Focused test command", "Rollback boundary", "- [ ] 1.1"}},
	}
	capPattern := regexp.MustCompile(`(?i)(under\s+\d+\s+words|each (scenario|task):[^\n]*lines max)`)
	assertDetail := func(t *testing.T, content string, keep []string) {
		t.Helper()
		if cap := capPattern.FindString(content); cap != "" {
			t.Errorf("arbitrary planning artifact cap remains: %s", cap)
		}
		for _, want := range append([]string{"Do not truncate required detail to meet a word or line cap.", "Section C", "Return envelope per"}, keep...) {
			if !strings.Contains(content, want) {
				t.Errorf("missing sufficient-detail or retained requirement %q", want)
			}
		}
	}
	for _, phase := range phases {
		t.Run("embedded/"+phase.name, func(t *testing.T) {
			assertDetail(t, assets.MustRead("skills/"+phase.name+"/SKILL.md"), phase.keep)
		})
	}
	for _, mode := range []model.SDDModeID{model.SDDModeSingle, model.SDDModeMulti} {
		t.Run("installed/"+string(mode), func(t *testing.T) {
			home := t.TempDir()
			mockNoPackageManager(t)
			if _, err := Inject(home, opencodeAdapter(), mode); err != nil {
				t.Fatal(err)
			}
			for _, phase := range phases {
				t.Run(phase.name, func(t *testing.T) {
					path := filepath.Join(opencodeAdapter().SkillsDir(home), phase.name, "SKILL.md")
					if mode == model.SDDModeMulti {
						path = filepath.Join(SharedPromptDir(home), phase.name+".md")
					}
					content, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					assertDetail(t, string(content), phase.keep)
				})
			}
		})
	}
}
