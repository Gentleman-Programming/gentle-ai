package skills

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/claude"
	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/vscode"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/skillregistry"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

func claudeAdapter() agents.Adapter   { return claude.NewAdapter() }
func opencodeAdapter() agents.Adapter { return opencode.NewAdapter() }

func TestInjectCommentWriterLanguageContractForOpenCode(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, opencodeAdapter(), []model.SkillID{model.SkillCommentWriter})
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("Inject() first changed = false")
	}

	path := filepath.Join(home, ".config", "opencode", "skills", "comment-writer", "SKILL.md")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	text := string(content)

	for _, required := range []string{
		"target context language",
		"explicitly requests a language",
		"neutral/professional Spanish by default",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("installed comment-writer missing language contract %q", required)
		}
	}
	if strings.Contains(text, "If writing in Spanish, use Rioplatense Spanish/voseo") {
		t.Fatal("installed comment-writer still forces Rioplatense Spanish for all Spanish comments")
	}
}

func TestInjectWritesSkillFilesForOpenCode(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, opencodeAdapter(), []model.SkillID{model.SkillCreator})
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("Inject() first changed = false")
	}

	if len(skillFiles(result.Files)) != 2 {
		t.Fatalf("Inject() files len = %d, want SKILL.md plus local reference", len(skillFiles(result.Files)))
	}

	path := filepath.Join(home, ".config", "opencode", "skills", "skill-creator", "SKILL.md")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected skill file %q: %v", path, err)
	}
	assertNonEmptyFile(t, filepath.Join(home, ".config", "opencode", "skills", "skill-creator", "references", "skill-style-guide.md"))

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if len(content) == 0 {
		t.Fatalf("skill file is empty")
	}

	// Idempotent: second inject should not change.
	second, err := Inject(home, opencodeAdapter(), []model.SkillID{model.SkillCreator})
	if err != nil {
		t.Fatalf("Inject() second error = %v", err)
	}

	if second.Changed {
		t.Fatalf("Inject() second changed = true")
	}
}

func TestInjectWritesSkillFilesForClaude(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, claudeAdapter(), []model.SkillID{model.SkillCreator, model.SkillGoTesting})
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("Inject() changed = false")
	}

	for _, path := range []string{
		filepath.Join(home, ".claude", "skills", "skill-creator", "SKILL.md"),
		filepath.Join(home, ".claude", "skills", "go-testing", "SKILL.md"),
		filepath.Join(home, ".claude", "skills", "go-testing", "references", "examples.md"),
	} {
		if !containsFile(result.Files, path) {
			t.Fatalf("Inject() files = %v, missing %q", result.Files, path)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected skill file %q: %v", path, err)
		}
	}
}

func TestInjectCopiesNonSDDSkillReferences(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, opencodeAdapter(), []model.SkillID{model.SkillGoTesting, model.SkillChainedPR})
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Inject() changed = false")
	}

	skillsDir := filepath.Join(home, ".config", "opencode", "skills")
	tests := []struct {
		name string
		path string
	}{
		{name: "go-testing examples", path: filepath.Join(skillsDir, "go-testing", "references", "examples.md")},
		{name: "chained-pr details", path: filepath.Join(skillsDir, "chained-pr", "references", "chaining-details.md")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertNonEmptyFile(t, tt.path)
		})
	}
}

func TestInjectSkipsSddSkills(t *testing.T) {
	home := t.TempDir()

	// Retired SDD IDs must not install even when explicitly requested.
	result, err := Inject(home, claudeAdapter(), []model.SkillID{
		model.SkillSDDInit,
		model.SkillSDDApply,
		model.SkillCreator,
	})
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	// Only the non-SDD skill (skill-creator) should be written, including its local references.
	if len(skillFiles(result.Files)) != 2 {
		t.Fatalf("Inject() files len = %d, want 2 (skill-creator plus local reference)", len(skillFiles(result.Files)))
	}

	// No retired skill files may be created.
	for _, id := range []model.SkillID{model.SkillSDDInit, model.SkillSDDApply} {
		path := filepath.Join(home, ".claude", "skills", string(id), "SKILL.md")
		if _, statErr := os.Stat(path); statErr == nil {
			t.Fatalf("skills component must not write retired skill %q", id)
		}
	}
}

