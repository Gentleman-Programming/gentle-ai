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

const piAdapterHelperModeArgument = "--pi-adapter-helper-mode="
const piAdapterHelperOutputPathArgument = "--pi-adapter-helper-output-path="

func TestPiAdapterPassesOnlyRuntimeEnvironmentAndOptionalAuthLocator(t *testing.T) {
	for _, tc := range []struct {
		name    string
		locator *string
	}{
		{name: "locator present", locator: ptr("/private/pi")},
		{name: "locator absent"},
		{name: "locator empty", locator: ptr("")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("PATH", "/runtime/path")
			t.Setenv("HOME", "/runtime/home")
			t.Setenv("OPENAI_API_KEY", "sentinel-api-key")
			t.Setenv("UNRELATED_ENVIRONMENT", "sentinel-unrelated")
			t.Setenv("GENTLE_ARBITRARY_ENVIRONMENT", "sentinel-gentle")
			if tc.locator == nil {
				unsetenv(t, "PI_CODING_AGENT_DIR")
			} else {
				t.Setenv("PI_CODING_AGENT_DIR", *tc.locator)
			}

			environmentPath := filepath.Join(t.TempDir(), "environment")
			var command *exec.Cmd
			adapter := &PiAdapter{
				LookPath: func(string) (string, error) { return "pi", nil },
				commandContext: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
					command = exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPiAdapterHelperProcess$", "--",
						piAdapterHelperModeArgument+"environment",
						piAdapterHelperOutputPathArgument+environmentPath)
					return command
				},
			}
			if _, err := adapter.Review(context.Background(), NewInvocation([]byte("prompt"))); err != nil {
				t.Fatal(err)
			}
			if command.Env == nil {
				t.Fatal("Pi command Env is nil; want an explicit allowlist")
			}
			explicit := environmentMap([]byte(strings.Join(command.Env, "\x00")))
			if explicit["PATH"] != "/runtime/path" || explicit["HOME"] != "/runtime/home" {
				t.Errorf("explicit environment = %q, want runtime PATH and HOME", command.Env)
			}
			if tc.locator == nil || *tc.locator == "" {
				if _, found := explicit["PI_CODING_AGENT_DIR"]; found {
					t.Errorf("explicit environment includes PI_CODING_AGENT_DIR=%q; want absent", explicit["PI_CODING_AGENT_DIR"])
				}
			} else if explicit["PI_CODING_AGENT_DIR"] != *tc.locator {
				t.Errorf("explicit PI_CODING_AGENT_DIR = %q, want %q", explicit["PI_CODING_AGENT_DIR"], *tc.locator)
			}
			for _, name := range []string{"OPENAI_API_KEY", "UNRELATED_ENVIRONMENT", "GENTLE_ARBITRARY_ENVIRONMENT"} {
				if _, found := explicit[name]; found {
					t.Errorf("explicit environment leaks %s=%q", name, explicit[name])
				}
			}

			environment, err := os.ReadFile(environmentPath)
			if err != nil {
				t.Fatal(err)
			}
			got := environmentMap(environment)
			for _, name := range []string{"PATH", "HOME"} {
				if got[name] == "" {
					t.Errorf("child environment lacks required %s", name)
				}
			}
			if tc.locator == nil || *tc.locator == "" {
				if _, found := got["PI_CODING_AGENT_DIR"]; found {
					t.Errorf("child environment includes PI_CODING_AGENT_DIR=%q; want absent", got["PI_CODING_AGENT_DIR"])
				}
			} else if got["PI_CODING_AGENT_DIR"] != *tc.locator {
				t.Errorf("child PI_CODING_AGENT_DIR = %q, want %q", got["PI_CODING_AGENT_DIR"], *tc.locator)
			}
			for _, name := range []string{"OPENAI_API_KEY", "UNRELATED_ENVIRONMENT", "GENTLE_ARBITRARY_ENVIRONMENT"} {
				if _, found := got[name]; found {
					t.Errorf("child environment leaks %s=%q", name, got[name])
				}
			}
			for name := range got {
				if name == "PATH" || name == "HOME" || name == "PI_CODING_AGENT_DIR" || (runtime.GOOS == "windows" && (name == "SYSTEMROOT" || name == "USERPROFILE" || name == "HOMEDRIVE" || name == "HOMEPATH")) {
					continue
				}
				t.Errorf("child environment includes non-runtime variable %s", name)
			}
		})
	}
}

