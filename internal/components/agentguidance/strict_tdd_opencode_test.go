package agentguidance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

func TestStrictTDDOpenCodePrompt(t *testing.T) {
	for _, agent := range []model.AgentID{model.AgentOpenCode, model.AgentKilocode} {
		t.Run(string(agent), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.jsonc")
			seed := `{"agent":{"gentle-orchestrator":{"prompt":"User note\n","permission":{"task":{"review-refuter":"allow"}}},"review-refuter":{"mode":"subagent"}},"custom":true}`
			if err := os.WriteFile(path, []byte(seed), 0600); err != nil {
				t.Fatal(err)
			}
			for _, enabled := range []bool{true, true, false, false} {
				result, err := InjectStrictTDDWithOptions(t.TempDir(), agent, enabled, RoutingOptions{SettingsPath: path})
				if err != nil {
					t.Fatal(err)
				}
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(raw), "gentle-ai:strict-tdd-mode") != enabled {
					t.Fatalf("enabled=%v: %s", enabled, raw)
				}
				if !strings.Contains(string(raw), "User note") || !strings.Contains(string(raw), "review-refuter") || !strings.Contains(string(raw), `"custom": true`) {
					t.Fatalf("unrelated settings lost: %s", raw)
				}
				if result.Changed != (enabled != strings.Contains(seed, "gentle-ai:strict-tdd-mode")) {
					t.Fatalf("unexpected change: enabled=%v changed=%v", enabled, result.Changed)
				}
				seed = string(raw)
			}
		})
	}
}

func TestStrictTDDOpenCodeMalformedSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.jsonc")
	if err := os.WriteFile(path, []byte(`{"agent":`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := InjectStrictTDDWithOptions(t.TempDir(), model.AgentOpenCode, true, RoutingOptions{SettingsPath: path}); err == nil {
		t.Fatal("malformed settings accepted")
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != `{"agent":` {
		t.Fatal("malformed settings modified")
	}
}
