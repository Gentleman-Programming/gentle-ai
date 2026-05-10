package engram

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/agents"
	"github.com/gentleman-programming/gentle-ai/internal/agents/antigravity"
	"github.com/gentleman-programming/gentle-ai/internal/agents/claude"
	"github.com/gentleman-programming/gentle-ai/internal/agents/codex"
	"github.com/gentleman-programming/gentle-ai/internal/agents/gemini"
	"github.com/gentleman-programming/gentle-ai/internal/agents/openclaw"
	"github.com/gentleman-programming/gentle-ai/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/internal/agents/qwen"
	"github.com/gentleman-programming/gentle-ai/internal/agents/vscode"
)

func claudeAdapter() agents.Adapter   { return claude.NewAdapter() }
func opencodeAdapter() agents.Adapter { return opencode.NewAdapter() }
func codexAdapter() agents.Adapter    { return codex.NewAdapter() }
func geminiAdapter() agents.Adapter   { return gemini.NewAdapter() }
func qwenAdapter() agents.Adapter     { return qwen.NewAdapter() }
func openclawAdapter() agents.Adapter { return openclaw.NewAdapter() }
func antigravityAdapter() agents.Adapter {
	return antigravity.NewAdapter()
}

// assertArgsHaveToolsAgent is a shared helper that validates a JSON file
// contains the MCP "engram" entry with --tools=agent in args.
func assertArgsHaveToolsAgent(t *testing.T, path string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	text := string(content)
	if !strings.Contains(text, `"--tools=agent"`) {
		t.Fatalf("file %q missing --tools=agent in args; got:\n%s", path, text)
	}
}

func TestInjectClaudeWritesMCPConfig(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, claudeAdapter())
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("Inject() changed = false")
	}

	// Check MCP JSON file was created.
	mcpPath := filepath.Join(home, ".claude", "mcp", "engram.json")
	mcpContent, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatalf("ReadFile(engram.json) error = %v", err)
	}

	// Parse the JSON and validate the "command" key exists and references engram.
	// The command may be an absolute path (if engram is on PATH) or the relative
	// string "engram" (if not found). Both are valid.
	var parsed map[string]any
	if err := json.Unmarshal(mcpContent, &parsed); err != nil {
		t.Fatalf("Unmarshal(engram.json) error = %v", err)
	}
	cmd, ok := parsed["command"].(string)
	if !ok || cmd == "" {
		t.Fatalf("engram.json missing or empty command field; got: %s", mcpContent)
	}
	// Command must either be the literal "engram" or an absolute path ending in "engram".
	base := filepath.Base(cmd)
	if base != "engram" && base != "engram.exe" {
		t.Fatalf("engram.json command %q does not reference engram binary; got: %s", cmd, mcpContent)
	}
	if _, ok := parsed["args"]; !ok {
		t.Fatal("engram.json missing args field")
	}
	// RED: must include --tools=agent
	assertArgsHaveToolsAgent(t, mcpPath)
}

func TestInjectClaudeWritesProtocolSection(t *testing.T) {
	home := t.TempDir()

	_, err := Inject(home, claudeAdapter())
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	claudeMDPath := filepath.Join(home, ".claude", "CLAUDE.md")
	content, err := os.ReadFile(claudeMDPath)
	if err != nil {
		t.Fatalf("ReadFile(CLAUDE.md) error = %v", err)
	}

	text := string(content)
	if !strings.Contains(text, "<!-- gentle-ai:engram-protocol -->") {
		t.Fatal("CLAUDE.md missing open marker for engram-protocol")
	}
	if !strings.Contains(text, "<!-- /gentle-ai:engram-protocol -->") {
		t.Fatal("CLAUDE.md missing close marker for engram-protocol")
	}
	// Real content check.
	if !strings.Contains(text, "mem_save") {
		t.Fatal("CLAUDE.md missing real engram protocol content (expected 'mem_save')")
	}
}

func TestInjectClaudeIsIdempotent(t *testing.T) {
	home := t.TempDir()

	first, err := Inject(home, claudeAdapter())
	if err != nil {
		t.Fatalf("Inject() first error = %v", err)
	}
	if !first.Changed {
		t.Fatalf("Inject() first changed = false")
	}

	second, err := Inject(home, claudeAdapter())
	if err != nil {
		t.Fatalf("Inject() second error = %v", err)
	}
	if second.Changed {
		t.Fatalf("Inject() second changed = true")
	}
}

func TestInjectOpenCodeMergesEngramToSettings(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, opencodeAdapter())
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("Inject() changed = false")
	}

	// Should include opencode.json and AGENTS.md (fallback protocol injection).
	if len(result.Files) != 2 {
		t.Fatalf("Inject() files = %v, want exactly 2 (opencode.json + AGENTS.md)", result.Files)
	}

	configPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	config, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(opencode.json) error = %v", err)
	}

	text := string(config)
	if !strings.Contains(text, `"engram"`) {
		t.Fatal("opencode.json missing engram server entry")
	}
	if !strings.Contains(text, `"mcp"`) {
		t.Fatal("opencode.json missing mcp key")
	}
	if strings.Contains(text, `"mcpServers"`) {
		t.Fatal("opencode.json should use 'mcp' key, not 'mcpServers'")
	}
	if !strings.Contains(text, `"type": "local"`) {
		t.Fatal("opencode.json engram missing type: local")
	}
	// OpenCode 1.3.3+: command must be an array, no separate "args" field.
	if !strings.Contains(text, `"--tools=agent"`) {
		t.Fatal("opencode.json missing --tools=agent in command array")
	}
	if strings.Contains(text, `"args"`) {
		t.Fatal("opencode.json must NOT have a separate args field — command must be an array")
	}

	// Verify NO plugin files or plugin arrays exist.
	pluginPath := filepath.Join(home, ".config", "opencode", "plugins", "engram.ts")
	if _, err := os.Stat(pluginPath); err == nil {
		t.Fatal("plugin file should NOT exist — old approach removed")
	}
	if strings.Contains(text, `"plugins"`) {
		t.Fatal("opencode.json should NOT contain plugins key")
	}

	agentsPath := filepath.Join(home, ".config", "opencode", "AGENTS.md")
	agentsContent, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("ReadFile(AGENTS.md) error = %v", err)
	}
	agentsText := string(agentsContent)
	if !strings.Contains(agentsText, "<!-- gentle-ai:engram-protocol -->") {
		t.Fatal("AGENTS.md missing engram protocol section marker")
	}
	if !strings.Contains(agentsText, "mem_save") {
		t.Fatal("AGENTS.md missing engram protocol content (expected 'mem_save')")
	}
}

