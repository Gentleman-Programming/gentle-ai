package reviewassets_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/reviewassets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

func claudeAgentsDir(t *testing.T) (string, agents.Adapter, string) {
	t.Helper()
	adapter, err := agents.NewAdapter(model.AgentClaudeCode)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	return home, adapter, adapter.SubAgentsDir(home)
}

func claudeFrontmatter(t *testing.T, path string) (map[string]string, string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.HasPrefix(content, "---\n") {
		t.Fatalf("%s has no frontmatter", path)
	}
	end := strings.Index(content[4:], "\n---\n")
	if end < 0 {
		t.Fatalf("%s has unterminated frontmatter", path)
	}
	fields := map[string]string{}
	for _, line := range strings.Split(content[4:4+end], "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok || strings.HasPrefix(line, " ") {
			continue
		}
		fields[key] = strings.TrimSpace(value)
	}
	return fields, content[4+end+5:]
}

// #5610: the ODD Explorer, Worker and Verify picks reach Claude Code through
// managed agents whose frontmatter carries the picked model and effort.
func TestClaudeODDAgentsCarryPickedModelAndEffort(t *testing.T) {
	home, adapter, dir := claudeAgentsDir(t)
	opts := reviewassets.InstallOptions{ClaudePhaseAssignments: map[string]model.ClaudePhaseAssignment{
		"odd-explorer": {Model: model.ClaudeModelSonnet, Effort: model.ClaudeEffortMedium},
		"odd-worker":   {Model: model.ClaudeModelHaiku},
		"odd-verify":   {Model: model.ClaudeModelOpus, Effort: model.ClaudeEffortHigh},
	}}
	if _, err := reviewassets.InstallNativeAgents(home, adapter, opts); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		file, name, model, effort string
		bodyMarker                string
		readOnly, canRunCommands  bool
	}{
		{"gentle-ai-explore.md", "gentle-ai-explore", "sonnet", "medium", "You are the read-only explorer for generic ODD work.", true, false},
		{"gentle-ai-worker.md", "gentle-ai-worker", "haiku", "", "", false, true},
		{"gentle-ai-verify.md", "gentle-ai-verify", "opus", "high", "You are the read-only technical verifier for generic ODD work.", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fields, body := claudeFrontmatter(t, filepath.Join(dir, tc.file))
			if fields["name"] != tc.name || fields["model"] != tc.model {
				t.Fatalf("frontmatter = %v, want name %q model %q", fields, tc.name, tc.model)
			}
			if effort, ok := fields["effort"]; tc.effort == "" && ok || tc.effort != "" && effort != tc.effort {
				t.Fatalf("effort = %q (present %v), want %q", effort, ok, tc.effort)
			}
			if strings.Contains(body, "{{") {
				t.Fatalf("unrendered placeholder in %s", tc.file)
			}
			if tc.bodyMarker != "" && !strings.Contains(body, tc.bodyMarker) {
				t.Fatalf("%s body does not restate the ODD role prompt", tc.file)
			}
			denied := fields["disallowedTools"]
			if !strings.Contains(denied, "Agent") {
				t.Fatalf("%s may delegate: disallowedTools = %q", tc.file, denied)
			}
			if tc.readOnly && !(strings.Contains(denied, "Edit") && strings.Contains(denied, "Write")) {
				t.Fatalf("%s is not read-only: disallowedTools = %q", tc.file, denied)
			}
			if !tc.canRunCommands && !strings.Contains(denied, "Bash") {
				t.Fatalf("%s may run commands: disallowedTools = %q", tc.file, denied)
			}
		})
	}
}

// #5610: the managed Claude orchestrator routes ODD delegation to those agents
// instead of the built-in general-purpose agent.
func TestClaudeOrchestratorRoutesODDDelegationToManagedAgents(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "assets", "claude", "orchestrator.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{"`subagent_type: gentle-ai-explore`", "`subagent_type: gentle-ai-worker`", "`subagent_type: gentle-ai-verify`"} {
		if !strings.Contains(content, want) {
			t.Errorf("orchestrator does not route to %s", want)
		}
	}
}

// legacySeededClaudePreset returns what the picker persisted for a preset
// before the ODD rows inherited: the preset without the retired sdd-* rows,
// with every ODD row seeded to Sonnet.
func legacySeededClaudePreset(preset map[string]model.ClaudeModelAlias) map[string]model.ClaudePhaseAssignment {
	assignments := model.ClaudePhaseAssignmentsFromModelPreset(preset)
	for key := range assignments {
		if strings.HasPrefix(key, "sdd-") {
			delete(assignments, key)
		}
	}
	for _, role := range []string{"odd-explorer", "odd-worker", "odd-verify"} {
		assignments[role] = model.ClaudePhaseAssignment{Model: model.ClaudeModelSonnet}
	}
	return assignments
}

