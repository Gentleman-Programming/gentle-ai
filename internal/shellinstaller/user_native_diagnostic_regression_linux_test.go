//go:build linux && amd64

package shellinstaller

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestUserNativeDiagnosticRegression(t *testing.T) {
	if testing.Short() {
		t.Skip("requires external Node process")
	}
	node := os.Getenv("GENTLE_PRIVATE_DIAGNOSTIC_NODE")
	if node == "" {
		t.Skip("requires protected qualified Node")
	}
	cases := []string{"utf8-tail", "planning-tail-only", "planning-throws", "plan-changed", "runtime-throws", "empty-tail", "compatibility-output", "symlink-existing"}
	for _, scenario := range cases {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			source := root + "/source"
			home := root + "/home"
			for _, p := range []string{source, source + "/scripts", home} {
				if e := os.Mkdir(p, 0700); e != nil {
					t.Fatal(e)
				}
			}
			tail := "DIAGNOSTIC_PRIVATE_" + strings.Repeat("界", 2000)
			if scenario == "empty-tail" {
				tail = ""
			}
			tailJSON, e := json.Marshal(tail)
			if e != nil {
				t.Fatal(e)
			}
			adapters := `export function hostAdapters() {
  return {
    marker: "receiver-preserved",
    run: async function(command, args, options) {
      if (this.marker !== "receiver-preserved") throw Error("adapter receiver changed");
      if (!options.env.HOME.endsWith("/home") || options.deadlineMs !== 12345 ||
          options.stderrTail !== 7 || options.signal !== "original-signal" ||
          options.nested.keep !== true) throw Error("adapter options changed");
      if (command !== "fixture-command" ||
          JSON.stringify(args) !== '["setup","value with spaces","$(not executed)"]') {
        throw Error("adapter argv changed");
      }
      return { code: 1, marker: "return-preserved", stderrTail: ` + string(tailJSON) + ` };
    }
  };
}`
			runtime := `export function createInstallerRuntime(options) {
  async function invoke() {
    return await options.adapters.run("fixture-command",
      ["setup", "value with spaces", "$(not executed)"], {
        env: options.env, deadlineMs: 12345, stderrTail: 7,
        signal: "original-signal", nested: { keep: true }
      });
  }
  return {
    collectPlan: async () => {
      if (options.adapters) await invoke();
`
			if scenario == "planning-throws" {
				runtime += `throw Error("fixture planning failure");`
			}
			runtime += `
      return { plan: { version: "same" } };
    },
    runInstall: async (selection) => {
      if (JSON.stringify(selection.plan) !== '{"version":"same"}' || selection.consent !== true) {
        throw Error("install selection changed");
      }
`
			if scenario == "runtime-throws" {
				runtime += `throw Error("fixture runtime failure");`
			}
			if scenario != "planning-tail-only" && scenario != "compatibility-output" {
				runtime += `
      if (options.adapters) {
        const result = await invoke();
        if (result.code !== 1 || result.marker !== "return-preserved") {
          throw Error("adapter result changed");
        }
      }
`
			}
			runtime += `
      return { outcome: "failed", reason: "unchanged", completed: [] };
    }
  };
}`
			for name, value := range map[string]string{"installer-runtime.mjs": runtime, "installer-probes.mjs": adapters} {
				if e := os.WriteFile(source+"/scripts/"+name, []byte(value), 0600); e != nil {
					t.Fatal(e)
				}
			}
			receipt := root + "/native-setup.stderr"
			target := root + "/sentinel"
			if e := os.WriteFile(target, []byte("preserve"), 0600); e != nil {
				t.Fatal(e)
			}
			if scenario == "symlink-existing" {
				if e := os.Symlink(target, receipt); e != nil {
					t.Fatal(e)
				}
			}
			before := nativeChildFixtureStamp(t, root)
			plan := map[string]any{"version": "same"}
			if scenario == "plan-changed" {
				plan["version"] = "changed"
			}
			request, e := json.Marshal(map[string]any{"operation": "run-with-location", "plan": plan, "consent": true})
			if e != nil {
				t.Fatal(e)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, node, "--liftoff-only", "--input-type=module", "-e", nativeRuntimeClient, source)
			cmd.Env = []string{"HOME=" + home, "PATH=/usr/bin:/bin", "UV_THREADPOOL_SIZE=1"}
			cmd.Stdin = bytes.NewReader(request)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			e = cmd.Run()
			if bytes.Contains(stdout.Bytes(), []byte("DIAGNOSTIC_PRIVATE_")) {
				t.Fatal("diagnostics leaked to stdout")
			}
			expected := `{"result":{"outcome":"failed","reason":"unchanged","completed":[]},"installation":null}`
			if scenario == "plan-changed" {
				expected = `{"result":{"outcome":"blocked","reason":"selection-changed","completed":[]},"installation":null}`
			}
			throws := scenario == "planning-throws" || scenario == "runtime-throws" || scenario == "symlink-existing"
			if throws {
				if e == nil || stdout.Len() != 0 {
					t.Fatal("expected exclusive/throw refusal", e, stdout.String())
				}
			} else if e != nil || stdout.String() != expected {
				t.Fatal("original result changed", e, stdout.String(), stderr.String())
			}
			data, readErr := os.ReadFile(receipt)
			if scenario == "utf8-tail" {
				info, statErr := os.Lstat(receipt)
				expected := []byte(tail)
				expected = expected[len(expected)-4096:]
				if readErr != nil || statErr != nil || !bytes.Equal(data, expected) || info.Mode().Perm() != 0600 {
					t.Fatal("private byte tail differs", readErr, statErr, len(data))
				}
			} else if scenario == "symlink-existing" {
				info, statErr := os.Lstat(receipt)
				if statErr != nil || info.Mode()&os.ModeSymlink == 0 || string(data) != "preserve" || before != nativeChildFixtureStamp(t, root) {
					t.Fatal("exclusive symlink refusal changed state", statErr)
				}
			} else if !os.IsNotExist(readErr) || before != nativeChildFixtureStamp(t, root) {
				t.Fatal("no-diagnostic case changed state", readErr)
			}
			sentinel, e := os.ReadFile(target)
			if e != nil || string(sentinel) != "preserve" {
				t.Fatal("sentinel changed", e)
			}
		})
	}
}
