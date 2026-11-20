package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/engram"
)

// TestEngramReachablePassesOnAntigravityOnlyHost pins #5607: Antigravity's
// Engram registration lives only in the gentle-ai-engram plugin, so the check
// warned "no persisted MCP configuration" on Antigravity-only hosts.
func TestEngramReachablePassesOnAntigravityOnlyHost(t *testing.T) {
	t.Setenv(engramHealthEnvVar, "")
	setStdioProbeForTest(t, nil)
	homeDir := t.TempDir()
	pluginMCP := filepath.Join(engram.AntigravityPluginDir(homeDir), "mcp_config.json")
	if err := os.MkdirAll(filepath.Dir(pluginMCP), 0o755); err != nil {
		t.Fatalf("MkdirAll(plugin) error = %v", err)
	}
	content := `{"mcpServers":{"engram":{"command":"/configured/agy-engram","args":["mcp","--tools=agent"]}}}`
	if err := os.WriteFile(pluginMCP, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(plugin mcp) error = %v", err)
	}

	got := checkEngramReachable(context.Background(), homeDir, []string{"antigravity"})

	if got.Status != CheckStatusPass {
		t.Fatalf("status = %s (%s), want pass from the plugin-owned registration", got.Status, got.Detail)
	}
	if !strings.Contains(got.Detail, pluginMCP) {
		t.Fatalf("detail = %q, want it to name the plugin registration %q", got.Detail, pluginMCP)
	}
}

// TestEngramReachablePassesWithGlobalMCPAntigravityHost pins #5988: Antigravity's
// Engram registration in ~/.gemini/config/mcp_config.json is discovered and probed.
func TestEngramReachablePassesWithGlobalMCPAntigravityHost(t *testing.T) {
	t.Setenv(engramHealthEnvVar, "")
	setStdioProbeForTest(t, nil)
	homeDir := t.TempDir()
	globalMCP := filepath.Join(homeDir, ".gemini", "config", "mcp_config.json")
	if err := os.MkdirAll(filepath.Dir(globalMCP), 0o755); err != nil {
		t.Fatalf("MkdirAll(global) error = %v", err)
	}
	content := `{"mcpServers":{"engram":{"command":"/configured/agy-engram","args":["mcp","--tools=agent"]}}}`
	if err := os.WriteFile(globalMCP, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(global mcp) error = %v", err)
	}

	got := checkEngramReachable(context.Background(), homeDir, []string{"antigravity"})

	if got.Status != CheckStatusPass {
		t.Fatalf("status = %s (%s), want pass from global MCP registration", got.Status, got.Detail)
	}
	if !strings.Contains(got.Detail, globalMCP) {
		t.Fatalf("detail = %q, want it to name the global MCP registration %q", got.Detail, globalMCP)
	}
}
