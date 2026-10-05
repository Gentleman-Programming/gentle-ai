package reviewerprovider

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

const codexAdapterHelperEnvironment = "GENTLE_AI_REVIEWER_PROVIDER_CODEX_HELPER"
const codexAdapterPromptPathEnvironment = "GENTLE_AI_REVIEWER_PROVIDER_CODEX_PROMPT_PATH"
const codexAdapterArgumentsPathEnvironment = "GENTLE_AI_REVIEWER_PROVIDER_CODEX_ARGUMENTS_PATH"
const codexAdapterWorkdirPathEnvironment = "GENTLE_AI_REVIEWER_PROVIDER_CODEX_WORKDIR_PATH"
const codexAdapterFailEnvironment = "GENTLE_AI_REVIEWER_PROVIDER_CODEX_FAIL"

func TestCodexAdapterReturnsNoBytesWhenUnavailable(t *testing.T) {
	adapter := &CodexAdapter{LookPath: func(string) (string, error) { return "", errors.New("not found") }}
	raw, err := adapter.Review(context.Background(), NewInvocation([]byte("provider prompt")))
	if err == nil || !strings.Contains(err.Error(), "codex reviewer transport unavailable") {
		t.Fatalf("Review() error = %v, want unavailable transport error", err)
	}
	if raw != nil {
		t.Fatalf("Review() raw = %q with transport error, want no result bytes", raw)
	}
}

func TestCodexAdapterUsesStdinAndReturnsUntouchedRawOutput(t *testing.T) {
	promptPath := filepath.Join(t.TempDir(), "prompt")
	argumentsPath := filepath.Join(t.TempDir(), "arguments")
	t.Setenv(codexAdapterHelperEnvironment, "1")
	t.Setenv(codexAdapterPromptPathEnvironment, promptPath)
	t.Setenv(codexAdapterArgumentsPathEnvironment, argumentsPath)
	t.Setenv(codexReviewerLoopbackBaseURLEnvironment, "")

	var commandArguments []string
	var commandEnvironment []string
	adapter := &CodexAdapter{
		LookPath: func(string) (string, error) { return "codex", nil },
		commandContext: func(ctx context.Context, _ string, arguments ...string) *exec.Cmd {
			commandArguments = append([]string(nil), arguments...)
			command := exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestCodexAdapterHelperProcess$", "--"}, arguments...)...)
			commandEnvironment = command.Env
			return command
		},
	}
	prompt := []byte("provider prompt\nwith bytes")
	raw, err := adapter.Review(context.Background(), NewInvocation(prompt))
	if err != nil {
		t.Fatal(err)
	}
	if want := []byte("raw\x00reviewer\xffoutput"); !bytes.Equal(raw, want) {
		t.Fatalf("Review() = %q, want untouched raw bytes %q", raw, want)
	}
	if got, err := os.ReadFile(promptPath); err != nil || !bytes.Equal(got, prompt) {
		t.Fatalf("reviewer stdin = %q, %v; want %q", got, err, prompt)
	}
	arguments, err := os.ReadFile(argumentsPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(arguments), string(prompt)) {
		t.Fatalf("codex arguments carried provider prompt: %q", arguments)
	}
	wantArguments := []string{
		"exec", "--skip-git-repo-check", "--ignore-user-config", "--sandbox", "read-only", "-C", commandArguments[6],
		"--output-last-message", commandArguments[8],
	}
	if !slices.Equal(commandArguments, wantArguments) {
		t.Fatalf("default codex arguments = %q, want %q", commandArguments, wantArguments)
	}
	if commandEnvironment != nil {
		t.Fatalf("default Codex environment = %q, want inherited environment", commandEnvironment)
	}
}

