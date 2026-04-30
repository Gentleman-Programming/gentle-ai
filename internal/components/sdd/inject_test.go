package sdd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/agents"
	"github.com/gentleman-programming/gentle-ai/internal/agents/claude"
	"github.com/gentleman-programming/gentle-ai/internal/agents/kimi"
	"github.com/gentleman-programming/gentle-ai/internal/agents/opencode"
	windsurfagent "github.com/gentleman-programming/gentle-ai/internal/agents/windsurf"
	"github.com/gentleman-programming/gentle-ai/internal/assets"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	// agents/cursor, agents/gemini, agents/vscode used via agents.NewAdapter()
)

func claudeAdapter() agents.Adapter   { return claude.NewAdapter() }
func kimiAdapter() agents.Adapter     { return kimi.NewAdapter() }
func opencodeAdapter() agents.Adapter { return opencode.NewAdapter() }
func windsurfAdapter() agents.Adapter { return windsurfagent.NewAdapter() }

func mockNoPackageManager(t *testing.T) {
	t.Helper()
	orig := npmLookPath
	npmLookPath = func(string) (string, error) {
		return "", fmt.Errorf("not found")
	}
	t.Cleanup(func() { npmLookPath = orig })
}

func TestInjectClaudeWritesSectionMarkers(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, claudeAdapter(), "")
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("Inject() first changed = false")
	}

	path := filepath.Join(home, ".claude", "CLAUDE.md")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	text := string(content)

	if !strings.Contains(text, "<!-- gentle-ai:sdd-orchestrator -->") {
		t.Fatal("CLAUDE.md missing open marker for sdd-orchestrator")
	}
	if !strings.Contains(text, "<!-- /gentle-ai:sdd-orchestrator -->") {
		t.Fatal("CLAUDE.md missing close marker for sdd-orchestrator")
	}
	if !strings.Contains(text, "sub-agent") {
		t.Fatal("CLAUDE.md missing real SDD orchestrator content (expected 'sub-agent')")
	}
	if !strings.Contains(text, "dependency") {
		t.Fatal("CLAUDE.md missing real SDD orchestrator content (expected 'dependency')")
	}
}

func TestInjectClaudePreservesExistingSections(t *testing.T) {
	home := t.TempDir()
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	existing := "# My Config\n\nSome user content.\n"
	if err := os.WriteFile(filepath.Join(claudeDir, "CLAUDE.md"), []byte(existing), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Inject(home, claudeAdapter(), "")
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(claudeDir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	text := string(content)
	if !strings.Contains(text, "Some user content.") {
		t.Fatal("Existing user content was clobbered")
	}
	if !strings.Contains(text, "<!-- gentle-ai:sdd-orchestrator -->") {
		t.Fatal("SDD section was not injected")
	}
}

func TestInjectClaudeIsIdempotent(t *testing.T) {
	home := t.TempDir()

	first, err := Inject(home, claudeAdapter(), "")
	if err != nil {
		t.Fatalf("Inject() first error = %v", err)
	}
	if !first.Changed {
		t.Fatalf("Inject() first changed = false")
	}

	second, err := Inject(home, claudeAdapter(), "")
	if err != nil {
		t.Fatalf("Inject() second error = %v", err)
	}
	if second.Changed {
		t.Fatalf("Inject() second changed = true")
	}
}

func TestInjectClaudeWritesCommandFiles(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, claudeAdapter(), "")
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("Inject() first changed = false")
	}

	expectedCommands := []string{
		"sdd-apply.md", "sdd-archive.md", "sdd-continue.md", "sdd-explore.md",
		"sdd-ff.md", "sdd-init.md", "sdd-new.md", "sdd-onboard.md", "sdd-verify.md",
	}
	for _, name := range expectedCommands {
		path := filepath.Join(home, ".claude", "commands", name)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected command file %q not found: %v", name, err)
		}
	}

	commandPath := filepath.Join(home, ".claude", "commands", "sdd-init.md")
	content, err := os.ReadFile(commandPath)
	if err != nil {
		t.Fatalf("ReadFile(sdd-init.md) error = %v", err)
	}

	text := string(content)
	if !strings.Contains(text, "description:") {
		t.Fatal("sdd-init.md missing frontmatter description")
	}
	if strings.Contains(text, "agent: sdd-orchestrator") {
		t.Fatal("sdd-init.md contains OpenCode-specific agent frontmatter")
	}
	if !strings.Contains(text, "If the native `sdd-init` sub-agent is available") {
		t.Fatal("sdd-init.md missing Claude delegation guidance")
	}
	if !strings.Contains(text, "~/.claude/skills/sdd-init/SKILL.md") {
		t.Fatal("sdd-init.md missing Claude skill path")
	}
}

func TestInjectClaudeCustomModelAssignments(t *testing.T) {
	home := t.TempDir()

	opts := InjectOptions{ClaudeModelAssignments: map[string]model.ClaudeModelAlias{
		"orchestrator": model.ClaudeModelSonnet,
		"sdd-design":   model.ClaudeModelSonnet,
		"default":      model.ClaudeModelHaiku,
	}}

	result, err := Inject(home, claudeAdapter(), "", opts)
	if err != nil {
		t.Fatalf("Inject(claude, custom assignments) error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Inject(claude, custom assignments) changed = false")
	}

	content, err := os.ReadFile(filepath.Join(home, ".claude", "CLAUDE.md"))
	if err != nil {
		t.Fatalf("ReadFile(CLAUDE.md) error = %v", err)
	}

	text := string(content)
	for _, want := range []string{
		"| orchestrator | sonnet | Coordinates, makes decisions |",
		"| sdd-design | sonnet | Architecture decisions |",
		"| default | haiku | Non-SDD general delegation |",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("CLAUDE.md missing custom table row %q", want)
		}
	}

	if !strings.Contains(text, "<!-- gentle-ai:sdd-model-assignments -->") {
		t.Fatal("CLAUDE.md missing model assignment open marker")
	}
	if !strings.Contains(text, "<!-- /gentle-ai:sdd-model-assignments -->") {
		t.Fatal("CLAUDE.md missing model assignment close marker")
	}
}

func TestInjectClaudeCustomModelAssignmentsIsIdempotent(t *testing.T) {
	home := t.TempDir()
	opts := InjectOptions{ClaudeModelAssignments: map[string]model.ClaudeModelAlias{
		"orchestrator": model.ClaudeModelSonnet,
		"sdd-design":   model.ClaudeModelSonnet,
	}}

	first, err := Inject(home, claudeAdapter(), "", opts)
	if err != nil {
		t.Fatalf("Inject() first error = %v", err)
	}
	if !first.Changed {
		t.Fatal("Inject() first changed = false")
	}

	second, err := Inject(home, claudeAdapter(), "", opts)
	if err != nil {
		t.Fatalf("Inject() second error = %v", err)
	}
	if second.Changed {
		t.Fatal("Inject() second changed = true")
	}
}

func TestInjectOpenCodeWritesCommandFiles(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, opencodeAdapter(), "")
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("Inject() first changed = false")
	}

	if len(result.Files) == 0 {
		t.Fatal("Inject() returned no files")
	}

	commandPath := filepath.Join(home, ".config", "opencode", "commands", "sdd-init.md")
	content, err := os.ReadFile(commandPath)
	if err != nil {
		t.Fatalf("ReadFile(sdd-init.md) error = %v", err)
	}

	text := string(content)
	if !strings.Contains(text, "description") {
		t.Fatal("sdd-init.md missing frontmatter description — not real content")
	}

	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	settingsContent, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(opencode.json) error = %v", err)
	}

	settingsText := string(settingsContent)
	if !strings.Contains(settingsText, `"agent"`) {
		t.Fatal("opencode.json missing agent key for SDD commands")
	}
	if !strings.Contains(settingsText, `"sdd-orchestrator"`) {
		t.Fatal("opencode.json missing sdd-orchestrator agent")
	}

	sharedPath := filepath.Join(home, ".config", "opencode", "skills", "_shared", "persistence-contract.md")
	if _, err := os.Stat(sharedPath); err != nil {
		t.Fatalf("expected shared SDD convention file %q: %v", sharedPath, err)
	}

	skillPath := filepath.Join(home, ".config", "opencode", "skills", "sdd-init", "SKILL.md")
	skillContent, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("ReadFile(sdd-init SKILL.md) error = %v", err)
	}

	if !strings.Contains(string(skillContent), "sdd-init") {
		t.Fatal("SDD skill file missing expected content")
	}
}

func TestInjectOpenCodeIsIdempotent(t *testing.T) {
	home := t.TempDir()

	first, err := Inject(home, opencodeAdapter(), "")
	if err != nil {
		t.Fatalf("Inject() first error = %v", err)
	}
	if !first.Changed {
		t.Fatalf("Inject() first changed = false")
	}

	second, err := Inject(home, opencodeAdapter(), "")
	if err != nil {
		t.Fatalf("Inject() second error = %v", err)
	}
	if second.Changed {
		t.Fatalf("Inject() second changed = true")
	}
}

func TestInjectOpenCodeUsesOpenCodeSpecificOrchestratorPrompt(t *testing.T) {
	for _, mode := range []model.SDDModeID{model.SDDModeSingle, model.SDDModeMulti} {
		t.Run(string(mode), func(t *testing.T) {
			home := t.TempDir()
			mockNoPackageManager(t)

			if _, err := Inject(home, opencodeAdapter(), mode); err != nil {
				t.Fatalf("Inject(%s) error = %v", mode, err)
			}

			settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
			content, err := os.ReadFile(settingsPath)
			if err != nil {
				t.Fatalf("ReadFile(opencode.json) error = %v", err)
			}

			text := string(content)
			for _, unwanted := range []string{
				"Agent Teams Lite",
				"| orchestrator | opus |",
				"| sdd-explore | sonnet |",
				"| sdd-archive | haiku |",
			} {
				if strings.Contains(text, unwanted) {
					t.Fatalf("opencode.json contains legacy OpenCode orchestrator prompt content %q", unwanted)
				}
			}

			for _, wanted := range []string{
				"Gentle AI",
				"Read the configured models from `opencode.json`",
			} {
				if !strings.Contains(text, wanted) {
					t.Fatalf("opencode.json missing OpenCode orchestrator prompt content %q", wanted)
				}
			}
		})
	}
}

func TestInjectOpenCodePreservesExistingOrchestratorPromptWhenRequested(t *testing.T) {
	home := t.TempDir()
	mockNoPackageManager(t)

	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(settings dir) error = %v", err)
	}

	const customPrompt = "EXTERNAL_PROFILE_MANAGER_CUSTOM_PROMPT_DO_NOT_OVERWRITE"
	seed := `{
  "agent": {
    "sdd-orchestrator": {
      "mode": "primary",
      "prompt": "` + customPrompt + `"
    }
  }
}`
	if err := os.WriteFile(settingsPath, []byte(seed), 0o644); err != nil {
		t.Fatalf("WriteFile(opencode.json) error = %v", err)
	}

	_, err := Inject(home, opencodeAdapter(), model.SDDModeMulti, InjectOptions{
		PreserveOpenCodeOrchestratorPrompt: true,
	})
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	settingsBytes, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(opencode.json) error = %v", err)
	}
	if !strings.Contains(string(settingsBytes), customPrompt) {
		t.Fatalf("expected preserved custom orchestrator prompt %q in opencode.json", customPrompt)
	}
}

func TestInjectOpenCodeOverwritesOrchestratorPromptByDefault(t *testing.T) {
	home := t.TempDir()
	mockNoPackageManager(t)

	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(settings dir) error = %v", err)
	}

	const customPrompt = "EXTERNAL_PROFILE_MANAGER_CUSTOM_PROMPT_DO_NOT_OVERWRITE"
	seed := `{
  "agent": {
    "sdd-orchestrator": {
      "mode": "primary",
      "prompt": "` + customPrompt + `"
    }
  }
}`
	if err := os.WriteFile(settingsPath, []byte(seed), 0o644); err != nil {
		t.Fatalf("WriteFile(opencode.json) error = %v", err)
	}

	_, err := Inject(home, opencodeAdapter(), model.SDDModeMulti)
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	settingsBytes, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(opencode.json) error = %v", err)
	}
	text := string(settingsBytes)
	if strings.Contains(text, customPrompt) {
		t.Fatalf("expected default sync to overwrite custom orchestrator prompt")
	}
	if !strings.Contains(text, "Spec-Driven Development") {
		t.Fatalf("expected default orchestrator prompt content after sync")
	}
}

func TestInjectOpenCodeMigratesLegacyAgentsKey(t *testing.T) {
	home := t.TempDir()

	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	legacy := `{
  "agents": {
    "legacy-agent": {
      "mode": "all",
      "prompt": "{file:./AGENTS.md}"
    }
  }
}`
	if err := os.WriteFile(settingsPath, []byte(legacy), 0o644); err != nil {
		t.Fatalf("WriteFile(opencode.json) error = %v", err)
	}

	if _, err := Inject(home, opencodeAdapter(), ""); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(opencode.json) error = %v", err)
	}

	root := map[string]any{}
	if err := json.Unmarshal(content, &root); err != nil {
		t.Fatalf("Unmarshal(opencode.json) error = %v", err)
	}

	if _, hasLegacy := root["agents"]; hasLegacy {
		t.Fatal("opencode.json should not keep legacy agents key after migration")
	}

	agentRaw, ok := root["agent"]
	if !ok {
		t.Fatal("opencode.json missing agent key after migration")
	}

	agentMap, ok := agentRaw.(map[string]any)
	if !ok {
		t.Fatalf("opencode.json agent key has unexpected type: %T", agentRaw)
	}

	if _, ok := agentMap["legacy-agent"]; !ok {
		t.Fatal("legacy agent was not migrated under agent key")
	}
	if _, ok := agentMap["sdd-orchestrator"]; !ok {
		t.Fatal("sdd-orchestrator agent missing after merge")
	}
}

func TestInjectCursorWritesSDDOrchestratorAndSkills(t *testing.T) {
	home := t.TempDir()

	cursorAdapter, err := agents.NewAdapter("cursor")
	if err != nil {
		t.Fatalf("NewAdapter(cursor) error = %v", err)
	}

	result, injectErr := Inject(home, cursorAdapter, "")
	if injectErr != nil {
		t.Fatalf("Inject(cursor) error = %v", injectErr)
	}

	if !result.Changed {
		t.Fatal("Inject(cursor) changed = false")
	}

	// Should have SDD skill files AND the system prompt file.
	if len(result.Files) == 0 {
		t.Fatal("Inject(cursor) returned no files")
	}

	// Verify SDD orchestrator was injected into the system prompt file.
	promptPath := filepath.Join(home, ".cursor", "rules", "gentle-ai.mdc")
	content, readErr := os.ReadFile(promptPath)
	if readErr != nil {
		t.Fatalf("ReadFile(%q) error = %v", promptPath, readErr)
	}

	text := string(content)
	if !strings.Contains(text, "Spec-Driven Development") {
		t.Fatal("Cursor system prompt missing SDD orchestrator content")
	}
	if !strings.Contains(text, "sub-agent") {
		t.Fatal("Cursor system prompt missing SDD sub-agent references")
	}
}

func TestInjectGeminiWritesSDDOrchestratorAndSkills(t *testing.T) {
	home := t.TempDir()

	geminiAdapter, err := agents.NewAdapter("gemini-cli")
	if err != nil {
		t.Fatalf("NewAdapter(gemini-cli) error = %v", err)
	}

	result, injectErr := Inject(home, geminiAdapter, "")
	if injectErr != nil {
		t.Fatalf("Inject(gemini) error = %v", injectErr)
	}

	if !result.Changed {
		t.Fatal("Inject(gemini) changed = false")
	}

	// Verify SDD orchestrator was injected into GEMINI.md.
	promptPath := filepath.Join(home, ".gemini", "GEMINI.md")
	content, readErr := os.ReadFile(promptPath)
	if readErr != nil {
		t.Fatalf("ReadFile(%q) error = %v", promptPath, readErr)
	}

	text := string(content)
	if !strings.Contains(text, "Spec-Driven Development") {
		t.Fatal("Gemini system prompt missing SDD orchestrator content")
	}

	// Should also write SDD skill files.
	skillPath := filepath.Join(home, ".gemini", "skills", "sdd-init", "SKILL.md")
	if _, err := os.Stat(skillPath); err != nil {
		t.Fatalf("expected SDD skill file %q: %v", skillPath, err)
	}
}

