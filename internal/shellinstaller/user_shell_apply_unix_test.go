//go:build linux || darwin

package shellinstaller

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestApplyUserExperiencePreferencesInPlace(t *testing.T) {
	target := userUnixTempRoot(t)
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
	if err != nil || !json.Valid(state) || !containsBytes(state, `"other": true`) {
		t.Fatalf("state lost unrelated data: %s %v", state, err)
	}
	mcp, err := os.ReadFile(filepath.Join(agent, "mcp.json"))
	if err != nil || !containsBytes(mcp, `"other"`) || !containsBytes(mcp, `"enabled": false`) || containsBytes(mcp, `"enabled": true,`) {
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

// Manage "Apply preferences" must write the same canonical bytes the install
// stage writes: gentle-pi ignores a policy file without its schema, and a
// Context7 entry without url/exposure is not a usable MCP server.
func TestApplyUserExperiencePreferencesWritesInstallCanonicalBytes(t *testing.T) {
	for _, tt := range []struct {
		name    string
		context string
	}{
		{"canonical-context7", `{"mcpServers":{"context7":{"url":"https://mcp.context7.com/mcp","enabled":true,"exposure":"codemode"},"other":{"enabled":true}}}`},
		{"legacy-apply-context7", `{"mcpServers":{"context7":{"enabled":true},"other":{"enabled":true}}}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			target := userUnixTempRoot(t)
			agent := filepath.Join(target, "agent")
			writeInstallFixture(t, target, userManifest{Schema: userSchema, Destination: target, Prefix: filepath.Join(target, "prefix"), Agent: agent, Mode: "separate"})
			installed := UserExperience{Persona: "gentleman", BackgroundSubagents: "on", StrictTDD: "on", GlobalRDD: "on", Context7: "on"}
			files, err := userExperienceConfigFiles(&installed)
			if err != nil {
				t.Fatal(err)
			}
			fixture := map[string]string{
				"home/.gentle-ai/state.json": `{"rdd_mode":"on","rdd_mode_recorded_at":"2026-01-01T00:00:00Z","other":true}`,
				"agent/mcp.json":             tt.context,
			}
			for _, file := range files {
				fixture["config/"+file.name] = string(file.data)
			}
			for path, data := range fixture {
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
			want, err := userExperienceConfigFiles(&selected)
			if err != nil {
				t.Fatal(err)
			}
			for _, file := range want {
				assertExperienceFile(t, filepath.Join(target, "config", file.name), string(file.data))
			}
			assertExperienceFile(t, filepath.Join(target, "config/strict-tdd.json"), "{\n  \"schema\": \"gentle-pi.strict-tdd/v1\",\n  \"policy\": \"off\"\n}\n")
			var mcp struct {
				Servers map[string]map[string]any `json:"mcpServers"`
			}
			data, err := os.ReadFile(filepath.Join(agent, "mcp.json"))
			if err != nil || json.Unmarshal(data, &mcp) != nil {
				t.Fatalf("mcp.json unreadable: %s %v", data, err)
			}
			entry := mcp.Servers["context7"]
			if len(entry) != 3 || entry["url"] != "https://mcp.context7.com/mcp" || entry["enabled"] != false || entry["exposure"] != "codemode" || mcp.Servers["other"]["enabled"] != true {
				t.Fatalf("Context7 entry is not canonical or lost unrelated servers: %s", data)
			}
			var state map[string]any
			data, err = os.ReadFile(filepath.Join(target, "home/.gentle-ai/state.json"))
			if err != nil || json.Unmarshal(data, &state) != nil || state["rdd_mode"] != "off" || state["other"] != true || state["rdd_mode_recorded_at"] == "2026-01-01T00:00:00Z" {
				t.Fatalf("RDD state is not the install shape: %s %v", data, err)
			}
			loaded, err := LoadUserShellExperience(UserShellInstall{Target: target, Mode: "separate"})
			if err != nil || loaded.Persona != "neutral" || loaded.BackgroundSubagents != "off" || loaded.StrictTDD != "off" || loaded.GlobalRDD != "off" || loaded.Context7 != "off" {
				t.Fatalf("applied preferences do not read back: %+v %v", loaded, err)
			}
		})
	}
}

// Custom (unmanaged) never claims the persona file, exactly like install.
func TestApplyUserExperiencePreferencesUnmanagedKeepsPersona(t *testing.T) {
	target := userUnixTempRoot(t)
	writeInstallFixture(t, target, userManifest{Schema: userSchema, Destination: target, Prefix: filepath.Join(target, "prefix"), Agent: filepath.Join(target, "agent"), Mode: "separate"})
	persona := filepath.Join(target, "config/persona.json")
	if err := os.MkdirAll(filepath.Dir(persona), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(persona, []byte("{\n  \"mode\": \"neutral\"\n}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ApplyUserExperiencePreferences(UserShellInstall{Target: target, Mode: "separate"}, UserExperience{Persona: "unmanaged", BackgroundSubagents: "on"}); err != nil {
		t.Fatal(err)
	}
	assertExperienceFile(t, persona, "{\n  \"mode\": \"neutral\"\n}\n")
	assertExperienceFile(t, filepath.Join(target, "config/background-subagents.json"), "{\n  \"schema\": \"gentle-pi.background-subagents/v1\",\n  \"policy\": \"on\"\n}\n")
}