func TestCodexAdapterModelArgumentPreservesIsolation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("helper process uses POSIX argument handling")
	}
	t.Setenv(codexAdapterHelperEnvironment, "1")
	t.Setenv(codexAdapterPromptPathEnvironment, filepath.Join(t.TempDir(), "prompt"))
	t.Setenv(codexAdapterArgumentsPathEnvironment, filepath.Join(t.TempDir(), "arguments"))
	t.Setenv(codexReviewerLoopbackBaseURLEnvironment, "")
	for _, tc := range []struct {
		model    string
		assigned bool
	}{
		{"gpt-6.1-sol", true}, {"", false}, {"--unsafe", false}, {"bad model", false},
	} {
		var args []string
		adapter := &CodexAdapter{Model: tc.model, LookPath: func(string) (string, error) { return "codex", nil },
			commandContext: func(ctx context.Context, _ string, arguments ...string) *exec.Cmd {
				args = append([]string(nil), arguments...)
				return exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestCodexAdapterHelperProcess$", "--"}, arguments...)...)
			},
		}
		if _, err := adapter.Review(context.Background(), NewInvocation([]byte("prompt"))); err != nil {
			t.Fatal(err)
		}
		want := []string{"exec", "--skip-git-repo-check", "--ignore-user-config", "--sandbox", "read-only", "-C", args[6], "--output-last-message", args[8]}
		if tc.assigned {
			want = append(want, "--model", tc.model)
		}
		if !slices.Equal(args, want) {
			t.Errorf("model %q argv = %q, want %q", tc.model, args, want)
		}
	}
}

func TestCodexAdapterConfiguresApprovedLoopbackProvider(t *testing.T) {
	t.Setenv(codexAdapterHelperEnvironment, "1")
	t.Setenv(codexAdapterPromptPathEnvironment, filepath.Join(t.TempDir(), "prompt"))
	t.Setenv(codexAdapterArgumentsPathEnvironment, filepath.Join(t.TempDir(), "arguments"))
	const baseURL = "http://127.0.0.1:43123/v1"
	t.Setenv(codexReviewerLoopbackBaseURLEnvironment, baseURL)

	var commandArguments []string
	adapter := &CodexAdapter{
		LookPath: func(string) (string, error) { return "codex", nil },
		commandContext: func(ctx context.Context, _ string, arguments ...string) *exec.Cmd {
			commandArguments = append([]string(nil), arguments...)
			return exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestCodexAdapterHelperProcess$", "--"}, arguments...)...)
		},
	}
	if _, err := adapter.Review(context.Background(), NewInvocation([]byte("provider prompt"))); err != nil {
		t.Fatal(err)
	}

	wantArguments := []string{
		"exec", "--skip-git-repo-check", "--ignore-user-config", "--sandbox", "read-only", "-C", commandArguments[6],
		"--output-last-message", commandArguments[8],
		"--config", `model_provider="gentle_ai_reviewer_loopback"`,
		"--config", `model_providers.gentle_ai_reviewer_loopback={name="Gentle AI reviewer loopback",base_url="http://127.0.0.1:43123/v1",wire_api="responses"}`,
	}
	if !slices.Equal(commandArguments, wantArguments) {
		t.Fatalf("loopback Codex arguments = %q, want %q", commandArguments, wantArguments)
	}
}

func TestCodexAdapterRejectsUnsafeLoopbackProvider(t *testing.T) {
	t.Setenv(codexReviewerLoopbackBaseURLEnvironment, "http://127.0.0.1:43123/v1?next=https://example.com")
	adapter := &CodexAdapter{
		LookPath: func(string) (string, error) { return "codex", nil },
		commandContext: func(context.Context, string, ...string) *exec.Cmd {
			t.Fatal("Codex process invoked for an unsafe loopback URL")
			return nil
		},
	}
	raw, err := adapter.Review(context.Background(), NewInvocation([]byte("provider prompt")))
	if err == nil || !strings.Contains(err.Error(), "loopback base URL must not include a query or fragment") {
		t.Fatalf("Review() error = %v, want unsafe loopback rejection", err)
	}
	if raw != nil {
		t.Fatalf("Review() raw = %q, want no result bytes", raw)
	}
}

