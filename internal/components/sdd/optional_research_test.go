package sdd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v3/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

// These exercise rendered and installed instructions, not live model decisions.
func assertOptionalResearchGuidance(t *testing.T, content string) {
	t.Helper()
	for _, want := range []string{
		"### Optional Research and Product Discovery",
		"Research remains optional, including after selection.",
		"Missing, partial, unavailable or divergent research metadata does not block proposal work.",
		"Ask one focused product question at a time and wait for the answer",
		"Pause only work dependent on an unresolved product decision or unsafe missing evidence",
		"actually available and authorized",
		"primary sources", "contradictions", "freshness",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("missing optional research instruction %q", want)
		}
	}
	for _, stale := range []string{"Research and Pre-Proposal Gate (MANDATORY)", "proposal_ready", "gentle-ai.sdd-preproposal/v1", "selection makes completion mandatory", "selected research must be `done`", "matching hybrid state"} {
		if strings.Contains(content, stale) {
			t.Errorf("retired research admission remains: %q", stale)
		}
	}
}

func TestOptionalResearchRendersWithoutAdministrativeAdmission(t *testing.T) {
	for _, agent := range catalog.AllAgents() {
		if agent.ID == model.AgentPi {
			continue
		}
		t.Run(string(agent.ID), func(t *testing.T) {
			prompt := renderSDDOrchestratorAsset(agent.ID)
			if agent.ID == model.AgentClaudeCode {
				prompt += "\n" + renderBoundedReviewAsset(agent.ID, "claude/sdd-orchestrator-workflow.md")
			}
			assertOptionalResearchGuidance(t, prompt)
			assertResearchGatekeeperPrecedence(t, prompt)
		})
	}
}

func TestOptionalResearchMigratesInstalledOpenCodePrompt(t *testing.T) {
	const oldGate = "### Research and Pre-Proposal Gate (MANDATORY) — Offer `sdd-research` immediately after `sdd-explore`; selection makes completion mandatory."
	const userText = "Keep my proposal question round notes and custom release instructions."
	for _, mode := range []model.SDDModeID{model.SDDModeSingle, model.SDDModeMulti} {
		for _, variant := range []string{"inline", "managed", "optional-managed"} {
			name := string(mode) + "/" + variant
			t.Run(name, func(t *testing.T) {
				home := t.TempDir()
				mockNoPackageManager(t)
				settings := filepath.Join(home, ".config", "opencode", "opencode.json")
				gate := oldGate
				if variant == "optional-managed" {
					gate = strings.Split(researchLifecycleContract(), "#### Research-specific gatekeeper precedence")[0]
				}
				if variant != "inline" {
					gate = "<!-- gentle-ai:sdd-research-lifecycle -->\n" + gate + "\n<!-- /gentle-ai:sdd-research-lifecycle -->"
				}
				seed := map[string]any{"agent": map[string]any{"gentle-orchestrator": map[string]any{"mode": "primary", "prompt": "# Custom prompt\n" + userText + "\n" + strings.Replace(renderSDDOrchestratorAsset(model.AgentOpenCode), researchLifecycleContract(), gate, 1)}}}
				data, err := json.Marshal(seed)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(filepath.Dir(settings), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(settings, data, 0o644); err != nil {
					t.Fatal(err)
				}
				for i := 0; i < 2; i++ {
					if _, err := Inject(home, opencodeAdapter(), mode, InjectOptions{PreserveOpenCodeOrchestratorPrompt: true}); err != nil {
						t.Fatal(err)
					}
					prompt := readGentleOrchestratorPrompt(t, settings)
					assertOptionalResearchGuidance(t, prompt)
					assertResearchGatekeeperPrecedence(t, prompt)
					if strings.Count(prompt, userText) != 1 {
						t.Fatal("migration changed unrelated user text")
					}
					if strings.Count(prompt, "### Optional Research and Product Discovery") != 1 {
						t.Fatal("migration duplicated managed research instructions")
					}
				}
			})
		}
	}
}

// Check precedence together with the full ordinary gate, including Claude's lazy
// workflow. This is a prompt contract regression, not proof of model execution.
func assertResearchGatekeeperPrecedence(t *testing.T, prompt string) {
	t.Helper()
	for _, clause := range []string{
		"only (including named-profile variants)",
		"takes precedence over the generic Automatic Mode Gatekeeper",
		"Validate honest findings, source attribution and disclosed limitations",
		"do not require a persisted artifact or full-success status",
		"Do not automatically retry or STOP solely because research is partial, inline or tools are unavailable",
		"Never manufacture success or evidence",
		"Preserve real tool permissions, unresolved human product decisions and unsafe-dependent-work blocks",
		"Terminal transport failures retain their existing stop/continuation rules",
		"All other phases retain their existing gatekeeper checks and failure handling",
		"### Automatic Mode Gatekeeper (MANDATORY)",
		"**Contract conformance:**", "**Artifact existence:**",
		"**No hallucination:**", "**No drift from inputs:**", "**Routing coherence:**",
		"re-run the same phase exactly once", "STOP the automatic chain",
	} {
		if !strings.Contains(prompt, clause) {
			t.Errorf("full parent prompt missing scoped precedence or retained gate %q", clause)
		}
	}
	// Both supported wordings must retain the ordinary full-success requirement.
	if !strings.Contains(prompt, "status` indicates success (not partial, failed, or blocked)") &&
		!strings.Contains(prompt, "status is not partial/failed/blocked") {
		t.Error("ordinary phase success check was weakened")
	}
}

func TestOptionalResearchGatekeeperInstalledOpenCode(t *testing.T) {
	for _, mode := range []model.SDDModeID{model.SDDModeSingle, model.SDDModeMulti} {
		t.Run(string(mode), func(t *testing.T) {
			home := t.TempDir()
			mockNoPackageManager(t)
			if _, err := Inject(home, opencodeAdapter(), mode, InjectOptions{Profiles: []model.Profile{{Name: "focused"}}}); err != nil {
				t.Fatal(err)
			}
			agents := readOpenCodeAgents(t, opencodeAdapter().SettingsPath(home))
			for _, name := range []string{"gentle-orchestrator", "sdd-orchestrator-focused"} {
				t.Run(name, func(t *testing.T) {
					assertResearchGatekeeperPrecedence(t, agentPrompt(t, agents, name))
				})
			}
		})
	}
}

func TestOptionalResearchClaudeLazyWorkflowRetainsOrdinaryGate(t *testing.T) {
	parent := renderSDDOrchestratorAsset(model.AgentClaudeCode)
	if !strings.Contains(parent, "sdd-orchestrator-workflow.md") {
		t.Fatal("Claude parent no longer loads the tested lazy workflow")
	}
	workflow := renderBoundedReviewAsset(model.AgentClaudeCode, "claude/sdd-orchestrator-workflow.md")
	assertResearchGatekeeperPrecedence(t, workflow)
	if !strings.Contains(assets.MustRead("claude/sdd-orchestrator-workflow.md"), researchLifecyclePlaceholder) {
		t.Fatal("lazy workflow must receive the shared research contract")
	}
}
