package theme

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v3/internal/agents/claude"
	"github.com/gentleman-programming/gentle-ai/v3/internal/agents/opencode"
)

func claudeAdapter() agents.Adapter   { return claude.NewAdapter() }
func opencodeAdapter() agents.Adapter { return opencode.NewAdapter() }

func TestThemeInjectionNeverWritesOrSelectsTheme(t *testing.T) {
	for _, tt := range []struct {
		name    string
		adapter agents.Adapter
	}{
		{"claude", claudeAdapter()},
		{"opencode", opencodeAdapter()},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			settings := tt.adapter.SettingsPath(home)
			if err := os.MkdirAll(filepath.Dir(settings), 0o755); err != nil {
				t.Fatal(err)
			}
			original := []byte(`{"theme":"kanagawa","keep":true}`)
			if err := os.WriteFile(settings, original, 0o644); err != nil {
				t.Fatal(err)
			}
			for _, inject := range []func(string, agents.Adapter) (InjectionResult, error){Inject, InjectVisualThemes} {
				result, err := inject(home, tt.adapter)
				if err != nil || result.Changed {
					t.Fatalf("inject = %+v, %v; want no change", result, err)
				}
			}
			got, err := os.ReadFile(settings)
			if err != nil || string(got) != string(original) {
				t.Fatalf("settings changed: %q, %v", got, err)
			}
			for _, path := range VisualThemePaths(home, tt.adapter) {
				if _, err := os.Lstat(path); !os.IsNotExist(err) {
					t.Fatalf("theme asset %q created: %v", path, err)
				}
			}
		})
	}
}

func TestIsManagedVisualThemeRequiresExactContent(t *testing.T) {
	for _, tt := range []struct {
		name    string
		adapter agents.Adapter
		values  []any
	}{
		{"claude", claudeAdapter(), []any{axiomClaudeTheme, axiomDarkClaudeTheme}},
		{"opencode", opencodeAdapter(), []any{axiomOpenCodeTheme, axiomDarkOpenCodeTheme}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for i, path := range VisualThemePaths(t.TempDir(), tt.adapter) {
				content, err := json.MarshalIndent(tt.values[i], "", "  ")
				if err != nil {
					t.Fatal(err)
				}
				content = append(content, '\n')
				if !IsManagedVisualTheme(path, tt.adapter, content) {
					t.Fatalf("managed theme %q not recognised", path)
				}
				if IsManagedVisualTheme(path, tt.adapter, append(content, ' ')) {
					t.Fatalf("modified theme %q recognised", path)
				}
				if IsManagedVisualTheme(filepath.Join(filepath.Dir(path), "gentleman.json"), tt.adapter, content) {
					t.Fatal("legacy name recognised without provenance")
				}
			}
		})
	}
}

func TestRetireManagedVisualThemesRemovesExactAndPreservesModified(t *testing.T) {
	for _, tt := range []struct {
		name    string
		adapter agents.Adapter
		values  []any
	}{
		{"claude", claudeAdapter(), []any{axiomClaudeTheme, axiomDarkClaudeTheme}},
		{"opencode", opencodeAdapter(), []any{axiomOpenCodeTheme, axiomDarkOpenCodeTheme}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			paths := VisualThemePaths(home, tt.adapter)
			if err := os.MkdirAll(filepath.Dir(paths[0]), 0o755); err != nil {
				t.Fatal(err)
			}
			for i, path := range paths {
				content, err := json.MarshalIndent(tt.values[i], "", "  ")
				if err != nil {
					t.Fatal(err)
				}
				content = append(content, '\n')
				if i == 1 {
					content = append(content, ' ')
				}
				if err := os.WriteFile(path, content, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			settings := tt.adapter.SettingsPath(home)
			if err := os.WriteFile(settings, []byte(`{"theme":"personal"}`), 0o644); err != nil {
				t.Fatal(err)
			}
			owned, err := ManagedVisualThemePaths(home, tt.adapter)
			if err != nil || len(owned) != 1 || owned[0] != paths[0] {
				t.Fatalf("owned = %v, %v", owned, err)
			}
			result, err := RetireManagedVisualThemes(home, tt.adapter)
			if err != nil || !result.Changed || len(result.Files) != 1 || result.Files[0] != paths[0] {
				t.Fatalf("retire = %+v, %v", result, err)
			}
			if _, err := os.Stat(paths[0]); !os.IsNotExist(err) {
				t.Fatalf("exact asset remains: %v", err)
			}
			if _, err := os.Stat(paths[1]); err != nil {
				t.Fatalf("modified asset removed: %v", err)
			}
			if got, err := os.ReadFile(settings); err != nil || string(got) != `{"theme":"personal"}` {
				t.Fatalf("settings changed: %q, %v", got, err)
			}
		})
	}
}

func TestRetireManagedVisualThemesRejectsLinkedThemeAncestor(t *testing.T) {
	for _, tt := range []struct {
		name    string
		adapter agents.Adapter
		value   any
	}{
		{"claude", claudeAdapter(), axiomClaudeTheme},
		{"opencode", opencodeAdapter(), axiomOpenCodeTheme},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			external := t.TempDir()
			path := VisualThemePaths(home, tt.adapter)[0]
			if err := os.MkdirAll(filepath.Dir(filepath.Dir(path)), 0o755); err != nil {
				t.Fatal(err)
			}
			content, err := json.MarshalIndent(tt.value, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			content = append(content, '\n')
			externalTheme := filepath.Join(external, filepath.Base(path))
			if err := os.WriteFile(externalTheme, content, 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(external, filepath.Dir(path)); err != nil {
				if runtime.GOOS == "windows" {
					t.Skipf("Windows no permite crear el enlace en este entorno: %v", err)
				}
				t.Fatal(err)
			}
			owned, err := ManagedVisualThemePaths(home, tt.adapter)
			if err != nil || len(owned) != 0 {
				t.Fatalf("linked theme detected as managed: %v, %v", owned, err)
			}
			result, err := RetireManagedVisualThemes(home, tt.adapter)
			if err != nil || result.Changed {
				t.Fatalf("linked theme retired: %+v, %v", result, err)
			}
			if got, err := os.ReadFile(externalTheme); err != nil || string(got) != string(content) {
				t.Fatalf("external theme changed: %q, %v", got, err)
			}
		})
	}
}
