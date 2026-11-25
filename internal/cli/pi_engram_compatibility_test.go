package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v5/internal/installcmd"
	"github.com/gentleman-programming/gentle-ai/v5/internal/state"
	"github.com/gentleman-programming/gentle-ai/v5/internal/system"
	"github.com/gentleman-programming/gentle-ai/v5/internal/verify"
)

func TestPiEngramCompatibility(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX script stands in for the Engram binary")
	}
	for _, verb := range []string{"install", "sync"} {
		for _, tc := range []struct {
			name         string
			pathSupports bool
			override     string
			external     bool
			incompatible bool
			persistFails bool
		}{
			{name: "old PATH core", incompatible: true},
			{name: "compatible core", pathSupports: true},
			{name: "old explicit companion core", pathSupports: true, override: "ENGRAM_BIN", incompatible: true},
			{name: "compatible isolated core", override: system.EngramBinEnv},
			{name: "old isolated core", pathSupports: true, override: system.EngramBinEnv, incompatible: true},
			{name: "external server", external: true},
			{name: "old core and persistence failure", incompatible: true, persistFails: true},
		} {
			t.Run(verb+"/"+tc.name, func(t *testing.T) {
				home := setupPiSyncHost(t)
				agentDir := filepath.Join(home, "pi-agent")
				t.Setenv("PI_CODING_AGENT_DIR", agentDir)
				t.Setenv("GENTLE_AI_PI_CONFIG_DIR", filepath.Join(home, "pi-config"))
				t.Setenv("GENTLE_AI_CHANNEL", "stable")
				t.Setenv(state.HomeEnv, "")
				t.Setenv("GENTLE_AI_TELEMETRY_DISABLE", "1")
				t.Setenv("ENGRAM_URL", "")
				t.Setenv("ENGRAM_BIN", "")
				t.Setenv(system.EngramBinEnv, "")
				settings := filepath.Join(agentDir, "settings.json")
				mustWriteFile(t, settings, []byte(`{"packages":["npm:gentle-engram","npm:pi-mcp-adapter"]}`))
				if tc.persistFails {
					mustWriteFile(t, filepath.Join(agentDir, "mcp-adapter.json"), []byte(`{"mcpServers":{"context7":{"command":"npx","args":["context7-mcp"]}}}`))
					if err := os.MkdirAll(mustInstallStateLockPath(t, home), 0o755); err != nil {
						t.Fatal(err)
					}
				}
				store := filepath.Join(home, ".engram", "store")
				mustWriteFile(t, store, []byte("existing memories"))
				t.Setenv("ENGRAM_DATA_DIR", filepath.Dir(store))
				t.Setenv("ENGRAM_NO_UPDATE_CHECK", "0")

				binDir := t.TempDir()
				t.Setenv("PATH", binDir)
				log := filepath.Join(binDir, "probe.log")
				t.Setenv("ENGRAM_FIXTURE_PROBE_LOG", log)
				bin := filepath.Join(binDir, "engram")
				writePiEngramCompatibilityCore(t, bin, tc.pathSupports)
				if tc.override != "" {
					bin = filepath.Join(t.TempDir(), "engram")
					writePiEngramCompatibilityCore(t, bin, !tc.incompatible)
					t.Setenv(tc.override, bin)
				}
				if tc.external {
					t.Setenv("ENGRAM_URL", "http://engram.invalid")
				}
				fixtureLookPath := cmdLookPath
				cmdLookPath = func(name string) (string, error) {
					if name == "engram" {
						return exec.LookPath(name)
					}
					return fixtureLookPath(name)
				}
				t.Cleanup(installcmd.OverrideLookPath(cmdLookPath))

				var report verify.Report
				var err error
				args := []string{"--agent", "pi", "--scope", "global"}
				if verb == "install" {
					args = append(args, "--component", "engram")
					result, runErr := RunInstall(args, system.DetectionResult{Dependencies: system.DependencyReport{AllPresent: true}})
					report, err = result.Verify, runErr
				} else {
					persisted := state.InstallState{InstalledAgents: []string{"pi"}}
					persisted.SetSelection(piEngramSyncSelection())
					if err := state.Write(home, persisted); err != nil {
						t.Fatal(err)
					}
					result, runErr := RunSync(args)
					report, err = result.Verify, runErr
				}

				reportPrefix := "Verification checks: 0 passed, "
				passed := ""
				if verb == "install" {
					reportPrefix = "Verification checks: 2 passed, "
					passed = "[ok] verify:engram:binary - engram binary on PATH (restart shell if missing)\n[ok] verify:engram:version - engram version returns valid output\n"
				}
				if tc.incompatible {
					if err == nil || report.Ready {
						t.Fatalf("gentle-ai %s %s accepted an incompatible core: error = %v\n%s", verb, strings.Join(args, " "), err, verify.RenderReport(report))
					}
					prefix, retry := "post-sync", "gentle-ai sync"
					if verb == "install" {
						prefix, retry = "post-apply", "gentle-ai install --agent pi"
					}
					want := prefix + " verification failed:\n" + reportPrefix + "1 failed, 0 warnings, 0 skipped\n" + passed +
						fmt.Sprintf("[!!] verify:pi:engram:instance-id (Engram %q (engram 1.20.0) does not support instance-id, which the Pi companion requires. Upgrade that core before retrying)\n", bin) +
						"Installation completed with verification issues. Run `" + retry + "` to retry the failed checks.\n"
					if tc.persistFails {
						persist := "persist install state"
						if verb == "sync" {
							persist = "persist sync managed asset state"
						}
						if !strings.HasPrefix(err.Error(), persist+":") || !strings.HasSuffix(err.Error(), "\n"+want) {
							t.Fatalf("want both persistence and verification failures, got %v", err)
						}
					} else if err.Error() != want || report.RollbackRequired {
						t.Fatalf("error = %q, rollback = %t; want %q without rollback", err, report.RollbackRequired, want)
					}
				} else {
					want := reportPrefix + "0 failed, 0 warnings, 0 skipped\n" + passed + "You're ready. Start building.\n"
					if err != nil || !report.Ready || verify.RenderReport(report) != want {
						t.Fatalf("gentle-ai %s: error = %v, report = %q, want success with %q", verb, err, verify.RenderReport(report), want)
					}
				}
				if tc.persistFails {
					if _, err := os.Stat(filepath.Join(agentDir, "mcp.json")); !os.IsNotExist(err) {
						t.Fatalf("stat migrated mcp.json after persistence rollback = %v, want IsNotExist", err)
					}
				} else {
					assertFileNotContains(t, settings, "pi-mcp-adapter")
				}
				assertFileContains(t, settings, "npm:gentle-engram")
				if content, err := os.ReadFile(store); err != nil || string(content) != "existing memories" {
					t.Fatalf("Engram store = %q, %v; want original bytes", content, err)
				}
				probes, err := os.ReadFile(log)
				if tc.external {
					if !os.IsNotExist(err) {
						t.Fatalf("external server must skip the local probe: log = %q, err = %v", probes, err)
					}
					return
				}
				dataDir := strings.TrimSpace(string(probes))
				if err != nil || !filepath.IsAbs(dataDir) || dataDir == filepath.Dir(store) || strings.Contains(dataDir, "\n") {
					t.Fatalf("want exactly one probe with an isolated data dir; log = %q, err = %v", probes, err)
				}
				if _, err := os.Stat(dataDir); !os.IsNotExist(err) {
					t.Fatalf("probe data dir %q not removed: %v", dataDir, err)
				}
			})
		}
	}
}

func writePiEngramCompatibilityCore(t *testing.T, bin string, supportsInstanceID bool) {
	t.Helper()
	version := "1.20.0"
	instanceID := `echo 'Error: unknown command "instance-id" for "engram"' >&2; exit 1`
	if supportsInstanceID {
		version = "3.3.2"
		instanceID = `echo 'fixture-instance'`
	}
	script := fmt.Sprintf(`#!/bin/sh
case "$1" in
  version) echo 'engram %s';;
  instance-id)
    [ "$ENGRAM_NO_UPDATE_CHECK" = 1 ] && [ "$ENGRAM_DATA_DIR" != "$HOME/.engram" ] && [ -d "$ENGRAM_DATA_DIR" ] || exit 2
    echo "$ENGRAM_DATA_DIR" >> "$ENGRAM_FIXTURE_PROBE_LOG"
    %s;;
  *) exit 2;;
esac
`, version, instanceID)
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}
