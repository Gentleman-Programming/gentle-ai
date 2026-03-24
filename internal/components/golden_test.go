package components_test

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/agents"
	"github.com/gentleman-programming/gentle-ai/internal/agents/antigravity"
	"github.com/gentleman-programming/gentle-ai/internal/agents/claude"
	codexagent "github.com/gentleman-programming/gentle-ai/internal/agents/codex"
	"github.com/gentleman-programming/gentle-ai/internal/agents/cursor"
	"github.com/gentleman-programming/gentle-ai/internal/agents/gemini"
	"github.com/gentleman-programming/gentle-ai/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/internal/agents/vscode"
	"github.com/gentleman-programming/gentle-ai/internal/assets"
	"github.com/gentleman-programming/gentle-ai/internal/components/engram"
	"github.com/gentleman-programming/gentle-ai/internal/components/mcp"
	"github.com/gentleman-programming/gentle-ai/internal/components/persona"
	"github.com/gentleman-programming/gentle-ai/internal/components/sdd"
	"github.com/gentleman-programming/gentle-ai/internal/components/skills"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

var update = flag.Bool("update", false, "update golden files")

func claudeAdapter() agents.Adapter      { return claude.NewAdapter() }
func opencodeAdapter() agents.Adapter    { return opencode.NewAdapter() }
func cursorAdapter() agents.Adapter      { return cursor.NewAdapter() }
func geminiAdapter() agents.Adapter      { return gemini.NewAdapter() }
func vscodeAdapter() agents.Adapter      { return vscode.NewAdapter() }
func codexAdapter() agents.Adapter       { return codexagent.NewAdapter() }
func antigravityAdapter() agents.Adapter { return antigravity.NewAdapter() }

// ---------------------------------------------------------------------------
// Existing golden tests (context7, presets, SDD command)
// ---------------------------------------------------------------------------

func TestGoldenConfigs(t *testing.T) {
	type presetMapping struct {
		Preset string   `json:"preset"`
		Skills []string `json:"skills"`
	}

	presets := []presetMapping{
		{Preset: "full-gentleman", Skills: toStringSlice(skills.SkillsForPreset("full-gentleman"))},
		{Preset: "ecosystem-only", Skills: toStringSlice(skills.SkillsForPreset("ecosystem-only"))},
		{Preset: "minimal", Skills: toStringSlice(skills.SkillsForPreset("minimal"))},
	}
	presetsJSON, err := json.MarshalIndent(presets, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	presetsJSON = append(presetsJSON, '\n')

	commands := sdd.OpenCodeCommands()
	if len(commands) == 0 {
		t.Fatalf("OpenCodeCommands() returned no commands")
	}
	commandMarkdown := []byte("# " + commands[0].Name + "\n\n" + commands[0].Description + "\n\n" + commands[0].Body + "\n")

	tests := []struct {
		name    string
		path    string
		content []byte
	}{
		{name: "context7 server", path: "context7-server.json", content: mcp.DefaultContext7ServerJSON()},
		{name: "context7 overlay", path: "context7-overlay.json", content: mcp.DefaultContext7OverlayJSON()},
		{name: "skills presets", path: "skills-presets.json", content: presetsJSON},
		{name: "sdd command markdown", path: "sdd-command-sdd-init.md", content: commandMarkdown},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertGolden(t, tc.path, tc.content)
		})
	}
}

// ---------------------------------------------------------------------------
// SDD Injector golden tests
// ---------------------------------------------------------------------------

