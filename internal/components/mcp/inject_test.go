package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/antigravity"
	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/claude"
	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/codex"
	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/hermes"
	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/kilocode"
	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/kimi"
	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/openclaw"
	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/vscode"
	"github.com/gentleman-programming/gentle-ai/v2/internal/versions"
)

func cursorAdapter(t *testing.T) agents.Adapter {
	t.Helper()
	adapter, err := agents.NewAdapter("cursor")
	if err != nil {
		t.Fatalf("NewAdapter(cursor) error = %v", err)
	}
	return adapter
}

func antigravityAdapter() agents.Adapter { return antigravity.NewAdapter() }
func claudeAdapter() agents.Adapter      { return claude.NewAdapter() }
func hermesAdapter() agents.Adapter      { return hermes.NewAdapter() }
func kilocodeAdapter() agents.Adapter    { return kilocode.NewAdapter() }
func kimiAdapter() agents.Adapter        { return kimi.NewAdapter() }
func openclawAdapter() agents.Adapter    { return openclaw.NewAdapter() }
func opencodeAdapter() agents.Adapter    { return opencode.NewAdapter() }

func assertOnlyKeys(t *testing.T, path string, object map[string]any, keys ...string) {
	t.Helper()

	want := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		want[key] = struct{}{}
	}

	for _, key := range keys {
		if _, exists := object[key]; !exists {
			t.Fatalf("%q missing expected key %q; got %#v", path, key, object)
		}
	}

	for key := range object {
		if _, expected := want[key]; !expected {
			t.Fatalf("%q contains unexpected key %q; want only %v; got %#v", path, key, keys, object)
		}
	}
}

// readOpenCodeContext7Entry reads the mcp.context7 object from an OpenCode/KiloCode
// opencode.json config file. Navigates parsed["mcp"]["context7"].
func readOpenCodeContext7Entry(t *testing.T, path string) map[string]any {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", path, err)
	}

	mcp, ok := parsed["mcp"].(map[string]any)
	if !ok {
		t.Fatalf("%q missing object key mcp; got %#v", path, parsed["mcp"])
	}

	context7, ok := mcp["context7"].(map[string]any)
	if !ok {
		t.Fatalf("%q missing object key mcp.context7; got %#v", path, mcp["context7"])
	}

	return context7
}

// assertOpenCodeRemoteContext7Schema asserts the mcp.context7 entry in an
// OpenCode/KiloCode opencode.json is a valid remote entry with no legacy local keys.
func assertOpenCodeRemoteContext7Schema(t *testing.T, path string) {
	t.Helper()

	context7 := readOpenCodeContext7Entry(t, path)

	if got := context7["type"]; got != "remote" {
		t.Fatalf("%q mcp.context7.type = %#v; want %q", path, got, "remote")
	}
	if got := context7["url"]; got != "https://mcp.context7.com/mcp" {
		t.Fatalf("%q mcp.context7.url = %#v; want context7 remote URL", path, got)
	}
	if got := context7["enabled"]; got != true {
		t.Fatalf("%q mcp.context7.enabled = %#v; want true", path, got)
	}

	for _, key := range []string{"command", "args", "env", "environment"} {
		if _, exists := context7[key]; exists {
			t.Fatalf("%q mcp.context7 contains legacy local key %q; got %#v", path, key, context7)
		}
	}
}

// readMCPServersContext7Entry reads the mcpServers.context7 object used by
// agents that store Context7 under an mcpServers-based config file.
func readMCPServersContext7Entry(t *testing.T, path string) map[string]any {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", path, err)
	}

	mcpServers, ok := parsed["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("%q missing object key mcpServers; got %#v", path, parsed["mcpServers"])
	}

	context7, ok := mcpServers["context7"].(map[string]any)
	if !ok {
		t.Fatalf("%q missing object key mcpServers.context7; got %#v", path, mcpServers["context7"])
	}

	return context7
}

// assertAntigravityContext7Schema asserts the mcpServers.context7 entry in an
// Antigravity mcp_config.json is a valid remote entry with no legacy local keys.
func assertAntigravityContext7Schema(t *testing.T, path string) {
	t.Helper()

	context7 := readMCPServersContext7Entry(t, path)

	if got := context7["serverUrl"]; got != "https://mcp.context7.com/mcp" {
		t.Fatalf("%q mcpServers.context7.serverUrl = %#v; want context7 remote URL", path, got)
	}

	assertOnlyKeys(t, path, context7, "serverUrl")
}

