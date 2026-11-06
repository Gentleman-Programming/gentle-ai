package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

func TestRunSyncPiWarnsWithoutReenablingBuiltinMCP(t *testing.T) {
	home := setupPiSyncHost(t)
	dir := filepath.Join(home, "isolated-pi")
	t.Setenv("PI_CODING_AGENT_DIR", dir)
	settingsPath := filepath.Join(dir, "settings.json")
	mustWriteFile(t, settingsPath, []byte(`{"packages":["npm:pi-mcp-adapter"],"extensions":["./custom.ts","-builtin:mcp"]}`))
	legacy := `{"mcpServers":{"context7":{"command":"npx","args":["context7-mcp"]}}}`
	mustWriteFile(t, filepath.Join(dir, "mcp-adapter.json"), []byte(legacy))
	mustWriteFile(t, filepath.Join(dir, "npm", "package.json"), []byte(`{"dependencies":{"pi-mcp-adapter":"5.0.0"}}`))
	for i := 0; i < 2; i++ {
		result, err := RunSyncWithSelection(home, piEngramSyncSelection())
		if err != nil {
			t.Fatal(err)
		}
		report := RenderSyncReport(result)
		warning := "- WARNING: Pi built-in MCP is disabled in " + settingsPath + "; pi-mcp-adapter is absent, so servers in " + filepath.Join(dir, "mcp.json") + " will not load. If you want these servers enabled, remove -builtin:mcp from extensions in " + settingsPath + ", then restart Pi and run `gentle-ai doctor`. If MCP is intentionally disabled, keep the setting."
		if strings.Count(report, warning) != 1 {
			t.Fatalf("sync %d missing exact warning:\n%s", i, report)
		}
		var settings struct {
			Packages   []string `json:"packages"`
			Extensions []string `json:"extensions"`
		}
		data, err := os.ReadFile(settingsPath)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &settings); err != nil {
			t.Fatal(err)
		}
		if strings.Join(settings.Extensions, ",") != "./custom.ts,-builtin:mcp" {
			t.Fatalf("user extensions changed: %s", data)
		}
		for _, pkg := range settings.Packages {
			if strings.Contains(pkg, "pi-mcp-adapter") {
				t.Fatalf("adapter not retired: %s", data)
			}
		}
		data, err = os.ReadFile(filepath.Join(dir, "mcp.json"))
		if err != nil {
			t.Fatal(err)
		}
		var migrated map[string]any
		if err := json.Unmarshal(data, &migrated); err != nil {
			t.Fatal(err)
		}
		if len(migrated["mcpServers"].(map[string]any)) != 1 {
			t.Fatalf("unexpected servers: %s", data)
		}
		data, err = os.ReadFile(filepath.Join(dir, "mcp-adapter.json"))
		if err != nil || string(data) != legacy {
			t.Fatalf("legacy changed: %s, %v", data, err)
		}
	}
}

