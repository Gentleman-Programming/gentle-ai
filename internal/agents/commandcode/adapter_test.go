package commandcode

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

const testHome = "/tmp/home"

// detectingAdapter returns an adapter whose lookPath stub records every probed
// alias and resolves the aliases present in paths.
func detectingAdapter(goos string, paths map[string]string, stat statResult, probed *[]string) *Adapter {
	lookPath := func(alias string) (string, error) {
		*probed = append(*probed, alias)
		path, ok := paths[alias]
		if !ok {
			return "", errors.New("not found")
		}
		return path, nil
	}
	return &Adapter{goos: goos, lookPath: lookPath, statPath: func(string) statResult { return stat }}
}

func TestDetectionAliasesPerPlatform(t *testing.T) {
	for _, tt := range []struct {
		goos string
		want []string
	}{
		{goos: "windows", want: []string{"command-code", "cmdc"}},
		{goos: "linux", want: []string{"command-code", "cmd"}},
		{goos: "darwin", want: []string{"command-code", "cmd"}},
	} {
		if got := detectionAliases(tt.goos); !reflect.DeepEqual(got, tt.want) {
			t.Fatalf("detectionAliases(%q) = %v, want %v", tt.goos, got, tt.want)
		}
	}
}

func TestDetect(t *testing.T) {
	configPath := filepath.Join(testHome, ".commandcode")

	tests := []struct {
		name            string
		goos            string
		paths           map[string]string
		stat            statResult
		wantProbed      []string
		wantInstalled   bool
		wantBinary      string
		wantConfigFound bool
		wantErr         bool
	}{
		{
			name: "windows prefers command-code when both aliases exist", goos: "windows",
			paths:      map[string]string{"command-code": `C:\bin\command-code.CMD`, "cmdc": `C:\bin\cmdc.CMD`},
			stat:       statResult{isDir: true},
			wantProbed: []string{"command-code"}, wantInstalled: true,
			wantBinary: `C:\bin\command-code.CMD`, wantConfigFound: true,
		},
		{
			name: "windows falls back to cmdc when command-code is missing", goos: "windows",
			paths:      map[string]string{"cmdc": `C:\bin\cmdc.CMD`},
			stat:       statResult{isDir: true},
			wantProbed: []string{"command-code", "cmdc"}, wantInstalled: true,
			wantBinary: `C:\bin\cmdc.CMD`, wantConfigFound: true,
		},
		{
			name: "unix resolves cmd when command-code is missing", goos: "linux",
			paths:      map[string]string{"cmd": "/usr/local/bin/cmd"},
			stat:       statResult{isDir: true},
			wantProbed: []string{"command-code", "cmd"}, wantInstalled: true,
			wantBinary: "/usr/local/bin/cmd", wantConfigFound: true,
		},
		{
			// Bare cmd resolves to cmd.exe on Windows, so it is never probed
			// even when every alias is missing.
			name: "binary and config missing", goos: "windows",
			stat: statResult{err: os.ErrNotExist}, wantProbed: []string{"command-code", "cmdc"},
		},
		{
			name: "stat error bubbles up", goos: "windows",
			paths:      map[string]string{"cmdc": `C:\bin\cmdc.CMD`},
			stat:       statResult{err: errors.New("permission denied")},
			wantProbed: []string{"command-code", "cmdc"}, wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var probed []string
			a := detectingAdapter(tt.goos, tt.paths, tt.stat, &probed)
			installed, binaryPath, gotConfigPath, configFound, err := a.Detect(context.Background(), testHome)
			if !reflect.DeepEqual(probed, tt.wantProbed) {
				t.Fatalf("probed aliases = %v, want %v", probed, tt.wantProbed)
			}
			if (err != nil) != tt.wantErr {
				t.Fatalf("Detect() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if installed != tt.wantInstalled || binaryPath != tt.wantBinary || configFound != tt.wantConfigFound || gotConfigPath != configPath {
				t.Fatalf("Detect() = (%v, %q, %q, %v), want (%v, %q, %q, %v)",
					installed, binaryPath, gotConfigPath, configFound, tt.wantInstalled, tt.wantBinary, configPath, tt.wantConfigFound)
			}
		})
	}
}

func TestAdapterProjections(t *testing.T) {
	a := NewAdapter()
	root := filepath.Join(testHome, ".commandcode")

	for _, tt := range []struct{ name, got, want string }{
		{"GlobalConfigDir", a.GlobalConfigDir(testHome), root},
		{"SystemPromptDir", a.SystemPromptDir(testHome), root},
		{"SystemPromptFile", a.SystemPromptFile(testHome), filepath.Join(root, "AGENTS.md")},
		{"SkillsDir", a.SkillsDir(testHome), filepath.Join(root, "skills")},
		{"SettingsPath", a.SettingsPath(testHome), filepath.Join(root, "settings.json")},
		{"MCPConfigPath", a.MCPConfigPath(testHome, "context7"), filepath.Join(root, "mcp.json")},
	} {
		if tt.got != tt.want {
			t.Fatalf("%s = %q, want %q", tt.name, tt.got, tt.want)
		}
	}

	if a.Agent() != model.AgentCommandCode || a.Tier() != model.TierFull {
		t.Fatalf("identity = (%q, %q), want (%q, %q)", a.Agent(), a.Tier(), model.AgentCommandCode, model.TierFull)
	}
	if a.SystemPromptStrategy() != model.StrategyMarkdownSections || a.MCPStrategy() != model.StrategyMCPConfigFile {
		t.Fatalf("strategies = (%v, %v), want markdown sections and MCP config file", a.SystemPromptStrategy(), a.MCPStrategy())
	}

	// Slash commands and native sub-agents stay unclaimed until their native
	// formats are verified against a real install.
	for _, capability := range []struct {
		name string
		got  bool
		want bool
	}{
		{"SupportsSkills", a.SupportsSkills(), true},
		{"SupportsSystemPrompt", a.SupportsSystemPrompt(), true},
		{"SupportsMCP", a.SupportsMCP(), true},
		{"SupportsSlashCommands", a.SupportsSlashCommands(), false},
		{"SupportsSubAgents", a.SupportsSubAgents(), false},
		{"SupportsOutputStyles", a.SupportsOutputStyles(), false},
	} {
		if capability.got != capability.want {
			t.Fatalf("%s() = %v, want %v", capability.name, capability.got, capability.want)
		}
	}

	if _, err := a.InstallCommand(system.PlatformProfile{OS: "windows"}); err == nil {
		t.Fatal("InstallCommand() = nil error, want not-installable error")
	}
}
