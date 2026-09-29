package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// TestRunSyncPiEngramMigratesMCPAdapterConfigAndPassesVerification reproduces
// issue #5103: a Pi host that only has mcp-adapter.json (the file
// pi-mcp-adapter 3.x read) must end sync with an mcp.json that carries its
// servers plus Engram, so post-sync verification passes, and must keep
// mcp-adapter.json untouched.
func TestRunSyncPiEngramMigratesMCPAdapterConfigAndPassesVerification(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	}
	t.Setenv("PI_CODING_AGENT_DIR", "")
	t.Chdir(t.TempDir())

	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
	})
	osUserHomeDir = func() (string, error) { return home, nil }
	cmdLookPath = func(name string) (string, error) { return filepath.Join(home, "bin", name), nil }
	runCommand = func(string, ...string) error { return nil }

	agentDir := filepath.Join(home, ".pi", "agent")
	adapterPath := filepath.Join(agentDir, "mcp-adapter.json")
	adapterBody := `{"mcpServers":{"context7":{"command":"npx","args":["context7-mcp"]}}}`
	mustWriteFile(t, adapterPath, []byte(adapterBody))
	mcpPath := filepath.Join(agentDir, "mcp.json")

	selection := model.Selection{
		Agents:     []model.AgentID{model.AgentPi},
		Components: []model.ComponentID{model.ComponentEngram},
		Persona:    model.PersonaNeutral,
	}
	result, err := RunSyncWithSelection(home, selection)
	if err != nil {
		t.Fatalf("RunSyncWithSelection() error = %v", err)
	}
	if !result.Verify.Ready {
		t.Fatalf("post-sync verification ready = false, report = %#v", result.Verify)
	}

	var config struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	body, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatalf("ReadFile(mcp.json) error = %v", err)
	}
	if err := json.Unmarshal(body, &config); err != nil {
		t.Fatalf("mcp.json is not valid JSON: %v\n%s", err, body)
	}
	for _, name := range []string{"context7", "engram"} {
		if _, ok := config.MCPServers[name]; !ok {
			t.Fatalf("mcp.json servers = %s, missing %q", body, name)
		}
	}
	if !strings.Contains(string(config.MCPServers["engram"]), `"node"`) {
		t.Fatalf("mcp.json engram server = %s, want the pi-engram init node launcher", config.MCPServers["engram"])
	}

	if got, err := os.ReadFile(adapterPath); err != nil || string(got) != adapterBody {
		t.Fatalf("mcp-adapter.json after sync = %q (err %v), want byte-identical %q", got, err, adapterBody)
	}

	second, err := RunSyncWithSelection(home, selection)
	if err != nil {
		t.Fatalf("second RunSyncWithSelection() error = %v", err)
	}
	if !second.Verify.Ready {
		t.Fatalf("second post-sync verification ready = false, report = %#v", second.Verify)
	}
	again, err := os.ReadFile(mcpPath)
	if err != nil || string(again) != string(body) {
		t.Fatalf("second sync rewrote mcp.json (err %v):\n got %s\nwant %s", err, again, body)
	}
}
