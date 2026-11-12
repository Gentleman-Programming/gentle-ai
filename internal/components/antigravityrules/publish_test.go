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

func TestPublishWritesConfigAgentsMDAndSymlink(t *testing.T) {
	home := t.TempDir()
	staging := filemerge.InjectMarkdownSection("", "persona", "Persona text.\n")
	staging = filemerge.InjectMarkdownSection(staging, "engram-protocol", "Engram protocol.\n")
	writeFile(t, antigravityrules.StagingPath(home), staging)

	result, err := antigravityrules.Publish(home)
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("expected Publish() changed = true")
	}

	configAgents := antigravityrules.ConfigAgentsMDPath(home)
	if got := readFile(t, configAgents); got != staging {
		t.Fatalf("config AGENTS.md = %q, want %q", got, staging)
	}

	homeAgents := antigravityrules.AgentsMDPath(home)
	info, err := os.Lstat(homeAgents)
	if err != nil {
		t.Fatalf("Lstat(home AGENTS.md) error = %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("home AGENTS.md is not a symlink; mode = %v", info.Mode())
	}
	target, err := os.Readlink(homeAgents)
	if err != nil {
		t.Fatalf("Readlink(home AGENTS.md) error = %v", err)
	}
	wantTarget := filepath.Join(".gemini", "config", "AGENTS.md")
	if target != wantTarget {
		t.Fatalf("symlink target = %q, want %q", target, wantTarget)
	}

	// Idempotent publish
	again, err := antigravityrules.Publish(home)
	if err != nil || again.Changed {
		t.Fatalf("second Publish() = %+v, %v; want unchanged", again, err)
	}

	// Empty staging retires files
	writeFile(t, antigravityrules.StagingPath(home), "")
	retired, err := antigravityrules.Publish(home)
	if err != nil {
		t.Fatalf("Publish(empty) error = %v", err)
	}
	if !retired.Changed {
		t.Fatal("expected Publish(empty) changed = true")
	}
	if _, err := os.Stat(configAgents); !os.IsNotExist(err) {
		t.Fatalf("config AGENTS.md survived retirement: %v", err)
	}
	if _, err := os.Lstat(homeAgents); !os.IsNotExist(err) {
		t.Fatalf("home AGENTS.md symlink survived retirement: %v", err)
	}
}

func TestPublishMigratesLegacyGeminiSymlink(t *testing.T) {
	home := t.TempDir()
	legacyLink := antigravityrules.AgentsMDPath(home)
	if err := os.Symlink(filepath.Join(".gemini", "GEMINI.md"), legacyLink); err != nil {
		t.Fatal(err)
	}

	staging := filemerge.InjectMarkdownSection("", "persona", "Persona text.\n")
	writeFile(t, antigravityrules.StagingPath(home), staging)

	result, err := antigravityrules.Publish(home)
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("expected Publish() changed = true")
	}

	target, err := os.Readlink(legacyLink)
	if err != nil {
		t.Fatalf("Readlink() error = %v", err)
	}
	wantTarget := filepath.Join(".gemini", "config", "AGENTS.md")
	if target != wantTarget {
		t.Fatalf("migrated symlink target = %q, want %q", target, wantTarget)
	}
}

func TestPublishPreservesRegularAgentsMDUserContent(t *testing.T) {
	home := t.TempDir()
	agentsPath := antigravityrules.AgentsMDPath(home)
	userContent := "# My Custom Instructions\n\nRun tests before commit.\n"
	writeFile(t, agentsPath, userContent)

	staging := filemerge.InjectMarkdownSection("", "persona", "Persona text.\n")
	writeFile(t, antigravityrules.StagingPath(home), staging)

	result, err := antigravityrules.Publish(home)
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("expected Publish() changed = true")
	}

	content := readFile(t, agentsPath)
	if !strings.Contains(content, userContent) {
		t.Fatalf("user content was lost in regular AGENTS.md:\n%s", content)
	}
	if !strings.Contains(content, "<!-- gentle-ai:persona -->") {
		t.Fatalf("missing persona section in regular AGENTS.md:\n%s", content)
	}

	// Retirement removes only gentle-ai sections, keeping user content
	writeFile(t, antigravityrules.StagingPath(home), "")
	retired, err := antigravityrules.Publish(home)
	if err != nil {
		t.Fatalf("Publish(empty) error = %v", err)
	}
	if !retired.Changed {
		t.Fatal("expected Publish(empty) changed = true")
	}
	afterContent := readFile(t, agentsPath)
	if strings.Contains(afterContent, "<!-- gentle-ai:persona -->") {
		t.Fatalf("persona section survived retirement in regular AGENTS.md:\n%s", afterContent)
	}
	if !strings.Contains(afterContent, "Run tests before commit.") {
		t.Fatalf("user content was lost after retirement in regular AGENTS.md:\n%s", afterContent)
	}
}

