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
	"strings"
	"testing"
)

const claudeAdapterHelperEnvironment = "GENTLE_AI_REVIEWER_PROVIDER_CLAUDE_HELPER"
const claudeAdapterPromptPathEnvironment = "GENTLE_AI_REVIEWER_PROVIDER_CLAUDE_PROMPT_PATH"
const claudeAdapterArgumentsPathEnvironment = "GENTLE_AI_REVIEWER_PROVIDER_CLAUDE_ARGUMENTS_PATH"

func TestClaudeAdapterReturnsNoBytesWhenUnavailable(t *testing.T) {
	adapter := &ClaudeAdapter{LookPath: func(string) (string, error) { return "", errors.New("not found") }}
	raw, err := adapter.Review(context.Background(), NewInvocation([]byte("provider prompt")))
	if err == nil || !strings.Contains(err.Error(), "claude reviewer transport unavailable") {
		t.Fatalf("Review() error = %v, want unavailable transport error", err)
	}
	if raw != nil {
		t.Fatalf("Review() raw = %q with transport error, want no result bytes", raw)
	}
}

func TestClaudeAdapterUsesStdinAndReturnsUntouchedRawOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the helper process uses POSIX argument handling")
	}
	promptPath := filepath.Join(t.TempDir(), "prompt")
	argumentsPath := filepath.Join(t.TempDir(), "arguments")
	t.Setenv(claudeAdapterHelperEnvironment, "1")
	t.Setenv(claudeAdapterPromptPathEnvironment, promptPath)
	t.Setenv(claudeAdapterArgumentsPathEnvironment, argumentsPath)

	adapter := &ClaudeAdapter{
		LookPath: func(string) (string, error) { return "claude", nil },
		commandContext: func(ctx context.Context, _ string, arguments ...string) *exec.Cmd {
			return exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestClaudeAdapterHelperProcess$", "--"}, arguments...)...)
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
		t.Fatalf("claude arguments carried provider prompt: %q", arguments)
	}
	if strings.Contains(string(arguments), "--bare") {
		t.Fatal("claude arguments carry --bare: bare mode never reads OAuth or keychain credentials, so subscription-authenticated machines cannot run the reviewer")
	}
	for _, flag := range []string{"--print", "--output-format", "text", "--tools", "", "--setting-sources", "--system-prompt"} {
		if !strings.Contains(string(arguments), flag) {
			t.Fatalf("claude arguments = %q, missing %q", arguments, flag)
		}
	}
}

func TestClaudeAdapterHelperProcess(t *testing.T) {
	mode := os.Getenv(claudeAdapterHelperEnvironment)
	if mode == "" {
		return
	}
	if mode == "stdout-failure" {
		_, _ = os.Stdout.WriteString("Not logged in · Please run /login\n" + strings.Repeat("second line ", 100))
		os.Exit(1)
	}
	prompt, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(1)
	}
	if err := os.WriteFile(os.Getenv(claudeAdapterPromptPathEnvironment), prompt, 0o600); err != nil {
		os.Exit(1)
	}
	if err := os.WriteFile(os.Getenv(claudeAdapterArgumentsPathEnvironment), []byte(strings.Join(os.Args[1:], "\n")), 0o600); err != nil {
		os.Exit(1)
	}
	_, _ = os.Stdout.Write([]byte("raw\x00reviewer\xffoutput"))
	os.Exit(0)
}

// Issue #3289: a child that prints its reason to stdout (`Not logged in`) and
// exits non-zero produced `claude reviewer transport failed: exit status 1:`
// with nothing after the colon, because only stderr was formatted.
func TestClaudeAdapterFailureNamesStdoutReasonWhenStderrIsEmpty(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the helper process uses POSIX argument handling")
	}
	t.Setenv(claudeAdapterHelperEnvironment, "stdout-failure")
	t.Setenv(claudeAdapterPromptPathEnvironment, filepath.Join(t.TempDir(), "prompt"))
	t.Setenv(claudeAdapterArgumentsPathEnvironment, filepath.Join(t.TempDir(), "arguments"))
	adapter := &ClaudeAdapter{
		LookPath: func(string) (string, error) { return "claude", nil },
		commandContext: func(ctx context.Context, _ string, arguments ...string) *exec.Cmd {
			return exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestClaudeAdapterHelperProcess$", "--"}, arguments...)...)
		},
	}
	raw, err := adapter.Review(context.Background(), NewInvocation([]byte("provider prompt")))
	if raw != nil || err == nil || !strings.Contains(err.Error(), "claude reviewer transport failed") || !strings.Contains(err.Error(), "Not logged in") ||
		strings.Contains(err.Error(), "\n") || strings.Contains(err.Error(), "second line") || len(err.Error()) > 700 {
		t.Fatalf("Review() = %q, %v; want a single-line bounded transport failure naming the stdout reason", raw, err)
	}
}
