package pi

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// piEngramInitLauncherForTest is the node -e launcher gentle-engram's
// `pi-engram init` writes (gentle-engram 0.1.15 and 0.1.16 cli.js).
const piEngramInitLauncherForTest = "const { spawn } = require('node:child_process'); const bin = process.env.ENGRAM_BIN?.trim() ? process.env.ENGRAM_BIN : 'engram'; const child = spawn(bin, ['mcp', '--tools=agent'], { stdio: 'inherit' }); child.on('error', () => process.exit(127)); child.on('exit', (code, signal) => { if (typeof code === 'number') process.exit(code); process.kill(process.pid, signal || 'SIGTERM'); });"

func piEngramInitServerForTest() map[string]any {
	return map[string]any{
		"command":     "node",
		"args":        []any{"-e", piEngramInitLauncherForTest},
		"lifecycle":   "lazy",
		"directTools": false,
	}
}

func assertPiMCPServers(t *testing.T, path string, want map[string]any) {
	t.Helper()
	var config map[string]any
	readTestJSON(t, path, &config)
	if got := config["mcpServers"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("%s mcpServers = %#v, want %#v", path, got, want)
	}
}

func TestProvisionEngramMCPEnsuresMCPConfig(t *testing.T) {
	tests := []struct {
		name        string
		mcpAdapter  string
		mcp         string
		wantServers map[string]any
		wantKeys    map[string]any
	}{
		{
			name:       "only mcp-adapter.json migrates its servers into a new mcp.json",
			mcpAdapter: `{"mcpServers":{"context7":{"command":"npx","args":["context7-mcp"]},"engram":{"command":"engram","args":["mcp"]}},"settings":{"toolPrefix":"none"}}`,
			wantServers: map[string]any{
				"context7": map[string]any{"command": "npx", "args": []any{"context7-mcp"}},
				"engram":   map[string]any{"command": "engram", "args": []any{"mcp"}},
			},
		},
		{
			name:       "only mcp-adapter.json without engram gains the pi-engram init entry",
			mcpAdapter: `{"mcpServers":{"context7":{"command":"npx"}}}`,
			wantServers: map[string]any{
				"context7": map[string]any{"command": "npx"},
				"engram":   piEngramInitServerForTest(),
			},
		},
		{
			name:       "both files add only missing servers and keep mcp.json values and keys",
			mcpAdapter: `{"mcpServers":{"context7":{"command":"adapter-context7"},"analytics":{"command":"uvx"}}}`,
			mcp:        `{"activeMCP":"engram","mcpServers":{"context7":{"command":"npx"},"engram":{"command":"custom-engram"}}}`,
			wantServers: map[string]any{
				"context7":  map[string]any{"command": "npx"},
				"analytics": map[string]any{"command": "uvx"},
				"engram":    map[string]any{"command": "custom-engram"},
			},
			wantKeys: map[string]any{"activeMCP": "engram"},
		},
		{
			name:        "neither file creates mcp.json with the pi-engram init entry",
			wantServers: map[string]any{"engram": piEngramInitServerForTest()},
		},
		{
			name: "mcp.json without mcpServers keeps other keys and gains engram",
			mcp:  `{"imports":["cursor"]}`,
			wantServers: map[string]any{
				"engram": piEngramInitServerForTest(),
			},
			wantKeys: map[string]any{"imports": []any{"cursor"}},
		},
		{
			name:        "mcp-adapter.json without an mcpServers object is ignored",
			mcpAdapter:  `{"mcpServers":["not-an-object"]}`,
			wantServers: map[string]any{"engram": piEngramInitServerForTest()},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewAdapter()
			home := t.TempDir()
			setRealHome(t, home)
			agentDir := filepath.Join(t.TempDir(), "agent")
			t.Setenv("PI_CODING_AGENT_DIR", agentDir)
			mcpPath := filepath.Join(agentDir, "mcp.json")
			adapterPath := filepath.Join(agentDir, "mcp-adapter.json")
			if tt.mcpAdapter != "" {
				writeTestFile(t, adapterPath, tt.mcpAdapter)
			}
			if tt.mcp != "" {
				writeTestFile(t, mcpPath, tt.mcp)
			}

			changed, paths, err := a.ProvisionEngramMCP(home)
			if err != nil {
				t.Fatalf("ProvisionEngramMCP() error = %v", err)
			}
			if !changed || !reflect.DeepEqual(paths, []string{mcpPath}) {
				t.Fatalf("ProvisionEngramMCP() = (%v, %v), want (true, [%q])", changed, paths, mcpPath)
			}
			assertPiMCPServers(t, mcpPath, tt.wantServers)

			var config map[string]any
			readTestJSON(t, mcpPath, &config)
			for key, want := range tt.wantKeys {
				if got := config[key]; !reflect.DeepEqual(got, want) {
					t.Fatalf("mcp.json %q = %#v, want %#v preserved", key, got, want)
				}
			}

			if tt.mcpAdapter != "" {
				body, err := os.ReadFile(adapterPath)
				if err != nil {
					t.Fatalf("mcp-adapter.json must be kept: %v", err)
				}
				if string(body) != tt.mcpAdapter {
					t.Fatalf("mcp-adapter.json rewritten to %s, want byte-identical %s", body, tt.mcpAdapter)
				}
			} else if _, err := os.Stat(adapterPath); !os.IsNotExist(err) {
				t.Fatalf("stat mcp-adapter.json err = %v, want IsNotExist (never created)", err)
			}

			before, err := os.ReadFile(mcpPath)
			if err != nil {
				t.Fatalf("ReadFile(mcp.json) error = %v", err)
			}
			again, againPaths, err := a.ProvisionEngramMCP(home)
			if err != nil {
				t.Fatalf("ProvisionEngramMCP() second error = %v", err)
			}
			if again || len(againPaths) != 0 {
				t.Fatalf("ProvisionEngramMCP() second = (%v, %v), want idempotent no-op", again, againPaths)
			}
			after, err := os.ReadFile(mcpPath)
			if err != nil {
				t.Fatalf("ReadFile(mcp.json) second error = %v", err)
			}
			if string(after) != string(before) {
				t.Fatalf("second run rewrote mcp.json:\n got %s\nwant %s", after, before)
			}
		})
	}
}

