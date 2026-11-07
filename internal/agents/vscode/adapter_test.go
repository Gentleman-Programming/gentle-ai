package vscode

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

func TestDetectRequiresVSCodeAndCopilotExtension(t *testing.T) {
	home := t.TempDir()
	a := NewAdapter()
	a.lookPath = func(string) (string, error) { return filepath.Join(home, "bin", "code"), nil }

	if err := os.MkdirAll(filepath.Join(home, ".copilot", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	installed, _, _, _, err := a.Detect(context.Background(), home)
	if err != nil || installed {
		t.Fatalf("Detect() with skills only = %v, %v; want false, nil", installed, err)
	}

	for _, ext := range []string{"github.copilot-1.2.3", "github.copilot-chat-0.12.0"} {
		h := t.TempDir()
		_ = os.MkdirAll(filepath.Join(h, ".vscode", "extensions", ext), 0o755)
		if ins, _, _, ev, err := a.Detect(context.Background(), h); err != nil || !ins || !ev {
			t.Fatalf("Detect() with %s = %v, %v, %v; want true, true, nil", ext, ins, ev, err)
		}
	}
}

func TestDetectIgnoresStaleManagedConfiguration(t *testing.T) {
	home := t.TempDir()
	a := NewAdapter()
	a.lookPath = func(string) (string, error) { return filepath.Join(home, "bin", "code"), nil }
	for _, path := range []string{a.SystemPromptFile(home), a.MCPConfigPath(home, "codegraph")} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("stale"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	installed, _, _, _, err := a.Detect(context.Background(), home)
	if err != nil || installed {
		t.Fatalf("Detect() with stale managed config = %v, %v; want false, nil", installed, err)
	}
}

func TestStrategies(t *testing.T) {
	a := NewAdapter()

	if got := a.SystemPromptStrategy(); got != model.StrategyInstructionsFile {
		t.Fatalf("SystemPromptStrategy() = %v, want %v", got, model.StrategyInstructionsFile)
	}

	if got := a.MCPStrategy(); got != model.StrategyMCPConfigFile {
		t.Fatalf("MCPStrategy() = %v, want %v", got, model.StrategyMCPConfigFile)
	}
}

func TestSystemPromptFileUsesInstructionsExtension(t *testing.T) {
	a := NewAdapter()
	home := "/tmp/home"

	path := a.SystemPromptFile(home)
	if filepath.Ext(path) != ".md" {
		t.Fatalf("SystemPromptFile() should end with .md: %q", path)
	}

	if filepath.Base(path) != "gentle-ai.instructions.md" {
		t.Fatalf("SystemPromptFile() = %q, want filename gentle-ai.instructions.md", path)
	}
}

func TestSettingsPathUsesVSCodeUserProfile(t *testing.T) {
	a := NewAdapter()
	home := "/tmp/home"

	switch runtime.GOOS {
	case "darwin":
		path := a.SettingsPath(home)
		want := filepath.Join(home, "Library", "Application Support", "Code", "User", "settings.json")
		if path != want {
			t.Fatalf("SettingsPath() = %q, want %q", path, want)
		}
	case "windows":
		appData := filepath.Join(home, "AppData", "Roaming")
		t.Setenv("APPDATA", appData)
		path := a.SettingsPath(home)
		want := filepath.Join(appData, "Code", "User", "settings.json")
		if path != want {
			t.Fatalf("SettingsPath() = %q, want %q", path, want)
		}
	default:
		// A custom root never follows ambient XDG_CONFIG_HOME: containment wins.
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
		path := a.SettingsPath(home)
		want := filepath.Join(home, ".config", "Code", "User", "settings.json")
		if path != want {
			t.Fatalf("SettingsPath() = %q, want %q", path, want)
		}

		// The real user home is the only root that honors XDG_CONFIG_HOME.
		realHome, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("UserHomeDir() error = %v", err)
		}
		path = a.SettingsPath(realHome)
		want = filepath.Join(home, "xdg", "Code", "User", "settings.json")
		if path != want {
			t.Fatalf("SettingsPath(realHome) = %q, want %q", path, want)
		}
	}
}

func TestMCPConfigPathUsesVSCodeUserProfile(t *testing.T) {
	a := NewAdapter()
	home := "/tmp/home"

	switch runtime.GOOS {
	case "darwin":
		path := a.MCPConfigPath(home, "context7")
		want := filepath.Join(home, "Library", "Application Support", "Code", "User", "mcp.json")
		if path != want {
			t.Fatalf("MCPConfigPath() = %q, want %q", path, want)
		}
	case "windows":
		appData := filepath.Join(home, "AppData", "Roaming")
		t.Setenv("APPDATA", appData)
		path := a.MCPConfigPath(home, "context7")
		want := filepath.Join(appData, "Code", "User", "mcp.json")
		if path != want {
			t.Fatalf("MCPConfigPath() = %q, want %q", path, want)
		}
	default:
		// A custom root never follows ambient XDG_CONFIG_HOME: containment wins.
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
		path := a.MCPConfigPath(home, "context7")
		want := filepath.Join(home, ".config", "Code", "User", "mcp.json")
		if path != want {
			t.Fatalf("MCPConfigPath() = %q, want %q", path, want)
		}

		// The real user home is the only root that honors XDG_CONFIG_HOME.
		realHome, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("UserHomeDir() error = %v", err)
		}
		path = a.MCPConfigPath(realHome, "context7")
		want = filepath.Join(home, "xdg", "Code", "User", "mcp.json")
		if path != want {
			t.Fatalf("MCPConfigPath(realHome) = %q, want %q", path, want)
		}
	}
}

// useRealUserHome makes home the current user's home for os.UserHomeDir on
// every host, so the APPDATA and XDG_CONFIG_HOME overrides apply to it.
func useRealUserHome(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
}

// TestUserDirPerGOOS pins the injectable resolver behind SettingsPath and
// MCPConfigPath, which backup.WithConfigRoots reuses to admit a redirected
// %APPDATA%\Code\User. Overrides apply only to the real user home.
func TestUserDirPerGOOS(t *testing.T) {
	realHome := t.TempDir()
	otherHome := t.TempDir()
	appData := filepath.Join(t.TempDir(), "Roaming")
	xdg := filepath.Join(t.TempDir(), "xdg")
	useRealUserHome(t, realHome)
	tests := []struct {
		name    string
		home    string
		goos    string
		appData string
		xdg     string
		want    string
	}{
		{name: "windows APPDATA for the real home", home: realHome, goos: "windows", appData: appData, want: filepath.Join(appData, "Code", "User")},
		{name: "windows APPDATA for another home", home: otherHome, goos: "windows", appData: appData, want: filepath.Join(otherHome, "AppData", "Roaming", "Code", "User")},
		{name: "windows relative APPDATA", home: realHome, goos: "windows", appData: filepath.Join("relative", "Roaming"), want: filepath.Join(realHome, "AppData", "Roaming", "Code", "User")},
		{name: "windows unset APPDATA", home: realHome, goos: "windows", want: filepath.Join(realHome, "AppData", "Roaming", "Code", "User")},
		{name: "darwin", home: realHome, goos: "darwin", appData: appData, xdg: xdg, want: filepath.Join(realHome, "Library", "Application Support", "Code", "User")},
		{name: "linux XDG_CONFIG_HOME for the real home", home: realHome, goos: "linux", appData: appData, xdg: xdg, want: filepath.Join(xdg, "Code", "User")},
		{name: "linux XDG_CONFIG_HOME for another home", home: otherHome, goos: "linux", appData: appData, xdg: xdg, want: filepath.Join(otherHome, ".config", "Code", "User")},
		{name: "linux unset XDG_CONFIG_HOME", home: realHome, goos: "linux", appData: appData, want: filepath.Join(realHome, ".config", "Code", "User")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("APPDATA", tt.appData)
			t.Setenv("XDG_CONFIG_HOME", tt.xdg)
			if got := UserDir(tt.home, tt.goos); got != tt.want {
				t.Fatalf("UserDir(%q) = %q, want %q", tt.goos, got, tt.want)
			}
		})
	}
	t.Run("SettingsPath and MCPConfigPath use the host GOOS", func(t *testing.T) {
		t.Setenv("APPDATA", appData)
		t.Setenv("XDG_CONFIG_HOME", xdg)
		a := NewAdapter()
		for _, home := range []string{realHome, otherHome} {
			if got, want := a.SettingsPath(home), filepath.Join(UserDir(home, runtime.GOOS), "settings.json"); got != want {
				t.Fatalf("SettingsPath(%q) = %q, want %q", home, got, want)
			}
			if got, want := a.MCPConfigPath(home, "context7"), filepath.Join(UserDir(home, runtime.GOOS), "mcp.json"); got != want {
				t.Fatalf("MCPConfigPath(%q) = %q, want %q", home, got, want)
			}
		}
	})
}
