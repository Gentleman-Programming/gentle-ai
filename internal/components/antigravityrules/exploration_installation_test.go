package antigravityrules_test

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/antigravity"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/agentguidance"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/antigravityrules"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/communitytool"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/engram"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/persona"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// S1: installation paths preserve the complete policy in the always-on core.
func TestExplorationContractSurvivesPublicInstallation(t *testing.T) {
	engram.SetLookPathForTest(t, "/opt/homebrew/bin/engram", "")
	for _, tt := range []struct {
		name                   string
		p                      model.PersonaID
		memory, graph, desktop bool
	}{
		{"gentleman-codegraph", model.PersonaGentleman, true, true, false},
		{"neutral-desktop", model.PersonaNeutral, true, false, true},
		{"routing-only", model.PersonaCustom, false, false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			a := antigravity.NewAdapter()
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			read := func(path string) []byte { t.Helper(); b, err := os.ReadFile(path); must(err); return b }
			if tt.desktop {
				must(os.MkdirAll(filepath.Join(home, ".gemini/antigravity-desktop"), 0755))
			}
			if tt.p != model.PersonaCustom {
				_, err := persona.Inject(home, a, tt.p)
				must(err)
			}
			if tt.memory {
				_, err := engram.Inject(home, a)
				must(err)
			}
			_, err := agentguidance.InjectRoutingWithOptions(home, model.AgentAntigravity, agentguidance.RoutingOptions{})
			must(err)
			path := a.SystemPromptFile(home)
			if tt.graph {
				must(os.WriteFile(path, []byte(filemerge.InjectMarkdownSection(string(read(path)), "codegraph-guidance", communitytool.CodeGraphGuidanceMarkdown())), 0644))
			}
			stage := string(read(path))
			const mapping = "more than approximately 5 sequential lookups requires one read-only explorer; with its handoff, re-evaluate task size. Return at most approximately 2k tokens with path:line evidence and one parent spot check. Do not reread the entire mapped evidence."
			if n := strings.Count(stage, mapping); n != 1 {
				t.Errorf("complete rule copies=%d, want 1", n)
			}
			var core string
			inline := 0
			rules := antigravityrules.Render(stage)
			for _, rule := range rules {
				if len(rule.Content) > 24000 {
					t.Errorf("%s exceeds per-file cap", rule.Name)
				}
				if strings.HasPrefix(rule.Content, "---\ntrigger: always_on\n---\n\n") {
					inline += len(rule.Content)
					if strings.Contains(rule.Content, "### Work Routing Ladder") {
						core = rule.Content
					}
				}
			}
			if inline > 54000 || antigravityrules.AlwaysOnBudgetBytes != 54000 || antigravityrules.MaxRuleFileBytes != 24000 {
				t.Fatal("budget changed or exceeded")
			}
			for _, want := range []string{
				"Understanding that needs more evidence or " + mapping,
				"one parallel batch, at most 3 calls and approximately 10k tokens",
				"bounded search/line ranges rather than whole large files",
				"Each trigger holds only when its condition truly holds",
				"no trigger holds by default, and when in doubt work inline",
				"Do not pass these rules to child agents as permission to orchestrate",
				"counts, --stat, tail, or summaries",
				"delegate long suites and builds; return concise observed results, including failures",
				"approximately 150k parent-context backstop is advisory guidance, not mechanically observed or enforced",
				"pause and delegate the next bounded unit without claiming runtime telemetry or enforcement",
			} {
				if !strings.Contains(core, want) {
					t.Errorf("missing always-on core clause: %q", want)
				}
			}
			_, err = antigravityrules.Publish(home)
			must(err)
			for _, rule := range rules {
				if string(read(filepath.Join(antigravityrules.RulesDir(home), rule.Name))) != rule.Content {
					t.Errorf("published bytes differ: %s", rule.Name)
				}
			}
			again, err := antigravityrules.Publish(home)
			must(err)
			if again.Changed || len(again.Files) != 0 {
				t.Fatal("publication is not idempotent")
			}
			before := sha256.Sum256(read(path))
			_, err = agentguidance.InjectRoutingWithOptions(home, model.AgentAntigravity, agentguidance.RoutingOptions{})
			must(err)
			if sha256.Sum256(read(path)) != before {
				t.Fatal("routing reinjection changed stored guidance")
			}
		})
	}
}