func TestInjectOpenCodeIsIdempotent(t *testing.T) {
	home := t.TempDir()

	first, err := Inject(home, opencodeAdapter())
	if err != nil {
		t.Fatalf("Inject() first error = %v", err)
	}
	if !first.Changed {
		t.Fatalf("Inject() first changed = false")
	}

	second, err := Inject(home, opencodeAdapter())
	if err != nil {
		t.Fatalf("Inject() second error = %v", err)
	}
	if second.Changed {
		t.Fatalf("Inject() second changed = true")
	}
}

// TestInjectOpenCodeMigratesFromOldFormat verifies that when a user's
// opencode.json contains the old v1.11.3 format (separate "args" key),
// Inject() replaces mcp.engram atomically so that "args" is absent and
// "command" is an array — the format required by OpenCode 1.3.3+.
func TestInjectOpenCodeMigratesFromOldFormat(t *testing.T) {
	home := t.TempDir()

	mockEngramLookPath(t, "/opt/homebrew/bin/engram", "")

	adapter := opencodeAdapter()
	configPath := adapter.SettingsPath(home)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}

	// Pre-seed with the old v1.11.3 format.
	oldFormat := `{"mcp": {"engram": {"command": "/opt/homebrew/bin/engram", "args": ["mcp","--tools=agent"], "type": "local"}}}`
	if err := os.WriteFile(configPath, []byte(oldFormat), 0o644); err != nil {
		t.Fatalf("WriteFile(opencode.json) error = %v", err)
	}

	result, err := Inject(home, adapter)
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("Inject() changed = false; expected migration to produce a change")
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(opencode.json) error = %v", err)
	}

	// (1) "args" key must be absent from mcp.engram.
	if strings.Contains(string(content), `"args"`) {
		t.Fatalf("mcp.engram still contains 'args' key after migration; got:\n%s", content)
	}

	// (2) command must be a []any containing the engram binary.
	var parsed map[string]any
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("Unmarshal(opencode.json) error = %v", err)
	}
	mcpMap, _ := parsed["mcp"].(map[string]any)
	engramMap, _ := mcpMap["engram"].(map[string]any)
	cmdRaw, ok := engramMap["command"]
	if !ok {
		t.Fatalf("mcp.engram missing command key; got:\n%s", content)
	}
	cmdArr, ok := cmdRaw.([]any)
	if !ok {
		t.Fatalf("mcp.engram.command must be []any after migration, got %T; got:\n%s", cmdRaw, content)
	}
	if len(cmdArr) == 0 {
		t.Fatalf("mcp.engram.command array is empty; got:\n%s", content)
	}
	firstElem, _ := cmdArr[0].(string)
	if firstElem == "" {
		t.Fatalf("mcp.engram.command[0] is empty or not a string; got:\n%s", content)
	}
	// Must end with "engram".
	if filepath.Base(firstElem) != "engram" {
		t.Fatalf("mcp.engram.command[0] = %q does not end with 'engram'; got:\n%s", firstElem, content)
	}

	// (3) Second Inject() call must be idempotent (changed=false).
	second, err := Inject(home, adapter)
	if err != nil {
		t.Fatalf("Inject() second error = %v", err)
	}
	if second.Changed {
		t.Fatalf("Inject() second changed = true; expected idempotent (no change)")
	}
}

func TestInjectOpenCodeMigratesCellarEngramCommandToStablePath(t *testing.T) {
	home := t.TempDir()

	mockEngramLookPath(t, "/opt/homebrew/bin/engram", "")

	adapter := opencodeAdapter()
	configPath := adapter.SettingsPath(home)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}

	oldFormat := `{"mcp": {"engram": {"command": ["/opt/homebrew/Cellar/engram/1.14.1/bin/engram", "mcp", "--tools=agent"], "type": "local"}}}`
	if err := os.WriteFile(configPath, []byte(oldFormat), 0o644); err != nil {
		t.Fatalf("WriteFile(opencode.json) error = %v", err)
	}

	result, err := Inject(home, adapter)
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("Inject() changed = false; expected Cellar command migration")
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(opencode.json) error = %v", err)
	}

	text := string(content)
	if strings.Contains(text, "/Cellar/") {
		t.Fatalf("opencode.json still contains versioned Homebrew Cellar path; got:\n%s", text)
	}
	if !strings.Contains(text, "/opt/homebrew/bin/engram") {
		t.Fatalf("opencode.json did not migrate to stable Homebrew symlink; got:\n%s", text)
	}

	second, err := Inject(home, adapter)
	if err != nil {
		t.Fatalf("Inject() second error = %v", err)
	}
	if second.Changed {
		t.Fatalf("Inject() second changed = true; expected idempotent Cellar migration")
	}
}

