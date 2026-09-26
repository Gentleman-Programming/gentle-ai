package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v3/internal/system"
)

func TestOpenCodeUpgradeRetiresOwnedAgents(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(t *testing.T)
	}{
		{"install", func(t *testing.T) {
			t.Helper()
			if _, err := RunInstall([]string{"--agent", "opencode", "--component", "persona"}, system.DetectionResult{}); err != nil {
				t.Fatal(err)
			}
		}},
		{"sync", func(t *testing.T) {
			t.Helper()
			if _, err := RunSync([]string{"--agent", "opencode"}); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := installTestHome(t)
			path := filepath.Join(home, ".config", "opencode", "opencode.json")
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			fixture, err := os.ReadFile("testdata/opencode-v3.7.0-upgrade.json")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, fixture, 0600); err != nil {
				t.Fatal(err)
			}
			before, err := filemerge.UnmarshalJSONObject(fixture)
			if err != nil {
				t.Fatal(err)
			}
			tc.run(t)
			first, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(first, []byte(`"__managed_by"`)) {
				t.Fatal("legacy marker remains")
			}
			after, err := filemerge.UnmarshalJSONObject(first)
			if err != nil {
				t.Fatal(err)
			}
			agents := after["agent"].(map[string]any)
			original := before["agent"].(map[string]any)
			for _, key := range []string{"sdd-init", "sdd-apply", "general", "explore"} {
				if _, ok := agents[key]; ok {
					t.Errorf("retired owned agent %s remains", key)
				}
			}
			if !reflect.DeepEqual(after["provider"], before["provider"]) {
				t.Error("provider changed")
			}
			// Sync also refreshes managed MCP entries; the user's unrelated server stays intact.
			beforeMCP := before["mcp"].(map[string]any)
			afterMCP := after["mcp"].(map[string]any)
			if !reflect.DeepEqual(afterMCP["my-server"], beforeMCP["my-server"]) {
				t.Error("user MCP server changed")
			}
			if !reflect.DeepEqual(agents["user-owned"], original["user-owned"]) {
				t.Error("user-owned agent changed")
			}
			if !reflect.DeepEqual(agents["other-marked"], map[string]any{"prompt": "keep this"}) {
				t.Error("unknown marked agent changed beyond marker removal")
			}
			for _, key := range []string{"jd-judge-a", "review-risk", "review-refuter", "review-validator", "gentle-orchestrator"} {
				entry := agents[key].(map[string]any)
				if _, stale := entry["tools"]; stale {
					t.Errorf("%s retained stale tools", key)
				}
				if strings.Contains(entry["prompt"].(string), "obsolete") || strings.Contains(entry["prompt"].(string), "{file:./AGENTS.md}") {
					t.Errorf("%s retained stale prompt", key)
				}
			}
			judge := agents["jd-judge-a"].(map[string]any)
			if judge["model"] != "user/judge" || judge["variant"] != "low" {
				t.Errorf("judge model/variant lost: %v", judge)
			}
			tc.run(t)
			second, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(first, second) {
				t.Errorf("second %s changed settings bytes", tc.name)
			}
		})
	}
}

func TestOpenCodeUpgradeUnknownMarkerJSONC(t *testing.T) {
	for _, tc := range []struct {
		name      string
		input     string
		wantError bool
	}{
		{"preserve comments and v2 agents", `{
  // user note
  "agent": {"custom": {"prompt": "keep", "__managed_by": "gentle-ai/sdd"}, "other": {"__managed_by": "another-owner"}},
  "agents": {"native": {"__managed_by": "gentle-ai/sdd"}},
}`, false},
		{"refuse attached comment", `{"agent":{"custom":{"__managed_by": /* keep */ "gentle-ai/sdd","prompt":"keep"}}}`, true},
		{"refuse attached owned role comment", `{"agent":{"gentle-orchestrator":{"__managed_by":"gentle-ai/sdd",/* keep */"prompt":"old"}}}`, true},
		{"refuse duplicate keys", `{"agent":{"custom":{"__managed_by":"gentle-ai/sdd","__managed_by":"gentle-ai/sdd"}}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "opencode.jsonc")
			if err := os.WriteFile(path, []byte(tc.input), 0600); err != nil {
				t.Fatal(err)
			}
			changed, err := migrateLegacyOpenCodeAgents(path, "opencode")
			if tc.wantError {
				if err == nil {
					t.Fatal("expected refusal")
				}
				got, readErr := os.ReadFile(path)
				if readErr != nil || string(got) != tc.input {
					t.Fatalf("refusal changed settings: %v", readErr)
				}
				return
			}
			if err != nil || !changed {
				t.Fatalf("expected cleanup, changed=%v err=%v", changed, err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(got, []byte("// user note")) || !bytes.Contains(got, []byte(`"agents": {"native": {"__managed_by": "gentle-ai/sdd"}}`)) ||
				!bytes.Contains(got, []byte(`"other": {"__managed_by": "another-owner"}`)) || bytes.Contains(got, []byte(`"custom": {"prompt": "keep", "__managed_by"`)) {
				t.Fatalf("JSONC preservation/cleanup failed: %s", got)
			}
		})
	}
}

func TestOpenCodeUpgradePreservesUnmarkedGeneral(t *testing.T) {
	home := installTestHome(t)
	path := filepath.Join(home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"agent":{"general":{"prompt":"my general","model":"user/model"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := RunInstall([]string{"--agent", "opencode", "--component", "persona"}, system.DetectionResult{}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	root, err := filemerge.UnmarshalJSONObject(raw)
	if err != nil {
		t.Fatal(err)
	}
	general := root["agent"].(map[string]any)["general"]
	if !reflect.DeepEqual(general, map[string]any{"prompt": "my general", "model": "user/model"}) {
		t.Fatalf("unmarked general changed: %v", general)
	}
}
