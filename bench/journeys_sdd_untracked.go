package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

var sddSelectedUntrackedCapability = &Capability{
	Verb:  []string{"sdd-attempt", "acquire"},
	Flags: []string{"--untracked-scope", "--expected-untracked-inventory", "--intended-untracked"},
}

func sddSelectedUntrackedCandidate(sandbox *Sandbox) error {
	return sandbox.write(filepath.Join(sandbox.Repo, "docs", "selected.md"), "initial selected candidate\n")
}

func driveSelectedUntrackedSDDAttempt(r *journeyRun) error {
	selection, err := readStatusForContract(r, reviewContractV2)
	if err != nil {
		return err
	}
	digest := selection.argument("expected_untracked_inventory")
	if digest == "" {
		return fmt.Errorf("review status did not publish the canonical untracked inventory: %+v", selection.NextTransition)
	}
	acquire := r.run([]string{
		"sdd-attempt", "acquire", "--cwd", r.sandbox.Repo, "--change", sddChange, "--request-id", "bench-selected-acquire",
		"--work-unit", "selected untracked lifecycle", "--evidence-goal", "account only declared untracked bytes",
		"--max-attempts", "2", "--max-changed-lines", "20", "--untracked-scope", "select",
		"--expected-untracked-inventory", digest, "--intended-untracked", "docs/selected.md",
	}, false)
	var claimed sddCompactAttemptResult
	if err := json.Unmarshal([]byte(acquire.Stdout), &claimed); err != nil || acquire.ExitCode != 0 || claimed.State != "proceed" || claimed.Token == "" {
		return fmt.Errorf("selected acquire = %#v parse=%v exit=%d", claimed, err, acquire.ExitCode)
	}
	if err := r.sandbox.write(filepath.Join(r.sandbox.Repo, "docs", "attempt.md"), "tracked change\n"); err != nil {
		return err
	}
	if err := r.sandbox.write(filepath.Join(r.sandbox.Repo, "docs", "selected.md"), "corrected selected candidate\n"); err != nil {
		return err
	}
	settled := r.run(append([]string{
		"sdd-attempt", "settle", "--cwd", r.sandbox.Repo, "--change", sddChange, "--token", claimed.Token,
		"--request-id", "bench-selected-settle", "--outcome", "failed", "--evidence-revision", sddFailedEvidence,
	}, sddTerminalEvidence...), false)
	var result sddCompactAttemptResult
	if err := json.Unmarshal([]byte(settled.Stdout), &result); err != nil || settled.ExitCode != 0 || result.State != "proceed" {
		return fmt.Errorf("selected settle = %#v parse=%v exit=%d", result, err, settled.ExitCode)
	}
	var status struct {
		Attempts []struct {
			ChangedLines      int      `json:"changed_lines"`
			IntendedUntracked []string `json:"intended_untracked"`
		} `json:"attempts"`
	}
	if err := proveJSON(r.sandbox, &status, "sdd-attempt", "status", "--cwd", r.sandbox.Repo, "--change", sddChange); err != nil {
		return err
	}
	if len(status.Attempts) != 1 || status.Attempts[0].ChangedLines != 6 || len(status.Attempts[0].IntendedUntracked) != 1 || status.Attempts[0].IntendedUntracked[0] != "docs/selected.md" {
		return fmt.Errorf("selected SDD lifecycle status = %#v", status)
	}
	return nil
}

func selectedUntrackedSDDJourneys() []Journey {
	return []Journey{{
		ID:     "j84-sdd-attempt-selected-untracked-lifecycle",
		Title:  "SDD attempt: inventory-selected untracked bytes remain candidate provenance and accounting",
		Source: "issue #2716: SDD must not issue authority for undeclared untracked candidate scope",
		Steps: []Step{
			{Name: "fixture: runtime repository", Fixture: sddRuntimeRepo},
			{Name: "fixture: selected untracked candidate", Fixture: sddSelectedUntrackedCandidate},
			{Name: "acquire, settle, and prove selected-path accounting", Requires: sddSelectedUntrackedCapability, Composite: driveSelectedUntrackedSDDAttempt},
		},
	}}
}