func TestInjectCursorMergesEngramToSettings(t *testing.T) {
	home := t.TempDir()

	cursorAdapter, err := agents.NewAdapter("cursor")
	if err != nil {
		t.Fatalf("NewAdapter(cursor) error = %v", err)
	}

	result, injectErr := Inject(home, cursorAdapter)
	if injectErr != nil {
		t.Fatalf("Inject(cursor) error = %v", injectErr)
	}

	// Cursor uses MCPConfigFile strategy — engram gets merged into mcp.json.
	if !result.Changed {
		t.Fatalf("Inject(cursor) changed = false")
	}
}

func TestInjectCursorWithMalformedMCPJsonRecovery(t *testing.T) {
	// Real Windows users may have a ~/.cursor/mcp.json that starts with non-JSON
	// content (e.g. "allow: all" or just "a"). The installer should recover by
	// treating the broken file as {} and proceeding with the overlay merge.
	home := t.TempDir()

	cursorAdapter, err := agents.NewAdapter("cursor")
	if err != nil {
		t.Fatalf("NewAdapter(cursor) error = %v", err)
	}

	// Pre-create ~/.cursor/mcp.json with invalid (non-JSON) content.
	mcpPath := cursorAdapter.MCPConfigPath(home, "engram")
	if err := os.MkdirAll(filepath.Dir(mcpPath), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(mcpPath, []byte("allow: all"), 0o644); err != nil {
		t.Fatalf("WriteFile(malformed mcp.json) error = %v", err)
	}

	result, injectErr := Inject(home, cursorAdapter)
	if injectErr != nil {
		t.Fatalf("Inject(cursor) with malformed mcp.json error = %v; want nil (should recover)", injectErr)
	}
	if !result.Changed {
		t.Fatalf("Inject(cursor) changed = false; want true")
	}

	content, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatalf("ReadFile(mcp.json) error = %v", err)
	}

	text := string(content)
	if !strings.Contains(text, `"mcpServers"`) {
		t.Fatalf("mcp.json missing mcpServers key after recovery; got:\n%s", text)
	}
	if !strings.Contains(text, `"engram"`) {
		t.Fatalf("mcp.json missing engram server after recovery; got:\n%s", text)
	}
}

func TestInjectVSCodeMergesEngramToMCPConfigFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	adapter := vscode.NewAdapter()

	result, err := Inject(home, adapter)
	if err != nil {
		t.Fatalf("Inject(vscode) error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("Inject(vscode) changed = false")
	}

	mcpPath := adapter.MCPConfigPath(home, "engram")
	content, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatalf("ReadFile(mcp.json) error = %v", err)
	}

	text := string(content)
	if !strings.Contains(text, `"servers"`) {
		t.Fatal("mcp.json missing servers key")
	}
	if !strings.Contains(text, `"engram"`) {
		t.Fatal("mcp.json missing engram server")
	}
	if !strings.Contains(text, `"mcp"`) {
		t.Fatal("mcp.json missing engram args mcp")
	}
	if strings.Contains(text, `"mcpServers"`) {
		t.Fatal("mcp.json should use 'servers' key, not 'mcpServers'")
	}
	// RED: VS Code overlay must include --tools=agent
	assertArgsHaveToolsAgent(t, mcpPath)
}

// ─── Gemini tests ─────────────────────────────────────────────────────────────

func TestInjectGeminiToolsFlagPresent(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, geminiAdapter())
	if err != nil {
		t.Fatalf("Inject(gemini) error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("Inject(gemini) changed = false")
	}

	settingsPath := filepath.Join(home, ".gemini", "settings.json")
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(settings.json) error = %v", err)
	}
	text := string(content)
	if !strings.Contains(text, `"mcpServers"`) {
		t.Fatal("settings.json missing mcpServers key")
	}
	if !strings.Contains(text, `"engram"`) {
		t.Fatal("settings.json missing engram entry")
	}
	// RED: Gemini overlay must use --tools=agent
	if !strings.Contains(text, `"--tools=agent"`) {
		t.Fatal("settings.json missing --tools=agent in args")
	}
}

func TestInjectAntigravityCopiesGeminiSettingsAfterEngramSetup(t *testing.T) {
	home := t.TempDir()
	sourcePath := filepath.Join(home, ".gemini", "settings.json")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(sourcePath), err)
	}
	want := []byte("{\"theme\":\"dark\"}\n")
	if err := os.WriteFile(sourcePath, want, 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", sourcePath, err)
	}

	result, err := Inject(home, antigravityAdapter())
	if err != nil {
		t.Fatalf("Inject(antigravity) error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("Inject(antigravity) changed = false")
	}

	settingsPath := filepath.Join(home, ".gemini", "antigravity", "settings.json")
	got, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", settingsPath, err)
	}
	if string(got) != string(want) {
		t.Fatalf("antigravity settings = %q, want %q", got, want)
	}

	mcpPath := filepath.Join(home, ".gemini", "antigravity", "mcp_config.json")
	assertArgsHaveToolsAgent(t, mcpPath)
}

func TestInjectAntigravityInitializesEmptySettingsWhenGeminiMissing(t *testing.T) {
	home := t.TempDir()

	first, err := Inject(home, antigravityAdapter())
	if err != nil {
		t.Fatalf("Inject(antigravity) first error = %v", err)
	}
	if !first.Changed {
		t.Fatalf("Inject(antigravity) first changed = false")
	}

	settingsPath := filepath.Join(home, ".gemini", "antigravity", "settings.json")
	got, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", settingsPath, err)
	}
	if strings.TrimSpace(string(got)) != "{}" {
		t.Fatalf("antigravity settings = %q, want empty JSON object", got)
	}

	second, err := Inject(home, antigravityAdapter())
	if err != nil {
		t.Fatalf("Inject(antigravity) second error = %v", err)
	}
	if second.Changed {
		t.Fatalf("Inject(antigravity) second changed = true; want false")
	}
}

// ─── Codex tests ──────────────────────────────────────────────────────────────

