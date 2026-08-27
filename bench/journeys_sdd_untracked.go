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
		Revision string `json:"revision"`
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
	rescoped := r.run([]string{
		"sdd-attempt", "rescope", "--cwd", r.sandbox.Repo, "--change", sddChange, "--expected-revision", status.Revision,
		"--request-id", "bench-selected-rescope", "--work-unit", "selected untracked continuation",
		"--evidence-goal", "prove the rescope successor preserves selected bytes", "--max-attempts", "2", "--max-changed-lines", "20",
		"--reason", "maintainer narrowed the failed selected-untracked objective", "--actor", "bench",
	}, false)
	if rescoped.ExitCode != 0 {
		return fmt.Errorf("selected rescope = exit=%d stderr=%s", rescoped.ExitCode, firstLine(rescoped.Stderr))
	}
	admission := r.run([]string{
		"sdd-attempt", "status", "--cwd", r.sandbox.Repo, "--change", sddChange,
		"--work-unit", "selected untracked continuation", "--evidence-goal", "prove the rescope successor preserves selected bytes",
		"--max-attempts", "2", "--max-changed-lines", "20",
	}, false)
	var continuationStatus struct {
		BlockedReason string `json:"blocked_reason"`
		BlockedExit   string `json:"blocked_exit"`
	}
	if err := json.Unmarshal([]byte(admission.Stdout), &continuationStatus); err != nil || admission.ExitCode != 0 || continuationStatus.BlockedReason != "" || continuationStatus.BlockedExit != "" {
		return fmt.Errorf("declaration-free selected rescope status = %#v parse=%v exit=%d", continuationStatus, err, admission.ExitCode)
	}
	continued := r.run([]string{
		"sdd-attempt", "acquire", "--cwd", r.sandbox.Repo, "--change", sddChange, "--request-id", "bench-selected-acquire-successor",
		"--work-unit", "selected untracked continuation", "--evidence-goal", "prove the rescope successor preserves selected bytes",
		"--max-attempts", "2", "--max-changed-lines", "20",
	}, false)
	var successor sddCompactAttemptResult
	if err := json.Unmarshal([]byte(continued.Stdout), &successor); err != nil || continued.ExitCode != 0 || successor.State != "proceed" || successor.Token == "" {
		return fmt.Errorf("declaration-free selected rescope acquire = %#v parse=%v exit=%d", successor, err, continued.ExitCode)
	}
	var continuedStatus struct {
		ActiveAttempt *struct {
			IntendedUntracked []string `json:"intended_untracked"`
		} `json:"active_attempt"`
	}
	if err := proveJSON(r.sandbox, &continuedStatus, "sdd-attempt", "status", "--cwd", r.sandbox.Repo, "--change", sddChange); err != nil {
		return err
	}
	if continuedStatus.ActiveAttempt == nil || len(continuedStatus.ActiveAttempt.IntendedUntracked) != 1 || continuedStatus.ActiveAttempt.IntendedUntracked[0] != "docs/selected.md" {
		return fmt.Errorf("selected rescope successor swept untracked paths: %#v", continuedStatus)
	}
	return nil
}

func selectedUntrackedSDDJourneys() []Journey {
	return []Journey{{
		ID:     "j84-sdd-attempt-selected-untracked-lifecycle",
		Title:  "SDD attempt: selected untracked scope survives a zero-drift rescope continuation",
		Source: "issues #2716 and #3801: explicit selected scope remains provenance and a fresh rescope successor must continue it without sweeping workspace files",
		Steps: []Step{
			{Name: "fixture: runtime repository", Fixture: sddRuntimeRepo},
			{Name: "fixture: selected untracked candidate", Fixture: sddSelectedUntrackedCandidate},
			{Name: "acquire, settle, and prove selected-path accounting", Requires: sddSelectedUntrackedCapability, Composite: driveSelectedUntrackedSDDAttempt},
		},
	}}
}
