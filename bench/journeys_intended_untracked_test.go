package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestIntendedUntrackedCompositesRejectMissingNextTransition(t *testing.T) {
	for _, test := range []struct {
		name      string
		composite func(*journeyRun) error
		overlay   bool
	}{
		{name: "plain", composite: selectIntendedUntrackedAndRunPrintedStart},
		{name: "overlay", composite: selectOverlayIntendedUntrackedAndRunPrintedStart, overlay: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			output := `{"next_transition":null}`
			if test.overlay {
				output = `{"projection":{"kind":"base-workspace-overlay"},"next_transition":null}`
			}
			run := malformedIntendedUntrackedJourneyRun(t, output, test.overlay)
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("composite panicked on missing next_transition: %v", recovered)
				}
			}()
			if err := test.composite(run); err == nil || !strings.Contains(err.Error(), "next_transition") {
				t.Fatalf("composite error = %v, want a diagnostic next_transition error", err)
			}
		})
	}
}

func TestIntendedUntrackedTransitionGuardsRejectNilBranches(t *testing.T) {
	for _, test := range []struct {
		name, kind, payload, want string
	}{
		{name: "nil collect", kind: "collect", payload: `{"next_transition":{"kind":"collect","collect":null}}`, want: "collect transition is missing"},
		{name: "nil execute", kind: "execute", payload: `{"next_transition":{"kind":"execute","execute":null}}`, want: "execute transition is missing"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var status waveCorrectionStatus
			if err := json.Unmarshal([]byte(test.payload), &status); err != nil {
				t.Fatal(err)
			}
			var err error
			if test.kind == "collect" {
				_, err = intendedUntrackedCollectSubmission(status, test.name)
			} else {
				err = intendedUntrackedExecuteTransition(status, test.name)
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("guard error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestIntendedUntrackedSubmissionArgumentsExpandRepeatedPathsInOrder(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "repo")
	descriptor := waveSubmissionDescriptor{
		OperationToken: "status",
		ArgumentTokens: []string{
			"--contract=" + reviewContractV2,
			"--next-transition=true",
			"--cwd={{cwd}}",
			"--projection=workspace",
			"--untracked-scope={{untracked_scope}}",
			"--expected-untracked-inventory=sha256:" + strings.Repeat("a", 64),
			"--intended-untracked={{intended_untracked}}",
		},
		Values: []waveSubmissionValue{
			{Slot: "cwd", SubstitutionLocation: 2},
			{Slot: "untracked_scope", SubstitutionLocation: 4},
			{Slot: "intended_untracked", SubstitutionLocation: 6, Repeated: true},
		},
	}
	got, err := intendedUntrackedSubmissionArguments(&journeyRun{sandbox: &Sandbox{Repo: repo}}, descriptor, "select", []string{"first path", "second,path"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"review", "status",
		"--contract=" + reviewContractV2,
		"--next-transition=true",
		"--cwd=" + repo,
		"--projection=workspace",
		"--untracked-scope=select",
		"--expected-untracked-inventory=sha256:" + strings.Repeat("a", 64),
		"--intended-untracked=first path",
		"--intended-untracked=second,path",
	}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("expanded submission = %v, want %v", got, want)
	}
}

func malformedIntendedUntrackedJourneyRun(t *testing.T, output string, overlay bool) *journeyRun {
	t.Helper()
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if overlay {
		for _, args := range [][]string{
			{"-C", repo, "init", "-q"},
			{"-C", repo, "config", "user.email", "bench@example.invalid"},
			{"-C", repo, "config", "user.name", "bench"},
			{"-C", repo, "commit", "--allow-empty", "-qm", "fixture"},
		} {
			if err := exec.Command("git", args...).Run(); err != nil {
				t.Fatalf("git %v: %v", args, err)
			}
		}
	}
	binary := filepath.Join(root, "fake-gentle-ai")
	script := "#!/bin/sh\nprintf '%s\\n' '" + output + "'\n"
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return &journeyRun{
		sandbox: &Sandbox{
			Binary:    binary,
			Root:      root,
			Home:      filepath.Join(root, "home"),
			Repo:      repo,
			TracePath: filepath.Join(root, "git-trace.log"),
			Scratch:   map[string]string{},
		},
		accumulator: newAccumulator(),
		step:        "malformed intended-untracked transition",
	}
}
