package model

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDiscoverCodexModels(t *testing.T) {
	tests := []struct {
		name        string
		lookPathErr error
		mode        string
		output      string
		cancel      bool
		timeout     time.Duration
		want        []string
		wantArgs    []string
		wantCommand bool
	}{
		{
			name:        "missing executable",
			lookPathErr: errors.New("codex not found"),
			want:        CodexAvailableModels(),
		},
		{
			name:        "command failure",
			mode:        "error",
			want:        CodexAvailableModels(),
			wantArgs:    []string{"/fake/bin/codex", "debug", "models"},
			wantCommand: true,
		},
		{
			name:        "invalid JSON",
			mode:        "output",
			output:      "not-json",
			want:        CodexAvailableModels(),
			wantArgs:    []string{"/fake/bin/codex", "debug", "models"},
			wantCommand: true,
		},
		{
			name:        "empty output",
			want:        CodexAvailableModels(),
			wantArgs:    []string{"/fake/bin/codex", "debug", "models"},
			wantCommand: true,
		},
		{
			name:        "no selectable entries",
			mode:        "output",
			output:      `{"models":[{"slug":"hidden","visibility":"hide"},{"slug":"unsupported","supported_in_api":false}]}`,
			want:        CodexAvailableModels(),
			wantArgs:    []string{"/fake/bin/codex", "debug", "models"},
			wantCommand: true,
		},
		{
			name:        "cancellation",
			mode:        "wait",
			cancel:      true,
			want:        CodexAvailableModels(),
			wantArgs:    []string{"/fake/bin/codex", "debug", "models"},
			wantCommand: true,
		},
		{
			name:        "timeout",
			mode:        "wait",
			timeout:     50 * time.Millisecond,
			want:        CodexAvailableModels(),
			wantArgs:    []string{"/fake/bin/codex", "debug", "models"},
			wantCommand: true,
		},
		{
			name:        "output cap",
			mode:        "output-limit",
			want:        CodexAvailableModels(),
			wantArgs:    []string{"/fake/bin/codex", "debug", "models"},
			wantCommand: true,
		},
		{
			name:        "optional fields, whitespace, and duplicates",
			mode:        "output",
			output:      `{"models":[{"slug":"  gpt-5.6  "},{"slug":"gpt-5.6","visibility":"list","supported_in_api":true},{"slug":"legacy"},{"slug":"codex-auto-review","visibility":"hide","supported_in_api":true},{"slug":"hidden","visibility":"hide"},{"slug":"unsupported","supported_in_api":false},{"slug":"   "}]}`,
			want:        []string{"gpt-5.6", "legacy"},
			wantArgs:    []string{"/fake/bin/codex", "debug", "models"},
			wantCommand: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalLookPath := codexLookPath
			originalCommand := codexCommand
			originalTimeout := codexModelDiscoveryTimeout
			t.Cleanup(func() {
				codexLookPath = originalLookPath
				codexCommand = originalCommand
				codexModelDiscoveryTimeout = originalTimeout
			})

			called := false
			var gotArgs []string
			codexLookPath = func(string) (string, error) {
				if tt.lookPathErr != nil {
					return "", tt.lookPathErr
				}
				return "/fake/bin/codex", nil
			}
			codexCommand = func(ctx context.Context, path string, args ...string) *exec.Cmd {
				called = true
				gotArgs = append([]string{path}, args...)
				return codexHelperCommand(ctx, tt.mode, tt.output)
			}
			if tt.timeout != 0 {
				codexModelDiscoveryTimeout = tt.timeout
			}

			ctx := context.Background()
			if tt.cancel {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			got := DiscoverCodexModels(ctx)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("DiscoverCodexModels() = %v, want %v", got, tt.want)
			}
			if called != tt.wantCommand {
				t.Fatalf("command called = %v, want %v", called, tt.wantCommand)
			}
			if tt.wantArgs != nil {
				if !reflect.DeepEqual(tt.wantArgs, gotArgs) {
					t.Fatalf("command args = %v, want %v", gotArgs, tt.wantArgs)
				}
			}
		})
	}
}

func codexHelperCommand(ctx context.Context, mode, output string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCodexDiscoveryHelperProcess$", "--")
	cmd.Env = append(os.Environ(), "GO_WANT_CODEX_HELPER=1", "GO_CODEX_HELPER_MODE="+mode, "GO_CODEX_HELPER_OUTPUT="+output)
	return cmd
}

func TestCodexDiscoveryHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_CODEX_HELPER") != "1" {
		return
	}

	switch os.Getenv("GO_CODEX_HELPER_MODE") {
	case "error":
		os.Exit(1)
	case "wait":
		select {}
	case "output-limit":
		_, _ = os.Stdout.WriteString(strings.Repeat("x", codexModelDiscoveryOutputLimit+1))
	default:
		_, _ = os.Stdout.WriteString(os.Getenv("GO_CODEX_HELPER_OUTPUT"))
	}
	os.Exit(0)
}
