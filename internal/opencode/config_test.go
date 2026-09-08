package opencode

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveEffectiveConfigReadsJSONCAndAssignments(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
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
	t.Setenv("XDG_CONFIG_HOME", "")
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
	if snapshot.Path != projectJSONC || snapshot.WritePath != projectJSON {
		t.Fatalf("paths = (%q, %q), want JSONC read / JSON write", snapshot.Path, snapshot.WritePath)
	}
	if _, ok := snapshot.Providers["json"]; !ok {
		t.Fatalf("providers = %#v, want project JSON provider", snapshot.Providers)
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

func TestResolveEffectiveConfigStopsProjectSearchAtGitRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	parent := t.TempDir()
	project := filepath.Join(parent, "repo")
	nested := filepath.Join(project, "packages", "app")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o700); err != nil {
		t.Fatalf("mkdir git root: %v", err)
	}
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatalf("mkdir nested project: %v", err)
	}
	parentConfig := filepath.Join(parent, "opencode.jsonc")
	if err := os.WriteFile(parentConfig, []byte(`{"provider":{"outside":{"models":{"m":{}}}}}`), 0o600); err != nil {
		t.Fatalf("write parent config: %v", err)
	}
	globalConfig := filepath.Join(home, ".config", "opencode", "opencode.jsonc")
	if err := os.MkdirAll(filepath.Dir(globalConfig), 0o700); err != nil {
		t.Fatalf("mkdir global config: %v", err)
	}
	if err := os.WriteFile(globalConfig, []byte(`{"provider":{"global":{"models":{"m":{}}}}}`), 0o600); err != nil {
		t.Fatalf("write global config: %v", err)
	}

	snapshot, err := ResolveEffectiveConfigForHome(home, nested)
	if err != nil {
		t.Fatalf("ResolveEffectiveConfigForHome() error = %v", err)
	}
	if snapshot.Path != globalConfig {
		t.Fatalf("effective path = %q, want global config %q without crossing git root to %q", snapshot.Path, globalConfig, parentConfig)
	}
}

