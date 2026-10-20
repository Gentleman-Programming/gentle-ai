package uninstall

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/cursor"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

type commandCodeTestAdapter struct{ *cursor.Adapter }

func newCommandCodeTestAdapter() agents.Adapter     { return commandCodeTestAdapter{cursor.NewAdapter()} }
func (commandCodeTestAdapter) Agent() model.AgentID { return model.AgentID("command-code") }
func (commandCodeTestAdapter) SettingsPath(home string) string {
	return filepath.Join(home, ".commandcode", "settings.json")
}
func (commandCodeTestAdapter) SystemPromptFile(home string) string {
	return filepath.Join(home, ".commandcode", "AGENTS.md")
}
func (commandCodeTestAdapter) SystemPromptStrategy() model.SystemPromptStrategy {
	return model.StrategyMarkdownSections
}
func (commandCodeTestAdapter) MCPStrategy() model.MCPStrategy {
	return model.StrategyMCPConfigFile
}
func (commandCodeTestAdapter) MCPConfigPath(home, _ string) string {
	return filepath.Join(home, ".commandcode", "mcp.json")
}
func (commandCodeTestAdapter) GlobalConfigDir(home string) string {
	return filepath.Join(home, ".commandcode")
}
func (commandCodeTestAdapter) SkillsDir(home string) string {
	return filepath.Join(home, ".commandcode", "skills")
}
func (commandCodeTestAdapter) SupportsSkills() bool       { return true }
func (commandCodeTestAdapter) SupportsSystemPrompt() bool { return true }
func (commandCodeTestAdapter) SupportsMCP() bool          { return true }
func (commandCodeTestAdapter) SupportsOutputStyles() bool { return false }

func TestCommandCodeUninstall_SettingsHooks(t *testing.T) {
	home := t.TempDir()
	settingsPath := filepath.Join(home, ".commandcode", "settings.json")
	_ = os.MkdirAll(filepath.Dir(settingsPath), 0o755)

	initial := map[string]any{
		"userSetting": "preserve-me",
		"hooks": map[string]any{
			"SessionStart": []any{map[string]any{"matcher": "", "hooks": []any{
				map[string]any{"type": "command", "command": `gentle-ai skill-registry refresh --quiet --no-gitignore --cwd "${COMMANDCODE_PROJECT_DIR:-$PWD}" || true`},
				map[string]any{"type": "command", "command": "user-custom-session-hook"},
			}}},
			"Stop": []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "user-custom-stop-hook"}}}},
		},
	}
	data, _ := json.MarshalIndent(initial, "", "  ")
	_ = os.WriteFile(settingsPath, data, 0o644)

	op := rewriteSkillRegistryHook(settingsPath)
	changed, removed, err := op.apply(settingsPath)
	if err != nil || !changed || removed {
		t.Fatalf("op.apply = (%v, %v, %v), want (true, false, nil)", changed, removed, err)
	}

	updated, _ := os.ReadFile(settingsPath)
	if strings.Contains(string(updated), "skill-registry refresh") {
		t.Errorf("managed hook not pruned: %s", string(updated))
	}
	if !strings.Contains(string(updated), "preserve-me") || !strings.Contains(string(updated), "user-custom-session-hook") || !strings.Contains(string(updated), "user-custom-stop-hook") {
		t.Errorf("user content lost: %s", string(updated))
	}

	// Idempotency: second apply must not change file
	if changed2, removed2, err := op.apply(settingsPath); err != nil || changed2 || removed2 {
		t.Errorf("second op.apply = (%v, %v, %v), want (false, false, nil)", changed2, removed2, err)
	}

	// Empty settings file is removed
	onlyManaged := map[string]any{"hooks": map[string]any{"SessionStart": []any{map[string]any{"matcher": "", "hooks": []any{
		map[string]any{"type": "command", "command": `gentle-ai skill-registry refresh --quiet --no-gitignore --cwd "${COMMANDCODE_PROJECT_DIR:-$PWD}" || true`},
	}}}}}
	d2, _ := json.MarshalIndent(onlyManaged, "", "  ")
	_ = os.WriteFile(settingsPath, d2, 0o644)
	if changed3, removed3, err := op.apply(settingsPath); err != nil || !changed3 || !removed3 {
		t.Fatalf("empty settings apply = (%v, %v, %v), want (true, true, nil)", changed3, removed3, err)
	}
	if _, err := os.Stat(settingsPath); !os.IsNotExist(err) {
		t.Errorf("expected %s deleted, stat = %v", settingsPath, err)
	}
}