func ptr(value string) *string { return &value }

func unsetenv(t *testing.T, name string) {
	t.Helper()
	value, found := os.LookupEnv(name)
	if err := os.Unsetenv(name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if found {
			_ = os.Setenv(name, value)
		} else {
			_ = os.Unsetenv(name)
		}
	})
}

func environmentMap(environment []byte) map[string]string {
	values := make(map[string]string)
	for _, entry := range strings.Split(string(environment), "\x00") {
		name, value, found := strings.Cut(entry, "=")
		if found {
			values[name] = value
		}
	}
	return values
}

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
	var commandArguments []string
	var command *exec.Cmd
	adapter := &PiAdapter{
		LookPath: func(string) (string, error) { return "pi", nil },
		commandContext: func(ctx context.Context, _ string, arguments ...string) *exec.Cmd {
			commandArguments = append([]string(nil), arguments...)
			helperArguments := append([]string{"-test.run=^TestPiAdapterHelperProcess$", "--", piAdapterHelperModeArgument + "success", piAdapterHelperOutputPathArgument + promptPath}, arguments...)
			command = exec.CommandContext(ctx, os.Args[0], helperArguments...)
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
	for name, want := range map[string]string{"fail": "pi reviewer transport failed", "empty": "produced no final message"} {
		adapter := &PiAdapter{
			LookPath: func(string) (string, error) { return "pi", nil },
			commandContext: func(ctx context.Context, _ string, arguments ...string) *exec.Cmd {
				helperArguments := append([]string{"-test.run=^TestPiAdapterHelperProcess$", "--", piAdapterHelperModeArgument + name}, arguments...)
				return exec.CommandContext(ctx, os.Args[0], helperArguments...)
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
	mode := piAdapterHelperOption(piAdapterHelperModeArgument)
	if mode == "" {
		return
	}
	if outputPath := piAdapterHelperOption(piAdapterHelperOutputPathArgument); outputPath != "" {
		var output []byte
		if mode == "environment" {
			output = []byte(strings.Join(os.Environ(), "\x00"))
		} else {
			var err error
			output, err = io.ReadAll(os.Stdin)
			if err != nil {
				os.Exit(1)
			}
		}
		if err := os.WriteFile(outputPath, output, 0o600); err != nil {
			os.Exit(1)
		}
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

func piAdapterHelperOption(prefix string) string {
	for _, argument := range os.Args {
		if strings.HasPrefix(argument, prefix) {
			return strings.TrimPrefix(argument, prefix)
		}
	}
	return ""
}

// Issue #3289: a pi child that prints its reason to stdout and exits non-zero
// must surface that reason instead of an empty tail after the exit status.
func TestPiAdapterFailureNamesStdoutReasonWhenStderrIsEmpty(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the helper process uses POSIX argument handling")
	}
	adapter := &PiAdapter{
		LookPath: func(string) (string, error) { return "pi", nil },
		commandContext: func(ctx context.Context, _ string, arguments ...string) *exec.Cmd {
			helperArguments := append([]string{"-test.run=^TestPiAdapterHelperProcess$", "--", piAdapterHelperModeArgument + "stdout-failure"}, arguments...)
			return exec.CommandContext(ctx, os.Args[0], helperArguments...)
		},
	}
	raw, err := adapter.Review(context.Background(), NewInvocation([]byte("prompt")))
	if raw != nil || err == nil || !strings.Contains(err.Error(), "pi reviewer transport failed") ||
		!strings.Contains(err.Error(), "Not logged in") || strings.Contains(err.Error(), "\n") {
		t.Fatalf("Review() = %q, %v; want a single-line transport failure naming the stdout reason", raw, err)
	}
}