// assertKimiContext7Schema asserts the mcpServers.context7 entry in a Kimi
// mcp.json is the documented remote HTTP config with no legacy local keys.
func assertKimiContext7Schema(t *testing.T, path string) {
	t.Helper()

	context7 := readMCPServersContext7Entry(t, path)

	if got := context7["transport"]; got != "http" {
		t.Fatalf("%q mcpServers.context7.transport = %#v; want %q", path, got, "http")
	}
	if got := context7["url"]; got != "https://mcp.context7.com/mcp" {
		t.Fatalf("%q mcpServers.context7.url = %#v; want context7 remote URL", path, got)
	}

	assertOnlyKeys(t, path, context7, "transport", "url")
}

func TestInjectOpenCodeMergesContext7AndIsIdempotent(t *testing.T) {
	home := t.TempDir()

	first, err := Inject(home, home, opencodeAdapter())
	if err != nil {
		t.Fatalf("Inject() first error = %v", err)
	}
	if !first.Changed {
		t.Fatalf("Inject() first changed = false")
	}

	second, err := Inject(home, home, opencodeAdapter())
	if err != nil {
		t.Fatalf("Inject() second error = %v", err)
	}
	if second.Changed {
		t.Fatalf("Inject() second changed = true")
	}

	configPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	config, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(opencode.json) error = %v", err)
	}

	if len(config) == 0 {
		t.Fatalf("opencode.json is empty")
	}

	assertOpenCodeRemoteContext7Schema(t, configPath)

	text := string(config)
	if !strings.Contains(text, `"mcp"`) {
		t.Fatal("opencode.json missing mcp key")
	}
	if !strings.Contains(text, `"type": "remote"`) {
		t.Fatal("opencode.json context7 missing type: remote")
	}
	if strings.Contains(text, `"mcpServers"`) {
		t.Fatal("opencode.json should use 'mcp' key, not 'mcpServers'")
	}
}

func TestInjectOpenClawMergesContext7UnderMCPDotServersAndMigratesLegacyMCPServers(t *testing.T) {
	home := t.TempDir()
	adapter := openclawAdapter()
	configPath := adapter.SettingsPath(home)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(openclaw config dir) error = %v", err)
	}

	existing := `{
  "mcpServers": {
    "legacyDocs": {
      "command": "legacy-docs"
    },
    "context7": {
      "command": "old-context7"
    }
  },
  "mcp": {
    "sessionIdleTtlMs": 120000,
    "servers": {
      "context7": {
        "command": "npx",
        "args": ["-y", "@upstash/context7-mcp"]
      }
    }
  },
  "theme": "kanagawa"
}`
	if err := os.WriteFile(configPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("WriteFile(openclaw.json) error = %v", err)
	}

	first, err := Inject(home, home, adapter)
	if err != nil {
		t.Fatalf("Inject(openclaw) first error = %v", err)
	}
	if !first.Changed {
		t.Fatalf("Inject(openclaw) first changed = false")
	}

	second, err := Inject(home, home, adapter)
	if err != nil {
		t.Fatalf("Inject(openclaw) second error = %v", err)
	}
	if second.Changed {
		t.Fatalf("Inject(openclaw) second changed = true")
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(openclaw.json) error = %v", err)
	}
	text := string(content)
	if strings.Contains(text, `"mcpServers"`) {
		t.Fatalf("openclaw.json must use mcp.servers, not root mcpServers; got:\n%s", text)
	}
	if !strings.Contains(text, `"mcp"`) || !strings.Contains(text, `"servers"`) {
		t.Fatalf("openclaw.json missing mcp.servers; got:\n%s", text)
	}
	if !strings.Contains(text, `"legacyDocs"`) {
		t.Fatalf("openclaw.json should migrate legacy mcpServers entries into mcp.servers; got:\n%s", text)
	}
	if !strings.Contains(text, `"sessionIdleTtlMs": 120000`) {
		t.Fatalf("openclaw.json should preserve existing mcp fields; got:\n%s", text)
	}
	if !strings.Contains(text, `"context7"`) || !strings.Contains(text, `@upstash/context7-mcp@`) {
		t.Fatalf("openclaw.json missing context7 under mcp.servers; got:\n%s", text)
	}
}

