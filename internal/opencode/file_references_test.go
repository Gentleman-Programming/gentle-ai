package opencode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestSettingsFileReferencesLeavesEmptyPlaceholderLiteral(t *testing.T) {
	home := t.TempDir()
	settings := filepath.Join(home, "opencode.json")
	promptFile := filepath.Join(home, "prompt.md")
	if err := os.WriteFile(promptFile, []byte("Keep this prompt.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, prompt string
		wantFile     bool
	}{
		{"empty", "{file:}", false},
		{"embedded empty", "Before {file:} after", false},
		{"empty alongside valid", "Before {file:} and {file:./prompt.md} after", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original, err := json.Marshal(map[string]any{"agent": map[string]any{"custom": map[string]any{"prompt": tc.prompt}}})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(settings, original, 0o600); err != nil {
				t.Fatal(err)
			}
			refs, err := SettingsFileReferences(settings, home)
			if err != nil {
				t.Fatal(err)
			}
			var want []string
			if tc.wantFile {
				want = []string{promptFile}
			}
			if !slices.Equal(refs, want) {
				t.Fatalf("references = %v, want %v", refs, want)
			}
			if err := ValidateSettingsFileReferences(settings, home); err != nil {
				t.Fatalf("literal placeholder rejected: %v", err)
			}
			if after, err := os.ReadFile(settings); err != nil || string(after) != string(original) {
				t.Fatalf("settings changed: %s (%v)", after, err)
			}
		})
	}
}

func TestSettingsFileReferences(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	absolute := filepath.Join(home, "absolute.md")
	for _, relative := range []string{"relative.md", "../parent.md", "~/home.md", absolute} {
		t.Run(relative, func(t *testing.T) {
			settings := filepath.Join(dir, "opencode.jsonc")
			encoded, err := json.Marshal(map[string]any{"agent": map[string]any{"custom": map[string]any{"prompt": "Before {file:" + relative + "} after"}}, "instructions": []any{"{file:" + relative + "}"}})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(settings, append([]byte("// {file:ignored-comment.md}\n"), encoded...), 0o600); err != nil {
				t.Fatal(err)
			}
			refs, err := SettingsFileReferences(settings, home)
			if err != nil {
				t.Fatal(err)
			}
			want := relative
			if strings.HasPrefix(want, "~/") {
				want = filepath.Join(home, want[2:])
			} else if !filepath.IsAbs(want) {
				want = filepath.Join(dir, want)
			}
			if !slices.Equal(refs, []string{filepath.Clean(want)}) {
				t.Fatalf("references = %v, want %s", refs, want)
			}
		})
	}
}
