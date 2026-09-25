package agenthooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

func TestInstallCodexTelemetry(t *testing.T) {
	home := t.TempDir()
	adapter, err := agents.NewAdapter(model.AgentCodex)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(adapter.GlobalConfigDir(home), "hooks.json")
	first, err := InstallCodexTelemetry(home, adapter)
	if err != nil || !first.Changed || len(first.Files) != 1 || first.Files[0] != path {
		t.Fatalf("first = %+v, %v", first, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var root struct {
		Hooks map[string][]struct {
			Hooks []map[string]any `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	for _, event := range []string{"SubagentStop", "Stop"} {
		entries := root.Hooks[event]
		if len(entries) != 1 || len(entries[0].Hooks) != 1 {
			t.Fatalf("%s: %+v", event, entries)
		}
		hook := entries[0].Hooks[0]
		if hook["type"] != "command" || hook["command"] != "gentle-ai telemetry runtime codex --json" || hook["async"] != true || hook["timeout"] != float64(4) {
			t.Fatalf("%s: %+v", event, hook)
		}
	}
	second, err := InstallCodexTelemetry(home, adapter)
	if err != nil || second.Changed {
		t.Fatalf("second = %+v, %v", second, err)
	}
	again, err := os.ReadFile(path)
	if err != nil || string(again) != string(data) {
		t.Fatalf("idempotence: %v", err)
	}
}

func TestInstallCodexTelemetryRejectsUnsafePathsAndShapes(t *testing.T) {
	adapter, err := agents.NewAdapter(model.AgentCodex)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, content string
		symlink       bool
	}{
		{name: "invalid JSON", content: "{"},
		{name: "invalid hooks", content: `{"hooks":[]}`},
		{name: "invalid event", content: `{"hooks":{"Stop":{}}}`},
		{name: "symlink", symlink: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			path := filepath.Join(adapter.GlobalConfigDir(home), "hooks.json")
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatal(err)
			}
			if tc.symlink {
				if err := os.Symlink(filepath.Join(home, "absent"), path); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(path, []byte(tc.content), 0644); err != nil {
				t.Fatal(err)
			}
			result, err := InstallCodexTelemetry(home, adapter)
			if err == nil || result.Changed {
				t.Fatalf("result = %+v, error = %v", result, err)
			}
			if !tc.symlink {
				got, readErr := os.ReadFile(path)
				if readErr != nil || strings.TrimSpace(string(got)) != tc.content {
					t.Fatalf("original changed: %q %v", got, readErr)
				}
			}
		})
	}
}