func TestRunDoctorPiMCPDiagnosticIsReadOnly(t *testing.T) {
	for _, tt := range []struct {
		name, settings, npm, mcp string
		warn                     bool
		inactive                 bool
		dangling                 string
	}{
		{"disabled with servers", `{"extensions":["-builtin:mcp"]}`, `{}`, `{"mcpServers":{"context7":{"command":"npx"}}}`, true, false, ""},
		{"explicitly enabled", `{"extensions":["+builtin:mcp"]}`, `{}`, `{"mcpServers":{"context7":{}}}`, false, false, ""},
		{"no servers", `{"extensions":["-builtin:mcp"]}`, `{}`, `{"mcpServers":{}}`, false, false, ""},
		{"adapter in settings", `{"extensions":["-builtin:mcp"],"packages":[{"source":"npm:pi-mcp-adapter@5.0.0"}]}`, `{}`, `{"mcpServers":{"context7":{}}}`, false, false, ""},
		{"adapter in npm", `{"extensions":["-builtin:mcp"]}`, `{"dependencies":{"pi-mcp-adapter":"5.0.0"}}`, `{"mcpServers":{"context7":{}}}`, false, false, ""},
		{"filtered adapter", `{"extensions":["-builtin:mcp"],"packages":[{"source":"npm:pi-mcp-adapter@5.0.0","extensions":[]}]}`, `{}`, `{"mcpServers":{"context7":{}}}`, true, true, ""},
		{"filtered adapter with npm", `{"extensions":["-builtin:mcp"],"packages":[{"source":"npm:pi-mcp-adapter@5.0.0","extensions":[]}]}`, `{"dependencies":{"pi-mcp-adapter":"5.0.0"}}`, `{"mcpServers":{"context7":{}}}`, true, true, ""},
		{"autoload false does not disable extensions", `{"extensions":["-builtin:mcp"],"packages":[{"source":"npm:pi-mcp-adapter@5.0.0","autoload":false}]}`, `{}`, `{"mcpServers":{"context7":{}}}`, false, false, ""},
		{"absent settings", "", `{}`, `{"mcpServers":{"context7":{}}}`, false, false, ""},
		{"absent mcp", `{"extensions":["-builtin:mcp"]}`, `{}`, "", false, false, ""},
		{"dangling settings", `{}`, `{}`, `{"mcpServers":{"context7":{}}}`, true, false, "settings.json"},
		{"dangling mcp", `{"extensions":["-builtin:mcp"]}`, `{}`, `{}`, true, false, "mcp.json"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := setupPiSyncHost(t)
			dir := filepath.Join(home, ".pi", "agent")
			files := map[string]string{"settings.json": tt.settings, "npm/package.json": tt.npm, "mcp.json": tt.mcp}
			for name, body := range files {
				if body != "" {
					mustWriteFile(t, filepath.Join(dir, name), []byte(body))
				}
			}
			missing := filepath.Join(dir, "missing-target.json")
			var readErr error
			if tt.dangling != "" {
				path := filepath.Join(dir, tt.dangling)
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(missing, path); err != nil {
					if runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(1314)) {
						t.Skip("symlink creation requires Windows privilege")
					}
					t.Fatal(err)
				}
				_, readErr = os.ReadFile(path)
				if !os.IsNotExist(readErr) {
					t.Fatalf("expected dangling symlink read failure: %v", readErr)
				}
			}
			statePath := filepath.Join(home, ".gentle-ai", "state.json")
			state := `{"installed_agents":["pi"]}`
			mustWriteFile(t, statePath, []byte(state))
			oldHome := osUserHomeDirDoctor
			osUserHomeDirDoctor = func() (string, error) { return home, nil }
			t.Cleanup(func() { osUserHomeDirDoctor = oldHome })
			oldSpace := availableBytesFn
			availableBytesFn = func(string) (int64, error) { return 1 << 30, nil }
			t.Cleanup(func() { availableBytesFn = oldSpace })
			oldLook, oldPath := lookPathFn, pathDirsFn
			lookPathFn = func(name string) (string, error) { return filepath.Join(home, "bin", name), nil }
			pathDirsFn = func() []string { return nil }
			t.Cleanup(func() { lookPathFn, pathDirsFn = oldLook, oldPath })
			var out bytes.Buffer
			if err := RunDoctor(context.Background(), &out); err != nil {
				t.Fatal(err)
			}
			var line string
			for _, candidate := range strings.Split(out.String(), "\n") {
				if strings.Contains(candidate, "pi:mcp") {
					line = candidate
				}
			}
			if line == "" {
				t.Fatalf("Pi check not registered:\n%s", out.String())
			}
			if strings.Contains(line, "[!!]") != tt.warn {
				t.Fatalf("wrong diagnostic: %s", line)
			}
			if tt.warn {
				want := "  [!!]  pi:mcp                         Pi built-in MCP is disabled in " + filepath.Join(dir, "settings.json") + "; pi-mcp-adapter is absent, so servers in " + filepath.Join(dir, "mcp.json") + " will not load. If you want these servers enabled, remove -builtin:mcp from extensions in " + filepath.Join(dir, "settings.json") + ", then restart Pi and run `gentle-ai doctor`. If MCP is intentionally disabled, keep the setting."
				if tt.inactive {
					want = strings.Replace(want, "pi-mcp-adapter is absent", "pi-mcp-adapter is inactive", 1)
				}
				if tt.dangling != "" {
					want = "  [!!]  pi:mcp                         Pi MCP configuration could not be inspected: inspect " + filepath.Join(dir, tt.dangling) + ": " + readErr.Error() + "; inspect or repair it manually, then run `gentle-ai doctor`"
				}
				if line != want {
					t.Fatalf("wrong warning:\n got %s\nwant %s", line, want)
				}
			}
			for name, want := range files {
				path := filepath.Join(dir, name)
				if name == tt.dangling {
					if target, err := os.Readlink(path); err != nil || target != missing {
						t.Fatalf("doctor changed dangling symlink: %q, %v", target, err)
					}
					if _, err := os.Lstat(missing); !os.IsNotExist(err) {
						t.Fatalf("doctor created symlink target: %v", err)
					}
					continue
				}
				got, err := os.ReadFile(path)
				if want == "" && os.IsNotExist(err) {
					if _, err := os.Lstat(path); !os.IsNotExist(err) {
						t.Fatalf("doctor created absent path: %v", err)
					}
					continue
				}
				if err != nil || string(got) != want {
					t.Fatalf("doctor changed %s: %s, %v", name, got, err)
				}
			}
			if got, err := os.ReadFile(statePath); err != nil || string(got) != state {
				t.Fatalf("doctor changed stored state: %s, %v", got, err)
			}
		})
	}
}
