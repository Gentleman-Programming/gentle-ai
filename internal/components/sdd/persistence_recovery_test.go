package sdd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

// Contract checks inspect shipped and installed instructions, not live backend behavior.
func assertPersistenceRecovery(t *testing.T, content string) {
	t.Helper()
	for _, stale := range []string{"{{GENTLE_AI_SDD_SECTION:", "If unsure which mode to use, default", "Engram (primary)", "Engram first", "simultaneously", "both writes MUST succeed", "persists DAG state after each phase", "parse YAML → restore state", "If you return without calling mem_save", "update DAG state", "Update it after each phase completes", "- proposal.md ✅", "- specs/ ✅", "- design.md ✅"} {
		if strings.Contains(content, stale) {
			t.Errorf("retired persistence obligation: %q", stale)
		}
	}
}

func TestPersistenceRecoveryUsesResolvedArtifactsWithoutRequiredSnapshots(t *testing.T) {
	contracts := map[string][]string{
		"persistence-contract.md": {"artifactStore", "artifactPaths", "successful writes", "partial", "not atomic", "optional recovery hints", "READ-MERGE-WRITE", "mem_get_observation"},
		"engram-convention.md":    {"optional recovery hint", "mem_get_observation", "archive-report", "topic_key"},
		"openspec-convention.md":  {"optional recovery hint", "dependsOn", "READ it first and UPDATE it"},
		"sdd-phase-common.md":     {"artifactStore", "artifactPaths", "successful writes", "partial", "not atomic", "Never substitute another store's copy"},
	}
	for name, required := range contracts {
		t.Run(name, func(t *testing.T) {
			content := assets.MustRead("skills/_shared/" + name)
			assertPersistenceRecovery(t, content)
			for _, want := range required {
				if !strings.Contains(content, want) {
					t.Errorf("missing preservation clause %q", want)
				}
			}
		})
	}
	assertPersistenceRecovery(t, assets.MustRead("skills/sdd-archive/SKILL.md"))
	for _, agent := range catalog.AllAgents() {
		if agent.ID == model.AgentPi {
			continue
		}
		t.Run("render/"+string(agent.ID), func(t *testing.T) {
			prompt := renderSDDOrchestratorAsset(agent.ID)
			if agent.ID == model.AgentClaudeCode {
				lazy, err := renderClaudeSessionPreflight()
				if err != nil {
					t.Fatal(err)
				}
				prompt += lazy
			}
			assertPersistenceRecovery(t, prompt)
			if strings.Contains(prompt, "### Recovery") || strings.Contains(prompt, "## Recovery") {
				for _, want := range []string{"optional recovery hints", "artifactStore", "artifactPaths", "hybrid", "mem_get_observation"} {
					if !strings.Contains(prompt, want) {
						t.Errorf("recovery missing %q", want)
					}
				}
			}
		})
	}
	t.Run("installed/claude-lazy-recovery", func(t *testing.T) {
		home := t.TempDir()
		if _, err := writeClaudeLazySDDWorkflow(home, claudeAdapter()); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(claudeAdapter().SkillsDir(home), "_shared", "sdd-orchestrator-workflow.md"))
		if err != nil {
			t.Fatal(err)
		}
		assertPersistenceRecovery(t, string(b))
		if !strings.Contains(string(b), "optional recovery hints") {
			t.Fatal("installed lazy workflow lacks shared recovery")
		}
	})

	for _, mode := range []model.SDDModeID{model.SDDModeSingle, model.SDDModeMulti} {
		t.Run("installed/"+string(mode), func(t *testing.T) {
			home := t.TempDir()
			mockNoPackageManager(t)
			if _, err := Inject(home, opencodeAdapter(), mode); err != nil {
				t.Fatal(err)
			}
			for name, required := range contracts {
				b, err := os.ReadFile(filepath.Join(opencodeAdapter().SkillsDir(home), "_shared", name))
				if err != nil {
					t.Fatal(err)
				}
				assertPersistenceRecovery(t, string(b))
				for _, want := range required {
					if !strings.Contains(string(b), want) {
						t.Errorf("installed %s missing %q", name, want)
					}
				}
			}
		})
	}
}
