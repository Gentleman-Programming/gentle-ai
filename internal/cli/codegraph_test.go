package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCodeGraphHelpAndInvalidArgumentsDoNotInitialize(t *testing.T) {
	originalRoot := codeGraphGitTopLevel
	originalInit := codeGraphInit
	t.Cleanup(func() {
		codeGraphGitTopLevel = originalRoot
		codeGraphInit = originalInit
	})
	codeGraphGitTopLevel = func(string) (string, error) {
		t.Fatal("help or invalid arguments must not inspect a project")
		return "", nil
	}
	codeGraphInit = func(string, ...string) error {
		t.Fatal("help or invalid arguments must not initialize an index")
		return nil
	}
	for _, args := range [][]string{nil, {"init"}, {"--help"}, {"-h"}, {"init", "--help"}, {"init", "-h"}} {
		t.Run("help/"+strings.Join(args, " "), func(t *testing.T) {
			var output bytes.Buffer
			if err := RunCodeGraph(args, &output); err != nil {
				t.Fatalf("RunCodeGraph(%v) error = %v", args, err)
			}
			if !strings.Contains(output.String(), "USAGE\n  gentle-ai codegraph init --cwd <project-root>\n") {
				t.Fatalf("missing help in stdout: %q", output.String())
			}
		})
	}
	for _, args := range [][]string{
		{"query"}, {"init", "--cwd"}, {"init", "--cwd", ""},
		{"init", "--cwd", "/project", "extra"}, {"init", "--unknown"},
		{"query", "--help"}, {"init", "--unknown", "--help"},
	} {
		t.Run("invalid/"+strings.Join(args, " "), func(t *testing.T) {
			var output bytes.Buffer
			err := RunCodeGraph(args, &output)
			if err == nil || err.Error() != "usage: gentle-ai codegraph init --cwd <project-root>" {
				t.Fatalf("error = %v, want exact usage error", err)
			}
			if output.Len() != 0 {
				t.Fatalf("invalid arguments wrote stdout: %q", output.String())
			}
		})
	}
}

func TestRunCodeGraphInitValidatesCanonicalProjectAndPropagatesInitFailure(t *testing.T) {
	workspace := t.TempDir()
	root := filepath.Join(workspace, "project")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}

	originalRoot := codeGraphGitTopLevel
	originalInit := codeGraphInit
	originalHome := codeGraphUserHomeDir
	originalTemp := codeGraphTempDir
	t.Cleanup(func() {
		codeGraphGitTopLevel = originalRoot
		codeGraphInit = originalInit
		codeGraphUserHomeDir = originalHome
		codeGraphTempDir = originalTemp
	})
	codeGraphGitTopLevel = func(path string) (string, error) {
		assertSameFile(t, path, root)
		return root, nil
	}
	codeGraphUserHomeDir = func() (string, error) { return filepath.Join(workspace, "home"), nil }
	codeGraphTempDir = func() string { return filepath.Join(workspace, "temporary") }

	var output bytes.Buffer
	var called []string
	codeGraphInit = func(name string, args ...string) error {
		called = append([]string{name}, args...)
		return nil
	}
	if err := RunCodeGraph([]string{"init", "--cwd", root}, &output); err != nil {
		t.Fatalf("RunCodeGraph() error = %v", err)
	}
	if len(called) != 3 || called[0] != "codegraph" || called[1] != "init" {
		t.Fatalf("command = %v, want codegraph init <root>", called)
	}
	assertSameFile(t, called[2], root)
	if !strings.Contains(output.String(), called[2]) {
		t.Fatalf("output = %q, want canonical root", output.String())
	}

	codeGraphInit = func(string, ...string) error { return errors.New("init failed") }
	if err := RunCodeGraph([]string{"init", "--cwd", root}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "init failed") {
		t.Fatalf("subprocess error = %v, want propagated init failure", err)
	}
}

