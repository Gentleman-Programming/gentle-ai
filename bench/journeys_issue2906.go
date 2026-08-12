package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var finalizeActionEligibilityPreflightCapability = &Capability{
	Verb:  []string{"review", "finalize"},
	Flags: []string{"--action-eligibility", "--next-transition", "--contract", "--cwd"},
}

// issue2906Journeys proves the missing contract is rejected before FINALIZE
// can inspect or mutate an authority, and never creates a defect report.
func issue2906Journeys() []Journey {
	return []Journey{{
		ID:     "j99-issue-2906-finalize-missing-contract",
		Title:  "FINALIZE action outputs without a contract are preflight refusals",
		Source: "#2906: missing FINALIZE --contract is input validation, not an unknown outcome",
		Steps: []Step{
			{Name: "fixture: repo", Fixture: baseRepo},
			{
				Name:      "action eligibility and next transition require a contract",
				Requires:  finalizeActionEligibilityPreflightCapability,
				Composite: assertIssue2906MissingContractPreflight,
			},
		},
	}}
}

func assertIssue2906MissingContractPreflight(run *journeyRun) error {
	cases := []struct {
		name  string
		flags []string
	}{
		{name: "action eligibility only", flags: []string{"--action-eligibility"}},
		{name: "next transition only", flags: []string{"--next-transition"}},
		{name: "both outputs", flags: []string{"--action-eligibility", "--next-transition"}},
	}
	for _, test := range cases {
		caseRoot, err := os.MkdirTemp(run.sandbox.Root, "issue2906-missing-contract-")
		if err != nil {
			return fmt.Errorf("%s create isolated sandbox: %w", test.name, err)
		}
		defer func() { _ = os.RemoveAll(caseRoot) }()
		caseSandbox, err := newSandbox(run.sandbox.Binary, caseRoot)
		if err != nil {
			return fmt.Errorf("%s create isolated sandbox: %w", test.name, err)
		}
		if err := baseRepo(caseSandbox); err != nil {
			return fmt.Errorf("%s create isolated repository: %w", test.name, err)
		}
		gitBefore, err := gitOut(caseSandbox, caseSandbox.Repo, "status", "--porcelain=v1")
		if err != nil {
			return fmt.Errorf("%s inspect repository before invocation: %w", test.name, err)
		}
		headBefore, err := gitOut(caseSandbox, caseSandbox.Repo, "rev-parse", "HEAD")
		if err != nil {
			return fmt.Errorf("%s inspect repository head before invocation: %w", test.name, err)
		}
		caseRun := &journeyRun{sandbox: caseSandbox, probe: newCapabilityProbe(caseSandbox), accumulator: run.accumulator, step: run.step}
		args := append([]string{"review", "finalize"}, test.flags...)
		args = append(args, "--cwd", caseSandbox.Repo)
		observation := caseRun.run(args, false)
		if observation.ExitCode == 0 {
			return fmt.Errorf("%s accepted missing --contract", test.name)
		}
		want := "Error: --action-eligibility and --next-transition require --contract " + reviewContract
		if !strings.Contains(observation.Stderr, want) {
			return fmt.Errorf("%s diagnostic = %q, want %q", test.name, strings.TrimSpace(observation.Stderr), want)
		}
		if strings.Contains(observation.Stdout+observation.Stderr, "operation_outcome_unknown") ||
			strings.Contains(observation.Stdout+observation.Stderr, "defect report") {
			return fmt.Errorf("%s emitted generic fault or defect-report text", test.name)
		}
		gitAfter, err := gitOut(caseSandbox, caseSandbox.Repo, "status", "--porcelain=v1")
		if err != nil {
			return fmt.Errorf("%s inspect repository after invocation: %w", test.name, err)
		}
		headAfter, err := gitOut(caseSandbox, caseSandbox.Repo, "rev-parse", "HEAD")
		if err != nil {
			return fmt.Errorf("%s inspect repository head after invocation: %w", test.name, err)
		}
		if gitAfter != gitBefore || headAfter != headBefore {
			return fmt.Errorf("%s mutated repository: status %q -> %q, HEAD %q -> %q", test.name, gitBefore, gitAfter, headBefore, headAfter)
		}
		authorityRoot := filepath.Join(caseSandbox.Repo, ".git", "gentle-ai")
		if _, err := os.Stat(authorityRoot); !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%s created authority state: %v", test.name, err)
		}
		reportDir := filepath.Join(authorityRoot, "defect-reports")
		if _, err := os.Stat(reportDir); !errors.Is(err, os.ErrNotExist) {
			if err == nil {
				return fmt.Errorf("%s wrote a defect report", test.name)
			}
			return fmt.Errorf("%s inspect defect reports: %w", test.name, err)
		}
	}
	return nil
}
