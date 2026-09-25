package app_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/reviewassets"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
	"github.com/gentleman-programming/gentle-ai/v3/internal/reviewerprovider"
)

// Provenance rule (#2524, root 4 of #2440): a runtime's rendered surface must
// never tell it to identify as another runtime. Before #2484, codex's rendered
// AGENTS.md carried `--agent claude-code` and nothing went red. Check both
// the retained golden surfaces and the currently rendered review contract:
// the goldens no longer contain the review command, but reviewassets still
// binds runtime identity in the installed contract. Derive golden owners from
// catalog.AllAgents() and inspect the actual per-runtime render, not an
// invented golden or a removed SDD overlay.

var rawAgentBindingRegexp = regexp.MustCompile(`--agent[= ]+[A-Za-z0-9._{}-]+`)

// agentFlagValues returns every value bound to --agent in one documented
// command, including the `--agent=value` spelling. A trailing bare --agent
// is returned as an empty value so the caller can reject it.
func agentFlagValues(command string) []string {
	words := strings.Fields(command)
	var out []string
	for index, word := range words {
		if word == "--agent" {
			if index+1 < len(words) {
				out = append(out, words[index+1])
			} else {
				out = append(out, "")
			}
			continue
		}
		if value, ok := strings.CutPrefix(word, "--agent="); ok {
			out = append(out, value)
		}
	}
	return out
}

// goldenOwners derives which runtimes a golden filename names, by exact
// token match against the derived slug set.
func goldenOwners(name string, slugToAgent map[string]model.AgentID) []model.AgentID {
	tokens := strings.FieldsFunc(strings.TrimSuffix(name, ".golden"), func(r rune) bool {
		return r == '-' || r == '.'
	})
	seen := map[model.AgentID]bool{}
	var owners []model.AgentID
	for _, token := range tokens {
		if id, ok := slugToAgent[token]; ok && !seen[id] {
			seen[id] = true
			owners = append(owners, id)
		}
	}
	return owners
}

func TestRenderedSurfaceNamesOnlyItsOwnRuntime(t *testing.T) {
	slugToAgent := map[string]model.AgentID{}
	for _, agent := range catalog.AllAgents() {
		slug := strings.SplitN(string(agent.ID), "-", 2)[0]
		if other, duplicate := slugToAgent[slug]; duplicate {
			t.Fatalf("agents %s and %s project the same filename slug %q; the derivation is ambiguous and must be repaired before this rule can decide ownership", other, agent.ID, slug)
		}
		slugToAgent[slug] = agent.ID
	}

	goldenDir := filepath.Join("..", "..", "testdata", "golden")
	entries, err := os.ReadDir(goldenDir)
	if err != nil {
		t.Fatal(err)
	}

	verified := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".golden") {
			continue
		}
		content, readErr := os.ReadFile(filepath.Join(goldenDir, name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		owners := goldenOwners(name, slugToAgent)

		found := 0
		for _, invocation := range extractInvocations("golden:"+name, string(content)) {
			for _, value := range agentFlagValues(invocation.command) {
				found++
				if value == "" {
					t.Errorf("%s documents a bare --agent with no value\n  command: %s", invocation.source, invocation.command)
					continue
				}
				if len(owners) != 1 {
					t.Errorf("%s binds --agent %s but its filename derives %d owning runtimes instead of exactly one; provenance cannot be decided", invocation.source, value, len(owners))
					continue
				}
				expected := string(owners[0])
				if value != expected {
					t.Errorf("%s binds --agent %s inside %s's rendered surface; a runtime's surface must name its own identity\n  command: %s", invocation.source, value, expected, invocation.command)
					continue
				}
				verified++
			}
		}

		if raw := len(rawAgentBindingRegexp.FindAllString(string(content), -1)); raw != found {
			t.Errorf("golden:%s carries %d --agent bindings but only %d sit inside extractable gentle-ai invocations; the difference is invisible to this rule and must be moved into a documented invocation or removed", name, raw, found)
		}
	}

	// The review contract is now rendered separately from the historical
	// combined goldens. Require a real bound command for every non-Pi review
	// runtime, and reject cross-binding or unextractable bindings there too.
	for _, agent := range catalog.AllAgents() {
		id := agent.ID
		if !reviewerprovider.RegisteredRuntime(id) && id != model.AgentPi {
			continue
		}
		t.Run(string(id), func(t *testing.T) {
			content, err := reviewassets.ReviewExecutionContractFor(id)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(content, "{{GENTLE_AI_RUNTIME_AGENT_ID}}") {
				t.Fatal("rendered review contract has an unbound runtime identity")
			}
			found := 0
			for _, invocation := range extractInvocations("review-contract:"+string(id), content) {
				for _, value := range agentFlagValues(invocation.command) {
					found++
					if value != string(id) {
						t.Errorf("%s binds --agent %q instead of %q: %s", invocation.source, value, id, invocation.command)
					}
				}
			}
			if raw := len(rawAgentBindingRegexp.FindAllString(content, -1)); raw != found {
				t.Errorf("review contract carries %d --agent bindings but only %d are extractable invocations", raw, found)
			}
			if id == model.AgentPi {
				if found != 0 {
					t.Errorf("Pi uses facade operations, not --agent CLI bindings; found %d", found)
				}
			} else if found == 0 {
				t.Fatal("rendered review contract contains no bound --agent invocation")
			}
			verified += found
		})
	}
	if verified == 0 {
		t.Fatal("verified no runtime identity binding in current review contracts or rendered goldens")
	}
}