func TestInjectCodexWritesTOMLMCP(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, codexAdapter())
	if err != nil {
		t.Fatalf("Inject(codex) error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("Inject(codex) changed = false")
	}

	configPath := filepath.Join(home, ".codex", "config.toml")
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config.toml) error = %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "[mcp_servers.engram]") {
		t.Fatalf("config.toml missing [mcp_servers.engram] block; got:\n%s", text)
	}
	// command must reference the engram binary — either relative ("engram") or an
	// absolute path (when engram is on PATH). Both are valid.
	if !strings.Contains(text, "command = ") {
		t.Fatalf("config.toml missing command field; got:\n%s", text)
	}
	cmdLine := ""
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "command = ") {
			cmdLine = strings.TrimSpace(line)
			break
		}
	}
	if cmdLine == "" {
		t.Fatalf("config.toml missing command line; got:\n%s", text)
	}
	// The command value must end with "engram" or "engram.exe".
	cmdVal := strings.TrimPrefix(cmdLine, "command = ")
	cmdVal = strings.Trim(cmdVal, `"`)
	base := filepath.Base(cmdVal)
	if base != "engram" && base != "engram.exe" {
		t.Fatalf("config.toml command %q does not reference engram binary; got:\n%s", cmdVal, text)
	}
	if !strings.Contains(text, `"--tools=agent"`) {
		t.Fatalf("config.toml missing --tools=agent; got:\n%s", text)
	}
}

func TestInjectCodexWritesInstructionFiles(t *testing.T) {
	home := t.TempDir()

	_, err := Inject(home, codexAdapter())
	if err != nil {
		t.Fatalf("Inject(codex) error = %v", err)
	}

	instructionsPath := filepath.Join(home, ".codex", "engram-instructions.md")
	content, err := os.ReadFile(instructionsPath)
	if err != nil {
		t.Fatalf("ReadFile(engram-instructions.md) error = %v", err)
	}
	if !strings.Contains(string(content), "mem_save") {
		t.Fatal("engram-instructions.md missing expected content (mem_save)")
	}

	compactPath := filepath.Join(home, ".codex", "engram-compact-prompt.md")
	compactContent, err := os.ReadFile(compactPath)
	if err != nil {
		t.Fatalf("ReadFile(engram-compact-prompt.md) error = %v", err)
	}
	if !strings.Contains(string(compactContent), "FIRST ACTION REQUIRED") {
		t.Fatal("engram-compact-prompt.md missing expected content (FIRST ACTION REQUIRED)")
	}
}

func TestInjectCodexInjectsTOMLKeys(t *testing.T) {
	home := t.TempDir()

	_, err := Inject(home, codexAdapter())
	if err != nil {
		t.Fatalf("Inject(codex) error = %v", err)
	}

	configPath := filepath.Join(home, ".codex", "config.toml")
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config.toml) error = %v", err)
	}
	text := string(content)

	instructionsPath := filepath.Join(home, ".codex", "engram-instructions.md")
	if !strings.Contains(text, `model_instructions_file`) {
		t.Fatalf("config.toml missing model_instructions_file key; got:\n%s", text)
	}
	if !strings.Contains(text, instructionsPath) {
		t.Fatalf("config.toml model_instructions_file does not reference %q; got:\n%s", instructionsPath, text)
	}

	compactPath := filepath.Join(home, ".codex", "engram-compact-prompt.md")
	if !strings.Contains(text, `experimental_compact_prompt_file`) {
		t.Fatalf("config.toml missing experimental_compact_prompt_file key; got:\n%s", text)
	}
	if !strings.Contains(text, compactPath) {
		t.Fatalf("config.toml experimental_compact_prompt_file does not reference %q; got:\n%s", compactPath, text)
	}
}

// ─── Engram setup absolute path preservation tests ────────────────────────────

// TestInjectClaudePreservesAbsoluteCommandFromEngramSetup verifies that when
// `engram setup claude-code` has already written an absolute-path command to
// ~/.claude/mcp/engram.json (Engram v1.10.3+ behaviour), a subsequent call to
// Inject() does NOT overwrite the absolute path with the relative "engram".
func TestInjectClaudePreservesAbsoluteCommandFromEngramSetup(t *testing.T) {
	home := t.TempDir()

	// Simulate what `engram setup claude-code` writes on v1.10.3+:
	// an absolute path as the command value.
	absPath := "/opt/homebrew/bin/engram"
	mcpPath := filepath.Join(home, ".claude", "mcp", "engram.json")
	if err := os.MkdirAll(filepath.Dir(mcpPath), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	setupContent := []byte(`{
  "command": "/opt/homebrew/bin/engram",
  "args": ["mcp", "--tools=agent"]
}
`)
	if err := os.WriteFile(mcpPath, setupContent, 0o644); err != nil {
		t.Fatalf("WriteFile(engram.json) error = %v", err)
	}

	// Now run Inject — should NOT overwrite the absolute command.
	_, err := Inject(home, claudeAdapter())
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	content, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatalf("ReadFile(engram.json) error = %v", err)
	}

	text := string(content)
	if !strings.Contains(text, absPath) {
		t.Fatalf("Inject() overwrote absolute command path; want %q preserved, got:\n%s", absPath, text)
	}
	// Still must have --tools=agent.
	assertArgsHaveToolsAgent(t, mcpPath)
}

