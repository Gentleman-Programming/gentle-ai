package system_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

// TestCodexHomeResolver checks override resolution, rejection without mutation,
// and isolated-home behavior through the public resolver APIs.
func TestCodexHomeResolver(t *testing.T) {
	for _, scenario := range []string{"unset", "empty", "absolute", "relative", "missing", "file", "isolated"} {
		t.Run(scenario, func(t *testing.T) {
			home := t.TempDir()
			for key, value := range map[string]string{
				"HOME": home, "USERPROFILE": home,
				"APPDATA":         filepath.Join(home, "appdata"),
				"LOCALAPPDATA":    filepath.Join(home, "localappdata"),
				"XDG_CONFIG_HOME": filepath.Join(home, "xdg-config"),
				"XDG_DATA_HOME":   filepath.Join(home, "xdg-data"),
			} {
				t.Setenv(key, value)
			}
			root := filepath.Join(home, "custom-codex")
			if err := os.Mkdir(root, 0o755); err != nil {
				t.Fatal(err)
			}
			want, err := filepath.EvalSymlinks(root)
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv("CODEX_HOME", root)
			invalid := false
			switch scenario {
			case "unset", "empty":
				t.Setenv("CODEX_HOME", "")
				if scenario == "unset" {
					if err := os.Unsetenv("CODEX_HOME"); err != nil {
						t.Fatal(err)
					}
				}
				want = filepath.Join(home, ".codex")
			case "relative":
				t.Chdir(home)
				t.Setenv("CODEX_HOME", "./custom-codex/../custom-codex")
			case "missing", "file":
				invalid = true
				want = filepath.Join(home, "invalid")
				t.Setenv("CODEX_HOME", want)
				if scenario == "file" {
					if err := os.WriteFile(want, []byte("keep"), 0o644); err != nil {
						t.Fatal(err)
					}
				}
			case "isolated":
				home = t.TempDir()
				want = filepath.Join(home, ".codex")
			}
			if got := system.CodexConfigDir(home); got != want {
				t.Errorf("CodexConfigDir = %q, want %q", got, want)
			}
			err = system.ValidateCodexHome(home)
			if (err != nil) != invalid || (invalid && !strings.Contains(err.Error(), "CODEX_HOME")) {
				t.Errorf("ValidateCodexHome = %v, want rejection: %v", err, invalid)
			}
			if _, err := os.Stat(filepath.Join(home, ".codex")); !os.IsNotExist(err) {
				t.Errorf("resolver created fallback directory: %v", err)
			}
			if scenario == "missing" {
				if _, err := os.Stat(want); !os.IsNotExist(err) {
					t.Errorf("resolver created missing override: %v", err)
				}
			}
			if scenario == "file" {
				if raw, err := os.ReadFile(want); err != nil || string(raw) != "keep" {
					t.Errorf("resolver changed non-directory override: %q, %v", raw, err)
				}
			}
		})
	}
}