func TestInjectSkipsUnknownSkillGracefully(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, opencodeAdapter(), []model.SkillID{
		model.SkillCreator,
		model.SkillID("nonexistent-skill"),
	})
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	if len(skillFiles(result.Files)) != 2 {
		t.Fatalf("Inject() files len = %d, want 2", len(skillFiles(result.Files)))
	}

	if len(result.Skipped) != 1 {
		t.Fatalf("Inject() skipped len = %d, want 1", len(result.Skipped))
	}

	if result.Skipped[0] != "nonexistent-skill" {
		t.Fatalf("Inject() skipped[0] = %q, want nonexistent-skill", result.Skipped[0])
	}
}

// noSkillsAdapter is a mock adapter that does not support skills.
type noSkillsAdapter struct{}

func (a noSkillsAdapter) Agent() model.AgentID    { return "no-skills" }
func (a noSkillsAdapter) Tier() model.SupportTier { return model.TierFull }
func (a noSkillsAdapter) Detect(_ context.Context, _ string) (bool, string, string, bool, error) {
	return false, "", "", false, nil
}
func (a noSkillsAdapter) InstallCommand(_ system.PlatformProfile) ([][]string, error) {
	return nil, nil
}
func (a noSkillsAdapter) GlobalConfigDir(_ string) string  { return "" }
func (a noSkillsAdapter) SystemPromptDir(_ string) string  { return "" }
func (a noSkillsAdapter) SystemPromptFile(_ string) string { return "" }
func (a noSkillsAdapter) SkillsDir(_ string) string        { return "" }
func (a noSkillsAdapter) SettingsPath(_ string) string     { return "" }
func (a noSkillsAdapter) SystemPromptStrategy() model.SystemPromptStrategy {
	return model.StrategyFileReplace
}
func (a noSkillsAdapter) MCPStrategy() model.MCPStrategy          { return model.StrategyMergeIntoSettings }
func (a noSkillsAdapter) MCPConfigPath(_ string, _ string) string { return "" }
func (a noSkillsAdapter) SupportsOutputStyles() bool              { return false }
func (a noSkillsAdapter) OutputStyleDir(_ string) string          { return "" }
func (a noSkillsAdapter) SupportsSlashCommands() bool             { return false }
func (a noSkillsAdapter) CommandsDir(_ string) string             { return "" }
func (a noSkillsAdapter) SupportsSubAgents() bool                 { return false }
func (a noSkillsAdapter) SubAgentsDir(_ string) string            { return "" }
func (a noSkillsAdapter) EmbeddedSubAgentsDir() string            { return "" }
func (a noSkillsAdapter) SupportsSkills() bool                    { return false }
func (a noSkillsAdapter) SupportsSystemPrompt() bool              { return false }
func (a noSkillsAdapter) SupportsMCP() bool                       { return false }

func TestInjectSkipsUnsupportedAgent(t *testing.T) {
	home := t.TempDir()

	// Mock adapter that does not support skills — Inject should skip gracefully.
	result, injectErr := Inject(home, noSkillsAdapter{}, []model.SkillID{model.SkillCreator})
	if injectErr != nil {
		t.Fatalf("Inject() unexpected error = %v", injectErr)
	}

	// All skills should be skipped.
	if len(result.Skipped) != 1 {
		t.Fatalf("Inject() skipped = %v, want 1 skill", result.Skipped)
	}
	if result.Changed {
		t.Fatal("Inject() changed = true, want false for unsupported agent")
	}
}

func TestInjectVSCodeWritesSkillFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	adapter := vscode.NewAdapter()

	result, err := Inject(home, adapter, []model.SkillID{model.SkillCreator})
	if err != nil {
		t.Fatalf("Inject(vscode) error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("Inject(vscode) changed = false")
	}
	if len(skillFiles(result.Files)) != 2 {
		t.Fatalf("Inject(vscode) files len = %d, want 2", len(skillFiles(result.Files)))
	}

	path := filepath.Join(home, ".copilot", "skills", "skill-creator", "SKILL.md")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected skill file %q: %v", path, err)
	}
}

