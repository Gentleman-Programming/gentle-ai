package opencode

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNativeCodeGraphWiring(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		want       bool
	}{
		{"native", `{"mcp":{"servers":{"codegraph":{"type":"local","command":["codegraph","serve","--mcp"]}}}}`, true},
		{"native disabled shadows legacy", `{"mcp":{"codegraph":{"type":"local","command":["codegraph","serve","--mcp"]},"servers":{"codegraph":{"type":"local","command":["codegraph","serve","--mcp"],"disabled":true}}}}`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", "")
			a := NewAdapter()
			path := a.SettingsPath(home)
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(tt.body), 0600); err != nil {
				t.Fatal(err)
			}
			_, got := a.EffectiveCodeGraphWiring(home)
			if got != tt.want {
				t.Fatalf("configured=%v want %v", got, tt.want)
			}
		})
	}
}
