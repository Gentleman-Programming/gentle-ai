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

// agyFrontmatterKeys are the only frontmatter keys agy custom agents read.
var agyFrontmatterKeys = []string{"name", "description", "model"}

func TestAntigravityJudgmentDayInstallAndRefresh(t *testing.T) {
	adapter, err := agents.NewAdapter(model.AgentAntigravity)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	dir := filepath.Join(home, ".gemini", "config", "agents")
	allNames := reviewassets.NativeAgentManifest[model.AgentAntigravity]
	for _, flow := range []string{"fresh", "refresh", "idempotent"} {
		t.Run(flow, func(t *testing.T) {
			opts := reviewassets.InstallOptions{}
			if flow != "fresh" {
				opts.CodeGraphGuidanceMarkdown = "Use CodeGraph before broad filesystem search."
			}
			result, err := reviewassets.InstallNativeAgents(home, adapter, opts)
			if err != nil {
				t.Fatal(err)
			}
			if result.Changed != (flow != "idempotent") {
				t.Fatalf("%s: unexpected result %+v", flow, result)
			}
			for _, name := range allNames {
				data, err := os.ReadFile(filepath.Join(dir, name))
				if err != nil {
					t.Fatal(err)
				}
				text := string(data)
				fields := frontmatterFields(t, name, text)
				if len(fields) != len(agyFrontmatterKeys) {
					t.Errorf("%s frontmatter keys = %v, want exactly %v", name, fields, agyFrontmatterKeys)
				}
				for _, key := range agyFrontmatterKeys {
					if strings.TrimSpace(fields[key]) == "" {
						t.Errorf("%s frontmatter missing %q", name, key)
					}
				}
				if got, want := fields["name"], strings.TrimSuffix(name, ".md"); got != want {
					t.Errorf("%s name = %q, want %q", name, got, want)
				}
				if fields["model"] != "inherit" {
					t.Errorf("%s model = %q, want inherit", name, fields["model"])
				}
				if strings.Contains(text, "{{") {
					t.Errorf("%s contains unresolved placeholders", name)
				}
				if strings.HasPrefix(name, "jd-judge-") {
					for _, want := range []string{"You are read-only: do not edit code", "frozen ledger", "fix delta", "2 fix rounds", "no `review-refuter`"} {
						if !strings.Contains(text, want) {
							t.Errorf("%s missing judge contract %q", name, want)
						}
					}
				} else if name == "jd-fix-agent.md" {
					for _, want := range []string{"Fix ONLY the confirmed issues", "Only the parent may launch"} {
						if !strings.Contains(text, want) {
							t.Errorf("%s missing fix contract %q", name, want)
						}
					}
				} else if name == "gentle-ai-explore.md" {
					for _, want := range []string{"Read and search only. Do not edit, write, run commands, or mutate state.", "Do not fix findings, delegate to child agents, commit, or push."} {
						if !strings.Contains(text, want) {
							t.Errorf("%s missing explorer read-only contract %q", name, want)
						}
					}
				} else if name == "gentle-ai-verify.md" {
					for _, want := range []string{"Read-only means no edits to the repository or its git state", "Do not edit, write, or fix findings."} {
						if !strings.Contains(text, want) {
							t.Errorf("%s missing verifier read-only contract %q", name, want)
						}
					}
				}
				if flow != "fresh" && strings.HasPrefix(name, "jd-") && !strings.Contains(text, opts.CodeGraphGuidanceMarkdown) {
					t.Errorf("%s was not refreshed", name)
				}
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != len(allNames)+1 {
				t.Fatalf("got %d entries, want %d agents plus ownership ledger", len(entries), len(allNames))
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), "review-") {
					t.Errorf("installed RDD agent %s on Antigravity", entry.Name())
				}
			}
		})
	}
	userPath := filepath.Join(dir, "jd-judge-a.md")
	if err := os.WriteFile(userPath, []byte("user-customized judge\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := reviewassets.InstallNativeAgents(home, adapter, reviewassets.InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(userPath); err != nil || string(data) != "user-customized judge\n" {
		t.Fatalf("user judge overwritten: %q, %v", data, err)
	}
}

// frontmatterFields parses the top-level "key: value" lines of a Markdown
// agent's YAML frontmatter.
func frontmatterFields(t *testing.T, name, text string) map[string]string {
	t.Helper()
	if !strings.HasPrefix(text, "---\n") {
		t.Fatalf("%s does not start with frontmatter", name)
	}
	end := strings.Index(text[4:], "\n---\n")
	if end < 0 {
		t.Fatalf("%s frontmatter is not closed", name)
	}
	fields := map[string]string{}
	for _, line := range strings.Split(text[4:4+end], "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok || strings.HasPrefix(line, " ") {
			t.Fatalf("%s has a non key/value frontmatter line %q", name, line)
		}
		fields[key] = strings.TrimSpace(value)
	}
	return fields
}