func TestRunCodeGraphInitRejectsUnsafeOrUnrecognizedRoots(t *testing.T) {
	workspace := t.TempDir()
	home := filepath.Join(workspace, "home")
	temp := filepath.Join(workspace, "temporary")
	for _, path := range []string{home, temp} {
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	outside := filepath.Join(temp, "outside")
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(workspace, "escape")
	if err := os.Symlink(outside, symlink); err != nil {
		t.Fatal(err)
	}

	originalRoot := codeGraphGitTopLevel
	originalInit := codeGraphInit
	originalHome := codeGraphUserHomeDir
	originalTemp := codeGraphTempDir
	t.Cleanup(func() {
		codeGraphGitTopLevel = originalRoot
		codeGraphInit = originalInit
		codeGraphUserHomeDir = originalHome
		codeGraphTempDir = originalTemp
	})
	codeGraphGitTopLevel = func(path string) (string, error) {
		if path == filepath.Join(workspace, "not-a-project") {
			return "", errors.New("not a git repository")
		}
		return path, nil
	}
	codeGraphUserHomeDir = func() (string, error) { return home, nil }
	codeGraphTempDir = func() string { return temp }
	codeGraphInit = func(string, ...string) error { t.Fatal("codegraph init must not run for rejected roots"); return nil }

	volumeRoot := filepath.VolumeName(workspace) + string(filepath.Separator)
	for _, path := range []string{"", volumeRoot, home, temp, outside, symlink, filepath.Join(workspace, "not-a-project")} {
		t.Run(filepath.Base(path), func(t *testing.T) {
			if err := RunCodeGraph([]string{"init", "--cwd", path}, &bytes.Buffer{}); err == nil {
				t.Fatalf("RunCodeGraph(%q) error = nil, want rejection", path)
			}
		})
	}
}

func TestRunCodeGraphInitAcceptsProjectBelowHome(t *testing.T) {
	workspace := t.TempDir()
	home := filepath.Join(workspace, "home")
	root := filepath.Join(home, "work", "project-feature")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}

	originalRoot := codeGraphGitTopLevel
	originalInit := codeGraphInit
	originalHome := codeGraphUserHomeDir
	originalTemp := codeGraphTempDir
	t.Cleanup(func() {
		codeGraphGitTopLevel = originalRoot
		codeGraphInit = originalInit
		codeGraphUserHomeDir = originalHome
		codeGraphTempDir = originalTemp
	})
	codeGraphGitTopLevel = func(path string) (string, error) { return path, nil }
	codeGraphUserHomeDir = func() (string, error) { return home, nil }
	codeGraphTempDir = func() string { return filepath.Join(workspace, "temporary") }

	var calledRoot string
	codeGraphInit = func(name string, args ...string) error {
		if name == "codegraph" && len(args) == 2 && args[0] == "init" {
			calledRoot = args[1]
		}
		return nil
	}
	if err := RunCodeGraph([]string{"init", "--cwd", root}, &bytes.Buffer{}); err != nil {
		t.Fatalf("RunCodeGraph() error = %v", err)
	}
	if calledRoot == "" {
		t.Fatal("codegraph init was not called for a project below HOME")
	}
	assertSameFile(t, calledRoot, root)
}

func TestRunCodeGraphInitDoesNotCrossNestedWorkspaceBoundary(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Git")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git unavailable")
	}
	workspace := t.TempDir()
	parent := filepath.Join(workspace, "parent")
	nested := filepath.Join(parent, "src", "nested")
	marker := filepath.Join(parent, ".codegraph", "sentinel")
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, []byte("parent index unchanged"), 0o644); err != nil {
		t.Fatal(err)
	}
	initGit := func(root string) {
		t.Helper()
		if output, err := exec.Command("git", "-C", root, "init", "--quiet").CombinedOutput(); err != nil {
			t.Fatalf("git init: %v: %s", err, output)
		}
	}
	initGit(parent)

	originalInit, originalHome, originalTemp := codeGraphInit, codeGraphUserHomeDir, codeGraphTempDir
	t.Cleanup(func() {
		codeGraphInit, codeGraphUserHomeDir, codeGraphTempDir = originalInit, originalHome, originalTemp
	})
	codeGraphUserHomeDir = func() (string, error) { return filepath.Join(workspace, "home"), nil }
	codeGraphTempDir = func() string { return filepath.Join(workspace, "temporary") }
	calls := 0
	var initializedRoot string
	codeGraphInit = func(name string, args ...string) error {
		calls++
		if name != "codegraph" || len(args) != 2 || args[0] != "init" {
			t.Fatalf("unexpected command: %s %v", name, args)
		}
		initializedRoot = args[1]
		assertSameFile(t, initializedRoot, nested)
		return nil
	}

	var output bytes.Buffer
	err := RunCodeGraph([]string{"init", "--cwd", nested}, &output)
	if want := fmt.Sprintf("unsafe CodeGraph root %q", nested); err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %s", err, want)
	}
	if calls != 0 || output.Len() != 0 {
		t.Fatalf("rejected target: calls=%d stdout=%q", calls, output.String())
	}
	for _, name := range []string{".git", ".codegraph"} {
		if _, err := os.Stat(filepath.Join(nested, name)); !os.IsNotExist(err) {
			t.Fatalf("rejected target changed %s: %v", name, err)
		}
	}

	initGit(nested)
	if err := RunCodeGraph([]string{"init", "--cwd", nested}, &output); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || output.String() != "CodeGraph initialized: "+initializedRoot+"\n" {
		t.Fatalf("initialized target: calls=%d stdout=%q", calls, output.String())
	}
	if content, err := os.ReadFile(marker); err != nil || string(content) != "parent index unchanged" {
		t.Fatalf("parent index changed: %q, %v", content, err)
	}
}

func assertSameFile(t *testing.T, got, want string) {
	t.Helper()
	gotInfo, gotErr := os.Stat(got)
	wantInfo, wantErr := os.Stat(want)
	if gotErr != nil || wantErr != nil || !os.SameFile(gotInfo, wantInfo) {
		t.Fatalf("paths %q and %q do not identify the same file: %v, %v", got, want, gotErr, wantErr)
	}
}