func TestInjectOpenCodeAndKilocodePreserveContext7Headers(t *testing.T) {
	tests := []struct {
		name    string
		adapter agents.Adapter
	}{
		{name: "OpenCode", adapter: opencodeAdapter()},
		{name: "KiloCode", adapter: kilocodeAdapter()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			configPath := tt.adapter.SettingsPath(home)
			if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
				t.Fatalf("MkdirAll error = %v", err)
			}

			existing := `{
	  "mcp": {
	    "context7": {
	      "type": "local",
	      "command": ["npx", "-y", "@upstash/context7-mcp"],
	      "args": ["legacy"],
	      "env": {"TOKEN": "x"},
	      "environment": {"TOKEN": "y"},
	      "headers": {
	        "CONTEXT7_API_KEY": "{env:CONTEXT7_API_KEY}",
	        "number": 42,
	        "nested": {"secret": "value"},
	        "list": ["secret"]
	      },
	      "enabled": false
	    },
	    "engram": {
	      "type": "local",
	      "command": ["engram-server"]
	    }
	  }
	}`
			if err := os.WriteFile(configPath, []byte(existing), 0o644); err != nil {
				t.Fatalf("WriteFile(opencode.json) error = %v", err)
			}

			first, err := Inject(home, home, tt.adapter)
			if err != nil {
				t.Fatalf("Inject() first error = %v", err)
			}
			if !first.Changed {
				t.Fatal("Inject() first changed = false")
			}

			assertOpenCodeRemoteContext7Schema(t, configPath)
			context7 := readOpenCodeContext7Entry(t, configPath)
			headers, ok := context7["headers"].(map[string]any)
			if !ok || headers["CONTEXT7_API_KEY"] != "{env:CONTEXT7_API_KEY}" {
				t.Fatalf("mcp.context7.headers = %#v; want existing API key header", context7["headers"])
			}
			for _, key := range []string{"number", "nested", "list"} {
				if _, exists := headers[key]; exists {
					t.Fatalf("mcp.context7.headers[%q] = %#v; want invalid value discarded", key, headers[key])
				}
			}

			content, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatalf("ReadFile(opencode.json) error = %v", err)
			}
			if !strings.Contains(string(content), `"engram"`) {
				t.Fatal("opencode.json missing existing mcp.engram entry")
			}

			second, err := Inject(home, home, tt.adapter)
			if err != nil {
				t.Fatalf("Inject() second error = %v", err)
			}
			if second.Changed {
				t.Fatal("Inject() second changed = true")
			}
		})
	}
}

