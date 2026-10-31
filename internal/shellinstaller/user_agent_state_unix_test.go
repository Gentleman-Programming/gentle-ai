//go:build linux || darwin

package shellinstaller

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// carryFixture builds a retired agent (from) and a freshly provisioned stage
// agent (to) with exactly the installer-owned content.
func carryFixture(t *testing.T) (from, to string) {
	t.Helper()
	parent := userUnixTempRoot(t)
	from = filepath.Join(parent, "retired", "agent")
	to = filepath.Join(parent, "stage", "agent")
	for _, dir := range []string{
		filepath.Join(from, "sessions", "2026"), filepath.Join(from, "projects", "demo"),
		filepath.Join(to, "bin"),
	} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path, data string, mode os.FileMode) {
		if err := os.WriteFile(path, []byte(data), mode); err != nil {
			t.Fatal(err)
		}
	}
	// Retired operator state.
	write(filepath.Join(from, "auth.json"), `{"providers":{"anthropic":{"token":"secret"}}}`, 0600)
	write(filepath.Join(from, "sessions", "2026", "s1.json"), `{"session":1}`, 0600)
	write(filepath.Join(from, "projects", "demo", "state.json"), `{"p":1}`, 0600)
	write(filepath.Join(from, "settings.json"), `{"models":{"default":"claude-x"},"packages":["`+filepath.Join(parent, "retired", "prefix", "lib/node_modules/gentle-pi")+`"]}`, 0600)
	write(filepath.Join(from, "mcp.json"), `{"mcpServers":{"context7":{"url":"https://mcp.context7.com/mcp","enabled":false,"exposure":"codemode"},"custom":{"url":"https://example.test/mcp"}}}`, 0600)
	// Fresh installer-owned content.
	write(filepath.Join(to, "bin", "fd"), "fresh-fd", 0700)
	write(filepath.Join(to, "settings.json"), `{"packages":["`+filepath.Join(parent, "stage", "prefix", "lib/node_modules/gentle-pi")+`"],"npmCommand":["node","npm"]}`, 0600)
	write(filepath.Join(to, "mcp.json"), `{"mcpServers":{"context7":{"url":"https://mcp.context7.com/mcp","enabled":true,"exposure":"codemode"}}}`, 0600)
	return from, to
}

func TestUserCarryAgentStatePreservesOperatorState(t *testing.T) {
	ctx := context.Background()
	from, to := carryFixture(t)
	if err := userCarryAgentState(ctx, from, to); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(to, "auth.json"))
	if err != nil || string(data) != `{"providers":{"anthropic":{"token":"secret"}}}` {
		t.Fatalf("auth state differs: %q %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(to, "sessions", "2026", "s1.json")); err != nil {
		t.Fatalf("sessions lost: %v", err)
	}
	if _, err := os.Stat(filepath.Join(to, "projects", "demo", "state.json")); err != nil {
		t.Fatalf("projects lost: %v", err)
	}
	var settings map[string]json.RawMessage
	if data, err = os.ReadFile(filepath.Join(to, "settings.json")); err != nil {
		t.Fatal(err)
	}
	if json.Unmarshal(data, &settings) != nil {
		t.Fatalf("carried settings malformed: %s", data)
	}
	if string(settings["models"]) == "" {
		t.Fatalf("operator models lost: %s", data)
	}
	var packages []string
	if json.Unmarshal(settings["packages"], &packages) != nil || len(packages) != 1 || !filepath.IsAbs(packages[0]) || !hasSuffix(packages[0], "stage/prefix/lib/node_modules/gentle-pi", "stage\\prefix\\lib\\node_modules\\gentle-pi") {
		t.Fatalf("installer packages declaration lost: %s", data)
	}
	if _, ok := settings["npmCommand"]; !ok {
		t.Fatalf("installer npmCommand lost: %s", data)
	}
	var mcp struct {
		Servers map[string]json.RawMessage `json:"mcpServers"`
	}
	if data, err = os.ReadFile(filepath.Join(to, "mcp.json")); err != nil {
		t.Fatal(err)
	}
	if json.Unmarshal(data, &mcp) != nil {
		t.Fatalf("carried mcp malformed: %s", data)
	}
	if _, ok := mcp.Servers["custom"]; !ok {
		t.Fatalf("operator mcp server lost: %s", data)
	}
	if _, ok := mcp.Servers["context7"]; !ok {
		t.Fatalf("installer context7 entry lost: %s", data)
	}
	// The installer-owned context7 entry stays the fresh one, not the retired bit.
	var entry struct {
		Enabled bool `json:"enabled"`
	}
	if json.Unmarshal(mcp.Servers["context7"], &entry) != nil || !entry.Enabled {
		t.Fatalf("context7 entry differs: %s", data)
	}
	fresh, err := os.ReadFile(filepath.Join(to, "bin", "fd"))
	if err != nil || string(fresh) != "fresh-fd" {
		t.Fatalf("installer tooling replaced: %q %v", fresh, err)
	}
}

func TestUserCarryAgentStateRefusesUnexpectedCollision(t *testing.T) {
	ctx := context.Background()
	from, to := carryFixture(t)
	if err := os.WriteFile(filepath.Join(to, "stray.txt"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(from, "stray.txt"), []byte("y"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := userCarryAgentState(ctx, from, to); err == nil {
		t.Fatal("an unexpected collision must be refused, not overwritten")
	}
}

func TestUserCarryAgentStateToleratesAbsentState(t *testing.T) {
	ctx := context.Background()
	parent := userUnixTempRoot(t)
	from := filepath.Join(parent, "retired", "agent")
	to := filepath.Join(parent, "stage", "agent")
	if err := os.MkdirAll(to, 0700); err != nil {
		t.Fatal(err)
	}
	// A retired agent without any operator state and without settings/mcp
	// (legacy tree) carries nothing and keeps the fresh content.
	if err := userCarryAgentState(ctx, from, to); err == nil {
		t.Fatal("the retired agent must exist to carry state")
	}
	if err := os.MkdirAll(from, 0700); err != nil {
		t.Fatal(err)
	}
	if err := userCarryAgentState(ctx, from, to); err != nil {
		t.Fatalf("an empty retired agent carries nothing: %v", err)
	}
}

func hasSuffix(value string, unix, windows string) bool {
	return len(value) >= len(unix) && (value[len(value)-len(unix):] == unix || value[len(value)-len(windows):] == windows)
}