func TestResolveEffectiveConfigMalformedModelIsPresentButNotCleared(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	projectDir := t.TempDir()
	configPath := filepath.Join(projectDir, "opencode.jsonc")
	if err := os.WriteFile(configPath, []byte(`{
		"agent": {
			"sdd-apply": {"model": "typo-without-provider"},
			"sdd-spec": {"model": ""}
		}
	}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	snapshot, err := ResolveEffectiveConfigForHome(home, projectDir)
	if err != nil {
		t.Fatalf("ResolveEffectiveConfigForHome() error = %v", err)
	}
	apply := snapshot.Assignments["sdd-apply"]
	if !apply.Present || apply.Cleared {
		t.Fatalf("malformed model assignment = %+v, want present but not cleared", apply)
	}
	spec := snapshot.Assignments["sdd-spec"]
	if !spec.Present || !spec.Cleared {
		t.Fatalf("empty model assignment = %+v, want explicit clear", spec)
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

func TestEffectiveSettingsPathPreservesSelectedWritePathOnReadError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	projectDir := t.TempDir()
	configPath := filepath.Join(projectDir, "opencode.jsonc")
	if err := os.WriteFile(configPath, []byte(`{"provider":`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if got := EffectiveSettingsPath(home, projectDir); got != configPath {
		t.Fatalf("EffectiveSettingsPath() = %q, want selected malformed config path %q", got, configPath)
	}
}

func TestResolveEffectiveConfigSelectsOpenCodeConfigFile(t *testing.T) {
	for _, tt := range []struct {
		name         string
		writeJSON    bool
		jsonManaged  bool
		writeJSONC   bool
		jsoncManaged bool
		wantName     string
	}{
		{name: "only json", writeJSON: true, wantName: "opencode.json"},
		{name: "only jsonc", writeJSONC: true, wantName: "opencode.jsonc"},
		{name: "both managed only in json", writeJSON: true, jsonManaged: true, writeJSONC: true, wantName: "opencode.json"},
		{name: "both managed only in jsonc", writeJSON: true, writeJSONC: true, jsoncManaged: true, wantName: "opencode.jsonc"},
		{name: "both managed in neither", writeJSON: true, writeJSONC: true, wantName: "opencode.json"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			projectDir := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", "")
			t.Setenv("OPENCODE_CONFIG_DIR", "")

			if tt.writeJSON {
				writeOpenCodeConfigFixture(t, filepath.Join(projectDir, "opencode.json"), tt.jsonManaged)
			}
			if tt.writeJSONC {
				writeOpenCodeConfigFixture(t, filepath.Join(projectDir, "opencode.jsonc"), tt.jsoncManaged)
			}

			snapshot, err := ResolveEffectiveConfigForHome(home, projectDir)
			if err != nil {
				t.Fatalf("ResolveEffectiveConfigForHome() error = %v", err)
			}
			wantPath := filepath.Join(projectDir, tt.wantName)
			if snapshot.Path != wantPath || snapshot.WritePath != wantPath {
				t.Fatalf("paths = (%q, %q), want %q", snapshot.Path, snapshot.WritePath, wantPath)
			}
		})
	}

	// Regression for CodeRabbit r3952255572: when JSON has a user-owned agent
	// with the managed shape (hidden + prompt + permission) but no Gentle AI
	// ownership marker, and JSONC has the real Gentle AI-managed config with
	// the marker, the resolver must select JSONC.
	t.Run("json user-owned managed shape without marker selects jsonc with marker", func(t *testing.T) {
		home := t.TempDir()
		projectDir := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("XDG_CONFIG_HOME", "")
		t.Setenv("OPENCODE_CONFIG_DIR", "")

		jsonPath := filepath.Join(projectDir, "opencode.json")
		// User-owned agent that matches hidden+prompt+permission shape
		// but lacks the __managed_by marker.
		if err := os.MkdirAll(projectDir, 0o755); err != nil {
			t.Fatalf("mkdir project dir: %v", err)
		}
		if err := os.WriteFile(jsonPath, []byte(`{
  "agent": {
    "gentle-orchestrator": {
      "mode": "primary",
      "hidden": true,
      "prompt": "my custom orchestrator",
      "permission": {"task": "allow"}
    }
  }
}`), 0o600); err != nil {
			t.Fatalf("write json fixture: %v", err)
		}

		jsoncPath := filepath.Join(projectDir, "opencode.jsonc")
		// Real Gentle AI managed config with the ownership marker.
		if err := os.WriteFile(jsoncPath, []byte(`{
  "agent": {
    "gentle-orchestrator": {
      "mode": "primary",
      "hidden": true,
      "prompt": "managed by Gentle AI",
      "permission": {},
      "__managed_by": "gentle-ai/sdd"
    }
  }
}`), 0o600); err != nil {
			t.Fatalf("write jsonc fixture: %v", err)
		}

		snapshot, err := ResolveEffectiveConfigForHome(home, projectDir)
		if err != nil {
			t.Fatalf("ResolveEffectiveConfigForHome() error = %v", err)
		}
		wantPath := filepath.Join(projectDir, "opencode.jsonc")
		if snapshot.Path != wantPath || snapshot.WritePath != wantPath {
			t.Fatalf("paths = (%q, %q), want %q", snapshot.Path, snapshot.WritePath, wantPath)
		}
	})
}

func TestResolveEffectiveConfigUsesOpenCodeConfigDir(t *testing.T) {
	home := t.TempDir()
	projectDir := t.TempDir()
	defaultDir := filepath.Join(home, ".config", "opencode")
	overrideDir := filepath.Join(t.TempDir(), "opencode")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("OPENCODE_CONFIG_DIR", overrideDir)
	writeOpenCodeConfigFixture(t, filepath.Join(defaultDir, "opencode.json"), true)
	writeOpenCodeConfigFixture(t, filepath.Join(overrideDir, "opencode.jsonc"), false)

	snapshot, err := ResolveEffectiveConfigForHome(home, projectDir)
	if err != nil {
		t.Fatalf("ResolveEffectiveConfigForHome() error = %v", err)
	}
	wantPath := filepath.Join(overrideDir, "opencode.jsonc")
	if snapshot.Path != wantPath || snapshot.WritePath != wantPath {
		t.Fatalf("paths = (%q, %q), want OPENCODE_CONFIG_DIR config %q", snapshot.Path, snapshot.WritePath, wantPath)
	}
}

func TestRuntimeConfigPreservesWriteAuthorityAndLayeredReads(t *testing.T) {
	home, project, override := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	// os.UserHomeDir (used by ResolveEffectiveConfig) reads USERPROFILE on
	// Windows, not HOME, so both must point at the fixture home directory.
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("OPENCODE_CONFIG_DIR", override)
	global := DefaultSettingsPathForHome(home)
	writeOpenCodeConfigFixture(t, global, false)
	jsonPath := filepath.Join(project, "opencode.json")
	jsoncPath := filepath.Join(project, "opencode.jsonc")
	writeOpenCodeConfigFixture(t, jsonPath, true)
	for _, path := range []string{jsoncPath, filepath.Join(override, "opencode.jsonc")} {
		if err := os.WriteFile(path, []byte(`{"agent":{"gentle-orchestrator":{"model":"custom/override"}},"provider":{"custom":{"name":"Higher priority","models":{"__replace__":{}}}}}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := ResolveEffectiveConfig(project)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.WritePath != jsonPath || snapshot.Path != filepath.Join(override, "opencode.jsonc") {
		t.Fatalf("read/write paths = %q / %q", snapshot.Path, snapshot.WritePath)
	}
	if snapshot.Assignments["gentle-orchestrator"].Assignment.ModelID != "override" || snapshot.Providers["custom"].Name != "Higher priority" || len(snapshot.Providers["custom"].Models) != 1 || len(snapshot.Providers["fixture"].Models) != 1 || len(snapshot.Diagnostics) == 0 {
		t.Fatalf("missing layered reads or conflict warning: %+v", snapshot)
	}
	// The same directory's JSONC wins without the additive config directory.
	t.Setenv("OPENCODE_CONFIG_DIR", "")
	snapshot, err = ResolveEffectiveConfig(project)
	if err != nil || snapshot.Path != jsoncPath || snapshot.WritePath != jsonPath || snapshot.Assignments["gentle-orchestrator"].Assignment.ModelID != "override" {
		t.Fatalf("JSONC precedence: %+v, %v", snapshot, err)
	}
	for _, path := range []string{jsoncPath, jsonPath} {
		if path == jsonPath {
			writeOpenCodeConfigFixture(t, jsoncPath, false)
		}
		if err := os.WriteFile(path, []byte(`{"broken":`), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := ResolveEffectiveConfig(project); err == nil {
			t.Fatalf("malformed config %s silently ignored", path)
		}
	}
}

func writeOpenCodeConfigFixture(t *testing.T, path string, managed bool) {
	t.Helper()
	content := `{"provider":{"fixture":{"models":{"m":{}}}}}`
	if managed {
		content = `{
  "agent": {
    "gentle-orchestrator": {
      "mode": "primary",
      "hidden": true,
      "prompt": "managed by Gentle AI",
      "permission": {}
    }
  }
}`
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}
}