func TestProvisionEngramMCPRefusesMalformedMCPConfigWithoutClobbering(t *testing.T) {
	tests := []struct {
		name       string
		file       string
		body       string
		wantSubstr string
	}{
		{name: "malformed mcp.json", file: "mcp.json", body: `{"mcpServers":`, wantSubstr: "unmarshal pi json file"},
		{name: "malformed mcp-adapter.json", file: "mcp-adapter.json", body: `{not json`, wantSubstr: "unmarshal pi json file"},
		{name: "mcp.json with non-object mcpServers", file: "mcp.json", body: `{"mcpServers":["engram"]}`, wantSubstr: "mcpServers"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewAdapter()
			home := t.TempDir()
			setRealHome(t, home)
			agentDir := filepath.Join(t.TempDir(), "agent")
			t.Setenv("PI_CODING_AGENT_DIR", agentDir)
			path := filepath.Join(agentDir, tt.file)
			writeTestFile(t, path, tt.body)

			_, _, err := a.ProvisionEngramMCP(home)
			if err == nil || !strings.Contains(err.Error(), tt.wantSubstr) || !strings.Contains(err.Error(), path) {
				t.Fatalf("ProvisionEngramMCP() error = %v, want error naming %q and containing %q", err, path, tt.wantSubstr)
			}
			body, readErr := os.ReadFile(path)
			if readErr != nil || string(body) != tt.body {
				t.Fatalf("%s after error = %q (err %v), want byte-identical %q", path, body, readErr, tt.body)
			}
			if tt.file == "mcp-adapter.json" {
				if _, statErr := os.Stat(filepath.Join(agentDir, "mcp.json")); !os.IsNotExist(statErr) {
					t.Fatalf("stat mcp.json err = %v, want IsNotExist after a malformed mcp-adapter.json", statErr)
				}
			}
		})
	}
}
