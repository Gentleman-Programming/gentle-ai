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

func TestCursorJudgmentDayInstallAndRefresh(t *testing.T) {
	adapter, err := agents.NewAdapter(model.AgentCursor)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	dir := adapter.SubAgentsDir(home)
	names := []string{"jd-judge-a.md", "jd-judge-b.md", "jd-fix-agent.md"}
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
			for _, name := range names {
				data, err := os.ReadFile(filepath.Join(dir, name))
				if err != nil {
					t.Fatal(err)
				}
				text := string(data)
				for _, want := range []string{"name: " + strings.TrimSuffix(name, ".md"), "model: inherit", "background: false"} {
					if !strings.Contains(text, want) {
						t.Errorf("%s missing %q", name, want)
					}
				}
				if strings.Contains(text, "{{") {
					t.Errorf("%s contains unresolved placeholders", name)
				}
				if strings.HasPrefix(name, "jd-judge-") {
					for _, want := range []string{"readonly: true", "frozen ledger", "fix delta", "2 fix rounds", "no `review-refuter`"} {
						if !strings.Contains(text, want) {
							t.Errorf("%s missing judge contract %q", name, want)
						}
					}
				} else {
					for _, want := range []string{"readonly: false", "Fix ONLY the confirmed issues", "Only the parent may launch"} {
						if !strings.Contains(text, want) {
							t.Errorf("%s missing fix contract %q", name, want)
						}
					}
				}
				if flow != "fresh" && !strings.Contains(text, opts.CodeGraphGuidanceMarkdown) {
					t.Errorf("%s was not refreshed", name)
				}
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != len(names)+1 {
				t.Fatalf("got %d entries, want three JD agents plus ownership ledger", len(entries))
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), "review-") {
					t.Errorf("installed RDD agent %s on Cursor", entry.Name())
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
