package model

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func TestDiscoverCodexModels(t *testing.T) {
	tests := []struct {
		name         string
		lookPathErr  error
		command      func(context.Context, string, ...string) *exec.Cmd
		want         []string
		wantArgs     []string
		wantLookPath bool
	}{
		{
			name:        "missing executable",
			lookPathErr: errors.New("codex not found"),
			want:        CodexAvailableModels(),
		},
		{
			name: "command failure",
			command: func(context.Context, string, ...string) *exec.Cmd {
				return exec.Command("false")
			},
			want:         CodexAvailableModels(),
			wantArgs:     []string{"/fake/bin/codex", "debug", "models"},
			wantLookPath: true,
		},
		{
			name: "invalid JSON",
			command: func(context.Context, string, ...string) *exec.Cmd {
				return exec.Command("printf", "%s", "not-json")
			},
			want:         CodexAvailableModels(),
			wantArgs:     []string{"/fake/bin/codex", "debug", "models"},
			wantLookPath: true,
		},
		{
			name: "empty output",
			command: func(context.Context, string, ...string) *exec.Cmd {
				return exec.Command("true")
			},
			want:         CodexAvailableModels(),
			wantArgs:     []string{"/fake/bin/codex", "debug", "models"},
			wantLookPath: true,
		},
		{
			name: "no selectable entries",
			command: func(context.Context, string, ...string) *exec.Cmd {
				return exec.Command("printf", "%s", `{"models":[{"slug":"hidden","visibility":"hide"},{"slug":"unsupported","supported_in_api":false}]}`)
			},
			want:         CodexAvailableModels(),
			wantArgs:     []string{"/fake/bin/codex", "debug", "models"},
			wantLookPath: true,
		},
		{
			name: "cancellation",
			command: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
				return exec.CommandContext(ctx, "sleep", "1")
			},
			want:         CodexAvailableModels(),
			wantArgs:     []string{"/fake/bin/codex", "debug", "models"},
			wantLookPath: true,
		},
		{
			name: "output cap",
			command: func(context.Context, string, ...string) *exec.Cmd {
				return exec.Command("printf", "%s", strings.Repeat("x", codexModelDiscoveryOutputLimit+1))
			},
			want:         CodexAvailableModels(),
			wantArgs:     []string{"/fake/bin/codex", "debug", "models"},
			wantLookPath: true,
		},
		{
			name: "optional fields, whitespace, and duplicates",
			command: func(context.Context, string, ...string) *exec.Cmd {
				return exec.Command("printf", "%s", `{"models":[{"slug":"  gpt-5.6  "},{"slug":"gpt-5.6","visibility":"list","supported_in_api":true},{"slug":"legacy"},{"slug":"codex-auto-review","visibility":"hide","supported_in_api":true},{"slug":"hidden","visibility":"hide"},{"slug":"unsupported","supported_in_api":false},{"slug":"   "}]}`)
			},
			want:         []string{"gpt-5.6", "legacy"},
			wantArgs:     []string{"/fake/bin/codex", "debug", "models"},
			wantLookPath: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalLookPath := codexLookPath
			originalCommand := codexCommand
			t.Cleanup(func() {
				codexLookPath = originalLookPath
				codexCommand = originalCommand
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
				if tt.command == nil {
					return exec.CommandContext(ctx, "false")
				}
				return tt.command(ctx, path, args...)
			}

			ctx := context.Background()
			if tt.name == "cancellation" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			got := DiscoverCodexModels(ctx)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("DiscoverCodexModels() = %v, want %v", got, tt.want)
			}
			if called != tt.wantLookPath {
				t.Fatalf("command called = %v, want %v", called, tt.wantLookPath)
			}
			if tt.wantArgs != nil {
				if !reflect.DeepEqual(tt.wantArgs, gotArgs) {
					t.Fatalf("command args = %v, want %v", gotArgs, tt.wantArgs)
				}
			}
		})
	}
}
