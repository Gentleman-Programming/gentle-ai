package agentguidance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

func TestStrictTDDCarrierLifecycle(t *testing.T) {
	for _, agent := range []model.AgentID{model.AgentClaudeCode, model.AgentCodex, model.AgentOpenClaw, model.AgentKimi, model.AgentCursor, model.AgentGeminiCLI, model.AgentWindsurf, model.AgentKiroIDE, model.AgentVSCodeCopilot, model.AgentAntigravity, model.AgentQwenCode, model.AgentTrae, model.AgentHermes} {
		t.Run(string(agent), func(t *testing.T) {
			home := t.TempDir()
			adapter, err := agents.NewAdapter(agent)
			if err != nil {
				t.Fatal(err)
			}
			path := adapter.SystemPromptFile(home)
			if got := StrictTDDPath(home, agent); got != path && agent != model.AgentKimi {
				t.Fatalf("carrier = %q, want %q", got, path)
			}
			if agent == model.AgentKimi {
				path = filepath.Join(adapter.GlobalConfigDir(home), "strict-tdd-mode.md")
			}
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatal(err)
			}
			if agent != model.AgentKimi {
				if err := os.WriteFile(path, []byte("user before\nuser after\n"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			first, err := InjectStrictTDDWithOptions(home, agent, true, RoutingOptions{})
			if err != nil || !first.Changed {
				t.Fatalf("enable: %+v %v", first, err)
			}
			again, err := InjectStrictTDDWithOptions(home, agent, true, RoutingOptions{})
			if err != nil || again.Changed {
				t.Fatalf("repeat: %+v %v", again, err)
			}
			raw, err := os.ReadFile(path)
			if err != nil || !strings.Contains(string(raw), "Strict TDD Mode: enabled") {
				t.Fatalf("content: %q %v", raw, err)
			}
			off, err := InjectStrictTDDWithOptions(home, agent, false, RoutingOptions{})
			if err != nil || !off.Changed {
				t.Fatalf("disable: %+v %v", off, err)
			}
			if againOff, err := InjectStrictTDDWithOptions(home, agent, false, RoutingOptions{}); err != nil || againOff.Changed {
				t.Fatalf("repeat disable: %+v %v", againOff, err)
			}
			if agent == model.AgentKimi {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("module remains: %v", err)
				}
				return
			}
			raw, err = os.ReadFile(path)
			if err != nil || string(raw) != "user before\nuser after\n" {
				t.Fatalf("user content changed: %q %v", raw, err)
			}
		})
	}
}

func TestStrictTDDUnsupportedCarrier(t *testing.T) {
	for _, agent := range []model.AgentID{model.AgentPi, "unknown"} {
		t.Run(string(agent), func(t *testing.T) {
			home := t.TempDir()
			if path := StrictTDDPath(home, agent); path != "" {
				t.Fatalf("unexpected carrier: %q", path)
			}
			for _, enabled := range []bool{true, false} {
				result, err := InjectStrictTDDWithOptions(home, agent, enabled, RoutingOptions{})
				if err != nil || result.Changed {
					t.Fatalf("enabled=%v: %+v %v", enabled, result, err)
				}
			}
		})
	}
}

func TestStrictTDDModifiedKimiModuleFailsClosed(t *testing.T) {
	home := t.TempDir()
	adapter, err := agents.NewAdapter(model.AgentKimi)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(adapter.GlobalConfigDir(home), "strict-tdd-mode.md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("user module"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, enabled := range []bool{true, false} {
		if _, err := InjectStrictTDDWithOptions(home, model.AgentKimi, enabled, RoutingOptions{}); err == nil {
			t.Fatalf("enabled=%v: expected ownership error", enabled)
		}
		raw, err := os.ReadFile(path)
		if err != nil || string(raw) != "user module" {
			t.Fatalf("modified module lost: %q %v", raw, err)
		}
	}
}
