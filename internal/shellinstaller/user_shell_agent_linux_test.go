//go:build linux && amd64

package shellinstaller

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
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
	// The guest umask (0055) strips the group/other bits from Mkdir's
	// argument (0755 -> 0700); make the intended owned 0755 mode real
	// regardless of umask.
	if err := os.Chmod(agent, 0755); err != nil {
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
	// The Main SDK selection binds the agent as a private 0700 directory, so a
	// 0755 agent is refused up front, before any write, with the exact fix.
	before := agentTargetRecord(t, agent)
	if err := userValidateInstall(allowed); err == nil || !strings.Contains(err.Error(), "Main preview") || !strings.Contains(err.Error(), "chmod 700 "+strconv.Quote(agent)) {
		t.Fatalf("Main on a 0755 agent was not refused up front: %v", err)
	}
	if after := agentTargetRecord(t, agent); after != before {
		t.Fatalf("refused Main selection changed the agent:\n%s\n->\n%s", before, after)
	}
	if err := os.Chmod(agent, 0700); err != nil {
		t.Fatal(err)
	}
	if err := userValidateInstall(allowed); err != nil {
		t.Fatalf("Main was refused for a 0700 agent reuse: %v", err)
	}
	if err := os.Chmod(agent, 0755); err != nil {
		t.Fatal(err)
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
