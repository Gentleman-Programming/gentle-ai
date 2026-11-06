package antigravityrules_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/antigravityrules"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/state"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func publishedRules(t *testing.T, home string) []string {
	t.Helper()
	entries, err := os.ReadDir(antigravityrules.RulesDir(home))
	if err != nil {
		t.Fatalf("ReadDir(rules) error = %v", err)
	}
	var names []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "gentle-ai-") {
			names = append(names, entry.Name())
		}
	}
	return names
}

func TestRulesDirIsTheSharedAgyRulesDirectory(t *testing.T) {
	home := t.TempDir()
	if got, want := antigravityrules.RulesDir(home), filepath.Join(home, ".gemini", "config", "rules"); got != want {
		t.Fatalf("RulesDir() = %q, want %q", got, want)
	}
}

func TestPublishRemovesOnlyStaleGentleAIRules(t *testing.T) {
	home := t.TempDir()
	rules := antigravityrules.RulesDir(home)
	writeFile(t, antigravityrules.StagingPath(home), filemerge.InjectMarkdownSection("", "persona", "Be kind.\n"))
	stale := filepath.Join(rules, "gentle-ai-07-retired.md")
	writeFile(t, stale, frontmatter+"\nold\n")
	userRule := filepath.Join(rules, "my-team.md")
	writeFile(t, userRule, frontmatter+"\nmine\n")
	userNamed := filepath.Join(rules, "gentle-ai-notes.md")
	writeFile(t, userNamed, "my own notes\n")
	userEdited := filepath.Join(rules, "gentle-ai-08-custom.md")
	writeFile(t, userEdited, "no managed frontmatter\n")
	staleDemoted := filepath.Join(rules, "gentle-ai-06-retired-part2.md")
	writeFile(t, staleDemoted, "---\ntrigger: model_decision\ndescription: \"Gentle AI retired guidance\"\n---\n\nold\n")
	userDemoted := filepath.Join(rules, "gentle-ai-09-team.md")
	writeFile(t, userDemoted, "---\ntrigger: model_decision\n---\n\nmine\n")

	if _, err := antigravityrules.Publish(home); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	for _, removed := range []string{stale, staleDemoted} {
		if _, err := os.Stat(removed); !os.IsNotExist(err) {
			t.Fatalf("stale owned rule %s still present; stat err = %v", removed, err)
		}
	}
	for _, kept := range []string{userRule, userNamed, userEdited, userDemoted} {
		if _, err := os.Stat(kept); err != nil {
			t.Fatalf("user rule %s was removed: %v", kept, err)
		}
	}
	if got := readFile(t, filepath.Join(rules, "gentle-ai-01-persona.md")); got != frontmatter+"\nBe kind.\n" {
		t.Fatalf("persona rule = %q", got)
	}
	if again, err := antigravityrules.Publish(home); err != nil || again.Changed {
		t.Fatalf("second Publish() = %+v, %v; want unchanged", again, err)
	}

	// An emptied staging file retires every owned rule (uninstall path).
	writeFile(t, antigravityrules.StagingPath(home), "")
	result, err := antigravityrules.Publish(home)
	if err != nil {
		t.Fatalf("Publish(empty) error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Publish(empty) changed = false, want owned rules removed")
	}
	if names := publishedRules(t, home); strings.Join(names, ",") != "gentle-ai-08-custom.md,gentle-ai-09-team.md,gentle-ai-notes.md" {
		t.Fatalf("remaining gentle-ai rules = %v", names)
	}
}

func TestMigrateLegacyPromptPreservesUserContent(t *testing.T) {
	user := "# My Gemini rules\n\nAlways answer in haiku.\n"
	legacy := filemerge.InjectMarkdownSection(user, "persona", "persona body\n")
	legacy = filemerge.InjectMarkdownSection(legacy, "engram-protocol", "engram body\n")
	legacy = filemerge.InjectMarkdownSection(legacy, "agent-routing", "routing body\n")

	tests := []struct {
		name        string
		initial     string
		geminiInUse bool
		want        string
		wantRemoved bool
	}{
		{name: "antigravity only strips every managed section", initial: legacy, want: user},
		// Gemini CLI owns GEMINI.md, and baseline Antigravity left the marked
		// persona as the only persona copy there, so nothing is removed.
		{name: "managed gemini-cli owns the file and keeps every section", initial: legacy, geminiInUse: true, want: legacy},
		{name: "managed-only file is removed", initial: filemerge.InjectMarkdownSection("", "persona", "p\n"), wantRemoved: true},
		{name: "user-only file is untouched", initial: user, want: user},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			path := antigravityrules.LegacyPromptPath(home)
			writeFile(t, path, tt.initial)

			result, err := antigravityrules.MigrateLegacyPrompt(home, tt.geminiInUse)
			if err != nil {
				t.Fatalf("MigrateLegacyPrompt() error = %v", err)
			}
			if result.Changed != (tt.initial != tt.want) {
				t.Fatalf("changed = %v, want %v", result.Changed, tt.initial != tt.want)
			}
			if tt.wantRemoved {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("GEMINI.md still present; stat err = %v", err)
				}
				return
			}
			if got := readFile(t, path); got != tt.want {
				t.Fatalf("GEMINI.md = %q, want %q", got, tt.want)
			}
		})
	}

	t.Run("missing file is a no-op", func(t *testing.T) {
		result, err := antigravityrules.MigrateLegacyPrompt(t.TempDir(), false)
		if err != nil || result.Changed {
			t.Fatalf("MigrateLegacyPrompt(missing) = %+v, %v", result, err)
		}
	})
}

func TestGeminiCLIManaged(t *testing.T) {
	tests := []struct {
		name     string
		selected []model.AgentID
		state    string
		want     bool
	}{
		{name: "antigravity only", selected: []model.AgentID{model.AgentAntigravity}},
		{name: "gemini-cli selected", selected: []model.AgentID{model.AgentAntigravity, model.AgentGeminiCLI}, want: true},
		{name: "gemini-cli recorded in state", state: `{"installed_agents":["gemini-cli"]}`, want: true},
		{name: "other agents recorded in state", state: `{"installed_agents":["antigravity"]}`},
		{name: "unreadable state preserves GEMINI.md", state: `{not json`, want: true},
		{name: "no state and no selection", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			if tt.state != "" {
				writeFile(t, state.Path(home), tt.state)
			}
			if got := antigravityrules.GeminiCLIManaged(home, tt.selected); got != tt.want {
				t.Fatalf("GeminiCLIManaged() = %v, want %v", got, tt.want)
			}
		})
	}
}