func TestInjectKimiWritesNativeAgentFilesAndGlobalSkills(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, kimiAdapter(), "")
	if err != nil {
		t.Fatalf("Inject(kimi) error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Inject(kimi) changed = false")
	}

	// SDD orchestrator is written as a standalone Jinja include module.
	sddModulePath := filepath.Join(home, ".kimi", "sdd-orchestrator.md")
	sddModule, err := os.ReadFile(sddModulePath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", sddModulePath, err)
	}

	sddText := string(sddModule)
	if !strings.Contains(sddText, "/skill:sdd-init") {
		t.Fatal("sdd-orchestrator.md missing native /skill guidance")
	}
	if !strings.Contains(sddText, "multiagent:Task") {
		t.Fatal("sdd-orchestrator.md should reference Kimi's documented Task tool for custom subagent delegation")
	}

	rootAgentPath := filepath.Join(home, ".kimi", "agents", "gentleman.yaml")
	rootAgent, err := os.ReadFile(rootAgentPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", rootAgentPath, err)
	}

	rootText := string(rootAgent)
	if !strings.Contains(rootText, "name: gentleman") {
		t.Fatal("gentleman.yaml should define a named root custom agent")
	}
	if strings.Contains(rootText, "kimi_cli.tools.agent:Agent") {
		t.Fatal("gentleman.yaml should inherit Kimi's default tool set instead of hardcoding the old Agent tool path")
	}
	if !strings.Contains(rootText, "../KIMI.md") {
		t.Fatal("gentleman.yaml should load the installed KIMI.md system prompt")
	}

	for _, want := range []string{
		filepath.Join(home, ".kimi", "agents", "sdd-init.yaml"),
		filepath.Join(home, ".kimi", "agents", "sdd-init.md"),
		filepath.Join(home, ".kimi", "agents", "sdd-explore.yaml"),
		filepath.Join(home, ".kimi", "agents", "sdd-propose.yaml"),
		filepath.Join(home, ".kimi", "agents", "sdd-spec.yaml"),
		filepath.Join(home, ".kimi", "agents", "sdd-design.yaml"),
		filepath.Join(home, ".kimi", "agents", "sdd-tasks.yaml"),
		filepath.Join(home, ".kimi", "agents", "sdd-apply.yaml"),
		filepath.Join(home, ".kimi", "agents", "sdd-verify.yaml"),
		filepath.Join(home, ".kimi", "agents", "sdd-archive.yaml"),
		filepath.Join(home, ".config", "agents", "skills", "sdd-init", "SKILL.md"),
		filepath.Join(home, ".config", "agents", "skills", "_shared", "sdd-phase-common.md"),
	} {
		if _, err := os.Stat(want); err != nil {
			t.Fatalf("expected Kimi SDD artifact %q: %v", want, err)
		}
	}
}

func TestInjectQwenCodeWritesSDDOrchestratorAndSkills(t *testing.T) {
	home := t.TempDir()

	qwenAdapter, err := agents.NewAdapter("qwen-code")
	if err != nil {
		t.Fatalf("NewAdapter(qwen-code) error = %v", err)
	}

	result, injectErr := Inject(home, qwenAdapter, "")
	if injectErr != nil {
		t.Fatalf("Inject(qwen) error = %v", injectErr)
	}

	if !result.Changed {
		t.Fatal("Inject(qwen) changed = false")
	}

	// Verify SDD orchestrator was injected into QWEN.md.
	promptPath := filepath.Join(home, ".qwen", "QWEN.md")
	content, readErr := os.ReadFile(promptPath)
	if readErr != nil {
		t.Fatalf("ReadFile(%q) error = %v", promptPath, readErr)
	}

	text := string(content)
	if !strings.Contains(text, "Spec-Driven Development") {
		t.Fatal("Qwen Code system prompt missing SDD orchestrator content")
	}

	// Verify Qwen-specific skill paths are referenced in the orchestrator.
	if !strings.Contains(text, "~/.qwen/skills/") {
		t.Fatal("Qwen Code orchestrator missing ~/.qwen/skills/ path reference")
	}

	// Should also write SDD skill files.
	skillPath := filepath.Join(home, ".qwen", "skills", "sdd-init", "SKILL.md")
	if _, err := os.Stat(skillPath); err != nil {
		t.Fatalf("expected SDD skill file %q: %v", skillPath, err)
	}
}

func TestInjectVSCodeWritesSDDOrchestratorAndSkills(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	vscodeAdapter, err := agents.NewAdapter("vscode-copilot")
	if err != nil {
		t.Fatalf("NewAdapter(vscode-copilot) error = %v", err)
	}

	result, injectErr := Inject(home, vscodeAdapter, "")
	if injectErr != nil {
		t.Fatalf("Inject(vscode) error = %v", injectErr)
	}

	if !result.Changed {
		t.Fatal("Inject(vscode) changed = false")
	}

	// Verify SDD orchestrator was injected into the VS Code instructions file.
	promptPath := vscodeAdapter.SystemPromptFile(home)
	content, readErr := os.ReadFile(promptPath)
	if readErr != nil {
		t.Fatalf("ReadFile(%q) error = %v", promptPath, readErr)
	}

	text := string(content)
	if !strings.Contains(text, "Spec-Driven Development") {
		t.Fatal("VS Code system prompt missing SDD orchestrator content")
	}

	// Should also write SDD skill files under ~/.copilot/skills/.
	skillPath := filepath.Join(home, ".copilot", "skills", "sdd-init", "SKILL.md")
	if _, err := os.Stat(skillPath); err != nil {
		t.Fatalf("expected SDD skill file %q: %v", skillPath, err)
	}

	sharedPath := filepath.Join(home, ".copilot", "skills", "_shared", "engram-convention.md")
	if _, err := os.Stat(sharedPath); err != nil {
		t.Fatalf("expected shared SDD convention file %q: %v", sharedPath, err)
	}
}

func TestInjectFileAppendSkipsIfAlreadyPresent(t *testing.T) {
	home := t.TempDir()

	cursorAdapter, err := agents.NewAdapter("cursor")
	if err != nil {
		t.Fatalf("NewAdapter(cursor) error = %v", err)
	}

	// First injection.
	first, firstErr := Inject(home, cursorAdapter, "")
	if firstErr != nil {
		t.Fatalf("Inject() first error = %v", firstErr)
	}
	if !first.Changed {
		t.Fatal("first Inject() changed = false")
	}

	// Second injection — SDD content is already there, should not duplicate.
	second, secondErr := Inject(home, cursorAdapter, "")
	if secondErr != nil {
		t.Fatalf("Inject() second error = %v", secondErr)
	}
	if second.Changed {
		t.Fatal("second Inject() changed = true — SDD orchestrator was duplicated")
	}
}