func TestCodexReviewerLoopbackBaseURLRejectsUnsafeEndpoints(t *testing.T) {
	for _, endpoint := range []string{
		"ftp://127.0.0.1:43123/v1",
		"http://user@127.0.0.1:43123/v1",
		"http://example.com:43123/v1",
		"http://localhost:43123/v1",
		"http://127.0.0.1:0/v1",
		"http://127.0.0.1:43123/redirect",
		"http://127.0.0.1:43123/v1/",
		"http://127.0.0.1:43123/%76%31",
		"http://127.0.0.1:43123/v1?redirect=https://example.com",
		"http://127.0.0.1:43123/v1#redirect",
	} {
		t.Run(endpoint, func(t *testing.T) {
			if _, enabled, err := codexReviewerLoopbackBaseURL(endpoint); err == nil || enabled {
				t.Fatalf("codexReviewerLoopbackBaseURL(%q) = enabled=%t, err=%v; want rejection", endpoint, enabled, err)
			}
		})
	}
}

func TestCodexReviewerLoopbackBaseURLAcceptsNumericLoopbackV1Endpoint(t *testing.T) {
	for _, endpoint := range []string{
		"http://127.0.0.1:43123/v1",
		"https://[::1]:43123/v1",
	} {
		t.Run(endpoint, func(t *testing.T) {
			got, enabled, err := codexReviewerLoopbackBaseURL(endpoint)
			if err != nil || !enabled || got != endpoint {
				t.Fatalf("codexReviewerLoopbackBaseURL(%q) = %q, %t, %v; want %q, true, nil", endpoint, got, enabled, err, endpoint)
			}
		})
	}
}

func TestCodexAdapterHelperProcess(t *testing.T) {
	if os.Getenv(codexAdapterHelperEnvironment) != "1" {
		return
	}
	prompt, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(1)
	}
	if err := os.WriteFile(os.Getenv(codexAdapterPromptPathEnvironment), prompt, 0o600); err != nil {
		os.Exit(1)
	}
	if err := os.WriteFile(os.Getenv(codexAdapterArgumentsPathEnvironment), []byte(strings.Join(os.Args[1:], "\n")), 0o600); err != nil {
		os.Exit(1)
	}
	if path := os.Getenv(codexAdapterWorkdirPathEnvironment); path != "" {
		// Act like a probe that leaves read-only output behind (a Go module
		// cache does): cleanup must still remove the whole scratch.
		workdir, err := os.Getwd()
		if err != nil || os.WriteFile(path, []byte(workdir), 0o600) != nil ||
			os.MkdirAll(filepath.Join(workdir, "probe-cache", "locked"), 0o755) != nil ||
			os.WriteFile(filepath.Join(workdir, "probe-cache", "locked", "entry"), []byte("cached"), 0o444) != nil ||
			os.Chmod(filepath.Join(workdir, "probe-cache", "locked"), 0o555) != nil {
			os.Exit(1)
		}
	}
	if os.Getenv(codexAdapterFailEnvironment) == "1" {
		os.Exit(3)
	}
	for index, argument := range os.Args {
		if argument == "--output-last-message" && index+1 < len(os.Args) {
			if err := os.WriteFile(os.Args[index+1], []byte("raw\x00reviewer\xffoutput"), 0o600); err != nil {
				os.Exit(1)
			}
			return
		}
	}
	os.Exit(1)
}

func codexProbeAdapterForTest(t *testing.T, commandArguments *[]string) *CodexAdapter {
	t.Helper()
	return &CodexAdapter{
		LookPath: func(string) (string, error) { return "codex", nil },
		commandContext: func(ctx context.Context, _ string, arguments ...string) *exec.Cmd {
			*commandArguments = append([]string(nil), arguments...)
			return exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestCodexAdapterHelperProcess$", "--"}, arguments...)...)
		},
	}
}

