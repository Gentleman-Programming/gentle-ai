package copilotcli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/system"
)

func TestStrategies(t *testing.T) {
	a := NewAdapter()

	if got := a.SystemPromptStrategy(); got != model.StrategyInstructionsFile {
		t.Fatalf("SystemPromptStrategy() = %v, want StrategyInstructionsFile", got)
	}

	if got := a.MCPStrategy(); got != model.StrategyMCPConfigFile {
		t.Fatalf("MCPStrategy() = %v, want StrategyMCPConfigFile", got)
	}
}

func TestDetectionFound(t *testing.T) {
	a := &Adapter{
		lookPath: func(string) (string, error) {
			return "/usr/local/bin/copilot", nil
		},
		statPath: func(string) statResult {
			return statResult{exists: true}
		},
	}

	installed, binaryPath, configPath, configFound, err := a.Detect(context.Background(), "/tmp/home")
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if !installed {
		t.Fatal("Detect() installed = false, want true")
	}
	if binaryPath != "/usr/local/bin/copilot" {
		t.Fatalf("Detect() binaryPath = %q, want %q", binaryPath, "/usr/local/bin/copilot")
	}
	wantConfigPath := filepath.Join("/tmp/home", ".copilot")
	if configPath != wantConfigPath {
		t.Fatalf("Detect() configPath = %q, want %q", configPath, wantConfigPath)
	}
	if !configFound {
		t.Fatal("Detect() configFound = false, want true")
	}
}

func TestDetectionNotFound(t *testing.T) {
	errNotFound := errors.New("not found")
	a := &Adapter{
		lookPath: func(string) (string, error) {
			return "", errNotFound
		},
		statPath: func(string) statResult {
			return statResult{err: os.ErrNotExist}
		},
	}

	installed, binaryPath, _, configFound, err := a.Detect(context.Background(), "/tmp/home")
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if installed {
		t.Fatal("Detect() installed = true, want false")
	}
	if binaryPath != "" {
		t.Fatalf("Detect() binaryPath = %q, want empty", binaryPath)
	}
	if configFound {
		t.Fatal("Detect() configFound = true, want false")
	}
}

func TestDetectionMissingConfig(t *testing.T) {
	a := &Adapter{
		lookPath: func(string) (string, error) {
			return "/usr/local/bin/copilot", nil
		},
		statPath: func(string) statResult {
			return statResult{err: os.ErrNotExist}
		},
	}

	installed, _, _, configFound, err := a.Detect(context.Background(), "/tmp/home")
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if !installed {
		t.Fatal("Detect() installed = false, want true (binary found)")
	}
	if configFound {
		t.Fatal("Detect() configFound = true, want false (config missing)")
	}
}

func TestPaths(t *testing.T) {
	a := NewAdapter()
	home := "/tmp/home"

	if got := a.GlobalConfigDir(home); got != filepath.Join(home, ".copilot") {
		t.Fatalf("GlobalConfigDir() = %q, want %q", got, filepath.Join(home, ".copilot"))
	}

	if got := a.MCPConfigPath(home, ""); got != filepath.Join(home, ".copilot", "mcp-config.json") {
		t.Fatalf("MCPConfigPath() = %q, want %q", got, filepath.Join(home, ".copilot", "mcp-config.json"))
	}

	if got := a.SkillsDir(home); got != filepath.Join(home, ".copilot", "skills") {
		t.Fatalf("SkillsDir() = %q, want %q", got, filepath.Join(home, ".copilot", "skills"))
	}

	if got := a.SystemPromptFile(home); got != filepath.Join(home, ".github", "copilot-instructions.md") {
		t.Fatalf("SystemPromptFile() = %q, want %q", got, filepath.Join(home, ".github", "copilot-instructions.md"))
	}
}

func TestCapabilities(t *testing.T) {
	a := NewAdapter()

	if got := a.Agent(); got != model.AgentCopilotCLI {
		t.Fatalf("Agent() = %q, want %q", got, model.AgentCopilotCLI)
	}

	if got := a.Tier(); got != model.TierFull {
		t.Fatalf("Tier() = %q, want %q", got, model.TierFull)
	}

	if got := a.SupportsAutoInstall(); got {
		t.Fatal("SupportsAutoInstall() = true, want false")
	}

	if got := a.SupportsSkills(); !got {
		t.Fatal("SupportsSkills() = false, want true")
	}

	if got := a.SupportsSubAgents(); got {
		t.Fatal("SupportsSubAgents() = true, want false")
	}

	if got := a.SupportsMCP(); !got {
		t.Fatal("SupportsMCP() = false, want true")
	}

	if got := a.SupportsSystemPrompt(); !got {
		t.Fatal("SupportsSystemPrompt() = false, want true")
	}
}

func TestInstallCommandReturnsError(t *testing.T) {
	a := NewAdapter()

	_, err := a.InstallCommand(system.PlatformProfile{})
	if err == nil {
		t.Fatal("InstallCommand() error = nil, want AgentNotInstallableError")
	}

	var notInstallable AgentNotInstallableError
	if errors.As(err, &notInstallable) {
		if notInstallable.Agent != model.AgentCopilotCLI {
			t.Fatalf("AgentNotInstallableError.Agent = %q, want %q", notInstallable.Agent, model.AgentCopilotCLI)
		}
	} else {
		t.Fatalf("InstallCommand() error type = %T, want AgentNotInstallableError", err)
	}
}
