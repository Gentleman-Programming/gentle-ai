//go:build linux && amd64

package shellinstaller

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sharedStagePaths(t *testing.T) (root, prefix, agent string) {
	t.Helper()
	parent := t.TempDir()
	if err := os.Chmod(parent, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"prefix", "agent"} {
		if err := os.Mkdir(filepath.Join(parent, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(parent, "target"), filepath.Join(parent, "prefix"), filepath.Join(parent, "agent")
}

func sharedDefaultsProfile() UserExperience {
	profile := DefaultUserExperience()
	profile.StrictTDD, profile.GlobalRDD, profile.CodeGraph, profile.Context7, profile.GGA = "on", "on", "on", "on", "off"
	return profile
}

// Shared keeps the launcher-private preferences in the stage root and defers
// Context7 to the existing agent so its recovery preimages are used.
func TestUserSharedDefaultsStageConsumesSelections(t *testing.T) {
	root, _, _ := sharedStagePaths(t)
	profile := sharedDefaultsProfile()
	if err := userPrepareInstallStage(context.Background(), root, "shared", &profile); err != nil {
		t.Fatalf("shared defaults stage refused: %v", err)
	}
	assertExperienceFile(t, filepath.Join(root, "config/strict-tdd.json"), "{\n  \"schema\": \"gentle-pi.strict-tdd/v1\",\n  \"policy\": \"on\"\n}\n")
	state, err := os.ReadFile(filepath.Join(root, "home/.gentle-ai/state.json"))
	if err != nil || !strings.Contains(string(state), `"rdd_mode": "on"`) {
		t.Fatalf("shared RDD state differs: %s %v", state, err)
	}
	for _, path := range []string{"agent/mcp.json"} {
		if _, err := os.Lstat(filepath.Join(root, path)); !os.IsNotExist(err) {
			t.Fatalf("shared stage wrote the private agent instead of the existing one: %s %v", path, err)
		}
	}
}

func TestUserSharedContext7MergesAgent(t *testing.T) {
	_, _, agent := sharedStagePaths(t)
	preimage := `{"mcpServers":{"engram":{"command":"keep-me"}},"producer_owned":true}`
	if err := os.WriteFile(filepath.Join(agent, "mcp.json"), []byte(preimage), 0600); err != nil {
		t.Fatal(err)
	}
	if err := userStageContext7Shared(context.Background(), agent, "on"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(agent, "mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fields struct {
		MCPServers map[string]struct {
			Command  string `json:"command"`
			URL      string `json:"url"`
			Enabled  bool   `json:"enabled"`
			Exposure string `json:"exposure"`
		} `json:"mcpServers"`
		ProducerOwned bool `json:"producer_owned"`
	}
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("shared Context7 wrote invalid JSON: %v", err)
	}
	context7 := fields.MCPServers["context7"]
	if !fields.ProducerOwned || fields.MCPServers["engram"].Command != "keep-me" || context7.URL != "https://mcp.context7.com/mcp" || !context7.Enabled || context7.Exposure != "codemode" {
		t.Fatalf("shared Context7 did not preserve the agent: %s", data)
	}
	if info, err := os.Lstat(filepath.Join(agent, "mcp.json")); err != nil || info.Mode() != 0600 {
		t.Fatalf("shared Context7 permissions differ: %v %v", info, err)
	}
	if err := userStageContext7Shared(context.Background(), agent, "off"); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(filepath.Join(agent, "mcp.json"))
	if !strings.Contains(string(data), `"enabled": false`) || !strings.Contains(string(data), `"command": "keep-me"`) {
		t.Fatalf("shared OFF dropped the preimage: %s", data)
	}
}

func TestUserSharedContext7RefusesUnsafePreimage(t *testing.T) {
	_, _, agent := sharedStagePaths(t)
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte(`{"keep":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(agent, "mcp.json")); err != nil {
		t.Fatal(err)
	}
	if err := userStageContext7Shared(context.Background(), agent, "on"); err == nil {
		t.Fatal("shared Context7 followed a symlinked preimage")
	}
	if data, err := os.ReadFile(outside); err != nil || string(data) != `{"keep":true}` {
		t.Fatalf("refusal changed the outside file: %s %v", data, err)
	}
}

func TestUserSharedDefaultsRefuseMain(t *testing.T) {
	root, prefix, agent := sharedStagePaths(t)
	profile := sharedDefaultsProfile()
	profile.SDKSource = "main-preview"
	req := UserInstallRequest{Destination: root, Mode: "shared", SharedPrefix: prefix, SharedAgent: agent, Experience: &profile}
	if err := userValidateInstall(req); err == nil || !strings.Contains(err.Error(), "Main preview is not available with the retained Shared prefix") {
		t.Fatalf("shared Main preview was not refused precisely: %v", err)
	}
}