// TestCodexAdapterRunsRefuterProbeInIsolatedScratchWithoutNetwork pins S11 for
// the one runtime that can isolate a probe: Go materializes the candidate tree
// into a fresh directory under the system temp dir (never the workspace), and
// Codex runs there with workspace-write confined to that directory, network
// access disabled explicitly, and the temp-dir writable roots excluded.
func TestCodexAdapterRunsRefuterProbeInIsolatedScratchWithoutNetwork(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("helper process uses POSIX permissions")
	}
	workdirPath := filepath.Join(t.TempDir(), "workdir")
	t.Setenv(codexAdapterHelperEnvironment, "1")
	t.Setenv(codexAdapterPromptPathEnvironment, filepath.Join(t.TempDir(), "prompt"))
	t.Setenv(codexAdapterArgumentsPathEnvironment, filepath.Join(t.TempDir(), "arguments"))
	t.Setenv(codexAdapterWorkdirPathEnvironment, workdirPath)
	t.Setenv(codexReviewerLoopbackBaseURLEnvironment, "")

	var commandArguments []string
	var materialized string
	invocation := NewInvocation([]byte("refuter prompt")).WithProbeWorkspace(t.TempDir(), func(_ context.Context, dir string) error {
		materialized = dir
		return os.WriteFile(filepath.Join(dir, "candidate.go"), []byte("package candidate\n"), 0o644)
	})
	raw, err := codexProbeAdapterForTest(t, &commandArguments).Review(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	if want := []byte("raw\x00reviewer\xffoutput"); !bytes.Equal(raw, want) {
		t.Fatalf("Review() = %q, want untouched raw bytes %q", raw, want)
	}
	if materialized == "" {
		t.Fatal("probe workspace was never materialized")
	}
	systemTemp, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if relative, err := filepath.Rel(systemTemp, materialized); err != nil || strings.HasPrefix(relative, "..") {
		t.Fatalf("probe scratch %q is not under the system temp dir %q", materialized, systemTemp)
	}
	if workspace, err := os.Getwd(); err == nil {
		if relative, err := filepath.Rel(workspace, materialized); err == nil && !strings.HasPrefix(relative, "..") {
			t.Fatalf("probe scratch %q is inside the workspace %q", materialized, workspace)
		}
	}
	if workdir, err := os.ReadFile(workdirPath); err != nil || string(workdir) != materialized {
		t.Fatalf("Codex ran in %q, %v; want the materialized candidate %q", workdir, err, materialized)
	}
	wantArguments := []string{
		"exec", "--skip-git-repo-check", "--ignore-user-config", "--sandbox", "workspace-write", "-C", materialized,
		"--output-last-message", commandArguments[8],
		"--config", "sandbox_workspace_write.network_access=false",
		"--config", "sandbox_workspace_write.exclude_tmpdir_env_var=true",
		"--config", "sandbox_workspace_write.exclude_slash_tmp=true",
	}
	if !slices.Equal(commandArguments, wantArguments) {
		t.Fatalf("probe Codex arguments = %q, want %q", commandArguments, wantArguments)
	}
	if relative, err := filepath.Rel(materialized, commandArguments[8]); err == nil && !strings.HasPrefix(relative, "..") {
		t.Fatalf("final-message path %q is writable by the probe inside %q", commandArguments[8], materialized)
	}
	if _, err := os.Stat(filepath.Dir(materialized)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("probe scratch %q survived the refuter: %v", filepath.Dir(materialized), err)
	}
}

