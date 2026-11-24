package engram

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func injectCodexMultiAgentConfig(t *testing.T, seed string, opts InjectOptions) string {
	t.Helper()
	validCodexRuntime(t)

	home := t.TempDir()
	configPath := filepath.Join(home, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(.codex) error = %v", err)
	}
	if err := os.WriteFile(configPath, []byte(seed), 0o644); err != nil {
		t.Fatalf("WriteFile(config.toml) error = %v", err)
	}
	if _, err := InjectWithOptions(home, codexAdapter(), opts); err != nil {
		t.Fatalf("InjectWithOptions(codex) error = %v", err)
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config.toml) error = %v", err)
	}
	return string(content)
}

func TestCodexMultiAgentMigratesLegacyDisablement(t *testing.T) {
	seed := `model = "user-model"

[features]
multi_agent = false
web_search = true

[profiles.review]
model = "review-model"

[permissions]
sandbox = "workspace-write"

[mcp_servers.user]
command = "user-mcp"
`
	got := injectCodexMultiAgentConfig(t, seed, InjectOptions{})

	for _, preserved := range []string{
		`model = "user-model"`,
		`web_search = true`,
		`[profiles.review]`,
		`model = "review-model"`,
		`[permissions]`,
		`sandbox = "workspace-write"`,
		`[mcp_servers.user]`,
		`command = "user-mcp"`,
	} {
		if !strings.Contains(got, preserved) {
			t.Errorf("config.toml did not preserve %q:\n%s", preserved, got)
		}
	}
	if strings.Contains(got, "multi_agent =") {
		t.Fatalf("config.toml retained deprecated multi_agent key:\n%s", got)
	}
	if !strings.Contains(got, "enabled = false") {
		t.Fatalf("config.toml did not migrate legacy disablement to agents.enabled:\n%s", got)
	}
}

func TestCodexMultiAgentPreservesExplicitModernValue(t *testing.T) {
	tests := []struct {
		name string
		seed string
		want string
	}{
		{
			name: "explicit false wins over legacy true",
			seed: "[features]\nmulti_agent = true\n\n[agents]\nenabled = false\n",
			want: "enabled = false",
		},
		{
			name: "explicit true wins over legacy false",
			seed: "[features]\nmulti_agent = false\n\n[agents]\nenabled = true\n",
			want: "enabled = true",
		},
		{
			name: "malformed explicit value is left for Codex to diagnose",
			seed: "[features]\nmulti_agent = false\n\n[agents]\nenabled = user_choice\n",
			want: "enabled = user_choice",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := injectCodexMultiAgentConfig(t, tt.seed, InjectOptions{})
			if strings.Contains(got, "multi_agent =") {
				t.Fatalf("config.toml retained deprecated multi_agent key:\n%s", got)
			}
			if !strings.Contains(got, tt.want) {
				t.Fatalf("config.toml missing preserved modern value %q:\n%s", tt.want, got)
			}
		})
	}
}

func TestCodexMultiAgentPreservesExplicitDisableVariants(t *testing.T) {
	tests := []struct {
		name string
		seed string
	}{
		{name: "spaces", seed: "[agents]\nenabled = false\n"},
		{name: "tabs", seed: "[agents]\nenabled\t=\tfalse\n"},
		{name: "quoted key", seed: "[agents]\n\"enabled\" = false\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := injectCodexMultiAgentConfig(t, tt.seed, InjectOptions{})

			var parsed map[string]any
			if _, err := toml.Decode(got, &parsed); err != nil {
				t.Fatalf("config.toml is invalid after injection: %v\n%s", err, got)
			}
			agents, ok := parsed["agents"].(map[string]any)
			if !ok {
				t.Fatalf("config.toml missing parsed [agents] table:\n%s", got)
			}
			if enabled, ok := agents["enabled"]; !ok || enabled != false {
				t.Fatalf("explicit agents.enabled=false changed to %#v:\n%s", enabled, got)
			}
		})
	}
}

func TestCodexMultiAgentPreservesV2Choice(t *testing.T) {
	for _, v2 := range []string{"true", "false"} {
		t.Run(v2, func(t *testing.T) {
			seed := "[features]\nmulti_agent = true\nmulti_agent_v2 = " + v2 + "\nunrelated = true\n"
			got := injectCodexMultiAgentConfig(t, seed, InjectOptions{})

			if strings.Contains(got, "multi_agent =") {
				t.Fatalf("config.toml retained deprecated multi_agent key:\n%s", got)
			}
			for _, preserved := range []string{"multi_agent_v2 = " + v2, "unrelated = true"} {
				if !strings.Contains(got, preserved) {
					t.Fatalf("config.toml did not preserve %q:\n%s", preserved, got)
				}
			}
			if !strings.Contains(got, "enabled = true") {
				t.Fatalf("config.toml did not enable the current agents switch:\n%s", got)
			}
		})
	}
}

func TestCodexMultiAgentIgnoresExamplesInsideMultilineStrings(t *testing.T) {
	seed := `developer_instructions = """
[features]
multi_agent = false
"""

[features]
multi_agent = true
`
	got := injectCodexMultiAgentConfig(t, seed, InjectOptions{})

	if !strings.Contains(got, "[features]\nmulti_agent = false\n\"\"\"") {
		t.Fatalf("config.toml changed the multiline instruction example:\n%s", got)
	}
	if strings.Count(got, "multi_agent = true") != 0 {
		t.Fatalf("config.toml retained the real deprecated key:\n%s", got)
	}
	if !strings.Contains(got, "enabled = true") {
		t.Fatalf("config.toml treated an instruction example as legacy disablement:\n%s", got)
	}
}

func TestCodexMultiAgentMigrationIsIdempotent(t *testing.T) {
	validCodexRuntime(t)
	home := t.TempDir()
	configPath := filepath.Join(home, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("[features]\nmulti_agent = false\nkeep = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	first, err := InjectWithOptions(home, codexAdapter(), InjectOptions{})
	if err != nil {
		t.Fatalf("first InjectWithOptions(codex) error = %v", err)
	}
	if !first.Changed {
		t.Fatal("first InjectWithOptions(codex) changed = false, want true")
	}
	firstContent, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	second, err := InjectWithOptions(home, codexAdapter(), InjectOptions{})
	if err != nil {
		t.Fatalf("second InjectWithOptions(codex) error = %v", err)
	}
	if second.Changed {
		t.Fatal("second InjectWithOptions(codex) changed = true, want false")
	}
	secondContent, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(secondContent) != string(firstContent) {
		t.Fatalf("second injection changed config bytes:\nfirst:\n%s\nsecond:\n%s", firstContent, secondContent)
	}
}