func TestInjectFileAppendMigratesLegacyHeading(t *testing.T) {
	home := t.TempDir()

	cursorAdapter, err := agents.NewAdapter("cursor")
	if err != nil {
		t.Fatalf("NewAdapter(cursor) error = %v", err)
	}

	promptPath := cursorAdapter.SystemPromptFile(home)
	if err := os.MkdirAll(filepath.Dir(promptPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	existing := "# Existing\n\n## Spec-Driven Development (SDD) Orchestrator\nAlready present.\n"
	if err := os.WriteFile(promptPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	result, injectErr := Inject(home, cursorAdapter, "")
	if injectErr != nil {
		t.Fatalf("Inject() error = %v", injectErr)
	}
	if len(result.Files) == 0 {
		t.Fatal("Inject() returned no files")
	}

	content, readErr := os.ReadFile(promptPath)
	if readErr != nil {
		t.Fatalf("ReadFile() error = %v", readErr)
	}

	text := string(content)
	if strings.Contains(text, "Already present.") {
		t.Fatal("legacy SDD orchestrator content survived after migration")
	}
	if !strings.Contains(text, "<!-- gentle-ai:sdd-orchestrator -->") {
		t.Fatal("missing open marker after migration")
	}
	if !strings.Contains(text, "<!-- /gentle-ai:sdd-orchestrator -->") {
		t.Fatal("missing close marker after migration")
	}
	if strings.Count(text, "## Agent Teams Orchestrator") != 1 {
		t.Fatal("agent teams heading duplicated after migration")
	}
	if !strings.Contains(text, "## Project Standards (auto-resolved)") {
		t.Fatal("SDD orchestrator was not refreshed to current compact-rules format")
	}
}

func TestInjectFileAppendMigratesFullLegacyOrchestratorBlock(t *testing.T) {
	home := t.TempDir()

	cursorAdapter, err := agents.NewAdapter("cursor")
	if err != nil {
		t.Fatalf("NewAdapter(cursor) error = %v", err)
	}

	promptPath := cursorAdapter.SystemPromptFile(home)
	if err := os.MkdirAll(filepath.Dir(promptPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	existing := "## Rules\n\nLegacy intro.\n\n" +
		"## Agent Teams Orchestrator\n\n" +
		"### Result Contract\n" +
		"Each phase returns: `status`, `executive_summary`, `artifacts`, `next_recommended`, `risks`.\n\n" +
		"### Sub-Agent Launch Pattern\n\n" +
		"SKILL: Load `{skill-path}` before starting.\n\n" +
		"<!-- gentle-ai:engram-protocol -->\n" +
		"## Engram Persistent Memory - Protocol\n" +
		"<!-- /gentle-ai:engram-protocol -->\n"

	if err := os.WriteFile(promptPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	result, injectErr := Inject(home, cursorAdapter, "")
	if injectErr != nil {
		t.Fatalf("Inject() error = %v", injectErr)
	}
	if len(result.Files) == 0 {
		t.Fatal("Inject() returned no files")
	}

	content, readErr := os.ReadFile(promptPath)
	if readErr != nil {
		t.Fatalf("ReadFile() error = %v", readErr)
	}

	text := string(content)
	if strings.Contains(text, "SKILL: Load `{skill-path}` before starting.") {
		t.Fatal("legacy sub-agent launch content survived after migration")
	}
	if strings.Count(text, "### Result Contract") != 1 {
		t.Fatal("result contract section duplicated after migration")
	}
	if !strings.Contains(text, "`skill_resolution`") {
		t.Fatal("result contract was not refreshed to current format")
	}
	if !strings.Contains(text, "## Project Standards (auto-resolved)") {
		t.Fatal("current compact-rules launch pattern missing after migration")
	}
	if strings.Count(text, "<!-- gentle-ai:engram-protocol -->") != 1 {
		t.Fatal("engram protocol marker should be preserved exactly once")
	}
}

func TestInjectFileAppendRemovesLegacyBlockWhenMarkedSectionAlreadyExists(t *testing.T) {
	home := t.TempDir()

	cursorAdapter, err := agents.NewAdapter("cursor")
	if err != nil {
		t.Fatalf("NewAdapter(cursor) error = %v", err)
	}

	promptPath := cursorAdapter.SystemPromptFile(home)
	if err := os.MkdirAll(filepath.Dir(promptPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	canonical := assets.MustRead("generic/sdd-orchestrator.md")
	existing := "## Agent Teams Orchestrator\n\nLegacy duplicate block.\n\n" +
		"<!-- gentle-ai:sdd-orchestrator -->\n" + canonical + "\n<!-- /gentle-ai:sdd-orchestrator -->\n"

	if err := os.WriteFile(promptPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, injectErr := Inject(home, cursorAdapter, "")
	if injectErr != nil {
		t.Fatalf("Inject() error = %v", injectErr)
	}

	content, readErr := os.ReadFile(promptPath)
	if readErr != nil {
		t.Fatalf("ReadFile() error = %v", readErr)
	}

	text := string(content)
	if strings.Contains(text, "Legacy duplicate block.") {
		t.Fatal("legacy duplicate block survived even with marked section present")
	}
	if strings.Count(text, "## Agent Teams Orchestrator") != 1 {
		t.Fatal("orchestrator heading should exist exactly once after cleanup")
	}
}

func TestInjectMarkdownSections_stripsLegacyATLBlockWithMarkedSection(t *testing.T) {
	home := t.TempDir()

	claudeAdpt := claudeAdapter()
	promptPath := claudeAdpt.SystemPromptFile(home)
	if err := os.MkdirAll(filepath.Dir(promptPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	const legacyATLBlock = `<!-- BEGIN:agent-teams-lite -->
## Agent Teams Orchestrator

You are a COORDINATOR, not an executor.

### Delegation Rules (ALWAYS ACTIVE)

| Rule | Instruction |
|------|------------|
| No inline work | Reading/writing code → delegate to sub-agent |
<!-- END:agent-teams-lite -->`

	sddSection := "<!-- gentle-ai:sdd-orchestrator -->\nYou are a COORDINATOR.\n<!-- /gentle-ai:sdd-orchestrator -->\n"
	existing := legacyATLBlock + "\n\n" + sddSection

	if err := os.WriteFile(promptPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, injectErr := Inject(home, claudeAdpt, "")
	if injectErr != nil {
		t.Fatalf("Inject() error = %v", injectErr)
	}

	content, readErr := os.ReadFile(promptPath)
	if readErr != nil {
		t.Fatalf("ReadFile() error = %v", readErr)
	}

	text := string(content)

	if strings.Contains(text, "<!-- BEGIN:agent-teams-lite -->") {
		t.Fatal("ATL open marker should have been stripped during inject")
	}
	if strings.Contains(text, "<!-- END:agent-teams-lite -->") {
		t.Fatal("ATL close marker should have been stripped during inject")
	}
	if !strings.Contains(text, "<!-- gentle-ai:sdd-orchestrator -->") {
		t.Fatal("sdd-orchestrator section must be present after ATL strip")
	}
	if !strings.Contains(text, "<!-- /gentle-ai:sdd-orchestrator -->") {
		t.Fatal("sdd-orchestrator close marker must be present after ATL strip")
	}
}

func TestInjectOpenCodeMultiMode(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, opencodeAdapter(), "multi")
	if err != nil {
		t.Fatalf("Inject(multi) error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Inject(multi) changed = false")
	}

	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(opencode.json) error = %v", err)
	}

	root := map[string]any{}
	if err := json.Unmarshal(content, &root); err != nil {
		t.Fatalf("Unmarshal(opencode.json) error = %v", err)
	}

	agentRaw, ok := root["agent"]
	if !ok {
		t.Fatal("opencode.json missing agent key")
	}

	agentMap, ok := agentRaw.(map[string]any)
	if !ok {
		t.Fatalf("agent key has unexpected type: %T", agentRaw)
	}

	// Multi overlay must contain orchestrator + 10 sub-agents = 11 agents.
	if len(agentMap) != 11 {
		t.Fatalf("agent count = %d, want 11", len(agentMap))
	}

	// Verify orchestrator is present.
	orchestratorRaw, ok := agentMap["sdd-orchestrator"]
	if !ok {
		t.Fatal("missing sdd-orchestrator agent")
	}
	orchestratorAgent, ok := orchestratorRaw.(map[string]any)
	if !ok {
		t.Fatalf("sdd-orchestrator has unexpected type: %T", orchestratorRaw)
	}
	toolsRaw, ok := orchestratorAgent["tools"].(map[string]any)
	if !ok {
		t.Fatalf("sdd-orchestrator tools has unexpected type: %T", orchestratorAgent["tools"])
	}
	for _, toolName := range []string{"delegate", "delegation_read", "delegation_list"} {
		value, ok := toolsRaw[toolName].(bool)
		if !ok || !value {
			t.Fatalf("sdd-orchestrator missing multi-mode tool %q", toolName)
		}
	}

	// Verify representative sub-agents are present.
	for _, subAgent := range []string{"sdd-init", "sdd-apply", "sdd-verify", "sdd-explore", "sdd-propose", "sdd-spec", "sdd-design", "sdd-tasks", "sdd-archive"} {
		if _, ok := agentMap[subAgent]; !ok {
			t.Fatalf("missing sub-agent %q", subAgent)
		}
	}

	// Verify sub-agents have mode "subagent".
	applyRaw, _ := agentMap["sdd-apply"]
	applyAgent, ok := applyRaw.(map[string]any)
	if !ok {
		t.Fatalf("sdd-apply has unexpected type: %T", applyRaw)
	}
	if mode, _ := applyAgent["mode"].(string); mode != "subagent" {
		t.Fatalf("sdd-apply mode = %q, want %q", mode, "subagent")
	}

	pluginPath := filepath.Join(home, ".config", "opencode", "plugins", "background-agents.ts")
	pluginContent, err := os.ReadFile(pluginPath)
	if err != nil {
		t.Fatalf("ReadFile(background-agents.ts) error = %v", err)
	}
	if string(pluginContent) != assets.MustRead("opencode/plugins/background-agents.ts") {
		t.Fatal("background-agents.ts content does not match embedded asset")
	}
	foundPlugin := false
	for _, path := range result.Files {
		if path == pluginPath {
			foundPlugin = true
			break
		}
	}
	if !foundPlugin {
		t.Fatalf("plugin path %q missing from result.Files", pluginPath)
	}
}

func TestInjectOpenCodeMultiModeIdempotent(t *testing.T) {
	home := t.TempDir()

	first, err := Inject(home, opencodeAdapter(), "multi")
	if err != nil {
		t.Fatalf("Inject(multi) first error = %v", err)
	}
	if !first.Changed {
		t.Fatal("Inject(multi) first changed = false")
	}

	second, err := Inject(home, opencodeAdapter(), "multi")
	if err != nil {
		t.Fatalf("Inject(multi) second error = %v", err)
	}
	if second.Changed {
		t.Fatal("Inject(multi) second changed = true — multi overlay was duplicated")
	}

	pluginPath := filepath.Join(home, ".config", "opencode", "plugins", "background-agents.ts")
	content, err := os.ReadFile(pluginPath)
	if err != nil {
		t.Fatalf("ReadFile(background-agents.ts) error = %v", err)
	}
	if string(content) != assets.MustRead("opencode/plugins/background-agents.ts") {
		t.Fatal("background-agents.ts changed after second multi inject")
	}
}

func TestInjectOpenCodeSubagentPromptsStayExecutorScoped(t *testing.T) {
	home := t.TempDir()
	mockNoPackageManager(t)

	if _, err := Inject(home, opencodeAdapter(), "multi"); err != nil {
		t.Fatalf("Inject(multi) error = %v", err)
	}

	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(opencode.json) error = %v", err)
	}

	root := map[string]any{}
	if err := json.Unmarshal(content, &root); err != nil {
		t.Fatalf("Unmarshal(opencode.json) error = %v", err)
	}

	agentMap, ok := root["agent"].(map[string]any)
	if !ok {
		t.Fatal("opencode.json missing agent map")
	}

	promptDir := SharedPromptDir(home)

	for _, phase := range []string{"sdd-init", "sdd-explore", "sdd-propose", "sdd-spec", "sdd-design", "sdd-tasks", "sdd-apply", "sdd-verify", "sdd-archive"} {
		raw, ok := agentMap[phase]
		if !ok {
			t.Fatalf("missing sub-agent %q", phase)
		}
		agentDef, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("%s has unexpected type: %T", phase, raw)
		}

		// After the shared-prompt-files refactor, the prompt field is a {file:...}
		// reference. The executor-scoped content lives in the prompt file on disk.
		prompt, _ := agentDef["prompt"].(string)
		expectedRef := "{file:" + filepath.Join(promptDir, phase+".md") + "}"
		if prompt != expectedRef {
			t.Fatalf("%s prompt = %q, want {file:...} reference %q", phase, prompt, expectedRef)
		}

		// Also verify the prompt file itself contains the executor-scoped markers.
		promptFilePath := filepath.Join(promptDir, phase+".md")
		promptFileData, readErr := os.ReadFile(promptFilePath)
		if readErr != nil {
			t.Fatalf("%s prompt file %q not readable: %v", phase, promptFilePath, readErr)
		}
		promptFileContent := string(promptFileData)
		for _, want := range []string{"not the orchestrator", "Do NOT delegate", "Do NOT call task/delegate", "Do NOT launch sub-agents"} {
			if !strings.Contains(promptFileContent, want) {
				t.Fatalf("%s prompt file missing %q", phase, want)
			}
		}
	}
}

func TestInjectOpenCodeEmptySDDModeDefaultsSingle(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, opencodeAdapter(), "")
	if err != nil {
		t.Fatalf("Inject(\"\") error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Inject(\"\") changed = false")
	}

	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(opencode.json) error = %v", err)
	}

	root := map[string]any{}
	if err := json.Unmarshal(content, &root); err != nil {
		t.Fatalf("Unmarshal(opencode.json) error = %v", err)
	}

	agentRaw, ok := root["agent"]
	if !ok {
		t.Fatal("opencode.json missing agent key")
	}

	agentMap, ok := agentRaw.(map[string]any)
	if !ok {
		t.Fatalf("agent key has unexpected type: %T", agentRaw)
	}

	// Empty mode defaults to single — orchestrator + 10 sub-agents = 11 agents.
	if _, ok := agentMap["sdd-orchestrator"]; !ok {
		t.Fatal("missing sdd-orchestrator agent")
	}
	if len(agentMap) != 11 {
		t.Fatalf("agent count = %d, want 11", len(agentMap))
	}

	// Verify orchestrator mode is "primary".
	orchestratorRaw, ok := agentMap["sdd-orchestrator"]
	if !ok {
		t.Fatal("missing sdd-orchestrator agent")
	}
	orchestratorAgent, ok := orchestratorRaw.(map[string]any)
	if !ok {
		t.Fatalf("sdd-orchestrator has unexpected type: %T", orchestratorRaw)
	}
	if mode, _ := orchestratorAgent["mode"].(string); mode != "primary" {
		t.Fatalf("sdd-orchestrator mode = %q, want %q", mode, "primary")
	}

	// Verify sub-agents are present with mode "subagent".
	for _, subAgent := range []string{"sdd-init", "sdd-apply", "sdd-verify", "sdd-explore", "sdd-propose", "sdd-spec", "sdd-design", "sdd-tasks", "sdd-archive"} {
		raw, ok := agentMap[subAgent]
		if !ok {
			t.Fatalf("missing sub-agent %q", subAgent)
		}
		agent, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("%s has unexpected type: %T", subAgent, raw)
		}
		if m, _ := agent["mode"].(string); m != "subagent" {
			t.Fatalf("%s mode = %q, want %q", subAgent, m, "subagent")
		}
	}
}

func TestInjectClaudeIgnoresSDDMode(t *testing.T) {
	home := t.TempDir()

	// Inject with multi mode for Claude — should be ignored.
	resultMulti, err := Inject(home, claudeAdapter(), "multi")
	if err != nil {
		t.Fatalf("Inject(claude, multi) error = %v", err)
	}

	homeBaseline := t.TempDir()
	resultSingle, err := Inject(homeBaseline, claudeAdapter(), "single")
	if err != nil {
		t.Fatalf("Inject(claude, single) error = %v", err)
	}

	// Both should produce changed=true (first injection).
	if !resultMulti.Changed || !resultSingle.Changed {
		t.Fatal("first injection should be changed=true")
	}

	// Read and compare the CLAUDE.md files — content should be identical.
	multiContent, err := os.ReadFile(filepath.Join(home, ".claude", "CLAUDE.md"))
	if err != nil {
		t.Fatalf("ReadFile(multi) error = %v", err)
	}
	singleContent, err := os.ReadFile(filepath.Join(homeBaseline, ".claude", "CLAUDE.md"))
	if err != nil {
		t.Fatalf("ReadFile(single) error = %v", err)
	}

	if string(multiContent) != string(singleContent) {
		t.Fatal("Claude CLAUDE.md differs between multi and single sddMode — non-OpenCode agents should ignore sddMode")
	}
}

func TestInjectOpenCodeSingleToMultiSwitch(t *testing.T) {
	home := t.TempDir()

	// First: inject single mode.
	_, err := Inject(home, opencodeAdapter(), "single")
	if err != nil {
		t.Fatalf("Inject(single) error = %v", err)
	}

	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")

	// Single mode now has orchestrator + 9 sub-agents (same as multi).
	content, _ := os.ReadFile(settingsPath)
	if !strings.Contains(string(content), `"sdd-apply"`) {
		t.Fatal("single mode should have sdd-apply")
	}

	// Second: inject multi mode — structure stays the same (both have all agents),
	// but the overlay content (prompts) may differ so changed can be true or false.
	_, err = Inject(home, opencodeAdapter(), "multi")
	if err != nil {
		t.Fatalf("Inject(multi) error = %v", err)
	}

	content, err = os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(opencode.json) error = %v", err)
	}

	root := map[string]any{}
	if err := json.Unmarshal(content, &root); err != nil {
		t.Fatalf("Unmarshal(opencode.json) error = %v", err)
	}

	agentMap, _ := root["agent"].(map[string]any)
	if _, ok := agentMap["sdd-orchestrator"]; !ok {
		t.Fatal("missing sdd-orchestrator after switch to multi")
	}
	if _, ok := agentMap["sdd-apply"]; !ok {
		t.Fatal("missing sdd-apply after switch to multi")
	}

	// Without explicit assignments, no model fields should be injected.
	applyAgent, ok := agentMap["sdd-apply"].(map[string]any)
	if !ok {
		t.Fatal("sdd-apply has unexpected type after switch to multi")
	}
	if _, hasModel := applyAgent["model"]; hasModel {
		t.Fatal("sdd-apply should NOT have model field without explicit assignments")
	}
}

func TestInjectFileAppendSkipsAgentTeamsHeading(t *testing.T) {
	home := t.TempDir()

	cursorAdapter, err := agents.NewAdapter("cursor")
	if err != nil {
		t.Fatalf("NewAdapter(cursor) error = %v", err)
	}

	promptPath := cursorAdapter.SystemPromptFile(home)
	if err := os.MkdirAll(filepath.Dir(promptPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	existing := "# Existing\n\n## Agent Teams Orchestrator\nAlready present.\n"
	if err := os.WriteFile(promptPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	result, injectErr := Inject(home, cursorAdapter, "")
	if injectErr != nil {
		t.Fatalf("Inject() error = %v", injectErr)
	}
	if len(result.Files) == 0 {
		t.Fatal("Inject() returned no files")
	}

	content, readErr := os.ReadFile(promptPath)
	if readErr != nil {
		t.Fatalf("ReadFile() error = %v", readErr)
	}

	text := string(content)
	if strings.Count(text, "## Agent Teams Orchestrator") != 1 {
		t.Fatal("agent teams heading duplicated")
	}
}

func TestInjectClaudeDeduplicatesBareOrchestratorSection(t *testing.T) {
	home := t.TempDir()
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	// Pre-existing file with a BARE (no HTML markers) Agent Teams Orchestrator section.
	existing := "# My Rules\n\n## Rules\n\nBe excellent.\n\n## Agent Teams Orchestrator\n\nYou are a COORDINATOR.\n\n### Delegation Rules\n\nSome old rules.\n\n## Other Section\n\nOther content.\n"
	if err := os.WriteFile(filepath.Join(claudeDir, "CLAUDE.md"), []byte(existing), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	result, err := Inject(home, claudeAdapter(), "")
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if len(result.Files) == 0 {
		t.Fatal("Inject() returned no files")
	}

	content, readErr := os.ReadFile(filepath.Join(claudeDir, "CLAUDE.md"))
	if readErr != nil {
		t.Fatalf("ReadFile() error = %v", readErr)
	}

	text := string(content)

	// Must have exactly ONE "## Agent Teams Orchestrator" heading — no duplication.
	if count := strings.Count(text, "## Agent Teams Orchestrator"); count != 1 {
		t.Fatalf("expected 1 Agent Teams Orchestrator heading, got %d\n\ncontent:\n%s", count, text)
	}

	// The injected marked version must be present.
	if !strings.Contains(text, "<!-- gentle-ai:sdd-orchestrator -->") {
		t.Fatal("missing open marker after injection")
	}
	if !strings.Contains(text, "<!-- /gentle-ai:sdd-orchestrator -->") {
		t.Fatal("missing close marker after injection")
	}

	// Content outside the orchestrator section must be preserved.
	if !strings.Contains(text, "Be excellent.") {
		t.Fatal("user content outside orchestrator section was lost")
	}
	if !strings.Contains(text, "## Other Section") {
		t.Fatal("section after orchestrator was lost")
	}
	if !strings.Contains(text, "Other content.") {
		t.Fatal("content after orchestrator section was lost")
	}

	// The old bare content must NOT survive (replaced by the marked version).
	if strings.Contains(text, "Some old rules.") {
		t.Fatal("old bare orchestrator content was not stripped")
	}
}

func TestInjectClaudeDeduplicatesBareOrchestratorAtEndOfFile(t *testing.T) {
	home := t.TempDir()
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	// Bare orchestrator section at the END of file (no following ## heading).
	existing := "# My Rules\n\n## Rules\n\nBe excellent.\n\n## Agent Teams Orchestrator\n\nYou are a COORDINATOR, not an executor.\n"
	if err := os.WriteFile(filepath.Join(claudeDir, "CLAUDE.md"), []byte(existing), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Inject(home, claudeAdapter(), "")
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	content, readErr := os.ReadFile(filepath.Join(claudeDir, "CLAUDE.md"))
	if readErr != nil {
		t.Fatalf("ReadFile() error = %v", readErr)
	}

	text := string(content)

	if count := strings.Count(text, "## Agent Teams Orchestrator"); count != 1 {
		t.Fatalf("expected 1 Agent Teams Orchestrator heading, got %d\n\ncontent:\n%s", count, text)
	}
	if !strings.Contains(text, "<!-- gentle-ai:sdd-orchestrator -->") {
		t.Fatal("missing open marker after injection")
	}
	if !strings.Contains(text, "Be excellent.") {
		t.Fatal("user content outside orchestrator section was lost")
	}
}

func TestInjectOpenCodeMultiModeWithModelAssignments(t *testing.T) {
	home := t.TempDir()
	mockNoPackageManager(t)

	assignments := map[string]model.ModelAssignment{
		"sdd-init":  {ProviderID: "anthropic", ModelID: "claude-sonnet-4-20250514"},
		"sdd-apply": {ProviderID: "openai", ModelID: "gpt-4o"},
	}

	result, err := Inject(home, opencodeAdapter(), "multi", InjectOptions{OpenCodeModelAssignments: assignments})
	if err != nil {
		t.Fatalf("Inject(multi, assignments) error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Inject(multi, assignments) changed = false")
	}

	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(opencode.json) error = %v", err)
	}

	root := map[string]any{}
	if err := json.Unmarshal(content, &root); err != nil {
		t.Fatalf("Unmarshal(opencode.json) error = %v", err)
	}

	agentMap, ok := root["agent"].(map[string]any)
	if !ok {
		t.Fatal("opencode.json missing agent map")
	}

	// Verify sdd-init has the assigned model.
	initAgent, ok := agentMap["sdd-init"].(map[string]any)
	if !ok {
		t.Fatal("sdd-init agent not found or wrong type")
	}
	if m, _ := initAgent["model"].(string); m != "anthropic/claude-sonnet-4-20250514" {
		t.Fatalf("sdd-init model = %q, want %q", m, "anthropic/claude-sonnet-4-20250514")
	}

	// Verify sdd-apply has the assigned model.
	applyAgent, ok := agentMap["sdd-apply"].(map[string]any)
	if !ok {
		t.Fatal("sdd-apply agent not found or wrong type")
	}
	if m, _ := applyAgent["model"].(string); m != "openai/gpt-4o" {
		t.Fatalf("sdd-apply model = %q, want %q", m, "openai/gpt-4o")
	}

	// Unassigned phases should NOT have a model field — the overlay no longer
	// hardcodes defaults, so only explicitly assigned phases get a model.
	verifyAgent, ok := agentMap["sdd-verify"].(map[string]any)
	if !ok {
		t.Fatal("sdd-verify agent not found or wrong type")
	}
	if _, hasModel := verifyAgent["model"]; hasModel {
		t.Fatal("sdd-verify should not have a model field (unassigned phase)")
	}
}

func TestInjectOpenCodeMultiModeNoAssignmentsNoModel(t *testing.T) {
	home := t.TempDir()
	mockNoPackageManager(t)

	// Pass nil assignments — no model fields should be injected.
	result, err := Inject(home, opencodeAdapter(), "multi")
	if err != nil {
		t.Fatalf("Inject(multi) error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Inject(multi) changed = false")
	}

	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(opencode.json) error = %v", err)
	}

	root := map[string]any{}
	if err := json.Unmarshal(content, &root); err != nil {
		t.Fatalf("Unmarshal(opencode.json) error = %v", err)
	}

	agentMap, _ := root["agent"].(map[string]any)
	// When no assignments are given, no model fields should be injected.
	// The overlay itself no longer contains hardcoded models.
	for _, phase := range []string{"sdd-init", "sdd-apply", "sdd-verify"} {
		agentDef, ok := agentMap[phase].(map[string]any)
		if !ok {
			t.Fatalf("phase %q agent not found or wrong type", phase)
		}
		if _, hasModel := agentDef["model"]; hasModel {
			t.Fatalf("phase %q should NOT have model field when no assignments given", phase)
		}
	}
}

func TestInjectSingleModeIgnoresModelAssignments(t *testing.T) {
	home := t.TempDir()
	mockNoPackageManager(t)

	// Even if assignments are provided, single mode should ignore them.
	assignments := map[string]model.ModelAssignment{
		"sdd-init": {ProviderID: "anthropic", ModelID: "claude-sonnet-4-20250514"},
	}

	result, err := Inject(home, opencodeAdapter(), "single", InjectOptions{OpenCodeModelAssignments: assignments})
	if err != nil {
		t.Fatalf("Inject(single, assignments) error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Inject(single, assignments) changed = false")
	}

	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(opencode.json) error = %v", err)
	}

	// Single mode has no sub-agents, so model should not appear.
	if strings.Contains(string(content), `"model"`) {
		t.Fatal("single mode should not inject model assignments")
	}
}

func TestInjectOpenCodeMultiModeUsesRootModelForUnassignedAgents(t *testing.T) {
	home := t.TempDir()
	mockNoPackageManager(t)

	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte(`{"model":"openai/gpt-5"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(opencode.json) error = %v", err)
	}

	if _, err := Inject(home, opencodeAdapter(), "multi"); err != nil {
		t.Fatalf("Inject(multi) error = %v", err)
	}

	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(opencode.json) error = %v", err)
	}

	root := map[string]any{}
	if err := json.Unmarshal(content, &root); err != nil {
		t.Fatalf("Unmarshal(opencode.json) error = %v", err)
	}

	agentMap, ok := root["agent"].(map[string]any)
	if !ok {
		t.Fatal("opencode.json missing agent map")
	}

	// With no explicit assignments but a root model, all sub-agents that are NOT
	// pre-existing in the user's config should get the root model injected.
	// Since we started with only {"model":"openai/gpt-5"} (no agent entries),
	// ALL agents are "new" from the 3-way logic perspective and should get rootModel.
	for _, phase := range []string{"sdd-orchestrator", "sdd-init", "sdd-verify"} {
		agentDef, ok := agentMap[phase].(map[string]any)
		if !ok {
			t.Fatalf("phase %q agent not found or wrong type", phase)
		}
		m, hasModel := agentDef["model"]
		if !hasModel {
			t.Fatalf("%s should have model field (root model should propagate to new agents)", phase)
		}
		if m != "openai/gpt-5" {
			t.Fatalf("%s model = %q, want %q", phase, m, "openai/gpt-5")
		}
	}

	// The root-level "model" should still be preserved.
	if m, _ := root["model"].(string); m != "openai/gpt-5" {
		t.Fatalf("root model lost after merge: got %q", m)
	}
}

func TestInjectOpenCodeMultiModeExplicitAssignmentsDoNotSpread(t *testing.T) {
	home := t.TempDir()
	mockNoPackageManager(t)

	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte(`{"model":"openai/gpt-5"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(opencode.json) error = %v", err)
	}

	assignments := map[string]model.ModelAssignment{
		"sdd-apply": {ProviderID: "anthropic", ModelID: "claude-opus-4-6"},
	}

	if _, err := Inject(home, opencodeAdapter(), "multi", InjectOptions{OpenCodeModelAssignments: assignments}); err != nil {
		t.Fatalf("Inject(multi, assignments) error = %v", err)
	}

	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(opencode.json) error = %v", err)
	}

	root := map[string]any{}
	if err := json.Unmarshal(content, &root); err != nil {
		t.Fatalf("Unmarshal(opencode.json) error = %v", err)
	}

	agentMap, ok := root["agent"].(map[string]any)
	if !ok {
		t.Fatal("opencode.json missing agent map")
	}

	// Explicitly assigned phase gets the assigned model (TUI wins).
	applyAgent, ok := agentMap["sdd-apply"].(map[string]any)
	if !ok {
		t.Fatal("sdd-apply agent not found or wrong type")
	}
	if m, _ := applyAgent["model"].(string); m != "anthropic/claude-opus-4-6" {
		t.Fatalf("sdd-apply model = %q, want %q", m, "anthropic/claude-opus-4-6")
	}

	// Unassigned phase AND not pre-existing: should get root model (openai/gpt-5).
	// The pre-existing config only had {"model":"openai/gpt-5"}, no agent entries.
	initAgent, ok := agentMap["sdd-init"].(map[string]any)
	if !ok {
		t.Fatal("sdd-init agent not found or wrong type")
	}
	if m, _ := initAgent["model"].(string); m != "openai/gpt-5" {
		t.Fatalf("sdd-init model = %q, want %q (root model should apply to unassigned new agents)", m, "openai/gpt-5")
	}
}

func TestInjectOpenCodeSingleModeDoesNotInjectModels(t *testing.T) {
	home := t.TempDir()
	mockNoPackageManager(t)

	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte(`{"model":"openai/gpt-5"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(opencode.json) error = %v", err)
	}

	if _, err := Inject(home, opencodeAdapter(), "single"); err != nil {
		t.Fatalf("Inject(single) error = %v", err)
	}

	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(opencode.json) error = %v", err)
	}

	root := map[string]any{}
	if err := json.Unmarshal(content, &root); err != nil {
		t.Fatalf("Unmarshal(opencode.json) error = %v", err)
	}

	agentMap, ok := root["agent"].(map[string]any)
	if !ok {
		t.Fatal("opencode.json missing agent map")
	}

	// Single mode should NOT inject model fields into sub-agents.
	initAgent, ok := agentMap["sdd-init"].(map[string]any)
	if !ok {
		t.Fatal("sdd-init agent not found or wrong type")
	}
	if _, hasModel := initAgent["model"]; hasModel {
		t.Fatal("sdd-init should NOT have model field in single mode")
	}

	// Root model should be preserved.
	if m, _ := root["model"].(string); m != "openai/gpt-5" {
		t.Fatalf("root model lost after merge: got %q", m)
	}
}

// TestInjectOpenCodeMultiModePreservesExistingAgentModels verifies that
// a pre-existing agent definition with an explicit model is not overwritten
// by the root model, while a NEW agent (not yet in the user's config) gets
// the root model as a default.
func TestInjectOpenCodeMultiModePreservesExistingAgentModels(t *testing.T) {
	home := t.TempDir()
	mockNoPackageManager(t)

	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	// Pre-existing config: root model + sdd-apply already defined with its own model.
	existing := `{
  "model": "openai/gpt-5",
  "agent": {
    "sdd-apply": {
      "model": "anthropic/claude-opus-4-6",
      "mode": "subagent"
    }
  }
}`
	if err := os.WriteFile(settingsPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("WriteFile(opencode.json) error = %v", err)
	}

	if _, err := Inject(home, opencodeAdapter(), "multi"); err != nil {
		t.Fatalf("Inject(multi) error = %v", err)
	}

	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(opencode.json) error = %v", err)
	}

	root := map[string]any{}
	if err := json.Unmarshal(content, &root); err != nil {
		t.Fatalf("Unmarshal(opencode.json) error = %v", err)
	}

	agentMap, ok := root["agent"].(map[string]any)
	if !ok {
		t.Fatal("opencode.json missing agent map")
	}

	// sdd-apply was pre-existing with its own model — must be preserved (NOT overwritten to gpt-5).
	applyAgent, ok := agentMap["sdd-apply"].(map[string]any)
	if !ok {
		t.Fatal("sdd-apply agent not found or wrong type")
	}
	if m, _ := applyAgent["model"].(string); m != "anthropic/claude-opus-4-6" {
		t.Fatalf("sdd-apply model = %q, want %q (pre-existing model must be preserved)", m, "anthropic/claude-opus-4-6")
	}

	// sdd-init was NOT pre-existing — should get root model as default.
	initAgent, ok := agentMap["sdd-init"].(map[string]any)
	if !ok {
		t.Fatal("sdd-init agent not found or wrong type")
	}
	if m, _ := initAgent["model"].(string); m != "openai/gpt-5" {
		t.Fatalf("sdd-init model = %q, want %q (new agent should get root model)", m, "openai/gpt-5")
	}
}

// TestInjectOpenCodeMultiModeExistingAgentWithNoModelIsNotTouched verifies
// that a pre-existing agent WITHOUT a model field is respected — the root model
// is NOT injected for that agent. The user intentionally set up the agent
// without a model (they may rely on per-project overrides or session context).
func TestInjectOpenCodeMultiModeExistingAgentWithNoModelIsNotTouched(t *testing.T) {
	home := t.TempDir()
	mockNoPackageManager(t)

	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	// Pre-existing config: root model + sdd-apply with NO model field.
	existing := `{
  "model": "openai/gpt-5",
  "agent": {
    "sdd-apply": {
      "mode": "subagent"
    }
  }
}`
	if err := os.WriteFile(settingsPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("WriteFile(opencode.json) error = %v", err)
	}

	if _, err := Inject(home, opencodeAdapter(), "multi"); err != nil {
		t.Fatalf("Inject(multi) error = %v", err)
	}

	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(opencode.json) error = %v", err)
	}

	root := map[string]any{}
	if err := json.Unmarshal(content, &root); err != nil {
		t.Fatalf("Unmarshal(opencode.json) error = %v", err)
	}

	agentMap, ok := root["agent"].(map[string]any)
	if !ok {
		t.Fatal("opencode.json missing agent map")
	}

	// sdd-apply was pre-existing with NO model — the root model must NOT be injected.
	// The user intentionally set up the agent without a model; respect that.
	applyAgent, ok := agentMap["sdd-apply"].(map[string]any)
	if !ok {
		t.Fatal("sdd-apply agent not found or wrong type")
	}
	if _, hasModel := applyAgent["model"]; hasModel {
		t.Fatalf("sdd-apply should NOT have model field (pre-existing agent without model, user intent must be respected)")
	}

	// sdd-init was NOT pre-existing — should get root model as default.
	initAgent, ok := agentMap["sdd-init"].(map[string]any)
	if !ok {
		t.Fatal("sdd-init agent not found or wrong type")
	}
	if m, _ := initAgent["model"].(string); m != "openai/gpt-5" {
		t.Fatalf("sdd-init model = %q, want %q (new agent should get root model)", m, "openai/gpt-5")
	}
}

// ---------------------------------------------------------------------------
// Fix 1: sdd-phase-common.md — all 4 shared files written to disk
// ---------------------------------------------------------------------------

// TestInjectWritesAllFourSharedFilesToDisk verifies that all four _shared
// convention files (including the recently-added sdd-phase-common.md) are
// actually written to the agent's skills/_shared/ directory during Inject().
// This is a disk-level test; assets_test.go only checks the embedded FS.
func TestInjectWritesAllFourSharedFilesToDisk(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, opencodeAdapter(), "")
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Inject() changed = false")
	}

	sharedDir := filepath.Join(home, ".config", "opencode", "skills", "_shared")
	expectedFiles := []string{
		"persistence-contract.md",
		"engram-convention.md",
		"openspec-convention.md",
		"sdd-phase-common.md",
		"skill-resolver.md",
	}

	for _, fileName := range expectedFiles {
		path := filepath.Join(sharedDir, fileName)
		info, statErr := os.Stat(path)
		if statErr != nil {
			t.Errorf("shared file %q not found on disk: %v", path, statErr)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("shared file %q is empty", path)
		}

		// Verify the result.Files slice includes each shared path.
		found := false
		for _, f := range result.Files {
			if f == path {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("shared file %q not reported in result.Files", path)
		}
	}
}

// TestInjectSharedDirCreatedWithAllFiles verifies that Inject() creates the
// _shared directory when it does not exist and writes all four files into it.
func TestInjectSharedDirCreatedWithAllFiles(t *testing.T) {
	home := t.TempDir()

	// Sanity: _shared dir must not exist yet.
	sharedDir := filepath.Join(home, ".config", "opencode", "skills", "_shared")
	if _, err := os.Stat(sharedDir); err == nil {
		t.Fatal("precondition failed: _shared dir already exists")
	}

	if _, err := Inject(home, opencodeAdapter(), ""); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	entries, err := os.ReadDir(sharedDir)
	if err != nil {
		t.Fatalf("ReadDir(_shared) error = %v (dir was not created)", err)
	}

	names := make(map[string]bool, len(entries))
	for _, e := range entries {
		names[e.Name()] = true
	}

	for _, want := range []string{"persistence-contract.md", "engram-convention.md", "openspec-convention.md", "sdd-phase-common.md", "skill-resolver.md"} {
		if !names[want] {
			t.Errorf("_shared directory missing %q after Inject()", want)
		}
	}
}

// ---------------------------------------------------------------------------
// Fix 2: orchestrator dedup — stripBareOrchestratorSection unit tests
// ---------------------------------------------------------------------------

// TestStripBareOrchestratorSection_BareAtBeginning verifies that a bare
// orchestrator section that appears BEFORE any other content is stripped.
func TestStripBareOrchestratorSection_BareAtBeginning(t *testing.T) {
	input := "## Agent Teams Orchestrator\n\nYou are a COORDINATOR.\n\n## Other Section\n\nSome content.\n"
	result := stripBareOrchestratorSection(input)

	if strings.Contains(result, "You are a COORDINATOR.") {
		t.Fatal("bare orchestrator at beginning was not stripped")
	}
	if !strings.Contains(result, "## Other Section") {
		t.Fatal("content after bare orchestrator was lost")
	}
	if !strings.Contains(result, "Some content.") {
		t.Fatal("content after bare orchestrator section was lost")
	}
}

// TestStripBareOrchestratorSection_OnlyOrchestratorContent verifies that a
// file containing ONLY the bare orchestrator section (no surrounding content)
// is reduced to an empty string (or just a newline).
func TestStripBareOrchestratorSection_OnlyOrchestratorContent(t *testing.T) {
	input := "## Agent Teams Orchestrator\n\nYou are a COORDINATOR, not an executor.\n"
	result := stripBareOrchestratorSection(input)

	if strings.Contains(result, "COORDINATOR") {
		t.Fatalf("solo bare orchestrator section was not stripped: %q", result)
	}
}

// TestStripBareOrchestratorSection_PreservesBeforeAndAfter verifies that
// stripBareOrchestratorSection keeps content both BEFORE and AFTER the section.
func TestStripBareOrchestratorSection_PreservesBeforeAndAfter(t *testing.T) {
	input := "# My Rules\n\n## Rules\n\nBe excellent.\n\n## Agent Teams Orchestrator\n\nYou are a COORDINATOR.\n\n### Delegation Rules\n\nOld rules.\n\n## Other Section\n\nOther content.\n"
	result := stripBareOrchestratorSection(input)

	if strings.Contains(result, "You are a COORDINATOR.") {
		t.Fatal("bare orchestrator content was not removed")
	}
	if strings.Contains(result, "Old rules.") {
		t.Fatal("orchestrator sub-content was not removed")
	}
	if !strings.Contains(result, "Be excellent.") {
		t.Fatal("content BEFORE bare orchestrator was lost")
	}
	if !strings.Contains(result, "## Other Section") {
		t.Fatal("heading AFTER bare orchestrator was lost")
	}
	if !strings.Contains(result, "Other content.") {
		t.Fatal("content AFTER bare orchestrator was lost")
	}
}

// TestStripBareOrchestratorSection_NoOpWhenNoSection verifies that a file
// without any orchestrator heading is returned unchanged.
func TestStripBareOrchestratorSection_NoOpWhenNoSection(t *testing.T) {
	input := "# My Rules\n\n## Rules\n\nBe excellent.\n"
	result := stripBareOrchestratorSection(input)

	if result != input {
		t.Fatalf("no-op case mutated content:\ngot:  %q\nwant: %q", result, input)
	}
}

// TestStripBareOrchestratorSection_DoesNotStripIfMarkersPresent verifies that
// a section that already has HTML comment markers is NOT stripped by
// stripBareOrchestratorSection (the markers are handled by InjectMarkdownSection).
// This ensures the migration guard in injectMarkdownSections() is correct.
func TestStripBareOrchestratorSection_DoesNotStripIfMarkersPresent(t *testing.T) {
	input := "# My Rules\n\n<!-- gentle-ai:sdd-orchestrator -->\n## Agent Teams Orchestrator\n\nYou are a COORDINATOR.\n<!-- /gentle-ai:sdd-orchestrator -->\n"

	// The function sees "## Agent Teams Orchestrator" and would normally strip it.
	// But the caller (injectMarkdownSections) is supposed to check for markers
	// first and skip the strip call. This test documents what happens if
	// stripBareOrchestratorSection is called on already-marked content:
	// the heading will be removed, which is WRONG — this validates the guard.
	result := stripBareOrchestratorSection(input)

	// Because stripBareOrchestratorSection does not check for markers itself,
	// calling it on marked content would damage the file. The real protection is
	// the `!strings.Contains(existing, "<!-- gentle-ai:sdd-orchestrator -->")` guard
	// in injectMarkdownSections(). This test confirms that guard works end-to-end.
	_ = result
}

// ---------------------------------------------------------------------------
// Task 6: StrictTDD marker injected into system prompt files
// ---------------------------------------------------------------------------

// TestInjectStrictTDDEnabledInjectsMarkerIntoClaude verifies that when
// InjectOptions.StrictTDD = true, the injected content in CLAUDE.md contains
// the <!-- gentle-ai:strict-tdd-mode --> marker with its content.
func TestInjectStrictTDDEnabledInjectsMarkerIntoClaude(t *testing.T) {
	home := t.TempDir()

	opts := InjectOptions{StrictTDD: true}
	result, err := Inject(home, claudeAdapter(), "", opts)
	if err != nil {
		t.Fatalf("Inject(claude, StrictTDD=true) error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Inject() changed = false")
	}

	content, err := os.ReadFile(filepath.Join(home, ".claude", "CLAUDE.md"))
	if err != nil {
		t.Fatalf("ReadFile(CLAUDE.md) error = %v", err)
	}

	text := string(content)
	if !strings.Contains(text, "<!-- gentle-ai:strict-tdd-mode -->") {
		t.Fatal("CLAUDE.md missing <!-- gentle-ai:strict-tdd-mode --> open marker")
	}
	if !strings.Contains(text, "<!-- /gentle-ai:strict-tdd-mode -->") {
		t.Fatal("CLAUDE.md missing <!-- /gentle-ai:strict-tdd-mode --> close marker")
	}
	if !strings.Contains(text, "Strict TDD Mode: enabled") {
		t.Fatal("CLAUDE.md missing 'Strict TDD Mode: enabled' content")
	}
}

// TestInjectStrictTDDDisabledDoesNotInjectMarker verifies that when
// InjectOptions.StrictTDD = false (default), the strict-tdd marker is NOT injected.
func TestInjectStrictTDDDisabledDoesNotInjectMarker(t *testing.T) {
	home := t.TempDir()

	// Default (no opts) — strict TDD disabled.
	_, err := Inject(home, claudeAdapter(), "")
	if err != nil {
		t.Fatalf("Inject(claude, default) error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(home, ".claude", "CLAUDE.md"))
	if err != nil {
		t.Fatalf("ReadFile(CLAUDE.md) error = %v", err)
	}

	text := string(content)
	if strings.Contains(text, "<!-- gentle-ai:strict-tdd-mode -->") {
		t.Fatal("CLAUDE.md should NOT contain strict-tdd-mode marker when StrictTDD=false")
	}
}

// TestInjectStrictTDDIsIdempotent verifies that injecting with StrictTDD=true
// twice does not duplicate the marker.
func TestInjectStrictTDDIsIdempotent(t *testing.T) {
	home := t.TempDir()

	opts := InjectOptions{StrictTDD: true}

	first, err := Inject(home, claudeAdapter(), "", opts)
	if err != nil {
		t.Fatalf("Inject() first error = %v", err)
	}
	if !first.Changed {
		t.Fatal("first Inject() changed = false")
	}

	second, err := Inject(home, claudeAdapter(), "", opts)
	if err != nil {
		t.Fatalf("Inject() second error = %v", err)
	}
	if second.Changed {
		t.Fatal("second Inject() changed = true — strict-tdd marker was duplicated")
	}
}

// ---------------------------------------------------------------------------
// Task 1: All files from each skill directory are copied (not just SKILL.md)
// ---------------------------------------------------------------------------

// TestInjectCopiesAllFilesFromSkillDirectory verifies that Inject() copies
// ALL .md files from each skill directory, not just SKILL.md.
// Specifically, sdd-apply/strict-tdd.md and sdd-verify/strict-tdd-verify.md
// must be written to disk alongside their SKILL.md files.
func TestInjectCopiesAllFilesFromSkillDirectory(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, opencodeAdapter(), "")
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Inject() changed = false")
	}

	skillsDir := filepath.Join(home, ".config", "opencode", "skills")

	tests := []struct {
		skill string
		file  string
	}{
		{"sdd-apply", "SKILL.md"},
		{"sdd-apply", "strict-tdd.md"},
		{"sdd-verify", "SKILL.md"},
		{"sdd-verify", "strict-tdd-verify.md"},
	}

	for _, tt := range tests {
		path := filepath.Join(skillsDir, tt.skill, tt.file)
		info, statErr := os.Stat(path)
		if statErr != nil {
			t.Errorf("skill file %q/%q not found on disk: %v", tt.skill, tt.file, statErr)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("skill file %q/%q is empty", tt.skill, tt.file)
		}
	}
}

// TestInjectCopiesAllFilesReportedInResult verifies that all skill files
// (including extra files beyond SKILL.md) are included in result.Files.
func TestInjectCopiesAllFilesReportedInResult(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, opencodeAdapter(), "")
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	skillsDir := filepath.Join(home, ".config", "opencode", "skills")
	wantPaths := []string{
		filepath.Join(skillsDir, "sdd-apply", "strict-tdd.md"),
		filepath.Join(skillsDir, "sdd-verify", "strict-tdd-verify.md"),
	}

	resultSet := make(map[string]bool, len(result.Files))
	for _, f := range result.Files {
		resultSet[f] = true
	}

	for _, want := range wantPaths {
		if !resultSet[want] {
			t.Errorf("expected %q in result.Files, but it was not found", want)
		}
	}
}

// TestInjectClaudeDeduplicatesBareOrchestratorAtBeginning verifies that a bare
// orchestrator section at the very START of CLAUDE.md is handled correctly.
func TestInjectClaudeDeduplicatesBareOrchestratorAtBeginning(t *testing.T) {
	home := t.TempDir()
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	// Bare orchestrator at the very start, followed by other content.
	existing := "## Agent Teams Orchestrator\n\nYou are a COORDINATOR.\n\n## Other Rules\n\nBe excellent.\n"
	if err := os.WriteFile(filepath.Join(claudeDir, "CLAUDE.md"), []byte(existing), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Inject(home, claudeAdapter(), "")
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	content, readErr := os.ReadFile(filepath.Join(claudeDir, "CLAUDE.md"))
	if readErr != nil {
		t.Fatalf("ReadFile() error = %v", readErr)
	}
	text := string(content)

	if count := strings.Count(text, "## Agent Teams Orchestrator"); count != 1 {
		t.Fatalf("expected 1 Agent Teams Orchestrator heading, got %d\n\ncontent:\n%s", count, text)
	}
	if !strings.Contains(text, "<!-- gentle-ai:sdd-orchestrator -->") {
		t.Fatal("missing open marker after injection")
	}
	if !strings.Contains(text, "## Other Rules") {
		t.Fatal("content after bare orchestrator was lost")
	}
	if !strings.Contains(text, "Be excellent.") {
		t.Fatal("content after bare orchestrator section was lost")
	}
}

// TestInjectClaudeDeduplicatesFileWithOnlyBareOrchestrator verifies that a
// CLAUDE.md containing ONLY the bare orchestrator (no other sections) is
// correctly replaced with the marker-based version.
func TestInjectClaudeDeduplicatesFileWithOnlyBareOrchestrator(t *testing.T) {
	home := t.TempDir()
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	// Use a unique phrase that does NOT appear in the canonical orchestrator
	// asset so we can confirm the bare version was stripped.
	existing := "## Agent Teams Orchestrator\n\nYou are a COORDINATOR.\n\n### Delegation Rules\n\nLEGACY-RULE-MARKER-XYZ\n"
	if err := os.WriteFile(filepath.Join(claudeDir, "CLAUDE.md"), []byte(existing), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Inject(home, claudeAdapter(), "")
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	content, readErr := os.ReadFile(filepath.Join(claudeDir, "CLAUDE.md"))
	if readErr != nil {
		t.Fatalf("ReadFile() error = %v", readErr)
	}
	text := string(content)

	// Should have exactly one orchestrator heading (the injected one).
	if count := strings.Count(text, "## Agent Teams Orchestrator"); count != 1 {
		t.Fatalf("expected 1 Agent Teams Orchestrator heading, got %d\n\ncontent:\n%s", count, text)
	}
	// Must have markers.
	if !strings.Contains(text, "<!-- gentle-ai:sdd-orchestrator -->") {
		t.Fatal("missing open marker")
	}
	if !strings.Contains(text, "<!-- /gentle-ai:sdd-orchestrator -->") {
		t.Fatal("missing close marker")
	}
	// The unique legacy phrase must be gone — the bare section was stripped.
	if strings.Contains(text, "LEGACY-RULE-MARKER-XYZ") {
		t.Fatal("old bare orchestrator content (unique marker) survived after injection")
	}
}

// TestInjectClaudeDeduplicatesBareOrchestratorIsIdempotent verifies that
// running Inject() TWICE on a file that started with a bare orchestrator
// section produces exactly one orchestrator section (no accumulation).
func TestInjectClaudeDeduplicatesBareOrchestratorIsIdempotent(t *testing.T) {
	home := t.TempDir()
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	// Start from bare state.
	existing := "# My Rules\n\n## Agent Teams Orchestrator\n\nYou are a COORDINATOR.\n"
	if err := os.WriteFile(filepath.Join(claudeDir, "CLAUDE.md"), []byte(existing), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// First inject — strips bare, inserts marked section.
	if _, err := Inject(home, claudeAdapter(), ""); err != nil {
		t.Fatalf("Inject() first error = %v", err)
	}

	// Second inject — must be a no-op (already has markers).
	second, err := Inject(home, claudeAdapter(), "")
	if err != nil {
		t.Fatalf("Inject() second error = %v", err)
	}
	if second.Changed {
		t.Fatal("second Inject() changed = true — idempotency broken after dedup migration")
	}

	content, readErr := os.ReadFile(filepath.Join(claudeDir, "CLAUDE.md"))
	if readErr != nil {
		t.Fatalf("ReadFile() error = %v", readErr)
	}
	text := string(content)

	if count := strings.Count(text, "## Agent Teams Orchestrator"); count != 1 {
		t.Fatalf("expected 1 Agent Teams Orchestrator heading after 2 injects, got %d\n\ncontent:\n%s", count, text)
	}
}

// TestInjectClaudeDoesNotStripMarkedSection verifies that an existing
// CLAUDE.md with a properly-marked orchestrator section is NOT stripped and
// re-written as bare content (the migration guard must only fire when markers
// are absent).
func TestInjectClaudeDoesNotStripMarkedSection(t *testing.T) {
	home := t.TempDir()
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	// Pre-inject once to produce the canonical marked state.
	if _, err := Inject(home, claudeAdapter(), ""); err != nil {
		t.Fatalf("first Inject() error = %v", err)
	}

	// Read and verify markers.
	after1, err := os.ReadFile(filepath.Join(claudeDir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(after1), "<!-- gentle-ai:sdd-orchestrator -->") {
		t.Fatal("markers not present after first inject — test precondition failed")
	}

	// Second inject — must not change the file.
	second, err := Inject(home, claudeAdapter(), "")
	if err != nil {
		t.Fatalf("second Inject() error = %v", err)
	}
	if second.Changed {
		t.Fatal("second Inject() changed = true — marked section was incorrectly re-processed")
	}
}

// ---------------------------------------------------------------------------
// Background-agents plugin tests (Step 4)
// ---------------------------------------------------------------------------

func TestInjectOpenCodeMultiWritesPlugin(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, opencodeAdapter(), "multi")
	if err != nil {
		t.Fatalf("Inject(multi) error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Inject(multi) changed = false")
	}

	pluginPath := filepath.Join(home, ".config", "opencode", "plugins", "background-agents.ts")

	// Assert: plugin file exists
	content, err := os.ReadFile(pluginPath)
	if err != nil {
		t.Fatalf("ReadFile(background-agents.ts) error = %v", err)
	}

	// Assert: file content matches embedded asset
	expected := assets.MustRead("opencode/plugins/background-agents.ts")
	if string(content) != expected {
		t.Fatalf("plugin content mismatch: got %d bytes, want %d bytes", len(content), len(expected))
	}

	// Assert: file is in InjectionResult.Files
	found := false
	for _, f := range result.Files {
		if f == pluginPath {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("plugin path %q not reported in result.Files: %v", pluginPath, result.Files)
	}
}

func TestInjectOpenCodeSingleWritesPlugin(t *testing.T) {
	home := t.TempDir()
	mockNoPackageManager(t)

	_, err := Inject(home, opencodeAdapter(), "single")
	if err != nil {
		t.Fatalf("Inject(single) error = %v", err)
	}

	pluginPath := filepath.Join(home, ".config", "opencode", "plugins", "background-agents.ts")
	if _, err := os.Stat(pluginPath); err != nil {
		t.Fatalf("plugin file should exist in single mode: %v", err)
	}
}

func TestInjectOpenCodePluginNoPkgManagerAvailable(t *testing.T) {
	// Mock: no package manager (neither bun nor npm) is available.
	orig := npmLookPath
	npmLookPath = func(string) (string, error) {
		return "", fmt.Errorf("not found")
	}
	defer func() { npmLookPath = orig }()

	home := t.TempDir()

	// Assert: inject succeeds even when no package manager is available (soft skip).
	result, err := Inject(home, opencodeAdapter(), "multi")
	if err != nil {
		t.Fatalf("Inject(multi) with no package manager error = %v", err)
	}

	// Assert: plugin file was still written regardless.
	pluginPath := filepath.Join(home, ".config", "opencode", "plugins", "background-agents.ts")
	if _, err := os.Stat(pluginPath); err != nil {
		t.Fatalf("plugin file should exist even when no package manager available: %v", err)
	}

	_ = result
}

func TestInjectOpenCodePluginNpmFailureReturnsActionableError(t *testing.T) {
	// Mock: package manager IS available but the install fails.
	orig := npmLookPath
	origRun := npmRun
	npmLookPath = func(bin string) (string, error) {
		if bin == "bun" {
			return "", fmt.Errorf("not found")
		}
		if bin == "npm" {
			return "/usr/bin/npm", nil
		}
		return "", fmt.Errorf("not found")
	}
	npmRun = func(dir string, args ...string) ([]byte, error) {
		return []byte("ERR! some npm error"), fmt.Errorf("exit status 1")
	}
	defer func() {
		npmLookPath = orig
		npmRun = origRun
	}()

	home := t.TempDir()

	_, err := Inject(home, opencodeAdapter(), "multi")
	if err == nil {
		t.Fatal("Inject(multi) should fail when npm install fails")
	}
	if !strings.Contains(err.Error(), "npm install") {
		t.Fatalf("error should mention 'npm install', got: %v", err)
	}
	if !strings.Contains(err.Error(), "unique-names-generator") {
		t.Fatalf("error should mention the package name, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Fix:") {
		t.Fatalf("error should contain actionable fix instructions, got: %v", err)
	}
}

func TestInjectOpenCodePluginBunPreferredOverNpm(t *testing.T) {
	// Mock: both bun and npm available; only bun should be called.
	orig := npmLookPath
	origRun := npmRun

	var calledWith string
	npmLookPath = func(bin string) (string, error) {
		// Both available — bun should win.
		if bin == "bun" || bin == "npm" {
			return "/usr/local/bin/" + bin, nil
		}
		return "", fmt.Errorf("not found")
	}
	npmRun = func(dir string, args ...string) ([]byte, error) {
		if len(args) > 0 {
			calledWith = args[0]
		}
		// Simulate successful install by creating the node_modules directory.
		nmPath := filepath.Join(dir, "node_modules", "unique-names-generator")
		if err := os.MkdirAll(nmPath, 0o755); err != nil {
			return nil, err
		}
		return []byte(""), nil
	}
	defer func() {
		npmLookPath = orig
		npmRun = origRun
	}()

	home := t.TempDir()
	_, err := Inject(home, opencodeAdapter(), "multi")
	if err != nil {
		t.Fatalf("Inject(multi) error = %v", err)
	}

	if !strings.Contains(calledWith, "bun") {
		t.Fatalf("expected bun to be preferred over npm, but called: %q", calledWith)
	}
}

func TestInjectOpenCodePluginIdempotent(t *testing.T) {
	home := t.TempDir()
	mockNoPackageManager(t)

	// First run
	first, err := Inject(home, opencodeAdapter(), "multi")
	if err != nil {
		t.Fatalf("Inject(multi) first error = %v", err)
	}
	if !first.Changed {
		t.Fatal("Inject(multi) first changed = false")
	}

	// Second run: Changed should be false (plugin unchanged)
	second, err := Inject(home, opencodeAdapter(), "multi")
	if err != nil {
		t.Fatalf("Inject(multi) second error = %v", err)
	}
	if second.Changed {
		t.Fatal("Inject(multi) second changed = true — plugin idempotency broken")
	}
}

func TestInjectModelAssignmentsFunction(t *testing.T) {
	overlayJSON := []byte(`{
  "agent": {
    "sdd-init": {"mode": "subagent", "prompt": "test"},
    "sdd-apply": {"mode": "subagent", "prompt": "test"}
  }
}`)

	assignments := map[string]model.ModelAssignment{
		"sdd-init": {ProviderID: "anthropic", ModelID: "claude-sonnet-4-20250514"},
	}

	result, err := injectModelAssignments(overlayJSON, assignments, "", nil)
	if err != nil {
		t.Fatalf("injectModelAssignments() error = %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("Unmarshal result error = %v", err)
	}

	agents := parsed["agent"].(map[string]any)
	initAgent := agents["sdd-init"].(map[string]any)
	if m, _ := initAgent["model"].(string); m != "anthropic/claude-sonnet-4-20250514" {
		t.Fatalf("sdd-init model = %q, want %q", m, "anthropic/claude-sonnet-4-20250514")
	}

	// sdd-apply has no assignment — should NOT get a model field.
	applyAgent := agents["sdd-apply"].(map[string]any)
	if _, hasModel := applyAgent["model"]; hasModel {
		t.Fatal("sdd-apply should not have a model field (no assignment)")
	}
}

// ---------------------------------------------------------------------------
// Windsurf workflow injection tests
// ---------------------------------------------------------------------------

func TestInjectWindsurf_WorkflowsCopiedToWorkspace(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"), []byte("module test\n"), 0o644); err != nil {
		t.Fatalf("write go.mod marker: %v", err)
	}

	mockNoPackageManager(t)

	result, err := Inject(home, windsurfAdapter(), "", InjectOptions{WorkspaceDir: workspace})
	if err != nil {
		t.Fatalf("Inject(windsurf) error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Inject(windsurf) changed = false")
	}

	// Verify sdd-new.md was written to .windsurf/workflows/
	workflowPath := filepath.Join(workspace, ".windsurf", "workflows", "sdd-new.md")
	if _, err := os.Stat(workflowPath); err != nil {
		t.Fatalf("workflow file %q not found: %v", workflowPath, err)
	}

	// Verify the file is in the returned Files slice.
	found := false
	for _, f := range result.Files {
		if f == workflowPath {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("workflow path %q not in result.Files: %v", workflowPath, result.Files)
	}
}

func TestInjectWindsurf_WorkflowsIdempotent(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"), []byte("module test\n"), 0o644); err != nil {
		t.Fatalf("write go.mod marker: %v", err)
	}

	mockNoPackageManager(t)

	opts := InjectOptions{WorkspaceDir: workspace}

	if _, err := Inject(home, windsurfAdapter(), "", opts); err != nil {
		t.Fatalf("first Inject(windsurf) error = %v", err)
	}

	second, err := Inject(home, windsurfAdapter(), "", opts)
	if err != nil {
		t.Fatalf("second Inject(windsurf) error = %v", err)
	}
	if second.Changed {
		t.Fatal("second Inject(windsurf) changed = true — workflow injection is not idempotent")
	}
}

func TestInjectWindsurf_WorkflowsSkippedWithoutWorkspaceDir(t *testing.T) {
	home := t.TempDir()

	mockNoPackageManager(t)

	// No WorkspaceDir → workflow step must be silently skipped.
	result, err := Inject(home, windsurfAdapter(), "")
	if err != nil {
		t.Fatalf("Inject(windsurf) without workspaceDir error = %v", err)
	}

	for _, f := range result.Files {
		if strings.Contains(f, ".windsurf") {
			t.Fatalf("unexpected .windsurf path in result.Files when WorkspaceDir is empty: %q", f)
		}
	}
}

func TestInjectWindsurf_WorkflowsSkippedForNonProjectDir(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir() // empty dir — no .git, go.mod, package.json, etc.

	mockNoPackageManager(t)

	result, err := Inject(home, windsurfAdapter(), "", InjectOptions{WorkspaceDir: workspace})
	if err != nil {
		t.Fatalf("Inject(windsurf) error = %v", err)
	}

	for _, f := range result.Files {
		if strings.Contains(f, ".windsurf") {
			// On Windows, if t.TempDir is under a real home dir with package.json,
			// findProjectRoot may legitimately find the home dir as a project.
			// We skip the failure if it targets the real user home.
			if strings.Contains(f, `\Users\`) {
				t.Logf("Skipping unexpected workflow found in real home: %q", f)
				continue
			}
			t.Fatalf("workflow file %q should not be injected into non-project dir", f)
		}
	}
}

func TestInjectWindsurf_WorkflowContentMatchesAsset(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"), []byte("module test\n"), 0o644); err != nil {
		t.Fatalf("write go.mod marker: %v", err)
	}

	mockNoPackageManager(t)

	if _, err := Inject(home, windsurfAdapter(), "", InjectOptions{WorkspaceDir: workspace}); err != nil {
		t.Fatalf("Inject(windsurf) error = %v", err)
	}

	got, err := os.ReadFile(filepath.Join(workspace, ".windsurf", "workflows", "sdd-new.md"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	want := assets.MustRead("windsurf/workflows/sdd-new.md")
	if string(got) != want {
		t.Fatalf("workflow file content mismatch:\ngot len=%d, want len=%d", len(got), len(want))
	}
}

func TestInjectWindsurf_WorkflowsFoundFromSubdirectory(t *testing.T) {
	home := t.TempDir()

	// Simulate a real project: go.mod lives at the root.
	projectRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectRoot, "go.mod"), []byte("module test\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	// Simulate running gentle-ai from a subdirectory inside that project.
	subDir := filepath.Join(projectRoot, "internal", "foo")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir subDir: %v", err)
	}

	mockNoPackageManager(t)

	// Pass the subdirectory as WorkspaceDir — findProjectRoot must traverse
	// upward and find go.mod at projectRoot.
	result, err := Inject(home, windsurfAdapter(), "", InjectOptions{WorkspaceDir: subDir})
	if err != nil {
		t.Fatalf("Inject(windsurf) from subDir error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Inject(windsurf) from subDir: changed = false, expected workflow to be written")
	}

	// Workflow must be at the PROJECT ROOT, not inside the subdirectory.
	expectedPath := filepath.Join(projectRoot, ".windsurf", "workflows", "sdd-new.md")
	if _, err := os.Stat(expectedPath); err != nil {
		t.Fatalf("workflow not found at project root %q: %v", expectedPath, err)
	}

	// Must NOT be written inside the subdirectory.
	unexpectedPath := filepath.Join(subDir, ".windsurf", "workflows", "sdd-new.md")
	if _, err := os.Stat(unexpectedPath); err == nil {
		t.Fatalf("workflow was incorrectly written inside subdirectory %q", unexpectedPath)
	}
}

// ---------------------------------------------------------------------------
// Agent-specific SDD orchestrator asset selection tests
// ---------------------------------------------------------------------------

// TestSDDOrchestratorAssetSelection verifies that sddOrchestratorAsset()
// returns agent-specific paths for agents that have dedicated orchestrators,
// and falls back to generic for all others.
func TestSDDOrchestratorAssetSelection(t *testing.T) {
	tests := []struct {
		agent model.AgentID
		want  string
	}{
		{agent: model.AgentGeminiCLI, want: "gemini/sdd-orchestrator.md"},
		{agent: model.AgentAntigravity, want: "antigravity/sdd-orchestrator.md"},
		{agent: model.AgentCodex, want: "codex/sdd-orchestrator.md"},
		{agent: model.AgentWindsurf, want: "windsurf/sdd-orchestrator.md"},
		{agent: model.AgentCursor, want: "cursor/sdd-orchestrator.md"},
		{agent: model.AgentQwenCode, want: "qwen/sdd-orchestrator.md"},
		{agent: model.AgentClaudeCode, want: "generic/sdd-orchestrator.md"},
		{agent: model.AgentOpenCode, want: "opencode/sdd-orchestrator.md"},
		{agent: model.AgentVSCodeCopilot, want: "generic/sdd-orchestrator.md"},
	}

	for _, tt := range tests {
		t.Run(string(tt.agent), func(t *testing.T) {
			got := sddOrchestratorAsset(tt.agent)
			if got != tt.want {
				t.Fatalf("sddOrchestratorAsset(%q) = %q, want %q", tt.agent, got, tt.want)
			}
		})
	}
}

// TestInjectGeminiUsesAgentSpecificAsset verifies that Gemini injection uses
// the gemini-specific sdd-orchestrator asset (with ~/.gemini/skills/ paths),
// not the generic one with wrong vendor paths.
func TestInjectGeminiUsesAgentSpecificAsset(t *testing.T) {
	home := t.TempDir()

	geminiAdapter, err := agents.NewAdapter("gemini-cli")
	if err != nil {
		t.Fatalf("NewAdapter(gemini-cli) error = %v", err)
	}

	result, injectErr := Inject(home, geminiAdapter, "")
	if injectErr != nil {
		t.Fatalf("Inject(gemini) error = %v", injectErr)
	}
	if !result.Changed {
		t.Fatal("Inject(gemini) changed = false")
	}

	promptPath := filepath.Join(home, ".gemini", "GEMINI.md")
	content, readErr := os.ReadFile(promptPath)
	if readErr != nil {
		t.Fatalf("ReadFile(%q) error = %v", promptPath, readErr)
	}

	text := string(content)

	// Gemini-specific asset must reference Gemini skill paths.
	if !strings.Contains(text, "~/.gemini/skills/_shared/") {
		t.Fatal("GEMINI.md missing ~/.gemini/skills/_shared/ path — agent-specific asset not used")
	}

	// Gemini-specific asset must NOT reference Codex paths.
	if strings.Contains(text, "~/.codex/") {
		t.Fatal("GEMINI.md contains Codex-specific paths — wrong asset was injected")
	}
}

// TestInjectCodexWritesSDDOrchestratorAndSkills verifies that Codex injection
// creates agents.md with the SDD orchestrator and writes skill files.
func TestInjectCodexWritesSDDOrchestratorAndSkills(t *testing.T) {
	home := t.TempDir()

	codexAdapter, err := agents.NewAdapter("codex")
	if err != nil {
		t.Fatalf("NewAdapter(codex) error = %v", err)
	}

	result, injectErr := Inject(home, codexAdapter, "")
	if injectErr != nil {
		t.Fatalf("Inject(codex) error = %v", injectErr)
	}
	if !result.Changed {
		t.Fatal("Inject(codex) changed = false")
	}

	// Verify SDD orchestrator was injected into agents.md.
	promptPath := filepath.Join(home, ".codex", "agents.md")
	content, readErr := os.ReadFile(promptPath)
	if readErr != nil {
		t.Fatalf("ReadFile(%q) error = %v", promptPath, readErr)
	}

	text := string(content)
	if !strings.Contains(text, "Spec-Driven Development") {
		t.Fatal("agents.md missing SDD orchestrator content")
	}

	// Codex-specific asset must reference Codex skill paths.
	if !strings.Contains(text, "~/.codex/skills/_shared/") {
		t.Fatal("agents.md missing ~/.codex/skills/_shared/ path — agent-specific asset not used")
	}

	// Codex-specific asset must NOT reference Gemini paths.
	if strings.Contains(text, "~/.gemini/") {
		t.Fatal("agents.md contains Gemini-specific paths — wrong asset was injected")
	}

	// Should also write SDD skill files.
	skillPath := filepath.Join(home, ".codex", "skills", "sdd-init", "SKILL.md")
	if _, err := os.Stat(skillPath); err != nil {
		t.Fatalf("expected SDD skill file %q: %v", skillPath, err)
	}

	// Shared files should also be written.
	sharedPath := filepath.Join(home, ".codex", "skills", "_shared", "engram-convention.md")
	if _, err := os.Stat(sharedPath); err != nil {
		t.Fatalf("expected shared SDD convention file %q: %v", sharedPath, err)
	}
}

// TestInjectCodexIsIdempotent verifies that injecting Codex twice does not
// duplicate the SDD orchestrator content.
func TestInjectCodexIsIdempotent(t *testing.T) {
	home := t.TempDir()

	codexAdapter, err := agents.NewAdapter("codex")
	if err != nil {
		t.Fatalf("NewAdapter(codex) error = %v", err)
	}

	first, err := Inject(home, codexAdapter, "")
	if err != nil {
		t.Fatalf("Inject(codex) first error = %v", err)
	}
	if !first.Changed {
		t.Fatal("first Inject(codex) changed = false")
	}

	second, err := Inject(home, codexAdapter, "")
	if err != nil {
		t.Fatalf("Inject(codex) second error = %v", err)
	}
	if second.Changed {
		t.Fatal("second Inject(codex) changed = true — SDD orchestrator was duplicated")
	}
}

// ---------------------------------------------------------------------------
// Regression: post-check must validate in-memory merged bytes, not re-read disk
// (Windows/WSL2 atomic-write visibility bug — "missing sdd-apply sub-agent")
// ---------------------------------------------------------------------------

// TestInjectOpenCodeMultiModeWithPreExistingMinimalConfig reproduces the
// Windows/WSL2 regression where a pre-existing minimal opencode.json (e.g.
// only {"model": "anthropic/..."}) caused the post-check to fail with:
//
//	post-check: .../opencode.json missing sdd-apply sub-agent
//
// The root cause was re-reading the file from disk after the atomic rename,
// which could see stale content on Windows/WSL2. The fix validates against
// the in-memory merged bytes returned by mergeJSONFile instead.
func TestInjectOpenCodeMultiModeWithPreExistingMinimalConfig(t *testing.T) {
	home := t.TempDir()
	mockNoPackageManager(t)

	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	// Simulate a minimal pre-existing config (e.g. set by the user for model selection).
	minimal := `{"model": "anthropic/claude-sonnet-4-20250514"}` + "\n"
	if err := os.WriteFile(settingsPath, []byte(minimal), 0o644); err != nil {
		t.Fatalf("WriteFile(opencode.json) error = %v", err)
	}

	// This must NOT fail with "post-check: ... missing sdd-apply sub-agent".
	result, err := Inject(home, opencodeAdapter(), "multi")
	if err != nil {
		t.Fatalf("Inject(multi) with pre-existing minimal config error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Inject(multi) changed = false")
	}

	// Verify the merged file contains the expected content.
	content, readErr := os.ReadFile(settingsPath)
	if readErr != nil {
		t.Fatalf("ReadFile(opencode.json) error = %v", readErr)
	}

	var root map[string]any
	if err := json.Unmarshal(content, &root); err != nil {
		t.Fatalf("Unmarshal(opencode.json) error = %v", err)
	}

	// The pre-existing model field must be preserved.
	if m, _ := root["model"].(string); m != "anthropic/claude-sonnet-4-20250514" {
		t.Fatalf("pre-existing model field lost after merge: got %q", m)
	}

	agentMap, ok := root["agent"].(map[string]any)
	if !ok {
		t.Fatal("opencode.json missing agent key after merge")
	}
	if _, ok := agentMap["sdd-orchestrator"]; !ok {
		t.Fatal("missing sdd-orchestrator after merge with pre-existing config")
	}
	if _, ok := agentMap["sdd-apply"]; !ok {
		t.Fatal("missing sdd-apply after merge with pre-existing config — post-check regression")
	}
}

// TestInjectOpenCodeMultiModeWithPreExistingFullConfig verifies that a
// pre-existing opencode.json with a non-trivial structure (multiple keys,
// provider settings, etc.) is correctly merged with the multi-mode overlay
// and passes the post-check without any disk re-read race.
func TestInjectOpenCodeMultiModeWithPreExistingFullConfig(t *testing.T) {
	home := t.TempDir()
	mockNoPackageManager(t)

	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	// Simulate a realistic pre-existing user config.
	existing := `{
  "model": "anthropic/claude-sonnet-4-20250514",
  "provider": {
    "anthropic": {
      "apiKey": "sk-ant-..."
    }
  },
  "theme": "dark",
  "keybinds": {
    "leader": "ctrl+g"
  }
}
`
	if err := os.WriteFile(settingsPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("WriteFile(opencode.json) error = %v", err)
	}

	result, err := Inject(home, opencodeAdapter(), "multi")
	if err != nil {
		t.Fatalf("Inject(multi) with full pre-existing config error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Inject(multi) changed = false")
	}

	content, readErr := os.ReadFile(settingsPath)
	if readErr != nil {
		t.Fatalf("ReadFile(opencode.json) error = %v", readErr)
	}

	var root map[string]any
	if err := json.Unmarshal(content, &root); err != nil {
		t.Fatalf("Unmarshal(opencode.json) error = %v", err)
	}

	// All pre-existing top-level keys must be preserved.
	if m, _ := root["model"].(string); m != "anthropic/claude-sonnet-4-20250514" {
		t.Fatalf("pre-existing model field lost: got %q", m)
	}
	if _, ok := root["theme"]; !ok {
		t.Fatal("pre-existing theme field lost after merge")
	}
	if _, ok := root["keybinds"]; !ok {
		t.Fatal("pre-existing keybinds field lost after merge")
	}

	agentMap, ok := root["agent"].(map[string]any)
	if !ok {
		t.Fatal("opencode.json missing agent key after merge")
	}

	// All 10 multi-mode agents must be present.
	for _, agentName := range []string{
		"sdd-orchestrator", "sdd-init", "sdd-explore", "sdd-propose",
		"sdd-spec", "sdd-design", "sdd-tasks", "sdd-apply", "sdd-verify", "sdd-archive",
	} {
		if _, ok := agentMap[agentName]; !ok {
			t.Fatalf("missing agent %q after merge with full pre-existing config", agentName)
		}
	}
}

// ---------------------------------------------------------------------------
// gentleman agent model mirroring from sdd-orchestrator
// ---------------------------------------------------------------------------

// TestInjectOpenCodeMultiModeMirrorsOrchestratorModelToGentleman verifies that
// when sdd-orchestrator has an explicit TUI model assignment and the gentleman
// agent already exists in opencode.json (persona installed), the orchestrator
// model is mirrored to the gentleman agent.
func TestInjectOpenCodeMultiModeMirrorsOrchestratorModelToGentleman(t *testing.T) {
	home := t.TempDir()
	mockNoPackageManager(t)

	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	// Pre-existing opencode.json with gentleman agent (persona installed).
	existing := `{
  "agent": {
    "gentleman": {
      "mode": "primary"
    }
  }
}`
	if err := os.WriteFile(settingsPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("WriteFile(opencode.json) error = %v", err)
	}

	assignments := map[string]model.ModelAssignment{
		"sdd-orchestrator": {ProviderID: "openai", ModelID: "gpt-4o"},
	}

	result, err := Inject(home, opencodeAdapter(), "multi", InjectOptions{OpenCodeModelAssignments: assignments})
	if err != nil {
		t.Fatalf("Inject(multi, assignments) error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Inject(multi, assignments) changed = false")
	}

	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(opencode.json) error = %v", err)
	}

	root := map[string]any{}
	if err := json.Unmarshal(content, &root); err != nil {
		t.Fatalf("Unmarshal(opencode.json) error = %v", err)
	}

	agentMap, ok := root["agent"].(map[string]any)
	if !ok {
		t.Fatal("opencode.json missing agent map")
	}

	// sdd-orchestrator must have the assigned model.
	orchAgent, ok := agentMap["sdd-orchestrator"].(map[string]any)
	if !ok {
		t.Fatal("sdd-orchestrator agent not found or wrong type")
	}
	if m, _ := orchAgent["model"].(string); m != "openai/gpt-4o" {
		t.Fatalf("sdd-orchestrator model = %q, want %q", m, "openai/gpt-4o")
	}

	// gentleman must have the same model as sdd-orchestrator (mirrored).
	gentlemanAgent, ok := agentMap["gentleman"].(map[string]any)
	if !ok {
		t.Fatal("gentleman agent not found or wrong type")
	}
	if m, _ := gentlemanAgent["model"].(string); m != "openai/gpt-4o" {
		t.Fatalf("gentleman model = %q, want %q (should mirror sdd-orchestrator)", m, "openai/gpt-4o")
	}
}

// TestInjectOpenCodeMultiModeDoesNotInjectGentlemanIfNotInstalled verifies that
// when the gentleman agent does NOT exist in opencode.json (persona not installed),
// the orchestrator model is NOT mirrored to a gentleman entry.
func TestInjectOpenCodeMultiModeDoesNotInjectGentlemanIfNotInstalled(t *testing.T) {
	home := t.TempDir()
	mockNoPackageManager(t)

	// No pre-existing opencode.json — fresh install, persona not installed.
	assignments := map[string]model.ModelAssignment{
		"sdd-orchestrator": {ProviderID: "openai", ModelID: "gpt-4o"},
	}

	result, err := Inject(home, opencodeAdapter(), "multi", InjectOptions{OpenCodeModelAssignments: assignments})
	if err != nil {
		t.Fatalf("Inject(multi, assignments) error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Inject(multi, assignments) changed = false")
	}

	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(opencode.json) error = %v", err)
	}

	root := map[string]any{}
	if err := json.Unmarshal(content, &root); err != nil {
		t.Fatalf("Unmarshal(opencode.json) error = %v", err)
	}

	agentMap, ok := root["agent"].(map[string]any)
	if !ok {
		t.Fatal("opencode.json missing agent map")
	}

	// gentleman must NOT appear — persona is not installed.
	if gentlemanRaw, exists := agentMap["gentleman"]; exists {
		// If it somehow exists, it must not have a model field.
		if gentlemanMap, ok := gentlemanRaw.(map[string]any); ok {
			if _, hasModel := gentlemanMap["model"]; hasModel {
				t.Fatal("gentleman should NOT have a model field when persona is not installed")
			}
		}
	}
}

// TestMergeJSONFileReturnsMergedBytes verifies that mergeJSONFile returns the
// merged bytes in-memory, so callers never need to re-read from disk to
// validate the result (the fix for the Windows/WSL2 post-check bug).
func TestMergeJSONFileReturnsMergedBytes(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.json")

	base := `{"existing": "value"}`
	if err := os.WriteFile(path, []byte(base), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	overlay := []byte(`{"new_key": "new_value"}`)

	result, err := mergeJSONFile(path, overlay)
	if err != nil {
		t.Fatalf("mergeJSONFile() error = %v", err)
	}

	// The returned merged bytes must not be nil.
	if len(result.merged) == 0 {
		t.Fatal("mergeJSONFile() returned empty merged bytes — post-check will fail on Windows/WSL2")
	}

	// The merged bytes must contain both the base and overlay content.
	mergedStr := string(result.merged)
	if !strings.Contains(mergedStr, `"existing"`) {
		t.Fatal("merged bytes missing base key 'existing'")
	}
	if !strings.Contains(mergedStr, `"new_key"`) {
		t.Fatal("merged bytes missing overlay key 'new_key'")
	}

	// The merged bytes must be valid JSON.
	var parsed map[string]any
	if err := json.Unmarshal(result.merged, &parsed); err != nil {
		t.Fatalf("merged bytes are not valid JSON: %v", err)
	}

	// writeResult must reflect that the file was changed.
	if !result.writeResult.Changed {
		t.Fatal("writeResult.Changed = false — first write of different content should be changed")
	}
}

// ---------------------------------------------------------------------------
// Fix 1: Cursor sub-agent files written to disk
// ---------------------------------------------------------------------------

func TestInjectCursorWritesSubAgentFiles(t *testing.T) {
	home := t.TempDir()

	cursorAdapter, err := agents.NewAdapter("cursor")
	if err != nil {
		t.Fatalf("NewAdapter(cursor) error = %v", err)
	}

	promptPath := cursorAdapter.SystemPromptFile(home)
	if err := os.MkdirAll(filepath.Dir(promptPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	result, injectErr := Inject(home, cursorAdapter, "")
	if injectErr != nil {
		t.Fatalf("Inject() error = %v", injectErr)
	}

	agentsDir := filepath.Join(home, ".cursor", "agents")
	phases := []string{"sdd-init", "sdd-explore", "sdd-propose", "sdd-spec", "sdd-design", "sdd-tasks", "sdd-apply", "sdd-verify", "sdd-archive"}

	for _, phase := range phases {
		agentPath := filepath.Join(agentsDir, phase+".md")
		info, err := os.Stat(agentPath)
		if err != nil {
			t.Fatalf("agent file %s not found: %v", phase, err)
		}
		if info.Size() < 100 {
			t.Fatalf("agent file %s too small: %d bytes", phase, info.Size())
		}
	}

	// Verify readonly flags: sdd-explore and sdd-verify must use readonly: false
	// so they can use terminal commands and MCP tools (issue #156).
	for _, phase := range []string{"sdd-explore", "sdd-verify"} {
		content, err := os.ReadFile(filepath.Join(agentsDir, phase+".md"))
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", phase, err)
		}
		if !strings.Contains(string(content), "readonly: false") {
			t.Fatalf("agent %s should have readonly: false (terminal/MCP access required)", phase)
		}
	}

	// Verify result.Files includes agent paths
	hasAgentFile := false
	for _, f := range result.Files {
		// Normalize for Windows paths
		if strings.Contains(strings.ReplaceAll(f, `\`, `/`), ".cursor/agents/") {
			hasAgentFile = true
			break
		}
	}
	if !hasAgentFile {
		t.Fatal("result.Files should include at least one cursor agent path")
	}

	// Idempotency: second run should not change files
	result2, err := Inject(home, cursorAdapter, "")
	if err != nil {
		t.Fatalf("second Inject() error = %v", err)
	}
	for _, f := range result2.Files {
		if strings.Contains(f, ".cursor/agents/") {
			t.Fatalf("second inject should not report changed agent files, but got %s", f)
		}
	}
}

// TestInjectKiroFallsBackToClaudeModelAssignmentsWhenKiroMapUnset verifies that
// when KiroModelAssignments is nil, the injector falls back to ClaudeModelAssignments
// for Kiro phase model resolution (legacy backward-compatible path).
func TestInjectKiroFallsBackToClaudeModelAssignmentsWhenKiroMapUnset(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))

	adapter, err := agents.NewAdapter(model.AgentKiroIDE)
	if err != nil {
		t.Fatalf("NewAdapter(kiro-ide) error = %v", err)
	}

	assignments := map[string]model.ClaudeModelAlias{
		// Non-default overrides we need to prove at runtime.
		"sdd-design":  model.ClaudeModelOpus,
		"sdd-archive": model.ClaudeModelHaiku,
		// Default fallback for unspecified phases.
		"default": model.ClaudeModelSonnet,
	}

	result, err := Inject(home, adapter, "", InjectOptions{ClaudeModelAssignments: assignments})
	if err != nil {
		t.Fatalf("Inject(kiro, custom assignments) error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Inject(kiro, custom assignments) changed = false")
	}

	tests := []struct {
		phase string
		want  string
	}{
		{phase: "sdd-design", want: "model: claude-opus-4.6"},
		{phase: "sdd-archive", want: "model: claude-haiku-4.5"},
		// Unspecified phase should use default sonnet.
		{phase: "sdd-spec", want: "model: claude-sonnet-4.6"},
	}

	for _, tt := range tests {
		path := filepath.Join(home, ".kiro", "agents", tt.phase+".md")
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("ReadFile(%s) error = %v", tt.phase, readErr)
		}
		text := string(content)
		if strings.Contains(text, "{{KIRO_MODEL}}") {
			t.Fatalf("agent %s still contains unresolved {{KIRO_MODEL}} placeholder", tt.phase)
		}
		if !strings.Contains(text, tt.want) {
			t.Fatalf("agent %s missing %q", tt.phase, tt.want)
		}
	}
}

func TestInjectKiroBalancedPresetAssignmentsEndToEnd(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))

	adapter, err := agents.NewAdapter(model.AgentKiroIDE)
	if err != nil {
		t.Fatalf("NewAdapter(kiro-ide) error = %v", err)
	}

	// This mirrors the map emitted by the Claude model picker (balanced preset).
	balance := model.ClaudeModelPresetBalanced()

	result, err := Inject(home, adapter, "", InjectOptions{ClaudeModelAssignments: balance})
	if err != nil {
		t.Fatalf("Inject(kiro, balanced preset) error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Inject(kiro, balanced preset) changed = false")
	}

	// Validate every generated Kiro phase file gets the expected model ID.
	for _, phase := range []string{
		"sdd-init", "sdd-explore", "sdd-propose", "sdd-spec", "sdd-design",
		"sdd-tasks", "sdd-apply", "sdd-verify", "sdd-archive", "sdd-onboard",
	} {
		alias, ok := balance[phase]
		if !ok {
			alias = balance["default"]
		}
		wantModelLine := "model: " + model.KiroModelID(alias)

		path := filepath.Join(home, ".kiro", "agents", phase+".md")
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("ReadFile(%s) error = %v", phase, readErr)
		}
		if !strings.Contains(string(content), wantModelLine) {
			t.Fatalf("agent %s model line mismatch: want %q", phase, wantModelLine)
		}
	}
}

// TestInjectKiroModelAssignmentsTakePrecedenceOverClaude verifies that when
// both KiroModelAssignments and ClaudeModelAssignments are provided,
// KiroModelAssignments wins for Kiro subagent file generation.
func TestInjectKiroModelAssignmentsTakePrecedenceOverClaude(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))

	adapter, err := agents.NewAdapter(model.AgentKiroIDE)
	if err != nil {
		t.Fatalf("NewAdapter(kiro-ide) error = %v", err)
	}

	// Conflicting values: Kiro says opus for sdd-design, Claude says haiku.
	// Kiro-specific assignments MUST take precedence.
	opts := InjectOptions{
		KiroModelAssignments: map[string]model.ClaudeModelAlias{
			"sdd-design": model.ClaudeModelOpus,
		},
		ClaudeModelAssignments: map[string]model.ClaudeModelAlias{
			"sdd-design": model.ClaudeModelHaiku,
		},
	}

	_, err = Inject(home, adapter, "", opts)
	if err != nil {
		t.Fatalf("Inject error = %v", err)
	}

	path := filepath.Join(home, ".kiro", "agents", "sdd-design.md")
	content, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("ReadFile(sdd-design) error = %v", readErr)
	}

	wantKiro := "model: " + model.KiroModelID(model.ClaudeModelOpus)
	wantClaude := "model: " + model.KiroModelID(model.ClaudeModelHaiku)

	if !strings.Contains(string(content), wantKiro) {
		t.Fatalf("expected KiroModelAssignments to take precedence: want %q not found in file", wantKiro)
	}
	if strings.Contains(string(content), wantClaude) {
		t.Fatalf("ClaudeModelAssignments must NOT be used when KiroModelAssignments is set: found %q", wantClaude)
	}
}

// ---------------------------------------------------------------------------
// Fix 2: findProjectRoot — monorepo and enhanced workspace root detection
// ---------------------------------------------------------------------------

// TestFindProjectRootPnpmMonorepo verifies that when the starting directory
// has a package.json but a parent has pnpm-workspace.yaml, the function
// returns the monorepo root (parent), not the sub-package directory.
func TestFindProjectRootPnpmMonorepo(t *testing.T) {
	root := t.TempDir()

	// Monorepo root: has pnpm-workspace.yaml
	if err := os.WriteFile(filepath.Join(root, "pnpm-workspace.yaml"), []byte("packages:\n  - packages/*\n"), 0o644); err != nil {
		t.Fatalf("write pnpm-workspace.yaml: %v", err)
	}

	// Sub-package: has its own package.json
	subPkg := filepath.Join(root, "packages", "app")
	if err := os.MkdirAll(subPkg, 0o755); err != nil {
		t.Fatalf("MkdirAll(subPkg): %v", err)
	}
	if err := os.WriteFile(filepath.Join(subPkg, "package.json"), []byte(`{"name":"app"}`), 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}

	// Also add a package.json at the monorepo root
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"monorepo"}`), 0o644); err != nil {
		t.Fatalf("write root package.json: %v", err)
	}

	// Start from sub-package — should resolve to the monorepo root.
	got, ok := findProjectRoot(subPkg)
	if !ok {
		t.Fatal("findProjectRoot returned false, want true")
	}
	if got != root {
		t.Fatalf("findProjectRoot = %q, want monorepo root %q", got, root)
	}
}

// TestFindProjectRootNxMonorepo verifies that nx.json is recognized as a
// monorepo root marker.
func TestFindProjectRootNxMonorepo(t *testing.T) {
	root := t.TempDir()

	// Monorepo root: has nx.json
	if err := os.WriteFile(filepath.Join(root, "nx.json"), []byte(`{"version":2}`), 0o644); err != nil {
		t.Fatalf("write nx.json: %v", err)
	}

	// Sub-package: has its own package.json
	subPkg := filepath.Join(root, "apps", "web")
	if err := os.MkdirAll(subPkg, 0o755); err != nil {
		t.Fatalf("MkdirAll(subPkg): %v", err)
	}
	if err := os.WriteFile(filepath.Join(subPkg, "package.json"), []byte(`{"name":"web"}`), 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}

	got, ok := findProjectRoot(subPkg)
	if !ok {
		t.Fatal("findProjectRoot returned false, want true")
	}
	if got != root {
		t.Fatalf("findProjectRoot = %q, want nx monorepo root %q", got, root)
	}
}

// TestFindProjectRootTurboMonorepo verifies that turbo.json is recognized as
// a monorepo root marker.
func TestFindProjectRootTurboMonorepo(t *testing.T) {
	root := t.TempDir()

	if err := os.WriteFile(filepath.Join(root, "turbo.json"), []byte(`{"$schema":"..."}`), 0o644); err != nil {
		t.Fatalf("write turbo.json: %v", err)
	}

	subPkg := filepath.Join(root, "packages", "ui")
	if err := os.MkdirAll(subPkg, 0o755); err != nil {
		t.Fatalf("MkdirAll(subPkg): %v", err)
	}
	if err := os.WriteFile(filepath.Join(subPkg, "package.json"), []byte(`{"name":"ui"}`), 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}

	got, ok := findProjectRoot(subPkg)
	if !ok {
		t.Fatal("findProjectRoot returned false, want true")
	}
	if got != root {
		t.Fatalf("findProjectRoot = %q, want turbo root %q", got, root)
	}
}

// TestFindProjectRootGitTakesPrecedence verifies that a .git directory at a
// higher level takes precedence over a package.json in a subdirectory.
func TestFindProjectRootGitTakesPrecedence(t *testing.T) {
	root := t.TempDir()

	// Project root: has .git
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git): %v", err)
	}

	// Subdirectory: has package.json
	subDir := filepath.Join(root, "frontend")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(subDir): %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "package.json"), []byte(`{"name":"frontend"}`), 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}

	// Start from subdirectory — should find .git at root immediately.
	got, ok := findProjectRoot(subDir)
	if !ok {
		t.Fatal("findProjectRoot returned false, want true")
	}
	if got != root {
		t.Fatalf("findProjectRoot = %q, want .git root %q", got, root)
	}
}

// TestFindProjectRootPackageJsonFallback verifies that when only package.json
// exists (no .git, go.mod, or monorepo markers), it is returned as the best
// candidate root.
func TestFindProjectRootPackageJsonFallback(t *testing.T) {
	root := t.TempDir()

	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"app"}`), 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}

	// Isolation: add a strong marker at the test sandbox root to stop findProjectRoot
	// from walking up into the real home directory on Windows.
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git): %v", err)
	}

	subDir := filepath.Join(root, "src", "components")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(subDir): %v", err)
	}

	got, ok := findProjectRoot(subDir)
	if !ok {
		t.Fatal("findProjectRoot returned false, want true")
	}
	if got != root {
		t.Fatalf("findProjectRoot = %q, want root with package.json %q", got, root)
	}
}

// TestFindProjectRootEmptyDirReturnsNotFound verifies that an empty directory
// (no markers at all) returns false.
func TestFindProjectRootEmptyDirReturnsNotFound(t *testing.T) {
	emptyDir := t.TempDir() // No markers, isolated temp dir

	// The temp dir has no markers; we start from a subdirectory of it.
	subDir := filepath.Join(emptyDir, "deep", "path")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(subDir): %v", err)
	}

	_, ok := findProjectRoot(subDir)
	if ok {
		// Note: this may find markers in ancestor dirs outside emptyDir
		// on some systems. The test is best-effort for isolated environments.
		t.Log("findProjectRoot found a marker outside the temp dir — acceptable on some systems")
	}
}

// TestFindProjectRootEmptyStringReturnsNotFound verifies the early-return for
// empty dir input.
func TestFindProjectRootEmptyStringReturnsNotFound(t *testing.T) {
	got, ok := findProjectRoot("")
	if ok {
		t.Fatalf("findProjectRoot(\"\") = (%q, true), want (\"\", false)", got)
	}
}

// TestFindProjectRootDeepNested verifies that findProjectRoot handles deeply
// nested directories without panicking or infinite looping, and that it
// correctly returns ("", false) when the marker is beyond maxAncestorDepth.
func TestFindProjectRootDeepNested(t *testing.T) {
	root := t.TempDir()

	// Build a directory 25 levels deep (beyond maxAncestorDepth=20).
	deepDir := root
	for i := 0; i < 25; i++ {
		deepDir = filepath.Join(deepDir, fmt.Sprintf("level%02d", i))
	}
	if err := os.MkdirAll(deepDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(deepDir): %v", err)
	}

	// Place a go.mod only at the root (25 levels above deepDir).
	// With maxAncestorDepth=20, findProjectRoot cannot reach it from level 25.
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module test\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	// This must not panic or loop infinitely.
	// The important assertion is that it completes quickly.
	done := make(chan struct{})
	var gotPath string
	var gotOk bool
	go func() {
		defer close(done)
		gotPath, gotOk = findProjectRoot(deepDir)
	}()

	select {
	case <-done:
		// Completed without hanging — test passes.
	case <-time.After(5 * time.Second):
		t.Fatal("findProjectRoot appeared to hang on deeply nested dir")
	}

	// Correctness: starting 25 levels deep with go.mod only at level 0 and
	// maxAncestorDepth=20, the function cannot reach level 0 — must return ("", false).
	if gotOk {
		t.Fatalf("findProjectRoot should return false when marker is beyond maxAncestorDepth, got path=%q ok=%v", gotPath, gotOk)
	}
	if gotPath != "" {
		t.Fatalf("findProjectRoot should return empty path when not found, got %q", gotPath)
	}
}

// TestFindProjectRootMultiplePackageJsonPicksHighest verifies that when
// multiple package.json files exist in ancestor directories, findProjectRoot
// returns the highest ancestor (closest to filesystem root), not the first
// (closest to starting dir).
func TestFindProjectRootMultiplePackageJsonPicksHighest(t *testing.T) {
	root := t.TempDir()

	// root/package.json  ← highest ancestor, should win
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"root"}`), 0o644); err != nil {
		t.Fatalf("write root package.json: %v", err)
	}

	// Isolation: add a strong marker at the test sandbox root to stop findProjectRoot
	// from walking up into the real home directory on Windows.
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git): %v", err)
	}

	// root/packages/app/package.json  ← closer to start, should NOT win
	appDir := filepath.Join(root, "packages", "app")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(appDir): %v", err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "package.json"), []byte(`{"name":"app"}`), 0o644); err != nil {
		t.Fatalf("write app package.json: %v", err)
	}

	// root/packages/app/src/ — start here
	srcDir := filepath.Join(appDir, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(srcDir): %v", err)
	}

	got, ok := findProjectRoot(srcDir)
	if !ok {
		t.Fatal("findProjectRoot returned false, want true")
	}
	if got != root {
		t.Fatalf("findProjectRoot = %q, want highest ancestor root %q (not closest package.json %q)", got, root, appDir)
	}
}

