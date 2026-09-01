package app_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

// Provenance rule (#2524, root 4 of #2440): a runtime's rendered surface must
// never tell it to identify as another runtime. Before #2484, codex's rendered
// AGENTS.md carried `--agent claude-code` and nothing went red. This rule
// closes that class.
//
// Both sides are DERIVED, never enumerated (#2471, root 10):
//
//   - the surfaces are every *.golden under testdata/golden, the pinned
//     renders of each runtime's installed instruction surface;
//   - the owning runtime comes from matching the golden's filename tokens
//     against the slug each model.AgentID projects (its first hyphen
//     segment: "claude-code" -> "claude", "gemini-cli" -> "gemini"), with
//     the slug set built from catalog.AllAgents() at run time.
//
// Occurrences are read from the same extracted-invocation corpus PR #2522
// introduced, and a raw-bytes count cross-checks the extraction so an
// `--agent` binding outside any extractable invocation cannot hide.

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

	if verified == 0 {
		t.Fatal("verified no --agent binding in any rendered golden; either the surfaces stopped binding identity or the scanner went stale")
	}
}
