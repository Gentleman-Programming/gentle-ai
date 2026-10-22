//go:build linux && amd64

package shellinstaller

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func agentModeFixture(t *testing.T) (target, agent string) {
	t.Helper()
	parent := t.TempDir()
	if err := os.Chmod(parent, 0700); err != nil {
		t.Fatal(err)
	}
	agent = filepath.Join(parent, "agent")
	if err := os.Mkdir(agent, 0755); err != nil {
		t.Fatal(err)
	}
	target = filepath.Join(parent, "installed")
	return target, agent
}

func TestUserValidateAgentReuse(t *testing.T) {
	target, agent := agentModeFixture(t)
	profile := DefaultUserExperience()
	profile.StrictTDD, profile.GlobalRDD, profile.CodeGraph, profile.Context7 = "on", "on", "on", "on"
	req := UserInstallRequest{Destination: target, Mode: "agent", SharedAgent: agent, Experience: &profile}
	if err := userValidateInstall(req); err != nil {
		t.Fatalf("owned 0755 agent was refused: %v", err)
	}
	main := profile
	main.SDKSource = "main-preview"
	allowed := req
	allowed.Experience = &main
	if err := userValidateInstall(allowed); err != nil {
		t.Fatalf("Main was refused for agent reuse: %v", err)
	}
	gga := profile
	gga.GGA, gga.GGAProvider = "on", "codex"
	allowed.Experience = &gga
	if err := userValidateInstall(allowed); err != nil {
		t.Fatalf("GGA was refused for agent reuse: %v", err)
	}
	withPrefix := req
	withPrefix.SharedPrefix = filepath.Join(filepath.Dir(agent), "prefix")
	if err := userValidateInstall(withPrefix); err == nil {
		t.Fatal("agent reuse accepted a shared prefix")
	}
}

func TestUserStageOwnedAgentContext7(t *testing.T) {
	_, agent := agentModeFixture(t)
	stage := filepath.Join(t.TempDir(), "stage")
	if err := os.Mkdir(stage, 0700); err != nil {
		t.Fatal(err)
	}
	preimage := `{"mcpServers":{"engram":{"command":"keep-me"}},"producer_owned":true}`
	if err := os.WriteFile(filepath.Join(agent, "mcp.json"), []byte(preimage), 0600); err != nil {
		t.Fatal(err)
	}
	if err := userStageOwnedAgentContext7(context.Background(), agent, "on", stage); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(agent, "mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"command": "keep-me"`) || !strings.Contains(string(data), `"context7"`) || !strings.Contains(string(data), `"enabled": true`) {
		t.Fatalf("agent mcp merge differs: %s", data)
	}
	backup, err := os.ReadFile(filepath.Join(stage, "state/agent.preimage/mcp.json"))
	if err != nil || string(backup) != preimage {
		t.Fatalf("agent preimage differs: %s %v", backup, err)
	}
	if info, err := os.Lstat(agent); err != nil || info.Mode().Perm() != 0755 {
		t.Fatalf("agent directory mode changed: %v %v", info, err)
	}
}

func TestUserStageOwnedAgentRefusesSymlink(t *testing.T) {
	_, agent := agentModeFixture(t)
	stage := filepath.Join(t.TempDir(), "stage")
	if err := os.Mkdir(stage, 0700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte(`{"keep":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(agent, "mcp.json")); err != nil {
		t.Fatal(err)
	}
	if err := userStageOwnedAgentContext7(context.Background(), agent, "on", stage); err == nil {
		t.Fatal("agent reuse followed a symlinked mcp.json")
	}
	if data, err := os.ReadFile(outside); err != nil || string(data) != `{"keep":true}` {
		t.Fatalf("refusal changed the outside file: %s %v", data, err)
	}
}