// TestCodexAdapterRemovesProbeScratchOnError pins S11 cleanup: the scratch
// copy is removed after the refuter returns on every path, including a
// materialization failure, a failing or timed-out Codex process, and probe
// output left read-only inside the copy.
func TestCodexAdapterRemovesProbeScratchOnError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("helper process uses POSIX permissions")
	}
	t.Setenv(codexAdapterHelperEnvironment, "1")
	t.Setenv(codexAdapterPromptPathEnvironment, filepath.Join(t.TempDir(), "prompt"))
	t.Setenv(codexAdapterArgumentsPathEnvironment, filepath.Join(t.TempDir(), "arguments"))
	t.Setenv(codexAdapterWorkdirPathEnvironment, filepath.Join(t.TempDir(), "workdir"))
	t.Setenv(codexReviewerLoopbackBaseURLEnvironment, "")

	t.Run("materialization fails", func(t *testing.T) {
		var materialized string
		adapter := &CodexAdapter{
			LookPath: func(string) (string, error) { return "codex", nil },
			commandContext: func(context.Context, string, ...string) *exec.Cmd {
				t.Fatal("Codex invoked after the probe workspace failed to materialize")
				return nil
			},
		}
		invocation := NewInvocation([]byte("refuter prompt")).WithProbeWorkspace(t.TempDir(), func(_ context.Context, dir string) error {
			materialized = dir
			if err := os.MkdirAll(filepath.Join(dir, "partial"), 0o755); err != nil {
				return err
			}
			return errors.New("tree unavailable")
		})
		raw, err := adapter.Review(context.Background(), invocation)
		if err == nil || !strings.Contains(err.Error(), "tree unavailable") || raw != nil {
			t.Fatalf("Review() = %q, %v; want the materialization error and no bytes", raw, err)
		}
		if _, statErr := os.Stat(filepath.Dir(materialized)); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("probe scratch survived a materialization failure: %v", statErr)
		}
	})

	t.Run("codex fails after leaving read-only output", func(t *testing.T) {
		t.Setenv(codexAdapterFailEnvironment, "1")
		var commandArguments []string
		var materialized string
		invocation := NewInvocation([]byte("refuter prompt")).WithProbeWorkspace(t.TempDir(), func(_ context.Context, dir string) error {
			materialized = dir
			return nil
		})
		raw, err := codexProbeAdapterForTest(t, &commandArguments).Review(context.Background(), invocation)
		if err == nil || raw != nil {
			t.Fatalf("Review() = %q, %v; want a transport error and no bytes", raw, err)
		}
		if _, statErr := os.Stat(filepath.Dir(materialized)); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("probe scratch survived a failing refuter: %v", statErr)
		}
	})

	t.Run("context already expired", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		var commandArguments []string
		var materialized string
		invocation := NewInvocation([]byte("refuter prompt")).WithProbeWorkspace(t.TempDir(), func(_ context.Context, dir string) error {
			materialized = dir
			return nil
		})
		if _, err := codexProbeAdapterForTest(t, &commandArguments).Review(ctx, invocation); err == nil {
			t.Fatal("Review() with an expired context succeeded")
		}
		if materialized != "" {
			if _, statErr := os.Stat(filepath.Dir(materialized)); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("probe scratch survived a timed-out refuter: %v", statErr)
			}
		}
	})
}

