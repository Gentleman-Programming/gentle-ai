package agents

import (
	"slices"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

type settingsPathAdapter struct {
	Adapter
	path string
}

func (a settingsPathAdapter) SettingsPath(string) string { return a.path }

func TestJSONSettingsPathKeepsOnlyJSONNativeSettings(t *testing.T) {
	home := t.TempDir()
	var nonJSON []model.AgentID
	for _, agent := range defaultAgentIDs {
		adapter, err := NewAdapter(agent)
		if err != nil {
			t.Fatal(err)
		}
		native, got := adapter.SettingsPath(home), JSONSettingsPath(home, adapter)
		if got == "" && native != "" {
			nonJSON = append(nonJSON, agent)
		} else if got != native {
			t.Fatalf("JSONSettingsPath(%s) = %q, want native %q", agent, got, native)
		}
	}
	if want := []model.AgentID{model.AgentKimi, model.AgentHermes}; !slices.Equal(nonJSON, want) {
		t.Fatalf("non-JSON native settings = %v, want %v", nonJSON, want)
	}
}

func TestJSONSettingsPathClassifiesExtensionCaseInsensitively(t *testing.T) {
	base, err := NewAdapter(model.AgentClaudeCode)
	if err != nil {
		t.Fatal(err)
	}
	for path, eligible := range map[string]bool{
		"/h/settings.JSON": true, "/h/opencode.JsonC": true, "/h/config.toml": false,
		"/h/config.yaml": false, "/h/settings.json.bak": false, "/h/settings": false, "": false,
	} {
		want := ""
		if eligible {
			want = path
		}
		if got := JSONSettingsPath("/h", settingsPathAdapter{Adapter: base, path: path}); got != want {
			t.Fatalf("JSONSettingsPath(%q) = %q, want %q", path, got, want)
		}
	}
}