func TestCommandCodeUninstall_AGENTSSections(t *testing.T) {
	home := t.TempDir()
	agentsPath := filepath.Join(home, ".commandcode", "AGENTS.md")
	_ = os.MkdirAll(filepath.Dir(agentsPath), 0o755)

	content := strings.Join([]string{
		"# User Instructions", "Keep intact.", "",
		"<!-- gentle-ai:persona -->", "## Rules", "Persona rules.", "<!-- /gentle-ai:persona -->", "",
		"<!-- gentle-ai:engram-protocol -->", "## Engram Protocol", "Engram content.", "<!-- /gentle-ai:engram-protocol -->", "",
		"## User Footer", "Footer intact.",
	}, "\n") + "\n"
	_ = os.WriteFile(agentsPath, []byte(content), 0o644)

	svc, _ := NewService(home, t.TempDir(), "dev")
	adapter := newCommandCodeTestAdapter()

	for _, comp := range []model.ComponentID{model.ComponentPersona, model.ComponentEngram} {
		ops, _, err := svc.componentOperations(adapter, comp)
		if err != nil {
			t.Fatalf("componentOperations(%q) error = %v", comp, err)
		}
		for _, op := range ops {
			if op.typeID == opRewriteFile && op.path == agentsPath {
				if _, _, err := op.apply(agentsPath); err != nil {
					t.Fatalf("apply error: %v", err)
				}
			}
		}
	}

	data, _ := os.ReadFile(agentsPath)
	res := string(data)
	if strings.Contains(res, "gentle-ai:persona") || strings.Contains(res, "gentle-ai:engram-protocol") {
		t.Errorf("managed sections retained: %s", res)
	}
	if !strings.Contains(res, "# User Instructions") || !strings.Contains(res, "## User Footer") {
		t.Errorf("user text lost: %s", res)
	}

	// Idempotency: re-running operations leaves file untouched
	for _, comp := range []model.ComponentID{model.ComponentPersona, model.ComponentEngram} {
		ops, _, _ := svc.componentOperations(adapter, comp)
		for _, op := range ops {
			if op.typeID == opRewriteFile && op.path == agentsPath {
				if ch, rm, err := op.apply(agentsPath); err != nil || ch || rm {
					t.Errorf("second apply = (%v, %v, %v), want (false, false, nil)", ch, rm, err)
				}
			}
		}
	}
}

func TestCommandCodeUninstall_Context7Server(t *testing.T) {
	home := t.TempDir()
	mcpPath := filepath.Join(home, ".commandcode", "mcp.json")
	_ = os.MkdirAll(filepath.Dir(mcpPath), 0o755)

	initial := map[string]any{"mcpServers": map[string]any{
		"context7": map[string]any{"command": "npx", "args": []any{"-y", "@upstash/context7-mcp@latest"}},
		"user-mcp": map[string]any{"command": "my-tool"},
	}}
	data, _ := json.MarshalIndent(initial, "", "  ")
	_ = os.WriteFile(mcpPath, data, 0o644)

	svc, _ := NewService(home, t.TempDir(), "dev")
	adapter := newCommandCodeTestAdapter()

	c7Ops, _, err := svc.componentOperations(adapter, model.ComponentContext7)
	if err != nil {
		t.Fatalf("componentOperations(Context7) error = %v", err)
	}
	for _, op := range c7Ops {
		if op.typeID == opRewriteFile && op.path == mcpPath {
			if ch, rm, err := op.apply(mcpPath); err != nil || !ch || rm {
				t.Fatalf("apply Context7 = (%v, %v, %v), want (true, false, nil)", ch, rm, err)
			}
		}
	}

	updated, _ := os.ReadFile(mcpPath)
	if strings.Contains(string(updated), "context7") {
		t.Errorf("context7 still present: %s", string(updated))
	}
	if !strings.Contains(string(updated), "user-mcp") {
		t.Errorf("user-mcp was lost: %s", string(updated))
	}

	// Idempotency
	for _, op := range c7Ops {
		if op.typeID == opRewriteFile && op.path == mcpPath {
			if ch, rm, err := op.apply(mcpPath); err != nil || ch || rm {
				t.Errorf("second apply = (%v, %v, %v), want (false, false, nil)", ch, rm, err)
			}
		}
	}

	// Empty mcp.json is deleted when only context7 is present
	onlyC7 := map[string]any{"mcpServers": map[string]any{
		"context7": map[string]any{"command": "npx"},
	}}
	d2, _ := json.MarshalIndent(onlyC7, "", "  ")
	_ = os.WriteFile(mcpPath, d2, 0o644)
	for _, op := range c7Ops {
		if op.typeID == opRewriteFile && op.path == mcpPath {
			if ch, rm, err := op.apply(mcpPath); err != nil || !ch || !rm {
				t.Fatalf("empty mcp apply = (%v, %v, %v), want (true, true, nil)", ch, rm, err)
			}
		}
	}
	if _, err := os.Stat(mcpPath); !os.IsNotExist(err) {
		t.Errorf("expected %s deleted, stat = %v", mcpPath, err)
	}
}

func TestCommandCodeUninstall_Skills(t *testing.T) {
	home := t.TempDir()
	skillDir := filepath.Join(home, ".commandcode", "skills")
	managedSkill := filepath.Join(skillDir, "go-testing", "SKILL.md")
	userSkill := filepath.Join(skillDir, "my-custom-skill", "SKILL.md")

	_ = os.MkdirAll(filepath.Dir(managedSkill), 0o755)
	_ = os.MkdirAll(filepath.Dir(userSkill), 0o755)
	_ = os.WriteFile(managedSkill, []byte("# go-testing"), 0o644)
	_ = os.WriteFile(userSkill, []byte("# custom"), 0o644)

	svc, _ := NewService(home, t.TempDir(), "dev")
	adapter := newCommandCodeTestAdapter()

	skillOps, _, err := svc.componentOperations(adapter, model.ComponentSkills)
	if err != nil {
		t.Fatalf("componentOperations(Skills) error = %v", err)
	}
	for _, op := range skillOps {
		if _, _, err := op.apply(op.path); err != nil {
			t.Fatalf("apply skill op (%s) error = %v", op.path, err)
		}
	}

	if _, err := os.Stat(filepath.Dir(managedSkill)); !os.IsNotExist(err) {
		t.Errorf("managed skill dir %q still exists: %v", filepath.Dir(managedSkill), err)
	}
	if _, err := os.Stat(userSkill); err != nil {
		t.Errorf("user skill %q should survive: %v", userSkill, err)
	}

	// Idempotency: second apply causes no error
	for _, op := range skillOps {
		if _, _, err := op.apply(op.path); err != nil {
			t.Errorf("second skill op (%s) error = %v", op.path, err)
		}
	}
}