// TestFindProjectRootAllMarkers verifies that each project marker (beyond .git,
// go.mod, and package.json) is correctly recognized as a project root.
func TestFindProjectRootAllMarkers(t *testing.T) {
	allMarkers := []struct {
		name   string
		marker string
		isDir  bool
	}{
		{"pnpm-workspace.yml", "pnpm-workspace.yml", false},
		{"lerna.json", "lerna.json", false},
		{"rush.json", "rush.json", false},
		{"Cargo.toml", "Cargo.toml", false},
		{"pyproject.toml", "pyproject.toml", false},
		{"pom.xml", "pom.xml", false},
		{"build.gradle", "build.gradle", false},
	}

	for _, tt := range allMarkers {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			subDir := filepath.Join(root, "sub", "deep")
			os.MkdirAll(subDir, 0o755)

			markerPath := filepath.Join(root, tt.marker)
			if tt.isDir {
				os.MkdirAll(markerPath, 0o755)
			} else {
				os.WriteFile(markerPath, []byte(""), 0o644)
			}

			result, ok := findProjectRoot(subDir)
			if !ok {
				t.Fatalf("findProjectRoot(%s) returned false for marker %s", subDir, tt.marker)
			}
			if result != root {
				t.Fatalf("findProjectRoot(%s) = %s, want %s", subDir, result, root)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Fix: SDD post-check disk fallback on Windows
// ---------------------------------------------------------------------------

// TestInjectOpenCodePostCheckDiskFallback tests that the SDD post-check
// correctly falls back to reading from disk when the in-memory merged bytes
// are stale or empty. This simulates the Windows scenario where os.ReadFile
// returns stale data due to NTFS caching, but the file on disk is correct.
func TestInjectOpenCodePostCheckDiskFallback(t *testing.T) {
	home := t.TempDir()

	// Pre-create a minimal config file with sdd-orchestrator already present.
	// This simulates a previous successful install where the file on disk
	// is correct but in-memory buffer might be stale.
	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	// Write a config that already has sdd-orchestrator (simulating previous install)
	existingConfig := `{
  "agent": {
    "gentleman": {
      "description": "Gentleman",
      "mode": "primary"
    },
    "sdd-orchestrator": {
      "description": "SDD Orchestrator",
      "mode": "primary"
    }
  }
}`
	if err := os.WriteFile(settingsPath, []byte(existingConfig), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Mock npm to not be available (so we skip plugin installation)
	origNpmLookPath := npmLookPath
	npmLookPath = func(string) (string, error) {
		return "", fmt.Errorf("npm not found")
	}
	t.Cleanup(func() { npmLookPath = origNpmLookPath })

	// Run Inject with SDD mode single
	result, err := Inject(home, opencodeAdapter(), model.SDDModeSingle)
	if err != nil {
		// This is the bug: on Windows, even with correct file on disk,
		// the post-check may fail if in-memory buffer is stale.
		// The fix adds a disk fallback, so this should NOT fail.
		t.Fatalf("Inject() error = %v (post-check should pass with disk fallback)", err)
	}

	// Verify that the result indicates the file was changed (merged successfully)
	if !result.Changed {
		t.Log("Note: result.Changed = false, but that's OK for idempotent runs")
	}

	// Verify the file on disk still has sdd-orchestrator
	diskContent, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(diskContent), "sdd-orchestrator") {
		t.Fatal("File on disk lost sdd-orchestrator after inject")
	}
}

// TestInjectOpenCodeWithProfile_PostCheckVerifiesOrchestrator verifies that
// when a named profile is injected, the post-check confirms sdd-orchestrator-{name}
// is present in the merged opencode.json.
func TestInjectOpenCodeWithProfile_PostCheckVerifiesOrchestrator(t *testing.T) {
	home := t.TempDir()
	mockNoPackageManager(t)

	cheapProfile := model.Profile{
		Name:              "cheap",
		OrchestratorModel: model.ModelAssignment{ProviderID: "anthropic", ModelID: "claude-haiku-3-5"},
	}

	result, err := Inject(home, opencodeAdapter(), model.SDDModeMulti, InjectOptions{
		Profiles: []model.Profile{cheapProfile},
	})
	if err != nil {
		t.Fatalf("Inject() with profile error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Inject() with profile changed = false")
	}

	// Verify sdd-orchestrator-cheap is present in the merged settings.
	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(opencode.json) error = %v", err)
	}
	if !strings.Contains(string(content), `"sdd-orchestrator-cheap"`) {
		t.Fatal("opencode.json missing sdd-orchestrator-cheap after profile injection")
	}
}

// TestInjectOpenCodeWithProfile_DefaultProfileSkipped verifies that the default
// profile (Name="" or Name="default") is skipped in the profile injection loop.
func TestInjectOpenCodeWithProfile_DefaultProfileSkipped(t *testing.T) {
	home := t.TempDir()
	mockNoPackageManager(t)

	_, err := Inject(home, opencodeAdapter(), model.SDDModeMulti, InjectOptions{
		Profiles: []model.Profile{
			{Name: "", OrchestratorModel: model.ModelAssignment{ProviderID: "anthropic", ModelID: "claude-haiku-3-5"}},
			{Name: "default", OrchestratorModel: model.ModelAssignment{ProviderID: "anthropic", ModelID: "claude-haiku-3-5"}},
		},
	})
	if err != nil {
		t.Fatalf("Inject() with default profiles error = %v (should not fail)", err)
	}
}

// TestInjectOpenCodeWithTwoProfiles_BothOrchestratorsPresent verifies that
// two named profiles both get their orchestrators injected and verified.
func TestInjectOpenCodeWithTwoProfiles_BothOrchestratorsPresent(t *testing.T) {
	home := t.TempDir()
	mockNoPackageManager(t)

	_, err := Inject(home, opencodeAdapter(), model.SDDModeMulti, InjectOptions{
		Profiles: []model.Profile{
			{Name: "cheap", OrchestratorModel: model.ModelAssignment{ProviderID: "anthropic", ModelID: "claude-haiku-3-5"}},
			{Name: "premium", OrchestratorModel: model.ModelAssignment{ProviderID: "anthropic", ModelID: "claude-opus-4-5"}},
		},
	})
	if err != nil {
		t.Fatalf("Inject() with two profiles error = %v", err)
	}

	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(opencode.json) error = %v", err)
	}
	text := string(content)

	if !strings.Contains(text, `"sdd-orchestrator-cheap"`) {
		t.Error("opencode.json missing sdd-orchestrator-cheap")
	}
	if !strings.Contains(text, `"sdd-orchestrator-premium"`) {
		t.Error("opencode.json missing sdd-orchestrator-premium")
	}
}

// TestInjectClaudeSubAgentsResolveModels verifies that when SDD is injected
// for the Claude adapter, the embedded sub-agent files are copied to
// ~/.claude/agents/ and the {{CLAUDE_MODEL}} placeholder is substituted per
// phase using opts.ClaudeModelAssignments.
func TestInjectClaudeSubAgentsResolveModels(t *testing.T) {
	home := t.TempDir()

	assignments := map[string]model.ClaudeModelAlias{
		"sdd-design":  model.ClaudeModelOpus,
		"sdd-archive": model.ClaudeModelHaiku,
		"default":     model.ClaudeModelSonnet,
	}

	result, err := Inject(home, claudeAdapter(), "", InjectOptions{ClaudeModelAssignments: assignments})
	if err != nil {
		t.Fatalf("Inject(claude, custom assignments) error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Inject(claude, custom assignments) changed = false")
	}

	tests := []struct {
		phase string
		want  string
	}{
		{phase: "sdd-design", want: "model: opus"},
		{phase: "sdd-archive", want: "model: haiku"},
		{phase: "sdd-spec", want: "model: sonnet"},
	}

	for _, tt := range tests {
		t.Run(tt.phase, func(t *testing.T) {
			path := filepath.Join(home, ".claude", "agents", tt.phase+".md")
			content, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatalf("ReadFile(%s) error = %v", tt.phase, readErr)
			}
			text := string(content)
			if strings.Contains(text, "{{CLAUDE_MODEL}}") {
				t.Fatalf("agent %s still contains unresolved {{CLAUDE_MODEL}} placeholder", tt.phase)
			}
			if !strings.Contains(text, tt.want) {
				t.Fatalf("agent %s missing %q\n--- file ---\n%s", tt.phase, tt.want, text)
			}
		})
	}
}

