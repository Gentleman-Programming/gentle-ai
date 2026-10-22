//go:build linux || darwin

package shellinstaller

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestApplyUserExperiencePreferencesInPlace(t *testing.T) {
	target, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0700); err != nil {
		t.Fatal(err)
	}
	agent := filepath.Join(target, "agent")
	writeInstallFixture(t, target, userManifest{Schema: userSchema, Destination: target, Prefix: filepath.Join(target, "prefix"), Agent: agent, Mode: "separate"})
	for path, data := range map[string]string{
		"config/persona.json":              `{"mode":"gentleman"}`,
		"config/background-subagents.json": `{"policy":"on"}`,
		"config/strict-tdd.json":           `{"policy":"on"}`,
		"home/.gentle-ai/state.json":       `{"rdd_mode":"on","other":true}`,
		"agent/mcp.json":                   `{"mcpServers":{"context7":{"enabled":true},"other":{"enabled":true}}}`,
	} {
		full := filepath.Join(target, path)
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	selected := UserExperience{Persona: "neutral", BackgroundSubagents: "off", StrictTDD: "off", GlobalRDD: "off", Context7: "off"}
	if err := ApplyUserExperiencePreferences(UserShellInstall{Target: target, Mode: "separate"}, selected); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadUserShellExperience(UserShellInstall{Target: target, Mode: "separate"})
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Persona != "neutral" || loaded.BackgroundSubagents != "off" || loaded.StrictTDD != "off" || loaded.GlobalRDD != "off" || loaded.Context7 != "off" {
		t.Fatalf("preferences differ: %+v", loaded)
	}
	state, err := os.ReadFile(filepath.Join(target, "home/.gentle-ai/state.json"))
	if err != nil || !json.Valid(state) || !containsBytes(state, `"other":true`) {
		t.Fatalf("state lost unrelated data: %s %v", state, err)
	}
	mcp, err := os.ReadFile(filepath.Join(agent, "mcp.json"))
	if err != nil || !containsBytes(mcp, `"other"`) || containsBytes(mcp, `"context7":{"enabled":true}`) {
		t.Fatalf("context7 merge differs: %s %v", mcp, err)
	}
	if err := ApplyUserExperiencePreferences(UserShellInstall{Target: target, Mode: "agent"}, selected); err == nil {
		t.Fatal("in-place preferences accepted an agent install")
	}
}

func containsBytes(data []byte, value string) bool {
	return len(value) == 0 || json.Valid(data) && stringContains(string(data), value)
}

func stringContains(haystack, needle string) bool {
	for index := 0; index+len(needle) <= len(haystack); index++ {
		if haystack[index:index+len(needle)] == needle {
			return true
		}
	}
	return false
}