func TestInjectOpenCodeAndKilocodeRecoverMalformedSettingsAndDiscardInvalidHeaders(t *testing.T) {
	tests := []struct {
		name     string
		adapter  agents.Adapter
		existing string
	}{
		{name: "OpenCode malformed settings", adapter: opencodeAdapter(), existing: `{malformed`},
		{name: "KiloCode malformed settings", adapter: kilocodeAdapter(), existing: `{malformed`},
		{name: "OpenCode malformed headers", adapter: opencodeAdapter(), existing: `{"mcp":{"context7":{"headers":`},
		{name: "KiloCode malformed headers", adapter: kilocodeAdapter(), existing: `{"mcp":{"context7":{"headers":`},
		{name: "OpenCode non-object headers", adapter: opencodeAdapter(), existing: `{"mcp":{"context7":{"headers":["secret"]}}}`},
		{name: "KiloCode non-object headers", adapter: kilocodeAdapter(), existing: `{"mcp":{"context7":{"headers":"secret"}}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			configPath := tt.adapter.SettingsPath(home)
			if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
				t.Fatalf("MkdirAll error = %v", err)
			}
			if err := os.WriteFile(configPath, []byte(tt.existing), 0o644); err != nil {
				t.Fatalf("WriteFile(opencode.json) error = %v", err)
			}

			first, err := Inject(home, home, tt.adapter)
			if err != nil {
				t.Fatalf("Inject() first error = %v", err)
			}
			if !first.Changed {
				t.Fatal("Inject() first changed = false")
			}

			assertOpenCodeRemoteContext7Schema(t, configPath)
			context7 := readOpenCodeContext7Entry(t, configPath)
			if _, exists := context7["headers"]; exists {
				t.Fatalf("mcp.context7.headers = %#v; want invalid headers discarded", context7["headers"])
			}

			second, err := Inject(home, home, tt.adapter)
			if err != nil {
				t.Fatalf("Inject() second error = %v", err)
			}
			if second.Changed {
				t.Fatal("Inject() second changed = true")
			}
		})
	}
}

func TestInjectOpenCodePreservesOtherMCPEntriesWhenReplacingContext7(t *testing.T) {
	home := t.TempDir()
	adapter := opencodeAdapter()
	configPath := adapter.SettingsPath(home)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}

	legacy := `{
	  "mcp": {
	    "context7": {
	      "type": "local",
	      "command": ["npx", "-y", "@upstash/context7-mcp"],
	      "args": ["legacy"],
	      "env": {"TOKEN": "x"},
	      "enabled": false
	    },
	    "engram": {
	      "type": "local",
	      "command": ["engram-server"],
	      "args": ["--port", "9000"]
	    }
	  }
	}`
	if err := os.WriteFile(configPath, []byte(legacy), 0o644); err != nil {
		t.Fatalf("WriteFile(opencode.json) error = %v", err)
	}

	_, err := Inject(home, home, adapter)
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	assertOpenCodeRemoteContext7Schema(t, configPath)

	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(opencode.json) error = %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("Unmarshal(opencode.json) error = %v", err)
	}

	mcp, ok := parsed["mcp"].(map[string]any)
	if !ok {
		t.Fatalf("opencode.json missing object key mcp; got %#v", parsed["mcp"])
	}

	engram, ok := mcp["engram"].(map[string]any)
	if !ok {
		t.Fatalf("opencode.json mcp.engram missing after inject; got %#v", mcp["engram"])
	}
	if engram["type"] != "local" {
		t.Fatalf("mcp.engram.type = %#v; want %q", engram["type"], "local")
	}
	cmd, _ := engram["command"].([]any)
	if len(cmd) == 0 || cmd[0] != "engram-server" {
		t.Fatalf("mcp.engram.command = %#v; want [engram-server ...]", engram["command"])
	}
}

func TestInjectClaudeWritesUserConfigAndIsIdempotent(t *testing.T) {
	home := t.TempDir()
	userConfigPath := filepath.Join(home, ".claude.json")
	userConfig := `{"oauthAccount":{"emailAddress":"user@example.com"},"projects":{"/repo":{"allowedTools":[]}},"mcpServers":{"codegraph":{"command":"codegraph","args":["serve","--mcp"]}}}`
	// Seeded intentionally loose: injection must tighten it to 0600.
	if err := os.WriteFile(userConfigPath, []byte(userConfig), 0o644); err != nil {
		t.Fatalf("WriteFile(user config) error = %v", err)
	}

	first, err := Inject(home, home, claudeAdapter())
	if err != nil {
		t.Fatalf("Inject() first error = %v", err)
	}
	if !first.Changed {
		t.Fatalf("Inject() first changed = false")
	}

	afterFirst, err := os.ReadFile(userConfigPath)
	if err != nil {
		t.Fatalf("ReadFile(user config after first) error = %v", err)
	}

	// Loosen the mode: the no-op run must still re-tighten 0600.
	if runtime.GOOS != "windows" {
		if err := os.Chmod(userConfigPath, 0o644); err != nil {
			t.Fatalf("Chmod(loosen) error = %v", err)
		}
	}
	second, err := Inject(home, home, claudeAdapter())
	if err != nil {
		t.Fatalf("Inject() second error = %v", err)
	}
	if second.Changed {
		t.Fatalf("Inject() second changed = true")
	}

	raw, err := os.ReadFile(userConfigPath)
	if err != nil {
		t.Fatalf("ReadFile(user config) error = %v", err)
	}
	if string(raw) != string(afterFirst) {
		t.Fatalf("second Inject() must leave ~/.claude.json byte-identical; got diff")
	}
	if info, statErr := os.Stat(userConfigPath); statErr != nil {
		t.Fatalf("Stat(user config) error = %v", statErr)
	} else if mode := info.Mode().Perm(); runtime.GOOS != "windows" && mode != 0o600 {
		t.Fatalf("~/.claude.json mode = %o; want 0600 (holds the OAuth session)", mode)
	}
	root := map[string]any{}
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("Unmarshal(user config) error = %v", err)
	}
	if _, ok := root["oauthAccount"].(map[string]any); !ok {
		t.Fatalf("oauthAccount must be preserved; got %s", raw)
	}
	if _, ok := root["projects"].(map[string]any); !ok {
		t.Fatalf("projects must be preserved; got %s", raw)
	}
	servers, _ := root["mcpServers"].(map[string]any)
	if codegraph, _ := servers["codegraph"].(map[string]any); codegraph["command"] != "codegraph" {
		t.Fatalf("existing codegraph registration must be preserved; got %#v", servers["codegraph"])
	}
	context7, _ := servers["context7"].(map[string]any)
	if context7["command"] != "npx" {
		t.Fatalf("mcpServers.context7.command = %#v; want npx", context7["command"])
	}
	if args := fmt.Sprintf("%v", context7["args"]); !strings.Contains(args, versions.Context7MCP) {
		t.Fatalf("context7.args = %s; want pinned version %s", args, versions.Context7MCP)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "mcp", "context7.json")); !os.IsNotExist(err) {
		t.Fatalf("Claude Context7 must not be written to ~/.claude/mcp/context7.json; stat err = %v", err)
	}
}

// TestInjectClaudeSettingsInertBlockCleanup: the inert settings.json block is
// removed when it only holds the managed context7 entry, and left untouched
// when it carries servers gentle-ai does not manage.
func TestInjectClaudeSettingsInertBlockCleanup(t *testing.T) {
	cases := []struct {
		name           string
		settings       string
		wantKeyRemoved bool
	}{
		{"managed-only block is removed", `{"theme":"dark","mcpServers":{"context7":{"command":"npx","args":["--","context7-mcp"]}}}`, true},
		{"foreign block is left alone", `{"theme":"dark","mcpServers":{"context7":{"command":"npx","args":["--","context7-mcp"]},"stranded":{"command":"stranded-server"}}}`, false},
		{"user-authored context7 is left alone", `{"theme":"dark","mcpServers":{"context7":{"command":"my-own-proxy"}}}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			settingsPath := filepath.Join(home, ".claude", "settings.json")
			if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
				t.Fatalf("MkdirAll(settings dir) error = %v", err)
			}
			if err := os.WriteFile(settingsPath, []byte(tc.settings), 0o644); err != nil {
				t.Fatalf("WriteFile(settings) error = %v", err)
			}

			if _, err := Inject(home, home, claudeAdapter()); err != nil {
				t.Fatalf("Inject() error = %v", err)
			}

			readMCPServersContext7Entry(t, filepath.Join(home, ".claude.json"))

			settingsRaw, err := os.ReadFile(settingsPath)
			if err != nil {
				t.Fatalf("ReadFile(settings) error = %v", err)
			}
			settings := map[string]any{}
			if err := json.Unmarshal(settingsRaw, &settings); err != nil {
				t.Fatalf("Unmarshal(settings) error = %v", err)
			}
			_, hasKey := settings["mcpServers"]
			if tc.wantKeyRemoved && hasKey {
				t.Fatalf("inert mcpServers key must be removed; got %s", settingsRaw)
			}
			if !tc.wantKeyRemoved && !hasKey {
				t.Fatalf("foreign mcpServers block must be left untouched; got %s", settingsRaw)
			}
			if settings["theme"] != "dark" {
				t.Fatalf("settings.theme = %#v; want dark preserved", settings["theme"])
			}
		})
	}
}

