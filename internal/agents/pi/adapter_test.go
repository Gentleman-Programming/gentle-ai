package pi

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/system"
	"github.com/gentleman-programming/gentle-ai/internal/versions"
)

func TestAdapterIdentityAndCapabilities(t *testing.T) {
	a := NewAdapter()

	if got := a.Agent(); got != model.AgentPi {
		t.Fatalf("Agent() = %q, want %q", got, model.AgentPi)
	}
	if got := a.Tier(); got != model.TierFull {
		t.Fatalf("Tier() = %q, want %q", got, model.TierFull)
	}
	if got := a.SystemPromptStrategy(); got != model.StrategyAppendToFile {
		t.Fatalf("SystemPromptStrategy() = %v, want %v", got, model.StrategyAppendToFile)
	}
	if got := a.MCPStrategy(); got != model.StrategyMCPConfigFile {
		t.Fatalf("MCPStrategy() = %v, want %v", got, model.StrategyMCPConfigFile)
	}

	tests := []struct {
		name string
		got  bool
		want bool
	}{
		{"SupportsAutoInstall", a.SupportsAutoInstall(), true},
		{"SupportsSkills", a.SupportsSkills(), false},
		{"SupportsMCP", a.SupportsMCP(), true},
		{"SupportsSystemPrompt", a.SupportsSystemPrompt(), false},
		{"SupportsSlashCommands", a.SupportsSlashCommands(), false},
		{"SupportsOutputStyles", a.SupportsOutputStyles(), false},
		{"SupportsSubAgents", a.SupportsSubAgents(), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("%s = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestAdapterPaths(t *testing.T) {
	a := NewAdapter()
	homeDir := t.TempDir()
	piDir := filepath.Join(homeDir, ".pi")
	piAgentDir := filepath.Join(piDir, "agent")

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"GlobalConfigDir", a.GlobalConfigDir(homeDir), piDir},
		{"SystemPromptDir", a.SystemPromptDir(homeDir), ""},
		{"SystemPromptFile", a.SystemPromptFile(homeDir), ""},
		{"SkillsDir", a.SkillsDir(homeDir), ""},
		{"SettingsPath", a.SettingsPath(homeDir), filepath.Join(piAgentDir, "settings.json")},
		{"CommandsDir", a.CommandsDir(homeDir), ""},
		{"MCPConfigPath", a.MCPConfigPath(homeDir, "context7"), filepath.Join(piAgentDir, "mcp.json")},
		{"OutputStyleDir", a.OutputStyleDir(homeDir), ""},
		{"SubAgentsDir", a.SubAgentsDir(homeDir), ""},
		{"EmbeddedSubAgentsDir", a.EmbeddedSubAgentsDir(), ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("%s = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestAdapterDetectUsesPiBinaryAndConfigPath(t *testing.T) {
	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".pi")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}

	a := &Adapter{
		lookPath: func(file string) (string, error) {
			if file != "pi" {
				t.Fatalf("lookPath called with %q, want pi", file)
			}
			return "/usr/local/bin/pi", nil
		},
		statPath: defaultStat,
	}

	installed, binaryPath, configPath, configFound, err := a.Detect(context.Background(), homeDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if !installed {
		t.Fatalf("Detect() installed = false, want true")
	}
	if binaryPath != "/usr/local/bin/pi" {
		t.Fatalf("Detect() binaryPath = %q, want /usr/local/bin/pi", binaryPath)
	}
	if configPath != configDir {
		t.Fatalf("Detect() configPath = %q, want %q", configPath, configDir)
	}
	if !configFound {
		t.Fatalf("Detect() configFound = false, want true")
	}
}

func TestAdapterDetectMissingPiBinary(t *testing.T) {
	homeDir := t.TempDir()
	a := &Adapter{
		lookPath: func(file string) (string, error) {
			return "", os.ErrNotExist
		},
		statPath: defaultStat,
	}

	installed, binaryPath, configPath, configFound, err := a.Detect(context.Background(), homeDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if installed {
		t.Fatalf("Detect() installed = true, want false")
	}
	if binaryPath != "" {
		t.Fatalf("Detect() binaryPath = %q, want empty", binaryPath)
	}
	if configPath != filepath.Join(homeDir, ".pi") {
		t.Fatalf("Detect() configPath = %q, want ~/.pi under home", configPath)
	}
	if configFound {
		t.Fatalf("Detect() configFound = true, want false")
	}
}

func TestAdapterInstallCommandSequenceUsesNpmWhenPnpmIsUnavailable(t *testing.T) {
	a := &Adapter{
		lookPath: func(file string) (string, error) {
			if file == "pnpm" {
				return "", os.ErrNotExist
			}
			return "/usr/local/bin/" + file, nil
		},
		statPath: defaultStat,
	}
	commands, err := a.InstallCommand(system.PlatformProfile{})
	if err != nil {
		t.Fatalf("InstallCommand() error = %v", err)
	}

	want := [][]string{
		{"pi", "install", "npm:gentle-pi"},
		{"pi", "install", "npm:gentle-engram"},
		{"pi", "install", "npm:pi-mcp-adapter"},
		{"npm", "exec", "--yes", "--package", "gentle-engram@" + versions.GentleEngram, "--", "pi-engram", "init"},
		{"pi", "install", "npm:pi-subagents"},
		{"pi", "install", "npm:pi-intercom"},
		{"pi", "install", "npm:@juicesharp/rpiv-ask-user-question"},
		{"pi", "install", "npm:pi-web-access"},
		{"pi", "install", "npm:pi-lens"},
		{"pi", "install", "npm:@juicesharp/rpiv-todo"},
		{"pi", "install", "npm:pi-btw"},
	}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("InstallCommand() = %#v, want %#v", commands, want)
	}
}

func TestAdapterInstallCommandSequenceUsesPnpmForEngramInitWhenAvailable(t *testing.T) {
	a := &Adapter{
		lookPath: func(file string) (string, error) {
			if file == "pnpm" {
				return "/usr/local/bin/pnpm", nil
			}
			return "", os.ErrNotExist
		},
		statPath: defaultStat,
	}
	commands, err := a.InstallCommand(system.PlatformProfile{})
	if err != nil {
		t.Fatalf("InstallCommand() error = %v", err)
	}

	want := []string{"pnpm", "dlx", "gentle-engram@" + versions.GentleEngram, "pi-engram", "init"}
	if !reflect.DeepEqual(commands[3], want) {
		t.Fatalf("InstallCommand()[3] = %#v, want %#v", commands[3], want)
	}
}

func TestAdapterDetectStatError(t *testing.T) {
	homeDir := t.TempDir()
	expectedErr := os.ErrPermission

	a := &Adapter{
		lookPath: func(file string) (string, error) {
			return "/usr/bin/pi", nil
		},
		statPath: func(path string) statResult {
			return statResult{err: expectedErr}
		},
	}

	_, _, _, _, err := a.Detect(context.Background(), homeDir)
	if err != expectedErr {
		t.Fatalf("Detect() expected error %v, got %v", expectedErr, err)
	}
}

func TestAppendPiPackage(t *testing.T) {
	tests := []struct {
		name     string
		existing any
		desired  string
		want     []any
	}{
		{
			name:     "existing nil",
			existing: nil,
			desired:  "npm:pi-mcp-adapter",
			want:     []any{"npm:pi-mcp-adapter"},
		},
		{
			name:     "existing slice of strings with duplicate to remove",
			existing: []string{"npm:pi-mcp-adapter", "other"},
			desired:  "npm:pi-mcp-adapter",
			want:     []any{"other", "npm:pi-mcp-adapter"},
		},
		{
			name:     "existing slice of any",
			existing: []any{"other"},
			desired:  "npm:pi-mcp-adapter",
			want:     []any{"other", "npm:pi-mcp-adapter"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := appendPiPackage(tt.existing, tt.desired)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("appendPiPackage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPiPackagesAsSlice(t *testing.T) {
	tests := []struct {
		name     string
		existing any
		want     []any
	}{
		{
			name:     "slice of any",
			existing: []any{"a", "b"},
			want:     []any{"a", "b"},
		},
		{
			name:     "slice of strings",
			existing: []string{"a", "b"},
			want:     []any{"a", "b"},
		},
		{
			name:     "default nil",
			existing: 123,
			want:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.want != nil {
				got := piPackagesAsSlice(tt.existing)
				if !reflect.DeepEqual(got, tt.want) {
					t.Errorf("piPackagesAsSlice() = %v, want %v", got, tt.want)
				}
			}
		})
	}

	t.Run("map parsing contents", func(t *testing.T) {
		input := map[string]any{
			"npm:package1": "1.0.0",
			"other":        "2.0.0",
		}
		got := piPackagesAsSlice(input)
		if len(got) != 2 {
			t.Errorf("expected 2 packages, got %d: %v", len(got), got)
		}
		hasPkg1 := false
		hasOther := false
		for _, v := range got {
			s, ok := v.(string)
			if !ok {
				t.Fatalf("expected string, got %T", v)
			}
			if s == "npm:package1@1.0.0" {
				hasPkg1 = true
			}
			if s == "other" {
				hasOther = true
			}
		}
		if !hasPkg1 || !hasOther {
			t.Errorf("missing expected values in %v", got)
		}
	})
}

func TestPiPackageIdentity(t *testing.T) {
	tests := []struct {
		name string
		pkg  any
		want string
	}{
		{"string match", "npm:pi-mcp-adapter", "npm:pi-mcp-adapter"},
		{"string version match", "npm:pi-mcp-adapter@2.0.0", "npm:pi-mcp-adapter"},
		{"map match", map[string]any{"source": "npm:pi-mcp-adapter@2.0.0"}, "npm:pi-mcp-adapter"},
		{"invalid type", 123, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := piPackageIdentity(tt.pkg); got != tt.want {
				t.Errorf("piPackageIdentity() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAdapterProvisionEngramMCP(t *testing.T) {
	t.Run("successful provisioning", func(t *testing.T) {
		homeDir := t.TempDir()
		agentDir := filepath.Join(homeDir, ".pi", "agent")
		npmDir := filepath.Join(homeDir, ".pi", "npm")

		if err := os.MkdirAll(agentDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(npmDir, 0o755); err != nil {
			t.Fatal(err)
		}

		// Write initial settings.json and package.json files
		settingsPath := filepath.Join(agentDir, "settings.json")
		if err := os.WriteFile(settingsPath, []byte(`{"packages": ["npm:some-pkg"]}`), 0o644); err != nil {
			t.Fatal(err)
		}

		packagePath := filepath.Join(npmDir, "package.json")
		if err := os.WriteFile(packagePath, []byte(`{"dependencies": {}}`), 0o644); err != nil {
			t.Fatal(err)
		}

		a := NewAdapter()
		changed, paths, err := a.ProvisionEngramMCP(homeDir)
		if err != nil {
			t.Fatalf("ProvisionEngramMCP() error = %v", err)
		}
		if !changed {
			t.Error("expected changed = true")
		}
		if len(paths) != 2 {
			t.Errorf("expected 2 modified paths, got %d", len(paths))
		}

		// Verify settings.json merged packages correctly
		settingsContent, err := os.ReadFile(settingsPath)
		if err != nil {
			t.Fatal(err)
		}
		var settings map[string]any
		if err := json.Unmarshal(settingsContent, &settings); err != nil {
			t.Fatal(err)
		}
		pkgs, ok := settings["packages"].([]any)
		if !ok {
			t.Fatalf("expected packages to be slice, got %T", settings["packages"])
		}
		if len(pkgs) != 2 || pkgs[1] != "npm:pi-mcp-adapter" {
			t.Errorf("packages = %v, expected npm:pi-mcp-adapter", pkgs)
		}

		// Verify package.json dependencies updated correctly
		packageContent, err := os.ReadFile(packagePath)
		if err != nil {
			t.Fatal(err)
		}
		var packageData map[string]any
		if err := json.Unmarshal(packageContent, &packageData); err != nil {
			t.Fatal(err)
		}
		deps, ok := packageData["dependencies"].(map[string]any)
		if !ok {
			t.Fatalf("expected dependencies to be map, got %T", packageData["dependencies"])
		}
		if deps["pi-mcp-adapter"] != "^2.6.0" {
			t.Errorf("dependencies[pi-mcp-adapter] = %v, want ^2.6.0", deps["pi-mcp-adapter"])
		}
	})

	t.Run("settings missing", func(t *testing.T) {
		homeDir := t.TempDir()
		npmDir := filepath.Join(homeDir, ".pi", "npm")
		if err := os.MkdirAll(npmDir, 0o755); err != nil {
			t.Fatal(err)
		}
		packagePath := filepath.Join(npmDir, "package.json")
		if err := os.WriteFile(packagePath, []byte(`{"dependencies": {}}`), 0o644); err != nil {
			t.Fatal(err)
		}

		a := NewAdapter()
		changed, _, err := a.ProvisionEngramMCP(homeDir)
		if err != nil {
			t.Fatalf("ProvisionEngramMCP() error = %v", err)
		}
		if !changed {
			t.Error("expected changed = true")
		}
	})
}

func TestAdapterProvisionEngramMCP_InvalidJSON(t *testing.T) {
	homeDir := t.TempDir()
	agentDir := filepath.Join(homeDir, ".pi", "agent")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}

	settingsPath := filepath.Join(agentDir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte(`{"packages": `), 0o644); err != nil {
		t.Fatal(err)
	}

	a := NewAdapter()
	_, _, err := a.ProvisionEngramMCP(homeDir)
	if err == nil {
		t.Fatal("expected error due to invalid JSON in settings.json")
	}
}
