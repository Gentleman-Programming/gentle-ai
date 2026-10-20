package communitytool

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/opencode"
)

func TestReconcileOpenCodeCodeGraphFilePriority(t *testing.T) {
	valid := `{"type":"local","command":["codegraph","serve","--mcp"]}`
	disabled := `{"type":"local","command":["codegraph","serve","--mcp"],"disabled":true}`
	invalid := `{"type":"local","command":["other"]}`
	wrap := func(entry string) string { return `{"mcp":{"servers":{"codegraph":` + entry + `}}}` }
	for _, tt := range []struct {
		name, json, jsonc      string
		configured, diagnostic bool
		wantJSON               bool
	}{
		{"empty JSONC", wrap(valid), `{}`, true, false, true},
		{"unrelated JSONC", wrap(valid), "// preserve metadata\n" + `{"metadata":"keep","mcp":{"servers":{"other":{"type":"local","command":["other"]}}}}`, true, false, true},
		{"valid JSONC over invalid JSON", wrap(invalid), wrap(valid), true, false, false},
		{"valid JSONC over disabled JSON", wrap(disabled), wrap(valid), true, false, false},
		{"disabled JSONC over valid JSON", wrap(valid), wrap(disabled), false, true, false},
		{"invalid JSONC over valid JSON", wrap(valid), wrap(invalid), false, true, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", "")
			jsonPath := filepath.Join(home, ".config", "opencode", "opencode.json")
			jsoncPath := jsonPath + "c"
			mustWrite(t, jsonPath, tt.json)
			mustWrite(t, jsoncPath, tt.jsonc)
			calls := 0
			runner := RunnerFunc(func(string, ...string) error { calls++; return nil })
			for attempt := 0; attempt < 2; attempt++ {
				wantPath := ""
				if tt.configured {
					wantPath = jsoncPath
					if tt.wantJSON {
						wantPath = jsonPath
					}
				}
				adapter := opencode.NewAdapter()
				if path, configured := adapter.EffectiveCodeGraphWiring(home); path != wantPath || configured != tt.configured {
					t.Errorf("attempt %d: effective = (%q, %v), want (%q, %v)", attempt, path, configured, wantPath, tt.configured)
				}
				if err := adapter.NativeCodeGraphWiringError(home); (err != nil) != tt.diagnostic {
					t.Errorf("native diagnostic = %v, want error=%v", err, tt.diagnostic)
				}
				if got := NeedsOpenCodeCodeGraphReconcile(home); got == tt.configured {
					t.Errorf("needs reconcile = %v, configured=%v", got, tt.configured)
				}
				result, err := ReconcileOpenCodeCodeGraph(home, runner)
				if (err != nil) != tt.diagnostic || result.Changed || len(result.Files) != 0 {
					t.Errorf("reconcile = (%#v, %v), want unchanged, error=%v", result, err, tt.diagnostic)
				}
				if tt.diagnostic && err != nil {
					for _, want := range []string{jsoncPath, "mcp.servers.codegraph", "edit"} {
						if !strings.Contains(err.Error(), want) {
							t.Errorf("diagnostic %q must name %q", err, want)
						}
					}
				}
				for path, before := range map[string]string{jsonPath: tt.json, jsoncPath: tt.jsonc} {
					if after, err := os.ReadFile(path); err != nil || string(after) != before {
						t.Errorf("attempt %d: %s bytes = %q, error=%v", attempt, path, after, err)
					}
				}
			}
			if calls != 0 {
				t.Errorf("installer calls = %d, want zero", calls)
			}
		})
	}
}

func TestReconcileOpenCodeCodeGraphNativeValidPreservesBytes(t *testing.T) {
	for _, extension := range []string{".json", ".jsonc"} {
		for _, tt := range []struct{ name, command string }{
			{"canonical", `["codegraph","serve","--mcp"]`},
			{"no watch", `["codegraph","serve","--mcp","--no-watch"]`},
			{"absolute POSIX and path", `["/opt/bin/codegraph","serve","--mcp","--path","/work/my project"]`},
			{"absolute Windows and short path", `["C:\\Tools\\codegraph.exe","serve","-p","C:\\Work\\project","--mcp","--no-watch"]`},
		} {
			t.Run(extension+"/"+tt.name, func(t *testing.T) {
				home := t.TempDir()
				t.Setenv("XDG_CONFIG_HOME", "")
				path := filepath.Join(home, ".config", "opencode", "opencode"+extension)
				comment := ""
				if extension == ".jsonc" {
					comment = "// keep this user comment\n"
				}
				// Native enabled is not the legacy enable switch. Also retain unknown
				// user metadata and a stale legacy entry without rewriting either.
				before := fmt.Sprintf("{\n%s\"userMetadata\":{\"keep\":true},\"mcp\":{\"codegraph\":{\"type\":\"local\",\"command\":[\"other\"]},\"servers\":{\"codegraph\":{\"type\":\"local\",\"command\":%s,\"enabled\":false,\"disabled\":false,\"userMetadata\":\"keep\"}}}}\n", comment, tt.command)
				mustWrite(t, path, before)
				calls := 0
				runner := RunnerFunc(func(string, ...string) error {
					calls++
					return nil
				})
				for attempt := 0; attempt < 2; attempt++ {
					if NeedsOpenCodeCodeGraphReconcile(home) {
						t.Errorf("attempt %d: valid native command needs reconciliation", attempt)
					}
					result, err := ReconcileOpenCodeCodeGraph(home, runner)
					if err != nil || result.Changed || len(result.Files) != 0 {
						t.Errorf("attempt %d: reconcile = (%#v, %v), want unchanged success", attempt, result, err)
					}
					after, err := os.ReadFile(path)
					if err != nil || string(after) != before {
						t.Fatalf("attempt %d: config bytes changed: %q, error=%v", attempt, after, err)
					}
				}
				if calls != 0 {
					t.Errorf("installer called %d times for valid native wiring, want zero", calls)
				}
			})
		}
	}
}