// TestInjectClaudePreservesAbsoluteCommandIsIdempotent verifies that calling
// Inject() twice when an absolute-path engram.json already exists does not
// cause repeated writes (idempotency).
func TestInjectClaudePreservesAbsoluteCommandIsIdempotent(t *testing.T) {
	home := t.TempDir()

	absPath := "/usr/local/bin/engram"
	mcpPath := filepath.Join(home, ".claude", "mcp", "engram.json")
	if err := os.MkdirAll(filepath.Dir(mcpPath), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	setupContent := []byte(`{
  "command": "/usr/local/bin/engram",
  "args": ["mcp", "--tools=agent"]
}
`)
	if err := os.WriteFile(mcpPath, setupContent, 0o644); err != nil {
		t.Fatalf("WriteFile(engram.json) error = %v", err)
	}

	first, err := Inject(home, claudeAdapter())
	if err != nil {
		t.Fatalf("Inject() first error = %v", err)
	}

	second, err := Inject(home, claudeAdapter())
	if err != nil {
		t.Fatalf("Inject() second error = %v", err)
	}
	if second.Changed {
		t.Fatalf("Inject() second changed = true after absolute-path setup; want idempotent (no change)")
	}

	// Absolute path must still be present.
	content, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatalf("ReadFile(engram.json) error = %v", err)
	}
	if !strings.Contains(string(content), absPath) {
		t.Fatalf("absolute command path %q was lost after second Inject(); got:\n%s", absPath, string(content))
	}
	_ = first // first result not the focus of this test
}

// TestInjectClaudeAddsToolsAgentWhenSetupWritesBareArgs verifies that if
// `engram setup` wrote an absolute command but with bare args (no --tools=agent),
// Inject() adds --tools=agent while preserving the absolute path.
func TestInjectClaudeAddsToolsAgentWhenSetupWritesBareArgs(t *testing.T) {
	home := t.TempDir()

	absPath := "/home/user/go/bin/engram"
	mcpPath := filepath.Join(home, ".claude", "mcp", "engram.json")
	if err := os.MkdirAll(filepath.Dir(mcpPath), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	// Bare mcp arg without --tools=agent — older engram setup format.
	setupContent := []byte(`{
  "command": "/home/user/go/bin/engram",
  "args": ["mcp"]
}
`)
	if err := os.WriteFile(mcpPath, setupContent, 0o644); err != nil {
		t.Fatalf("WriteFile(engram.json) error = %v", err)
	}

	_, err := Inject(home, claudeAdapter())
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	content, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatalf("ReadFile(engram.json) error = %v", err)
	}
	text := string(content)

	// Absolute path must be preserved.
	if !strings.Contains(text, absPath) {
		t.Fatalf("absolute path %q was lost; got:\n%s", absPath, text)
	}
	// --tools=agent must be added.
	assertArgsHaveToolsAgent(t, mcpPath)
}

func TestInjectClaudeMigratesCellarCommandToStablePath(t *testing.T) {
	home := t.TempDir()

	mockEngramLookPath(t, "/usr/local/bin/engram", "")

	mcpPath := filepath.Join(home, ".claude", "mcp", "engram.json")
	if err := os.MkdirAll(filepath.Dir(mcpPath), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	setupContent := []byte(`{
  "command": "/usr/local/Cellar/engram/1.14.1/bin/engram",
  "args": ["mcp", "--tools=agent"]
}
`)
	if err := os.WriteFile(mcpPath, setupContent, 0o644); err != nil {
		t.Fatalf("WriteFile(engram.json) error = %v", err)
	}

	result, err := Inject(home, claudeAdapter())
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("Inject() changed = false; expected Cellar command migration")
	}

	content, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatalf("ReadFile(engram.json) error = %v", err)
	}
	text := string(content)
	if strings.Contains(text, "/Cellar/") {
		t.Fatalf("engram.json still contains versioned Homebrew Cellar path; got:\n%s", text)
	}
	if !strings.Contains(text, "/usr/local/bin/engram") {
		t.Fatalf("engram.json did not migrate to stable Homebrew symlink; got:\n%s", text)
	}
	assertArgsHaveToolsAgent(t, mcpPath)
}

func TestInjectCodexIsIdempotent(t *testing.T) {
	home := t.TempDir()

	first, err := Inject(home, codexAdapter())
	if err != nil {
		t.Fatalf("Inject(codex) first error = %v", err)
	}
	if !first.Changed {
		t.Fatalf("Inject(codex) first changed = false")
	}

	second, err := Inject(home, codexAdapter())
	if err != nil {
		t.Fatalf("Inject(codex) second error = %v", err)
	}
	if second.Changed {
		t.Fatalf("Inject(codex) second changed = true (should be idempotent)")
	}

	// Verify only one [mcp_servers.engram] block.
	configPath := filepath.Join(home, ".codex", "config.toml")
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config.toml) error = %v", err)
	}
	count := strings.Count(string(content), "[mcp_servers.engram]")
	if count != 1 {
		t.Fatalf("config.toml has %d [mcp_servers.engram] blocks, want exactly 1; got:\n%s", count, string(content))
	}
}

// ─── Absolute path resolution tests ──────────────────────────────────────────

// mockEngramLookPath sets EngramLookPath to a mock and restores it after the test.
func mockEngramLookPath(t *testing.T, result string, errMsg string) {
	t.Helper()
	orig := EngramLookPath
	EngramLookPath = func(string) (string, error) {
		if errMsg != "" {
			return "", fmt.Errorf("%s", errMsg)
		}
		return result, nil
	}
	t.Cleanup(func() { EngramLookPath = orig })
}

