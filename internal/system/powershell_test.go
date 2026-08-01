package system

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPowerShellRunnerFakePATH(t *testing.T) {
	tests := []struct {
		name, want  string
		hosts       []string
		unavailable bool
	}{
		{name: "both prefers pwsh", hosts: []string{"pwsh", "powershell.exe"}, want: "pwsh"},
		{name: "legacy only", hosts: []string{"powershell.exe"}, want: "powershell"},
		{name: "neither", unavailable: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := fakePowerShellPATH(t, tt.hosts...)
			runner := NewPowerShellRunner()
			runner.RunCommand = func(_ context.Context, name string, _ ...string) ([]byte, error) {
				return []byte(strings.TrimSuffix(filepath.Base(name), ".exe")), nil
			}
			out, err := runner.Run(context.Background(), "-Command", "exit 0")
			if tt.unavailable {
				var unavailable *PowerShellUnavailableError
				if !errors.As(err, &unavailable) || !strings.Contains(err.Error(), "pwsh") || !strings.Contains(err.Error(), "powershell") {
					t.Fatalf("Run() error = %v, want typed actionable diagnostics", err)
				}
				return
			}
			if err != nil || string(out) != tt.want {
				t.Fatalf("Run() = %q, %v; want host %q from PATH %q", out, err, tt.want, dir)
			}
		})
	}
}

func TestPowerShellRunnerLaunchFailureFallbackPreservesErrors(t *testing.T) {
	fakePowerShellPATH(t, "pwsh", "powershell.exe")
	runner := NewPowerShellRunner()
	legacyFails := false
	runner.RunCommand = func(_ context.Context, name string, _ ...string) ([]byte, error) {
		if strings.HasPrefix(filepath.Base(name), "pwsh") {
			return nil, &exec.Error{Name: name, Err: errors.New("launch failed")}
		}
		if legacyFails {
			return []byte("legacy failed"), errors.New("legacy execution failed")
		}
		return []byte("powershell"), nil
	}
	out, err := runner.Run(context.Background(), "-Command", "exit 0")
	if err != nil || string(out) != "powershell" {
		t.Fatalf("Run() fallback = %q, %v", out, err)
	}
	legacyFails = true
	_, err = runner.Run(context.Background(), "-Command", "exit 0")
	if err == nil || !strings.Contains(err.Error(), "launch failed") || !strings.Contains(err.Error(), "legacy failed") {
		t.Fatalf("Run() error = %v, want both launch diagnostics", err)
	}
}

func fakePowerShellPATH(t *testing.T, hosts ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, host := range hosts {
		if runtime.GOOS == "windows" && filepath.Ext(host) == "" {
			host += ".exe"
		}
		if err := os.WriteFile(filepath.Join(dir, host), nil, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	return dir
}