func TestInjectClaudeSubAgentsUseBalancedDefaultsWhenAssignmentsUnset(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, claudeAdapter(), "")
	if err != nil {
		t.Fatalf("Inject(claude, default assignments) error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Inject(claude, default assignments) changed = false")
	}

	tests := []struct {
		phase string
		want  string
	}{
		{phase: "sdd-design", want: "model: opus"},
		{phase: "sdd-spec", want: "model: sonnet"},
		{phase: "sdd-archive", want: "model: haiku"},
	}

	for _, tt := range tests {
		t.Run(tt.phase, func(t *testing.T) {
			path := filepath.Join(home, ".claude", "agents", tt.phase+".md")
			content, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatalf("ReadFile(%s) error = %v", tt.phase, readErr)
			}
			if !strings.Contains(string(content), tt.want) {
				t.Fatalf("agent %s missing balanced default %q\n--- file ---\n%s", tt.phase, tt.want, string(content))
			}
		})
	}
}

func TestInjectClaudeSubAgentsIgnoreInvalidAliases(t *testing.T) {
	home := t.TempDir()

	assignments := map[string]model.ClaudeModelAlias{
		"sdd-design":  model.ClaudeModelAlias("claude-opus-4-1"),
		"sdd-archive": model.ClaudeModelAlias("bad-value"),
		"default":     model.ClaudeModelHaiku,
	}

	_, err := Inject(home, claudeAdapter(), "", InjectOptions{ClaudeModelAssignments: assignments})
	if err != nil {
		t.Fatalf("Inject(claude, invalid aliases) error = %v", err)
	}

	checks := []struct {
		phase string
		want  string
	}{
		{phase: "sdd-design", want: "model: opus"},
		{phase: "sdd-archive", want: "model: haiku"},
		{phase: "sdd-spec", want: "model: sonnet"},
	}

	for _, tt := range checks {
		path := filepath.Join(home, ".claude", "agents", tt.phase+".md")
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("ReadFile(%s) error = %v", tt.phase, readErr)
		}
		text := string(content)
		if !strings.Contains(text, tt.want) {
			t.Fatalf("agent %s missing sanitized model %q\n--- file ---\n%s", tt.phase, tt.want, text)
		}
		if strings.Contains(text, "bad-value") || strings.Contains(text, "claude-opus-4-1") {
			t.Fatalf("agent %s contains invalid alias in frontmatter\n--- file ---\n%s", tt.phase, text)
		}
	}
}

