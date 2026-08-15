package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// issue3094Journeys proves the public runtime contract from #3094: an
// interrupted attempt has no caller-supplied evidence revision, still closes
// exactly once, and an idempotent replay does not publish a second terminal
// record.
func issue3094Journeys() []Journey {
	return []Journey{{
		ID:     "j103-sdd-interrupted-settlement-omits-evidence",
		Title:  "Interrupted runtime settlement omits evidence and replays without a second record",
		Source: "https://github.com/Gentleman-Programming/gentle-ai/issues/3094",
		Steps: []Step{
			{Name: "fixture: repository with a committed OpenSpec change", Fixture: sddRuntimeRepo},
			{Name: "acquire through the public compact CLI and retain its token", Requires: sddAttemptAcquireCapability, Composite: issue3094Acquire},
			{Name: "observe the active running attempt with empty evidence", Requires: sddAttemptAcquireCapability, Composite: issue3094ObserveActive},
			{Name: "settle interrupted without caller evidence", Requires: sddAttemptSettleCapability, Composite: issue3094SettleInterrupted},
			{Name: "verify no active attempt and one empty-evidence terminal record", Requires: sddAttemptSettleCapability, Composite: issue3094VerifyTerminal},
			{Name: "replay the identical compact settlement without publishing another record", Requires: sddAttemptSettleCapability, Composite: issue3094ReplaySettlement},
		},
	}}
}

func issue3094Run(r *journeyRun, args []string) (sddRuntimeStatus, error) {
	result := r.run(args, false)
	if result.ExitCode != 0 {
		return sddRuntimeStatus{}, fmt.Errorf("CLI %v failed: %s", args, firstLine(result.Stderr))
	}
	var status sddRuntimeStatus
	if err := json.Unmarshal([]byte(result.Stdout), &status); err != nil {
		return status, fmt.Errorf("parse CLI %v: %w", args, err)
	}
	return status, nil
}

func issue3094Acquire(r *journeyRun) error {
	objective := append([]string{}, sddObjective[:len(sddObjective)-4]...)
	objective = append(objective, "--max-attempts", "2", "--max-changed-lines", "20")
	args := append([]string{"sdd-attempt", "acquire", "--cwd", r.sandbox.Repo, "--change", sddChange, "--request-id", "issue3094-acquire"}, objective...)
	result := r.run(args, false)
	if result.ExitCode != 0 {
		return fmt.Errorf("CLI %v failed: %s", args, firstLine(result.Stderr))
	}
	var acquired sddCompactAttemptResult
	if err := json.Unmarshal([]byte(result.Stdout), &acquired); err != nil {
		return fmt.Errorf("parse CLI %v: %w", args, err)
	}
	if acquired.State != "proceed" || acquired.Token == "" {
		return fmt.Errorf("acquire result = %#v, want proceed with a token", acquired)
	}
	return r.sandbox.write(filepath.Join(r.sandbox.Repo, ".issue3094-token"), acquired.Token)
}

func issue3094ObserveActive(r *journeyRun) error {
	status, err := readRuntimeStatus(r)
	if err != nil {
		return err
	}
	if status.ActiveAttempt == nil || status.ActiveAttempt.Outcome != "running" || status.EvidenceRevision != "" {
		return fmt.Errorf("active status = %#v, want running with empty evidence", status)
	}
	return nil
}

func issue3094SettleInterrupted(r *journeyRun) error {
	tokenBytes, err := os.ReadFile(filepath.Join(r.sandbox.Repo, ".issue3094-token"))
	if err != nil {
		return err
	}
	token := string(tokenBytes)
	status, err := readRuntimeStatus(r)
	if err != nil {
		return err
	}
	legacyEvidence := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	interruptedArgs := append([]string{"--outcome", "interrupted", "--evidence-revision", legacyEvidence}, sddTerminalEvidence...)
	interruptedArgs = append([]string{"sdd-attempt", "settle", "--cwd", r.sandbox.Repo, "--change", sddChange, "--token", token, "--request-id", "issue3094-settle"}, interruptedArgs...)
	observation := r.run(interruptedArgs, false)
	if observation.ExitCode == 0 {
		return fmt.Errorf("interrupted settle with caller evidence was accepted")
	}
	unchanged, err := readRuntimeStatus(r)
	if err != nil || unchanged.Revision != status.Revision || unchanged.ActiveAttempt == nil {
		return fmt.Errorf("evidence-bearing interrupted refusal mutated runtime: before=%#v after=%#v err=%v", status, unchanged, err)
	}
	settleArgs := append([]string{"sdd-attempt", "settle", "--cwd", r.sandbox.Repo, "--change", sddChange, "--token", token, "--request-id", "issue3094-settle", "--outcome", "interrupted"}, sddTerminalEvidence...)
	_, err = issue3094Run(r, settleArgs)
	return err
}

func issue3094VerifyTerminal(r *journeyRun) error {
	status, err := readRuntimeStatus(r)
	if err != nil {
		return err
	}
	if status.ActiveAttempt != nil || status.EvidenceRevision != "" || len(status.Attempts) != 1 || status.Attempts[0].Outcome != "interrupted" || status.Attempts[0].EvidenceRevision != "" {
		return fmt.Errorf("terminal status = %#v, want one interrupted empty-evidence record", status)
	}
	return nil
}

func issue3094ReplaySettlement(r *journeyRun) error {
	status, err := readRuntimeStatus(r)
	if err != nil {
		return err
	}
	tokenBytes, err := os.ReadFile(filepath.Join(r.sandbox.Repo, ".issue3094-token"))
	if err != nil {
		return err
	}
	args := append([]string{"sdd-attempt", "settle", "--cwd", r.sandbox.Repo, "--change", sddChange, "--token", string(tokenBytes), "--request-id", "issue3094-settle", "--outcome", "interrupted"}, sddTerminalEvidence...)
	if _, err = issue3094Run(r, args); err != nil {
		return fmt.Errorf("replayed settlement was not idempotent: %w", err)
	}
	final, err := readRuntimeStatus(r)
	if err != nil {
		return err
	}
	if len(final.Attempts) != 1 || final.Revision != status.Revision || final.ActiveAttempt != nil || final.EvidenceRevision != "" {
		return fmt.Errorf("replayed status = %#v, want unchanged one-record terminal state", final)
	}
	return nil
}