func TestGoldenSDD_Claude(t *testing.T) {
	home := t.TempDir()

	result, err := sdd.Inject(home, claudeAdapter(), "")
	if err != nil {
		t.Fatalf("sdd.Inject(claude) error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("sdd.Inject(claude) changed = false")
	}

	claudeMD := readTestFile(t, filepath.Join(home, ".claude", "CLAUDE.md"))
	assertGolden(t, "sdd-claude-claudemd.golden", claudeMD)
}

func TestGoldenSDD_OpenCode(t *testing.T) {
	home := t.TempDir()

	result, err := sdd.Inject(home, opencodeAdapter(), "")
	if err != nil {
		t.Fatalf("sdd.Inject(opencode) error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("sdd.Inject(opencode) changed = false")
	}

	// Golden-check a representative command file.
	sddInit := readTestFile(t, filepath.Join(home, ".config", "opencode", "commands", "sdd-init.md"))
	assertGolden(t, "sdd-opencode-cmd-sdd-init.golden", sddInit)

	// Golden-check a representative SDD skill file.
	skillInit := readTestFile(t, filepath.Join(home, ".config", "opencode", "skills", "sdd-init", "SKILL.md"))
	assertGolden(t, "sdd-opencode-skill-sdd-init.golden", skillInit)

	// Verify ALL expected command files exist.
	expectedCommands := []string{
		"sdd-init.md", "sdd-apply.md", "sdd-archive.md", "sdd-continue.md",
		"sdd-explore.md", "sdd-ff.md", "sdd-new.md", "sdd-verify.md",
	}
	commandsDir := filepath.Join(home, ".config", "opencode", "commands")
	for _, name := range expectedCommands {
		path := filepath.Join(commandsDir, name)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected command file %q not found: %v", name, err)
		}
	}
}

func TestGoldenSDD_OpenCode_Multi(t *testing.T) {
	home := t.TempDir()

	result, err := sdd.Inject(home, opencodeAdapter(), "multi")
	if err != nil {
		t.Fatalf("sdd.Inject(opencode, multi) error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("sdd.Inject(opencode, multi) changed = false")
	}

	// Golden-check the settings file with multi overlay merged.
	settingsJSON := readTestFile(t, filepath.Join(home, ".config", "opencode", "opencode.json"))
	for _, toolName := range []string{"\"delegate\"", "\"delegation_read\"", "\"delegation_list\""} {
		if !strings.Contains(string(settingsJSON), toolName) {
			t.Fatalf("multi-mode settings missing orchestrator tool %s", toolName)
		}
	}
	assertGolden(t, "sdd-opencode-multi-settings.golden", settingsJSON)

	pluginPath := filepath.Join(home, ".config", "opencode", "plugins", "background-agents.ts")
	pluginContent := readTestFile(t, pluginPath)
	if string(pluginContent) != assets.MustRead("opencode/plugins/background-agents.ts") {
		t.Fatalf("plugin content mismatch for %q", pluginPath)
	}
}

func TestGoldenSDD_Cursor(t *testing.T) {
	home := t.TempDir()

	result, err := sdd.Inject(home, cursorAdapter(), "")
	if err != nil {
		t.Fatalf("sdd.Inject(cursor) error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("sdd.Inject(cursor) changed = false")
	}

	// Cursor writes SDD orchestrator to ~/.cursor/rules/gentle-ai.mdc.
	rulesFile := readTestFile(t, filepath.Join(home, ".cursor", "rules", "gentle-ai.mdc"))
	assertGolden(t, "sdd-cursor-rules.golden", rulesFile)

	// Golden-check a representative SDD skill file.
	skillInit := readTestFile(t, filepath.Join(home, ".cursor", "skills", "sdd-init", "SKILL.md"))
	assertGolden(t, "sdd-cursor-skill-sdd-init.golden", skillInit)

	// Verify ALL expected SDD skill files exist.
	expectedSkills := []string{
		"sdd-init", "sdd-apply", "sdd-archive", "sdd-explore",
		"sdd-propose", "sdd-spec", "sdd-design", "sdd-tasks", "sdd-verify",
	}
	skillsDir := filepath.Join(home, ".cursor", "skills")
	for _, name := range expectedSkills {
		path := filepath.Join(skillsDir, name, "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected SDD skill file %q not found: %v", name, err)
		}
	}
}

func TestGoldenSDD_Gemini(t *testing.T) {
	home := t.TempDir()

	result, err := sdd.Inject(home, geminiAdapter(), "")
	if err != nil {
		t.Fatalf("sdd.Inject(gemini) error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("sdd.Inject(gemini) changed = false")
	}

	// Gemini writes SDD orchestrator to ~/.gemini/GEMINI.md.
	geminiMD := readTestFile(t, filepath.Join(home, ".gemini", "GEMINI.md"))
	assertGolden(t, "sdd-gemini-geminimd.golden", geminiMD)

	// Golden-check a representative SDD skill file.
	skillInit := readTestFile(t, filepath.Join(home, ".gemini", "skills", "sdd-init", "SKILL.md"))
	assertGolden(t, "sdd-gemini-skill-sdd-init.golden", skillInit)

	// Verify ALL expected SDD skill files exist.
	expectedSkills := []string{
		"sdd-init", "sdd-apply", "sdd-archive", "sdd-explore",
		"sdd-propose", "sdd-spec", "sdd-design", "sdd-tasks", "sdd-verify",
	}
	skillsDir := filepath.Join(home, ".gemini", "skills")
	for _, name := range expectedSkills {
		path := filepath.Join(skillsDir, name, "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected SDD skill file %q not found: %v", name, err)
		}
	}
}

func TestGoldenSDD_VSCode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	adapter := vscodeAdapter()

	result, err := sdd.Inject(home, adapter, "")
	if err != nil {
		t.Fatalf("sdd.Inject(vscode) error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("sdd.Inject(vscode) changed = false")
	}

	// VS Code writes to a platform-specific path — use the adapter to resolve it.
	promptPath := adapter.SystemPromptFile(home)
	instructionsFile := readTestFile(t, promptPath)
	assertGolden(t, "sdd-vscode-instructions.golden", instructionsFile)

	// Golden-check a representative SDD skill file.
	skillInit := readTestFile(t, filepath.Join(home, ".copilot", "skills", "sdd-init", "SKILL.md"))
	assertGolden(t, "sdd-vscode-skill-sdd-init.golden", skillInit)

	// Verify ALL expected SDD skill files exist.
	expectedSkills := []string{
		"sdd-init", "sdd-apply", "sdd-archive", "sdd-explore",
		"sdd-propose", "sdd-spec", "sdd-design", "sdd-tasks", "sdd-verify",
	}
	skillsDir := filepath.Join(home, ".copilot", "skills")
	for _, name := range expectedSkills {
		path := filepath.Join(skillsDir, name, "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected SDD skill file %q not found: %v", name, err)
		}
	}
}

func TestGoldenSDD_Codex(t *testing.T) {
	home := t.TempDir()

	result, err := sdd.Inject(home, codexAdapter(), "")
	if err != nil {
		t.Fatalf("sdd.Inject(codex) error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("sdd.Inject(codex) changed = false")
	}

	// Codex writes SDD orchestrator to ~/.codex/agents.md.
	agentsMD := readTestFile(t, filepath.Join(home, ".codex", "agents.md"))
	assertGolden(t, "sdd-codex-agentsmd.golden", agentsMD)

	// Golden-check a representative SDD skill file.
	skillInit := readTestFile(t, filepath.Join(home, ".codex", "skills", "sdd-init", "SKILL.md"))
	assertGolden(t, "sdd-codex-skill-sdd-init.golden", skillInit)

	// Verify ALL expected SDD skill files exist.
	expectedSkills := []string{
		"sdd-init", "sdd-apply", "sdd-archive", "sdd-explore",
		"sdd-propose", "sdd-spec", "sdd-design", "sdd-tasks", "sdd-verify",
	}
	skillsDir := filepath.Join(home, ".codex", "skills")
	for _, name := range expectedSkills {
		path := filepath.Join(skillsDir, name, "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected SDD skill file %q not found: %v", name, err)
		}
	}
}

// ---------------------------------------------------------------------------
// Persona Injector golden tests
// ---------------------------------------------------------------------------

func TestGoldenPersona_Claude_Gentleman(t *testing.T) {
	home := t.TempDir()

	result, err := persona.Inject(home, claudeAdapter(), model.PersonaGentleman)
	if err != nil {
		t.Fatalf("persona.Inject(claude, gentleman) error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("persona.Inject(claude, gentleman) changed = false")
	}

	claudeMD := readTestFile(t, filepath.Join(home, ".claude", "CLAUDE.md"))
	assertGolden(t, "persona-claude-gentleman.golden", claudeMD)

	outputStyle := readTestFile(t, filepath.Join(home, ".claude", "output-styles", "gentleman.md"))
	assertGolden(t, "persona-claude-gentleman-outputstyle.golden", outputStyle)

	settingsJSON := readTestFile(t, filepath.Join(home, ".claude", "settings.json"))
	assertGolden(t, "persona-claude-gentleman-settings.golden", settingsJSON)
}

func TestGoldenPersona_Claude_Neutral(t *testing.T) {
	home := t.TempDir()

	result, err := persona.Inject(home, claudeAdapter(), model.PersonaNeutral)
	if err != nil {
		t.Fatalf("persona.Inject(claude, neutral) error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("persona.Inject(claude, neutral) changed = false")
	}

	claudeMD := readTestFile(t, filepath.Join(home, ".claude", "CLAUDE.md"))
	assertGolden(t, "persona-claude-neutral.golden", claudeMD)
}

func TestGoldenPersona_OpenCode_Gentleman(t *testing.T) {
	home := t.TempDir()

	result, err := persona.Inject(home, opencodeAdapter(), model.PersonaGentleman)
	if err != nil {
		t.Fatalf("persona.Inject(opencode, gentleman) error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("persona.Inject(opencode, gentleman) changed = false")
	}

	agentsMD := readTestFile(t, filepath.Join(home, ".config", "opencode", "AGENTS.md"))
	assertGolden(t, "persona-opencode-gentleman.golden", agentsMD)
}

func TestGoldenPersona_OpenCode_Neutral(t *testing.T) {
	home := t.TempDir()

	result, err := persona.Inject(home, opencodeAdapter(), model.PersonaNeutral)
	if err != nil {
		t.Fatalf("persona.Inject(opencode, neutral) error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("persona.Inject(opencode, neutral) changed = false")
	}

	agentsMD := readTestFile(t, filepath.Join(home, ".config", "opencode", "AGENTS.md"))
	assertGolden(t, "persona-opencode-neutral.golden", agentsMD)
}

func TestGoldenPersona_Claude_Custom(t *testing.T) {
	home := t.TempDir()

	result, err := persona.Inject(home, claudeAdapter(), model.PersonaCustom)
	if err != nil {
		t.Fatalf("persona.Inject(claude, custom) error = %v", err)
	}
	// Custom persona does nothing — no files written.
	if result.Changed {
		t.Fatalf("persona.Inject(claude, custom) changed = true, want false")
	}
	if len(result.Files) != 0 {
		t.Fatalf("persona.Inject(claude, custom) returned files %v, want none", result.Files)
	}
}

func TestGoldenPersona_OpenCode_Custom(t *testing.T) {
	home := t.TempDir()

	result, err := persona.Inject(home, opencodeAdapter(), model.PersonaCustom)
	if err != nil {
		t.Fatalf("persona.Inject(opencode, custom) error = %v", err)
	}
	// Custom persona does nothing — no files written.
	if result.Changed {
		t.Fatalf("persona.Inject(opencode, custom) changed = true, want false")
	}
	if len(result.Files) != 0 {
		t.Fatalf("persona.Inject(opencode, custom) returned files %v, want none", result.Files)
	}
}

// ---------------------------------------------------------------------------
// Engram Injector golden tests
// ---------------------------------------------------------------------------

func TestGoldenEngram_Claude(t *testing.T) {
	home := t.TempDir()

	result, err := engram.Inject(home, claudeAdapter())
	if err != nil {
		t.Fatalf("engram.Inject(claude) error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("engram.Inject(claude) changed = false")
	}

	// MCP server JSON config.
	mcpJSON := readTestFile(t, filepath.Join(home, ".claude", "mcp", "engram.json"))
	assertGolden(t, "engram-claude-mcp.golden", mcpJSON)

	// CLAUDE.md with engram-protocol section.
	claudeMD := readTestFile(t, filepath.Join(home, ".claude", "CLAUDE.md"))
	assertGolden(t, "engram-claude-claudemd.golden", claudeMD)
}

func TestGoldenEngram_OpenCode(t *testing.T) {
	home := t.TempDir()

	result, err := engram.Inject(home, opencodeAdapter())
	if err != nil {
		t.Fatalf("engram.Inject(opencode) error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("engram.Inject(opencode) changed = false")
	}

	configJSON := readTestFile(t, filepath.Join(home, ".config", "opencode", "opencode.json"))
	assertGolden(t, "engram-opencode-settings.golden", configJSON)
}

// ---------------------------------------------------------------------------
// Skills Injector golden tests
// ---------------------------------------------------------------------------

func TestGoldenSkills_Claude(t *testing.T) {
	home := t.TempDir()

	skillIDs := []model.SkillID{model.SkillGoTesting, model.SkillCreator}
	result, err := skills.Inject(home, claudeAdapter(), skillIDs)
	if err != nil {
		t.Fatalf("skills.Inject(claude) error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("skills.Inject(claude) changed = false")
	}

	goTestingSkill := readTestFile(t, filepath.Join(home, ".claude", "skills", "go-testing", "SKILL.md"))
	assertGolden(t, "skills-claude-go-testing.golden", goTestingSkill)

	skillCreator := readTestFile(t, filepath.Join(home, ".claude", "skills", "skill-creator", "SKILL.md"))
	assertGolden(t, "skills-claude-skill-creator.golden", skillCreator)
}

func TestGoldenSkills_OpenCode(t *testing.T) {
	home := t.TempDir()

	skillIDs := []model.SkillID{model.SkillGoTesting, model.SkillCreator}
	result, err := skills.Inject(home, opencodeAdapter(), skillIDs)
	if err != nil {
		t.Fatalf("skills.Inject(opencode) error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("skills.Inject(opencode) changed = false")
	}

	goTestingSkill := readTestFile(t, filepath.Join(home, ".config", "opencode", "skills", "go-testing", "SKILL.md"))
	assertGolden(t, "skills-opencode-go-testing.golden", goTestingSkill)

	skillCreator := readTestFile(t, filepath.Join(home, ".config", "opencode", "skills", "skill-creator", "SKILL.md"))
	assertGolden(t, "skills-opencode-skill-creator.golden", skillCreator)
}

// ---------------------------------------------------------------------------
// Combined injection golden test (multiple components writing to same CLAUDE.md)
// ---------------------------------------------------------------------------

func TestGoldenCombined_Claude(t *testing.T) {
	home := t.TempDir()

	// Inject persona first, then SDD, then Engram — all write sections into CLAUDE.md.
	if _, err := persona.Inject(home, claudeAdapter(), model.PersonaGentleman); err != nil {
		t.Fatalf("persona.Inject error = %v", err)
	}
	if _, err := sdd.Inject(home, claudeAdapter(), ""); err != nil {
		t.Fatalf("sdd.Inject error = %v", err)
	}
	if _, err := engram.Inject(home, claudeAdapter()); err != nil {
		t.Fatalf("engram.Inject error = %v", err)
	}

	claudeMD := readTestFile(t, filepath.Join(home, ".claude", "CLAUDE.md"))
	assertGolden(t, "combined-claude-claudemd.golden", claudeMD)
}

// ---------------------------------------------------------------------------
// Antigravity golden tests
// ---------------------------------------------------------------------------

func TestGoldenSDD_Antigravity(t *testing.T) {
	home := t.TempDir()

	result, err := sdd.Inject(home, antigravityAdapter(), "")
	if err != nil {
		t.Fatalf("sdd.Inject(antigravity) error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("sdd.Inject(antigravity) changed = false")
	}

	// Antigravity writes SDD orchestrator to ~/.gemini/GEMINI.md (StrategyAppendToFile).
	rulesFile := readTestFile(t, filepath.Join(home, ".gemini", "GEMINI.md"))
	assertGolden(t, "sdd-antigravity-rulesmd.golden", rulesFile)

	// Golden-check a representative SDD skill file.
	skillInit := readTestFile(t, filepath.Join(home, ".gemini", "antigravity", "skills", "sdd-init", "SKILL.md"))
	assertGolden(t, "sdd-antigravity-skill-sdd-init.golden", skillInit)

	// Verify ALL expected SDD skill files exist.
	expectedSkills := []string{
		"sdd-init", "sdd-apply", "sdd-archive", "sdd-explore",
		"sdd-propose", "sdd-spec", "sdd-design", "sdd-tasks", "sdd-verify",
	}
	skillsDir := filepath.Join(home, ".gemini", "antigravity", "skills")
	for _, name := range expectedSkills {
		path := filepath.Join(skillsDir, name, "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected SDD skill file %q not found: %v", name, err)
		}
	}
}

func TestGoldenPersona_Antigravity_Gentleman(t *testing.T) {
	home := t.TempDir()

	result, err := persona.Inject(home, antigravityAdapter(), model.PersonaGentleman)
	if err != nil {
		t.Fatalf("persona.Inject(antigravity, gentleman) error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("persona.Inject(antigravity, gentleman) changed = false")
	}

	rulesFile := readTestFile(t, filepath.Join(home, ".gemini", "GEMINI.md"))
	assertGolden(t, "persona-antigravity-gentleman.golden", rulesFile)
}

func TestGoldenEngram_Antigravity(t *testing.T) {
	home := t.TempDir()

	result, err := engram.Inject(home, antigravityAdapter())
	if err != nil {
		t.Fatalf("engram.Inject(antigravity) error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("engram.Inject(antigravity) changed = false")
	}

	// MCP config written to ~/.gemini/antigravity/mcp_config.json.
	mcpJSON := readTestFile(t, filepath.Join(home, ".gemini", "antigravity", "mcp_config.json"))
	assertGolden(t, "engram-antigravity-mcp.golden", mcpJSON)

	// GEMINI.md must contain the engram-protocol section.
	rulesFile := readTestFile(t, filepath.Join(home, ".gemini", "GEMINI.md"))
	assertGolden(t, "engram-antigravity-rulesmd.golden", rulesFile)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func goldenDir(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "testdata", "golden")
}

func toStringSlice(ids []model.SkillID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = string(id)
	}
	return out
}

func readTestFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	return data
}

func assertGolden(t *testing.T, name string, actual []byte) {
	t.Helper()
	goldenPath := filepath.Join(goldenDir(t), name)

	if *update {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("MkdirAll for golden dir: %v", err)
		}
		if err := os.WriteFile(goldenPath, actual, 0o644); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", goldenPath, err)
		}
		t.Logf("updated golden file: %s", goldenPath)
		return
	}

	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v\n\nRun with -update to generate golden files:\n  go test ./internal/components/ -run %s -update", goldenPath, err, t.Name())
	}

	if string(actual) != string(expected) {
		// Show first difference for easier debugging.
		diffIdx := firstDiffIndex(string(expected), string(actual))
		context := 80
		start := diffIdx - context
		if start < 0 {
			start = 0
		}

		t.Fatalf("golden mismatch for %s (first diff at byte %d)\n\nexpected[%d:%d]:\n%s\n\nactual[%d:%d]:\n%s\n\nRun with -update to regenerate:\n  go test ./internal/components/ -run %s -update",
			name, diffIdx,
			start, min(diffIdx+context, len(string(expected))), string(expected)[start:min(diffIdx+context, len(string(expected)))],
			start, min(diffIdx+context, len(string(actual))), string(actual)[start:min(diffIdx+context, len(string(actual)))],
			t.Name(),
		)
	}
}

func firstDiffIndex(a, b string) int {
	maxLen := len(a)
	if len(b) < maxLen {
		maxLen = len(b)
	}
	for i := 0; i < maxLen; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	if len(a) != len(b) {
		return maxLen
	}
	return -1
}
