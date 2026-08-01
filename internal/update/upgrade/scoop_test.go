package upgrade

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
	"github.com/gentleman-programming/gentle-ai/v2/internal/update"
)

func TestScoopOwnsExecutableWith(t *testing.T) {
	root := filepath.Join(t.TempDir(), "scoop")
	current := filepath.Join(root, "apps", "gentle-ai", "current")
	version := filepath.Join(root, "apps", "gentle-ai", "2.2.0")
	active := filepath.Join("shim", "gentle-ai.exe")

	tests := []struct {
		name        string
		resolvedExe string
		resolveErr  error
		want        bool
	}{
		{
			name:        "active executable resolves into the current Scoop package",
			resolvedExe: filepath.Join(version, "gentle-ai.exe"),
			want:        true,
		},
		{
			name:        "shadowing executable outside Scoop is not owned",
			resolvedExe: filepath.Join(t.TempDir(), "gentle-ai.exe"),
			want:        false,
		},
		{
			name:       "unresolvable executable is not owned",
			resolveErr: errors.New("resolve executable"),
			want:       false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resolve := func(path string) (string, error) {
				if tc.resolveErr != nil {
					return "", tc.resolveErr
				}
				switch path {
				case active:
					return tc.resolvedExe, nil
				case current:
					return version, nil
				default:
					t.Fatalf("resolved unexpected path %q", path)
					return "", nil
				}
			}
			owned := scoopOwnsExecutableWithResolvers(active, root, resolve, resolve)
			if owned != tc.want {
				t.Errorf("scoopOwnsExecutableWithResolvers() = %t, want %t", owned, tc.want)
			}
		})
	}
}

func TestScoopCommandCancelsRunningProcess(t *testing.T) {
	originalExecCommand := execCommand
	t.Cleanup(func() { execCommand = originalExecCommand })

	readyPath := filepath.Join(t.TempDir(), "scoop-command-started")
	t.Setenv("GENTLE_AI_SCOOP_COMMAND_HELPER", "1")
	t.Setenv("GENTLE_AI_SCOOP_COMMAND_READY", readyPath)
	execCommand = func(name string, args ...string) *exec.Cmd {
		if name != "scoop" || !sameStrings(args, []string{"update", "gentle-ai"}) {
			t.Errorf("command = %q %v, want scoop update gentle-ai", name, args)
			return exec.Command(os.Args[0], "-test.run=^$")
		}
		return exec.Command(os.Args[0], "-test.run=^TestScoopCommandCancellationHelper$", "--")
	}

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := scoopCommand(ctx, "update", "gentle-ai")
		result <- err
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(readyPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Scoop command did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("scoopCommand() error = nil, want canceled command failure")
		}
	case <-time.After(time.Second):
		t.Fatal("scoopCommand() did not return after context cancellation")
	}
}

func TestScoopCommandCancellationHelper(t *testing.T) {
	if os.Getenv("GENTLE_AI_SCOOP_COMMAND_HELPER") != "1" {
		return
	}
	if err := os.WriteFile(os.Getenv("GENTLE_AI_SCOOP_COMMAND_READY"), []byte("started"), 0o600); err != nil {
		os.Exit(2)
	}
	for {
		time.Sleep(time.Second)
	}
}