// TestEngramInjectUsesAbsolutePathWhenAvailable verifies that when engram is
// resolvable on PATH, its absolute path is written into the MCP config file
// for agents that use StrategyMCPConfigFile (e.g. Windsurf).
func TestEngramInjectUsesAbsolutePathWhenAvailable(t *testing.T) {
	home := t.TempDir()

	absPath := "/usr/local/bin/engram"
	mockEngramLookPath(t, absPath, "")

	windsurfAdapter, err := agents.NewAdapter("windsurf")
	if err != nil {
		t.Fatalf("NewAdapter(windsurf) error = %v", err)
	}

	result, injectErr := Inject(home, windsurfAdapter)
	if injectErr != nil {
		t.Fatalf("Inject(windsurf) error = %v", injectErr)
	}
	if !result.Changed {
		t.Fatalf("Inject(windsurf) changed = false")
	}

	mcpPath := windsurfAdapter.MCPConfigPath(home, "engram")
	content, readErr := os.ReadFile(mcpPath)
	if readErr != nil {
		t.Fatalf("ReadFile(%q) error = %v", mcpPath, readErr)
	}

	// Parse and validate the command field contains the absolute path.
	var parsed map[string]any
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", mcpPath, err)
	}

	mcpServersRaw, ok := parsed["mcpServers"]
	if !ok {
		t.Fatalf("mcp_config.json missing mcpServers key; got:\n%s", content)
	}
	mcpServers, ok := mcpServersRaw.(map[string]any)
	if !ok {
		t.Fatalf("mcpServers has unexpected type: %T", mcpServersRaw)
	}
	engramServerRaw, ok := mcpServers["engram"]
	if !ok {
		t.Fatalf("mcpServers missing engram entry; got:\n%s", content)
	}
	engramServer, ok := engramServerRaw.(map[string]any)
	if !ok {
		t.Fatalf("engram server has unexpected type: %T", engramServerRaw)
	}

	cmd, _ := engramServer["command"].(string)
	if cmd != absPath {
		t.Fatalf("mcp_config.json command = %q, want absolute path %q", cmd, absPath)
	}
}

// TestEngramInjectFallsBackToRelativeWhenNotFound verifies that when engram
// cannot be resolved on PATH, the config falls back to the relative "engram"
// command string.
func TestEngramInjectFallsBackToRelativeWhenNotFound(t *testing.T) {
	home := t.TempDir()

	mockEngramLookPath(t, "", "not found")

	windsurfAdapter, err := agents.NewAdapter("windsurf")
	if err != nil {
		t.Fatalf("NewAdapter(windsurf) error = %v", err)
	}

	result, injectErr := Inject(home, windsurfAdapter)
	if injectErr != nil {
		t.Fatalf("Inject(windsurf) error = %v", injectErr)
	}
	if !result.Changed {
		t.Fatalf("Inject(windsurf) changed = false")
	}

	mcpPath := windsurfAdapter.MCPConfigPath(home, "engram")
	content, readErr := os.ReadFile(mcpPath)
	if readErr != nil {
		t.Fatalf("ReadFile(%q) error = %v", mcpPath, readErr)
	}

	text := string(content)
	if !strings.Contains(text, `"command": "engram"`) {
		t.Fatalf("mcp_config.json should use relative fallback 'engram'; got:\n%s", text)
	}
}

// TestEngramInjectAbsolutePathForOpenCodeMergeStrategy verifies that the
// absolute path is used when the StrategyMergeIntoSettings strategy is
// applied for OpenCode.
func TestEngramInjectAbsolutePathForOpenCodeMergeStrategy(t *testing.T) {
	home := t.TempDir()

	absPath := "/usr/local/bin/engram"
	mockEngramLookPath(t, absPath, "")

	adapter := opencodeAdapter()
	settingsDir := filepath.Dir(adapter.SettingsPath(home))
	os.MkdirAll(settingsDir, 0o755)
	os.WriteFile(adapter.SettingsPath(home), []byte("{}"), 0o644)

	_, err := Inject(home, adapter)
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	content, err := os.ReadFile(adapter.SettingsPath(home))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	text := string(content)
	// For standard agents (OpenCode), prefer the stable Homebrew symlink when
	// available instead of a versioned Cellar path.
	if !strings.Contains(text, `"engram"`) {
		t.Fatalf("OpenCode settings missing stable engram command, got: %s", text)
	}
	// OpenCode 1.3.3+: command must be an array, no separate "args" field.
	if strings.Contains(text, `"args"`) {
		t.Fatalf("OpenCode settings must NOT have a separate args field; got: %s", text)
	}

	// Structurally verify command is a []any containing the stable path "engram".
	var parsed map[string]any
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("Unmarshal(opencode.json) error = %v", err)
	}
	mcpRaw, ok := parsed["mcp"]
	if !ok {
		t.Fatalf("opencode.json missing mcp key; got:\n%s", text)
	}
	mcpMap, ok := mcpRaw.(map[string]any)
	if !ok {
		t.Fatalf("mcp key has unexpected type %T; got:\n%s", mcpRaw, text)
	}
	engramRaw, ok := mcpMap["engram"]
	if !ok {
		t.Fatalf("mcp missing engram key; got:\n%s", text)
	}
	engramMap, ok := engramRaw.(map[string]any)
	if !ok {
		t.Fatalf("mcp.engram has unexpected type %T; got:\n%s", engramRaw, text)
	}
	cmdRaw, ok := engramMap["command"]
	if !ok {
		t.Fatalf("mcp.engram missing command key; got:\n%s", text)
	}
	cmdArr, ok := cmdRaw.([]any)
	if !ok {
		t.Fatalf("mcp.engram.command must be an array, got %T; value:\n%s", cmdRaw, text)
	}
	if len(cmdArr) == 0 {
		t.Fatalf("mcp.engram.command array is empty; got:\n%s", text)
	}
	firstElem, ok := cmdArr[0].(string)
	if !ok || firstElem != absPath {
		t.Fatalf("mcp.engram.command[0] = %v, want stable Homebrew symlink %q; got:\n%s", cmdArr[0], absPath, text)
	}
}