func TestInjectClaudeRefusesCorruptUserConfig(t *testing.T) {
	home := t.TempDir()
	userConfigPath := filepath.Join(home, ".claude.json")
	corrupt := []byte("{ this is not json")
	if err := os.WriteFile(userConfigPath, corrupt, 0o600); err != nil {
		t.Fatalf("WriteFile(corrupt user config) error = %v", err)
	}
	if _, err := Inject(home, home, claudeAdapter()); err == nil {
		t.Fatalf("Inject() error = nil; want refusal on corrupt ~/.claude.json")
	}
	after, err := os.ReadFile(userConfigPath)
	if err != nil {
		t.Fatalf("ReadFile(user config) error = %v", err)
	}
	if string(after) != string(corrupt) {
		t.Fatalf("corrupt ~/.claude.json must be left byte-identical; got %s", after)
	}
}

func TestInjectCursorWithMalformedMCPJsonRecovery(t *testing.T) {
	// Real Windows users may have a ~/.cursor/mcp.json that starts with non-JSON
	// content (e.g. "allow: all" or just "a"). The installer should recover by
	// treating the broken file as {} and proceeding with the overlay merge.
	home := t.TempDir()
	adapter := cursorAdapter(t)

	// Pre-create ~/.cursor/mcp.json with invalid (non-JSON) content.
	mcpPath := adapter.MCPConfigPath(home, "context7")
	if err := os.MkdirAll(filepath.Dir(mcpPath), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(mcpPath, []byte("allow: all"), 0o644); err != nil {
		t.Fatalf("WriteFile(malformed mcp.json) error = %v", err)
	}

	result, err := Inject(home, home, adapter)
	if err != nil {
		t.Fatalf("Inject(cursor) with malformed mcp.json error = %v; want nil (should recover)", err)
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
		t.Fatalf("mcp.json missing mcpServers key; got:\n%s", text)
	}
	if !strings.Contains(text, `"context7"`) {
		t.Fatalf("mcp.json missing context7 server entry; got:\n%s", text)
	}
}

// TestInjectCodexContext7TOML verifies that Context7 injection for Codex
// (StrategyTOMLFile) creates config.toml with [mcp_servers.context7] block.
func TestInjectCodexContext7TOML(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, home, codex.NewAdapter())
	if err != nil {
		t.Fatalf("Inject(codex) error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Inject(codex) first call changed = false; want true")
	}
	if len(result.Files) == 0 {
		t.Fatal("Inject(codex) files is empty; want config.toml path")
	}

	configTOML := filepath.Join(home, ".codex", "config.toml")
	if result.Files[0] != configTOML {
		t.Fatalf("Inject(codex) files[0] = %q; want %q", result.Files[0], configTOML)
	}

	content, err := os.ReadFile(configTOML)
	if err != nil {
		t.Fatalf("ReadFile(config.toml) error = %v", err)
	}
	text := string(content)

	if !strings.Contains(text, "[mcp_servers.context7]") {
		t.Fatalf("config.toml missing [mcp_servers.context7]; got:\n%s", text)
	}
	if !strings.Contains(text, `url = "https://mcp.context7.com/mcp"`) {
		t.Fatalf("config.toml missing Context7 remote URL; got:\n%s", text)
	}
	if strings.Contains(text, `command = "npx"`) || strings.Contains(text, "context7-mcp") {
		t.Fatalf("config.toml should use remote Context7 MCP, not local npx; got:\n%s", text)
	}
}

