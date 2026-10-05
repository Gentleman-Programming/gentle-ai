package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Community issue #1883: a corpus run with failed journeys exited 0, so a CI
// gate reading the exit code saw success in a run that measured nothing for
// the failed rows. runExitCode is the guard; these tests are what makes its
// deletion loud. They drive the same aggregate() path a real run takes, so a
// synthetic failing journey stands in for the real thing end to end.

func resultsWith(journeys ...JourneyResult) Results {
	results := Results{Journeys: journeys}
	results.Totals, results.JourneysCounted, results.JourneysUnsupported, results.JourneysFailed = aggregate(journeys)
	return results
}

func TestRunExitsNonzeroWhenAnyJourneyFailed(t *testing.T) {
	results := resultsWith(
		JourneyResult{ID: "ok", Status: StatusCompleted},
		JourneyResult{ID: "broken", Status: StatusFailed, FailureReason: "synthetic"},
	)
	if runExitCode(results) == 0 {
		t.Fatal("a run with a failed journey exited 0; a gate reading that exit sees success in a run that measured nothing for the failed row (issue #1883)")
	}
}

// `unsupported` must NOT fail the run: driving an older binary is a designed
// use, and "this build lacks that surface" is a real, honestly-labelled
// measurement. Failing on it would make cross-version comparison impossible
// to script. The distinction stays visible in the summary line and the table,
// where an unsupported journey renders as `unsup`, never as a number.
func TestRunExitsZeroWhenJourneysAreOnlyUnsupported(t *testing.T) {
	results := resultsWith(
		JourneyResult{ID: "ok", Status: StatusCompleted},
		JourneyResult{ID: "older-binary", Status: StatusUnsupported},
	)
	if exit := runExitCode(results); exit != 0 {
		t.Fatalf("a run with only unsupported journeys exited %d; unsupported is a measurement about the binary, not a failure of the run", exit)
	}
}

func TestRunExitsZeroWhenEverythingCompleted(t *testing.T) {
	results := resultsWith(JourneyResult{ID: "ok", Status: StatusCompleted})
	if exit := runExitCode(results); exit != 0 {
		t.Fatalf("a clean run exited %d", exit)
	}
}

func TestExecutableForGOOSUsesPlatformNativeExecutableRules(t *testing.T) {
	if !executableMode(0o644, "windows") {
		t.Fatal("Windows executable preflight rejected a regular executable file without Unix mode bits")
	}
	if executableMode(0o644, "linux") {
		t.Fatal("Unix executable preflight accepted a file without execute bits")
	}
	if !executableMode(0o755, "linux") {
		t.Fatal("Unix executable preflight rejected a file with execute bits")
	}
}

func TestCommandRunSelection(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a temporary executable to exercise the benchmark command boundary")
	}

	binary, invocations := benchmarkTestBinary(t)
	journeys := func() []Journey {
		return []Journey{{
			ID: "known",
			// This journey drives a stub binary through the command
			// boundary, not the review lifecycle, so the runner must leave
			// the kill switch alone.
			Review: reviewUntouched,
			Steps: []Step{{
				Name: "invoke test binary",
				Fixture: func(sandbox *Sandbox) error {
					return os.MkdirAll(sandbox.Repo, 0o755)
				},
				Args: func(*Sandbox) ([]string, error) {
					return []string{"review"}, nil
				},
			}},
		}}
	}
	for _, tt := range []struct {
		name          string
		only          string
		includeOnly   bool
		wantExit      int
		wantJourneys  int
		wantJourneyID string
		wantFailure   bool
	}{
		{name: "omitted selector runs the default corpus", wantJourneys: 1, wantJourneyID: "known"},
		{name: "empty selector runs the default corpus", only: " , ", includeOnly: true, wantJourneys: 1, wantJourneyID: "known"},
		{name: "one exact known id runs one journey", only: "known", includeOnly: true, wantJourneys: 1, wantJourneyID: "known"},
		{name: "unknown selector fails with a diagnostic envelope", only: "does-not-exist", includeOnly: true, wantExit: 1, wantFailure: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := os.WriteFile(invocations, nil, 0o644); err != nil {
				t.Fatalf("reset invocation log: %v", err)
			}
			out := filepath.Join(t.TempDir(), "results.json")
			args := []string{"--binary", binary, "--out", out}
			if tt.includeOnly {
				args = append(args, "--only", tt.only)
			}
			if exit := commandRunWith(args, func(string) bool { return true }, journeys); exit != tt.wantExit {
				t.Fatalf("commandRunWith() exit = %d, want %d", exit, tt.wantExit)
			}

			content, err := os.ReadFile(out)
			if err != nil {
				t.Fatalf("read results: %v", err)
			}
			var results Results
			if err := json.Unmarshal(content, &results); err != nil {
				t.Fatalf("decode results: %v", err)
			}
			if tt.wantFailure {
				if got := strings.Join(results.RequestedSelectors, ","); got != tt.only {
					t.Fatalf("requested_selectors = %q, want %q", got, tt.only)
				}
				if results.ResolvedIDs == nil || len(*results.ResolvedIDs) != 0 {
					t.Fatalf("resolved_ids = %#v, want []", results.ResolvedIDs)
				}
				if results.RunStatus != "failed" {
					t.Fatalf("run_status = %q, want failed", results.RunStatus)
				}
				if results.FailureReason != "empty_selected_population" {
					t.Fatalf("failure_reason = %q, want empty_selected_population", results.FailureReason)
				}
				calls, err := os.ReadFile(invocations)
				if err != nil {
					t.Fatalf("read invocation log: %v", err)
				}
				if got := strings.TrimSpace(string(calls)); got != "--version" {
					t.Fatalf("journey execution calls = %q, want only --version", got)
				}
				return
			}
			if len(results.Journeys) != tt.wantJourneys {
				t.Fatalf("journeys = %d, want %d", len(results.Journeys), tt.wantJourneys)
			}
			if tt.wantJourneyID != "" && results.Journeys[0].ID != tt.wantJourneyID {
				t.Fatalf("journey id = %q, want %q", results.Journeys[0].ID, tt.wantJourneyID)
			}
		})
	}
}

func benchmarkTestBinary(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	invocations := filepath.Join(dir, "invocations.log")
	source := filepath.Join(dir, "main.go")
	binary := filepath.Join(dir, "gentle-ai-test.exe")
	program := `package main
import (
	"fmt"
	"os"
	"strings"
)
func main() {
	args := strings.Join(os.Args[1:], " ")
	f, _ := os.OpenFile(` + strconv.Quote(invocations) + `, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if f != nil {
		_, _ = fmt.Fprintln(f, args)
		_ = f.Close()
	}
	if args == "--version" {
		fmt.Println("test")
		return
	}
	fmt.Fprintln(os.Stderr, ` + strconv.Quote(`unknown review command "review"`) + `)
	os.Exit(1)
}
`
	if err := os.WriteFile(source, []byte(program), 0o644); err != nil {
		t.Fatalf("write test binary source: %v", err)
	}
	if output, err := exec.Command("go", "build", "-o", binary, source).CombinedOutput(); err != nil {
		t.Fatalf("build test binary: %v\n%s", err, output)
	}
	return binary, invocations
}