// TestCodexAdapterRefusesProbeScratchInsideTheSourceWorkspace pins S11
// confinement: os.MkdirTemp honors TMPDIR, so a temp dir that resolves inside
// the reviewed workspace -- directly, as the workspace itself, through a
// symlink alias, or with the workspace named through an alias -- is refused
// before any scratch is created, the candidate is materialized, or Codex runs.
// A probe without a source root is refused the same way.
func TestCodexAdapterRefusesProbeScratchInsideTheSourceWorkspace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink aliases need POSIX permissions")
	}
	for _, tt := range []struct {
		name  string
		setup func(t *testing.T, source, outside string) (root, tmpdir string)
		want  string
	}{
		{name: "TMPDIR inside the workspace", setup: func(t *testing.T, source, _ string) (string, string) {
			return source, mkdirForTest(t, filepath.Join(source, "tmp"))
		}, want: "inside the reviewed workspace"},
		{name: "TMPDIR is the workspace", setup: func(_ *testing.T, source, _ string) (string, string) {
			return source, source
		}, want: "inside the reviewed workspace"},
		{name: "TMPDIR is a symlink alias into the workspace", setup: func(t *testing.T, source, outside string) (string, string) {
			return source, symlinkForTest(t, mkdirForTest(t, filepath.Join(source, "tmp")), filepath.Join(outside, "tmp-alias"))
		}, want: "inside the reviewed workspace"},
		{name: "workspace named through a symlink alias", setup: func(t *testing.T, source, outside string) (string, string) {
			return symlinkForTest(t, source, filepath.Join(outside, "repo-alias")), mkdirForTest(t, filepath.Join(source, "tmp"))
		}, want: "inside the reviewed workspace"},
		{name: "probe without a source root", setup: func(_ *testing.T, _, outside string) (string, string) {
			return "", outside
		}, want: "no source workspace root"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			source, outside := t.TempDir(), t.TempDir()
			root, tmpdir := tt.setup(t, source, outside)
			before := dirEntriesForTest(t, tmpdir)
			t.Setenv("TMPDIR", tmpdir)
			adapter := &CodexAdapter{
				LookPath: func(string) (string, error) { return "codex", nil },
				commandContext: func(context.Context, string, ...string) *exec.Cmd {
					t.Fatal("Codex invoked although the probe scratch is not confined")
					return nil
				},
			}
			materialized := false
			invocation := NewInvocation([]byte("refuter prompt")).WithProbeWorkspace(root, func(context.Context, string) error {
				materialized = true
				return nil
			})
			raw, err := adapter.Review(context.Background(), invocation)
			if err == nil || !strings.Contains(err.Error(), tt.want) || raw != nil {
				t.Fatalf("Review() = %q, %v; want a refusal naming %q", raw, err, tt.want)
			}
			if materialized {
				t.Fatal("candidate materialized although the probe scratch is not confined")
			}
			if after := dirEntriesForTest(t, tmpdir); !slices.Equal(before, after) {
				t.Fatalf("temp dir entries %v -> %v; want no scratch created", before, after)
			}
		})
	}
}

// TestCodexAdapterProbeConfinementComparesCanonicalPathsNotPrefixes is the
// PRESERVE half: a temp dir outside the workspace still probes, including one
// whose path merely shares the workspace's string prefix, and an invocation
// without a probe is never checked.
func TestCodexAdapterProbeConfinementComparesCanonicalPathsNotPrefixes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("helper process uses POSIX permissions")
	}
	parent := t.TempDir()
	source := mkdirForTest(t, filepath.Join(parent, "repo"))
	sibling := mkdirForTest(t, filepath.Join(parent, "repo-tmp"))
	t.Setenv(codexAdapterHelperEnvironment, "1")
	t.Setenv(codexAdapterPromptPathEnvironment, filepath.Join(t.TempDir(), "prompt"))
	t.Setenv(codexAdapterArgumentsPathEnvironment, filepath.Join(t.TempDir(), "arguments"))
	t.Setenv(codexAdapterWorkdirPathEnvironment, filepath.Join(t.TempDir(), "workdir"))
	t.Setenv(codexReviewerLoopbackBaseURLEnvironment, "")
	t.Setenv("TMPDIR", sibling)

	var commandArguments []string
	var materialized string
	invocation := NewInvocation([]byte("refuter prompt")).WithProbeWorkspace(source, func(_ context.Context, dir string) error {
		materialized = dir
		return nil
	})
	if _, err := codexProbeAdapterForTest(t, &commandArguments).Review(context.Background(), invocation); err != nil {
		t.Fatalf("probe with a prefix-sharing sibling temp dir: %v", err)
	}
	if relative, err := filepath.Rel(sibling, materialized); err != nil || strings.HasPrefix(relative, "..") {
		t.Fatalf("probe scratch %q is not under the sibling temp dir %q", materialized, sibling)
	}

	t.Setenv("TMPDIR", mkdirForTest(t, filepath.Join(source, "tmp")))
	if _, err := codexProbeAdapterForTest(t, &commandArguments).Review(context.Background(), NewInvocation([]byte("lens prompt"))); err != nil {
		t.Fatalf("read-only reviewer without a probe was checked for confinement: %v", err)
	}
	if !slices.Contains(commandArguments, "read-only") {
		t.Fatalf("reviewer without a probe ran %q, want the read-only sandbox", commandArguments)
	}
}

func mkdirForTest(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func symlinkForTest(t *testing.T, target, link string) string {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	return link
}

func dirEntriesForTest(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}
