package pi

import (
	"context"
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

func TestAdapterInstallCommandSequenceIsExact(t *testing.T) {
	a := NewAdapter()
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
