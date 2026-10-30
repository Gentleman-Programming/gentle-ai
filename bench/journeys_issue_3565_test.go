package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestIssue3565RefreshJourneyRequiresRegistry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("public-process stub requires a POSIX shell; exercised on Linux")
	}
	for _, tc := range []struct {
		name          string
		writeRegistry bool
		wantStatus    string
	}{
		{name: "success message without registry is rejected", wantStatus: StatusFailed},
		{name: "success with project registry is accepted", writeRegistry: true, wantStatus: StatusCompleted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			binary := filepath.Join(t.TempDir(), "gentle-ai-stub")
			script := "#!/bin/sh\nset -eu\ntest \"$*\" = 'skill-registry refresh --cwd .'\n"
			if tc.writeRegistry {
				script += "mkdir -p .atl\nprintf 'registry fixture\\n' > .atl/skill-registry.md\n"
			}
			const stdout = "Skill registry refreshed (1 skills): .atl/skill-registry.md\n"
			script += "printf 'Skill registry refreshed (1 skills): .atl/skill-registry.md\\n'\n"
			if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			journey := issue3565Journeys()[0]
			refresh := journey.Steps[2]
			originalAfter := refresh.After
			var observed Observation
			refresh.After = func(sandbox *Sandbox, observation Observation) error {
				observed = observation
				return originalAfter(sandbox, observation)
			}
			journey.Steps = []Step{journey.Steps[0], refresh}
			result := runJourney(binary, journey)
			if observed.ExitCode != 0 || observed.Stdout != stdout || observed.Stderr != "" {
				t.Fatalf("public command = exit %d stdout %q stderr %q", observed.ExitCode, observed.Stdout, observed.Stderr)
			}
			if result.Status != tc.wantStatus {
				t.Fatalf("journey status = %s (%s), want %s", result.Status, result.FailureReason, tc.wantStatus)
			}
			if !tc.writeRegistry && !strings.Contains(result.FailureReason, "refresh --cwd . did not write the project registry:") {
				t.Fatalf("failure reason = %q, want missing project registry diagnostic", result.FailureReason)
			}
			if tc.writeRegistry && result.FailureReason != "" {
				t.Fatalf("successful journey reported failure: %s", result.FailureReason)
			}
		})
	}
}
