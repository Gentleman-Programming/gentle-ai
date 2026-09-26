package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/components/opencodeagents"
	"github.com/gentleman-programming/gentle-ai/v3/internal/system"
)

func TestUninstallRemovesUntouchedInstalledOrchestrator(t *testing.T) {
	if testing.Short() {
		t.Skip("full install and uninstall integration")
	}
	home := installTestHome(t)
	t.Setenv("HOME", home)
	if _, err := RunInstall([]string{"--agent", "opencode", "--preset", "full-gentleman"}, system.DetectionResult{}); err != nil {
		t.Fatal(err)
	}
	if _, err := RunUninstall([]string{"--agent", "opencode", "--yes"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(home, ".config", "opencode", "opencode.json"))
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		t.Fatal(err)
	}
	if agents, ok := root["agent"].(map[string]any); ok && agents["gentle-orchestrator"] != nil {
		t.Fatalf("untouched managed orchestrator retained: %s", body)
	}
}

func TestFullPresetUninstallOpenCodeFamilyAgents(t *testing.T) {
	if testing.Short() {
		t.Skip("full install and uninstall integration")
	}
	for _, tc := range []struct{ agent, directory string }{{"opencode", "opencode"}, {"kilocode", "kilo"}} {
		t.Run(tc.agent, func(t *testing.T) {
			home := installTestHome(t)
			t.Setenv("HOME", home)
			path := filepath.Join(home, ".config", tc.directory, "opencode.json")
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(`{"default_agent":"build"}`), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := RunInstall([]string{"--agent", tc.agent, "--preset", "full-gentleman"}, system.DetectionResult{}); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var root map[string]any
			if err := json.Unmarshal(before, &root); err != nil {
				t.Fatal(err)
			}
			agents := root["agent"].(map[string]any)
			if agents["jd-judge-a"] == nil || agents["jd-judge-b"] == nil {
				t.Fatalf("install did not produce managed agents: %s", before)
			}
			agents["my-agent"] = map[string]any{"prompt": "User owned"}
			agents["jd-judge-a"].(map[string]any)["prompt"] = "User modified judge"
			orchestrator := agents["gentle-orchestrator"].(map[string]any)
			permission := orchestrator["permission"].(map[string]any)
			task := permission["task"].(map[string]any)
			task["my-agent"] = "allow"
			task["jd-judge-a"] = "allow"
			modified, err := json.Marshal(root)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, modified, 0600); err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			result, err := RunUninstall([]string{"--agent", tc.agent, "--yes"}, &output)
			if err != nil {
				t.Fatal(err)
			}
			if result.BackupPath == "" {
				t.Fatal("uninstall did not create backup")
			}
			first, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var after map[string]any
			if err := json.Unmarshal(first, &after); err != nil {
				t.Fatal(err)
			}
			if tc.agent == "opencode" && after["default_agent"] != "build" {
				t.Fatalf("default_agent not restored: %s", first)
			}
			remaining := after["agent"].(map[string]any)
			if remaining["my-agent"] == nil || remaining["jd-judge-a"].(map[string]any)["prompt"] != "User modified judge" {
				t.Fatalf("user agents lost: %s", first)
			}
			for name, value := range remaining {
				entry, ok := value.(map[string]any)
				if !ok {
					continue
				}
				owned, err := opencodeagents.Shape(name, entry)
				if err != nil {
					t.Fatal(err)
				}
				if name == "gentleman" {
					// The installed persona overlay is also managed by gentle-ai.
					if name == "gentleman" {
						t.Fatalf("gentleman retained: %s", first)
					}
				}
				if owned {
					t.Fatalf("managed %s retained: %s", name, first)
				}
			}
			if owner, ok := remaining["gentle-orchestrator"].(map[string]any); ok {
				prompt, _ := owner["prompt"].(string)
				if bytes.Contains([]byte(prompt), []byte("<!-- gentle-ai:")) {
					t.Fatalf("orchestrator markers retained: %s", first)
				}
				tasks := owner["permission"].(map[string]any)["task"].(map[string]any)
				if tasks["my-agent"] != "allow" || tasks["jd-judge-a"] != "allow" {
					t.Fatalf("user tasks lost: %s", first)
				}
				if _, ok := tasks["jd-judge-b"]; ok {
					t.Fatalf("managed task retained: %s", first)
				}
			}
			second, err := RunUninstall([]string{"--agent", tc.agent, "--yes"}, &output)
			if err != nil {
				t.Fatal(err)
			}
			final, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(first, final) || len(second.ChangedFiles) != 0 {
				t.Fatalf("second uninstall changed settings: %s", final)
			}
		})
	}
}
