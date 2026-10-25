//go:build linux && amd64

package shellinstaller

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUserNativeFailureReceipt(t *testing.T) {
	if testing.Short() {
		t.Skip("requires external Node process")
	}
	node := os.Getenv("GENTLE_PRIVATE_DIAGNOSTIC_NODE")
	if node == "" {
		t.Skip("requires qualified pinned Node")
	}
	for _, scenario := range []string{"failed", "non-failed", "legacy-run", "existing-receipt"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join(root, "source")
			scripts := filepath.Join(source, "scripts")
			home := filepath.Join(root, "home")
			for _, path := range []string{source, scripts, home} {
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			}
			outcome := "failed"
			if scenario == "non-failed" {
				outcome = "blocked"
			}
			runtime := `export function createInstallerRuntime(options) {
                return { collectPlan: async () => ({plan: {}}), runInstall: async () => {
                    if (options.adapters) {
                        const args = ["setup"];
                        const selection = {env: options.env, deadlineMs: 123, stderrTail: 4096};
                        const result = await options.adapters.run("fixture-command", args, selection);
                        if (result.code !== 1 || result.marker !== "preserved") throw Error("adapter result changed");
                    }
                    return {outcome: "` + outcome + `", failedStep: "shell-setup", completed: []};
                }};
            }`
			adapters := `export function hostAdapters() { return {fs: {}, run: async (command, args, options) => {
                if (command !== "fixture-command" || JSON.stringify(args) !== '["setup"]' ||
                    options.deadlineMs !== 123 || options.stderrTail !== 4096 || !options.env.HOME) throw Error("adapter call changed");
                return {code: 1, marker: "preserved", stderrTail: "sensitive diagnostic\n" + "x".repeat(5000)};
            }}; }`
			for name, data := range map[string]string{"installer-runtime.mjs": runtime, "installer-probes.mjs": adapters} {
				if err := os.WriteFile(filepath.Join(scripts, name), []byte(data), 0600); err != nil {
					t.Fatal(err)
				}
			}
			receipt := filepath.Join(root, "native-setup.stderr")
			if scenario == "existing-receipt" {
				if err := os.WriteFile(receipt, []byte("preserve"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			operation := "run-with-location"
			if scenario == "legacy-run" {
				operation = "run"
			}
			request, _ := json.Marshal(map[string]any{"operation": operation, "plan": map[string]any{}, "consent": true})
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, node, "--liftoff-only", "--input-type=module", "-e", nativeRuntimeClient, source)
			cmd.Env = []string{"HOME=" + home, "PATH=/usr/bin:/bin", "UV_THREADPOOL_SIZE=1"}
			cmd.Stdin = bytes.NewReader(request)
			output, err := cmd.CombinedOutput()
			if scenario == "existing-receipt" {
				data, readErr := os.ReadFile(receipt)
				if err == nil || readErr != nil || string(data) != "preserve" {
					t.Fatal("existing receipt overwritten", err, readErr)
				}
				return
			}
			if err != nil || !json.Valid(output) || bytes.Contains(output, []byte("sensitive diagnostic")) {
				t.Fatal("client result changed or diagnostics escaped", err, string(output))
			}
			expected := `{"outcome":"` + outcome + `","failedStep":"shell-setup","completed":[]}`
			if operation == "run-with-location" {
				expected = `{"result":` + expected + `,"installation":null}`
			}
			if string(output) != expected {
				t.Fatal("original result changed", string(output))
			}
			data, readErr := os.ReadFile(receipt)
			if scenario != "failed" {
				if !os.IsNotExist(readErr) {
					t.Fatal("non-failed or legacy result wrote receipt", readErr)
				}
				return
			}
			info, statErr := os.Stat(receipt)
			if readErr != nil || statErr != nil || string(data) != strings.Repeat("x", 4096) || info.Mode().Perm() != 0600 {
				t.Fatal("bounded private stderr receipt missing", readErr, statErr, len(data))
			}
		})
	}
}
