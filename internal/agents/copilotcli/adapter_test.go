package copilotcli

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

func TestCopilotCLIAdapter_Detect(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name        string
		copilotHome string
		lookPath    func(string) (string, error)
		statResult  statResult
		wantIns     bool
		wantBin     string
		wantCfg     string
		wantFound   bool
		wantErr     error
	}{
		{"found", "", func(string) (string, error) { return "/bin/copilot", nil }, statResult{isDir: true}, true, "/bin/copilot", "/home/u/.copilot", true, nil},
		{"missing", "", func(string) (string, error) { return "", exec.ErrNotFound }, statResult{err: os.ErrNotExist}, false, "", "/home/u/.copilot", false, nil},
		{"stat error", "", func(string) (string, error) { return "/bin/copilot", nil }, statResult{err: boom}, false, "", "/home/u/.copilot", false, boom},
		{"custom COPILOT_HOME", filepath.Join(t.TempDir(), "custom-copilot"), func(string) (string, error) { return "/bin/copilot", nil }, statResult{isDir: true}, true, "/bin/copilot", "", true, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("COPILOT_HOME", tt.copilotHome)
			wantCfg := tt.wantCfg
			if tt.copilotHome != "" {
				wantCfg = tt.copilotHome
			}
			a := &Adapter{
				lookPath: tt.lookPath,
				statPath: func(path string) statResult {
					if path != wantCfg {
						t.Fatalf("stat path = %q, want %q", path, wantCfg)
					}
					return tt.statResult
				},
			}
			ins, bin, cfg, found, err := a.Detect(context.Background(), "/home/u")
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("got %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil || ins != tt.wantIns || bin != tt.wantBin || cfg != wantCfg || found != tt.wantFound {
				t.Errorf("got (%v, %q, %q, %v, %v)", ins, bin, cfg, found, err)
			}
		})
	}
}

func TestCopilotCLIAdapter_PathsAndInstall(t *testing.T) {
	t.Setenv("COPILOT_HOME", "")
	a := NewAdapter()
	home := "/home/u"
	if a.GlobalConfigDir(home) != filepath.Join(home, ".copilot") || a.SystemPromptFile(home) != filepath.Join(home, ".copilot", "copilot-instructions.md") || a.SkillsDir(home) != filepath.Join(home, ".copilot", "skills") || a.SettingsPath(home) != filepath.Join(home, ".copilot", "settings.json") || a.MCPConfigPath(home, "") != filepath.Join(home, ".copilot", "mcp-config.json") || a.SystemPromptStrategy() != model.StrategyFileReplace || a.MCPStrategy() != model.StrategyMCPConfigFile {
		t.Fatal("unexpected path or strategy")
	}
	for _, tt := range []struct {
		profile system.PlatformProfile
		want    string
	}{{system.PlatformProfile{OS: "linux", NpmWritable: true}, "npm"}, {system.PlatformProfile{OS: "linux", NpmWritable: false}, "sudo"}} {
		cmd, err := a.InstallCommand(tt.profile)
		if err != nil || len(cmd) != 1 || cmd[0][0] != tt.want || cmd[0][len(cmd[0])-1] != "@github/copilot@latest" {
			t.Fatalf("InstallCommand(%+v) = %v, %v", tt.profile, cmd, err)
		}
	}
}
