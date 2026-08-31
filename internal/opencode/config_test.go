package opencode

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveEffectiveConfigReadsJSONCAndAssignments(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectDir := t.TempDir()
	configPath := filepath.Join(projectDir, "opencode.jsonc")
	if err := os.WriteFile(configPath, []byte(`{
		// OpenCode accepts JSONC configuration.
		"provider": {
			"lmstudio": {
				"name": "LM Studio",
				"url": "http://localhost:1234/v1",
				"options": {"baseURL": "http://ignored.example/v1"},
				"models": {
					"local-qwen": {"name": "Local Qwen", "tool_call": true,},
				},
			},
		},
		"agent": {
			"sdd-apply": {"model": "lmstudio/local-qwen", "variant": "high"},
			"sdd-spec": {"model": ""},
		},
	}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	snapshot, err := ResolveEffectiveConfig(projectDir)
	if err != nil {
		t.Fatalf("ResolveEffectiveConfig() error = %v", err)
	}
	if snapshot.Path != configPath || snapshot.WritePath != configPath {
		t.Fatalf("paths = (%q, %q), want effective opencode.jsonc", snapshot.Path, snapshot.WritePath)
	}
	provider := snapshot.Providers["lmstudio"]
	if provider.URL != "http://localhost:1234/v1" {
		t.Fatalf("provider URL = %q, want direct url", provider.URL)
	}
	model := provider.Models["local-qwen"]
	if model.ID != "local-qwen" || model.Name != "Local Qwen" || !model.ToolCall {
		t.Fatalf("configured model = %+v", model)
	}
	apply := snapshot.Assignments["sdd-apply"]
	if !apply.Present || apply.Cleared || apply.Assignment.ProviderID != "lmstudio" || apply.Assignment.ModelID != "local-qwen" || apply.Assignment.Effort != "high" {
		t.Fatalf("sdd-apply assignment = %+v", apply)
	}
	spec := snapshot.Assignments["sdd-spec"]
	if !spec.Present || !spec.Cleared || spec.Assignment.ProviderID != "" {
		t.Fatalf("sdd-spec cleared presence = %+v", spec)
	}
}

func TestResolveEffectiveConfigPrecedenceAndDefaultWriteTarget(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	projectDir := t.TempDir()
	parentDir := filepath.Dir(projectDir)
	parentConfig := filepath.Join(parentDir, "opencode.json")
	projectJSON := filepath.Join(projectDir, "opencode.json")
	projectJSONC := filepath.Join(projectDir, "opencode.jsonc")
	if err := os.WriteFile(parentConfig, []byte(`{"provider":{"parent":{"models":{"m":{}}}}}`), 0o600); err != nil {
		t.Fatalf("write parent config: %v", err)
	}
	if err := os.WriteFile(projectJSON, []byte(`{"provider":{"json":{"models":{"m":{}}}}}`), 0o600); err != nil {
		t.Fatalf("write project json: %v", err)
	}
	if err := os.WriteFile(projectJSONC, []byte(`{"provider":{"jsonc":{"models":{"m":{}}}}}`), 0o600); err != nil {
		t.Fatalf("write project jsonc: %v", err)
	}

	snapshot, err := ResolveEffectiveConfig(projectDir)
	if err != nil {
		t.Fatalf("ResolveEffectiveConfig() error = %v", err)
	}
	if snapshot.Path != projectJSONC {
		t.Fatalf("effective path = %q, want nearest project opencode.jsonc", snapshot.Path)
	}
	if _, ok := snapshot.Providers["jsonc"]; !ok {
		t.Fatalf("providers = %#v, want project JSONC provider", snapshot.Providers)
	}

	if err := os.Remove(parentConfig); err != nil {
		t.Fatalf("remove parent config: %v", err)
	}
	emptyHome := t.TempDir()
	t.Setenv("HOME", emptyHome)
	emptyProject := filepath.Join(emptyHome, "empty-project")
	if err := os.MkdirAll(emptyProject, 0o700); err != nil {
		t.Fatalf("mkdir empty project: %v", err)
	}
	missing, err := ResolveEffectiveConfig(emptyProject)
	if err != nil {
		t.Fatalf("ResolveEffectiveConfig(empty) error = %v", err)
	}
	wantDefault := DefaultSettingsPath()
	if missing.Path != "" || missing.WritePath != wantDefault {
		t.Fatalf("missing paths = (%q, %q), want empty read path and default write target %q", missing.Path, missing.WritePath, wantDefault)
	}
}

func TestResolveEffectiveConfigUsesBaseURLFallback(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectDir := t.TempDir()
	configPath := filepath.Join(projectDir, "opencode.jsonc")
	if err := os.WriteFile(configPath, []byte(`{
		"provider": {
			"lmstudio": {
				"options": {"baseURL": "http://localhost:1234/v1"},
				"models": {"local-qwen": {"name": "Local Qwen"}}
			}
		}
	}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	snapshot, err := ResolveEffectiveConfig(projectDir)
	if err != nil {
		t.Fatalf("ResolveEffectiveConfig() error = %v", err)
	}
	if got := snapshot.Providers["lmstudio"].URL; got != "http://localhost:1234/v1" {
		t.Fatalf("provider URL = %q, want options.baseURL fallback", got)
	}
}