// TestInjectCodexContext7Idempotent verifies that a second Inject call with
// the same pinned version is a no-op (Changed == false, single block).
func TestInjectCodexContext7Idempotent(t *testing.T) {
	home := t.TempDir()

	first, err := Inject(home, home, codex.NewAdapter())
	if err != nil {
		t.Fatalf("Inject(codex) first error = %v", err)
	}
	if !first.Changed {
		t.Fatal("Inject(codex) first changed = false; want true")
	}

	second, err := Inject(home, home, codex.NewAdapter())
	if err != nil {
		t.Fatalf("Inject(codex) second error = %v", err)
	}
	if second.Changed {
		t.Fatal("Inject(codex) second changed = true; want false (idempotent)")
	}

	configTOML := filepath.Join(home, ".codex", "config.toml")
	content, err := os.ReadFile(configTOML)
	if err != nil {
		t.Fatalf("ReadFile(config.toml) error = %v", err)
	}
	count := strings.Count(string(content), "[mcp_servers.context7]")
	if count != 1 {
		t.Fatalf("config.toml has %d [mcp_servers.context7] blocks; want exactly 1", count)
	}
}

// TestInjectCodexContext7CoexistsWithEngram verifies that injecting context7
// into a config.toml that already has [mcp_servers.engram] preserves both
// blocks and does not duplicate either.
func TestInjectCodexContext7CoexistsWithEngram(t *testing.T) {
	home := t.TempDir()

	// Pre-seed config.toml with an engram block.
	configTOML := filepath.Join(home, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(configTOML), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	existing := `[mcp_servers.engram]
command = "engram"
args = ["mcp", "--tools=agent"]
`
	if err := os.WriteFile(configTOML, []byte(existing), 0o644); err != nil {
		t.Fatalf("WriteFile(config.toml) error = %v", err)
	}

	_, err := Inject(home, home, codex.NewAdapter())
	if err != nil {
		t.Fatalf("Inject(codex) error = %v", err)
	}

	content, err := os.ReadFile(configTOML)
	if err != nil {
		t.Fatalf("ReadFile(config.toml) error = %v", err)
	}
	text := string(content)

	engramCount := strings.Count(text, "[mcp_servers.engram]")
	if engramCount != 1 {
		t.Fatalf("expected 1 [mcp_servers.engram], got %d; result:\n%s", engramCount, text)
	}

	context7Count := strings.Count(text, "[mcp_servers.context7]")
	if context7Count != 1 {
		t.Fatalf("expected 1 [mcp_servers.context7], got %d; result:\n%s", context7Count, text)
	}
}

func TestInjectCodexContext7ReplacesLegacyLocalBlock(t *testing.T) {
	home := t.TempDir()
	configTOML := filepath.Join(home, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(configTOML), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	existing := `[mcp_servers.context7]
command = "npx"
args = ["-y", "--package=@upstash/context7-mcp@2.2.5", "--", "context7-mcp"]

[mcp_servers.engram]
command = "engram"
args = ["mcp", "--tools=agent"]
`
	if err := os.WriteFile(configTOML, []byte(existing), 0o644); err != nil {
		t.Fatalf("WriteFile(config.toml) error = %v", err)
	}

	result, err := Inject(home, home, codex.NewAdapter())
	if err != nil {
		t.Fatalf("Inject(codex) error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Inject(codex) changed = false; expected legacy local block migration")
	}

	content, err := os.ReadFile(configTOML)
	if err != nil {
		t.Fatalf("ReadFile(config.toml) error = %v", err)
	}
	text := string(content)

	if count := strings.Count(text, "[mcp_servers.context7]"); count != 1 {
		t.Fatalf("expected 1 [mcp_servers.context7], got %d; result:\n%s", count, text)
	}
	if !strings.Contains(text, `url = "https://mcp.context7.com/mcp"`) {
		t.Fatalf("config.toml missing remote Context7 URL after migration; got:\n%s", text)
	}
	if strings.Contains(text, `command = "npx"`) || strings.Contains(text, "context7-mcp") {
		t.Fatalf("legacy local Context7 config survived migration; got:\n%s", text)
	}
	if !strings.Contains(text, "[mcp_servers.engram]") {
		t.Fatalf("engram block was not preserved; got:\n%s", text)
	}
}

func TestInjectVSCodeWritesContext7ToMCPConfigFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	adapter := vscode.NewAdapter()

	first, err := Inject(home, home, adapter)
	if err != nil {
		t.Fatalf("Inject() first error = %v", err)
	}
	if !first.Changed {
		t.Fatalf("Inject() first changed = false")
	}

	second, err := Inject(home, home, adapter)
	if err != nil {
		t.Fatalf("Inject() second error = %v", err)
	}
	if second.Changed {
		t.Fatalf("Inject() second changed = true")
	}

	path := adapter.MCPConfigPath(home, "context7")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(mcp.json) error = %v", err)
	}

	text := string(content)
	if !strings.Contains(text, `"servers"`) {
		t.Fatal("mcp.json missing servers key")
	}
	if !strings.Contains(text, `"context7"`) {
		t.Fatal("mcp.json missing context7 server")
	}
	if strings.Contains(text, `"mcpServers"`) {
		t.Fatal("mcp.json should use 'servers' key, not 'mcpServers'")
	}
}

