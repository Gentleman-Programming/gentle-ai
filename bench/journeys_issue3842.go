package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

// issue3842Journeys pins the replay reconciliation ratified by #3842. An SDD
// attempt acquired with --untracked-scope=select --intended-untracked=<path>
// records that selection immutably, and committing the selected file is the
// ordinary END of the work unit — landing it was the whole point of selecting
// it. Before the fix, every later capture replayed the recorded selection
// verbatim into the snapshot builder, whose "intended-untracked path ... is
// already tracked" refusal then dead-ended reset (and settle) with no exit.
// After the fix, a replayed HISTORICAL selection is reconciled against the
// current index first: a now-tracked path drops out of the overlay while the
// candidate tree stays byte-identical, so reset publishes and the next work
// unit can open. The expectation holds because only replays reconcile — a
// fresh caller-supplied selection of a tracked path keeps failing loudly as
// the scope declaration error it is.
func issue3842Journeys() []Journey {
	return []Journey{{
		ID:     "j124-sdd-attempt-reset-after-selected-untracked-lands",
		Review: reviewOptedIn,
		Title:  "SDD attempt: a landed selected-untracked file no longer dead-ends reset",
		Source: "#3842: replayed intended-untracked selections reconcile against the current index instead of tripping the already-tracked refusal",
		Steps: []Step{
			{Name: "fixture: runtime repository", Fixture: sddRuntimeRepo},
			{Name: "fixture: selected untracked candidate", Fixture: sddSelectedUntrackedCandidate},
			{Name: "acquire selected scope and settle the objective passed",
				Requires: sddSelectedUntrackedCapability, Composite: issue3842AcquireAndComplete},
			{Name: "fixture: the selected file lands in a commit", Fixture: issue3842CommitSelectedFile},
			{Name: "reset the completed objective with its exact revision",
				Requires: sddAttemptResetCapability, Composite: issue3842ResetPublishes},
		},
	}}
}

// issue3842AcquireAndComplete mirrors the selected-untracked acquire shape of
// j84: read the canonical untracked inventory from review status, acquire with
// the explicit selection, make a small tracked edit so the passed settle is
// legitimate work, and settle the objective to completion.
func issue3842AcquireAndComplete(r *journeyRun) error {
	selection, err := readStatusForContract(r, reviewContractV2)
	if err != nil {
		return err
	}
	digest := selection.argument("expected_untracked_inventory")
	if digest == "" {
		return fmt.Errorf("review status did not publish the canonical untracked inventory: %+v", selection.NextTransition)
	}
	acquire := r.run([]string{
		"sdd-attempt", "acquire", "--cwd", r.sandbox.Repo, "--change", sddChange, "--request-id", "issue3842-acquire",
		"--work-unit", "land the selected untracked file", "--evidence-goal", "prove the selected work unit completes",
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
	if err := r.sandbox.write(filepath.Join(r.sandbox.Repo, "docs", "selected.md"), "final selected candidate\n"); err != nil {
		return err
	}
	settled := r.run(append([]string{
		"sdd-attempt", "settle", "--cwd", r.sandbox.Repo, "--change", sddChange, "--token", claimed.Token,
		"--request-id", "issue3842-settle-passed", "--outcome", "passed", "--evidence-revision", sddCorrectedEvidence,
	}, sddTerminalEvidence...), false)
	var result sddCompactAttemptResult
	if err := json.Unmarshal([]byte(settled.Stdout), &result); err != nil || settled.ExitCode != 0 || result.State != "complete" {
		return fmt.Errorf("selected passed settle = %#v parse=%v exit=%d stderr=%s", result, err, settled.ExitCode, firstLine(settled.Stderr))
	}
	status, err := proveRuntime(r.sandbox)
	if err != nil {
		return err
	}
	if !status.Complete || status.ActiveAttempt != nil || status.NextAction != "complete" {
		return fmt.Errorf("completed selected objective status = %#v", status)
	}
	return nil
}

// issue3842CommitSelectedFile is the exact user gesture #3842 dead-ended on:
// the selected untracked file becomes tracked because the work unit landed.
func issue3842CommitSelectedFile(sandbox *Sandbox) error {
	tracked, err := gitOut(sandbox, sandbox.Repo, "ls-files", "docs/selected.md")
	if err != nil {
		return err
	}
	if tracked != "" {
		return fmt.Errorf("fixture expects the selected file untracked before the landing commit, git tracks %q", tracked)
	}
	if err := sandbox.git(sandbox.Repo, "add", "-A"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "commit", "-qm", "land the selected work unit"); err != nil {
		return err
	}
	tracked, err = gitOut(sandbox, sandbox.Repo, "ls-files", "docs/selected.md")
	if err != nil {
		return err
	}
	if tracked != "docs/selected.md" {
		return fmt.Errorf("fixture claims the selected file landed but git tracks %q", tracked)
	}
	return nil
}

// issue3842ResetPublishes drives the exact dead-end of #3842: reset the
// completed objective AFTER its selected file became tracked. Before the fix
// this invocation failed with "intended-untracked path docs/selected.md is
// already tracked"; after it, the replayed selection reconciles and the reset
// publishes a new revision that opens the next work unit.
func issue3842ResetPublishes(r *journeyRun) error {
	status, err := readRuntimeStatus(r)
	if err != nil {
		return err
	}
	if !status.Complete || status.Revision == "" {
		return fmt.Errorf("pre-reset runtime status = %#v", status)
	}
	reset := r.run(sddAttemptArgs(r, "reset", status.Revision, "issue3842-reset",
		"--reason", "the selected work unit landed; open the next objective", "--actor", "bench"), false)
	if reset.ExitCode != 0 {
		return fmt.Errorf("reset after the selected file landed exited %d (the #3842 dead-end): %s",
			reset.ExitCode, firstLine(reset.Stderr, reset.Stdout))
	}
	var after struct {
		Revision      string    `json:"revision"`
		Complete      bool      `json:"complete"`
		NextAction    string    `json:"next_action"`
		ActiveAttempt *struct{} `json:"active_attempt"`
		LastReset     *struct {
			Revision            string `json:"revision"`
			PreviousObjectiveID string `json:"previous_objective_id"`
		} `json:"last_reset"`
	}
	if err := proveJSON(r.sandbox, &after, "sdd-attempt", "status", "--cwd", r.sandbox.Repo, "--change", sddChange); err != nil {
		return err
	}
	if after.Revision == "" || after.Revision == status.Revision || after.Complete || after.ActiveAttempt != nil ||
		after.NextAction != "begin" || after.LastReset == nil || after.LastReset.Revision != after.Revision {
		return fmt.Errorf("reset did not publish a fresh objective scope: %#v (previous revision %s)", after, status.Revision)
	}
	return nil
}