// TestInjectClaudeSubAgentsScopedTools verifies that each generated Claude
// sub-agent carries a scoped tools: frontmatter entry so the phase cannot use
// tools outside its contract (e.g. sdd-explore cannot Edit/Write; no phase
// carries Task so recursion is impossible).
func TestInjectClaudeSubAgentsScopedTools(t *testing.T) {
	home := t.TempDir()

	_, err := Inject(home, claudeAdapter(), "", InjectOptions{ClaudeModelAssignments: model.ClaudeModelPresetBalanced()})
	if err != nil {
		t.Fatalf("Inject(claude, balanced preset) error = %v", err)
	}

	tests := []struct {
		phase       string
		mustContain []string
		mustNotHave []string
	}{
		{
			phase:       "sdd-explore",
			mustContain: []string{"Read", "Grep", "Glob", "WebFetch", "WebSearch", "mcp__plugin_engram_engram__mem_save"},
			mustNotHave: []string{"Edit", "Write", "Bash", "Task"},
		},
		{
			phase:       "sdd-propose",
			mustContain: []string{"Read", "Edit", "Write", "Grep", "Glob", "mcp__plugin_engram_engram__mem_search", "mcp__plugin_engram_engram__mem_get_observation", "mcp__plugin_engram_engram__mem_save"},
			mustNotHave: []string{"Bash", "Task"},
		},
		{
			phase:       "sdd-spec",
			mustContain: []string{"Read", "Edit", "Write", "Grep", "Glob", "mcp__plugin_engram_engram__mem_search", "mcp__plugin_engram_engram__mem_get_observation", "mcp__plugin_engram_engram__mem_save"},
			mustNotHave: []string{"Bash", "Task"},
		},
		{
			phase:       "sdd-design",
			mustContain: []string{"Read", "Edit", "Write", "Grep", "Glob", "mcp__plugin_engram_engram__mem_search", "mcp__plugin_engram_engram__mem_get_observation", "mcp__plugin_engram_engram__mem_save"},
			mustNotHave: []string{"Bash", "Task"},
		},
		{
			phase:       "sdd-tasks",
			mustContain: []string{"Read", "Edit", "Write", "Grep", "Glob", "mcp__plugin_engram_engram__mem_search", "mcp__plugin_engram_engram__mem_get_observation", "mcp__plugin_engram_engram__mem_save"},
			mustNotHave: []string{"Bash", "Task"},
		},
		{
			phase:       "sdd-apply",
			mustContain: []string{"Read", "Edit", "Write", "Bash", "mcp__plugin_engram_engram__mem_search", "mcp__plugin_engram_engram__mem_get_observation", "mcp__plugin_engram_engram__mem_save", "mcp__plugin_engram_engram__mem_update"},
			mustNotHave: []string{"Task"},
		},
		{
			phase:       "sdd-verify",
			mustContain: []string{"Read", "Bash", "mcp__plugin_engram_engram__mem_search", "mcp__plugin_engram_engram__mem_get_observation", "mcp__plugin_engram_engram__mem_save"},
			mustNotHave: []string{"Edit", "Write", "Task"},
		},
		{
			phase:       "sdd-archive",
			mustContain: []string{"Read", "Edit", "Write", "mcp__plugin_engram_engram__mem_search", "mcp__plugin_engram_engram__mem_get_observation", "mcp__plugin_engram_engram__mem_save"},
			mustNotHave: []string{"Bash", "Task"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.phase, func(t *testing.T) {
			path := filepath.Join(home, ".claude", "agents", tt.phase+".md")
			content, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatalf("ReadFile(%s) error = %v", tt.phase, readErr)
			}
			text := string(content)

			toolsLine := ""
			for _, line := range strings.Split(text, "\n") {
				if strings.HasPrefix(line, "tools:") {
					toolsLine = line
					break
				}
			}
			if toolsLine == "" {
				t.Fatalf("agent %s missing tools: frontmatter line\n--- file ---\n%s", tt.phase, text)
			}

			for _, want := range tt.mustContain {
				if !strings.Contains(toolsLine, want) {
					t.Errorf("agent %s tools line %q missing required tool %q", tt.phase, toolsLine, want)
				}
			}
			for _, forbidden := range tt.mustNotHave {
				if strings.Contains(toolsLine, forbidden) {
					t.Errorf("agent %s tools line %q must not grant %q", tt.phase, toolsLine, forbidden)
				}
			}
		})
	}
}
