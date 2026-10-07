package testenv_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/v4/internal/testenv"
)

// The child mocks the OS home before isolation, as affected package tests do.
// Write through the real adapter path so a leaked XDG override damages only
// the synthetic external sentinel, never an ambient OpenCode configuration.
func runOpenCodeSentinelSubprocess() {
	testenv.Isolate()
	if explicit := os.Getenv("GENTLE_AI_TESTENV_EXPLICIT_XDG"); explicit != "" {
		if err := os.Setenv("XDG_CONFIG_HOME", explicit); err != nil {
			panic(err)
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	path := opencode.NewAdapter().SystemPromptFile(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		panic(err)
	}
	if err := os.WriteFile(path, []byte("isolated prompt\n"), 0o600); err != nil {
		panic(err)
	}
	os.Exit(0)
}

func TestIsolateProtectsExternalOpenCodeConfig(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux XDG resolution regression")
	}
	for _, scenario := range []string{"inherited", "unset", "explicit_after_isolation"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			home := filepath.Join(root, "home")
			external := filepath.Join(root, "external")
			explicit := filepath.Join(root, "explicit")
			sentinel := filepath.Join(external, "opencode", "AGENTS.md")
			original := []byte("external configuration must remain unchanged\n")
			if err := os.MkdirAll(filepath.Dir(sentinel), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(sentinel, original, 0o600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0])
			// Replace inherited values rather than append duplicate environment keys.
			env := map[string]string{
				isolateSentinelSubprocessEnv: "xdg",
				"HOME":                       home, "USERPROFILE": home,
				"XDG_CONFIG_HOME":                external,
				"OPENCODE_CONFIG_DIR":            filepath.Join(root, "override-opencode"),
				"PI_CODING_AGENT_DIR":            filepath.Join(root, "override-pi"),
				"GENTLE_AI_TESTENV_EXPLICIT_XDG": "",
			}
			if scenario == "unset" {
				delete(env, "XDG_CONFIG_HOME")
			}
			want := filepath.Join(home, ".config", "opencode", "AGENTS.md")
			if scenario == "explicit_after_isolation" {
				env["GENTLE_AI_TESTENV_EXPLICIT_XDG"] = explicit
				want = filepath.Join(explicit, "opencode", "AGENTS.md")
			}
			for _, entry := range os.Environ() {
				i := strings.IndexByte(entry, '=')
				if i < 0 {
					continue
				}
				key := entry[:i]
				if _, replaced := env[key]; !replaced && key != "XDG_CONFIG_HOME" {
					cmd.Env = append(cmd.Env, entry)
				}
			}
			for key, value := range env {
				cmd.Env = append(cmd.Env, key+"="+value)
			}
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("OpenCode subprocess failed: %v (context: %v)\n%s", err, ctx.Err(), out)
			}
			got, err := os.ReadFile(sentinel)
			if err != nil || string(got) != string(original) {
				t.Fatalf("external sentinel changed: content=%q err=%v", got, err)
			}
			got, err = os.ReadFile(want)
			if err != nil || string(got) != "isolated prompt\n" {
				t.Fatalf("isolated prompt missing or incorrect: content=%q err=%v", got, err)
			}
		})
	}
}
