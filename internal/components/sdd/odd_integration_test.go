package sdd

import (
	"os"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/components/agentguidance"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

// Installation-contract coverage, not a live executor or Engram session.
func TestInstalledODDAndSimplifiedSDDCoexist(t *testing.T) {
	for _, agent := range []model.AgentID{model.AgentOpenCode, model.AgentClaudeCode, model.AgentCodex} {
		for _, mode := range []model.SDDModeID{model.SDDModeSingle, model.SDDModeMulti} {
			t.Run(string(agent)+"/"+string(mode), func(t *testing.T) {
				home := t.TempDir()
				mockNoPackageManager(t)
				installed, err := Inject(home, mustAdapter(t, agent), mode)
				if err != nil {
					t.Fatal(err)
				}
				routing, err := agentguidance.InjectRouting(home, agent)
				if err != nil {
					t.Fatal(err)
				}
				var content strings.Builder
				seen := map[string]bool{}
				for _, path := range append(installed.Files, routing.Files...) {
					if seen[path] {
						continue
					}
					seen[path] = true
					data, err := os.ReadFile(path)
					if err != nil {
						t.Fatalf("read installed artifact: %v", err)
					}
					content.Write(data)
					content.WriteByte('\n')
				}
				if agent == model.AgentOpenCode {
					content.WriteString(readGentleOrchestratorPrompt(t, opencodeAdapter().SettingsPath(home)))
				}
				for _, want := range []string{
					"Keep one feature document, not a separate plan file or topic",
					"odd/tasks/<feature-name>.md", "odd/<feature-name>/tasks",
					"Tests or frameworks being present does not enable TDD",
					"Forward mode, source, and runner on every implementation delegation",
					"not an RDD cycle per TODO checkbox",
					"Missing, stale, malformed, or failed reports do not gate archive",
					"SDD never offers, launches, or consumes RDD",
					"Research remains optional, including after selection",
					"optional recovery hints", "READ-MERGE-WRITE",
				} {
					if !strings.Contains(content.String(), want) {
						t.Errorf("installed contracts missing %q", want)
					}
				}
				for _, retired := range []string{"sdd-attempt acquire", "sdd-verify-validate", "gentle-ai.sdd-preproposal/v1", "odd/plans/", "both writes MUST succeed"} {
					if strings.Contains(content.String(), retired) {
						t.Errorf("retired competing contract survives: %q", retired)
					}
				}
				repeated, err := agentguidance.InjectRouting(home, agent)
				if err != nil {
					t.Fatal(err)
				}
				if repeated.Changed {
					t.Fatal("unchanged ODD routing reinjection must be a no-op")
				}
			})
		}
	}
}
