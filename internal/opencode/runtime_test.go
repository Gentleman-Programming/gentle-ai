package opencode

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRuntimeMajor(t *testing.T) {
	for _, tt := range []struct {
		version string
		want    int
	}{{"1.18.30", 1}, {"2.0.4", 2}, {"opencode 2.0.4", 2}, {"opencode v2.0.4", 2}, {"2.0.4\n", 2}, {"3.0.0", 0}, {"2.0.4-beta.1", 0}, {"failure 2.0.4", 0}, {"", 0}} {
		t.Run(tt.version, func(t *testing.T) {
			if got := ParseRuntimeMajor(tt.version); int(got) != tt.want {
				t.Fatalf("major %v", got)
			}
		})
	}
}
func TestDetectRuntimeMajorBounded(t *testing.T) {
	old := VersionRunnerOverride
	t.Cleanup(func() { VersionRunnerOverride = old })
	VersionRunnerOverride = func(ctx context.Context, cmd Command) (CommandOutput, error) {
		if cmd.Path != "opencode" || len(cmd.Args) != 1 || cmd.Args[0] != "--version" || cmd.OutputLimit != 4096 {
			t.Fatalf("unsafe probe %+v", cmd)
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("unbounded probe")
		}
		return CommandOutput{Stdout: []byte("2.0.4")}, nil
	}
	if got, err := DetectRuntimeMajor(context.Background()); got != RuntimeV2 || err != nil {
		t.Fatalf("%v %v", got, err)
	}
	VersionRunnerOverride = func(context.Context, Command) (CommandOutput, error) { return CommandOutput{}, errors.New("missing") }
	if _, err := DetectRuntimeMajor(context.Background()); err == nil {
		t.Fatal("unknown version accepted")
	}
}

func TestDetectRuntimeMajorAllowsSlowSupportedRuntime(t *testing.T) {
	old := VersionRunnerOverride
	t.Cleanup(func() { VersionRunnerOverride = old })
	for _, tt := range []struct {
		version string
		want    RuntimeMajor
	}{
		{"1.18.31\n", RuntimeV1},
		{"2.0.4\n", RuntimeV2},
	} {
		t.Run(tt.version, func(t *testing.T) {
			VersionRunnerOverride = func(ctx context.Context, _ Command) (CommandOutput, error) {
				timer := time.NewTimer(3500 * time.Millisecond)
				defer timer.Stop()
				select {
				case <-timer.C:
					return CommandOutput{Stdout: []byte(tt.version)}, nil
				case <-ctx.Done():
					return CommandOutput{}, ctx.Err()
				}
			}
			if got, err := DetectRuntimeMajor(context.Background()); got != tt.want || err != nil {
				t.Fatalf("slow supported runtime = %v, %v; want %v, nil", got, err, tt.want)
			}
		})
	}
}

// A probe that outlives its deadline is reported as a timeout, distinct from
// an absent or unsupported runtime, through the real deadline path.
func TestDetectRuntimeMajorReportsTimeoutDistinctly(t *testing.T) {
	oldRunner, oldTimeout := VersionRunnerOverride, runtimeVersionTimeout
	t.Cleanup(func() { VersionRunnerOverride, runtimeVersionTimeout = oldRunner, oldTimeout })
	runtimeVersionTimeout = 10 * time.Millisecond
	VersionRunnerOverride = func(ctx context.Context, _ Command) (CommandOutput, error) {
		<-ctx.Done()
		return CommandOutput{}, errors.New("signal: killed")
	}
	if got, err := DetectRuntimeMajor(context.Background()); got != RuntimeUnknown || !errors.Is(err, ErrRuntimeVersionTimeout) || err.Error() != "`opencode --version` timed out after 10ms; managed runtime assets were not selected" {
		t.Fatalf("timed-out probe = %v, %v; want unknown runtime and explicit 10ms timeout", got, err)
	}
	VersionRunnerOverride = func(context.Context, Command) (CommandOutput, error) { return CommandOutput{}, errors.New("missing") }
	if _, err := DetectRuntimeMajor(context.Background()); err == nil || errors.Is(err, ErrRuntimeVersionTimeout) {
		t.Fatalf("missing runtime error = %v, want a non-timeout failure", err)
	}
}
