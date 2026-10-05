package opencode

import (
	"context"
	"errors"
	"testing"
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
