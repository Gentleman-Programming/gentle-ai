package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	fmt.Println("child working directory is valid")
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
	if runtime.GOOS == "windows" {
		t.Skip("Windows keeps a process current directory open, so the deleted-CWD integration setup is impossible")
	}
	gone := filepath.Join(t.TempDir(), "gone")
	if err := os.Mkdir(gone, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(gone)
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}
}

func TestExecuteCommandSurvivesDeletedWorkingDirectory(t *testing.T) {
	argv := helperGetwdArgv(t)
	chdirIntoDeletedDir(t)

	if err := executeCommand(argv[0], argv[1:]...); err != nil {
		t.Fatalf("executeCommand from deleted working directory: %v", err)
	}
}

func TestCodeGraphInitSurvivesDeletedWorkingDirectory(t *testing.T) {
	argv := helperGetwdArgv(t)
	chdirIntoDeletedDir(t)

	if err := codeGraphInit(argv[0], argv[1:]...); err != nil {
		t.Fatalf("codeGraphInit from deleted working directory: %v", err)
	}
}

func TestCodeGraphHomeRunnerSurvivesDeletedWorkingDirectory(t *testing.T) {
	argv := helperGetwdArgv(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home directory available: %v", err)
	}
	chdirIntoDeletedDir(t)

	runner := codeGraphHomeRunner{homeDir: home}
	if err := runner.Run(argv[0], argv[1:]...); err != nil {
		t.Fatalf("codeGraphHomeRunner from deleted working directory: %v", err)
	}
}

func TestDefaultGoEnvSurvivesDeletedWorkingDirectory(t *testing.T) {
	if testing.Short() {
		t.Skip("requires the real go toolchain")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("go toolchain not on PATH: %v", err)
	}
	chdirIntoDeletedDir(t)

	if _, err := defaultGoEnv("GOBIN"); err != nil {
		t.Fatalf("defaultGoEnv from deleted working directory: %v", err)
	}
}