func TestInjectUsesRealEmbeddedContent(t *testing.T) {
	home := t.TempDir()

	_, err := Inject(home, claudeAdapter(), []model.SkillID{model.SkillCreator})
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	path := filepath.Join(home, ".claude", "skills", "skill-creator", "SKILL.md")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	// Real embedded content should be substantial (not a one-line stub).
	if len(content) < 100 {
		t.Fatalf("skill file content looks like a stub (len=%d)", len(content))
	}
}

func TestInjectRequiredBundledSkillsForEverySkillsCapableDefaultAdapter(t *testing.T) {
	registry, err := agents.NewDefaultRegistry()
	if err != nil {
		t.Fatalf("NewDefaultRegistry() error = %v", err)
	}

	required := requiredBundledSkillIDs()

	for _, agentID := range registry.SupportedAgents() {
		adapter, ok := registry.Get(agentID)
		if !ok {
			t.Fatalf("registry missing adapter %q", agentID)
		}
		if !adapter.SupportsSkills() {
			continue
		}

		t.Run(string(agentID), func(t *testing.T) {
			home := t.TempDir()
			result, injectErr := InjectWithCapability(home, adapter, required, "capable")
			if injectErr != nil {
				t.Fatalf("InjectWithCapability() error = %v", injectErr)
			}
			if !result.Changed {
				t.Fatalf("InjectWithCapability() changed = false")
			}
			if len(result.Skipped) != 0 {
				t.Fatalf("InjectWithCapability() skipped = %v, want none", result.Skipped)
			}

			skillsDir := adapter.SkillsDir(home)
			if skillsDir == "" {
				t.Fatalf("adapter %q supports skills but returned empty SkillsDir", agentID)
			}

			entries := skillregistry.List(t.TempDir(), home)
			for _, id := range required {
				path := filepath.Join(skillsDir, string(id), "SKILL.md")
				assertNonEmptyFile(t, path)
				if id == model.SkillSkillRegistry {
					continue // Registry plumbing intentionally excludes itself.
				}
				found := false
				for _, entry := range entries {
					if entry.Name == string(id) && entry.Path == path {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("skill registry did not discover %q at %q", id, path)
				}
			}
		})
	}
}

func TestInjectBundledSkillsAreRegistryDiscoverable(t *testing.T) {
	for _, skill := range []model.SkillID{
		model.SkillRDDDefectWorkflow,
		model.SkillSystemicIssueTriage,
		model.SkillGentleAIBench,
	} {
		t.Run(string(skill), func(t *testing.T) {
			home := t.TempDir()
			project := t.TempDir()

			_, err := Inject(home, opencodeAdapter(), []model.SkillID{skill})
			if err != nil {
				t.Fatalf("Inject() error = %v", err)
			}

			wantPath := filepath.Join(home, ".config", "opencode", "skills", string(skill), "SKILL.md")
			for _, entry := range skillregistry.List(project, home) {
				if entry.Name == string(skill) && entry.Path == wantPath {
					return
				}
			}

			t.Fatalf("skill registry did not discover %q at %q", skill, wantPath)
		})
	}
}

func TestInjectSkillCreatorAndImproverInstallLocalStyleGuideReference(t *testing.T) {
	home := t.TempDir()

	_, err := Inject(home, opencodeAdapter(), []model.SkillID{model.SkillCreator, model.SkillImprover})
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	skillsDir := filepath.Join(home, ".config", "opencode", "skills")
	for _, id := range []model.SkillID{model.SkillCreator, model.SkillImprover} {
		t.Run(string(id), func(t *testing.T) {
			guidePath := filepath.Join(skillsDir, string(id), "references", "skill-style-guide.md")
			assertNonEmptyFile(t, guidePath)

			skillContent, readErr := os.ReadFile(filepath.Join(skillsDir, string(id), "SKILL.md"))
			if readErr != nil {
				t.Fatalf("ReadFile(SKILL.md) error = %v", readErr)
			}
			text := string(skillContent)
			if !strings.Contains(text, "docs/skill-style-guide.md") {
				t.Fatalf("%s SKILL.md must preserve repo normative guide reference", id)
			}
			if !strings.Contains(text, "references/skill-style-guide.md") {
				t.Fatalf("%s SKILL.md must reference installed local style guide", id)
			}
		})
	}
}

func TestSkillPathForAgent(t *testing.T) {
	path := SkillPathForAgent("/home/test", claudeAdapter(), model.SkillCreator)
	want := filepath.Join("/home/test", ".claude", "skills", "skill-creator", "SKILL.md")
	if path != want {
		t.Fatalf("SkillPathForAgent() = %q, want %q", path, want)
	}

	path = SkillPathForAgent("/home/test", opencodeAdapter(), model.SkillCreator)
	want = filepath.Join("/home/test", ".config", "opencode", "skills", "skill-creator", "SKILL.md")
	if path != want {
		t.Fatalf("SkillPathForAgent() = %q, want %q", path, want)
	}
}

func assertNonEmptyFile(t *testing.T, path string) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("expected file %q: %v", path, err)
	}
	if info.Size() == 0 {
		t.Fatalf("expected file %q to be non-empty", path)
	}
}

