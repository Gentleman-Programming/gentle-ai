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
			if !strings.Contains(output.String(), "adds .codegraph/ to the worktree's local Git exclude file") {
				t.Fatalf("help does not describe the local exclusion: %q", output.String())
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

// C7: CodeGraph writes a self-negating `.codegraph/.gitignore` (`*` plus
// `!.gitignore`), so the index directory stays untracked and enters agents'
// diffs unless init excludes it in the worktree's local exclude file.
func TestRunCodeGraphInitExcludesIndexLocally(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Git")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git unavailable")
	}

	t.Run("linked worktree", func(t *testing.T) {
		workspace := t.TempDir()
		mainRoot := filepath.Join(workspace, "main")
		linked := filepath.Join(workspace, "linked")
		commitCodeGraphFixture(t, mainRoot)
		runCodeGraphTestGit(t, mainRoot, "worktree", "add", "--quiet", "-b", "feature", linked)
		stubCodeGraphIndexWriter(t, workspace)
		exclude := codeGraphTestExcludePath(t, linked)
		before := readCodeGraphTestFile(t, exclude)

		for run := 1; run <= 2; run++ {
			var output bytes.Buffer
			if err := RunCodeGraph([]string{"init", "--cwd", linked}, &output); err != nil {
				t.Fatalf("run %d: RunCodeGraph() error = %v", run, err)
			}
			if want := "CodeGraph initialized: " + canonicalCodeGraphTestPath(t, linked) + "\n"; output.String() != want {
				t.Fatalf("run %d: stdout = %q, want %q", run, output.String(), want)
			}
		}

		if got, want := readCodeGraphTestFile(t, exclude), withCodeGraphExcludeLine(before); got != want {
			t.Fatalf("exclude file = %q, want one appended .codegraph/ line: %q", got, want)
		}
		assertCodeGraphTestStatusClean(t, linked)
		assertCodeGraphTestStatusClean(t, mainRoot)
		for _, root := range []string{mainRoot, linked} {
			if _, err := os.Stat(filepath.Join(root, ".gitignore")); !os.IsNotExist(err) {
				t.Fatalf("init created or touched %s/.gitignore: %v", root, err)
			}
		}
	})

	t.Run("main checkout keeps existing exclude entries", func(t *testing.T) {
		workspace := t.TempDir()
		root := filepath.Join(workspace, "project")
		commitCodeGraphFixture(t, root)
		stubCodeGraphIndexWriter(t, workspace)
		exclude := codeGraphTestExcludePath(t, root)
		if err := os.WriteFile(exclude, []byte("# local\nbuild/"), 0o644); err != nil {
			t.Fatal(err)
		}

		for run := 1; run <= 2; run++ {
			if err := RunCodeGraph([]string{"init", "--cwd", root}, &bytes.Buffer{}); err != nil {
				t.Fatalf("run %d: RunCodeGraph() error = %v", run, err)
			}
		}

		if got, want := readCodeGraphTestFile(t, exclude), "# local\nbuild/\n.codegraph/\n"; got != want {
			t.Fatalf("exclude file = %q, want %q", got, want)
		}
		assertCodeGraphTestStatusClean(t, root)
	})

	t.Run("existing entry is left untouched", func(t *testing.T) {
		workspace := t.TempDir()
		root := filepath.Join(workspace, "project")
		commitCodeGraphFixture(t, root)
		stubCodeGraphIndexWriter(t, workspace)
		exclude := codeGraphTestExcludePath(t, root)
		if err := os.WriteFile(exclude, []byte("/.codegraph\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		if err := RunCodeGraph([]string{"init", "--cwd", root}, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}

		if got := readCodeGraphTestFile(t, exclude); got != "/.codegraph\n" {
			t.Fatalf("exclude file = %q, want it unchanged", got)
		}
		assertCodeGraphTestStatusClean(t, root)
	})
}

func TestRunCodeGraphInitReportsAndContinuesWhenIndexCannotBeExcluded(t *testing.T) {
	for _, tc := range []struct {
		name     string
		noGitBin bool
	}{
		{name: "not a git work tree"},
		{name: "git unavailable", noGitBin: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.noGitBin {
				if _, err := exec.LookPath("git"); err != nil {
					t.Skip("Git unavailable")
				}
			}
			workspace := t.TempDir()
			root := filepath.Join(workspace, "project")
			if err := os.Mkdir(root, 0o755); err != nil {
				t.Fatal(err)
			}
			stubCodeGraphIndexWriter(t, workspace)
			originalRoot, originalWarnings := codeGraphGitTopLevel, codeGraphWarnings
			t.Cleanup(func() { codeGraphGitTopLevel, codeGraphWarnings = originalRoot, originalWarnings })
			codeGraphGitTopLevel = func(path string) (string, error) { return path, nil }
			var warnings bytes.Buffer
			codeGraphWarnings = &warnings
			if tc.noGitBin {
				t.Setenv("PATH", "")
			}

			var output bytes.Buffer
			if err := RunCodeGraph([]string{"init", "--cwd", root}, &output); err != nil {
				t.Fatalf("RunCodeGraph() error = %v, want init to continue", err)
			}
			if want := "CodeGraph initialized: " + canonicalCodeGraphTestPath(t, root) + "\n"; output.String() != want {
				t.Fatalf("stdout = %q, want %q", output.String(), want)
			}
			if !strings.HasPrefix(warnings.String(), "WARNING: could not exclude .codegraph/ from git locally: ") {
				t.Fatalf("warnings = %q, want the exclusion failure reported", warnings.String())
			}
			if _, err := os.Stat(filepath.Join(root, ".git")); !os.IsNotExist(err) {
				t.Fatalf("init created .git in a non-repository: %v", err)
			}
		})
	}
}

