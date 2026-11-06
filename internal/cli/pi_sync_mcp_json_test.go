package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// setupPiSyncHost isolates a Pi sync run in a temporary home with stubbed
// command execution.
func setupPiSyncHost(t *testing.T) string {
	t.Helper()
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
	return home
}

func piEngramSyncSelection() model.Selection {
	return model.Selection{
		Agents:     []model.AgentID{model.AgentPi},
		Components: []model.ComponentID{model.ComponentEngram},
		Persona:    model.PersonaNeutral,
	}
}

// TestRunSyncPiEngramMigratesMCPAdapterConfigAndPassesVerification reproduces
// issue #5103: a Pi host that only has mcp-adapter.json (the file
// pi-mcp-adapter 3.x read) must pass post-sync verification, end sync with an
// mcp.json that carries its servers and no Engram server (Pi Engram is
// native-only), and keep mcp-adapter.json untouched.
func TestRunSyncPiEngramMigratesMCPAdapterConfigAndPassesVerification(t *testing.T) {
	home := setupPiSyncHost(t)
	agentDir := filepath.Join(home, ".pi", "agent")
	adapterPath := filepath.Join(agentDir, "mcp-adapter.json")
	adapterBody := `{"mcpServers":{"context7":{"command":"npx","args":["context7-mcp"]}}}`
	mustWriteFile(t, adapterPath, []byte(adapterBody))
	mcpPath := filepath.Join(agentDir, "mcp.json")

	selection := piEngramSyncSelection()
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
	if _, ok := config.MCPServers["context7"]; !ok {
		t.Fatalf("mcp.json servers = %s, missing migrated %q", body, "context7")
	}
	if _, ok := config.MCPServers["engram"]; ok {
		t.Fatalf("mcp.json servers = %s, want no engram server (Pi Engram is native-only)", body)
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

// TestRunSyncPiEngramWithoutMCPConfigPassesVerification covers a Pi host with
// neither mcp.json nor mcp-adapter.json: Pi Engram is native-only, so sync must
// pass verification without creating mcp.json.
func TestRunSyncPiEngramWithoutMCPConfigPassesVerification(t *testing.T) {
	home := setupPiSyncHost(t)
	agentDir := filepath.Join(home, ".pi", "agent")

	result, err := RunSyncWithSelection(home, piEngramSyncSelection())
	if err != nil {
		t.Fatalf("RunSyncWithSelection() error = %v", err)
	}
	if !result.Verify.Ready {
		t.Fatalf("post-sync verification ready = false, report = %#v", result.Verify)
	}
	for _, name := range []string{"mcp.json", "mcp-adapter.json"} {
		if _, err := os.Stat(filepath.Join(agentDir, name)); !os.IsNotExist(err) {
			t.Fatalf("stat %s err = %v, want IsNotExist (sync must not create it)", name, err)
		}
	}
}

// TestPiEngramMCPConfigIsBackedUpButNotVerified pins where the #5103 fix
// lives: Pi's mcp.json stays in the Engram path inventory so a migration write
// is backed up, but post-sync verification does not require it.
func TestPiEngramMCPConfigIsBackedUpButNotVerified(t *testing.T) {
	home := setupPiSyncHost(t)
	mcpPath := filepath.Join(home, ".pi", "agent", "mcp.json")
	selection := piEngramSyncSelection()
	adapters := resolveAdapters(selection.Agents)

	inventory := componentPathsWithWorkspaceScoped(home, "", ScopeGlobal, selection, adapters, model.ComponentEngram)
	if !containsPath(inventory, mcpPath) {
		t.Fatalf("Engram path inventory = %v, want %q kept for backups", inventory, mcpPath)
	}
	verified := verificationComponentPaths(home, "", ScopeGlobal, selection, adapters, model.ComponentEngram)
	if containsPath(verified, mcpPath) {
		t.Fatalf("Engram verification paths = %v, want %q excluded (Pi Engram is native-only)", verified, mcpPath)
	}
}
