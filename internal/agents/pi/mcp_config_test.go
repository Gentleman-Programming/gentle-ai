package pi

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func assertPiMCPServers(t *testing.T, path string, want map[string]any) {
	t.Helper()
	var config map[string]any
	readTestJSON(t, path, &config)
	if got := config["mcpServers"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("%s mcpServers = %#v, want %#v", path, got, want)
	}
}

// TestProvisionEngramMCPMigratesMCPAdapterServers covers the only mcp.json
// write Pi provisioning performs: servers from a legacy mcp-adapter.json that
// mcp.json lacks. Pi Engram is native-only (gentle-engram), so provisioning
// never adds an engram server of its own.
func TestProvisionEngramMCPMigratesMCPAdapterServers(t *testing.T) {
	tests := []struct {
		name        string
		mcpAdapter  string
		mcp         string
		wantServers map[string]any
		wantKeys    map[string]any
	}{
		{
			name:       "only mcp-adapter.json migrates its servers into a new mcp.json",
			mcpAdapter: `{"mcpServers":{"context7":{"command":"npx","args":["context7-mcp"]}},"settings":{"toolPrefix":"none"}}`,
			wantServers: map[string]any{
				"context7": map[string]any{"command": "npx", "args": []any{"context7-mcp"}},
			},
		},
		{
			name:       "an engram server the user kept in mcp-adapter.json is migrated like any other",
			mcpAdapter: `{"mcpServers":{"context7":{"command":"npx"},"engram":{"command":"engram","args":["mcp"]}}}`,
			wantServers: map[string]any{
				"context7": map[string]any{"command": "npx"},
				"engram":   map[string]any{"command": "engram", "args": []any{"mcp"}},
			},
		},
		{
			name:       "both files add only missing servers and keep mcp.json values and keys",
			mcpAdapter: `{"mcpServers":{"context7":{"command":"adapter-context7"},"analytics":{"command":"uvx"}}}`,
			mcp:        `{"activeMCP":"context7","mcpServers":{"context7":{"command":"npx"}}}`,
			wantServers: map[string]any{
				"context7":  map[string]any{"command": "npx"},
				"analytics": map[string]any{"command": "uvx"},
			},
			wantKeys: map[string]any{"activeMCP": "context7"},
		},
		{
			name:       "mcp.json without mcpServers keeps other keys and gains the adapter servers",
			mcpAdapter: `{"mcpServers":{"context7":{"command":"npx"}}}`,
			mcp:        `{"imports":["cursor"]}`,
			wantServers: map[string]any{
				"context7": map[string]any{"command": "npx"},
			},
			wantKeys: map[string]any{"imports": []any{"cursor"}},
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
			writeTestFile(t, adapterPath, tt.mcpAdapter)
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

			body, err := os.ReadFile(adapterPath)
			if err != nil {
				t.Fatalf("mcp-adapter.json must be kept: %v", err)
			}
			if string(body) != tt.mcpAdapter {
				t.Fatalf("mcp-adapter.json rewritten to %s, want byte-identical %s", body, tt.mcpAdapter)
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

// TestProvisionEngramMCPNeverAddsAnEngramServer pins the native-only contract:
// with nothing to migrate, mcp.json is neither created nor rewritten, and no
// engram server appears in it.
func TestProvisionEngramMCPNeverAddsAnEngramServer(t *testing.T) {
	tests := []struct {
		name       string
		mcpAdapter string
		mcp        string
	}{
		{name: "neither file"},
		{name: "mcp.json without an engram server", mcp: `{"mcpServers":{"context7":{"command":"npx"}}}`},
		{name: "mcp.json without mcpServers", mcp: `{"imports":["cursor"]}`},
		{name: "empty mcp-adapter.json servers", mcpAdapter: `{"mcpServers":{}}`},
		{name: "mcp-adapter.json without an mcpServers object", mcpAdapter: `{"mcpServers":["not-an-object"]}`},
		{name: "malformed mcp.json with nothing to migrate", mcp: `{"mcpServers":`},
		{name: "non-object mcpServers with nothing to migrate", mcp: `{"mcpServers":["context7"]}`},
		{
			name:       "every adapter server already in mcp.json",
			mcpAdapter: `{"mcpServers":{"context7":{"command":"adapter-context7"}}}`,
			mcp:        `{"mcpServers":{"context7":{"command":"npx"}}}`,
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
			if tt.mcpAdapter != "" {
				writeTestFile(t, filepath.Join(agentDir, "mcp-adapter.json"), tt.mcpAdapter)
			}
			if tt.mcp != "" {
				writeTestFile(t, mcpPath, tt.mcp)
			}

			changed, paths, err := a.ProvisionEngramMCP(home)
			if err != nil {
				t.Fatalf("ProvisionEngramMCP() error = %v", err)
			}
			if changed || len(paths) != 0 {
				t.Fatalf("ProvisionEngramMCP() = (%v, %v), want (false, []) with nothing to migrate", changed, paths)
			}

			body, err := os.ReadFile(mcpPath)
			if tt.mcp == "" {
				if !os.IsNotExist(err) {
					t.Fatalf("stat mcp.json err = %v (body %s), want IsNotExist: nothing to migrate creates nothing", err, body)
				}
				return
			}
			if err != nil || string(body) != tt.mcp {
				t.Fatalf("mcp.json = %q (err %v), want byte-identical %q", body, err, tt.mcp)
			}
			if strings.Contains(string(body), `"engram"`) {
				t.Fatalf("mcp.json = %s, want no engram server (Pi Engram is native-only)", body)
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
			if tt.file == "mcp.json" {
				writeTestFile(t, filepath.Join(agentDir, "mcp-adapter.json"), `{"mcpServers":{"context7":{"command":"npx"}}}`)
			}

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

// TestProvisionEngramMCPRefusesNonObjectMCPServersWhenMigrating keeps the
// non-object mcpServers guard for the write path: migrating into such a file
// would clobber it, so provisioning reports it instead.
func TestProvisionEngramMCPRefusesNonObjectMCPServersWhenMigrating(t *testing.T) {
	a := NewAdapter()
	home := t.TempDir()
	setRealHome(t, home)
	agentDir := filepath.Join(t.TempDir(), "agent")
	t.Setenv("PI_CODING_AGENT_DIR", agentDir)
	mcpPath := filepath.Join(agentDir, "mcp.json")
	mcpBody := `{"mcpServers":["context7"]}`
	writeTestFile(t, filepath.Join(agentDir, "mcp-adapter.json"), `{"mcpServers":{"context7":{"command":"npx"}}}`)
	writeTestFile(t, mcpPath, mcpBody)

	_, _, err := a.ProvisionEngramMCP(home)
	if err == nil || !strings.Contains(err.Error(), "mcpServers") || !strings.Contains(err.Error(), mcpPath) {
		t.Fatalf("ProvisionEngramMCP() error = %v, want error naming %q and mcpServers", err, mcpPath)
	}
	if body, readErr := os.ReadFile(mcpPath); readErr != nil || string(body) != mcpBody {
		t.Fatalf("mcp.json after error = %q (err %v), want byte-identical %q", body, readErr, mcpBody)
	}
}
