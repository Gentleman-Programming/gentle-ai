package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v5/internal/components/reviewassets"
	"github.com/gentleman-programming/gentle-ai/v5/internal/model"
	"github.com/gentleman-programming/gentle-ai/v5/internal/planner"
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

// TestAntigravityLegacyThreeRolePresetUpgradeSync verifies that syncing an
// older three-role Economy or Performance assignment map preserves those custom
// jd-* choices and writes model: inherit to gentle-ai-worker.md, and that
// syncing the reselected preset updates gentle-ai-worker.md to flash (Economy)
// or pro (Performance).
func TestAntigravityLegacyThreeRolePresetUpgradeSync(t *testing.T) {
	for _, tc := range []struct {
		name           string
		legacyState    map[string]string
		reselected     map[string]model.AntigravityModelTier
		wantWorkerTier string
	}{
		{
			name: "economy",
			legacyState: map[string]string{
				"jd-judge-a":   "flash",
				"jd-judge-b":   "flash",
				"jd-fix-agent": "flash_lite",
			},
			reselected:     model.AntigravityModelPresetEconomy(),
			wantWorkerTier: "flash",
		},
		{
			name: "performance",
			legacyState: map[string]string{
				"jd-judge-a":   "pro",
				"jd-judge-b":   "pro",
				"jd-fix-agent": "pro",
			},
			reselected:     model.AntigravityModelPresetPerformance(),
			wantWorkerTier: "pro",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			agentsDir := filepath.Join(home, ".gemini", "config", "agents")
			restored := model.AntigravityModelTiersFromStrings(tc.legacyState)

			// Syncing the legacy three-role map preserves custom jd-* tiers and
			// writes model: inherit for the new gentle-ai-worker.md agent.
			runSyncInjectionSteps(t, home, model.Selection{
				Agents:                      []model.AgentID{model.AgentAntigravity},
				AntigravityModelAssignments: restored,
			})

			workerBefore, err := os.ReadFile(filepath.Join(agentsDir, "gentle-ai-worker.md"))
			if err != nil {
				t.Fatalf("read worker after legacy sync: %v", err)
			}
			if !strings.Contains(string(workerBefore), "\nmodel: inherit\n") {
				t.Fatalf("legacy %s sync wrote unexpected worker model:\n%s", tc.name, workerBefore)
			}
			fixBefore, err := os.ReadFile(filepath.Join(agentsDir, "jd-fix-agent.md"))
			if err != nil {
				t.Fatalf("read jd-fix-agent after legacy sync: %v", err)
			}
			if wantFix := "\nmodel: " + tc.legacyState["jd-fix-agent"] + "\n"; !strings.Contains(string(fixBefore), wantFix) {
				t.Fatalf("legacy %s sync did not preserve jd-fix-agent %q:\n%s", tc.name, strings.TrimSpace(wantFix), fixBefore)
			}

			// Syncing after preset reselection writes the preset worker tier.
			runSyncInjectionSteps(t, home, model.Selection{
				Agents:                      []model.AgentID{model.AgentAntigravity},
				AntigravityModelAssignments: tc.reselected,
			})

			workerAfter, err := os.ReadFile(filepath.Join(agentsDir, "gentle-ai-worker.md"))
			if err != nil {
				t.Fatalf("read worker after preset reselection: %v", err)
			}
			if want := "\nmodel: " + tc.wantWorkerTier + "\n"; !strings.Contains(string(workerAfter), want) {
				t.Fatalf("reselected %s sync worker does not render %q:\n%s", tc.name, strings.TrimSpace(want), workerAfter)
			}
		})
	}
}
