package system

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestHelperProcessGetwd is not a real test: the deleted-working-directory
// tests below re-execute the test binary so this function runs as a child
// process. It mimics a Node.js child, which resolves its working directory
// during bootstrap and crashes with an ENOENT uv_cwd error when that
// directory no longer exists (issue #2148).
func TestHelperProcessGetwd(t *testing.T) {
	if os.Getenv("GENTLE_AI_WANT_GETWD_HELPER") != "1" {
		return
	}
	if _, err := os.Getwd(); err != nil {
		fmt.Fprintf(os.Stderr, "getwd failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("v1.2.3")
	os.Exit(0)
}

// helperGetwdArgv returns the argv that re-executes this test binary as the
// getwd helper child, and arms the helper via the environment.
func helperGetwdArgv(t *testing.T) []string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test binary: %v", err)
	}
	t.Setenv("GENTLE_AI_WANT_GETWD_HELPER", "1")
	return []string{exe, "-test.run=^TestHelperProcessGetwd$"}
}

// chdirIntoDeletedDir moves the test process into a directory that is then
// removed, reproducing a shell left sitting inside a deleted Git worktree or
// temporary directory (issue #2148). t.Chdir restores the original working
// directory when the test finishes.
func chdirIntoDeletedDir(t *testing.T) {
	t.Helper()
	gone := filepath.Join(t.TempDir(), "gone")
	if err := os.Mkdir(gone, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(gone)
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureCommandDir(t *testing.T) {
	t.Run("keeps a healthy inherited working directory", func(t *testing.T) {
		t.Chdir(t.TempDir())
		cmd := exec.Command(os.Args[0])
		EnsureCommandDir(cmd)
		if cmd.Dir != "" {
			t.Fatalf("cmd.Dir = %q, want empty so the child inherits the healthy working directory", cmd.Dir)
		}
	})

	t.Run("respects an explicitly set cmd.Dir", func(t *testing.T) {
		explicit := t.TempDir()
		chdirIntoDeletedDir(t)
		cmd := exec.Command(os.Args[0])
		cmd.Dir = explicit
		EnsureCommandDir(cmd)
		if cmd.Dir != explicit {
			t.Fatalf("cmd.Dir = %q, want explicit %q preserved", cmd.Dir, explicit)
		}
	})

	t.Run("falls back to the home directory when the working directory is deleted", func(t *testing.T) {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skipf("no home directory available: %v", err)
		}
		chdirIntoDeletedDir(t)
		cmd := exec.Command(os.Args[0])
		EnsureCommandDir(cmd)
		if cmd.Dir != home {
			t.Fatalf("cmd.Dir = %q, want home fallback %q", cmd.Dir, home)
		}
		info, err := os.Stat(cmd.Dir)
		if err != nil || !info.IsDir() {
			t.Fatalf("fallback %q is not an existing directory: %v", cmd.Dir, err)
		}
	})

	t.Run("tolerates a nil command", func(t *testing.T) {
		EnsureCommandDir(nil)
	})
}

func TestDetectSingleDepSurvivesDeletedWorkingDirectory(t *testing.T) {
	argv := helperGetwdArgv(t)
	chdirIntoDeletedDir(t)

	dep := detectSingleDep(context.Background(), Dependency{Name: "probe", DetectCmd: argv})

	if !dep.Installed {
		t.Fatalf("probe dependency not marked installed")
	}
	if dep.Version != "1.2.3" {
		t.Fatalf("version probe failed from deleted working directory: got version %q, want %q", dep.Version, "1.2.3")
	}
}

func TestPowerShellRunnerSurvivesDeletedWorkingDirectory(t *testing.T) {
	argv := helperGetwdArgv(t)
	chdirIntoDeletedDir(t)

	runner := NewPowerShellRunner()
	runner.LookPath = func(host string) (string, error) {
		if host == "pwsh" {
			return argv[0], nil
		}
		return "", fmt.Errorf("%s is not present in this test", host)
	}

	out, err := runner.Run(context.Background(), argv[1:]...)
	if err != nil {
		t.Fatalf("PowerShell runner failed from deleted working directory: %v (output: %s)", err, strings.TrimSpace(string(out)))
	}
}
