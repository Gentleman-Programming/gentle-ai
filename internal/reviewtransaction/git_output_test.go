package reviewtransaction

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func TestRunGitCapturesBoundedSeparateOutput(t *testing.T) {
	// Keep these subtests sequential because gitCommandContext is package-global.
	for _, tt := range []struct {
		name        string
		mode        string
		runner      string
		wantOutput  string
		wantLimits  []int
		wantActual  []int
		wantExit    int
		wantStderr  string
		wantDiag    string
		wantEntries int
	}{
		{name: "ordinary success", mode: "stdout", wantOutput: "machine-output\n"},
		{name: "ordinary stdout excludes diagnostics", mode: "stdout-stderr", wantOutput: "machine-output\n"},
		{name: "isolated stdout excludes diagnostics", mode: "stdout-stderr", runner: "isolated", wantOutput: "machine-output\n"},
		{name: "inventory default limit success", mode: "stdout", runner: "inventory", wantOutput: "machine-output\n"},
		{name: "inventory diagnostics are typed", mode: "stdout-stderr", runner: "inventory", wantDiag: "diagnostic output"},
		// #3498: a tracked inventory past the 8 MiB command limit is ordinary.
		{name: "inventory admits an eight MiB payload", mode: "stdout-overflow", runner: "inventory", wantOutput: strings.Repeat("o", defaultGitOutputLimit+1)},
		{name: "inventory default limit overflow names entries", mode: "inventory-overflow", runner: "inventory", wantLimits: []int{defaultGitInventoryLimit}, wantActual: []int{defaultGitInventoryLimit + 1}, wantEntries: defaultGitInventoryLimit/2 + 1},
		{name: "zero limit uses default", mode: "stdout-overflow", runner: "zero-limit", wantLimits: []int{defaultGitOutputLimit}, wantActual: []int{defaultGitOutputLimit + 1}},
		{name: "stdout overflow", mode: "stdout-overflow", wantLimits: []int{defaultGitOutputLimit}, wantActual: []int{defaultGitOutputLimit + 1}},
		{name: "stderr overflow", mode: "stderr-overflow", wantLimits: []int{defaultGitStderrLimit}, wantActual: []int{defaultGitStderrLimit + 1}},
		{name: "failed command retains stderr", mode: "failure", wantExit: 7, wantStderr: "meaningful diagnostic"},
		{name: "failed command with stdout overflow", mode: "failure-stdout-overflow", wantLimits: []int{defaultGitOutputLimit}, wantActual: []int{defaultGitOutputLimit + 1}, wantExit: 7, wantStderr: "actionable failure"},
		{name: "failed command with stderr overflow", mode: "failure-stderr-overflow", wantLimits: []int{defaultGitStderrLimit}, wantActual: []int{defaultGitStderrLimit + len("actionable failure\n") + 1}, wantExit: 7, wantStderr: "actionable failure"},
		{name: "failed command with simultaneous overflow", mode: "failure-both-overflow", wantLimits: []int{defaultGitOutputLimit, defaultGitStderrLimit}, wantActual: []int{defaultGitOutputLimit + 1, defaultGitStderrLimit + len("actionable failure\n") + 1}, wantExit: 7, wantStderr: "actionable failure"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			originalCommand := gitCommandContext
			t.Cleanup(func() { gitCommandContext = originalCommand })
			gitCommandContext = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
				return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRunGitOutputHelper$", "--")
			}

			extraEnv := []string{"GENTLE_AI_GIT_OUTPUT_HELPER=" + tt.mode}
			var output []byte
			var err error
			switch tt.runner {
			case "isolated":
				output, err = runGitIsolated(context.Background(), t.TempDir(), extraEnv, nil, "status")
			case "inventory":
				output, err = runGitInventoryWithEnv(context.Background(), t.TempDir(), extraEnv, "status")
			case "zero-limit":
				output, err = runGitCaptured(context.Background(), t.TempDir(), extraEnv, nil, 0, false, false, "status")
			default:
				output, err = runGit(context.Background(), t.TempDir(), extraEnv, nil, "status")
			}
			if len(tt.wantLimits) != 0 {
				var firstOverflow *GitOutputLimitError
				if !errors.Is(err, ErrGitOutputLimit) || !errors.As(err, &firstOverflow) || firstOverflow.Limit != tt.wantLimits[0] || firstOverflow.Entries != tt.wantEntries {
					t.Fatalf("output overflow typing = %T %#v", err, firstOverflow)
				}
				if tt.wantEntries != 0 && !strings.Contains(err.Error(), fmt.Sprintf("(%d NUL-terminated entries)", tt.wantEntries)) {
					t.Fatalf("inventory overflow = %q, want it to name %d entries", err, tt.wantEntries)
				}
				limits, actual := gitOutputLimitDetails(err)
				if !reflect.DeepEqual(limits, tt.wantLimits) || !reflect.DeepEqual(actual, tt.wantActual) {
					t.Fatalf("output limits = %v/%v, want %v/%v", limits, actual, tt.wantLimits, tt.wantActual)
				}
			}
			if tt.wantExit != 0 {
				var commandErr *GitCommandError
				if !errors.As(err, &commandErr) || commandErr.ExitCode != tt.wantExit || !strings.Contains(commandErr.Output, tt.wantStderr) || len(commandErr.Output) > defaultGitStderrLimit || strings.Contains(commandErr.Output, "machine-output") || len(output) != 0 {
					t.Fatalf("command error = %T %#v", err, commandErr)
				}
				return
			}
			if tt.wantDiag != "" {
				var diagnosticErr *GitInventoryDiagnosticsError
				if !errors.Is(err, ErrGitInventoryDiagnostics) || !errors.As(err, &diagnosticErr) || diagnosticErr.Diagnostics != tt.wantDiag || len(output) != 0 {
					t.Fatalf("inventory diagnostic error = %T %#v", err, diagnosticErr)
				}
				return
			}
			if len(tt.wantLimits) != 0 {
				return
			}
			if err != nil || string(output) != tt.wantOutput {
				t.Fatalf("runGit output = %q, error = %v", output, err)
			}
		})
	}
}

