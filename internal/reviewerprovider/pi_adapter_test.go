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

const piAdapterHelperEnvironment = "GENTLE_AI_REVIEWER_PROVIDER_PI_HELPER"
const piAdapterPromptPathEnvironment = "GENTLE_AI_REVIEWER_PROVIDER_PI_PROMPT_PATH"

func TestPiAdapterReturnsNoBytesWhenUnavailable(t *testing.T) {
	adapter := &PiAdapter{LookPath: func(string) (string, error) { return "", errors.New("not found") }}
	raw, err := adapter.Review(context.Background(), NewInvocation([]byte("provider prompt")))
	if err == nil || !strings.Contains(err.Error(), "pi reviewer transport unavailable") || raw != nil {
		t.Fatalf("Review() = %q, %v; want unavailable transport error and no bytes", raw, err)
	}
}

func TestPiAdapterUsesStdinLockedDownArgumentsAndReturnsUntouchedRawOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the helper process uses POSIX argument handling")
	}
	promptPath := filepath.Join(t.TempDir(), "prompt")
	t.Setenv(piAdapterHelperEnvironment, "1")
	t.Setenv(piAdapterPromptPathEnvironment, promptPath)
	var commandArguments []string
	var command *exec.Cmd
	adapter := &PiAdapter{
		LookPath: func(string) (string, error) { return "pi", nil },
		commandContext: func(ctx context.Context, _ string, arguments ...string) *exec.Cmd {
			commandArguments = append([]string(nil), arguments...)
			command = exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestPiAdapterHelperProcess$", "--"}, arguments...)...)
			return command
		},
	}
	prompt := []byte("provider prompt\nwith bytes")
	raw, err := adapter.Review(context.Background(), NewInvocation(prompt))
	if err != nil {
		t.Fatal(err)
	}
	if want := []byte("raw\x00pi\xffoutput"); !bytes.Equal(raw, want) {
		t.Fatalf("Review() = %q, want untouched raw bytes %q", raw, want)
	}
	if got, err := os.ReadFile(promptPath); err != nil || !bytes.Equal(got, prompt) {
		t.Fatalf("reviewer stdin = %q, %v; want %q", got, err, prompt)
	}
	if want := []string{
		"--print", "--mode", "text", "--no-session", "--no-tools", "--no-extensions",
		"--no-skills", "--no-prompt-templates", "--no-themes", "--no-context-files", "--no-approve",
	}; !slices.Equal(commandArguments, want) {
		t.Fatalf("pi arguments = %q, want %q", commandArguments, want)
	}
	if command.WaitDelay != piReviewerWaitDelay {
		t.Fatalf("pi WaitDelay = %v, want %v so a held pipe cannot outlive a context kill", command.WaitDelay, piReviewerWaitDelay)
	}
}

func TestPiAdapterFailsClosedOnProcessFailureAndEmptyOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the helper process uses POSIX argument handling")
	}
	t.Setenv(piAdapterPromptPathEnvironment, filepath.Join(t.TempDir(), "prompt"))
	for name, want := range map[string]string{"fail": "pi reviewer transport failed", "empty": "produced no final message"} {
		t.Setenv(piAdapterHelperEnvironment, name)
		adapter := &PiAdapter{
			LookPath: func(string) (string, error) { return "pi", nil },
			commandContext: func(ctx context.Context, _ string, arguments ...string) *exec.Cmd {
				return exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestPiAdapterHelperProcess$", "--"}, arguments...)...)
			},
		}
		raw, err := adapter.Review(context.Background(), NewInvocation([]byte("prompt")))
		if err == nil || !strings.Contains(err.Error(), want) || raw != nil {
			t.Fatalf("Review(%s) = %q, %v; want %q", name, raw, err, want)
		}
	}
}

// TestPiAdapterHelperProcess is the fake pi binary. It exits explicitly so a
// helper run never prints Go test PASS noise into the captured raw stream.
func TestPiAdapterHelperProcess(t *testing.T) {
	mode := os.Getenv(piAdapterHelperEnvironment)
	if mode == "" {
		return
	}
	prompt, err := io.ReadAll(os.Stdin)
	if err != nil || os.WriteFile(os.Getenv(piAdapterPromptPathEnvironment), prompt, 0o600) != nil {
		os.Exit(1)
	}
	switch mode {
	case "fail":
		os.Exit(3)
	case "empty":
		os.Exit(0)
	case "stdout-failure":
		_, _ = os.Stdout.WriteString("Not logged in\nsecond line\n")
		os.Exit(1)
	}
	if _, err := os.Stdout.WriteString("raw\x00pi\xffoutput"); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

// Issue #3289: a pi child that prints its reason to stdout and exits non-zero
// must surface that reason instead of an empty tail after the exit status.
func TestPiAdapterFailureNamesStdoutReasonWhenStderrIsEmpty(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the helper process uses POSIX argument handling")
	}
	t.Setenv(piAdapterPromptPathEnvironment, filepath.Join(t.TempDir(), "prompt"))
	t.Setenv(piAdapterHelperEnvironment, "stdout-failure")
	adapter := &PiAdapter{
		LookPath: func(string) (string, error) { return "pi", nil },
		commandContext: func(ctx context.Context, _ string, arguments ...string) *exec.Cmd {
			return exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestPiAdapterHelperProcess$", "--"}, arguments...)...)
		},
	}
	raw, err := adapter.Review(context.Background(), NewInvocation([]byte("prompt")))
	if raw != nil || err == nil || !strings.Contains(err.Error(), "pi reviewer transport failed") ||
		!strings.Contains(err.Error(), "Not logged in") || strings.Contains(err.Error(), "\n") {
		t.Fatalf("Review() = %q, %v; want a single-line transport failure naming the stdout reason", raw, err)
	}
}
