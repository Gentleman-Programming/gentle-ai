package reviewtransaction

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreparedCandidateInspectorUsesRegularEmptyAttributesFile(t *testing.T) {
	requireSnapshotGit(t)
	repo, snapshot := preparedCandidateSnapshot(t)

	original := gitCommandContext
	attributesFile := ""
	gitCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		for _, arg := range args {
			if value, ok := strings.CutPrefix(arg, "core.attributesFile="); ok {
				attributesFile = value
			}
		}
		proxyArgs := append([]string{"-test.run=^TestFrozenCandidateWindowsGitProxy$", "--", "gentle-ai-git-proxy", name}, args...)
		return exec.CommandContext(ctx, os.Args[0], proxyArgs...)
	}
	t.Cleanup(func() { gitCommandContext = original })

	inspector, err := (SnapshotBuilder{Repo: repo}).PrepareCandidateInspector(t.Context(), snapshot)
	if err != nil {
		t.Fatalf("PrepareCandidateInspector() error = %v", err)
	}
	defer func() {
		if err := inspector.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	}()

	output, err := inspector.Inspect(t.Context(), "patch", 0, "")
	if err != nil {
		t.Fatalf("Inspect() through native Git for Windows error = %v", err)
	}
	if len(output) == 0 {
		t.Fatal("Inspect() through native Git for Windows returned an empty patch")
	}
	if attributesFile == "" || strings.EqualFold(attributesFile, os.DevNull) {
		t.Fatalf("Git consumed core.attributesFile=%q, want empty regular file", attributesFile)
	}
	info, err := os.Stat(attributesFile)
	if err != nil {
		t.Fatalf("stat consumed attributes file: %v", err)
	}
	if !info.Mode().IsRegular() || info.Size() != 0 {
		t.Fatalf("consumed attributes file mode = %v, size = %d, want empty regular file", info.Mode(), info.Size())
	}
}

func TestFrozenCandidateWindowsGitProxy(t *testing.T) {
	marker := -1
	for index, arg := range os.Args {
		if arg == "gentle-ai-git-proxy" {
			marker = index
			break
		}
	}
	if marker < 0 || marker+1 >= len(os.Args) {
		return
	}

	args := os.Args[marker+1:]
	command := exec.Command(args[0], args[1:]...)
	command.Env = windowsGitProxyEnvironment(os.Environ())
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			os.Exit(exitError.ExitCode())
		}
		os.Exit(1)
	}
	os.Exit(0)
}

func windowsGitProxyEnvironment(environment []string) []string {
	gitDir := ""
	for _, entry := range environment {
		name, value, _ := strings.Cut(entry, "=")
		if strings.EqualFold(name, "GIT_DIR") {
			gitDir = value
			break
		}
	}
	if gitDir == "" {
		return environment
	}

	emptyConfig := filepath.Join(gitDir, "test-empty-config")
	if err := os.WriteFile(emptyConfig, nil, 0o600); err != nil {
		return environment
	}
	result := append([]string(nil), environment...)
	for index, entry := range result {
		name, value, found := strings.Cut(entry, "=")
		if found && (strings.EqualFold(name, "GIT_CONFIG_GLOBAL") || strings.EqualFold(name, "GIT_CONFIG_SYSTEM")) && strings.EqualFold(value, os.DevNull) {
			result[index] = name + "=" + emptyConfig
		}
	}
	return result
}
