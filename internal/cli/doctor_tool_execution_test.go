package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The isolated subprocess is this test binary, before Go parses --version.
// This avoids dependencies on a shell sleep command or installed real tools.
func init() {
	if os.Getenv("GENTLE_DOCTOR_HANG_HELPER") == "1" {
		if path := os.Getenv("GENTLE_DOCTOR_PID_FILE"); path != "" {
			_ = os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0o600)
		}
		time.Sleep(time.Minute)
		os.Exit(0)
	}
}

// Exercise the public report with real PATH entries, not mocked lookups.
func TestRunDoctorToolExecution(t *testing.T) {
	if testing.Short() {
		t.Skip("runs executable launchers")
	}
	cases := []struct {
		name, tool, arg                             string
		broken, hangs, launcher, powershell, cancel bool
	}{
		{name: "missing launcher target", tool: "gga", arg: "--version", broken: true},
		{name: "valid launcher", tool: "gga", arg: "--version"},
		{name: "valid engram version subcommand", tool: "engram", arg: "version"},
		{name: "valid installed agent", tool: "claude", arg: "--version"},
		{name: "hanging executable", tool: "gga", hangs: true},
		{name: "hanging launcher child", tool: "gga", hangs: true, launcher: true},
		{name: "canceled launcher child", tool: "gga", hangs: true, launcher: true, cancel: true},
		{name: "valid PowerShell launcher", tool: "gga", arg: "--version", powershell: true},
		{name: "hanging PowerShell child", tool: "gga", hangs: true, launcher: true, powershell: true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if tt.powershell && runtime.GOOS != "windows" {
				t.Skip("Windows PowerShell shim")
			}
			home := t.TempDir()
			bin := filepath.Join(t.TempDir(), "bin with spaces & symbols")
			if err := os.Mkdir(bin, 0o755); err != nil {
				t.Fatal(err)
			}
			filename := tt.tool
			script := "#!/bin/sh\n[ \"$#\" = 1 ] && [ \"$1\" = \"" + tt.arg + "\" ]\n"
			if tt.broken {
				script = "#!/bin/sh\nexec \"" + filepath.ToSlash(filepath.Join(bin, "missing-target")) + "\" \"$@\"\n"
			}
			if runtime.GOOS == "windows" {
				filename += ".cmd"
				script = "@echo off\r\nif not \"%~1\"==\"" + tt.arg + "\" exit /b 2\r\nif not \"%~2\"==\"\" exit /b 3\r\nexit /b 0\r\n"
				if tt.broken {
					script = "@echo off\r\n\"" + filepath.Join(bin, "missing-target.exe") + "\" %*\r\nexit /b %errorlevel%\r\n"
				}
			}
			if tt.powershell {
				filename = tt.tool + ".ps1"
				script = "if ($args.Count -ne 1 -or $args[0] -ne '--version') { exit 2 }; exit 0\n"
			}
			pidFile := filepath.Join(t.TempDir(), "child.pid")
			if tt.hangs {
				filename = tt.tool
				if runtime.GOOS == "windows" {
					filename += ".exe"
				}
				executable, err := os.Executable()
				if err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(executable)
				if err != nil {
					t.Fatal(err)
				}
				script = string(data)
				t.Setenv("GENTLE_DOCTOR_HANG_HELPER", "1")
				t.Setenv("GENTLE_DOCTOR_PID_FILE", pidFile)
				if tt.launcher {
					target := filepath.Join(bin, "hanging-child")
					if runtime.GOOS == "windows" {
						target += ".exe"
					}
					if err := os.WriteFile(target, data, 0o755); err != nil {
						t.Fatal(err)
					}
					filename = tt.tool
					script = fmt.Sprintf("#!/bin/sh\n\"%s\" \"$@\"\n", target)
					if runtime.GOOS == "windows" {
						filename += ".cmd"
						script = fmt.Sprintf("@echo off\r\n\"%s\" %%*\r\n", target)
						if tt.powershell {
							filename = tt.tool + ".ps1"
							script = fmt.Sprintf("& '%s' @args; exit $LASTEXITCODE\n", strings.ReplaceAll(target, "'", "''"))
						}
					}
				}
				t.Cleanup(func() {
					data, _ := os.ReadFile(pidFile)
					pid, _ := strconv.Atoi(string(data))
					if pid > 0 {
						process, _ := os.FindProcess(pid)
						if process != nil {
							_ = process.Kill()
						}
					}
				})
			}
			if err := os.WriteFile(filepath.Join(bin, filename), []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			if tt.tool == "claude" {
				if err := os.Mkdir(filepath.Join(home, ".gentle-ai"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(home, ".gentle-ai", "state.json"), []byte(`{"installed_agents":["claude-code"]}`), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.ReadDir(home)
			if err != nil {
				t.Fatal(err)
			}
			probePath := bin
			if tt.powershell {
				probePath += string(os.PathListSeparator) + filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0")
			}
			t.Setenv("PATH", probePath)
			t.Setenv(engramHealthEnvVar, "http://doctor-test.invalid")
			origHome, origHTTP, origDisk := osUserHomeDirDoctor, httpGetFn, availableBytesFn
			t.Cleanup(func() { osUserHomeDirDoctor, httpGetFn, availableBytesFn = origHome, origHTTP, origDisk })
			osUserHomeDirDoctor = func() (string, error) { return home, nil }
			httpGetFn = func(string, time.Duration) (int, error) { return 200, nil }
			availableBytesFn = func(string) (int64, error) { return diskWarnThreshold + 1, nil }
			var out bytes.Buffer
			started := time.Now()
			ctx := context.Background()
			if tt.cancel {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, time.Second)
				defer cancel()
			}
			if err := RunDoctor(ctx, &out); err != nil {
				t.Fatal(err)
			}
			elapsed := time.Since(started)
			var line string
			for _, candidate := range strings.Split(out.String(), "\n") {
				if strings.Contains(candidate, "tool:"+tt.tool+" ") {
					line = candidate
					break
				}
			}
			if line == "" {
				t.Fatalf("missing tool check: %s", out.String())
			}
			if tt.broken || tt.hangs {
				if strings.Contains(line, "[ok]") || !strings.Contains(line, "[!!]") || !strings.Contains(line, "version probe failed") {
					t.Fatalf("unusable launcher must warn, got %s", line)
				}
				if !strings.Contains(out.String(), "Repair or reinstall "+tt.tool) {
					t.Fatalf("missing repair hint: %s", out.String())
				}
			} else if !strings.Contains(line, "[ok]") {
				t.Fatalf("valid launcher must pass, got %s", line)
			}
			if tt.hangs && !tt.cancel && (elapsed > doctorToolProbeTimeout+3*time.Second || !strings.Contains(line, "did not complete within 5s")) {
				t.Fatalf("unbounded or unexplained timeout (%s): %s", elapsed, line)
			}
			if tt.cancel && (elapsed > 3*time.Second || !strings.Contains(line, "context deadline exceeded")) {
				t.Fatalf("parent cancellation not honored (%s): %s", elapsed, line)
			}
			if tt.hangs {
				data, err := os.ReadFile(pidFile)
				if err != nil {
					t.Fatal(err)
				}
				pid, err := strconv.Atoi(string(data))
				if err != nil {
					t.Fatal(err)
				}
				assertDoctorProbeProcessExited(t, pid)
			}
			after, err := os.ReadDir(home)
			if err != nil || len(after) != len(before) {
				t.Fatalf("doctor mutated isolated home: %v, %v", after, err)
			}
			if tt.tool == "claude" {
				data, err := os.ReadFile(filepath.Join(home, ".gentle-ai", "state.json"))
				if err != nil || string(data) != `{"installed_agents":["claude-code"]}` {
					t.Fatalf("doctor mutated state: %s, %v", data, err)
				}
			}
		})
	}
}