func TestPublishWritesRegularAgentsMDInWorkspace(t *testing.T) {
	workspace := t.TempDir()
	if err := os.Mkdir(filepath.Join(workspace, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	staging := filemerge.InjectMarkdownSection("", "persona", "Workspace persona.\n")
	writeFile(t, antigravityrules.StagingPath(workspace), staging)

	result, err := antigravityrules.Publish(workspace)
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("expected Publish() changed = true")
	}

	agentsPath := antigravityrules.AgentsMDPath(workspace)
	info, err := os.Lstat(agentsPath)
	if err != nil {
		t.Fatalf("Lstat() error = %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("workspace AGENTS.md must be a regular file, got symlink")
	}
	if got := readFile(t, agentsPath); got != staging {
		t.Fatalf("workspace AGENTS.md = %q, want %q", got, staging)
	}
}

func TestPublishReportsErrorOnDirectoryAGENTS(t *testing.T) {
	home := t.TempDir()
	agentsPath := antigravityrules.AgentsMDPath(home)
	if err := os.Mkdir(agentsPath, 0o755); err != nil {
		t.Fatal(err)
	}

	staging := filemerge.InjectMarkdownSection("", "persona", "Persona text.\n")
	writeFile(t, antigravityrules.StagingPath(home), staging)

	_, err := antigravityrules.Publish(home)
	if err == nil || !strings.Contains(err.Error(), "expected") || !strings.Contains(err.Error(), "got directory") {
		t.Fatalf("expected directory error, got: %v", err)
	}
}

func TestPublishPreservesUnrelatedSymlink(t *testing.T) {
	home := t.TempDir()
	unrelatedTarget := filepath.Join(t.TempDir(), "custom-GEMINI.md")
	writeFile(t, unrelatedTarget, "# Unrelated\n")

	agentsPath := antigravityrules.AgentsMDPath(home)
	if err := os.Symlink(unrelatedTarget, agentsPath); err != nil {
		t.Fatal(err)
	}

	staging := filemerge.InjectMarkdownSection("", "persona", "Persona text.\n")
	writeFile(t, antigravityrules.StagingPath(home), staging)

	result, err := antigravityrules.Publish(home)
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	// Symlink should remain untouched and pointing to unrelatedTarget
	info, err := os.Lstat(agentsPath)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("unrelated symlink was replaced: %v", err)
	}
	target, err := os.Readlink(agentsPath)
	if err != nil || target != unrelatedTarget {
		t.Fatalf("symlink target = %q, want %q", target, unrelatedTarget)
	}
	if result.Changed {
		for _, f := range result.Files {
			if f == agentsPath {
				t.Fatalf("agentsPath reported changed for unrelated symlink")
			}
		}
	}
}

func TestPublishWorkspaceWithoutGitDirCreatesRegularFile(t *testing.T) {
	workspace := t.TempDir() // No .git subdirectory

	staging := filemerge.InjectMarkdownSection("", "persona", "Non-git workspace persona.\n")
	writeFile(t, antigravityrules.StagingPath(workspace), staging)

	result, err := antigravityrules.Publish(workspace, antigravityrules.WithWorkspaceScope(true))
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("expected Publish() changed = true")
	}

	agentsPath := antigravityrules.AgentsMDPath(workspace)
	info, err := os.Lstat(agentsPath)
	if err != nil {
		t.Fatalf("Lstat() error = %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("workspace AGENTS.md must be a regular file, got symlink")
	}
	if got := readFile(t, agentsPath); got != staging {
		t.Fatalf("workspace AGENTS.md = %q, want %q", got, staging)
	}
}

func TestPublishRemovesRetiredAntigravitySectionsFromWorkspaceAGENTS(t *testing.T) {
	workspace := t.TempDir()
	agentsPath := antigravityrules.AgentsMDPath(workspace)

	initialContent := filemerge.InjectMarkdownSection("", "persona", "Active persona.\n")
	initialContent = filemerge.InjectMarkdownSection(initialContent, "engram-protocol", "Engram protocol.\n")
	writeFile(t, agentsPath, initialContent)

	// Staging now only has persona (engram was dropped/uninstalled)
	staging := filemerge.InjectMarkdownSection("", "persona", "Active persona.\n")
	writeFile(t, antigravityrules.StagingPath(workspace), staging)

	result, err := antigravityrules.Publish(workspace, antigravityrules.WithWorkspaceScope(true))
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("expected Publish() changed = true")
	}

	content := readFile(t, agentsPath)
	if !strings.Contains(content, "<!-- gentle-ai:persona -->") {
		t.Fatalf("expected persona section preserved, got:\n%s", content)
	}
	if strings.Contains(content, "<!-- gentle-ai:engram-protocol -->") {
		t.Fatalf("expected retired engram-protocol section removed, got:\n%s", content)
	}
}

func TestPublishPreservesOpenClawSectionsWhenOpenClawIsManaged(t *testing.T) {
	home := t.TempDir()
	agentsPath := antigravityrules.AgentsMDPath(home)

	initialContent := filemerge.InjectMarkdownSection("", "persona", "Antigravity persona.\n")
	initialContent = filemerge.InjectMarkdownSection(initialContent, "engram-protocol", "OpenClaw engram protocol.\n")
	initialContent = filemerge.InjectMarkdownSection(initialContent, "agent-routing", "OpenClaw routing guidance.\n")
	writeFile(t, agentsPath, initialContent)

	// Antigravity staging is emptied (uninstalled)
	writeFile(t, antigravityrules.StagingPath(home), "")

	// Publish with OpenClaw actively managed
	result, err := antigravityrules.Publish(home, antigravityrules.WithActiveAgents([]model.AgentID{model.AgentOpenClaw}))
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("expected Publish() changed = true")
	}

	content := readFile(t, agentsPath)
	if strings.Contains(content, "<!-- gentle-ai:persona -->") {
		t.Fatalf("expected Antigravity persona removed, got:\n%s", content)
	}
	if !strings.Contains(content, "<!-- gentle-ai:engram-protocol -->") {
		t.Fatalf("expected OpenClaw engram-protocol preserved, got:\n%s", content)
	}
	if !strings.Contains(content, "<!-- gentle-ai:agent-routing -->") {
		t.Fatalf("expected OpenClaw agent-routing preserved, got:\n%s", content)
	}
}
