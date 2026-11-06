//go:build linux && shell_context7_canonical

package shellinstaller

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"
)

// Characterizes the published CLI, not a replacement MCP client or SDK session.
// Explicitly run inside the existing offline Guest with its read-only closure.
func TestUserContext7FrozenBuiltinMCPCLI(t *testing.T) {
	lockData, err := os.ReadFile("/source/scripts/user-locks/modern/package-lock.json")
	if err != nil {
		t.Fatal(err)
	}
	var lock struct {
		Packages map[string]struct{ Version, Resolved, Integrity string }
	}
	if err := json.Unmarshal(lockData, &lock); err != nil {
		t.Fatal(err)
	}
	pin := lock.Packages["node_modules/@earendil-works/pi-coding-agent"]
	if pin.Version == "" || pin.Resolved != "https://registry.npmjs.org/@earendil-works/pi-coding-agent/-/pi-coding-agent-"+pin.Version+".tgz" {
		t.Fatal("fixture must use the committed frozen Pi package")
	}
	data, err := os.ReadFile("/frozen-pi.tgz")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha512.Sum512(data)
	if "sha512-"+base64.StdEncoding.EncodeToString(digest[:]) != pin.Integrity {
		t.Fatal("archive differs from the frozen lock; do not execute")
	}
	pkg := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := context7ProofCommand(ctx, "/usr/bin/tar", "-xzf", "/frozen-pi.tgz", "--strip-components=1", "-C", pkg)
	if output, err := context7ProofOutput(cmd); err != nil {
		t.Fatalf("authenticated extraction: %v\n%s", err, output)
	}
	// Reuse the already installed read-only stock dependency closure, no lifecycle.
	if err := os.Symlink("/node_modules/@earendil-works/pi-coding-agent/node_modules", filepath.Join(pkg, "node_modules")); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, state         string
		enabled, broken     bool
		invalid, builtinOff bool
		exit                int
	}{
		{name: "connected", state: "connected", enabled: true},
		{name: "disabled", state: "disabled"},
		{name: "disconnected", enabled: true, broken: true, exit: 1},
		{name: "invalid", invalid: true, exit: 1},
		{name: "shell-health-is-not-session-readiness", state: "connected", enabled: true, builtinOff: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			var methods []string
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				requests++
				mu.Unlock()
				if tc.broken {
					http.NotFound(w, r)
					return
				}
				if r.Method != http.MethodPost {
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				var request struct {
					ID     json.RawMessage `json:"id"`
					Method string          `json:"method"`
				}
				if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536)).Decode(&request); err != nil {
					http.Error(w, "invalid fixture request", http.StatusBadRequest)
					return
				}
				mu.Lock()
				methods = append(methods, request.Method)
				mu.Unlock()
				if len(request.ID) == 0 {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				var result any
				switch request.Method {
				case "initialize":
					result = map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]string{"name": "offline-docs-fixture", "version": "1"}}
				case "tools/list":
					result = map[string]any{"tools": []any{map[string]any{"name": "fixture_docs", "description": "Offline fixture, not the Context7 service", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{}}}}}
				default:
					result = map[string]any{}
				}
				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result}); err != nil {
					t.Errorf("fixture response: %v", err)
				}
			}))
			defer server.Close()
			root := installStagePath(t)
			var profile *UserExperience
			if tc.name == "disabled" {
				selected := UserExperience{Persona: "gentleman", BackgroundSubagents: "on", Context7: "off"}
				profile = &selected
			}
			if err := userPrepareInstallStage(ctx, root, "separate", profile); err != nil {
				t.Fatal(err)
			}
			agent := filepath.Join(root, "agent")
			entry := map[string]any{"url": server.URL, "enabled": tc.enabled, "exposure": "codemode", "timeout": 1}
			if tc.invalid {
				entry = map[string]any{"url": 17}
			}
			config, err := json.Marshal(map[string]any{"mcpServers": map[string]any{"context7": entry}})
			if err != nil {
				t.Fatal(err)
			}
			configPath := filepath.Join(agent, "mcp.json")
			transport := server.URL
			if tc.name == "disabled" {
				// Consume the preparer's unchanged official OFF file, not a fixture URL.
				transport = "https://mcp.context7.com/mcp"
				config, err = os.ReadFile(configPath)
				if err != nil {
					t.Fatal(err)
				}
			} else if err := userToolWrite(configPath, config, 0600); err != nil {
				// Local transport bytes are fixtures, not fixed-endpoint producer output.
				t.Fatal(err)
			}
			before, err := os.Stat(configPath)
			if err != nil {
				t.Fatal(err)
			}
			if tc.builtinOff {
				if err := os.WriteFile(filepath.Join(agent, "settings.json"), []byte(`{"extensions":["-builtin:mcp"]}`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			runCtx, stop := context.WithTimeout(ctx, 15*time.Second)
			defer stop()
			cmd := context7ProofCommand(runCtx, "/opt/node/bin/node", filepath.Join(pkg, "dist/bundle/cli.js"), "mcp", "list", "--json")
			cmd.Dir = filepath.Join(root, "project")
			cmd.Env = []string{"HOME=" + filepath.Join(root, "home"), "PI_CODING_AGENT_DIR=" + agent, "GENTLE_PI_CONFIG_HOME=" + filepath.Join(root, "config"), "PATH=/opt/node/bin:/usr/bin:/bin", "GENTLE_AI_TELEMETRY_DISABLE=1"}
			output, runErr := context7ProofOutput(cmd)
			if runCtx.Err() != nil {
				t.Fatalf("CLI did not finish/reap: %v\n%s", runCtx.Err(), output)
			}
			exit := 0
			if runErr != nil {
				var exitErr *exec.ExitError
				if !errors.As(runErr, &exitErr) {
					t.Fatalf("CLI start failed: %v\n%s", runErr, output)
				}
				exit = exitErr.ExitCode()
			}
			if exit != tc.exit {
				t.Fatalf("CLI exit %d want %d: %s", exit, tc.exit, output)
			}
			var report struct {
				Servers []struct {
					Name, State, Scope, Source, Exposure, Error, Transport string
					Enabled                                                bool
					Tools                                                  []string
				}
				Errors []json.RawMessage
			}
			if err := json.Unmarshal(output, &report); err != nil {
				t.Fatalf("actual CLI JSON: %v\n%s", err, output)
			}
			if tc.invalid {
				if len(report.Errors) == 0 || len(report.Servers) != 0 {
					t.Fatalf("invalid config did not fail closed: %s", output)
				}
			} else {
				if len(report.Errors) != 0 || len(report.Servers) != 1 {
					t.Fatalf("unexpected CLI reports: %s", output)
				}
				s := report.Servers[0]
				if s.Name != "context7" || s.Scope != "global" || s.Source != configPath || s.Enabled != tc.enabled || s.Exposure != "codemode" || s.Transport != transport {
					t.Fatalf("canonical policy/source mismatch: %s", output)
				}
				if tc.broken {
					if s.State == "connected" || s.Error == "" || len(s.Tools) != 0 {
						t.Fatalf("broken endpoint reported healthy: %s", output)
					}
				} else if s.State != tc.state || (tc.enabled && !slices.Equal(s.Tools, []string{"fixture_docs"})) || (!tc.enabled && len(s.Tools) != 0) {
					t.Fatalf("connection/tool discovery mismatch: %s", output)
				}
			}
			mu.Lock()
			observed := slices.Clone(methods)
			requestCount := requests
			mu.Unlock()
			if tc.enabled && !tc.broken {
				if !slices.Contains(observed, "initialize") || !slices.Contains(observed, "tools/list") {
					t.Fatalf("no actual MCP handshake/tool discovery: %v", observed)
				}
			} else if tc.broken {
				if requestCount == 0 {
					t.Fatal("disconnection control never contacted the broken fixture")
				}
			} else if requestCount != 0 {
				t.Fatalf("disabled/invalid endpoint was contacted %d times", requestCount)
			}
			after, err := os.Stat(configPath)
			if err != nil {
				t.Fatal(err)
			}
			readback, err := os.ReadFile(configPath)
			if err != nil || string(readback) != string(config) || !os.SameFile(before, after) || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
				t.Fatal("read-only CLI health mutated canonical config")
			}
			t.Logf("FROZEN_PI_BUILTIN_MCP_CLI %s exit=%d requests=%d methods=%v report=%s", tc.name, exit, requestCount, observed, output)
		})
	}
}