// stubCodeGraphIndexWriter replaces the CodeGraph binary with one that writes
// the same self-negating .gitignore the real index carries.
func stubCodeGraphIndexWriter(t *testing.T, workspace string) {
	t.Helper()
	originalInit, originalHome, originalTemp := codeGraphInit, codeGraphUserHomeDir, codeGraphTempDir
	t.Cleanup(func() {
		codeGraphInit, codeGraphUserHomeDir, codeGraphTempDir = originalInit, originalHome, originalTemp
	})
	codeGraphUserHomeDir = func() (string, error) { return filepath.Join(workspace, "home"), nil }
	codeGraphTempDir = func() string { return filepath.Join(workspace, "temporary") }
	codeGraphInit = func(name string, args ...string) error {
		index := filepath.Join(args[len(args)-1], ".codegraph")
		if err := os.MkdirAll(index, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(index, ".gitignore"), []byte("*\n!.gitignore\n"), 0o644); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(index, "codegraph.db"), []byte("index"), 0o644)
	}
}

func commitCodeGraphFixture(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	runCodeGraphTestGit(t, root, "init", "--quiet")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCodeGraphTestGit(t, root, "add", "README.md")
	runCodeGraphTestGit(t, root, "-c", "user.name=Test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "fixture")
}

func runCodeGraphTestGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	output, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
	return string(output)
}

func codeGraphTestExcludePath(t *testing.T, root string) string {
	t.Helper()
	path := strings.TrimSpace(runCodeGraphTestGit(t, root, "rev-parse", "--git-path", "info/exclude"))
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func readCodeGraphTestFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(content)
}

func withCodeGraphExcludeLine(content string) string {
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return content + ".codegraph/\n"
}

func assertCodeGraphTestStatusClean(t *testing.T, root string) {
	t.Helper()
	if status := runCodeGraphTestGit(t, root, "status", "--short", "--untracked-files=all"); status != "" {
		t.Fatalf("git status in %s = %q, want clean", root, status)
	}
}

func canonicalCodeGraphTestPath(t *testing.T, path string) string {
	t.Helper()
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}

func assertSameFile(t *testing.T, got, want string) {
	t.Helper()
	gotInfo, gotErr := os.Stat(got)
	wantInfo, wantErr := os.Stat(want)
	if gotErr != nil || wantErr != nil || !os.SameFile(gotInfo, wantInfo) {
		t.Fatalf("paths %q and %q do not identify the same file: %v, %v", got, want, gotErr, wantErr)
	}
}
