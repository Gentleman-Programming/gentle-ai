package opencode

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeCodeGraphCommandVariants(t *testing.T) {
	for _, tt := range []struct {
		name, command string
		want          bool
	}{
		{"canonical", `["codegraph","serve","--mcp"]`, true},
		{"no watch", `["codegraph","serve","--mcp","--no-watch"]`, true},
		{"long path", `["codegraph","serve","--mcp","--path","/work/my project"]`, true},
		{"equals path", `["codegraph","serve","--mcp","--path=/work/my project"]`, true},
		{"empty equals path", `["codegraph","serve","--mcp","--path="]`, false},
		{"short path and no watch", `["codegraph","serve","-p","/work/project","--no-watch","--mcp"]`, true},
		{"absolute POSIX", `["/opt/tools/codegraph","serve","--mcp"]`, true},
		{"absolute Windows", `["C:\\Tools\\codegraph.exe","serve","--mcp","--no-watch"]`, true},
		{"absolute Windows npm shim", `["C:\\Tools\\codegraph.cmd","serve","--mcp"]`, true},
		{"relative executable", `["tools/codegraph","serve","--mcp"]`, false},
		{"wrong POSIX basename", `["/opt/tools/not-codegraph","serve","--mcp"]`, false},
		{"wrong Windows basename", `["C:\\Tools\\other.exe","serve","--mcp"]`, false},
		{"missing mcp", `["codegraph","serve","--no-watch"]`, false},
		{"wrong subcommand", `["codegraph","install","--mcp"]`, false},
		{"unknown flag", `["codegraph","serve","--mcp","--invented"]`, false},
		{"extra positional", `["codegraph","serve","--mcp","project"]`, false},
		{"missing path value", `["codegraph","serve","--mcp","--path"]`, false},
		{"flag as path value", `["codegraph","serve","--path","--mcp"]`, false},
		{"numeric executable", `[42,"serve","--mcp"]`, false},
		{"numeric path", `["codegraph","serve","--mcp","--path",42]`, false},
		{"boolean option", `["codegraph","serve","--mcp",true]`, false},
		{"null option", `["codegraph","serve","--mcp",null]`, false},
		{"object option", `["codegraph","serve","--mcp",{}]`, false},
		{"string command", `"codegraph serve --mcp"`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", "")
			a := NewAdapter()
			path := a.SettingsPath(home)
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			body := fmt.Sprintf(`{"mcp":{"servers":{"codegraph":{"type":"local","command":%s}}}}`, tt.command)
			if err := os.WriteFile(path, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			gotPath, got := a.EffectiveCodeGraphWiring(home)
			if got != tt.want || (got && gotPath != path) {
				t.Fatalf("EffectiveCodeGraphWiring() = (%q, %v), want configured=%v at %q", gotPath, got, tt.want, path)
			}
		})
	}
}

func TestNativeCodeGraphWiring(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		want       bool
	}{
		{"native", `{"mcp":{"servers":{"codegraph":{"type":"local","command":["codegraph","serve","--mcp"]}}}}`, true},
		{"legacy canonical", `{"mcp":{"codegraph":{"type":"local","command":["codegraph","serve","--mcp"],"enabled":true}}}`, true},
		{"legacy no watch", `{"mcp":{"codegraph":{"type":"local","command":["codegraph","serve","--mcp","--no-watch"]}}}`, true},
		{"legacy disabled", `{"mcp":{"codegraph":{"type":"local","command":["codegraph","serve","--mcp"],"enabled":false}}}`, false},
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
