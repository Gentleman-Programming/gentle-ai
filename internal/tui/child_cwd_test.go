package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// TestHelperProcessGetwd is not a real test: the deleted-working-directory
// test below re-executes the test binary so this function runs as a child
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

func TestExecuteExternalCommandSurvivesDeletedWorkingDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows keeps a process current directory open, so the deleted-CWD integration setup is impossible")
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test binary: %v", err)
	}
	t.Setenv("GENTLE_AI_WANT_GETWD_HELPER", "1")

	gone := filepath.Join(t.TempDir(), "gone")
	if err := os.Mkdir(gone, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(gone)
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}

	if err := executeExternalCommand(exec.Command, exe, "-test.run=^TestHelperProcessGetwd$"); err != nil {
		t.Fatalf("external command runner from deleted working directory: %v", err)
	}
}