// TestEngramInjectAbsolutePathForGeminiMergeStrategy verifies that the
// absolute path is also used when the StrategyMergeIntoSettings strategy is
// applied (e.g. Gemini CLI).
func TestEngramInjectAbsolutePathForGeminiMergeStrategy(t *testing.T) {
	home := t.TempDir()

	absPath := "/opt/homebrew/bin/engram"
	mockEngramLookPath(t, absPath, "")

	result, err := Inject(home, geminiAdapter())
	if err != nil {
		t.Fatalf("Inject(gemini) error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("Inject(gemini) changed = false")
	}

	settingsPath := filepath.Join(home, ".gemini", "settings.json")
	content, readErr := os.ReadFile(settingsPath)
	if readErr != nil {
		t.Fatalf("ReadFile(settings.json) error = %v", readErr)
	}

	text := string(content)
	// For standard agents (Gemini), we now prioritize a stable relative path
	// "engram" instead of a dynamic absolute path to ensure idempotency.
	if !strings.Contains(text, `"engram"`) {
		t.Fatalf("settings.json missing stable relative path 'engram'; got:\n%s", text)
	}
}

func TestQwenEngramIdempotency(t *testing.T) {
	orig := EngramLookPath
	t.Cleanup(func() { EngramLookPath = orig })

	homeDir := t.TempDir()
	adapter := qwenAdapter()
	settingsPath := adapter.SettingsPath(homeDir)

	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		t.Fatal(err)
	}

	EngramLookPath = func(string) (string, error) {
		return "", os.ErrNotExist
	}

	_, err := Inject(homeDir, adapter)
	if err != nil {
		t.Fatalf("First injection failed: %v", err)
	}

	content1, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate engram being found later (e.g. after go install or manual install)
	absPath := "/usr/local/bin/engram"
	EngramLookPath = func(string) (string, error) {
		return absPath, nil
	}

	_, err = Inject(homeDir, adapter)
	if err != nil {
		t.Fatalf("Second injection failed: %v", err)
	}

	content2, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}

	if string(content1) != string(content2) {
		t.Errorf("Idempotency failure! Settings changed between runs despite engram command being stable-relative.\nRun 1:\n%s\nRun 2:\n%s", string(content1), string(content2))
	}
}

func TestInjectOpenClawMergesEngramIntoMCPServersPreservingStdioAndRemoteFields(t *testing.T) {
	home := t.TempDir()
	adapter := openclawAdapter()
	configPath := adapter.SettingsPath(home)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	existing := `{
  "mcp": {
    "sessionIdleTtlMs": 120000,
    "servers": {
      "filesystem": {
        "command": "npx",
        "args": ["-y", "@modelcontextprotocol/server-filesystem"],
        "env": {"ROOT": "/workspace"},
        "unknownStdioField": true
      },
      "linear": {
        "url": "https://mcp.linear.app/sse",
        "transport": "sse",
        "headers": {"Authorization": "Bearer existing-token"},
        "unknownRemoteField": "preserve-me"
      }
    }
  },
  "theme": "kanagawa"
}`
	if err := os.WriteFile(configPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("WriteFile(openclaw.json) error = %v", err)
	}

	result, err := Inject(home, adapter)
	if err != nil {
		t.Fatalf("Inject(openclaw) error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Inject(openclaw) changed = false")
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(openclaw.json) error = %v", err)
	}
	root := unmarshalObjectForTest(t, content)
	mcp := objectAtForTest(t, root, "mcp")
	if got := mcp["sessionIdleTtlMs"]; got != float64(120000) {
		t.Fatalf("mcp.sessionIdleTtlMs = %v, want preserved 120000", got)
	}
	servers := objectAtForTest(t, mcp, "servers")

	filesystem := objectAtForTest(t, servers, "filesystem")
	if got := filesystem["command"]; got != "npx" {
		t.Fatalf("filesystem.command = %v, want npx", got)
	}
	if got := filesystem["unknownStdioField"]; got != true {
		t.Fatalf("filesystem.unknownStdioField = %v, want true", got)
	}
	args, ok := filesystem["args"].([]any)
	if !ok || len(args) != 2 || args[1] != "@modelcontextprotocol/server-filesystem" {
		t.Fatalf("filesystem.args = %#v, want preserved stdio args", filesystem["args"])
	}

	linear := objectAtForTest(t, servers, "linear")
	if got := linear["url"]; got != "https://mcp.linear.app/sse" {
		t.Fatalf("linear.url = %v, want preserved remote url", got)
	}
	if got := linear["transport"]; got != "sse" {
		t.Fatalf("linear.transport = %v, want sse", got)
	}
	headers := objectAtForTest(t, linear, "headers")
	if got := headers["Authorization"]; got != "Bearer existing-token" {
		t.Fatalf("linear Authorization header = %v, want preserved token", got)
	}
	if got := linear["unknownRemoteField"]; got != "preserve-me" {
		t.Fatalf("linear.unknownRemoteField = %v, want preserve-me", got)
	}

	engram := objectAtForTest(t, servers, "engram")
	if got := engram["command"]; got != "engram" {
		t.Fatalf("engram.command = %v, want engram", got)
	}
	engramArgs, ok := engram["args"].([]any)
	if !ok || len(engramArgs) != 2 || engramArgs[0] != "mcp" || engramArgs[1] != "--tools=agent" {
		t.Fatalf("engram.args = %#v, want [mcp --tools=agent]", engram["args"])
	}
	if _, hasMCPServers := root["mcpServers"]; hasMCPServers {
		t.Fatal("OpenClaw config must use mcp.servers, not top-level mcpServers")
	}
}