func TestGitOutputOverflowReportsStderrCollectorLimit(t *testing.T) {
	err := gitOutputOverflow([]string{"status"}, defaultGitOutputLimit, &boundedGitOutput{}, &boundedGitOutput{limit: 17, total: 18, exceeded: true}, true)
	var overflow *GitOutputLimitError
	if !errors.As(err, &overflow) || overflow.Limit != 17 || overflow.Actual != 18 {
		t.Fatalf("stderr overflow = %T %#v", err, overflow)
	}
}

func gitOutputLimitDetails(err error) (limits, actual []int) {
	switch typed := err.(type) {
	case *GitOutputLimitError:
		return []int{typed.Limit}, []int{typed.Actual}
	case interface{ Unwrap() []error }:
		for _, cause := range typed.Unwrap() {
			causeLimits, causeActual := gitOutputLimitDetails(cause)
			limits, actual = append(limits, causeLimits...), append(actual, causeActual...)
		}
	case interface{ Unwrap() error }:
		return gitOutputLimitDetails(typed.Unwrap())
	}
	return limits, actual
}

func TestRunGitTraceDoesNotContaminateMachineOutput(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	t.Setenv("GIT_TRACE", "1")

	output, err := runGit(context.Background(), repo, nil, nil, "rev-parse", "--is-inside-work-tree")
	if err != nil || string(output) != "true\n" {
		t.Fatalf("traced rev-parse output = %q, error = %v", output, err)
	}
}

func TestRunGitOutputHelper(t *testing.T) {
	mode := os.Getenv("GENTLE_AI_GIT_OUTPUT_HELPER")
	if mode == "" {
		return
	}
	switch mode {
	case "stdout":
		fmt.Fprint(os.Stdout, "machine-output\n")
	case "stdout-stderr":
		fmt.Fprint(os.Stdout, "machine-output\n")
		fmt.Fprint(os.Stderr, "diagnostic output\n")
	case "stdout-overflow":
		fmt.Fprint(os.Stdout, strings.Repeat("o", defaultGitOutputLimit+1))
	case "inventory-overflow":
		fmt.Fprint(os.Stdout, strings.Repeat("o\x00", defaultGitInventoryLimit/2)+"\x00")
	case "stderr-overflow":
		fmt.Fprint(os.Stderr, strings.Repeat("e", defaultGitStderrLimit+1))
	case "failure":
		fmt.Fprint(os.Stderr, "meaningful diagnostic\n")
		os.Exit(7)
	case "failure-stdout-overflow":
		fmt.Fprint(os.Stdout, strings.Repeat("o", defaultGitOutputLimit+1))
		fmt.Fprint(os.Stderr, "actionable failure\n")
		os.Exit(7)
	case "failure-stderr-overflow":
		fmt.Fprint(os.Stdout, "machine-output\n")
		fmt.Fprint(os.Stderr, "actionable failure\n")
		fmt.Fprint(os.Stderr, strings.Repeat("e", defaultGitStderrLimit+1))
		os.Exit(7)
	case "failure-both-overflow":
		fmt.Fprint(os.Stdout, strings.Repeat("o", defaultGitOutputLimit+1))
		fmt.Fprint(os.Stderr, "actionable failure\n")
		fmt.Fprint(os.Stderr, strings.Repeat("e", defaultGitStderrLimit+1))
		os.Exit(7)
	}
	os.Exit(0)
}
