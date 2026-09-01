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
	if runtime.GOOS == "windows" {
		t.Skip("the helper process uses POSIX argument handling")
	}
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

func TestCodexAdapterConfiguresApprovedLoopbackProvider(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the helper process uses POSIX argument handling")
	}
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