func TestInjectOpenClawHandlesJSON5ConfigAndMissingConfigPath(t *testing.T) {
	t.Run("preserves JSON5 compatible config content as normalized JSON", func(t *testing.T) {
		home := t.TempDir()
		adapter := openclawAdapter()
		configPath := adapter.SettingsPath(home)
		if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}

		existing := `{
  // OpenClaw user config with JSON5-style comments.
  "ui": {
    "theme": "kanagawa", // trailing comma survives via normalization
  },
  "mcp": {
    "servers": {
      "remoteDocs": {
        "url": "https://docs.example/mcp",
        "transport": "http",
      },
    },
  },
}`
		if err := os.WriteFile(configPath, []byte(existing), 0o644); err != nil {
			t.Fatalf("WriteFile(openclaw.json) error = %v", err)
		}

		if _, err := Inject(home, adapter); err != nil {
			t.Fatalf("Inject(openclaw) error = %v", err)
		}

		content, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("ReadFile(openclaw.json) error = %v", err)
		}
		root := unmarshalObjectForTest(t, content)
		ui := objectAtForTest(t, root, "ui")
		if got := ui["theme"]; got != "kanagawa" {
			t.Fatalf("ui.theme = %v, want preserved kanagawa", got)
		}
		servers := objectAtForTest(t, objectAtForTest(t, root, "mcp"), "servers")
		remoteDocs := objectAtForTest(t, servers, "remoteDocs")
		if got := remoteDocs["transport"]; got != "http" {
			t.Fatalf("remoteDocs.transport = %v, want preserved http", got)
		}
		if _, ok := servers["engram"]; !ok {
			t.Fatal("mcp.servers.engram missing after JSON5 merge")
		}
	})

	t.Run("creates canonical OpenClaw config when missing", func(t *testing.T) {
		home := t.TempDir()
		adapter := openclawAdapter()
		configPath := filepath.Join(home, ".openclaw", "openclaw.json")

		if _, err := os.Stat(configPath); !os.IsNotExist(err) {
			t.Fatalf("expected missing OpenClaw config before inject, got err=%v", err)
		}
		if _, err := Inject(home, adapter); err != nil {
			t.Fatalf("Inject(openclaw) error = %v", err)
		}
		content, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("ReadFile(created openclaw.json) error = %v", err)
		}
		servers := objectAtForTest(t, objectAtForTest(t, unmarshalObjectForTest(t, content), "mcp"), "servers")
		if _, ok := servers["engram"]; !ok {
			t.Fatal("created OpenClaw config missing mcp.servers.engram")
		}
	})
}

func TestInjectOpenClawWritesEngramProtocolToWorkspaceAgentsOnly(t *testing.T) {
	workspace := t.TempDir()
	adapter := openclawAdapter()
	toolsPath := filepath.Join(workspace, "TOOLS.md")
	if err := os.WriteFile(toolsPath, []byte("# Tool guidance\n\nUser-owned tool notes.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(TOOLS.md) error = %v", err)
	}

	first, err := Inject(workspace, adapter)
	if err != nil {
		t.Fatalf("Inject(openclaw) first error = %v", err)
	}
	if !first.Changed {
		t.Fatal("Inject(openclaw) first changed = false")
	}

	agentsPath := filepath.Join(workspace, "AGENTS.md")
	agentsContent, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("ReadFile(AGENTS.md) error = %v", err)
	}
	agentsText := string(agentsContent)
	for _, want := range []string{
		"<!-- gentle-ai:engram-protocol -->",
		"<!-- /gentle-ai:engram-protocol -->",
		"mem_save",
	} {
		if !strings.Contains(agentsText, want) {
			t.Fatalf("OpenClaw AGENTS.md missing Engram protocol content %q; got:\n%s", want, agentsText)
		}
	}
	if _, err := os.Stat(filepath.Join(workspace, ".openclaw", "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("OpenClaw Engram injection must not write global .openclaw/AGENTS.md; stat err=%v", err)
	}

	toolsContent, err := os.ReadFile(toolsPath)
	if err != nil {
		t.Fatalf("ReadFile(TOOLS.md) error = %v", err)
	}
	toolsText := string(toolsContent)
	if strings.Contains(toolsText, "gentle-ai:engram-protocol") || strings.Contains(toolsText, "mem_save") {
		t.Fatalf("TOOLS.md must not receive Engram protocol sections; got:\n%s", toolsText)
	}
	if !strings.Contains(toolsText, "User-owned tool notes.") {
		t.Fatalf("TOOLS.md user content was modified; got:\n%s", toolsText)
	}

	second, err := Inject(workspace, adapter)
	if err != nil {
		t.Fatalf("Inject(openclaw) second error = %v", err)
	}
	if second.Changed {
		t.Fatal("OpenClaw Engram injection should be idempotent")
	}
	updated, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("ReadFile(AGENTS.md) second error = %v", err)
	}
	if count := strings.Count(string(updated), "<!-- gentle-ai:engram-protocol -->"); count != 1 {
		t.Fatalf("AGENTS.md has %d Engram protocol markers, want exactly 1", count)
	}
}

func TestInjectOpenClawRejectsAmbiguousWorkspacePath(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)

	result, err := Inject("", openclawAdapter())
	if err == nil {
		t.Fatalf("Inject(openclaw, empty workspace) error = nil, want deterministic ambiguity error; result=%+v", result)
	}
	if _, statErr := os.Stat(filepath.Join(cwd, ".openclaw", "openclaw.json")); !os.IsNotExist(statErr) {
		t.Fatalf("ambiguous OpenClaw workspace must not create relative config; stat err=%v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(cwd, "AGENTS.md")); !os.IsNotExist(statErr) {
		t.Fatalf("ambiguous OpenClaw workspace must not create relative AGENTS.md; stat err=%v", statErr)
	}
}

func unmarshalObjectForTest(t *testing.T, content []byte) map[string]any {
	t.Helper()
	var root map[string]any
	if err := json.Unmarshal(content, &root); err != nil {
		t.Fatalf("Unmarshal JSON error = %v; content:\n%s", err, content)
	}
	return root
}

func objectAtForTest(t *testing.T, root map[string]any, key string) map[string]any {
	t.Helper()
	value, ok := root[key]
	if !ok {
		t.Fatalf("missing object key %q in %#v", key, root)
	}
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("key %q has type %T, want object", key, value)
	}
	return object
}