func TestInjectWithCapabilitySkipsRetiredSDDSkills(t *testing.T) {
	for _, capability := range []string{"", "capable", "small"} {
		t.Run(capability, func(t *testing.T) {
			home := t.TempDir()
			skillDir := opencodeAdapter().SkillsDir(home)
			ids := []model.SkillID{model.SkillSDDApply, model.SkillJudgmentDay}
			paths, err := DirectoryPaths(skillDir, ids, capability)
			if err != nil {
				t.Fatalf("DirectoryPaths() error = %v", err)
			}
			for _, path := range paths {
				if strings.Contains(path, "sdd-apply") {
					t.Fatalf("DirectoryPaths() includes retired skill: %q", path)
				}
			}
			result, err := InjectWithCapability(home, opencodeAdapter(), ids, capability)
			if err != nil {
				t.Fatalf("InjectWithCapability() error = %v", err)
			}
			if len(result.Skipped) != 0 || len(result.Files) == 0 {
				t.Fatalf("InjectWithCapability() = %+v, want retained skill only", result)
			}
			assertNonEmptyFile(t, filepath.Join(skillDir, "judgment-day", "SKILL.md"))
			if _, err := os.Stat(filepath.Join(skillDir, "sdd-apply", "SKILL.md")); !os.IsNotExist(err) {
				t.Fatalf("retired skill unexpectedly installed: %v", err)
			}
		})
	}
}

func TestInjectWithCapability_WritesNonSDDSkillsRegardlessOfCapability(t *testing.T) {
	home := t.TempDir()

	// Non-SDD skills should always be written, regardless of capability.
	result, err := InjectWithCapability(home, opencodeAdapter(), []model.SkillID{model.SkillCreator}, "capable")
	if err != nil {
		t.Fatalf("InjectWithCapability() error = %v", err)
	}
	if len(skillFiles(result.Files)) != 2 {
		t.Fatalf("InjectWithCapability() files len = %d, want 2", len(skillFiles(result.Files)))
	}
	if len(result.Skipped) != 0 {
		t.Fatalf("InjectWithCapability() skipped len = %d, want 0", len(result.Skipped))
	}
}

func TestInjectWithCapabilityRetainsSkillFrontmatter(t *testing.T) {
	home := t.TempDir()
	_, err := InjectWithCapability(home, opencodeAdapter(), []model.SkillID{model.SkillJudgmentDay}, "capable")
	if err != nil {
		t.Fatalf("InjectWithCapability() error = %v", err)
	}
	path := filepath.Join(home, ".config", "opencode", "skills", "judgment-day", "SKILL.md")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.HasPrefix(string(content), "---\n") {
		t.Fatalf("retained skill must start with YAML frontmatter delimiter, got prefix %q", string(content[:min(len(content), 16)]))
	}
}

func containsFile(files []string, want string) bool {
	for _, file := range files {
		if file == want {
			return true
		}
	}
	return false
}

func requiredBundledSkillIDs() []model.SkillID {
	return []model.SkillID{
		model.SkillCreator,
		model.SkillSkillRegistry,
		model.SkillCognitiveDoc,
		model.SkillCommentWriter,
		model.SkillJudgmentDay,
		model.SkillWorkUnitCommits,
		model.SkillGoTesting,
		model.SkillImprover,
		model.SkillRDDDefectWorkflow,
		model.SkillSystemicIssueTriage,
		model.SkillGentleAIBench,
	}
}

