package communitytool

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
