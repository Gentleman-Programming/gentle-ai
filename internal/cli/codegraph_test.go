package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func assertSameFile(t *testing.T, got, want string) {
	t.Helper()
	gotInfo, gotErr := os.Stat(got)
	wantInfo, wantErr := os.Stat(want)
	if gotErr != nil || wantErr != nil || !os.SameFile(gotInfo, wantInfo) {
		t.Fatalf("paths %q and %q do not identify the same file: %v, %v", got, want, gotErr, wantErr)
	}
}