func TestInjectBackupsExistingSkillFiles(t *testing.T) {
	// Create a temporary home with an existing user-authored skill file
	home := t.TempDir()
	// Inject a skill first to establish the file
	homeDir := home
	_, err := Inject(homeDir, opencodeAdapter(), []model.SkillID{model.SkillCreator})
	if err != nil {
		t.Fatalf("first Inject() error = %v", err)
	}

	// Verify the file was created
	skillsDir := filepath.Join(homeDir, ".config", "opencode", "skills")
	tskPath := filepath.Join(skillsDir, "skill-creator", "SKILL.md")
	_, err = os.ReadFile(tskPath)
	if err != nil {
		t.Fatalf("read original skill: %v", err)
	}

	// Modify the skill file to simulate a user-authored change
	userContent := []byte("---\nname: skill-creator\ndescription: USER MODIFIED\n---\n# User Modified\n")
	if err := os.WriteFile(tskPath, userContent, 0o644); err != nil {
		t.Fatalf("write user modification: %v", err)
	}

	// Inject again - this should backup the user-modified file before overwriting
	_, err = Inject(homeDir, opencodeAdapter(), []model.SkillID{model.SkillCreator})
	if err != nil {
		t.Fatalf("second Inject() error = %v", err)
	}

	// Verify the file was overwritten with the managed skill content
	newContent, err := os.ReadFile(tskPath)
	if err != nil {
		t.Fatalf("read new skill: %v", err)
	}
	if string(newContent) == string(userContent) {
		t.Fatalf("skill file was not overwritten")
	}
	if !strings.Contains(string(newContent), "Trigger: new skills") { // skill-creator managed content
		t.Fatalf("managed skill content not written")
	}

	// Verify backup was created in ~/.gentle-ai/backups/timestamp/
	backupRoot := filepath.Join(homeDir, ".gentle-ai", "backups")
	entries, err := os.ReadDir(backupRoot)
	if err != nil {
		t.Fatalf("read backup root: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("no backup directories created")
	}
	latestBackup := entries[len(entries)-1]
	backupPath := filepath.Join(backupRoot, latestBackup.Name(), "skill-creator", "SKILL.md")
	backupContent, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(backupContent) != string(userContent) {
		t.Fatalf("backup does not match user-modified content: got %q, want %q", string(backupContent), string(userContent))
	}
}

func TestBackupExistingPreservesContentAndMode(t *testing.T) {
	home := t.TempDir()
	skillDir := filepath.Join(home, "skills")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(skillDir, "sub", "skill.md")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("contenido original"), 0o640); err != nil {
		t.Fatal(err)
	}

	backupExisting(home, skillDir, dest, "20261022000000")

	backupPath := filepath.Join(home, ".gentle-ai", "backups", "20261022000000", "sub", "skill.md")
	got, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("backup missing: %v", err)
	}
	if string(got) != "contenido original" {
		t.Fatalf("content mismatch: %q", got)
	}
	if info, err := os.Stat(backupPath); err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("file mode = %v (err %v), want 0640", info.Mode().Perm(), err)
	}
	if dirInfo, err := os.Stat(filepath.Dir(backupPath)); err != nil || dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode = %v (err %v), want 0700", dirInfo.Mode().Perm(), err)
	}
}

func TestBackupExistingNeverTruncatesOnCollision(t *testing.T) {
	home := t.TempDir()
	skillDir := filepath.Join(home, "skills")
	dest := filepath.Join(skillDir, "skill.md")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("v1"), 0o640); err != nil {
		t.Fatal(err)
	}
	const ts = "20261022000000"
	backupExisting(home, skillDir, dest, ts)
	if err := os.WriteFile(dest, []byte("v2"), 0o640); err != nil {
		t.Fatal(err)
	}
	backupExisting(home, skillDir, dest, ts)

	first, err := os.ReadFile(filepath.Join(home, ".gentle-ai", "backups", ts, "skill.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != "v1" {
		t.Fatalf("the first backup was truncated: %q", first)
	}
	second, err := os.ReadFile(filepath.Join(home, ".gentle-ai", "backups", ts, "skill.md.1"))
	if err != nil {
		t.Fatalf("the collision suffix is missing: %v", err)
	}
	if string(second) != "v2" {
		t.Fatalf("collision backup content: %q", second)
	}
}