func TestReconcileOpenCodeCodeGraphNativeUnmetCondition(t *testing.T) {
	for _, extension := range []string{".json", ".jsonc"} {
		for _, tt := range []struct {
			name, entry string
			disabled    bool
		}{
			{"wrong program", `{"type":"local","command":["other","serve","--mcp"]}`, false},
			{"unsupported option", `{"type":"local","command":["codegraph","serve","--mcp","--invented"]}`, false},
			{"typed command element", `{"type":"local","command":["codegraph","serve","--mcp",42]}`, false},
			{"wrong type", `{"type":"remote","command":["codegraph","serve","--mcp"]}`, false},
			{"disabled", `{"type":"local","command":["codegraph","serve","--mcp"],"disabled":true}`, true},
			{"nonboolean disabled", `{"type":"local","command":["codegraph","serve","--mcp"],"disabled":"false"}`, true},
			{"null disabled", `{"type":"local","command":["codegraph","serve","--mcp"],"disabled":null}`, true},
		} {
			t.Run(extension+"/"+tt.name, func(t *testing.T) {
				home := t.TempDir()
				t.Setenv("XDG_CONFIG_HOME", "")
				path := filepath.Join(home, ".config", "opencode", "opencode"+extension)
				comment := ""
				if extension == ".jsonc" {
					comment = "// preserve native user settings\n"
				}
				// A canonical legacy entry must never reactivate the native entry.
				before := fmt.Sprintf("{\n%s\"userMetadata\":\"keep\",\"mcp\":{\"codegraph\":{\"type\":\"local\",\"command\":[\"codegraph\",\"serve\",\"--mcp\"]},\"servers\":{\"codegraph\":%s}}}\n", comment, tt.entry)
				mustWrite(t, path, before)
				calls := 0
				runner := RunnerFunc(func(string, ...string) error {
					calls++
					return nil // Simulate an installer that cannot repair native schema.
				})
				for attempt := 0; attempt < 2; attempt++ {
					if !NeedsOpenCodeCodeGraphReconcile(home) {
						t.Errorf("attempt %d: unmet native condition was hidden by legacy wiring", attempt)
					}
					result, err := ReconcileOpenCodeCodeGraph(home, runner)
					if err == nil {
						t.Errorf("attempt %d: expected actionable native diagnostic", attempt)
					} else {
						message := err.Error()
						for _, want := range []string{path, "mcp.servers.codegraph", "edit"} {
							if !strings.Contains(strings.ToLower(message), strings.ToLower(want)) {
								t.Errorf("diagnostic %q must name %q", message, want)
							}
						}
						if tt.disabled {
							for _, want := range []string{"enable or remove", "complete sync"} {
								if !strings.Contains(message, want) {
									t.Errorf("diagnostic %q must say %q", message, want)
								}
							}
							if strings.Contains(message, "leave it disabled if intentional") {
								t.Errorf("diagnostic promises unsupported disabled sync: %q", message)
							}
							if !strings.Contains(strings.ToLower(message), "disabled") {
								t.Errorf("diagnostic %q must explain disabled native entry", message)
							}
						} else {
							for _, want := range []string{"codegraph", "serve", "--mcp"} {
								if !strings.Contains(message, want) {
									t.Errorf("diagnostic %q must name expected command token %q", message, want)
								}
							}
						}
					}
					if result.Changed || len(result.Files) != 0 {
						t.Errorf("attempt %d: native refusal reported writes: %#v", attempt, result)
					}
					after, readErr := os.ReadFile(path)
					if readErr != nil || string(after) != before {
						t.Fatalf("attempt %d: native refusal changed config bytes: %q, error=%v", attempt, after, readErr)
					}
				}
				if calls != 0 {
					t.Errorf("installer called %d times for authoritative invalid/disabled native entry, want zero", calls)
				}
			})
		}
	}
}
