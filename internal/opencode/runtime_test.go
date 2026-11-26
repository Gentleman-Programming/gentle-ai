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
func TestRuntimeProvidesPluginSDK(t *testing.T) {
	for _, tc := range []struct {
		version string
		want    bool
	}{
		{"1.18.30", false}, {"2.0.4", false}, {"2.0.24", false},
		{"2.0.25", true}, {"2.3.0", true}, {"2.10.0", true},
		{"opencode v2.3.0\n", true}, {"OpenCode 2.3.0", true},
		{"2.00.025", true}, {"2.00.024", false},
		{"2.18446744073709551615.0", true}, {"2.0.18446744073709551615", true},
		{"2.18446744073709551616.0", false}, {"2.1.18446744073709551616", false},
		{"", false}, {"3.0.0", false}, {"2.3.0-rc.1", false}, {"failure 2.3.0", false},
	} {
		t.Run(tc.version, func(t *testing.T) {
			if got := parseRuntimeVersion(tc.version).ProvidesPluginSDK(); got != tc.want {
				t.Errorf("host SDK capability = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDetectedRuntimeSDKCapabilityDoesNotDependOnVersionText(t *testing.T) {
	old := VersionRunnerOverride
	t.Cleanup(func() { VersionRunnerOverride = old })
	for _, tc := range []struct {
		detected, replacement string
		want                  bool
	}{
		{"2.0.24", "2.0.25", false},
		{"2.0.25", "2.0.24", true},
	} {
		t.Run(tc.detected, func(t *testing.T) {
			VersionRunnerOverride = func(context.Context, Command) (CommandOutput, error) {
				return CommandOutput{Stdout: []byte(tc.detected)}, nil
			}
			version, err := DetectRuntimeEvidence(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			version.Version = tc.replacement
			if got := version.ProvidesPluginSDK(); got != tc.want {
				t.Errorf("host SDK capability = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDetectRuntimeVersionRetainsEvidenceWithOneProbe(t *testing.T) {
	old := VersionRunnerOverride
	t.Cleanup(func() { VersionRunnerOverride = old })
	calls := 0
	VersionRunnerOverride = func(ctx context.Context, command Command) (CommandOutput, error) {
		calls++
		if _, bounded := ctx.Deadline(); !bounded || command.Path != "opencode" || len(command.Args) != 1 || command.Args[0] != "--version" || command.OutputLimit != 4096 {
			t.Fatalf("unsafe version probe: %+v", command)
		}
		return CommandOutput{Stdout: []byte("opencode v2.3.0\n")}, nil
	}
	version, err := DetectRuntimeVersion(context.Background())
	if err != nil || version != (RuntimeVersion{Major: RuntimeV2, Version: "2.3.0"}) || calls != 1 {
		t.Fatalf("version = %+v, error = %v, probes = %d", version, err, calls)
	}
	calls = 0
	if major, err := DetectRuntimeMajor(context.Background()); major != RuntimeV2 || err != nil || calls != 1 {
		t.Fatalf("major = %v, error = %v, probes = %d", major, err, calls)
	}
}

func TestDetectRuntimeVersionPreservesLargeComponents(t *testing.T) {
	old := VersionRunnerOverride
	t.Cleanup(func() { VersionRunnerOverride = old })
	for _, tc := range []struct {
		version string
		major   RuntimeMajor
	}{
		{"1.18446744073709551616.0", RuntimeV1},
		{"2.0.18446744073709551616", RuntimeV2},
	} {
		t.Run(tc.version, func(t *testing.T) {
			calls := 0
			VersionRunnerOverride = func(context.Context, Command) (CommandOutput, error) {
				calls++
				return CommandOutput{Stdout: []byte(tc.version)}, nil
			}
			if ParseRuntimeMajor(tc.version) != tc.major {
				t.Fatal("large components changed runtime major selection")
			}
			version, err := DetectRuntimeEvidence(context.Background())
			if err != nil || version.Major != tc.major || version.Version != tc.version || version.ProvidesPluginSDK() || calls != 1 {
				t.Fatalf("version = %+v, error = %v, probes = %d", version, err, calls)
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
		return CommandOutput{Stdout: []byte("OpenCode v2.4.1\n")}, nil
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
