package opencode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

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
