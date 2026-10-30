package mcp

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/versions"
)

func assertContext7FileUnchanged(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		t.Fatalf("Context7 config changed: error=%v\n%s", err, got)
	}
}

func TestInjectClaudeWorkspacePreservesCustomContext7(t *testing.T) {
	home, workspace := t.TempDir(), t.TempDir()
	custom := `{"mcpServers":{"context7":{"command":"npx","args":["-y","@upstash/context7-mcp@latest"],"env":{"TOKEN_SOURCE":"keychain"}}}}`
	settings := filepath.Join(workspace, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settings), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(workspace, ".mcp.json"), settings} {
		if err := os.WriteFile(path, []byte(custom), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := Inject(home, workspace, claudeAdapter())
	if err != nil || result.Changed {
		t.Fatalf("result=%+v error=%v", result, err)
	}
	assertContext7FileUnchanged(t, filepath.Join(workspace, ".mcp.json"), custom)
	assertContext7FileUnchanged(t, settings, custom)
}

func TestInjectContext7PreservesConfiguredServers(t *testing.T) {
	registry, err := agents.NewDefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range registry.SupportedAgents() {
		adapter, _ := registry.Get(id)
		if !adapter.SupportsMCP() {
			continue
		}
		for _, customization := range []string{"launcher", "remote", "version pin", "near-miss pin", "managed legacy"} {
			t.Run(string(id)+"/"+customization, func(t *testing.T) {
				home := t.TempDir()
				t.Setenv("HOME", home)
				t.Setenv("PI_CODING_AGENT_DIR", filepath.Join(home, ".pi", "agent"))
				t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
				t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
				first, err := Inject(home, home, adapter)
				if err != nil || !first.Changed || len(first.Files) == 0 {
					t.Fatalf("missing-entry install: result=%+v error=%v", first, err)
				}
				path := first.Files[0]
				installed, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				want := "@upstash/context7-mcp@" + versions.Context7MCP
				switch id {
				case model.AgentCodex, model.AgentOpenCode, model.AgentKilocode, model.AgentVSCodeCopilot, model.AgentAntigravity, model.AgentKimi:
					want = "https://mcp.context7.com/mcp"
				}
				if !bytes.Contains(installed, []byte(want)) {
					t.Fatalf("managed default missing %q: %s", want, installed)
				}
				pin := "latest"
				if customization == "near-miss pin" {
					pin = "4.2.0"
				}
				pinned := customization == "version pin" || customization == "near-miss pin"
				managed := customization == "managed legacy"
				var custom []byte
				switch adapter.MCPStrategy() {
				case model.StrategyTOMLFile:
					custom = []byte("# user config\n[mcp_servers.context7]\ncommand = '/custom/launcher'\nargs = ['--custom']\nenv = { TOKEN_SOURCE = 'keychain' }\n")
					if customization == "remote" {
						custom = []byte("[mcp_servers.\"context7\"] # custom endpoint\nurl = 'https://example.com/mcp'\n")
					}
					if pinned {
						custom = []byte("[mcp_servers.context7]\ncommand = 'npx'\nargs = ['-y', '--package=@upstash/context7-mcp@" + pin + "', '--', 'context7-mcp']\n")
					}
					if managed {
						custom = []byte("[mcp_servers.context7]\nargs = ['-y', '@upstash/context7-mcp']\ncommand = 'npx'\n")
					}
				case model.StrategyMergeIntoYAML:
					custom = []byte("# user config\nmcp_servers:\n  context7:\n    command: /custom/launcher\n    args: ['--custom']\n    env: {TOKEN_SOURCE: keychain}\n")
					if customization == "remote" {
						custom = []byte("mcp_servers: {context7: {url: 'https://example.com/mcp'}}\n")
					}
					if pinned {
						custom = []byte("mcp_servers:\n  context7:\n    command: npx\n    args: ['-y', '--package=@upstash/context7-mcp@" + pin + "', '--', 'context7-mcp']\n")
					}
					if managed {
						custom = []byte("mcp_servers:\n  context7:\n    args: ['-y', '@upstash/context7-mcp']\n    command: npx\n")
					}
				default:
					var root map[string]any
					if err := json.Unmarshal(installed, &root); err != nil {
						t.Fatal(err)
					}
					servers, _ := root["mcpServers"].(map[string]any)
					if servers == nil {
						servers, _ = root["servers"].(map[string]any)
					}
					if servers == nil {
						servers, _ = root["mcp"].(map[string]any)
						if nested, ok := servers["servers"].(map[string]any); ok {
							servers = nested
						}
					}
					entry := map[string]any{"command": "/custom/launcher", "args": []string{"--custom"}, "env": map[string]string{"TOKEN_SOURCE": "keychain"}}
					if customization == "remote" {
						entry = map[string]any{"url": "https://example.com/mcp", "headers": map[string]any{"X-Custom": "keep", "number": json.Number("9007199254740993")}}
					}
					if pinned {
						entry = map[string]any{"command": "npx", "args": []string{"-y", "--package=@upstash/context7-mcp@" + pin, "--", "context7-mcp"}}
					}
					if managed {
						entry = map[string]any{"command": "npx", "args": []string{"-y", "@upstash/context7-mcp"}}
					}
					if servers == nil {
						root = entry
					} else {
						servers["context7"] = entry
					}
					custom, err = json.MarshalIndent(root, "", "  ")
					if err != nil {
						t.Fatal(err)
					}
					custom = append(custom, '\n')
					if id == model.AgentOpenClaw && customization == "remote" {
						custom = append(custom, []byte(" \t\n")...)
					}
				}
				if err := os.WriteFile(path, custom, 0o600); err != nil {
					t.Fatal(err)
				}
				expected := custom
				if managed {
					expected = installed
				}
				for i := 0; i < 2; i++ {
					result, err := Inject(home, home, adapter)
					if err != nil {
						t.Fatal(err)
					}
					got, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(got, expected) {
						t.Fatalf("configured Context7 overwritten:\n%s", got)
					}
					if result.Changed != (managed && i == 0) {
						t.Fatalf("Changed=%v; want migration only on first managed call", result.Changed)
					}
				}
			})
		}
	}
}
