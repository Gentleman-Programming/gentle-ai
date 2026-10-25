package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Exercise the public doctor entry point with isolated homes and deterministic
// unrelated checks. Doctor must never rewrite state or create missing configs.
func TestRunDoctor_AgentConfigDirectories(t *testing.T) {
	tests := []struct {
		id  string
		dir string
	}{
		{"claude-code", ".claude"},
		{"opencode", ".config/opencode"},
		{"cursor", ".cursor"},
		{"windsurf", ".codeium/windsurf"},
		{"codex", ".codex"},
		{"pi", ".pi"},
		{"gemini-cli", ".gemini"},
		{"kilocode", ".config/kilo"},
		{"vscode-copilot", ".copilot"},
		{"kiro-ide", ".kiro"},
		{"kimi", ".kimi"},
		{"qwen-code", ".qwen"},
		{"openclaw", ".openclaw"},
		{"trae-ide", ".trae"},
		{"hermes", ".hermes"},
		{"antigravity", ".gemini/antigravity"},
	}
	for _, tt := range tests {
		for _, present := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/present=%t", tt.id, present), func(t *testing.T) {
				home := t.TempDir()
				dir := filepath.Join(home, filepath.FromSlash(tt.dir))
				if present {
					if err := os.MkdirAll(dir, 0o755); err != nil {
						t.Fatal(err)
					}
				}
				output := runDoctorAgentConfigFixture(t, home, fmt.Sprintf(`{"installed_agents":[%q]}`, tt.id))
				want := fmt.Sprintf("  [!!]  state:json                     state lists 1 agent(s) whose config dirs are missing: %s", tt.id)
				if present {
					want = fmt.Sprintf("  [ok]  state:json                     state file OK — 1 agent(s) installed: %s", tt.id)
				}
				if !strings.Contains(output, want+"\n") {
					t.Fatalf("missing exact state result %q:\n%s", want, output)
				}
				if !present {
					if !strings.Contains(output, "Run 'gentle-ai sync' to restore missing config files") {
						t.Fatalf("missing sync remedy:\n%s", output)
					}
					if _, err := os.Lstat(dir); !os.IsNotExist(err) {
						t.Fatalf("doctor created missing config: %v", err)
					}
				}
			})
		}
	}
}

func TestRunDoctor_AgentConfigSpecialCases(t *testing.T) {
	tests := []struct {
		name, payload, want string
		dirs                []string
	}{
		{"kimi current", `{"installed_agents":["kimi"]}`, "  [ok]  state:json                     state file OK — 1 agent(s) installed: kimi", []string{".kimi-code"}},
		{"kimi both layouts", `{"installed_agents":["kimi"]}`, "  [ok]  state:json                     state file OK — 1 agent(s) installed: kimi", []string{".kimi", ".kimi-code"}},
		{"conductor detection only", `{"installed_agents":["conductor"]}`, "  [ok]  state:json                     state file OK — 1 agent(s) installed: conductor", nil},
		{"unknown", `{"installed_agents":["future-agent"]}`, "  [!!]  state:json                     state lists unrecognized agent IDs: future-agent; inspect or repair the state file, then re-run 'gentle-ai doctor'", nil},
		{"unknown and missing", `{"installed_agents":["future-agent","pi"]}`, "  [!!]  state:json                     state lists unrecognized agent IDs: future-agent; inspect or repair the state file, then re-run 'gentle-ai doctor'; config dirs are missing: pi", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			for _, dir := range tt.dirs {
				if err := os.MkdirAll(filepath.Join(home, dir), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			output := runDoctorAgentConfigFixture(t, home, tt.payload)
			if !strings.Contains(output, tt.want+"\n") {
				t.Fatalf("missing exact state result %q:\n%s", tt.want, output)
			}
			if tt.name == "unknown and missing" {
				if !strings.Contains(output, "       Remedy: Run 'gentle-ai sync' to restore missing config files\n") {
					t.Fatalf("missing sync remedy for absent managed config:\n%s", output)
				}
			} else if strings.Contains(output, "gentle-ai sync") {
				t.Fatalf("unexpected sync remedy:\n%s", output)
			}
		})
	}
}

func runDoctorAgentConfigFixture(t *testing.T, home, payload string) string {
	t.Helper()
	stubDoctorToolProbe(t)
	originalLook, originalAvail, originalDirs := lookPathFn, availableBytesFn, pathDirsFn
	originalHome, originalExe := osUserHomeDirDoctor, osExecutableDoctor
	originalHTTP := httpGetFn
	t.Cleanup(func() {
		lookPathFn, availableBytesFn, pathDirsFn = originalLook, originalAvail, originalDirs
		osUserHomeDirDoctor, osExecutableDoctor = originalHome, originalExe
		httpGetFn = originalHTTP
	})
	lookPathFn = func(name string) (string, error) { return filepath.Join(home, "bin", name), nil }
	availableBytesFn = func(string) (int64, error) { return 1024 * 1024 * 1024, nil }
	pathDirsFn = func() []string { return []string{filepath.Join(home, "bin")} }
	osUserHomeDirDoctor = func() (string, error) { return home, nil }
	osExecutableDoctor = func() (string, error) { return filepath.Join(home, "bin", "gentle-ai"), nil }
	t.Setenv(engramHealthEnvVar, "https://engram.example.test")
	httpGetFn = func(string, time.Duration) (int, error) { return 200, nil }
	statePath := filepath.Join(home, ".gentle-ai", "state.json")
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := RunDoctor(ctx, &output); err != nil {
		t.Fatalf("RunDoctor: %v", err)
	}
	after, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != payload {
		t.Fatal("doctor modified state")
	}
	return output.String()
}