func TestInjectAntigravityReplacesLegacyContext7LocalConfig(t *testing.T) {
	home := t.TempDir()
	adapter := antigravityAdapter()
	configPath := adapter.MCPConfigPath(home, "context7")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}

	legacy := `{
	  "mcpServers": {
	    "context7": {
	      "command": "npx",
	      "args": ["-y", "@upstash/context7-mcp"],
	      "env": {"TOKEN": "x"}
	    }
	  }
	}`
	if err := os.WriteFile(configPath, []byte(legacy), 0o644); err != nil {
		t.Fatalf("WriteFile(mcp_config.json) error = %v", err)
	}

	first, err := Inject(home, home, adapter)
	if err != nil {
		t.Fatalf("Inject() first error = %v", err)
	}
	if !first.Changed {
		t.Fatalf("Inject() first changed = false; expected migration to rewrite legacy context7")
	}

	assertAntigravityContext7Schema(t, configPath)

	second, err := Inject(home, home, adapter)
	if err != nil {
		t.Fatalf("Inject() second error = %v", err)
	}
	if second.Changed {
		t.Fatalf("Inject() second changed = true; expected idempotent context7 rewrite")
	}

	assertAntigravityContext7Schema(t, configPath)
}

func TestInjectKimiWritesContext7ToMCPConfigFile(t *testing.T) {
	home := t.TempDir()

	first, err := Inject(home, home, kimiAdapter())
	if err != nil {
		t.Fatalf("Inject(kimi) first error = %v", err)
	}
	if !first.Changed {
		t.Fatalf("Inject(kimi) first changed = false")
	}

	second, err := Inject(home, home, kimiAdapter())
	if err != nil {
		t.Fatalf("Inject(kimi) second error = %v", err)
	}
	if second.Changed {
		t.Fatalf("Inject(kimi) second changed = true")
	}

	path := filepath.Join(home, ".kimi", "mcp.json")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(kimi mcp.json) error = %v", err)
	}

	text := string(content)
	if !strings.Contains(text, `"mcpServers"`) {
		t.Fatal("kimi mcp.json missing mcpServers key")
	}
	if !strings.Contains(text, `"context7"`) {
		t.Fatal("kimi mcp.json missing context7 server")
	}
	if !strings.Contains(text, `"transport": "http"`) {
		t.Fatal("kimi mcp.json should set transport=http for documented remote MCP configuration")
	}
	if !strings.Contains(text, `"url": "https://mcp.context7.com/mcp"`) {
		t.Fatal("kimi mcp.json should use the documented remote MCP URL for context7")
	}
}

func TestInjectKimiReplacesLegacyContext7LocalConfig(t *testing.T) {
	home := t.TempDir()
	adapter := kimiAdapter()
	configPath := adapter.MCPConfigPath(home, "context7")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}

	legacy := `{
	  "mcpServers": {
	    "context7": {
	      "command": "npx",
	      "args": ["-y", "@upstash/context7-mcp"],
	      "env": {"TOKEN": "x"}
	    }
	  }
	}`
	if err := os.WriteFile(configPath, []byte(legacy), 0o644); err != nil {
		t.Fatalf("WriteFile(kimi mcp.json) error = %v", err)
	}

	first, err := Inject(home, home, adapter)
	if err != nil {
		t.Fatalf("Inject(kimi) first error = %v", err)
	}
	if !first.Changed {
		t.Fatalf("Inject(kimi) first changed = false; expected migration to rewrite legacy context7")
	}

	assertKimiContext7Schema(t, configPath)

	second, err := Inject(home, home, adapter)
	if err != nil {
		t.Fatalf("Inject(kimi) second error = %v", err)
	}
	if second.Changed {
		t.Fatalf("Inject(kimi) second changed = true; expected idempotent context7 rewrite")
	}

	assertKimiContext7Schema(t, configPath)
}