func claudeLegacyAliases(assignments map[string]model.ClaudePhaseAssignment) map[string]model.ClaudeModelAlias {
	aliases := make(map[string]model.ClaudeModelAlias, len(assignments))
	for key, value := range assignments {
		aliases[key] = value.Model
	}
	return aliases
}

// #5612 review: an ODD row without an explicit pick inherits the session model
// instead of the general delegation row's Sonnet, and the Sonnet that older
// pickers seeded into every preset is not an explicit pick.
func TestClaudeODDAgentsInheritSessionModelWithoutPick(t *testing.T) {
	seededBalanced := legacySeededClaudePreset(model.ClaudeModelPresetBalanced())
	withRetiredRow := legacySeededClaudePreset(model.ClaudeModelPresetPerformance())
	withRetiredRow["sdd-apply"] = model.ClaudePhaseAssignment{Model: model.ClaudeModelOpus}
	// Sync drops the orchestrator row before it installs agents.
	withoutOrchestrator := legacySeededClaudePreset(model.ClaudeModelPresetDiversity())
	delete(withoutOrchestrator, "orchestrator")
	for name, opts := range map[string]reviewassets.InstallOptions{
		"nothing persisted": {},
		"general delegation row only": {ClaudePhaseAssignments: map[string]model.ClaudePhaseAssignment{
			"default": {Model: model.ClaudeModelOpus, Effort: model.ClaudeEffortHigh},
		}},
		"legacy seeded preset":                    {ClaudePhaseAssignments: seededBalanced, ClaudeModelAssignments: claudeLegacyAliases(seededBalanced)},
		"legacy seeded preset with a retired row": {ClaudePhaseAssignments: withRetiredRow},
		"legacy seeded preset as sync passes it":  {ClaudePhaseAssignments: withoutOrchestrator, ClaudeModelAssignments: claudeLegacyAliases(withoutOrchestrator)},
		"legacy seeded preset in the model-only map": {
			ClaudeModelAssignments: claudeLegacyAliases(legacySeededClaudePreset(model.ClaudeModelPresetEconomy())),
		},
	} {
		t.Run(name, func(t *testing.T) {
			home, adapter, dir := claudeAgentsDir(t)
			if _, err := reviewassets.InstallNativeAgents(home, adapter, opts); err != nil {
				t.Fatal(err)
			}
			for _, file := range []string{"gentle-ai-explore.md", "gentle-ai-worker.md", "gentle-ai-verify.md"} {
				fields, _ := claudeFrontmatter(t, filepath.Join(dir, file))
				if fields["model"] != "inherit" {
					t.Errorf("%s model = %q, want inherit", file, fields["model"])
				}
				if effort, ok := fields["effort"]; ok {
					t.Errorf("%s writes effort %q without a pick", file, effort)
				}
			}
		})
	}
}

// #5612 review: a Custom map that differs from every preset keeps its ODD
// picks, even when one of them equals the old Sonnet seed.
func TestClaudeODDAgentsKeepCustomPicksThatMatchTheOldSeed(t *testing.T) {
	custom := legacySeededClaudePreset(model.ClaudeModelPresetBalanced())
	custom["jd-judge-a"] = model.ClaudePhaseAssignment{Model: model.ClaudeModelOpus}
	home, adapter, dir := claudeAgentsDir(t)
	if _, err := reviewassets.InstallNativeAgents(home, adapter, reviewassets.InstallOptions{ClaudePhaseAssignments: custom}); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"gentle-ai-explore.md", "gentle-ai-worker.md", "gentle-ai-verify.md"} {
		fields, _ := claudeFrontmatter(t, filepath.Join(dir, file))
		if fields["model"] != "sonnet" {
			t.Errorf("%s model = %q, want the custom sonnet pick", file, fields["model"])
		}
	}
}

// #5612 review: the inherit fallback is ODD-only; review and Judgment Day
// agents still fall back to the general delegation row.
func TestClaudeReviewAgentsStillFallBackToGeneralDelegation(t *testing.T) {
	home, adapter, dir := claudeAgentsDir(t)
	opts := reviewassets.InstallOptions{ClaudePhaseAssignments: map[string]model.ClaudePhaseAssignment{
		"default": {Model: model.ClaudeModelOpus},
	}}
	if _, err := reviewassets.InstallNativeAgents(home, adapter, opts); err != nil {
		t.Fatal(err)
	}
	fields, _ := claudeFrontmatter(t, filepath.Join(dir, "review-risk.md"))
	if fields["model"] != "opus" {
		t.Fatalf("review-risk model = %q, want the general delegation opus", fields["model"])
	}
}
