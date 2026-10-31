//go:build linux || darwin

package shellinstaller

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func context7Stage(t *testing.T) string {
	t.Helper()
	root := userUnixTempRoot(t)
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "agent"), 0700); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestUserStageContext7Canonical(t *testing.T) {
	for _, choice := range []string{"on", "off"} {
		t.Run(choice, func(t *testing.T) {
			root := context7Stage(t)
			// A dangling config directory must not be inspected as a fallback.
			if err := os.Symlink("missing-persona", filepath.Join(root, "config")); err != nil {
				t.Fatal(err)
			}
			if err := userStageContext7(context.Background(), root, "separate", choice); err != nil {
				t.Fatal(err)
			}
			want := fmt.Sprintf("{\n  \"mcpServers\": {\n    \"context7\": {\n      \"url\": \"https://mcp.context7.com/mcp\",\n      \"enabled\": %t,\n      \"exposure\": \"codemode\"\n    }\n  }\n}\n", choice == "on")
			assertExperienceFile(t, filepath.Join(root, "agent", "mcp.json"), want)
			link, err := os.Readlink(filepath.Join(root, "config"))
			if err != nil || link != "missing-persona" {
				t.Fatalf("config/persona fallback changed: %q %v", link, err)
			}
		})
	}
	if err := userStageContext7(nil, "missing", "shared", ""); err != nil {
		t.Fatalf("absent preference must not inspect paths: %v", err)
	}
}

func TestUserStageContext7RefusesPreimages(t *testing.T) {
	for _, scenario := range []string{"shared-on", "shared-off", "mode", "invalid", "nil-context", "canceled", "relative", "existing", "symlink", "hardlink", "unrelated", "agent-mode", "root-mode", "agent-link"} {
		t.Run(scenario, func(t *testing.T) {
			root := context7Stage(t)
			agent := filepath.Join(root, "agent")
			path := filepath.Join(agent, "mcp.json")
			outside := filepath.Join(t.TempDir(), "canary")
			if err := os.WriteFile(outside, []byte("preserve malformed historical bytes\n"), 0600); err != nil {
				t.Fatal(err)
			}
			before, err := os.Stat(outside)
			if err != nil {
				t.Fatal(err)
			}
			ctx, mode, choice, callRoot := context.Background(), "separate", "on", root
			switch scenario {
			case "shared-on":
				mode = "shared"
			case "shared-off":
				mode, choice = "shared", "off"
			case "mode":
				mode = "other"
			case "invalid":
				choice = "inherit"
			case "nil-context":
				ctx = nil
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "relative":
				callRoot = "relative"
			case "existing":
				if err := os.WriteFile(path, []byte("preserve"), 0600); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink(outside, path); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(outside, path); err != nil {
					t.Fatal(err)
				}
			case "unrelated":
				if err := os.WriteFile(filepath.Join(agent, "history.json"), []byte("historical"), 0600); err != nil {
					t.Fatal(err)
				}
			case "agent-mode":
				if err := os.Chmod(agent, 0755); err != nil {
					t.Fatal(err)
				}
			case "root-mode":
				if err := os.Chmod(root, 0755); err != nil {
					t.Fatal(err)
				}
			case "agent-link":
				if err := os.Remove(agent); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Dir(outside), agent); err != nil {
					t.Fatal(err)
				}
			}
			entries, err := os.ReadDir(agent)
			if err != nil {
				t.Fatal(err)
			}
			agentBefore, err := os.Stat(agent)
			if err != nil {
				t.Fatal(err)
			}
			preimages := make(map[string]os.FileInfo)
			for _, entry := range entries {
				info, err := entry.Info()
				if err != nil {
					t.Fatal(err)
				}
				preimages[entry.Name()] = info
			}
			if err := userStageContext7(ctx, callRoot, mode, choice); err == nil {
				t.Fatal("unsafe canonical MCP stage was accepted")
			}
			afterEntries, err := os.ReadDir(agent)
			if err != nil || len(afterEntries) != len(entries) {
				t.Fatalf("refusal changed agent contents: %v", err)
			}
			agentAfter, err := os.Stat(agent)
			if err != nil || !os.SameFile(agentBefore, agentAfter) || agentBefore.Mode() != agentAfter.Mode() || !agentBefore.ModTime().Equal(agentAfter.ModTime()) {
				t.Fatal("refusal changed the agent directory preimage")
			}
			for _, entry := range afterEntries {
				fresh, err := entry.Info()
				prior := preimages[entry.Name()]
				if err != nil || prior == nil || !os.SameFile(prior, fresh) || prior.Mode() != fresh.Mode() || !prior.ModTime().Equal(fresh.ModTime()) {
					t.Fatal("refusal changed an agent entry preimage")
				}
			}
			after, err := os.Stat(outside)
			data, readErr := os.ReadFile(outside)
			if err != nil || readErr != nil || string(data) != "preserve malformed historical bytes\n" || !os.SameFile(before, after) || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
				t.Fatal("refusal changed the outside preimage")
			}
			if scenario == "existing" {
				assertExperienceFile(t, path, "preserve")
			}
			if scenario == "unrelated" {
				assertExperienceFile(t, filepath.Join(agent, "history.json"), "historical")
			}
		})
	}
}