// TestInjectHermesContext7IntoYAML verifies that Inject(hermes) writes context7
// under mcp_servers: in ~/.hermes/config.yaml (StrategyMergeIntoYAML).
func TestInjectHermesContext7IntoYAML(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, home, hermesAdapter())
	if err != nil {
		t.Fatalf("Inject(hermes) error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Inject(hermes) changed = false")
	}

	configPath := filepath.Join(home, ".hermes", "config.yaml")
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config.yaml) error = %v", err)
	}
	text := string(content)

	if !strings.Contains(text, "mcp_servers:") {
		t.Fatal("config.yaml missing mcp_servers: key")
	}
	if !strings.Contains(text, "context7:") {
		t.Fatal("config.yaml missing context7: entry under mcp_servers:")
	}
	if !strings.Contains(text, "context7-mcp") {
		t.Fatal("config.yaml missing context7-mcp in args")
	}
	if result.Files[0] != configPath {
		t.Fatalf("result.Files[0] = %q, want %q", result.Files[0], configPath)
	}
}

// TestInjectHermesContext7Idempotent verifies calling Inject twice yields exactly
// one context7: entry (idempotent upsert), and Changed=false on the second call.
func TestInjectHermesContext7Idempotent(t *testing.T) {
	home := t.TempDir()

	first, err := Inject(home, home, hermesAdapter())
	if err != nil {
		t.Fatalf("Inject(hermes) first error = %v", err)
	}
	if !first.Changed {
		t.Fatal("Inject(hermes) first changed = false")
	}

	second, err := Inject(home, home, hermesAdapter())
	if err != nil {
		t.Fatalf("Inject(hermes) second error = %v", err)
	}
	if second.Changed {
		t.Fatal("Inject(hermes) second changed = true (not idempotent)")
	}

	configPath := filepath.Join(home, ".hermes", "config.yaml")
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config.yaml) error = %v", err)
	}
	text := string(content)

	// Exactly one context7: entry should be present.
	count := strings.Count(text, "  context7:")
	if count != 1 {
		t.Fatalf("config.yaml has %d context7: entries, want 1:\n%s", count, text)
	}
}

// TestInjectHermesStrategyMergeIntoYAMLDispatches verifies that StrategyMergeIntoYAML
// no longer hits the hard-error default case in the strategy switch.
func TestInjectHermesStrategyMergeIntoYAMLDispatches(t *testing.T) {
	home := t.TempDir()

	// Confirm no error is returned (the old code returned an error for strategy 4).
	result, err := Inject(home, home, hermesAdapter())
	if err != nil {
		t.Fatalf("Inject(hermes) with StrategyMergeIntoYAML returned error = %v (expected nil)", err)
	}
	if !result.Changed {
		t.Fatal("Inject(hermes) Changed = false, want true on first run")
	}
}

// TestInjectHermesPreservesExistingTopLevelKeys verifies that a full Inject
// round-trip on a config.yaml that already contains an unrelated top-level key
// (e.g. "model: claude") preserves that key after context7 injection.
// This covers the review-flagged coverage gap: existing non-managed content
// must survive the MCP upsert.
func TestInjectHermesPreservesExistingTopLevelKeys(t *testing.T) {
	home := t.TempDir()
	hermesDir := filepath.Join(home, ".hermes")
	if err := os.MkdirAll(hermesDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Pre-existing config.yaml with an unrelated top-level key.
	initial := "model: claude\n"
	configPath := filepath.Join(hermesDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(initial), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	result, err := Inject(home, home, hermesAdapter())
	if err != nil {
		t.Fatalf("Inject(hermes) error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Inject(hermes) changed = false, want true on first run")
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config.yaml) error = %v", err)
	}
	text := string(content)

	// The pre-existing key must be preserved verbatim.
	if !strings.Contains(text, "model: claude") {
		t.Fatalf("config.yaml lost pre-existing top-level key 'model: claude':\n%s", text)
	}
	// The injected context7 entry must also be present.
	if !strings.Contains(text, "mcp_servers:") {
		t.Fatal("config.yaml missing mcp_servers: after injection")
	}
	if !strings.Contains(text, "context7:") {
		t.Fatal("config.yaml missing context7: after injection")
	}

	// Second Inject must be idempotent and still preserve the original key.
	second, err := Inject(home, home, hermesAdapter())
	if err != nil {
		t.Fatalf("Inject(hermes) second error = %v", err)
	}
	if second.Changed {
		t.Fatal("Inject(hermes) second changed = true (not idempotent)")
	}

	content2, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config.yaml) second error = %v", err)
	}
	text2 := string(content2)
	if !strings.Contains(text2, "model: claude") {
		t.Fatalf("config.yaml lost pre-existing key on second Inject:\n%s", text2)
	}
}