func TestScoopRootWith(t *testing.T) {
	homeDir := t.TempDir()
	configuredRoot := filepath.Join(t.TempDir(), "custom-scoop")
	environmentRoot := filepath.Join(t.TempDir(), "environment-scoop")

	tests := []struct {
		name       string
		output     string
		commandErr error
		envRoot    string
		want       string
	}{
		{
			name:   "configured root path takes precedence",
			output: configuredRoot + "\n",
			want:   configuredRoot,
		},
		{
			name:    "unset root path falls back to SCOOP environment",
			output:  "'root_path' is not set\n",
			envRoot: environmentRoot,
			want:    environmentRoot,
		},
		{
			name:       "unreadable configuration falls back to the default root",
			commandErr: errors.New("scoop unavailable"),
			want:       filepath.Join(homeDir, "scoop"),
		},
		{
			name:       "unreadable configuration falls back to SCOOP environment",
			commandErr: errors.New("scoop unavailable"),
			envRoot:    environmentRoot,
			want:       environmentRoot,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := scoopRootWith(
				func(key string) string {
					if key == "SCOOP" {
						return tc.envRoot
					}
					return ""
				},
				func() (string, error) { return homeDir, nil },
				func(args ...string) ([]byte, error) {
					if !sameStrings(args, []string{"config", "root_path"}) {
						t.Fatalf("Scoop arguments = %v, want [config root_path]", args)
					}
					return []byte(tc.output), tc.commandErr
				},
			)
			if got != tc.want {
				t.Errorf("scoopRootWith() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEffectiveMethodRoutesActiveWindowsScoopOwnership(t *testing.T) {
	origScoopGentleAIOwned := scoopGentleAIOwned
	t.Cleanup(func() { scoopGentleAIOwned = origScoopGentleAIOwned })

	tests := []struct {
		name        string
		scoopOwned  bool
		goAvailable bool
		want        update.InstallMethod
	}{
		{
			name:        "active Scoop executable takes precedence over Go source install",
			scoopOwned:  true,
			goAvailable: true,
			want:        update.InstallScoop,
		},
		{
			name:        "shadowing non-Scoop executable keeps Go source install",
			scoopOwned:  false,
			goAvailable: true,
			want:        update.InstallGoInstall,
		},
		{
			name:        "non-Scoop executable without Go keeps the binary fallback",
			scoopOwned:  false,
			goAvailable: false,
			want:        update.InstallBinary,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scoopGentleAIOwned = func() bool { return tc.scoopOwned }
			profile := system.PlatformProfile{OS: "windows", PackageManager: "winget", GoAvailable: tc.goAvailable}
			if got := effectiveMethod(registryGentleAI(t), profile); got != tc.want {
				t.Errorf("effectiveMethod() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRunStrategyUpdatesActiveScoopGentleAI(t *testing.T) {
	origScoopGentleAIOwned := scoopGentleAIOwned
	t.Cleanup(func() { scoopGentleAIOwned = origScoopGentleAIOwned })
	scoopGentleAIOwned = func() bool { return true }

	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })
	var calls [][]string
	execCommand = func(name string, args ...string) *exec.Cmd {
		if name != "scoop" {
			t.Fatalf("command = %q, want scoop", name)
		}
		calls = append(calls, args)
		switch {
		case sameStrings(args, []string{"config", "IGNORE_RUNNING_PROCESSES"}):
			return mockCmd("echo", "false")
		case sameStrings(args, []string{"config", "IGNORE_RUNNING_PROCESSES", "true"}):
			return mockCmd("true")
		case sameStrings(args, []string{"update", "gentle-ai"}):
			return mockCmd("true")
		case sameStrings(args, []string{"config", "IGNORE_RUNNING_PROCESSES", "false"}):
			return mockCmd("true")
		default:
			t.Fatalf("unexpected Scoop arguments %v", args)
			return nil
		}
	}

	r := update.UpdateResult{Tool: registryGentleAI(t), LatestVersion: "2.2.0"}
	profile := system.PlatformProfile{OS: "windows", PackageManager: "winget", GoAvailable: true}
	if _, err := runStrategy(context.Background(), r, profile); err != nil {
		t.Fatalf("runStrategy() error = %v", err)
	}
	want := [][]string{
		{"config", "IGNORE_RUNNING_PROCESSES"},
		{"config", "IGNORE_RUNNING_PROCESSES", "true"},
		{"update", "gentle-ai"},
		{"config", "IGNORE_RUNNING_PROCESSES", "false"},
	}
	if len(calls) != len(want) {
		t.Fatalf("Scoop calls = %v, want %v", calls, want)
	}
	for i := range want {
		if !sameStrings(calls[i], want[i]) {
			t.Errorf("Scoop call %d = %v, want %v", i, calls[i], want[i])
		}
	}
}

func TestScoopUpgradeReturnsCommandFailure(t *testing.T) {
	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })
	var calls [][]string
	execCommand = func(name string, args ...string) *exec.Cmd {
		calls = append(calls, args)
		switch {
		case name != "scoop":
			t.Fatalf("command = %q, want scoop", name)
		case sameStrings(args, []string{"config", "IGNORE_RUNNING_PROCESSES"}):
			return mockCmd("echo", "false")
		case sameStrings(args, []string{"config", "IGNORE_RUNNING_PROCESSES", "true"}):
			return mockCmd("true")
		case sameStrings(args, []string{"update", "gentle-ai"}):
			return mockCmd("false")
		case sameStrings(args, []string{"config", "IGNORE_RUNNING_PROCESSES", "false"}):
			return mockCmd("true")
		}
		t.Fatalf("unexpected Scoop arguments %v", args)
		return nil
	}

	err := scoopUpgrade(context.Background())
	if err == nil {
		t.Fatal("scoopUpgrade() error = nil, want command failure")
	}
	if !strings.Contains(err.Error(), "scoop update gentle-ai") {
		t.Errorf("scoopUpgrade() error = %q, want command context", err)
	}
	want := [][]string{
		{"config", "IGNORE_RUNNING_PROCESSES"},
		{"config", "IGNORE_RUNNING_PROCESSES", "true"},
		{"update", "gentle-ai"},
		{"config", "IGNORE_RUNNING_PROCESSES", "false"},
	}
	if len(calls) != len(want) {
		t.Fatalf("Scoop calls = %v, want %v", calls, want)
	}
	for i := range want {
		if !sameStrings(calls[i], want[i]) {
			t.Errorf("Scoop call %d = %v, want %v", i, calls[i], want[i])
		}
	}
}

func TestScoopUpgradeRemovesTemporarySettingWhenOriginallyUnset(t *testing.T) {
	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })
	var calls [][]string
	execCommand = func(name string, args ...string) *exec.Cmd {
		if name != "scoop" {
			t.Fatalf("command = %q, want scoop", name)
		}
		calls = append(calls, args)
		switch {
		case sameStrings(args, []string{"config", "IGNORE_RUNNING_PROCESSES"}):
			return mockCmd("echo", "'IGNORE_RUNNING_PROCESSES' is not set")
		case sameStrings(args, []string{"config", "IGNORE_RUNNING_PROCESSES", "true"}),
			sameStrings(args, []string{"update", "gentle-ai"}),
			sameStrings(args, []string{"config", "rm", "IGNORE_RUNNING_PROCESSES"}):
			return mockCmd("true")
		default:
			t.Fatalf("unexpected Scoop arguments %v", args)
			return nil
		}
	}

	if err := scoopUpgrade(context.Background()); err != nil {
		t.Fatalf("scoopUpgrade() error = %v", err)
	}
	want := [][]string{
		{"config", "IGNORE_RUNNING_PROCESSES"},
		{"config", "IGNORE_RUNNING_PROCESSES", "true"},
		{"update", "gentle-ai"},
		{"config", "rm", "IGNORE_RUNNING_PROCESSES"},
	}
	if len(calls) != len(want) {
		t.Fatalf("Scoop calls = %v, want %v", calls, want)
	}
	for i := range want {
		if !sameStrings(calls[i], want[i]) {
			t.Errorf("Scoop call %d = %v, want %v", i, calls[i], want[i])
		}
	}
}

func TestRenderUpgradeReportShowsScoopCommandInDryRun(t *testing.T) {
	report := UpgradeReport{
		DryRun: true,
		Results: []ToolUpgradeResult{{
			ToolName:   "gentle-ai",
			OldVersion: "2.1.3",
			NewVersion: "2.1.4",
			Method:     update.InstallScoop,
			Status:     UpgradeSkipped,
		}},
	}

	if got := RenderUpgradeReport(report); !strings.Contains(got, "scoop update gentle-ai") {
		t.Errorf("RenderUpgradeReport() = %q, want Scoop command", got)
	}
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
