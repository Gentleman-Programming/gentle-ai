package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/agents"
	"github.com/gentleman-programming/gentle-ai/internal/agents/claude"
	"github.com/gentleman-programming/gentle-ai/internal/agents/codex"
	"github.com/gentleman-programming/gentle-ai/internal/agents/kimi"
	"github.com/gentleman-programming/gentle-ai/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/internal/agents/vscode"
)

func cursorAdapter(t *testing.T) agents.Adapter {
	t.Helper()
	adapter, err := agents.NewAdapter("cursor")
	if err != nil {
		t.Fatalf("NewAdapter(cursor) error = %v", err)
	}
	return adapter
}

func claudeAdapter() agents.Adapter   { return claude.NewAdapter() }
func kimiAdapter() agents.Adapter     { return kimi.NewAdapter() }
func opencodeAdapter() agents.Adapter { return opencode.NewAdapter() }

func TestInjectOpenCodeMergesContext7AndIsIdempotent(t *testing.T) {
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

	configPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	config, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(opencode.json) error = %v", err)
	}

	if len(config) == 0 {
		t.Fatalf("opencode.json is empty")
	}

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

func TestInjectClaudeWritesContext7FileAndIsIdempotent(t *testing.T) {
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

	path := filepath.Join(home, ".claude", "mcp", "context7.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected context7 file %q: %v", path, err)
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

	result, err := Inject(home, adapter)
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

// TestInjectCodexTOMLStrategyIsSkipped verifies that Context7 injection for
// Codex (StrategyTOMLFile) is a no-op — Codex does not get Context7 via MCP
// config since there is no JSON-based config path; it receives Context7 via
// its system prompt (agents.md) instead.
func TestInjectCodexTOMLStrategyIsSkipped(t *testing.T) {
	home := t.TempDir()

	result, err := Inject(home, codex.NewAdapter())
	if err != nil {
		t.Fatalf("Inject(codex) error = %v; want nil (TOML strategy must not error)", err)
	}
	if result.Changed {
		t.Fatal("Inject(codex) changed = true; want false (TOML strategy should be a no-op for context7)")
	}
	if len(result.Files) != 0 {
		t.Fatalf("Inject(codex) files = %v; want empty", result.Files)
	}

	// config.toml must NOT be created by the context7 injector.
	configTOML := filepath.Join(home, ".codex", "config.toml")
	if _, err := os.Stat(configTOML); err == nil {
		t.Fatal("config.toml should NOT be written by the context7 injector")
	}
}

func TestInjectVSCodeWritesContext7ToMCPConfigFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	adapter := vscode.NewAdapter()

	first, err := Inject(home, adapter)
	if err != nil {
		t.Fatalf("Inject() first error = %v", err)
	}
	if !first.Changed {
		t.Fatalf("Inject() first changed = false")
	}

	second, err := Inject(home, adapter)
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

func TestInjectKimiWritesContext7ToMCPConfigFile(t *testing.T) {
	home := t.TempDir()

	first, err := Inject(home, kimiAdapter())
	if err != nil {
		t.Fatalf("Inject(kimi) first error = %v", err)
	}
	if !first.Changed {
		t.Fatalf("Inject(kimi) first changed = false")
	}

	second, err := Inject(home, kimiAdapter())
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
