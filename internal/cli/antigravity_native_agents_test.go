package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/reviewassets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/planner"
)

// TestAntigravityInstallAndSyncWriteJudgmentDayAgents proves install and sync
// publish the Judgment Day agents where agy 1.4 discovers custom agents, and
// that both flows snapshot those files before writing them.
func TestAntigravityInstallAndSyncWriteJudgmentDayAgents(t *testing.T) {
	names := []string{
		"gentle-ai-explore.md",
		"gentle-ai-verify.md",
		"gentle-ai-worker.md",
		"jd-fix-agent.md",
		"jd-judge-a.md",
		"jd-judge-b.md",
	}
	for _, flow := range []string{"install", "sync"} {
		t.Run(flow, func(t *testing.T) {
			home := t.TempDir()
			dir := filepath.Join(home, ".gemini", "config", "agents")
			selection := model.Selection{
				Agents: []model.AgentID{model.AgentAntigravity},
				AntigravityModelAssignments: map[string]model.AntigravityModelTier{
					"jd-judge-a":       model.AntigravityModelPro,
					"gentle-ai-worker": model.AntigravityModelFlash,
				},
			}

			var targets []string
			var err error
			if flow == "install" {
				resolved := planner.ResolvedPlan{Agents: selection.Agents}
				targets, err = installBackupTargets(home, "", ScopeGlobal, selection, resolved, false)
			} else {
				targets, err = syncBackupTargetsScoped(home, "", ScopeGlobal, selection, resolveAdapters(selection.Agents))
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range append([]string{reviewassets.OwnershipLedgerFilename}, names...) {
				if !slices.Contains(targets, filepath.Join(dir, name)) {
					t.Errorf("%s backup targets miss %s", flow, name)
				}
			}

			var changed []string
			if flow == "install" {
				runInstallInjectionSteps(t, newTestInstallRuntime(t, home, selection))
			} else {
				changed = runSyncInjectionSteps(t, home, selection)
			}
			for _, name := range names {
				path := filepath.Join(dir, name)
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("%s did not write %s: %v", flow, name, err)
				}
				if !strings.HasPrefix(string(data), "---\nname: "+strings.TrimSuffix(name, ".md")+"\n") {
					t.Errorf("%s has no agy frontmatter:\n%s", name, data)
				}
				wantModel := "\nmodel: inherit\n"
				if name == "jd-judge-a.md" {
					wantModel = "\nmodel: pro\n"
				} else if name == "gentle-ai-worker.md" {
					wantModel = "\nmodel: flash\n"
				}
				if !strings.Contains(string(data), wantModel) {
					t.Errorf("%s %s does not render %q", flow, name, strings.TrimSpace(wantModel))
				}
				if flow == "sync" && !slices.Contains(changed, path) {
					t.Errorf("sync did not report %s as changed", path)
				}
			}
			if review := rddAgentFileNames(t, dir); len(review) > 0 {
				t.Errorf("%s installed RDD agents on Antigravity: %v", flow, review)
			}
			if _, err := os.Stat(filepath.Join(home, ".gemini", "antigravity-cli", "agents")); !os.IsNotExist(err) {
				t.Errorf("%s wrote agents under the CLI-private directory: %v", flow, err)
			}
		})
	}
}
