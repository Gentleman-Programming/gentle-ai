package main

// The canonical path-free projection of recorded strings (issue #1866).
//
// A results file must not carry machine-specific paths: two runs of the same
// journey against the same binary, in different sandboxes, must record the
// same strings. These tests pin the closed token vocabulary, its
// longest-path-first replacement order, and the journey records the runner
// produces.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// normalizerTestJourney drives a stub binary with an argv that carries the
// sandbox repository path, so the recorded command has a path to project.
func normalizerTestJourney() Journey {
	return Journey{
		ID: "known",
		// This journey drives a stub binary through the command boundary,
		// not the review lifecycle, so the runner must leave the kill
		// switch alone.
		Review: reviewUntouched,
		Steps: []Step{{
			Name: "invoke test binary",
			Fixture: func(sandbox *Sandbox) error {
				return os.MkdirAll(sandbox.Repo, 0o755)
			},
			Args: func(sandbox *Sandbox) ([]string, error) {
				return []string{"review", "--cwd", sandbox.Repo}, nil
			},
		}},
	}
}

// copyTestBinary copies binary to a different directory, so a second run
// drives the same bytes from a different path.
func copyTestBinary(t *testing.T, binary string) string {
	t.Helper()
	copied := filepath.Join(t.TempDir(), "elsewhere", filepath.Base(binary))
	if err := os.MkdirAll(filepath.Dir(copied), 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(copied, raw, 0o755); err != nil {
		t.Fatal(err)
	}
	return copied
}

func TestPathNormalizerAppliesLongestSpecificTokensFirst(t *testing.T) {
	normalizer := newPathNormalizer("/opt/binaries/gentle-ai", "/tmp/prov-root")
	normalizer.setSandbox("/tmp/prov-root/home", "/tmp/prov-root/home/demo")
	for _, tt := range []struct {
		name string
		in   string
		want string
	}{
		{"repository beats home and run root", "/tmp/prov-root/home/demo/.git/config", RepositoryToken + "/.git/config"},
		{"home beats run root", "/tmp/prov-root/home/.config/x", HomeToken + "/.config/x"},
		{"run root catches the rest", "/tmp/prov-root/tmp/scratch", RunRootToken + "/tmp/scratch"},
		{"target binary exact path", "/opt/binaries/gentle-ai", TargetBinaryToken},
		{"target binary inside an argv string", "exec /opt/binaries/gentle-ai --json", "exec " + TargetBinaryToken + " --json"},
		{"untouched string", "review start --json", "review start --json"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizer.Normalize(tt.in); got != tt.want {
				t.Fatalf("Normalize(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
	var nilNormalizer *PathNormalizer
	if got := nilNormalizer.Normalize("/tmp/x"); got != "/tmp/x" {
		t.Fatalf("nil normalizer changed %q into %q", "/tmp/x", got)
	}
	if got := nilNormalizer.NormalizeAll([]string{"/tmp/x"}); !reflect.DeepEqual(got, []string{"/tmp/x"}) {
		t.Fatalf("nil normalizer changed args: %q", got)
	}
	if got := normalizer.NormalizeAll(nil); got != nil {
		t.Fatalf("NormalizeAll(nil) = %#v, want nil", got)
	}
	if got := normalizer.NormalizeAll([]string{"/tmp/prov-root/home/demo", "plain"}); !reflect.DeepEqual(got, []string{RepositoryToken, "plain"}) {
		t.Fatalf("NormalizeAll = %#v", got)
	}
}

// TestPathNormalizerProjectsTargetBinaryUnderHome pins the CI layout: the
// target binary is built under $RUNNER_TEMP, which lives inside the runner's
// home, so the exact binary path must win over the home that contains it.
func TestPathNormalizerProjectsTargetBinaryUnderHome(t *testing.T) {
	normalizer := newPathNormalizer("/home/runner/work/_temp/gentle-ai", "")
	normalizer.setSandbox("/home/runner", "")
	for _, tt := range []struct{ in, want string }{
		{"/home/runner/work/_temp/gentle-ai", TargetBinaryToken},
		{"exec /home/runner/work/_temp/gentle-ai --version", "exec " + TargetBinaryToken + " --version"},
		{"/home/runner/work/_temp/other", HomeToken + "/work/_temp/other"},
	} {
		if got := normalizer.Normalize(tt.in); got != tt.want {
			t.Errorf("Normalize(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestRunJourneyRecordsTokensInsteadOfMachinePaths pins the runner: a
// recorded command whose argv carries the sandbox repository path records the
// repository token, and no recorded string carries the sandbox root or the
// target binary path.
func TestRunJourneyRecordsTokensInsteadOfMachinePaths(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a temporary executable to exercise the journey runner")
	}

	binary, _ := benchmarkTestBinary(t)
	result := runJourney(binary, normalizerTestJourney())
	if len(result.Commands) != 1 {
		t.Fatalf("expected one recorded command, got %#v", result)
	}
	if want := []string{"review", "--cwd", RepositoryToken}; !reflect.DeepEqual(result.Commands[0].Args, want) {
		t.Errorf("recorded args = %#v, want %#v", result.Commands[0].Args, want)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), binary) {
		t.Errorf("journey result still carries the target binary path:\n%s", encoded)
	}
	if strings.Contains(string(encoded), filepath.Join(os.TempDir(), "gentle-ai-bench-")) {
		t.Errorf("journey result still carries a sandbox path:\n%s", encoded)
	}
}

// TestRunJourneyRecordsIdenticalResultsAcrossSandboxes is the property the
// projection exists for: the same journey driven against two copies of the
// same binary at different paths, each in its own temp sandbox, records the
// same result.
func TestRunJourneyRecordsIdenticalResultsAcrossSandboxes(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a temporary executable to exercise the journey runner")
	}

	binary, _ := benchmarkTestBinary(t)
	first, err := json.Marshal(runJourney(binary, normalizerTestJourney()))
	if err != nil {
		t.Fatal(err)
	}
	second, err := json.Marshal(runJourney(copyTestBinary(t, binary), normalizerTestJourney()))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("journey results differ across sandboxes; the projection is not path-free:\n--- first\n%s\n--- second\n%s", first, second)
	}
}
